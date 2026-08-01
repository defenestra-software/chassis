package drm

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	drmlib "github.com/NeowayLabs/drm"
	"github.com/NeowayLabs/drm/mode"

	flatpak "gitlab.com/defenestra-software/go-flatbridge"

	"defenestra.io/chassis/internal/core/domain"
)

type Scanner struct {
	logger *slog.Logger
	cache  []*domain.GPUInfo
}

func NewScanner(logger *slog.Logger) *Scanner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scanner{logger: logger}
}

func (s *Scanner) Refresh() {
	s.cache = nil
}

func (s *Scanner) ScanGPUs() ([]*domain.GPUInfo, error) {
	if s.cache != nil {
		return s.cache, nil
	}

	var gpus []*domain.GPUInfo

	pattern := flatpak.HostPath("/sys/class/drm/card[0-9]*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to glob DRM cards: %w", err)
	}

	cardPattern := regexp.MustCompile(`/card(\d+)$`)

	for _, cardPath := range matches {
		if strings.Contains(filepath.Base(cardPath), "-") {
			continue
		}

		match := cardPattern.FindStringSubmatch(cardPath)
		if match == nil {
			continue
		}

		cardIndex, _ := strconv.Atoi(match[1])
		gpu := s.scanCard(cardIndex, cardPath)
		if gpu != nil {
			gpus = append(gpus, gpu)
		}
	}

	s.cache = gpus
	return gpus, nil
}

func (s *Scanner) scanCard(cardIndex int, sysfsPath string) *domain.GPUInfo {
	gpu := &domain.GPUInfo{
		CardIndex: cardIndex,
		CardPath:  fmt.Sprintf("/dev/dri/card%d", cardIndex),
	}

	gpu.PCIDevice = s.getPCIDevice(sysfsPath)

	gpu.DriverName = s.getDriverName(sysfsPath)

	s.getDriverVersion(gpu)

	s.getHardwareInfo(gpu, sysfsPath)

	gpu.Monitors = s.scanConnectors(cardIndex)

	s.logger.Debug("scanned GPU",
		"card", cardIndex,
		"pci", gpu.PCIDevice,
		"driver", gpu.DriverName,
		"vram_gb", gpu.VRAMTotalGB(),
		"gpu_clock_max", gpu.GPUClockMax,
		"monitors", len(gpu.Monitors))

	return gpu
}

func (s *Scanner) getPCIDevice(sysfsPath string) string {
	link, err := os.Readlink(sysfsPath)
	if err != nil {
		return ""
	}

	pciPattern := regexp.MustCompile(`([0-9a-f]{4}:[0-9a-f]{2}:[0-9a-f]{2}\.[0-9])`)
	matches := pciPattern.FindAllString(link, -1)
	if len(matches) > 0 {
		return matches[len(matches)-1]
	}
	return ""
}

func (s *Scanner) getDriverName(sysfsPath string) string {
	driverPath := filepath.Join(sysfsPath, "device", "driver")
	link, err := os.Readlink(driverPath)
	if err != nil {
		return ""
	}
	return filepath.Base(link)
}

func (s *Scanner) getDriverVersion(gpu *domain.GPUInfo) {
	devPath := flatpak.HostPath(gpu.CardPath)
	file, err := os.Open(devPath)
	if err != nil {
		s.logger.Debug("cannot open DRM device (need video group?)", "path", devPath, "error", err)
		return
	}
	defer file.Close()

	version, err := drmlib.GetVersion(file)
	if err != nil {
		s.logger.Debug("failed to get DRM version", "error", err)
		return
	}

	gpu.DriverName = version.Name
	gpu.DriverVersion = fmt.Sprintf("%d.%d.%d", version.Major, version.Minor, version.Patch)
	gpu.DriverDate = version.Date
	gpu.DriverDesc = version.Desc
}

func (s *Scanner) getHardwareInfo(gpu *domain.GPUInfo, sysfsPath string) {
	devicePath := filepath.Join(sysfsPath, "device")

	if data, err := os.ReadFile(filepath.Join(devicePath, "mem_info_vram_total")); err == nil {
		if val, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64); err == nil {
			gpu.VRAMTotal = val
		}
	}

	if data, err := os.ReadFile(filepath.Join(devicePath, "mem_info_vram_used")); err == nil {
		if val, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64); err == nil {
			gpu.VRAMUsed = val
		}
	}

	if data, err := os.ReadFile(filepath.Join(devicePath, "subsystem_vendor")); err == nil {
		gpu.SubsystemVendor = strings.TrimSpace(strings.TrimPrefix(string(data), "0x"))
	}
	if data, err := os.ReadFile(filepath.Join(devicePath, "subsystem_device")); err == nil {
		gpu.SubsystemDevice = strings.TrimSpace(strings.TrimPrefix(string(data), "0x"))
	}

	if data, err := os.ReadFile(filepath.Join(devicePath, "mem_info_vram_vendor")); err == nil {
		vendor := strings.TrimSpace(string(data))
		if vendor != "" && vendor != "unknown" {
			gpu.VRAMVendor = strings.ToUpper(vendor[:1]) + vendor[1:]
		}
	}

	if data, err := os.ReadFile(filepath.Join(devicePath, "vbios_version")); err == nil {
		gpu.VBIOSVersion = strings.TrimSpace(string(data))
	}

	if data, err := os.ReadFile(filepath.Join(devicePath, "unique_id")); err == nil {
		gpu.UniqueID = strings.TrimSpace(string(data))
	}

	if data, err := os.ReadFile(filepath.Join(devicePath, "current_link_speed")); err == nil {
		gpu.PCIeLinkSpeed = strings.TrimSpace(string(data))
	}
	if data, err := os.ReadFile(filepath.Join(devicePath, "current_link_width")); err == nil {
		gpu.PCIeLinkWidth = strings.TrimSpace(string(data))
	}
	if data, err := os.ReadFile(filepath.Join(devicePath, "max_link_speed")); err == nil {
		gpu.MaxLinkSpeed = strings.TrimSpace(string(data))
	}
	if data, err := os.ReadFile(filepath.Join(devicePath, "max_link_width")); err == nil {
		gpu.MaxLinkWidth = strings.TrimSpace(string(data))
	}

	s.parseAMDClocks(gpu, devicePath)

	if gpu.GPUClockMax == 0 {
		s.parseIntelClocks(gpu, devicePath)
	}
}

func (s *Scanner) parseAMDClocks(gpu *domain.GPUInfo, devicePath string) {
	data, err := os.ReadFile(filepath.Join(devicePath, "pp_od_clk_voltage"))
	if err != nil {
		return
	}

	lines := strings.Split(string(data), "\n")
	var inSCLK, inMCLK bool

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "OD_SCLK:") {
			inSCLK = true
			inMCLK = false
			continue
		}
		if strings.HasPrefix(line, "OD_MCLK:") {
			inSCLK = false
			inMCLK = true
			continue
		}
		if strings.HasPrefix(line, "OD_") {
			inSCLK = false
			inMCLK = false
			continue
		}

		if inSCLK || inMCLK {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				clockStr := strings.TrimSuffix(strings.TrimSuffix(parts[1], "Mhz"), "MHz")
				if clock, err := strconv.ParseUint(clockStr, 10, 32); err == nil {
					if inSCLK {
						if parts[0] == "0:" {
							gpu.GPUClockMin = uint32(clock)
						} else {
							gpu.GPUClockMax = uint32(clock)
						}
					} else if inMCLK {
						if parts[0] == "0:" {
							gpu.MemClockMin = uint32(clock)
						} else {
							gpu.MemClockMax = uint32(clock)
						}
					}
				}
			}
		}
	}
}

func (s *Scanner) parseIntelClocks(gpu *domain.GPUInfo, devicePath string) {
	if data, err := os.ReadFile(filepath.Join(devicePath, "gt_max_freq_mhz")); err == nil {
		if val, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 32); err == nil {
			gpu.GPUClockMax = uint32(val)
		}
	}
	if data, err := os.ReadFile(filepath.Join(devicePath, "gt_min_freq_mhz")); err == nil {
		if val, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 32); err == nil {
			gpu.GPUClockMin = uint32(val)
		}
	}
}

func (s *Scanner) scanConnectors(cardIndex int) []*domain.Monitor {
	var monitors []*domain.Monitor

	monitors = s.scanConnectorsDRM(cardIndex)
	if len(monitors) > 0 {
		return monitors
	}

	return s.scanConnectorsSysfs(cardIndex)
}

func (s *Scanner) scanConnectorsDRM(cardIndex int) []*domain.Monitor {
	devPath := flatpak.HostPath(fmt.Sprintf("/dev/dri/card%d", cardIndex))
	file, err := os.Open(devPath)
	if err != nil {
		return nil
	}
	defer file.Close()

	resources, err := mode.GetResources(file)
	if err != nil {
		s.logger.Debug("failed to get DRM resources", "error", err)
		return nil
	}

	var monitors []*domain.Monitor
	for _, connID := range resources.Connectors {
		conn, err := mode.GetConnector(file, connID)
		if err != nil {
			s.logger.Debug("failed to get connector", "id", connID, "error", err)
			continue
		}

		monitor := s.connectorToMonitor(conn)
		monitors = append(monitors, monitor)
	}

	return monitors
}

func (s *Scanner) connectorToMonitor(conn *mode.Connector) *domain.Monitor {
	connType := domain.ConnectorType(conn.Type)

	monitor := &domain.Monitor{
		ID:        conn.ID,
		Type:      connType,
		TypeIndex: conn.TypeID,
		Name:      fmt.Sprintf("%s-%d", connType.String(), conn.TypeID),
		Status:    domain.ConnectionStatus(conn.Connection),
		WidthMM:   conn.Width,
		HeightMM:  conn.Height,
		EncoderID: conn.EncoderID,
	}

	for i, m := range conn.Modes {
		displayMode := domain.DisplayMode{
			Width:       m.Hdisplay,
			Height:      m.Vdisplay,
			RefreshRate: m.Vrefresh,
			Name:        fmt.Sprintf("%dx%d@%dHz", m.Hdisplay, m.Vdisplay, m.Vrefresh),
		}

		if i == 0 && monitor.Status == domain.ConnectionConnected {
			displayMode.IsCurrent = true
			displayMode.IsPreferred = true
			monitor.CurrentMode = &displayMode
		}

		monitor.Modes = append(monitor.Modes, displayMode)
	}

	return monitor
}

func (s *Scanner) scanConnectorsSysfs(cardIndex int) []*domain.Monitor {
	var monitors []*domain.Monitor

	pattern := flatpak.HostPath(fmt.Sprintf("/sys/class/drm/card%d-*", cardIndex))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}

	connPattern := regexp.MustCompile(`card\d+-(.+)-(\d+)$`)

	for _, connPath := range matches {
		base := filepath.Base(connPath)
		match := connPattern.FindStringSubmatch(base)
		if match == nil {
			continue
		}

		connTypeName := match[1]
		connIndex, _ := strconv.Atoi(match[2])

		monitor := &domain.Monitor{
			Name:      fmt.Sprintf("%s-%d", connTypeName, connIndex),
			Type:      s.parseConnectorType(connTypeName),
			TypeIndex: uint32(connIndex),
		}

		statusPath := filepath.Join(connPath, "status")
		if data, err := os.ReadFile(statusPath); err == nil {
			status := strings.TrimSpace(string(data))
			switch status {
			case "connected":
				monitor.Status = domain.ConnectionConnected
			case "disconnected":
				monitor.Status = domain.ConnectionDisconnected
			default:
				monitor.Status = domain.ConnectionUnknown
			}
		}

		idPath := filepath.Join(connPath, "connector_id")
		if data, err := os.ReadFile(idPath); err == nil {
			id, _ := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 32)
			monitor.ID = uint32(id)
		}

		if monitor.Status == domain.ConnectionConnected {
			modesPath := filepath.Join(connPath, "modes")
			if data, err := os.ReadFile(modesPath); err == nil {
				lines := strings.Split(strings.TrimSpace(string(data)), "\n")
				for i, line := range lines {
					if line == "" {
						continue
					}
					displayMode := s.parseModeLine(line)
					if i == 0 {
						displayMode.IsCurrent = true
						displayMode.IsPreferred = true
						monitor.CurrentMode = &displayMode
					}
					monitor.Modes = append(monitor.Modes, displayMode)
				}
			}
		}

		dpmsPath := filepath.Join(connPath, "dpms")
		if data, err := os.ReadFile(dpmsPath); err == nil {
			monitor.DPMS = strings.TrimSpace(string(data))
		}

		monitors = append(monitors, monitor)
	}

	return monitors
}

func (s *Scanner) parseConnectorType(name string) domain.ConnectorType {
	typeMap := map[string]domain.ConnectorType{
		"VGA":       domain.ConnectorVGA,
		"DVI-I":     domain.ConnectorDVII,
		"DVI-D":     domain.ConnectorDVID,
		"DVI-A":     domain.ConnectorDVIA,
		"Composite": domain.ConnectorComposite,
		"SVIDEO":    domain.ConnectorSVideo,
		"LVDS":      domain.ConnectorLVDS,
		"Component": domain.ConnectorComponent,
		"DP":        domain.ConnectorDisplayPort,
		"HDMI-A":    domain.ConnectorHDMIA,
		"HDMI-B":    domain.ConnectorHDMIB,
		"TV":        domain.ConnectorTV,
		"eDP":       domain.ConnectorEDP,
		"Virtual":   domain.ConnectorVirtual,
		"DSI":       domain.ConnectorDSI,
		"DPI":       domain.ConnectorDPI,
		"Writeback": domain.ConnectorWriteback,
	}

	if t, ok := typeMap[name]; ok {
		return t
	}
	return domain.ConnectorUnknown
}

func (s *Scanner) parseModeLine(line string) domain.DisplayMode {
	parts := strings.Split(line, "x")
	mode := domain.DisplayMode{Name: line}

	if len(parts) == 2 {
		w, _ := strconv.ParseUint(parts[0], 10, 16)
		h, _ := strconv.ParseUint(parts[1], 10, 16)
		mode.Width = uint16(w)
		mode.Height = uint16(h)
	}

	return mode
}

func (s *Scanner) GetGPUByPCI(pciAddress string) *domain.GPUInfo {
	gpus, err := s.ScanGPUs()
	if err != nil {
		return nil
	}

	for _, gpu := range gpus {
		if gpu.PCIDevice == pciAddress {
			return gpu
		}
	}
	return nil
}

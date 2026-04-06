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

// Refresh clears the cache so the next scan reads fresh data.
func (s *Scanner) Refresh() {
	s.cache = nil
}

// ScanGPUs returns information about all GPUs with their connected monitors.
func (s *Scanner) ScanGPUs() ([]*domain.GPUInfo, error) {
	if s.cache != nil {
		return s.cache, nil
	}

	var gpus []*domain.GPUInfo

	// Find all DRM cards in /sys/class/drm
	pattern := flatpak.HostPath("/sys/class/drm/card[0-9]*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to glob DRM cards: %w", err)
	}

	cardPattern := regexp.MustCompile(`/card(\d+)$`)

	for _, cardPath := range matches {
		// Skip connector entries like card0-HDMI-A-1
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

// scanCard scans a single DRM card for GPU and monitor info.
func (s *Scanner) scanCard(cardIndex int, sysfsPath string) *domain.GPUInfo {
	gpu := &domain.GPUInfo{
		CardIndex: cardIndex,
		CardPath:  fmt.Sprintf("/dev/dri/card%d", cardIndex),
	}

	// Get PCI device address from sysfs symlink
	gpu.PCIDevice = s.getPCIDevice(sysfsPath)

	// Get driver name from sysfs
	gpu.DriverName = s.getDriverName(sysfsPath)

	// Try to get driver version via DRM ioctl
	s.getDriverVersion(gpu)

	// Get hardware info (VRAM, clocks, subsystem IDs)
	s.getHardwareInfo(gpu, sysfsPath)

	// Scan connectors
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

// getPCIDevice extracts the PCI address from the card's sysfs symlink.
func (s *Scanner) getPCIDevice(sysfsPath string) string {
	link, err := os.Readlink(sysfsPath)
	if err != nil {
		return ""
	}

	// Extract PCI address pattern (e.g., 0000:03:00.0)
	pciPattern := regexp.MustCompile(`([0-9a-f]{4}:[0-9a-f]{2}:[0-9a-f]{2}\.[0-9])`)
	matches := pciPattern.FindAllString(link, -1)
	if len(matches) > 0 {
		// Return the last match (the actual GPU, not bridges)
		return matches[len(matches)-1]
	}
	return ""
}

// getDriverName reads the driver name from sysfs.
func (s *Scanner) getDriverName(sysfsPath string) string {
	driverPath := filepath.Join(sysfsPath, "device", "driver")
	link, err := os.Readlink(driverPath)
	if err != nil {
		return ""
	}
	return filepath.Base(link)
}

// getDriverVersion attempts to get driver version via DRM ioctl.
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

// getHardwareInfo reads VRAM, clocks, and subsystem IDs from sysfs.
func (s *Scanner) getHardwareInfo(gpu *domain.GPUInfo, sysfsPath string) {
	devicePath := filepath.Join(sysfsPath, "device")

	// Read VRAM total
	if data, err := os.ReadFile(filepath.Join(devicePath, "mem_info_vram_total")); err == nil {
		if val, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64); err == nil {
			gpu.VRAMTotal = val
		}
	}

	// Read VRAM used
	if data, err := os.ReadFile(filepath.Join(devicePath, "mem_info_vram_used")); err == nil {
		if val, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64); err == nil {
			gpu.VRAMUsed = val
		}
	}

	// Read subsystem vendor/device IDs
	if data, err := os.ReadFile(filepath.Join(devicePath, "subsystem_vendor")); err == nil {
		gpu.SubsystemVendor = strings.TrimSpace(strings.TrimPrefix(string(data), "0x"))
	}
	if data, err := os.ReadFile(filepath.Join(devicePath, "subsystem_device")); err == nil {
		gpu.SubsystemDevice = strings.TrimSpace(strings.TrimPrefix(string(data), "0x"))
	}

	// Read VRAM vendor
	if data, err := os.ReadFile(filepath.Join(devicePath, "mem_info_vram_vendor")); err == nil {
		vendor := strings.TrimSpace(string(data))
		if vendor != "" && vendor != "unknown" {
			// Capitalize first letter
			gpu.VRAMVendor = strings.ToUpper(vendor[:1]) + vendor[1:]
		}
	}

	// Read VBIOS version
	if data, err := os.ReadFile(filepath.Join(devicePath, "vbios_version")); err == nil {
		gpu.VBIOSVersion = strings.TrimSpace(string(data))
	}

	// Read GPU unique ID (serial)
	if data, err := os.ReadFile(filepath.Join(devicePath, "unique_id")); err == nil {
		gpu.UniqueID = strings.TrimSpace(string(data))
	}

	// Read PCIe link info
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

	// Read clock info from pp_od_clk_voltage (AMD specific)
	s.parseAMDClocks(gpu, devicePath)

	// Try Intel-specific paths if AMD didn't work
	if gpu.GPUClockMax == 0 {
		s.parseIntelClocks(gpu, devicePath)
	}
}

// parseAMDClocks extracts clock info from AMD's pp_od_clk_voltage sysfs file.
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

		// Parse clock lines like "0: 255Mhz" or "1: 2855Mhz"
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

// parseIntelClocks extracts clock info from Intel's sysfs entries.
func (s *Scanner) parseIntelClocks(gpu *domain.GPUInfo, devicePath string) {
	// Intel exposes clocks differently - try gt_max_freq_mhz
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

// scanConnectors finds all connectors for a card and their status.
func (s *Scanner) scanConnectors(cardIndex int) []*domain.Monitor {
	var monitors []*domain.Monitor

	// First try DRM ioctl for full connector info
	monitors = s.scanConnectorsDRM(cardIndex)
	if len(monitors) > 0 {
		return monitors
	}

	// Fall back to sysfs scanning
	return s.scanConnectorsSysfs(cardIndex)
}

// scanConnectorsDRM uses DRM ioctls to get connector information.
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

// connectorToMonitor converts a DRM connector to our domain model.
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

	// Convert modes
	for i, m := range conn.Modes {
		displayMode := domain.DisplayMode{
			Width:       m.Hdisplay,
			Height:      m.Vdisplay,
			RefreshRate: m.Vrefresh,
			Name:        fmt.Sprintf("%dx%d@%dHz", m.Hdisplay, m.Vdisplay, m.Vrefresh),
		}

		// First mode is usually current/preferred
		if i == 0 && monitor.Status == domain.ConnectionConnected {
			displayMode.IsCurrent = true
			displayMode.IsPreferred = true
			monitor.CurrentMode = &displayMode
		}

		monitor.Modes = append(monitor.Modes, displayMode)
	}

	return monitor
}

// scanConnectorsSysfs falls back to reading connector info from sysfs.
func (s *Scanner) scanConnectorsSysfs(cardIndex int) []*domain.Monitor {
	var monitors []*domain.Monitor

	// Find connectors like card0-HDMI-A-1, card0-DP-1
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

		// Read status
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

		// Read connector_id
		idPath := filepath.Join(connPath, "connector_id")
		if data, err := os.ReadFile(idPath); err == nil {
			id, _ := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 32)
			monitor.ID = uint32(id)
		}

		// Read modes if connected
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

		// Read DPMS state
		dpmsPath := filepath.Join(connPath, "dpms")
		if data, err := os.ReadFile(dpmsPath); err == nil {
			monitor.DPMS = strings.TrimSpace(string(data))
		}

		monitors = append(monitors, monitor)
	}

	return monitors
}

// parseConnectorType converts a connector type string to our enum.
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

// parseModeLine parses a mode string like "1920x1080" into a DisplayMode.
func (s *Scanner) parseModeLine(line string) domain.DisplayMode {
	// Modes from sysfs are just "1920x1080", no refresh rate
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

// GetGPUByPCI finds GPU info for a specific PCI device.
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

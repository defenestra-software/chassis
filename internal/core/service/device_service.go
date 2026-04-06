package service

import (
	"encoding/json"
	"log/slog"
	"strings"

	"defenestra.io/chassis/internal/core/domain"
	"defenestra.io/chassis/internal/core/port"
)

type DeviceService struct {
	scanner          port.DeviceScanner
	gpuScanner       port.GPUScanner
	storageScanner   port.StorageScanner
	sysInfoScanner   port.SysInfoScanner
	systemInfoReader port.SystemInfoReader
	mutator          port.DriverMutator
	listener         port.EventListener
	logger           *slog.Logger
}

func NewDeviceService(
	scanner port.DeviceScanner,
	mutator port.DriverMutator,
	listener port.EventListener,
	logger *slog.Logger,
) *DeviceService {
	return &DeviceService{
		scanner:  scanner,
		mutator:  mutator,
		listener: listener,
		logger:   logger,
	}
}

func (s *DeviceService) SetGPUScanner(gpuScanner port.GPUScanner) {
	s.gpuScanner = gpuScanner
}

func (s *DeviceService) SetStorageScanner(storageScanner port.StorageScanner) {
	s.storageScanner = storageScanner
}

func (s *DeviceService) SetSysInfoScanner(scanner port.SysInfoScanner) {
	s.sysInfoScanner = scanner
}

func (s *DeviceService) SetSystemInfoReader(reader port.SystemInfoReader) {
	s.systemInfoReader = reader
}

func (s *DeviceService) GetDeviceTree() (*domain.DeviceNode, error) {
	tree, err := s.scanner.Scan()
	if err != nil {
		return nil, err
	}

	if s.gpuScanner != nil {
		s.enrichGPUs(tree)
	}

	if s.storageScanner != nil {
		s.enrichStorage(tree)
	}

	if s.sysInfoScanner != nil {
		for _, node := range s.sysInfoScanner.ScanProcessors() {
			tree.Children = append(tree.Children, node)
		}
		for _, node := range s.sysInfoScanner.ScanSensors() {
			tree.Children = append(tree.Children, node)
		}
		for _, node := range s.sysInfoScanner.ScanDisplays() {
			tree.Children = append(tree.Children, node)
		}
		for _, node := range s.sysInfoScanner.ScanPower() {
			tree.Children = append(tree.Children, node)
		}
		for _, node := range s.sysInfoScanner.ScanBluetoothHID() {
			tree.Children = append(tree.Children, node)
		}
		if mb := s.sysInfoScanner.ScanMotherboard(); mb != nil {
			tree.Children = append(tree.Children, mb)
		}
		for _, node := range s.sysInfoScanner.ScanMemory() {
			tree.Children = append(tree.Children, node)
		}

		s.sysInfoScanner.EnrichCameraInfo(tree)
		s.sysInfoScanner.EnrichSerialPorts(tree)
		s.sysInfoScanner.EnrichNetworkInfo(tree)
		s.sysInfoScanner.EnrichPeripheralBatteries(tree)
		s.sysInfoScanner.EnrichUPSBattery(tree)
		s.sysInfoScanner.EnrichUSBStorage(tree)
	}

	if s.systemInfoReader != nil {
		s.enrichSystemInfo(tree)
	}

	ExtractInterfaces(tree)

	return tree, nil
}

func (s *DeviceService) enrichGPUs(node *domain.DeviceNode) {
	if node == nil {
		return
	}

	if node.Class == domain.DeviceClassGPU && node.Type == domain.DeviceTypePCI {
		pciAddr := strings.TrimPrefix(node.ID, "pci:")
		if gpuInfo := s.gpuScanner.GetGPUByPCI(pciAddr); gpuInfo != nil {
			node.GPUInfo = gpuInfo
			s.logger.Debug("enriched GPU with DRM info",
				"device", node.ID,
				"driver", gpuInfo.DriverName,
				"monitors", len(gpuInfo.Monitors))
		}
	}

	for _, child := range node.Children {
		s.enrichGPUs(child)
	}
}

func (s *DeviceService) enrichStorage(node *domain.DeviceNode) {
	if node == nil {
		return
	}

	if node.Class == domain.DeviceClassStorage && node.Type == domain.DeviceTypePCI {
		pciAddr := strings.TrimPrefix(node.ID, "pci:")
		if storageInfo := s.storageScanner.GetStorageByPCI(pciAddr); storageInfo != nil {
			node.StorageInfo = storageInfo
			s.logger.Debug("enriched storage with drive info",
				"device", node.ID,
				"model", storageInfo.Model,
				"temp", storageInfo.Temperature)
		}
	}

	for _, child := range node.Children {
		s.enrichStorage(child)
	}
}

func (s *DeviceService) enrichSystemInfo(tree *domain.DeviceNode) {
	jsonData, err := s.systemInfoReader.ReadSystemInfo()
	if err != nil {
		s.logger.Debug("could not read system info from daemon", "error", err)
		return
	}

	var info struct {
		BoardSerial   string `json:"board_serial"`
		MemoryModules []struct {
			Label  string `json:"label"`
			Size   string `json:"size"`
			Type   string `json:"type"`
			Speed  string `json:"speed"`
			Vendor string `json:"vendor"`
			Serial string `json:"serial"`
			Part   string `json:"part"`
			Rank   string `json:"rank"`
		} `json:"memory_modules"`
	}

	if err := json.Unmarshal([]byte(jsonData), &info); err != nil {
		s.logger.Debug("could not parse system info", "error", err)
		return
	}

	if info.BoardSerial != "" {
		s.enrichMotherboardSerial(tree, info.BoardSerial)
	}

	// Add memory modules from SMBIOS - these are more detailed than ghw's,
	// so replace any existing memory nodes from ScanMemory().
	if len(info.MemoryModules) > 0 {
		filtered := tree.Children[:0]
		for _, child := range tree.Children {
			if !strings.HasPrefix(child.ID, "memory:") {
				filtered = append(filtered, child)
			}
		}
		tree.Children = filtered

		for i, mod := range info.MemoryModules {
			// Try to identify vendor from part number if manufacturer is unknown
			vendor := filterUnknown(mod.Vendor)
			if vendor == "" && mod.Part != "" {
				vendor = inferVendorFromPartNumber(mod.Part)
			}

			// Build name: "TeamGroup 32 GB DDR4" or "32 GB DDR4"
			nameParts := []string{}
			if vendor != "" {
				nameParts = append(nameParts, vendor)
			}
			nameParts = append(nameParts, mod.Size)
			if mod.Type != "" && mod.Type != "Unknown" {
				nameParts = append(nameParts, mod.Type)
			}
			name := strings.Join(nameParts, " ")

			// Build subtitle: "DIMM_A1 · 3200 MT/s"
			subtitle := mod.Label
			if mod.Speed != "" && mod.Speed != "Unknown" {
				subtitle = subtitle + " · " + mod.Speed
			}

			modInfo := &domain.MemoryModuleInfo{
				Label:        mod.Label,
				SizeString_:  mod.Size,
				Type:         filterUnknown(mod.Type),
				Speed:        filterUnknown(mod.Speed),
				Vendor:       vendor,
				SerialNumber: filterUnknown(mod.Serial),
				PartNumber:   filterUnknown(mod.Part),
				Rank:         filterUnknown(mod.Rank),
			}

			node := &domain.DeviceNode{
				ID:           "memory:" + mod.Label,
				Name:         name,
				Vendor:       vendor,
				Product:      subtitle,
				Type:         domain.DeviceTypeUnknown,
				Class:        domain.DeviceClassMemory,
				MemoryModule: modInfo,
				IOMMUGroup:   -1,
				Children:     make([]*domain.DeviceNode, 0),
			}

			tree.Children = append(tree.Children, node)
			s.logger.Debug("added memory module from SMBIOS", "slot", mod.Label, "size", mod.Size, "index", i)
		}
	}
}

func (s *DeviceService) enrichMotherboardSerial(node *domain.DeviceNode, serial string) {
	if node.ID == "system:motherboard" && node.MotherboardInfo != nil {
		node.MotherboardInfo.SerialNumber = serial
		return
	}
	for _, child := range node.Children {
		s.enrichMotherboardSerial(child, serial)
	}
}

func inferVendorFromPartNumber(partNumber string) string {
	pn := strings.ToUpper(strings.TrimSpace(partNumber))

	prefixes := map[string]string{
		"TEAMGROUP": "TeamGroup",
		"CMK":       "Corsair", // Vengeance
		"CMW":       "Corsair", // Vengeance RGB
		"CMT":       "Corsair", // Dominator
		"CM":        "Corsair",
		"F4-":       "G.Skill",
		"F5-":       "G.Skill",
		"KHX":       "Kingston", // HyperX
		"KVR":       "Kingston", // ValueRAM
		"KF":        "Kingston", // Fury
		"HMA":       "SK Hynix",
		"HMT":       "SK Hynix",
		"HMCG":      "SK Hynix",
		"M378":      "Samsung",
		"M471":      "Samsung",
		"M393":      "Samsung",
		"MTA":       "Micron",
		"CT":        "Crucial",
		"BL":        "Crucial", // Ballistix
		"AX4":       "ADATA",
		"AX5":       "ADATA",
		"AD4":       "ADATA",
		"AD5":       "ADATA",
		"PVV":       "Patriot", // Viper
		"PVS":       "Patriot",
		"FLARE":     "G.Skill",
		"TRIDENT":   "G.Skill",
		"RIPJAWS":   "G.Skill",
	}

	for prefix, vendor := range prefixes {
		if strings.HasPrefix(pn, prefix) {
			return vendor
		}
	}

	return ""
}

func filterUnknown(s string) string {
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)
	if lower == "unknown" || lower == "not specified" || lower == "not available" || lower == "" {
		return ""
	}
	return s
}

func (s *DeviceService) UnbindDriver(deviceID, busType string) error {
	s.logger.Info("unbinding driver", "device", deviceID, "bus", busType)

	if err := s.mutator.Unbind(deviceID, busType); err != nil {
		return &domain.OperationError{Op: "unbind", DeviceID: deviceID, Err: err}
	}

	return nil
}

func (s *DeviceService) BindDriver(deviceID, driverName string) error {
	s.logger.Info("binding driver", "device", deviceID, "driver", driverName)

	if err := s.mutator.Bind(deviceID, driverName); err != nil {
		return &domain.OperationError{Op: "bind", DeviceID: deviceID, Err: err}
	}

	return nil
}

// For PCI devices this writes to drivers_probe. For USB devices this rescans the bus.
func (s *DeviceService) RestoreDriver(deviceID, busType string) error {
	s.logger.Info("restoring default driver", "device", deviceID, "bus", busType)

	// Unbind current driver first (ignore error if already unbound)
	if err := s.mutator.Unbind(deviceID, busType); err != nil {
		s.logger.Debug("unbind during restore (may already be unbound)", "error", err)
	}

	if err := s.mutator.DriverProbe(deviceID); err != nil {
		return &domain.OperationError{Op: "driver_probe", DeviceID: deviceID, Err: err}
	}

	return nil
}

func (s *DeviceService) HasMutator() bool {
	return s.mutator != nil
}

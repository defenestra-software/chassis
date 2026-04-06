package storage

import (
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	flatpak "gitlab.com/defenestra-software/go-flatbridge"

	"defenestra.io/chassis/internal/core/domain"
	"defenestra.io/chassis/internal/core/port"
)

// Scanner reads storage device info and temperatures from sysfs.
// All data is world-readable - no root privileges needed.
type Scanner struct {
	logger   *slog.Logger
	cache    []*domain.StorageInfo
	mu       sync.RWMutex
	stopChan chan struct{}
}

var _ port.StorageScanner = (*Scanner)(nil)

func NewScanner(logger *slog.Logger) *Scanner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scanner{
		logger:   logger,
		stopChan: make(chan struct{}),
	}
}

// ScanStorage discovers NVMe and block storage devices from sysfs.
func (s *Scanner) ScanStorage() ([]*domain.StorageInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var infos []*domain.StorageInfo

	// Scan NVMe controllers
	nvmeInfos := s.scanNVMe()
	infos = append(infos, nvmeInfos...)

	// Scan SATA/SCSI block devices
	sataInfos := s.scanBlockDevices()
	infos = append(infos, sataInfos...)

	s.cache = infos
	return infos, nil
}

// GetStorageByPCI finds storage info for a specific PCI device address.
func (s *Scanner) GetStorageByPCI(pciAddress string) *domain.StorageInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.cache == nil {
		s.mu.RUnlock()
		s.ScanStorage()
		s.mu.RLock()
	}

	for _, info := range s.cache {
		if info.PCIDevice == pciAddress {
			return info
		}
	}
	return nil
}

// StartMonitor begins periodic temperature polling.
func (s *Scanner) StartMonitor(interval time.Duration, onChange func()) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-s.stopChan:
				return
			case <-ticker.C:
				if s.updateTemperatures() {
					onChange()
				}
			}
		}
	}()
	s.logger.Info("storage temperature monitor started", "interval", interval)
}

// Stop terminates the temperature monitor.
func (s *Scanner) Stop() {
	close(s.stopChan)
}

// Refresh clears the cache.
func (s *Scanner) Refresh() {
	s.mu.Lock()
	s.cache = nil
	s.mu.Unlock()
}

// updateTemperatures re-reads hwmon temps and returns true if any changed.
func (s *Scanner) updateTemperatures() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	changed := false
	for _, info := range s.cache {
		if info.Transport != "pcie" {
			continue
		}
		// Re-read hwmon for this NVMe device
		nvmePath := s.findNVMePathForPCI(info.PCIDevice)
		if nvmePath == "" {
			continue
		}
		newTemp := s.readHWMonTemp(nvmePath)
		if newTemp != info.Temperature && newTemp != 0 {
			info.Temperature = newTemp
			changed = true
		}
	}
	return changed
}

// ============================================================
// NVMe scanning
// ============================================================

var nvmePattern = regexp.MustCompile(`^nvme\d+$`)

func (s *Scanner) scanNVMe() []*domain.StorageInfo {
	var infos []*domain.StorageInfo

	nvmePath := flatpak.HostPath("/sys/class/nvme")
	entries, err := os.ReadDir(nvmePath)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if !nvmePattern.MatchString(entry.Name()) {
			continue
		}

		ctrlPath := filepath.Join(nvmePath, entry.Name())
		info := s.readNVMeController(ctrlPath, entry.Name())
		if info != nil {
			infos = append(infos, info)
		}
	}

	return infos
}

func (s *Scanner) readNVMeController(ctrlPath, name string) *domain.StorageInfo {
	info := &domain.StorageInfo{
		Model:     readSysfsFile(filepath.Join(ctrlPath, "model")),
		Serial:    readSysfsFile(filepath.Join(ctrlPath, "serial")),
		Firmware:  readSysfsFile(filepath.Join(ctrlPath, "firmware_rev")),
		State:     readSysfsFile(filepath.Join(ctrlPath, "state")),
		Transport: readSysfsFile(filepath.Join(ctrlPath, "transport")),
	}

	// Get PCI address from the 'address' file or symlink resolution
	info.PCIDevice = readSysfsFile(filepath.Join(ctrlPath, "address"))
	if info.PCIDevice == "" {
		// Fall back to resolving the symlink
		info.PCIDevice = s.resolvePCIAddress(ctrlPath)
	}

	// Find the block device (e.g., nvme0n1)
	nsEntries, _ := os.ReadDir(ctrlPath)
	for _, ns := range nsEntries {
		if strings.HasPrefix(ns.Name(), name+"n") {
			info.BlockDevice = ns.Name()
			// Read capacity from size file (in 512-byte sectors)
			sizeStr := readSysfsFile(filepath.Join(ctrlPath, ns.Name(), "size"))
			if sectors, err := strconv.ParseUint(sizeStr, 10, 64); err == nil {
				info.SizeBytes = sectors * 512
			}
			break
		}
	}

	// Read temperature from hwmon
	info.Temperature = s.readHWMonTemp(ctrlPath)
	info.TempCritical = s.readHWMonValue(ctrlPath, "temp1_crit")
	info.TempMax = s.readHWMonValue(ctrlPath, "temp1_max")

	s.logger.Debug("scanned NVMe",
		"name", name,
		"model", info.Model,
		"pci", info.PCIDevice,
		"temp", info.Temperature,
		"size", info.SizeString())

	return info
}

func (s *Scanner) readHWMonTemp(ctrlPath string) float64 {
	return s.readHWMonValue(ctrlPath, "temp1_input")
}

func (s *Scanner) readHWMonValue(ctrlPath, filename string) float64 {
	// hwmon directories are named hwmon0, hwmon1, etc.
	entries, err := os.ReadDir(ctrlPath)
	if err != nil {
		return 0
	}

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "hwmon") {
			continue
		}
		hwmonPath := filepath.Join(ctrlPath, entry.Name())
		valStr := readSysfsFile(filepath.Join(hwmonPath, filename))
		if valStr == "" {
			continue
		}
		millideg, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			continue
		}
		// hwmon reports in millidegrees Celsius
		return millideg / 1000.0
	}
	return 0
}

func (s *Scanner) findNVMePathForPCI(pciAddress string) string {
	nvmePath := flatpak.HostPath("/sys/class/nvme")
	entries, err := os.ReadDir(nvmePath)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if !nvmePattern.MatchString(entry.Name()) {
			continue
		}
		ctrlPath := filepath.Join(nvmePath, entry.Name())
		addr := readSysfsFile(filepath.Join(ctrlPath, "address"))
		if addr == pciAddress {
			return ctrlPath
		}
	}
	return ""
}

func (s *Scanner) resolvePCIAddress(ctrlPath string) string {
	link, err := os.Readlink(ctrlPath)
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

// ============================================================
// SATA/SCSI block device scanning
// ============================================================

var blockDevPattern = regexp.MustCompile(`^sd[a-z]+$`)

func (s *Scanner) scanBlockDevices() []*domain.StorageInfo {
	var infos []*domain.StorageInfo

	blockPath := flatpak.HostPath("/sys/block")
	entries, err := os.ReadDir(blockPath)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if !blockDevPattern.MatchString(entry.Name()) {
			continue
		}

		devPath := filepath.Join(blockPath, entry.Name())
		info := s.readBlockDevice(devPath, entry.Name())
		if info != nil {
			infos = append(infos, info)
		}
	}

	return infos
}

func (s *Scanner) readBlockDevice(devPath, name string) *domain.StorageInfo {
	devicePath := filepath.Join(devPath, "device")

	info := &domain.StorageInfo{
		BlockDevice: name,
		Model:       readSysfsFile(filepath.Join(devicePath, "model")),
		Serial:      readSysfsFile(filepath.Join(devicePath, "serial")),
		Firmware:    readSysfsFile(filepath.Join(devicePath, "rev")),
		Transport:   "sata",
	}

	// Skip if no model info (not a real disk)
	if info.Model == "" {
		return nil
	}

	// Read capacity
	sizeStr := readSysfsFile(filepath.Join(devPath, "size"))
	if sectors, err := strconv.ParseUint(sizeStr, 10, 64); err == nil {
		info.SizeBytes = sectors * 512
	}

	// Resolve PCI address
	info.PCIDevice = s.resolvePCIAddress(devPath)

	// Read temperature from hwmon if available
	info.Temperature = s.readBlockDeviceTemp(devPath)

	s.logger.Debug("scanned block device",
		"name", name,
		"model", info.Model,
		"pci", info.PCIDevice,
		"size", info.SizeString())

	return info
}

func (s *Scanner) readBlockDeviceTemp(devPath string) float64 {
	// Check for hwmon in the device directory
	devicePath := filepath.Join(devPath, "device")
	entries, err := os.ReadDir(devicePath)
	if err != nil {
		return 0
	}

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "hwmon") {
			continue
		}
		hwmonPath := filepath.Join(devicePath, entry.Name())
		// Check subdirectories too (hwmon/hwmonN/)
		subEntries, err := os.ReadDir(hwmonPath)
		if err != nil {
			continue
		}
		for _, sub := range subEntries {
			if strings.HasPrefix(sub.Name(), "hwmon") {
				hwmonPath = filepath.Join(hwmonPath, sub.Name())
				break
			}
		}

		valStr := readSysfsFile(filepath.Join(hwmonPath, "temp1_input"))
		if valStr == "" {
			continue
		}
		millideg, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			continue
		}
		return math.Round(millideg/1000.0*10) / 10
	}
	return 0
}

// ============================================================
// Helpers
// ============================================================

func readSysfsFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

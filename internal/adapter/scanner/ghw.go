package scanner

import (
	"os"
	"path/filepath"
	"strings"

	"defenestra.io/chassis/internal/core/domain"
	"defenestra.io/chassis/internal/core/port"

	"github.com/jaypipes/ghw"
)

type GHWScanner struct {
	chrootPath string
	cache      *domain.DeviceNode
}

// Use chrootPath for Flatpak/container environments, empty string for native.
func NewGHWScanner(chrootPath string) *GHWScanner {
	return &GHWScanner{
		chrootPath: chrootPath,
	}
}

var _ port.DeviceScanner = (*GHWScanner)(nil)

func (s *GHWScanner) Scan() (*domain.DeviceNode, error) {
	var opts []*ghw.WithOption
	if s.chrootPath != "" {
		opts = append(opts, ghw.WithChroot(s.chrootPath))
	}

	// Create root node
	root := &domain.DeviceNode{
		ID:       "root",
		Name:     "System Devices",
		Type:     domain.DeviceTypeUnknown,
		Class:    domain.DeviceClassOther,
		Children: make([]*domain.DeviceNode, 0),
	}

	// Scan PCI devices
	if pciRoot, err := s.scanPCIDevices(opts); err == nil {
		root.Children = append(root.Children, pciRoot)
	}

	// Scan USB devices
	if usbRoot, err := s.scanUSBDevices(); err == nil && len(usbRoot.Children) > 0 {
		root.Children = append(root.Children, usbRoot)
	}

	s.cache = root
	return root, nil
}

func (s *GHWScanner) GetDevice(id string) (*domain.DeviceNode, error) {
	if s.cache == nil {
		if _, err := s.Scan(); err != nil {
			return nil, err
		}
	}
	return s.findDevice(s.cache, id)
}

func (s *GHWScanner) findDevice(node *domain.DeviceNode, id string) (*domain.DeviceNode, error) {
	if node.ID == id {
		return node, nil
	}
	for _, child := range node.Children {
		if found, err := s.findDevice(child, id); err == nil {
			return found, nil
		}
	}
	return nil, domain.ErrDeviceNotFound
}

func (s *GHWScanner) GetDevicesByType(deviceType domain.DeviceType) ([]*domain.DeviceNode, error) {
	if s.cache == nil {
		if _, err := s.Scan(); err != nil {
			return nil, err
		}
	}

	var result []*domain.DeviceNode
	s.collectByType(s.cache, deviceType, &result)
	return result, nil
}

func (s *GHWScanner) collectByType(node *domain.DeviceNode, deviceType domain.DeviceType, result *[]*domain.DeviceNode) {
	if node.Type == deviceType {
		*result = append(*result, node)
	}
	for _, child := range node.Children {
		s.collectByType(child, deviceType, result)
	}
}

func (s *GHWScanner) GetIOMMUGroup(group int) ([]*domain.DeviceNode, error) {
	if s.cache == nil {
		if _, err := s.Scan(); err != nil {
			return nil, err
		}
	}

	var result []*domain.DeviceNode
	s.collectByIOMMU(s.cache, group, &result)
	return result, nil
}

func (s *GHWScanner) collectByIOMMU(node *domain.DeviceNode, group int, result *[]*domain.DeviceNode) {
	if node.IOMMUGroup == group {
		*result = append(*result, node)
	}
	for _, child := range node.Children {
		s.collectByIOMMU(child, group, result)
	}
}

// Refresh clears the cache and rescans devices.
func (s *GHWScanner) Refresh() (*domain.DeviceNode, error) {
	s.cache = nil
	return s.Scan()
}

func (s *GHWScanner) readSysfsFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (s *GHWScanner) readDriver(sysfsPath string) string {
	driverLink := filepath.Join(sysfsPath, "driver")
	target, err := os.Readlink(driverLink)
	if err != nil {
		return ""
	}
	return filepath.Base(target)
}

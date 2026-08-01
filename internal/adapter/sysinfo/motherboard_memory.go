package sysinfo

import (
	"fmt"

	"github.com/jaypipes/ghw"

	"defenestra.io/chassis/internal/core/domain"
)

func (s *Scanner) ScanMotherboard() *domain.DeviceNode {
	bb, err := ghw.Baseboard()
	if err != nil {
		s.logger.Debug("could not read baseboard info", "error", err)
		return nil
	}

	if bb.Vendor == "" && bb.Product == "" {
		return nil
	}

	name := bb.Product
	if name == "" {
		name = "Motherboard"
	}

	mbInfo := &domain.MotherboardInfo{
		Vendor:       bb.Vendor,
		Product:      bb.Product,
		Version:      bb.Version,
		SerialNumber: bb.SerialNumber,
	}

	bi, err := ghw.BIOS()
	if err == nil {
		mbInfo.BIOSVendor = bi.Vendor
		mbInfo.BIOSVersion = bi.Version
		mbInfo.BIOSDate = bi.Date
	}

	mem, err := ghw.Memory()
	if err == nil {
		mbInfo.TotalMemoryBytes = mem.TotalPhysicalBytes
		if mbInfo.TotalMemoryBytes <= 0 {
			mbInfo.TotalMemoryBytes = mem.TotalUsableBytes
		}
		mbInfo.MemoryModules = len(mem.Modules)
	}

	subtitle := "Motherboard"
	if mbInfo.TotalMemoryBytes > 0 {
		subtitle = fmt.Sprintf("Motherboard · %s RAM", mbInfo.TotalMemoryString())
	}

	node := &domain.DeviceNode{
		ID:              "system:motherboard",
		Name:            name,
		Vendor:          bb.Vendor,
		Product:         subtitle,
		Type:            domain.DeviceTypeUnknown,
		Class:           domain.DeviceClassBridge, // Shows under System Devices
		MotherboardInfo: mbInfo,
		IOMMUGroup:      -1,
		Children:        make([]*domain.DeviceNode, 0),
	}

	s.logger.Debug("scanned motherboard", "vendor", bb.Vendor, "product", bb.Product)
	return node
}

func (s *Scanner) ScanMemory() []*domain.DeviceNode {
	mem, err := ghw.Memory()
	if err != nil {
		return nil
	}

	var nodes []*domain.DeviceNode
	for i, mod := range mem.Modules {
		if mod.SizeBytes == 0 {
			continue // skip empty slots
		}

		name := fmt.Sprintf("%s %s", mod.Vendor, formatMemSize(mod.SizeBytes))
		if mod.Vendor == "" || mod.Vendor == "unknown" {
			name = fmt.Sprintf("Memory Module %s", formatMemSize(mod.SizeBytes))
		}

		label := mod.Label
		if label == "" {
			label = fmt.Sprintf("Slot %d", i)
		}

		modInfo := &domain.MemoryModuleInfo{
			Label:        label,
			Location:     mod.Location,
			Size:         formatMemSize(mod.SizeBytes),
			Vendor:       mod.Vendor,
			SerialNumber: mod.SerialNumber,
		}

		node := &domain.DeviceNode{
			ID:           fmt.Sprintf("memory:%d", i),
			Name:         name,
			Vendor:       mod.Vendor,
			Product:      fmt.Sprintf("%s · %s", label, formatMemSize(mod.SizeBytes)),
			Type:         domain.DeviceTypeUnknown,
			Class:        domain.DeviceClassMemory,
			MemoryModule: modInfo,
			IOMMUGroup:   -1,
			Children:     make([]*domain.DeviceNode, 0),
		}

		nodes = append(nodes, node)
	}

	s.logger.Debug("scanned memory", "modules", len(nodes))
	return nodes
}

func formatMemSize(bytes int64) string {
	gb := float64(bytes) / (1024 * 1024 * 1024)
	if gb >= 1 {
		if gb == float64(int(gb)) {
			return fmt.Sprintf("%.0f GB", gb)
		}
		return fmt.Sprintf("%.1f GB", gb)
	}
	mb := float64(bytes) / (1024 * 1024)
	return fmt.Sprintf("%.0f MB", mb)
}

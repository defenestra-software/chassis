package domain

import "fmt"

type MotherboardInfo struct {
	// Baseboard
	Vendor       string
	Product      string
	Version      string
	SerialNumber string

	// BIOS/Firmware
	BIOSVendor  string
	BIOSVersion string
	BIOSDate    string

	// Memory summary
	TotalMemoryBytes int64
	MemoryModules    int
}

func (m *MotherboardInfo) TotalMemoryString() string {
	if m.TotalMemoryBytes <= 0 {
		return ""
	}
	gb := float64(m.TotalMemoryBytes) / (1024 * 1024 * 1024)
	if gb >= 1 {
		if gb == float64(int(gb)) {
			return fmt.Sprintf("%.0f GB", gb)
		}
		return fmt.Sprintf("%.1f GB", gb)
	}
	mb := float64(m.TotalMemoryBytes) / (1024 * 1024)
	return fmt.Sprintf("%.0f MB", mb)
}

type MemoryModuleInfo struct {
	Label        string
	Location     string
	SizeBytes    int64
	SizeString_  string // from SMBIOS
	Type         string
	Speed        string
	Vendor       string
	SerialNumber string
	PartNumber   string
	Rank         string
}

func (m *MemoryModuleInfo) SizeString() string {
	if m.SizeBytes <= 0 {
		return "Empty"
	}
	gb := float64(m.SizeBytes) / (1024 * 1024 * 1024)
	if gb >= 1 {
		return fmt.Sprintf("%.0f GB", gb)
	}
	mb := float64(m.SizeBytes) / (1024 * 1024)
	return fmt.Sprintf("%.0f MB", mb)
}

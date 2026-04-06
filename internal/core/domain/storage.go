package domain

import "fmt"

type StorageInfo struct {
	Model       string
	Serial      string
	Firmware    string
	State       string
	Transport   string
	BlockDevice string

	Temperature  float64
	TempCritical float64
	TempMax      float64

	SizeBytes uint64
	PCIDevice string
	SMART     *SMARTData
}

type SMARTData struct {
	PowerOnHours     uint64
	PowerCycles      uint64
	UnsafeShutdowns  uint64
	MediaErrors      uint64
	PercentageUsed   uint8
	AvailableSpare   uint8
	DataReadBytes    uint64
	DataWrittenBytes uint64
	Temperature      int16

	Healthy bool
	Status  string

	RawJSON string
}

func (s *StorageInfo) TemperatureString() string {
	if s.Temperature == 0 {
		return "N/A"
	}
	return fmt.Sprintf("%.0f °C", s.Temperature)
}

func (s *StorageInfo) SizeString() string {
	if s.SizeBytes == 0 {
		return "Unknown"
	}
	mb := float64(s.SizeBytes) / (1024 * 1024)
	if mb < 1024 {
		return fmt.Sprintf("%.1f MB", mb)
	}
	gb := mb / 1024
	if gb >= 1000 {
		return fmt.Sprintf("%.1f TB", gb/1024)
	}
	return fmt.Sprintf("%.0f GB", gb)
}

func (d *SMARTData) DataReadString() string {
	return formatBytes(d.DataReadBytes)
}

func (d *SMARTData) DataWrittenString() string {
	return formatBytes(d.DataWrittenBytes)
}

func formatBytes(b uint64) string {
	if b == 0 {
		return "N/A"
	}
	tb := float64(b) / (1024 * 1024 * 1024 * 1024)
	if tb >= 1 {
		return fmt.Sprintf("%.1f TB", tb)
	}
	gb := float64(b) / (1024 * 1024 * 1024)
	return fmt.Sprintf("%.1f GB", gb)
}

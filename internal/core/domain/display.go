package domain

import "fmt"

type DisplayInfo struct {
	MonitorName     string
	Vendor          string
	Serial          string
	ManufactureYear int
	ManufactureWeek int
	PhysicalWidth   int
	PhysicalHeight  int
	EDIDVersion     string
	ConnectorType   string
	ConnectorIndex  string
	GPUCard         string
	CurrentMode     string
	MinRefresh      int
	MaxRefresh      int
	ModeCount       int
	Modes           []string
}

func (d *DisplayInfo) DiagonalInches() float64 {
	if d.PhysicalWidth == 0 || d.PhysicalHeight == 0 {
		return 0
	}
	wIn := float64(d.PhysicalWidth) / 25.4
	hIn := float64(d.PhysicalHeight) / 25.4
	diag := (wIn*wIn + hIn*hIn)
	// Simple sqrt approximation using Newton's method
	x := diag / 2
	for i := 0; i < 10; i++ {
		x = (x + diag/x) / 2
	}
	return x
}

func (d *DisplayInfo) SizeString() string {
	if d.PhysicalWidth == 0 || d.PhysicalHeight == 0 {
		return ""
	}
	diag := d.DiagonalInches()
	return fmt.Sprintf("%.0f\" (%d × %d mm)", diag, d.PhysicalWidth, d.PhysicalHeight)
}

func (d *DisplayInfo) RefreshString() string {
	if d.MinRefresh > 0 && d.MaxRefresh > 0 {
		return fmt.Sprintf("%d – %d Hz", d.MinRefresh, d.MaxRefresh)
	}
	if d.MaxRefresh > 0 {
		return fmt.Sprintf("%d Hz", d.MaxRefresh)
	}
	return ""
}

func (d *DisplayInfo) ManufactureString() string {
	if d.ManufactureYear > 0 {
		if d.ManufactureWeek > 0 {
			return fmt.Sprintf("Week %d, %d", d.ManufactureWeek, d.ManufactureYear)
		}
		return fmt.Sprintf("%d", d.ManufactureYear)
	}
	return ""
}

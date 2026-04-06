package domain

import "fmt"

type SensorInfo struct {
	Name       string
	Label      string
	Type       string // "temperature", "fan", "voltage", "power"
	Value      float64
	Critical   float64
	Max        float64
	Min        float64
	HWMonPath  string
	DeviceName string
}

func (s *SensorInfo) FormattedValue() string {
	switch s.Type {
	case "temperature":
		return fmt.Sprintf("%.0f °C", s.Value)
	case "fan":
		return fmt.Sprintf("%.0f RPM", s.Value)
	case "voltage":
		return fmt.Sprintf("%.3f V", s.Value)
	case "power":
		return fmt.Sprintf("%.1f W", s.Value)
	default:
		return fmt.Sprintf("%.1f", s.Value)
	}
}

func (s *SensorInfo) ThresholdString() string {
	if s.Critical > 0 {
		return fmt.Sprintf("critical: %.0f", s.Critical)
	}
	if s.Max > 0 {
		return fmt.Sprintf("max: %.0f", s.Max)
	}
	return ""
}

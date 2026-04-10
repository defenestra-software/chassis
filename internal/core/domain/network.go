package domain

import "fmt"

type NetworkInfo struct {
	InterfaceName string
	MACAddress    string
	Speed         string
	Duplex        string
	MTU           string
	State         string
	IsWireless    bool
	IsBluetooth   bool
	IsVirtual     bool

	SSID         string
	Signal       int    // Signal strength (0-100%)
	Frequency    uint32 // MHz (2500, 5000) = (2.5 GHz, 5 Ghz)
	Channel      int
	BitRate      int    // Current bitrate Kbit/s
	TxPower      int    // Transmit power dBm
	SecurityType string // "WPA2", "WPA3", "Open", etc
}

func (n *NetworkInfo) FrequencyString() string {
	if n.Frequency == 0 {
		return ""
	}
	ghz := float64(n.Frequency) / 1000.0
	if n.Channel > 0 {
		return fmt.Sprintf("%.1f GHz (Ch %d)", ghz, n.Channel)
	}
	return fmt.Sprintf("%.1f GHz", ghz)
}

func (n *NetworkInfo) BitRateString() string {
	if n.BitRate == 0 {
		return ""
	}
	if n.BitRate >= 1000 {
		return fmt.Sprintf("%.1f Mbit/s", float64(n.BitRate)/1000.0)
	}
	return fmt.Sprintf("%d Kbit/s", n.BitRate)
}

func (n *NetworkInfo) SignalString() string {
	if n.Signal == 0 {
		return ""
	}
	return fmt.Sprintf("%d%%", n.Signal)
}

func FrequencyToChannel(freqMHz uint32) int {
	switch {
	case freqMHz == 2484:
		return 14
	case freqMHz >= 2412 && freqMHz <= 2472:
		return int((freqMHz - 2407) / 5)
	case freqMHz >= 5170 && freqMHz <= 5825:
		return int((freqMHz - 5000) / 5)
	case freqMHz >= 5955 && freqMHz <= 7115:
		// 6 GHz (WiFi 6E)
		return int((freqMHz - 5950) / 5)
	default:
		return 0
	}
}

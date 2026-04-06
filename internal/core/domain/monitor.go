package domain

import "fmt"

type ConnectionStatus uint8

const (
	ConnectionConnected    ConnectionStatus = 1
	ConnectionDisconnected ConnectionStatus = 2
	ConnectionUnknown      ConnectionStatus = 3
)

func (c ConnectionStatus) String() string {
	switch c {
	case ConnectionConnected:
		return "Connected"
	case ConnectionDisconnected:
		return "Disconnected"
	default:
		return "Unknown"
	}
}

type ConnectorType uint32

const (
	ConnectorUnknown     ConnectorType = 0
	ConnectorVGA         ConnectorType = 1
	ConnectorDVII        ConnectorType = 2
	ConnectorDVID        ConnectorType = 3
	ConnectorDVIA        ConnectorType = 4
	ConnectorComposite   ConnectorType = 5
	ConnectorSVideo      ConnectorType = 6
	ConnectorLVDS        ConnectorType = 7
	ConnectorComponent   ConnectorType = 8
	Connector9PinDIN     ConnectorType = 9
	ConnectorDisplayPort ConnectorType = 10
	ConnectorHDMIA       ConnectorType = 11
	ConnectorHDMIB       ConnectorType = 12
	ConnectorTV          ConnectorType = 13
	ConnectorEDP         ConnectorType = 14
	ConnectorVirtual     ConnectorType = 15
	ConnectorDSI         ConnectorType = 16
	ConnectorDPI         ConnectorType = 17
	ConnectorWriteback   ConnectorType = 18
	ConnectorSPI         ConnectorType = 19
	ConnectorUSB         ConnectorType = 20
)

func (c ConnectorType) String() string {
	names := map[ConnectorType]string{
		ConnectorUnknown:     "Unknown",
		ConnectorVGA:         "VGA",
		ConnectorDVII:        "DVI-I",
		ConnectorDVID:        "DVI-D",
		ConnectorDVIA:        "DVI-A",
		ConnectorComposite:   "Composite",
		ConnectorSVideo:      "S-Video",
		ConnectorLVDS:        "LVDS",
		ConnectorComponent:   "Component",
		Connector9PinDIN:     "9-Pin DIN",
		ConnectorDisplayPort: "DisplayPort",
		ConnectorHDMIA:       "HDMI",
		ConnectorHDMIB:       "HDMI-B",
		ConnectorTV:          "TV",
		ConnectorEDP:         "eDP",
		ConnectorVirtual:     "Virtual",
		ConnectorDSI:         "DSI",
		ConnectorDPI:         "DPI",
		ConnectorWriteback:   "Writeback",
		ConnectorSPI:         "SPI",
		ConnectorUSB:         "USB",
	}
	if name, ok := names[c]; ok {
		return name
	}
	return "Unknown"
}

type DisplayMode struct {
	Width       uint16
	Height      uint16
	RefreshRate uint32
	Name        string
	IsCurrent   bool
	IsPreferred bool
}

type Monitor struct {
	ID          uint32
	Name        string
	Type        ConnectorType
	TypeIndex   uint32
	Status      ConnectionStatus
	WidthMM     uint32
	HeightMM    uint32
	CurrentMode *DisplayMode
	Modes       []DisplayMode
	EncoderID   uint32
	DPMS        string
}

func (m *Monitor) FormattedName() string {
	if m.Name != "" {
		return m.Name
	}
	return m.Type.String()
}

func (m *Monitor) Resolution() string {
	if m.CurrentMode == nil {
		return "N/A"
	}
	return m.CurrentMode.Name
}

type GPUInfo struct {
	CardIndex     int
	CardPath      string
	DriverName    string
	DriverVersion string
	DriverDate    string
	DriverDesc    string
	Monitors      []*Monitor
	PCIDevice     string

	VRAMTotal   uint64
	VRAMUsed    uint64
	GPUClockMax uint32
	GPUClockMin uint32
	MemClockMax uint32
	MemClockMin uint32

	SubsystemVendor string
	SubsystemDevice string

	VBIOSVersion  string
	VRAMVendor    string
	UniqueID      string
	PCIeLinkSpeed string
	PCIeLinkWidth string
	MaxLinkSpeed  string
	MaxLinkWidth  string
}

func (g *GPUInfo) VRAMTotalGB() float64 {
	return float64(g.VRAMTotal) / (1024 * 1024 * 1024)
}

func (g *GPUInfo) VRAMTotalString() string {
	mb := g.VRAMTotal / (1024 * 1024)
	if mb >= 1024 {
		gb := float64(mb) / 1024
		if gb == float64(int(gb)) {
			return fmt.Sprintf("%.0f GB", gb)
		}
		return fmt.Sprintf("%.1f GB", gb)
	}
	if mb > 0 {
		return fmt.Sprintf("%d MB", mb)
	}
	return ""
}

func (g *GPUInfo) VRAMUsedGB() float64 {
	return float64(g.VRAMUsed) / (1024 * 1024 * 1024)
}

func (g *GPUInfo) ConnectedMonitors() []*Monitor {
	var connected []*Monitor
	for _, m := range g.Monitors {
		if m.Status == ConnectionConnected {
			connected = append(connected, m)
		}
	}
	return connected
}

package domain

import "fmt"

type DeviceType string

const (
	DeviceTypePCI     DeviceType = "pci"
	DeviceTypeUSB     DeviceType = "usb"
	DeviceTypeStorage DeviceType = "storage"
	DeviceTypeUnknown DeviceType = "unknown"
)

type DeviceClass string

const (
	DeviceClassGPU           DeviceClass = "gpu"
	DeviceClassNetwork       DeviceClass = "network"
	DeviceClassStorage       DeviceClass = "storage"
	DeviceClassAudio         DeviceClass = "audio"
	DeviceClassUSBController DeviceClass = "usb_controller"
	DeviceClassBridge        DeviceClass = "bridge"
	DeviceClassInput         DeviceClass = "input"
	DeviceClassSerial        DeviceClass = "serial"
	DeviceClassWireless      DeviceClass = "wireless"
	DeviceClassCamera        DeviceClass = "camera"
	DeviceClassPrinter       DeviceClass = "printer"
	DeviceClassHub           DeviceClass = "hub"
	DeviceClassSecurity      DeviceClass = "security"
	DeviceClassDisplay       DeviceClass = "display"
	DeviceClassProcessor     DeviceClass = "processor"
	DeviceClassSensor        DeviceClass = "sensor"
	DeviceClassPower         DeviceClass = "power"
	DeviceClassLighting      DeviceClass = "lighting"
	DeviceClassMemory        DeviceClass = "memory"
	DeviceClassPortable      DeviceClass = "portable"
	DeviceClassDebug         DeviceClass = "debug"
	DeviceClassOther         DeviceClass = "other"
)

type DeviceNode struct {
	ID              string
	ParentID        string
	Name            string
	Vendor          string
	Product         string
	VendorID        string
	ProductID       string
	Driver          string
	Path            string
	Type            DeviceType
	Class           DeviceClass
	ClassCode       string
	IOMMUGroup      int
	Capabilities    []string
	Children        []*DeviceNode
	GPUInfo         *GPUInfo
	StorageInfo     *StorageInfo
	NetworkInfo     *NetworkInfo
	ProcessorInfo   *ProcessorInfo
	DisplayInfo     *DisplayInfo
	MotherboardInfo *MotherboardInfo
	MemoryModule    *MemoryModuleInfo
	DevicePaths     []string
	SensorReadings  []*SensorInfo
	BatteryInfo     *BatteryInfo
	HasDebugPort    bool
	Resources       *DeviceResources
	Interfaces      []*DeviceInterface
}

type ResourceType string

const (
	ResourceTypeMemory ResourceType = "memory"
	ResourceTypeIO     ResourceType = "io"
	ResourceTypeIRQ    ResourceType = "irq"
)

type Resource struct {
	Type         ResourceType
	Start        uint64
	End          uint64
	Size         uint64
	Flags        uint64
	Prefetchable bool
	Index        int
}

func (r *Resource) FormattedRange() string {
	switch r.Type {
	case ResourceTypeMemory:
		return fmt.Sprintf("0x%X - 0x%X", r.Start, r.End)
	case ResourceTypeIO:
		return fmt.Sprintf("0x%04X - 0x%04X", r.Start, r.End)
	case ResourceTypeIRQ:
		return fmt.Sprintf("%d", r.Start)
	default:
		return fmt.Sprintf("0x%X", r.Start)
	}
}

func (r *Resource) FormattedSize() string {
	if r.Size == 0 {
		return ""
	}
	if r.Size >= 1024*1024*1024 {
		return fmt.Sprintf("%.1f GB", float64(r.Size)/(1024*1024*1024))
	}
	if r.Size >= 1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(r.Size)/(1024*1024))
	}
	if r.Size >= 1024 {
		return fmt.Sprintf("%.1f KB", float64(r.Size)/1024)
	}
	return fmt.Sprintf("%d bytes", r.Size)
}

type DeviceResources struct {
	IRQ       int
	IRQs      []int
	NUMANode  int
	LocalCPUs string
	Memory    []Resource
	IOPorts   []Resource
}

type HardwareEvent struct {
	Action   string
	DeviceID string
	Type     DeviceType
}

func (d *DeviceNode) DisplayName() string {
	// Prefer the actual drive model from sysfs over the PCI database family name,
	// which often combines multiple SKUs into one string (e.g., "SN770 / SN740 256GB / SN560").
	if d.StorageInfo != nil && d.StorageInfo.Model != "" {
		return ShortenDeviceName(d.StorageInfo.Model, d.Vendor, d.Product)
	}
	return ShortenDeviceName(d.Name, d.Vendor, d.Product)
}

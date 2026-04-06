package domain

type InterfaceType string

const (
	InterfaceGraphics  InterfaceType = "graphics"
	InterfaceSensor    InterfaceType = "sensor"
	InterfaceStorage   InterfaceType = "storage"
	InterfaceNetwork   InterfaceType = "network"
	InterfaceDisplay   InterfaceType = "display"
	InterfaceAudio     InterfaceType = "audio"
	InterfaceInput     InterfaceType = "input"
	InterfaceSerial    InterfaceType = "serial"
	InterfacePower     InterfaceType = "power"
	InterfaceLighting  InterfaceType = "lighting"
	InterfaceUSB       InterfaceType = "usb"
	InterfaceProcessor InterfaceType = "processor"
	InterfaceMemory    InterfaceType = "memory"
	InterfaceCamera    InterfaceType = "camera"
	InterfaceSecurity  InterfaceType = "security"
	InterfacePortable  InterfaceType = "portable"
	InterfaceDebug     InterfaceType = "debug"
	InterfaceSystem    InterfaceType = "system"
	InterfaceOther     InterfaceType = "other"
)

type DeviceInterface struct {
	Type         InterfaceType
	Category     DeviceClass
	SectionTitle string
	ScrollTag    string

	Name     string
	Subtitle string

	GPUInfo         *GPUInfo
	StorageInfo     *StorageInfo
	NetworkInfo     *NetworkInfo
	ProcessorInfo   *ProcessorInfo
	DisplayInfo     *DisplayInfo
	SensorData      []*SensorInfo
	BatteryInfo     *BatteryInfo
	MotherboardInfo *MotherboardInfo
	MemoryModule    *MemoryModuleInfo
	DevicePaths     []string
}

func InterfaceTypeToCategory(t InterfaceType) DeviceClass {
	switch t {
	case InterfaceGraphics:
		return DeviceClassGPU
	case InterfaceSensor:
		return DeviceClassSensor
	case InterfaceStorage:
		return DeviceClassStorage
	case InterfaceNetwork:
		return DeviceClassNetwork
	case InterfaceDisplay:
		return DeviceClassDisplay
	case InterfaceAudio:
		return DeviceClassAudio
	case InterfaceInput:
		return DeviceClassInput
	case InterfaceSerial:
		return DeviceClassSerial
	case InterfacePower:
		return DeviceClassPower
	case InterfaceLighting:
		return DeviceClassLighting
	case InterfaceUSB:
		return DeviceClassUSBController
	case InterfaceProcessor:
		return DeviceClassProcessor
	case InterfaceMemory:
		return DeviceClassMemory
	case InterfaceCamera:
		return DeviceClassCamera
	case InterfaceSecurity:
		return DeviceClassSecurity
	case InterfacePortable:
		return DeviceClassPortable
	case InterfaceDebug:
		return DeviceClassDebug
	case InterfaceSystem:
		return DeviceClassBridge
	default:
		return DeviceClassOther
	}
}

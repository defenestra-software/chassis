package gtk

import (
	"strings"

	"defenestra.io/chassis/internal/core/domain"
)

var classDisplayNames = map[domain.DeviceClass]string{
	domain.DeviceClassGPU:            "Graphics Card",
	domain.DeviceClassNetwork:        "Network Controller",
	domain.DeviceClassStorage:        "Storage Controller",
	domain.DeviceClassAudio:          "Audio Device",
	domain.DeviceClassUSBController:  "USB Controller",
	domain.DeviceClassBridge:         "System Device",
	domain.DeviceClassInput:          "Input Device",
	domain.DeviceClassGameController: "Game Controller",
	domain.DeviceClassSerial:         "Serial Controller",
	domain.DeviceClassWireless:       "Wireless Device",
	domain.DeviceClassCamera:         "Camera",
	domain.DeviceClassPrinter:        "Printer",
	domain.DeviceClassHub:            "USB Hub",
	domain.DeviceClassSecurity:       "Security Device",
	domain.DeviceClassDisplay:        "Display",
	domain.DeviceClassProcessor:      "Processor",
	domain.DeviceClassSensor:         "Sensor",
	domain.DeviceClassPower:          "Power Supply",
	domain.DeviceClassLighting:       "Lighting Controller",
	domain.DeviceClassMemory:         "Memory Module",
	domain.DeviceClassPortable:       "Portable Device",
	domain.DeviceClassVRHeadset:      "VR Headset",
	domain.DeviceClassOther:          "Other",
}

var typeDisplayNames = map[domain.DeviceType]string{
	domain.DeviceTypePCI:     "PCI",
	domain.DeviceTypeUSB:     "USB",
	domain.DeviceTypeStorage: "Storage",
	domain.DeviceTypeUnknown: "Unknown",
}

var capabilityLabels = map[string]string{
	"unbind": "Can be disabled",
	"reset":  "Supports hardware reset",
}

type categoryInfo struct {
	class       domain.DeviceClass
	displayName string
	iconName    string
	sortOrder   int
}

var categoryRegistry = []categoryInfo{
	{domain.DeviceClassProcessor, "Processors", "computer-symbolic", 1},
	{domain.DeviceClassGPU, "Graphics", "applications-graphics-symbolic", 2},
	{domain.DeviceClassDisplay, "Displays", "video-single-display-symbolic", 3},
	{domain.DeviceClassNetwork, "Networking", "network-transmit-receive-symbolic", 4},
	{domain.DeviceClassStorage, "Storage", "drive-harddisk-symbolic", 5},
	{domain.DeviceClassAudio, "Audio", "audio-card-symbolic", 6},
	{domain.DeviceClassUSBController, "USB", "drive-removable-media-symbolic", 7},
	{domain.DeviceClassInput, "Input Devices", "input-keyboard-symbolic", 8},
	{domain.DeviceClassGameController, "Game Controllers", "input-gaming-symbolic", 9},
	{domain.DeviceClassVRHeadset, "VR Headsets", "video-display-symbolic", 10},
	{domain.DeviceClassCamera, "Cameras", "camera-photo-symbolic", 11},
	{domain.DeviceClassMemory, "Memory", "drive-harddisk-solidstate-symbolic", 12},
	{domain.DeviceClassSensor, "Sensors", "applications-engineering-symbolic", 13},
	{domain.DeviceClassLighting, "Lighting", "preferences-color-symbolic", 14},
	{domain.DeviceClassPower, "Power", "battery-symbolic", 15},
	{domain.DeviceClassPrinter, "Printers", "printer-symbolic", 16},
	{domain.DeviceClassPortable, "Portable Devices", "phone-symbolic", 17},
	{domain.DeviceClassSecurity, "Security Devices", "channel-secure-symbolic", 18},
	{domain.DeviceClassSerial, "Serial Devices", "preferences-other-symbolic", 19},
	{domain.DeviceClassDebug, "Debug Devices", "utilities-terminal-symbolic", 20},
	{domain.DeviceClassBridge, "System Devices", "emblem-system-symbolic", 21},
	{domain.DeviceClassOther, "Other Devices", "dialog-question-symbolic", 99},
}

var subsystemVendors = map[string]string{
	"1002": "AMD",
	"1043": "ASUS",
	"1458": "Gigabyte",
	"1462": "MSI",
	"10de": "NVIDIA",
	"1849": "ASRock",
	"196d": "Club 3D",
	"1682": "XFX",
	"1787": "Sapphire",
	"148c": "PowerColor",
	"174b": "Sapphire",
	"1da2": "Sapphire",
	"1092": "Diamond",
	"3842": "EVGA",
	"19da": "Zotac",
	"1569": "Palit",
	"1b4c": "Gainward",
	"10b0": "Gainward",
	"7377": "Colorful",
	"1af4": "QEMU/KVM",
	"15ad": "VMware",
	"80ee": "VirtualBox",
	"8086": "Intel",
	"1028": "Dell",
	"103c": "HP",
	"17aa": "Lenovo",
}

func getCategoryIcon(class domain.DeviceClass) string {
	for _, cat := range categoryRegistry {
		if cat.class == class {
			return cat.iconName
		}
	}
	return ""
}

func lookupSubsystemVendor(vendorID string) string {
	if name, ok := subsystemVendors[strings.ToLower(vendorID)]; ok {
		return name
	}
	return ""
}

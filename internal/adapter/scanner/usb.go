package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"defenestra.io/chassis/internal/core/domain"
)

func (s *GHWScanner) scanUSBDevices() (*domain.DeviceNode, error) {
	usbRoot := &domain.DeviceNode{
		ID:       "usb",
		ParentID: "root",
		Name:     "USB Devices",
		Type:     domain.DeviceTypeUSB,
		Class:    domain.DeviceClassOther,
		Children: make([]*domain.DeviceNode, 0),
	}

	usbPath := "/sys/bus/usb/devices"
	if s.chrootPath != "" {
		usbPath = filepath.Join(s.chrootPath, usbPath)
	}

	entries, err := os.ReadDir(usbPath)
	if err != nil {
		return usbRoot, nil // Return empty root if USB scanning fails
	}

	deviceMap := make(map[string]*domain.DeviceNode)

	for _, entry := range entries {
		name := entry.Name()
		// Skip interfaces (contain :), only process devices
		if strings.Contains(name, ":") {
			continue
		}

		devPath := filepath.Join(usbPath, name)
		node := s.usbDeviceToNode(devPath, name)
		if node != nil {
			deviceMap[node.ID] = node
		}
	}

	// Build hierarchy - USB devices have parent relationships like 1-0 -> 1-1 -> 1-1.1
	for _, node := range deviceMap {
		parentID := s.findUSBParent(node.ID, deviceMap)
		if parentID != "" {
			node.ParentID = parentID
			if parent, ok := deviceMap[parentID]; ok {
				parent.Children = append(parent.Children, node)
				continue
			}
		}
		// Top-level USB device (root hub)
		node.ParentID = "usb"
		usbRoot.Children = append(usbRoot.Children, node)
	}

	// Scan HID bus for paired peripherals behind receivers (e.g., Logitech
	// Unifying/Bolt mice and keyboards). These are HID devices with a
	// different VID:PID than their parent USB device.
	s.scanHIDPeripherals(deviceMap)

	return usbRoot, nil
}

func (s *GHWScanner) usbDeviceToNode(sysfsPath, name string) *domain.DeviceNode {
	// Read device info
	vendorID := s.readSysfsFile(filepath.Join(sysfsPath, "idVendor"))
	productID := s.readSysfsFile(filepath.Join(sysfsPath, "idProduct"))
	manufacturer := s.readSysfsFile(filepath.Join(sysfsPath, "manufacturer"))
	product := s.readSysfsFile(filepath.Join(sysfsPath, "product"))
	bDeviceClass := s.readSysfsFile(filepath.Join(sysfsPath, "bDeviceClass"))
	driver := s.readDriver(sysfsPath)

	// If driver is generic "usb", try to find a more specific interface-level driver
	if driver == "" || driver == "usb" {
		if betterDriver := s.findInterfaceDriver(sysfsPath, name); betterDriver != "" {
			driver = betterDriver
		} else if driver == "usb" {
			driver = "" // Clear generic "usb" driver - not useful
		}
	}

	// Read USB speed and port count
	usbSpeed := s.readSysfsFile(filepath.Join(sysfsPath, "speed"))
	maxPorts := s.readSysfsFile(filepath.Join(sysfsPath, "maxchild"))

	// Skip if no vendor/product ID (not a real device)
	if vendorID == "" && productID == "" {
		// Check if it's a root hub
		if product == "" && manufacturer == "" {
			return nil
		}
	}

	// Clean up root hub names - kernel puts full version string in manufacturer
	isRootHub := strings.HasPrefix(name, "usb")
	if isRootHub && manufacturer != "" && strings.Contains(manufacturer, "xhci-hcd") {
		manufacturer = ""
	}

	// Build name
	deviceName := name
	if isRootHub {
		// Friendly name for root hubs based on USB version and speed
		speedLabel := ""
		switch {
		case usbSpeed == "10000":
			speedLabel = "USB 3.1 Gen 2"
		case usbSpeed == "5000":
			speedLabel = "USB 3.0"
		case usbSpeed == "480":
			speedLabel = "USB 2.0"
		case usbSpeed == "12":
			speedLabel = "USB 1.1"
		default:
			speedLabel = "USB"
		}
		if maxPorts != "" && maxPorts != "0" {
			deviceName = fmt.Sprintf("%s Host Controller (%s ports)", speedLabel, maxPorts)
		} else {
			deviceName = fmt.Sprintf("%s Host Controller", speedLabel)
		}
		product = deviceName
	} else if product != "" {
		deviceName = product
	} else if manufacturer != "" {
		deviceName = manufacturer
	}

	// For non-root USB devices, add speed to product for subtitle display
	if !isRootHub && usbSpeed != "" {
		speedLabel := ""
		switch usbSpeed {
		case "10000":
			speedLabel = "USB 3.1"
		case "5000":
			speedLabel = "USB 3.0"
		case "480":
			speedLabel = "USB 2.0"
		case "12":
			speedLabel = "USB 1.1"
		case "1":
			speedLabel = "USB 1.0"
		}
		if speedLabel != "" && product != "" {
			product = product + " · " + speedLabel
		} else if speedLabel != "" {
			product = speedLabel
		}
	}

	// Determine device class - check device level first, then fall back
	// to interface-level descriptors (many USB devices report class 00 or ef
	// at the device level and define their actual class at the interface level)
	class := s.classifyUSBDevice(bDeviceClass)
	if class == domain.DeviceClassOther {
		class = s.classifyUSBDeviceByInterface(sysfsPath)
	}

	// Override for security keys - these report as HID but users expect
	// them under Security. Match by known vendor IDs and product names.
	if class == domain.DeviceClassInput {
		class = s.reclassifySecurityKey(class, vendorID, product)
	}

	// Override for known non-input HID devices (RGB controllers, etc.)
	// Check regardless of current class - some RGB controllers report as
	// HID (Input), others as vendor-specific (Other).
	class = s.reclassifyMiscHID(class, vendorID, productID)

	// Override for portable devices (phones, tablets, media players)
	if class == domain.DeviceClassOther {
		class = s.reclassifyPortableDevice(class, vendorID, product, sysfsPath)
	}

	// Check for vendor-specific debug interfaces (JTAG, SWD, etc.)
	// Skip for devices already reclassified as lighting controllers
	// due to some RGB controllers using vendor-specific interfaces (ff)
	hasDebug := class != domain.DeviceClassLighting && s.hasVendorDebugInterface(sysfsPath)

	id := fmt.Sprintf("usb:%s", name)

	return &domain.DeviceNode{
		ID:           id,
		ParentID:     "usb",
		Name:         deviceName,
		Vendor:       manufacturer,
		Product:      product,
		VendorID:     vendorID,
		ProductID:    productID,
		Driver:       driver,
		Path:         filepath.Join("/sys/bus/usb/devices", name),
		Type:         domain.DeviceTypeUSB,
		Class:        class,
		ClassCode:    bDeviceClass,
		IOMMUGroup:   -1,
		HasDebugPort: hasDebug,
		Capabilities: []string{"unbind"},
		Children:     make([]*domain.DeviceNode, 0),
	}
}

func (s *GHWScanner) findUSBParent(id string, deviceMap map[string]*domain.DeviceNode) string {
	// USB device names follow pattern: bus-port.port.port
	// e.g., 1-1.2.3 -> parent is 1-1.2 -> parent is 1-1 -> parent is 1-0 (root hub)
	name := strings.TrimPrefix(id, "usb:")

	// Root hubs (like "usb1", "usb2") have no parent
	if strings.HasPrefix(name, "usb") {
		return ""
	}

	// Find last dot or dash
	lastDot := strings.LastIndex(name, ".")
	lastDash := strings.LastIndex(name, "-")

	if lastDot > lastDash {
		parentName := name[:lastDot]
		parentID := "usb:" + parentName
		if _, ok := deviceMap[parentID]; ok {
			return parentID
		}
	}

	return ""
}

// scanHIDPeripherals finds wireless peripherals (mice, keyboards) behind USB
// receivers by scanning /sys/bus/hid/devices/. Devices with a VID:PID that
// differs from their parent USB device are actual peripherals, not just
// additional HID collections of the same device.
func (s *GHWScanner) scanHIDPeripherals(usbDeviceMap map[string]*domain.DeviceNode) {
	hidPath := "/sys/bus/hid/devices"
	if s.chrootPath != "" {
		hidPath = filepath.Join(s.chrootPath, hidPath)
	}

	entries, err := os.ReadDir(hidPath)
	if err != nil {
		return
	}

	// Track which HID VID:PIDs we've already added (skip duplicate collections)
	seen := map[string]bool{}

	for _, entry := range entries {
		devPath := filepath.Join(hidPath, entry.Name())

		// Parse HID device name: BBBB:VVVV:PPPP.NNNN
		parts := strings.SplitN(entry.Name(), ":", 3)
		if len(parts) != 3 {
			continue
		}
		hidVID := strings.ToLower(parts[1])
		pidParts := strings.SplitN(parts[2], ".", 2)
		if len(pidParts) != 2 {
			continue
		}
		hidPID := strings.ToLower(pidParts[0])
		hidVIDPID := hidVID + ":" + hidPID

		// Skip if we already processed this VID:PID
		if seen[hidVIDPID] {
			continue
		}

		// Find the parent USB device by walking up the sysfs path
		resolvedPath, err := filepath.EvalSymlinks(devPath)
		if err != nil {
			continue
		}

		parentUSBAddr := ""
		parentUSBVIDPID := ""
		pathParts := strings.Split(resolvedPath, "/")
		usbAddrPattern := regexp.MustCompile(`^\d+-[\d.]+$`)

		for i := len(pathParts) - 1; i >= 0; i-- {
			if usbAddrPattern.MatchString(pathParts[i]) {
				parentUSBAddr = pathParts[i]
				break
			}
		}

		if parentUSBAddr == "" {
			continue
		}

		// Get the parent USB device's VID:PID
		parentNode, ok := usbDeviceMap["usb:"+parentUSBAddr]
		if !ok {
			continue
		}
		parentUSBVIDPID = strings.ToLower(parentNode.VendorID + ":" + parentNode.ProductID)

		// If VID:PID matches the parent USB device, it's just another HID
		// collection of the same device (e.g., keyboard's media keys endpoint).
		// Skip it - the USB device already represents it.
		if hidVIDPID == parentUSBVIDPID {
			continue
		}

		seen[hidVIDPID] = true

		// This is a paired peripheral behind a receiver. Read its info.
		hidName := s.readHIDUeventField(devPath, "HID_NAME")
		if hidName == "" {
			continue
		}

		// Determine class from driver or name
		driver := s.readDriver(devPath)
		class := s.classifyHIDPeripheral(hidName, driver)

		peripheral := &domain.DeviceNode{
			ID:           fmt.Sprintf("hid:%s:%s", hidVID, hidPID),
			ParentID:     parentNode.ID,
			Name:         hidName,
			Vendor:       parentNode.Vendor,
			VendorID:     hidVID,
			ProductID:    hidPID,
			Driver:       driver,
			Path:         devPath,
			Type:         domain.DeviceTypeUSB,
			Class:        class,
			IOMMUGroup:   -1,
			Capabilities: []string{},
			Children:     make([]*domain.DeviceNode, 0),
		}

		parentNode.Children = append(parentNode.Children, peripheral)

		// Reclassify the parent receiver as USB Controller since it's
		// infrastructure, not an input device itself
		if parentNode.Class == domain.DeviceClassInput {
			parentNode.Class = domain.DeviceClassUSBController
		}
	}
}

func (s *GHWScanner) readHIDUeventField(devPath, field string) string {
	data, err := os.ReadFile(filepath.Join(devPath, "uevent"))
	if err != nil {
		return ""
	}
	prefix := field + "="
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}
	return ""
}

func (s *GHWScanner) classifyHIDPeripheral(name, driver string) domain.DeviceClass {
	nameLower := strings.ToLower(name)

	if strings.Contains(nameLower, "mouse") ||
		strings.Contains(nameLower, "trackball") ||
		strings.Contains(nameLower, "touchpad") {
		return domain.DeviceClassInput
	}
	if strings.Contains(nameLower, "keyboard") {
		return domain.DeviceClassInput
	}
	if strings.Contains(nameLower, "headset") ||
		strings.Contains(nameLower, "speaker") {
		return domain.DeviceClassAudio
	}

	// Check driver name
	switch driver {
	case "logitech-hidpp-device":
		return domain.DeviceClassInput
	}

	return domain.DeviceClassInput // Default: most HID peripherals are input
}

func (s *GHWScanner) classifyUSBDevice(bDeviceClass string) domain.DeviceClass {
	// USB device-level class codes
	switch bDeviceClass {
	case "09":
		return domain.DeviceClassHub
	case "03":
		return domain.DeviceClassInput // HID
	case "08":
		return domain.DeviceClassStorage // Mass storage
	case "02":
		return domain.DeviceClassSerial // CDC/Communications
	case "07":
		return domain.DeviceClassPrinter
	case "0e":
		return domain.DeviceClassCamera // Video
	case "e0":
		return domain.DeviceClassWireless // Wireless controller
	case "01":
		return domain.DeviceClassAudio
	default:
		return domain.DeviceClassOther
	}
}

// classifyUSBDeviceByInterface checks interface-level descriptors when the
// device-level class is 00 (defined at interface) or ef (miscellaneous).
// Many USB devices like webcams, BLE adapters, and audio devices only declare
// their actual function at the interface level (bInterfaceClass).
func (s *GHWScanner) classifyUSBDeviceByInterface(sysfsPath string) domain.DeviceClass {
	entries, err := os.ReadDir(sysfsPath)
	if err != nil {
		return domain.DeviceClassOther
	}

	// Interface directories look like "1-2:1.0" (contain a colon)
	// Use a priority system: some classes are more significant than others
	best := domain.DeviceClassOther
	bestPriority := 0

	for _, entry := range entries {
		if !strings.Contains(entry.Name(), ":") {
			continue
		}

		ifaceClass := s.readSysfsFile(filepath.Join(sysfsPath, entry.Name(), "bInterfaceClass"))
		if ifaceClass == "" {
			continue
		}

		candidate, priority := classifyUSBInterfaceClass(ifaceClass)
		if priority > bestPriority {
			best = candidate
			bestPriority = priority
		}
	}

	return best
}

func classifyUSBInterfaceClass(bInterfaceClass string) (domain.DeviceClass, int) {
	switch bInterfaceClass {
	case "0e": // Video
		return domain.DeviceClassCamera, 10
	case "01": // Audio
		return domain.DeviceClassAudio, 9
	case "03": // HID (keyboard, mouse, etc.)
		return domain.DeviceClassInput, 8
	case "e0": // Wireless (Bluetooth/BLE)
		return domain.DeviceClassWireless, 7
	case "08": // Mass Storage
		return domain.DeviceClassStorage, 6
	case "07": // Printer
		return domain.DeviceClassPrinter, 5
	case "02": // CDC/Communications
		return domain.DeviceClassSerial, 4
	case "09": // Hub
		return domain.DeviceClassHub, 3
	case "0a": // CDC-Data
		return domain.DeviceClassSerial, 2
	default:
		return domain.DeviceClassOther, 0
	}
}

// hasVendorDebugInterface checks if a USB device has a vendor-specific interface
// (bInterfaceClass ff) that is likely a debug/programming port (JTAG, SWD, etc.).
// Common on debug probes like J-Link, ST-Link, CMSIS-DAP, Black Magic Probe.
func (s *GHWScanner) hasVendorDebugInterface(sysfsPath string) bool {
	entries, err := os.ReadDir(sysfsPath)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if !strings.Contains(entry.Name(), ":") {
			continue
		}
		ifaceClass := s.readSysfsFile(filepath.Join(sysfsPath, entry.Name(), "bInterfaceClass"))
		if ifaceClass == "ff" {
			return true
		}
	}
	return false
}

// findInterfaceDriver checks USB interface directories and HID sub-devices
// for a more specific driver than the generic "usb" device driver.
func (s *GHWScanner) findInterfaceDriver(sysfsPath, deviceName string) string {
	entries, err := os.ReadDir(sysfsPath)
	if err != nil {
		return ""
	}

	skip := map[string]bool{"usb": true, "hub": true}
	bestDriver := ""

	for _, entry := range entries {
		// Interface dirs look like "1-4:1.0"
		if !strings.Contains(entry.Name(), ":") {
			continue
		}

		ifacePath := filepath.Join(sysfsPath, entry.Name())
		driverLink := filepath.Join(ifacePath, "driver")
		target, err := os.Readlink(driverLink)
		if err != nil {
			continue
		}
		drv := filepath.Base(target)

		if drv == "usbhid" {
			// Check HID sub-devices for a more specific driver (e.g., razerkbd)
			if hidDriver := s.findHIDDriver(ifacePath); hidDriver != "" {
				return hidDriver
			}
			if bestDriver == "" {
				bestDriver = drv
			}
			continue
		}

		if drv != "" && !skip[drv] {
			return drv
		}
	}
	return bestDriver
}

// findHIDDriver checks HID sub-device directories for a specific driver.
func (s *GHWScanner) findHIDDriver(ifacePath string) string {
	entries, err := os.ReadDir(ifacePath)
	if err != nil {
		return ""
	}

	hidPattern := regexp.MustCompile(`^[0-9A-Fa-f]{4}:[0-9A-Fa-f]{4}:[0-9A-Fa-f]{4}\.[0-9A-Fa-f]+$`)
	skip := map[string]bool{"hid-generic": true, "hid-multitouch": true}

	for _, entry := range entries {
		if !hidPattern.MatchString(entry.Name()) {
			continue
		}
		driverLink := filepath.Join(ifacePath, entry.Name(), "driver")
		target, err := os.Readlink(driverLink)
		if err != nil {
			continue
		}
		drv := filepath.Base(target)
		if drv != "" && !skip[drv] {
			return drv
		}
	}
	return ""
}

var securityKeyVendors = map[string]bool{
	"1050": true, // Yubico
	"096e": true, // Feitian
	"20a0": true, // Nitrokey
	"1209": true, // Generic (often used by open-source FIDO keys)
	"2581": true, // Ledger (crypto hardware wallet)
	"2c97": true, // Ledger (newer)
	"534c": true, // SatoshiLabs (Trezor)
}

var securityKeyPatterns = []string{
	"yubikey", "fido", "u2f", "security key", "nitrokey",
	"onlykey", "solokey", "trezor", "ledger",
}

func (s *GHWScanner) reclassifySecurityKey(current domain.DeviceClass, vendorID, product string) domain.DeviceClass {
	// Check vendor ID
	if securityKeyVendors[vendorID] {
		return domain.DeviceClassSecurity
	}

	// Check product name patterns
	productLower := strings.ToLower(product)
	for _, pattern := range securityKeyPatterns {
		if strings.Contains(productLower, pattern) {
			return domain.DeviceClassSecurity
		}
	}

	return current
}

// Sourced from OpenRGB's device detector tables.
var rgbControllerDevices = map[string]bool{
	// ITE Tech - Gigabyte RGB Fusion 2 USB controllers
	"048d:5702": true, // ITE Tech RGB LED Controller
	"048d:5711": true, // ITE Tech RGB LED Controller
	"048d:8297": true, // ITE Tech RGB Fusion 2 USB
	"048d:8910": true, // ITE Tech RGB LED Controller
	"048d:8950": true, // ITE Tech RGB Fusion 2 USB

	// ASUS AURA - motherboard + addressable LED controllers
	"0b05:1854": true, // ASUS AURA Core
	"0b05:1866": true, // ASUS AURA Core
	"0b05:1867": true, // ASUS AURA Addressable
	"0b05:1869": true, // ASUS AURA Core
	"0b05:1872": true, // ASUS AURA Addressable
	"0b05:18a3": true, // ASUS AURA Addressable
	"0b05:18a5": true, // ASUS AURA Addressable
	"0b05:18f3": true, // ASUS AURA Motherboard
	"0b05:1939": true, // ASUS AURA Motherboard
	"0b05:19af": true, // ASUS AURA Motherboard
	"0b05:1aa6": true, // ASUS AURA Motherboard
	"0b05:1bed": true, // ASUS AURA Motherboard

	// MSI Mystic Light controllers
	"1462:1562": true, // MSI Mystic Light
	"1462:1563": true, // MSI Mystic Light
	"1462:1564": true, // MSI Mystic Light
	"1462:1720": true, // MSI Mystic Light
	"1462:7b12": true, // MSI Mystic Light
	"1462:7b16": true, // MSI Mystic Light
	"1462:7b17": true, // MSI Mystic Light
	"1462:7b18": true, // MSI Mystic Light
	"1462:7b50": true, // MSI Mystic Light
	"1462:7b85": true, // MSI Mystic Light
	"1462:7b92": true, // MSI Mystic Light
	"1462:7b93": true, // MSI Mystic Light
	"1462:7c02": true, // MSI Mystic Light
	"1462:7c34": true, // MSI Mystic Light
	"1462:7c35": true, // MSI Mystic Light
	"1462:7c36": true, // MSI Mystic Light
	"1462:7c37": true, // MSI Mystic Light
	"1462:7c56": true, // MSI Mystic Light
	"1462:7c59": true, // MSI Mystic Light
	"1462:7c60": true, // MSI Mystic Light
	"1462:7c67": true, // MSI Mystic Light
	"1462:7c71": true, // MSI Mystic Light
	"1462:7c73": true, // MSI Mystic Light
	"1462:7c75": true, // MSI Mystic Light
	"1462:7c76": true, // MSI Mystic Light
	"1462:7c77": true, // MSI Mystic Light
	"1462:7c79": true, // MSI Mystic Light
	"1462:7c80": true, // MSI Mystic Light
	"1462:7c81": true, // MSI Mystic Light
	"1462:7c82": true, // MSI Mystic Light
	"1462:7c83": true, // MSI Mystic Light
	"1462:7c84": true, // MSI Mystic Light
	"1462:7c86": true, // MSI Mystic Light
	"1462:7c87": true, // MSI Mystic Light
	"1462:7c89": true, // MSI Mystic Light
	"1462:7c90": true, // MSI Mystic Light
	"1462:7c91": true, // MSI Mystic Light
	"1462:7c92": true, // MSI Mystic Light
	"1462:7c94": true, // MSI Mystic Light
	"1462:7c95": true, // MSI Mystic Light
	"1462:7d03": true, // MSI Mystic Light
	"1462:7d06": true, // MSI Mystic Light
	"1462:7d09": true, // MSI Mystic Light
	"1462:7d13": true, // MSI Mystic Light
	"1462:7d17": true, // MSI Mystic Light
	"1462:7d20": true, // MSI Mystic Light
	"1462:7d25": true, // MSI Mystic Light
	"1462:7d28": true, // MSI Mystic Light
	"1462:7d29": true, // MSI Mystic Light
	"1462:7d30": true, // MSI Mystic Light
	"1462:7d32": true, // MSI Mystic Light
	"1462:7d36": true, // MSI Mystic Light
	"1462:7d38": true, // MSI Mystic Light
	"1462:7d41": true, // MSI Mystic Light
	"1462:7d42": true, // MSI Mystic Light
	"1462:7d46": true, // MSI Mystic Light
	"1462:7d50": true, // MSI Mystic Light
	"1462:7d51": true, // MSI Mystic Light
	"1462:7d52": true, // MSI Mystic Light
	"1462:7d53": true, // MSI Mystic Light
	"1462:7d54": true, // MSI Mystic Light
	"1462:7d59": true, // MSI Mystic Light
	"1462:7d67": true, // MSI Mystic Light
	"1462:7d69": true, // MSI Mystic Light
	"1462:7d70": true, // MSI Mystic Light
	"1462:7d73": true, // MSI Mystic Light
	"1462:7d75": true, // MSI Mystic Light
	"1462:7d76": true, // MSI Mystic Light
	"1462:7d78": true, // MSI Mystic Light

	// Corsair - lighting nodes, commanders, and RGB controllers
	"1b1c:0c0b": true, // Corsair Lighting Node Pro
	"1b1c:0c10": true, // Corsair Commander Pro
	"1b1c:0c1a": true, // Corsair Lighting Node Core
	"1b1c:0c1c": true, // Corsair Commander Core
	"1b1c:0c1d": true, // Corsair Commander Core
	"1b1c:0c1e": true, // Corsair LS100 Lighting Kit
	"1b1c:0c23": true, // Corsair LT100
	"1b1c:0c2a": true, // Corsair Commander Core XT
	"1b1c:0c32": true, // Corsair Commander Core 2
	"1b1c:0c3c": true, // Corsair Commander Core
	"1b1c:0c3d": true, // Corsair Commander Core
	"1b1c:0c3e": true, // Corsair Commander Core
	"1b1c:1d00": true, // Corsair 1000D Obsidian RGB
	"1b1c:1d04": true, // Corsair SPEC OMEGA RGB

	// NZXT - Hue, Smart Device, and RGB fan controllers
	"1e71:2001": true, // NZXT Hue 2
	"1e71:2002": true, // NZXT Hue 2 Ambient
	"1e71:2005": true, // NZXT Hue 2 Motherboard
	"1e71:2006": true, // NZXT Smart Device V2
	"1e71:2009": true, // NZXT RGB Fan Controller
	"1e71:200b": true, // NZXT Hue 2 Motherboard
	"1e71:200d": true, // NZXT Smart Device V2.1
	"1e71:200e": true, // NZXT RGB Fan Controller
	"1e71:200f": true, // NZXT Smart Device V2.2
	"1e71:2010": true, // NZXT RGB Fan Controller
	"1e71:2011": true, // NZXT RGB Fan Controller
	"1e71:2012": true, // NZXT RGB Controller
	"1e71:2019": true, // NZXT RGB Fan Controller
	"1e71:201b": true, // NZXT B650E Motherboard
	"1e71:201f": true, // NZXT RGB Fan Controller
	"1e71:2020": true, // NZXT RGB Fan Controller
	"1e71:2021": true, // NZXT RGB Controller
	"1e71:2022": true, // NZXT RGB Fan Controller

	// Razer - standalone Chroma controllers (not peripherals)
	"1532:0f07": true, // Razer Chroma Mug Holder
	"1532:0f08": true, // Razer Base Station Chroma
	"1532:0f09": true, // Razer Chroma HDK
	"1532:0f0d": true, // Razer Laptop Stand Chroma
	"1532:0f0e": true, // Razer Chroma Case Fan Controller
	"1532:0f1a": true, // Razer Core X Chroma
	"1532:0f1f": true, // Razer Chroma Addressable RGB Controller
	"1532:0f20": true, // Razer Base Station V2 Chroma
}

// reclassifyMiscHID catches HID devices that aren't really input devices.
// Classification is based solely on known VID:PID pairs - product name
// matching is too unreliable since vendors reuse branding terms like
// "Chroma", "Aura", and "Mystic Light" across keyboards, mice, and
// actual RGB controllers.
func (s *GHWScanner) reclassifyMiscHID(current domain.DeviceClass, vendorID, productID string) domain.DeviceClass {
	vidpid := vendorID + ":" + productID
	if rgbControllerDevices[vidpid] {
		return domain.DeviceClassLighting
	}
	return current
}

var portableDeviceVendors = map[string]bool{
	"18d1": true, // Google (Pixel, Nexus)
	"04e8": true, // Samsung
	"2717": true, // Xiaomi
	"22b8": true, // Motorola
	"0bb4": true, // HTC
	"2a70": true, // OnePlus
	"1004": true, // LG
	"0fce": true, // Sony/Ericsson
	"2916": true, // Yota
	"05c6": true, // Qualcomm (many Android devices in fastboot)
	"1949": true, // Amazon (Kindle/Fire)
	"0e8d": true, // MediaTek (many budget phones)
	"2d95": true, // OPPO/Realme
	"2ae5": true, // Fairphone
	"1bbb": true, // T-Mobile
	"0502": true, // Acer (tablets)
	"17ef": true, // Lenovo (tablets)
	"2c1a": true, // Valve (Steam Deck, portable)
}

var portableNamePatterns = []string{
	"pixel", "galaxy", "iphone", "ipad", "phone",
	"oneplus", "kindle", "fire tablet", "steam deck",
	"nexus", "moto ", "redmi", "poco",
}

func (s *GHWScanner) reclassifyPortableDevice(current domain.DeviceClass, vendorID, product, sysfsPath string) domain.DeviceClass {
	// Check vendor ID
	if portableDeviceVendors[vendorID] {
		return domain.DeviceClassPortable
	}

	// Check product name
	productLower := strings.ToLower(product)
	for _, pattern := range portableNamePatterns {
		if strings.Contains(productLower, pattern) {
			return domain.DeviceClassPortable
		}
	}

	// Check for ADB interface (class ff, subclass 42, proto 01)
	entries, err := os.ReadDir(sysfsPath)
	if err == nil {
		for _, entry := range entries {
			if !strings.Contains(entry.Name(), ":") {
				continue
			}
			ifClass := s.readSysfsFile(filepath.Join(sysfsPath, entry.Name(), "bInterfaceClass"))
			ifSub := s.readSysfsFile(filepath.Join(sysfsPath, entry.Name(), "bInterfaceSubClass"))
			ifProto := s.readSysfsFile(filepath.Join(sysfsPath, entry.Name(), "bInterfaceProtocol"))
			if ifClass == "ff" && ifSub == "42" && ifProto == "01" {
				return domain.DeviceClassPortable // ADB
			}
			// MTP: class ff subclass ff proto 00, or class 06 (Still Image)
			if ifClass == "06" {
				return domain.DeviceClassPortable // PTP/MTP
			}
		}
	}

	return current
}

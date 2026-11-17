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
		if strings.Contains(name, ":") {
			continue
		}

		devPath := filepath.Join(usbPath, name)
		node := s.usbDeviceToNode(devPath, name)
		if node != nil {
			deviceMap[node.ID] = node
		}
	}

	for _, node := range deviceMap {
		parentID := s.findUSBParent(node.ID, deviceMap)
		if parentID != "" {
			node.ParentID = parentID
			if parent, ok := deviceMap[parentID]; ok {
				parent.Children = append(parent.Children, node)
				continue
			}
		}
		node.ParentID = "usb"
		usbRoot.Children = append(usbRoot.Children, node)
	}

	// HID behind Unifying/Bolt: different VID:PID than the parent USB dongle.
	s.scanHIDPeripherals(deviceMap)

	return usbRoot, nil
}

func (s *GHWScanner) usbDeviceToNode(sysfsPath, name string) *domain.DeviceNode {
	vendorID := s.readSysfsFile(filepath.Join(sysfsPath, "idVendor"))
	productID := s.readSysfsFile(filepath.Join(sysfsPath, "idProduct"))
	manufacturer := s.readSysfsFile(filepath.Join(sysfsPath, "manufacturer"))
	product := s.readSysfsFile(filepath.Join(sysfsPath, "product"))
	bDeviceClass := s.readSysfsFile(filepath.Join(sysfsPath, "bDeviceClass"))
	driver := s.readDriver(sysfsPath)

	if driver == "" || driver == "usb" {
		if betterDriver := s.findInterfaceDriver(sysfsPath, name); betterDriver != "" {
			driver = betterDriver
		} else if driver == "usb" {
			driver = "" // Clear generic "usb" driver - not useful
		}
	}

	usbSpeed := s.readSysfsFile(filepath.Join(sysfsPath, "speed"))
	maxPorts := s.readSysfsFile(filepath.Join(sysfsPath, "maxchild"))

	if vendorID == "" && productID == "" {
		if product == "" && manufacturer == "" {
			return nil
		}
	}

	// Kernel puts the full xhci version string in root-hub manufacturer.
	isRootHub := strings.HasPrefix(name, "usb")
	if isRootHub && manufacturer != "" && strings.Contains(manufacturer, "xhci-hcd") {
		manufacturer = ""
	}

	deviceName := name
	if isRootHub {
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

	// Class 00/ef at device level means the real class is on the interface.
	class := s.classifyUSBDevice(bDeviceClass)
	if class == domain.DeviceClassOther {
		class = s.classifyUSBDeviceByInterface(sysfsPath)
	}

	// Security keys report as HID; reclassify by known VID/product name.
	if class == domain.DeviceClassInput {
		class = s.reclassifySecurityKey(class, vendorID, product)
	}

	// DualSense and DualShock carry audio interfaces, which outrank HID above.
	hasGameController := s.hasXboxGamepadInterface(sysfsPath) || hasJoystickNode(sysfsPath)
	if hasGameController {
		switch class {
		case domain.DeviceClassInput, domain.DeviceClassAudio, domain.DeviceClassOther:
			class = domain.DeviceClassGameController
		}
	}

	if class == domain.DeviceClassOther {
		class = s.reclassifyPortableDevice(class, vendorID, product, sysfsPath)
	}

	known, _ := lookupUSBDevice(vendorID, productID)
	if known.Name != "" {
		deviceName = known.Name
	}
	if known.Class != "" {
		class = known.Class
	}

	// ff interfaces are debug probes, except on Xbox-protocol pads and on
	// listed devices that use ff for WebUSB or RGB control.
	hasDebug := known.Class == "" && !hasGameController && s.hasVendorDebugInterface(sysfsPath)

	id := fmt.Sprintf("usb:%s", name)

	return &domain.DeviceNode{
		ID:                id,
		ParentID:          "usb",
		Name:              deviceName,
		Vendor:            manufacturer,
		Product:           product,
		VendorID:          vendorID,
		ProductID:         productID,
		Driver:            driver,
		Path:              filepath.Join("/sys/bus/usb/devices", name),
		Type:              domain.DeviceTypeUSB,
		Class:             class,
		ClassCode:         bDeviceClass,
		IOMMUGroup:        -1,
		HasDebugPort:      hasDebug,
		HasGameController: hasGameController,
		Capabilities:      []string{"unbind"},
		Children:          make([]*domain.DeviceNode, 0),
	}
}

func (s *GHWScanner) findUSBParent(id string, deviceMap map[string]*domain.DeviceNode) string {
	// USB device names follow pattern: bus-port.port.port
	// e.g., 1-1.2.3 -> parent is 1-1.2 -> parent is 1-1 -> parent is 1-0 (root hub)
	name := strings.TrimPrefix(id, "usb:")

	if strings.HasPrefix(name, "usb") {
		return ""
	}

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

// scanHIDPeripherals finds wireless peripherals behind receivers (HID VID:PID
// differs from the parent USB device; same VID:PID is just another collection).
func (s *GHWScanner) scanHIDPeripherals(usbDeviceMap map[string]*domain.DeviceNode) {
	hidPath := "/sys/bus/hid/devices"
	if s.chrootPath != "" {
		hidPath = filepath.Join(s.chrootPath, hidPath)
	}

	entries, err := os.ReadDir(hidPath)
	if err != nil {
		return
	}

	seen := map[string]bool{}

	for _, entry := range entries {
		devPath := filepath.Join(hidPath, entry.Name())

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

		if seen[hidVIDPID] {
			continue
		}

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

		parentNode, ok := usbDeviceMap["usb:"+parentUSBAddr]
		if !ok {
			continue
		}
		parentUSBVIDPID = strings.ToLower(parentNode.VendorID + ":" + parentNode.ProductID)

		if hidVIDPID == parentUSBVIDPID {
			continue
		}

		seen[hidVIDPID] = true

		hidName := s.readHIDUeventField(devPath, "HID_NAME")
		if hidName == "" {
			continue
		}

		driver := s.readDriver(devPath)
		class := s.classifyHIDPeripheral(hidName, driver)
		if hasJoystickInput(devPath) {
			class = domain.DeviceClassGameController
		}

		if known, ok := lookupUSBDevice(hidVID, hidPID); ok {
			if known.Name != "" {
				hidName = known.Name
			}
			if known.Class != "" {
				class = known.Class
			}
		}

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

	switch driver {
	case "logitech-hidpp-device":
		return domain.DeviceClassInput
	}

	return domain.DeviceClassInput // Default: most HID peripherals are input
}

func (s *GHWScanner) classifyUSBDevice(bDeviceClass string) domain.DeviceClass {
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

// classifyUSBDeviceByInterface uses bInterfaceClass when device class is 00/ef.
func (s *GHWScanner) classifyUSBDeviceByInterface(sysfsPath string) domain.DeviceClass {
	entries, err := os.ReadDir(sysfsPath)
	if err != nil {
		return domain.DeviceClassOther
	}

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

// hasVendorDebugInterface is true when an interface is class ff (JTAG/SWD probes).
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

// hasJoystickNode is true when the kernel registered a joystick for the USB
// device, either on an interface (xpad) or on a HID device under one.
func hasJoystickNode(sysfsPath string) bool {
	for _, pattern := range []string{"*/input/input*/js*", "*/*/input/input*/js*"} {
		if matches, _ := filepath.Glob(filepath.Join(sysfsPath, pattern)); len(matches) > 0 {
			return true
		}
	}
	return false
}

func hasJoystickInput(hidDevPath string) bool {
	matches, _ := filepath.Glob(filepath.Join(hidDevPath, "input/input*/js*"))
	return len(matches) > 0
}

// hasXboxGamepadInterface detects Xbox-protocol controllers by interface
// descriptors: XInput pads (Xbox 360, Logitech F310/F710) use ff/5d,
// Xbox One GIP pads use ff/47.
func (s *GHWScanner) hasXboxGamepadInterface(sysfsPath string) bool {
	entries, err := os.ReadDir(sysfsPath)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if !strings.Contains(entry.Name(), ":") {
			continue
		}
		ifaceClass := s.readSysfsFile(filepath.Join(sysfsPath, entry.Name(), "bInterfaceClass"))
		if ifaceClass != "ff" {
			continue
		}
		subClass := s.readSysfsFile(filepath.Join(sysfsPath, entry.Name(), "bInterfaceSubClass"))
		if subClass == "5d" || subClass == "47" {
			return true
		}
	}
	return false
}

// findInterfaceDriver prefers an interface/HID driver over generic "usb".
func (s *GHWScanner) findInterfaceDriver(sysfsPath, deviceName string) string {
	entries, err := os.ReadDir(sysfsPath)
	if err != nil {
		return ""
	}

	skip := map[string]bool{"usb": true, "hub": true}
	bestDriver := ""

	for _, entry := range entries {
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
	"2581": true, // Ledger (crypto hardware wallet)
	"2c97": true, // Ledger (newer)
	"534c": true, // SatoshiLabs (Trezor)
}

var securityKeyPatterns = []string{
	"yubikey", "fido", "u2f", "security key", "nitrokey",
	"onlykey", "solokey", "trezor", "ledger",
}

func (s *GHWScanner) reclassifySecurityKey(current domain.DeviceClass, vendorID, product string) domain.DeviceClass {
	if securityKeyVendors[vendorID] {
		return domain.DeviceClassSecurity
	}

	productLower := strings.ToLower(product)
	for _, pattern := range securityKeyPatterns {
		if strings.Contains(productLower, pattern) {
			return domain.DeviceClassSecurity
		}
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
	if portableDeviceVendors[vendorID] {
		return domain.DeviceClassPortable
	}

	productLower := strings.ToLower(product)
	for _, pattern := range portableNamePatterns {
		if strings.Contains(productLower, pattern) {
			return domain.DeviceClassPortable
		}
	}

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

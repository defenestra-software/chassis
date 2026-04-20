package sysinfo

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/godbus/dbus/v5"
	"github.com/jaypipes/ghw"
	flatpak "gitlab.com/defenestra-software/go-flatbridge"

	"defenestra.io/chassis/internal/core/domain"
)

// EnrichCameraInfo attaches /dev/videoN paths and stable by-id/by-path links
// to camera devices by matching V4L sysfs entries to USB device addresses.
func (s *Scanner) EnrichCameraInfo(root *domain.DeviceNode) {
	v4lPath := flatpak.HostPath("/sys/class/video4linux")
	entries, err := os.ReadDir(v4lPath)
	if err != nil {
		return
	}

	// Collect all V4L devices grouped by USB address
	type v4lInfo struct {
		devPaths []string
	}
	byUSB := map[string]*v4lInfo{}

	for _, entry := range entries {
		resolved, err := filepath.EvalSymlinks(filepath.Join(v4lPath, entry.Name()))
		if err != nil {
			continue
		}

		parts := strings.Split(resolved, "/")
		usbPattern := regexp.MustCompile(`^\d+-[\d.]+$`)
		for i := len(parts) - 1; i >= 0; i-- {
			if usbPattern.MatchString(parts[i]) {
				usbAddr := parts[i]
				if byUSB[usbAddr] == nil {
					byUSB[usbAddr] = &v4lInfo{}
				}
				byUSB[usbAddr].devPaths = append(byUSB[usbAddr].devPaths, "/dev/"+entry.Name())
				break
			}
		}
	}

	// Collect by-id symlinks
	byIDPath := flatpak.HostPath("/dev/v4l/by-id")
	byIDEntries, _ := os.ReadDir(byIDPath)

	// Collect by-path symlinks
	byPathDir := flatpak.HostPath("/dev/v4l/by-path")
	byPathEntries, _ := os.ReadDir(byPathDir)

	for usbAddr, info := range byUSB {
		var allPaths []string

		// Add /dev/videoN paths
		allPaths = append(allPaths, info.devPaths...)

		// Find matching by-id links
		for _, e := range byIDEntries {
			target, err := os.Readlink(filepath.Join(byIDPath, e.Name()))
			if err != nil {
				continue
			}
			for _, dp := range info.devPaths {
				if strings.HasSuffix(target, filepath.Base(dp)) {
					allPaths = append(allPaths, filepath.Join("/dev/v4l/by-id", e.Name()))
				}
			}
		}

		// Find matching by-path links
		for _, e := range byPathEntries {
			target, err := os.Readlink(filepath.Join(byPathDir, e.Name()))
			if err != nil {
				continue
			}
			for _, dp := range info.devPaths {
				if strings.HasSuffix(target, filepath.Base(dp)) {
					allPaths = append(allPaths, filepath.Join("/dev/v4l/by-path", e.Name()))
				}
			}
		}

		targetID := "usb:" + usbAddr
		s.attachDevicePaths(root, targetID, allPaths)
	}
}

func (s *Scanner) attachDevicePaths(node *domain.DeviceNode, targetID string, paths []string) bool {
	if node.ID == targetID {
		node.DevicePaths = paths
		s.logger.Debug("device paths attached", "id", node.ID, "count", len(paths))
		return true
	}
	for _, child := range node.Children {
		if s.attachDevicePaths(child, targetID, paths) {
			return true
		}
	}
	return false
}

// ScanBluetoothHID finds HID devices connected via Bluetooth (bus type 0005).
// These are wireless gamepads, keyboards, mice connected via BLE.
func (s *Scanner) ScanBluetoothHID() []*domain.DeviceNode {
	var nodes []*domain.DeviceNode

	hidPath := flatpak.HostPath("/sys/bus/hid/devices")
	entries, err := os.ReadDir(hidPath)
	if err != nil {
		return nil
	}

	seen := map[string]bool{}

	for _, entry := range entries {
		name := entry.Name()
		// Bluetooth HID devices have bus type 0005
		if !strings.HasPrefix(name, "0005:") {
			continue
		}

		parts := strings.SplitN(name, ":", 3)
		if len(parts) != 3 {
			continue
		}
		vid := strings.ToLower(parts[1])
		pidParts := strings.SplitN(parts[2], ".", 2)
		if len(pidParts) != 2 {
			continue
		}
		pid := strings.ToLower(pidParts[0])
		vidpid := vid + ":" + pid

		// Skip duplicates (multiple HID collections for same device)
		if seen[vidpid] {
			continue
		}
		seen[vidpid] = true

		devPath := filepath.Join(hidPath, name)

		// Read HID name from uevent
		hidName := ""
		if data, err := os.ReadFile(filepath.Join(devPath, "uevent")); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "HID_NAME=") {
					hidName = strings.TrimPrefix(line, "HID_NAME=")
					break
				}
			}
		}
		if hidName == "" {
			continue
		}

		driver := ""
		if driverLink, err := os.Readlink(filepath.Join(devPath, "driver")); err == nil {
			driver = filepath.Base(driverLink)
		}

		// Classify - most BT HID devices are input (gamepads, keyboards, mice)
		class := domain.DeviceClassInput
		nameLower := strings.ToLower(hidName)
		if strings.Contains(nameLower, "audio") || strings.Contains(nameLower, "headset") || strings.Contains(nameLower, "speaker") {
			class = domain.DeviceClassAudio
		}

		// Try to enrich from BlueZ - get friendly name, vendor, battery, connection info
		vendor := ""
		var batteryInfo *domain.BatteryInfo
		btConnected := false

		btDevice := s.findBlueZDevice(vid, pid)
		if btDevice != nil {
			if btDevice.alias != "" && btDevice.alias != hidName {
				hidName = btDevice.alias
			}
			btConnected = btDevice.connected
			if btDevice.batteryPercent >= 0 {
				batteryInfo = &domain.BatteryInfo{
					Capacity: btDevice.batteryPercent,
				}
			}
		}

		// Look up vendor from VID
		if vendor == "" {
			switch vid {
			case "18d1":
				vendor = "Google"
			case "054c":
				vendor = "Sony"
			case "045e":
				vendor = "Microsoft"
			case "057e":
				vendor = "Nintendo"
			case "28de":
				vendor = "Valve"
			}
		}

		// Build subtitle for list view
		listSubtitle := "Bluetooth"
		if btConnected {
			listSubtitle = "Bluetooth · Connected"
		}
		if batteryInfo != nil && batteryInfo.Capacity >= 0 {
			listSubtitle = fmt.Sprintf("%s · %d%%", listSubtitle, batteryInfo.Capacity)
		}

		node := &domain.DeviceNode{
			ID:          fmt.Sprintf("bt-hid:%s:%s", vid, pid),
			Name:        hidName,
			Vendor:      vendor,
			Product:     listSubtitle,
			VendorID:    vid,
			ProductID:   pid,
			Driver:      driver,
			Path:        devPath,
			Type:        domain.DeviceTypeUnknown,
			Class:       class,
			BatteryInfo: batteryInfo,
			IOMMUGroup:  -1,
			Children:    make([]*domain.DeviceNode, 0),
		}

		nodes = append(nodes, node)
		s.logger.Debug("found Bluetooth HID device", "name", hidName, "vid", vid, "pid", pid)
	}

	return nodes
}

// EnrichSerialPorts attaches /dev/ttyACM* and /dev/ttyUSB* paths to USB devices,
// and creates a serial interface classification for devices that expose serial ports.
func (s *Scanner) EnrichSerialPorts(root *domain.DeviceNode) {
	// Scan ttyACM (CDC-ACM) and ttyUSB devices
	for _, pattern := range []string{"/sys/class/tty/ttyACM*", "/sys/class/tty/ttyUSB*"} {
		matches, err := filepath.Glob(flatpak.HostPath(pattern))
		if err != nil {
			continue
		}

		for _, ttyPath := range matches {
			ttyName := filepath.Base(ttyPath)

			// Resolve to find USB parent
			resolved, err := filepath.EvalSymlinks(filepath.Join(ttyPath, "device"))
			if err != nil {
				continue
			}

			// Walk up to find USB address
			parts := strings.Split(resolved, "/")
			usbPattern := regexp.MustCompile(`^\d+-[\d.]+$`)
			usbAddr := ""
			for i := len(parts) - 1; i >= 0; i-- {
				if usbPattern.MatchString(parts[i]) {
					usbAddr = parts[i]
					break
				}
			}
			if usbAddr == "" {
				continue
			}

			// Collect all paths for this serial port
			var devPaths []string
			devPaths = append(devPaths, "/dev/"+ttyName)

			// Check by-id
			byIDPath := flatpak.HostPath("/dev/serial/by-id")
			if idEntries, err := os.ReadDir(byIDPath); err == nil {
				for _, e := range idEntries {
					target, err := os.Readlink(filepath.Join(byIDPath, e.Name()))
					if err != nil {
						continue
					}
					if strings.HasSuffix(target, ttyName) {
						devPaths = append(devPaths, filepath.Join("/dev/serial/by-id", e.Name()))
					}
				}
			}

			// Check by-path
			byPathDir := flatpak.HostPath("/dev/serial/by-path")
			if pathEntries, err := os.ReadDir(byPathDir); err == nil {
				for _, e := range pathEntries {
					target, err := os.Readlink(filepath.Join(byPathDir, e.Name()))
					if err != nil {
						continue
					}
					if strings.HasSuffix(target, ttyName) {
						devPaths = append(devPaths, filepath.Join("/dev/serial/by-path", e.Name()))
					}
				}
			}

			// Attach to the USB device
			targetID := "usb:" + usbAddr
			s.attachSerialPaths(root, targetID, devPaths)
		}
	}
}

func (s *Scanner) attachSerialPaths(node *domain.DeviceNode, targetID string, paths []string) bool {
	if node.ID == targetID {
		// Add to device paths (merge, don't replace)
		node.DevicePaths = append(node.DevicePaths, paths...)
		s.logger.Debug("attached serial paths", "id", node.ID, "paths", len(paths))
		return true
	}
	for _, child := range node.Children {
		if s.attachSerialPaths(child, targetID, paths) {
			return true
		}
	}
	return false
}

// EnrichUSBStorage scans /sys/block for USB-backed block devices and attaches
// StorageInfo to the corresponding USB device nodes. This handles composite
// USB devices like the J-Link which expose both serial and mass storage.
func (s *Scanner) EnrichUSBStorage(root *domain.DeviceNode) {
	blockPath := flatpak.HostPath("/sys/block")
	entries, err := os.ReadDir(blockPath)
	if err != nil {
		return
	}

	usbAddrPattern := regexp.MustCompile(`\b(\d+-[\d.]+)\b`)

	for _, entry := range entries {
		// Only check sd* devices (USB mass storage appears as SCSI)
		if !strings.HasPrefix(entry.Name(), "sd") {
			continue
		}

		devPath := filepath.Join(blockPath, entry.Name())

		// Resolve the device symlink to check if it's USB-backed
		resolved, err := filepath.EvalSymlinks(filepath.Join(devPath, "device"))
		if err != nil {
			continue
		}

		// Look for a USB address in the resolved path
		matches := usbAddrPattern.FindAllString(resolved, -1)
		if len(matches) == 0 {
			continue
		}

		// Skip if path also contains PCI patterns without USB indicators
		// (SATA drives also go through sd* but have PCI, not USB parents)
		if !strings.Contains(resolved, "/usb") {
			continue
		}

		// Find the USB device address (last match before the SCSI host)
		usbAddr := ""
		for _, m := range matches {
			usbAddr = m
		}
		if usbAddr == "" {
			continue
		}

		targetID := "usb:" + usbAddr

		// Read basic storage info from sysfs
		deviceDir := filepath.Join(devPath, "device")
		model := readFile(filepath.Join(deviceDir, "model"))
		if model == "" {
			model = readFile(filepath.Join(deviceDir, "vendor"))
		}

		var sizeBytes uint64
		if sizeStr := readFile(filepath.Join(devPath, "size")); sizeStr != "" {
			if sectors, err := strconv.ParseUint(sizeStr, 10, 64); err == nil {
				sizeBytes = sectors * 512
			}
		}

		info := &domain.StorageInfo{
			Model:       model,
			BlockDevice: entry.Name(),
			Transport:   "usb",
			SizeBytes:   sizeBytes,
		}

		// Attach to the USB device node
		s.attachStorageInfo(root, targetID, info)
	}
}

func (s *Scanner) attachStorageInfo(node *domain.DeviceNode, targetID string, info *domain.StorageInfo) bool {
	if node.ID == targetID {
		node.StorageInfo = info
		// Also add /dev/sdX to device paths
		devPath := "/dev/" + info.BlockDevice
		node.DevicePaths = append(node.DevicePaths, devPath)
		s.logger.Debug("attached USB storage", "id", node.ID, "model", info.Model, "block", info.BlockDevice)
		return true
	}
	for _, child := range node.Children {
		if s.attachStorageInfo(child, targetID, info) {
			return true
		}
	}
	return false
}

// EnrichNetworkInfo attaches network interface info to PCI/USB network devices.
func (s *Scanner) EnrichNetworkInfo(root *domain.DeviceNode) {
	netInfo, err := ghw.Network()
	if err != nil {
		s.logger.Debug("could not read network info", "error", err)
		return
	}

	for _, nic := range netInfo.NICs {
		if nic.IsVirtual {
			continue
		}

		info := &domain.NetworkInfo{
			InterfaceName: nic.Name,
			MACAddress:    nic.MACAddress,
			Speed:         nic.Speed,
			Duplex:        nic.Duplex,
			IsVirtual:     nic.IsVirtual,
		}

		// Read extra info from sysfs
		netPath := flatpak.HostPath(filepath.Join("/sys/class/net", nic.Name))
		info.State = readFile(filepath.Join(netPath, "operstate"))
		info.MTU = readFile(filepath.Join(netPath, "mtu"))

		// Check if wireless
		if _, err := os.Stat(filepath.Join(netPath, "wireless")); err == nil {
			info.IsWireless = true
			s.enrichWirelessNM(nic.Name, info)
		}

		// Try to match to a PCI device
		if nic.PCIAddress != nil && *nic.PCIAddress != "" {
			targetID := "pci:" + *nic.PCIAddress
			s.attachNetworkInfo(root, targetID, info)
			continue
		}

		// Try to match via sysfs device link for USB devices
		deviceLink := filepath.Join(netPath, "device")
		if resolved, err := filepath.EvalSymlinks(deviceLink); err == nil {
			// Walk up to find USB address
			parts := strings.Split(resolved, "/")
			usbPattern := regexp.MustCompile(`^\d+-[\d.]+$`)
			for i := len(parts) - 1; i >= 0; i-- {
				if usbPattern.MatchString(parts[i]) {
					targetID := "usb:" + parts[i]
					s.attachNetworkInfo(root, targetID, info)
					break
				}
			}
		}
	}

	// Also check for Bluetooth
	btPath := flatpak.HostPath("/sys/class/bluetooth")
	btEntries, err := os.ReadDir(btPath)
	if err == nil {
		for _, entry := range btEntries {
			hciName := entry.Name()
			// Skip non-hci entries (like "hci0:16" rfkill sub-entries)
			if strings.Contains(hciName, ":") {
				continue
			}

			hciPath := filepath.Join(btPath, hciName)
			btAddr := readFile(filepath.Join(hciPath, "address"))

			// Get the actual BT driver (btusb, btrtl, etc.)
			btDriver := ""
			if driverLink, err := os.Readlink(filepath.Join(hciPath, "device", "driver")); err == nil {
				btDriver = filepath.Base(driverLink)
			}

			info := &domain.NetworkInfo{
				InterfaceName: hciName,
				MACAddress:    btAddr,
				IsBluetooth:   true,
			}

			// Query BlueZ D-Bus directly with the hci name
			s.enrichBlueZDirect(hciName, info)

			// Find parent USB device and update its driver
			if resolved, err := filepath.EvalSymlinks(filepath.Join(hciPath, "device")); err == nil {
				parts := strings.Split(resolved, "/")
				usbPattern := regexp.MustCompile(`^\d+-[\d.]+$`)
				for i := len(parts) - 1; i >= 0; i-- {
					if usbPattern.MatchString(parts[i]) {
						targetID := "usb:" + parts[i]
						if s.attachNetworkInfo(root, targetID, info) && btDriver != "" {
							// Override the generic "usb" driver with the actual BT driver
							s.setDeviceDriver(root, targetID, btDriver)
						}
						break
					}
				}
			}
		}
	}
}

type blueZDeviceInfo struct {
	alias          string
	connected      bool
	adapter        string
	batteryPercent int
}

// findBlueZDevice searches all BlueZ adapters for a connected device matching the VID:PID.
func (s *Scanner) findBlueZDevice(vid, pid string) *blueZDeviceInfo {
	conn, err := dbus.SystemBus()
	if err != nil {
		return nil
	}

	// Use GetManagedObjects to find all devices
	obj := conn.Object("org.bluez", "/")
	var result map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	err = obj.Call("org.freedesktop.DBus.ObjectManager.GetManagedObjects", 0).Store(&result)
	if err != nil {
		return nil
	}

	targetModalias := fmt.Sprintf("usb:v%sp%s", strings.ToUpper(vid), strings.ToUpper(pid))

	for path, ifaces := range result {
		dev, ok := ifaces["org.bluez.Device1"]
		if !ok {
			continue
		}

		// Check Modalias
		if modalias, ok := dev["Modalias"]; ok {
			if m, ok := modalias.Value().(string); ok {
				if strings.HasPrefix(strings.ToUpper(m), strings.ToUpper(targetModalias)) {
					info := &blueZDeviceInfo{batteryPercent: -1}

					if v, ok := dev["Alias"]; ok {
						if a, ok := v.Value().(string); ok {
							info.alias = a
						}
					}
					if v, ok := dev["Connected"]; ok {
						if c, ok := v.Value().(bool); ok {
							info.connected = c
						}
					}
					if v, ok := dev["Adapter"]; ok {
						if a, ok := v.Value().(dbus.ObjectPath); ok {
							info.adapter = string(a)
						}
					}

					// Check for battery
					if bat, ok := ifaces["org.bluez.Battery1"]; ok {
						if pct, ok := bat["Percentage"]; ok {
							if b, ok := pct.Value().(byte); ok {
								info.batteryPercent = int(b)
							}
						}
					}

					_ = path
					return info
				}
			}
		}
	}

	return nil
}

// enrichBlueZDirect queries BlueZ D-Bus for a specific adapter by its hci name.
// The sysfs name (hci0, hci2) matches the BlueZ object path (/org/bluez/hci0).
func (s *Scanner) enrichBlueZDirect(hciName string, info *domain.NetworkInfo) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return
	}

	objPath := dbus.ObjectPath("/org/bluez/" + hciName)
	obj := conn.Object("org.bluez", objPath)

	// Get MAC address
	if v, err := obj.GetProperty("org.bluez.Adapter1.Address"); err == nil {
		if addr, ok := v.Value().(string); ok && addr != "" {
			info.MACAddress = addr
		}
	}

	// Get alias (user-set friendly name)
	if v, err := obj.GetProperty("org.bluez.Adapter1.Alias"); err == nil {
		if alias, ok := v.Value().(string); ok && alias != "" {
			info.InterfaceName = fmt.Sprintf("%s (%s)", hciName, alias)
		}
	}

	// Get powered state
	if v, err := obj.GetProperty("org.bluez.Adapter1.Powered"); err == nil {
		if powered, ok := v.Value().(bool); ok {
			if powered {
				info.State = "up"
			} else {
				info.State = "down"
			}
		}
	}
}

// enrichWirelessNM queries NetworkManager D-Bus for wireless details of the given interface.
func (s *Scanner) enrichWirelessNM(ifaceName string, info *domain.NetworkInfo) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return
	}

	nm := conn.Object("org.freedesktop.NetworkManager", "/org/freedesktop/NetworkManager")
	var devPath dbus.ObjectPath
	err = nm.Call("org.freedesktop.NetworkManager.GetDeviceByIpIface", 0, ifaceName).Store(&devPath)
	if err != nil {
		s.logger.Debug("NM: could not find device", "iface", ifaceName, "error", err)
		return
	}

	dev := conn.Object("org.freedesktop.NetworkManager", devPath)

	apVar, err := dev.GetProperty("org.freedesktop.NetworkManager.Device.Wireless.ActiveAccessPoint")
	if err != nil {
		return
	}
	apPath, ok := apVar.Value().(dbus.ObjectPath)
	if !ok || !apPath.IsValid() || apPath == "/" {
		return
	}

	ap := conn.Object("org.freedesktop.NetworkManager", apPath)

	if v, err := ap.GetProperty("org.freedesktop.NetworkManager.AccessPoint.Ssid"); err == nil {
		if ssidBytes, ok := v.Value().([]byte); ok {
			info.SSID = string(ssidBytes)
		}
	}

	if v, err := ap.GetProperty("org.freedesktop.NetworkManager.AccessPoint.Strength"); err == nil {
		if strength, ok := v.Value().(byte); ok {
			info.Signal = int(strength)
		}
	}

	if v, err := ap.GetProperty("org.freedesktop.NetworkManager.AccessPoint.Frequency"); err == nil {
		if freq, ok := v.Value().(uint32); ok {
			info.Frequency = freq
			info.Channel = domain.FrequencyToChannel(freq)
		}
	}

	if v, err := ap.GetProperty("org.freedesktop.NetworkManager.AccessPoint.MaxBitrate"); err == nil {
		if rate, ok := v.Value().(uint32); ok {
			info.BitRate = int(rate)
		}
	}

	info.SecurityType = s.nmSecurityType(ap)
}

func (s *Scanner) nmSecurityType(ap dbus.BusObject) string {
	const (
		nmAPSecPairWEP40           = 0x1
		nmAPSecPairWEP104          = 0x2
		nmAPSecPairTKIP            = 0x4
		nmAPSecPairCCMP            = 0x8
		nmAPSecGroupWEP40          = 0x10
		nmAPSecGroupWEP104         = 0x20
		nmAPSecGroupTKIP           = 0x40
		nmAPSecGroupCCMP           = 0x80
		nmAPSecKeyMgmtPSK          = 0x100
		nmAPSecKeyMgmt8021X        = 0x200
		nmAPSecKeyMgmtSAE          = 0x400
		nmAPSecKeyMgmtOWE          = 0x800
		nmAPSecKeyMgmtOWETM        = 0x1000
		nmAPSecKeyMgmtEAPSuiteB192 = 0x2000
	)

	var rsnFlags, wpaFlags uint32

	if v, err := ap.GetProperty("org.freedesktop.NetworkManager.AccessPoint.RsnFlags"); err == nil {
		if f, ok := v.Value().(uint32); ok {
			rsnFlags = f
		}
	}
	if v, err := ap.GetProperty("org.freedesktop.NetworkManager.AccessPoint.WpaFlags"); err == nil {
		if f, ok := v.Value().(uint32); ok {
			wpaFlags = f
		}
	}

	switch {
	case rsnFlags&nmAPSecKeyMgmtSAE != 0:
		return "WPA3"
	case rsnFlags&nmAPSecKeyMgmtEAPSuiteB192 != 0:
		return "WPA3 Enterprise"
	case rsnFlags&nmAPSecKeyMgmt8021X != 0:
		return "WPA2 Enterprise"
	case rsnFlags&nmAPSecKeyMgmtPSK != 0:
		return "WPA2"
	case wpaFlags&nmAPSecKeyMgmt8021X != 0:
		return "WPA Enterprise"
	case wpaFlags&nmAPSecKeyMgmtPSK != 0:
		return "WPA"
	case rsnFlags&(nmAPSecPairWEP40|nmAPSecPairWEP104) != 0,
		wpaFlags&(nmAPSecPairWEP40|nmAPSecPairWEP104) != 0:
		return "WEP"
	case rsnFlags&nmAPSecKeyMgmtOWE != 0:
		return "OWE"
	default:
		if rsnFlags == 0 && wpaFlags == 0 {
			return "Open"
		}
		return ""
	}
}

func (s *Scanner) setDeviceDriver(node *domain.DeviceNode, targetID, driver string) {
	if node.ID == targetID {
		node.Driver = driver
		return
	}
	for _, child := range node.Children {
		s.setDeviceDriver(child, targetID, driver)
	}
}

func (s *Scanner) attachNetworkInfo(node *domain.DeviceNode, targetID string, info *domain.NetworkInfo) bool {
	if node.ID == targetID {
		node.NetworkInfo = info
		// Reclassify to Networking if it was Other/Unknown
		if node.Class == domain.DeviceClassOther {
			if info.IsWireless || info.IsBluetooth {
				node.Class = domain.DeviceClassWireless
			} else {
				node.Class = domain.DeviceClassNetwork
			}
		}
		return true
	}
	for _, child := range node.Children {
		if s.attachNetworkInfo(child, targetID, info) {
			return true
		}
	}
	return false
}

// EnrichPeripheralBatteries finds power_supply entries with scope=Device
// and attaches their battery info to matching USB device nodes in the tree.
func (s *Scanner) EnrichPeripheralBatteries(root *domain.DeviceNode) {
	s.logger.Debug("enriching peripheral batteries")
	psPath := flatpak.HostPath("/sys/class/power_supply")
	entries, err := os.ReadDir(psPath)
	if err != nil {
		return
	}

	for _, entry := range entries {
		devPath := filepath.Join(psPath, entry.Name())
		scope := readFile(filepath.Join(devPath, "scope"))
		if !strings.EqualFold(scope, "Device") {
			continue
		}

		// Build battery info
		battery := &domain.BatteryInfo{
			Status:        readFile(filepath.Join(devPath, "status")),
			CapacityLevel: readFile(filepath.Join(devPath, "capacity_level")),
			Model:         readFile(filepath.Join(devPath, "model_name")),
			Manufacturer:  readFile(filepath.Join(devPath, "manufacturer")),
			Capacity:      -1,
		}

		// Try numeric capacity first
		if capStr := readFile(filepath.Join(devPath, "capacity")); capStr != "" {
			if cap, err := strconv.Atoi(capStr); err == nil {
				battery.Capacity = cap
			}
		}

		// Find the HID device that owns this battery by resolving the
		// device symlink and extracting the HID VID:PID.
		hidID := s.findHIDDeviceID(devPath)
		if hidID != "" {
			if s.attachBattery(root, hidID, battery) {
				continue
			}
		}

		// Fall back to USB address matching
		usbAddr := s.findParentUSBAddress(devPath)
		if usbAddr != "" {
			targetID := "usb:" + usbAddr
			if !s.attachBattery(root, targetID, battery) {
				s.attachBatteryByPath(root, usbAddr, battery)
			}
		}
	}
}

// findHIDDeviceID resolves the power_supply device symlink and extracts the
// HID device ID (e.g., "hid:046d:405e") from the sysfs path.
func (s *Scanner) findHIDDeviceID(psDevPath string) string {
	deviceLink := filepath.Join(psDevPath, "device")
	resolved, err := filepath.EvalSymlinks(deviceLink)
	if err != nil {
		return ""
	}

	// Look for a HID device path segment like "0003:046D:405E.000C"
	parts := strings.Split(resolved, "/")
	hidPattern := regexp.MustCompile(`^[0-9A-Fa-f]{4}:([0-9A-Fa-f]{4}):([0-9A-Fa-f]{4})\.[0-9A-Fa-f]+$`)

	// Walk backwards - the last HID segment is the actual peripheral
	for i := len(parts) - 1; i >= 0; i-- {
		match := hidPattern.FindStringSubmatch(parts[i])
		if match != nil {
			vid := strings.ToLower(match[1])
			pid := strings.ToLower(match[2])
			return "hid:" + vid + ":" + pid
		}
	}
	return ""
}

// findParentUSBAddress resolves the power_supply device symlink and extracts
// the USB device address (e.g., "3-2") from the sysfs path.
func (s *Scanner) findParentUSBAddress(psDevPath string) string {
	deviceLink := filepath.Join(psDevPath, "device")

	// EvalSymlinks resolves the full chain to an absolute path
	resolved, err := filepath.EvalSymlinks(deviceLink)
	if err != nil {
		s.logger.Debug("battery: could not resolve device link", "path", deviceLink, "error", err)
		return ""
	}

	// Walk up the resolved path looking for a USB device directory
	// USB device paths contain segments like "3-2" (bus-port)
	parts := strings.Split(resolved, "/")
	usbDevPattern := regexp.MustCompile(`^\d+-[\d.]+$`)

	for i := len(parts) - 1; i >= 0; i-- {
		if usbDevPattern.MatchString(parts[i]) {
			s.logger.Debug("battery: found USB address", "address", parts[i], "from", resolved)
			return parts[i]
		}
	}

	s.logger.Debug("battery: no USB address found in path", "resolved", resolved)
	return ""
}

// attachBattery recursively finds a device node by ID and attaches battery info.
func (s *Scanner) attachBattery(node *domain.DeviceNode, targetID string, battery *domain.BatteryInfo) bool {
	if node.ID == targetID {
		node.BatteryInfo = battery
		return true
	}
	for _, child := range node.Children {
		if s.attachBattery(child, targetID, battery) {
			return true
		}
	}
	return false
}

// attachBatteryByPath attaches battery info to a USB device whose sysfs path
// contains the given USB address. This handles cases like Logitech HID++
// where the battery belongs to a sub-device behind a receiver.
func (s *Scanner) attachBatteryByPath(node *domain.DeviceNode, usbAddr string, battery *domain.BatteryInfo) bool {
	// Check if this node's path contains the USB address
	if node.Path != "" && node.Type == domain.DeviceTypeUSB {
		if strings.Contains(node.Path, "/"+usbAddr) {
			node.BatteryInfo = battery
			s.logger.Debug("attached battery by path match",
				"device", node.ID,
				"name", node.Name,
				"usb_addr", usbAddr)
			return true
		}
	}
	for _, child := range node.Children {
		if s.attachBatteryByPath(child, usbAddr, battery) {
			return true
		}
	}
	return false
}

// EnrichUPSBattery queries UPower over D-Bus for UPS devices and attaches
// battery info to matching USB device nodes.
func (s *Scanner) EnrichUPSBattery(root *domain.DeviceNode) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return
	}

	obj := conn.Object("org.freedesktop.UPower", "/org/freedesktop/UPower")
	var devicePaths []dbus.ObjectPath
	err = obj.Call("org.freedesktop.UPower.EnumerateDevices", 0).Store(&devicePaths)
	if err != nil {
		return
	}

	for _, path := range devicePaths {
		devObj := conn.Object("org.freedesktop.UPower", path)

		// Type 3 = UPS in UPower
		devType, err := devObj.GetProperty("org.freedesktop.UPower.Device.Type")
		if err != nil {
			continue
		}
		if t, ok := devType.Value().(uint32); !ok || t != 3 {
			continue
		}

		nativePath, err := devObj.GetProperty("org.freedesktop.UPower.Device.NativePath")
		if err != nil {
			continue
		}
		native, _ := nativePath.Value().(string)

		percentage, _ := devObj.GetProperty("org.freedesktop.UPower.Device.Percentage")
		pct := -1
		if p, ok := percentage.Value().(float64); ok {
			pct = int(p)
		}

		// UPower state: 1=charging, 2=discharging, 3=empty, 4=fully-charged, 5=pending-charge, 6=pending-discharge
		stateVal, _ := devObj.GetProperty("org.freedesktop.UPower.Device.State")
		status := ""
		if st, ok := stateVal.Value().(uint32); ok {
			switch st {
			case 1:
				status = "Charging"
			case 2:
				status = "Discharging"
			case 3:
				status = "Empty"
			case 4:
				status = "Fully charged"
			case 5:
				status = "Pending charge"
			case 6:
				status = "Pending discharge"
			}
		}

		battery := &domain.BatteryInfo{
			Capacity: pct,
			Status:   status,
		}

		// Extract USB device address from native path by walking backwards
		// through path segments (same approach as findParentUSBAddress)
		usbDevPattern := regexp.MustCompile(`^\d+-[\d.]+$`)
		parts := strings.Split(native, "/")
		for i := len(parts) - 1; i >= 0; i-- {
			if usbDevPattern.MatchString(parts[i]) {
				targetID := "usb:" + parts[i]
				if s.attachBattery(root, targetID, battery) {
					s.logger.Debug("attached UPS battery from UPower", "device", targetID, "percentage", pct)
				}
				break
			}
		}
	}
}

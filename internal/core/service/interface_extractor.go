package service

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	flatpak "gitlab.com/defenestra-software/go-flatbridge"

	"defenestra.io/chassis/internal/core/domain"
)

func ExtractInterfaces(root *domain.DeviceNode) {
	if root == nil {
		return
	}

	extractInterfacesRecursive(root)

	attachCPUSensors(root)
	attachGPUSensors(root)
	attachPCISensors(root) // NVMe, Ethernet, etc.
	attachMotherboardSensors(root)

	deduplicateSensors(root)
}

func extractInterfacesRecursive(root *domain.DeviceNode) {
	extractNodeInterfaces(root)
	for _, child := range root.Children {
		extractInterfacesRecursive(child)
	}
}

// attachCPUSensors moves k10temp/coretemp standalone nodes onto the processor.
func attachCPUSensors(root *domain.DeviceNode) {
	var processor *domain.DeviceNode
	for _, child := range root.Children {
		if child.ProcessorInfo != nil {
			processor = child
			break
		}
	}
	if processor == nil {
		return
	}

	cpuSensorNames := map[string]bool{
		"CPU Temperature": true,
		"cpu_thermal":     true,
	}

	for _, child := range root.Children {
		if child.Class != domain.DeviceClassSensor {
			continue
		}
		if !cpuSensorNames[child.Name] {
			continue
		}
		if len(child.SensorReadings) == 0 {
			continue
		}

		var subtitleParts []string
		for _, s := range child.SensorReadings {
			if s.Type == "temperature" {
				subtitleParts = append(subtitleParts, fmt.Sprintf("%s: %s", s.Label, s.FormattedValue()))
				if len(subtitleParts) >= 2 {
					break
				}
			}
		}

		processor.Interfaces = append(processor.Interfaces, &domain.DeviceInterface{
			Type:         domain.InterfaceSensor,
			Category:     domain.DeviceClassSensor,
			SectionTitle: "Sensors",
			ScrollTag:    "sensors",
			Subtitle:     strings.Join(subtitleParts, " · "),
			SensorData:   child.SensorReadings,
		})

		break
	}
}

// attachGPUSensors moves hwmon GPU temp nodes onto matching PCI GPUs.
// Sensor scanner stores the GPU PCI address in Driver.
func attachGPUSensors(root *domain.DeviceNode) {
	gpuNodes := map[string]*domain.DeviceNode{} // PCI address → node
	collectGPUNodes(root, gpuNodes)

	if len(gpuNodes) == 0 {
		return
	}

	for _, child := range root.Children {
		if child.Class != domain.DeviceClassSensor {
			continue
		}
		if len(child.SensorReadings) == 0 {
			continue
		}

		pciAddr := child.Driver
		if pciAddr == "" {
			continue
		}

		gpu, ok := gpuNodes[pciAddr]
		if !ok {
			continue
		}

		hasSensor := false
		for _, iface := range gpu.Interfaces {
			if iface.Type == domain.InterfaceSensor {
				hasSensor = true
				break
			}
		}
		if hasSensor {
			continue
		}

		var subtitleParts []string
		for _, s := range child.SensorReadings {
			if s.Type == "temperature" {
				subtitleParts = append(subtitleParts, fmt.Sprintf("%s: %s", s.Label, s.FormattedValue()))
				if len(subtitleParts) >= 2 {
					break
				}
			}
		}

		gpu.Interfaces = append(gpu.Interfaces, &domain.DeviceInterface{
			Type:         domain.InterfaceSensor,
			Category:     domain.DeviceClassSensor,
			SectionTitle: "Sensors",
			ScrollTag:    "sensors",
			Subtitle:     strings.Join(subtitleParts, " · "),
			SensorData:   child.SensorReadings,
		})
	}
}

func collectGPUNodes(node *domain.DeviceNode, gpus map[string]*domain.DeviceNode) {
	if node.GPUInfo != nil && node.Type == domain.DeviceTypePCI {
		pciAddr := strings.TrimPrefix(node.ID, "pci:")
		gpus[pciAddr] = node
	}
	for _, child := range node.Children {
		collectGPUNodes(child, gpus)
	}
}

var pciAddrRegex = regexp.MustCompile(`\b[0-9a-fA-F]{4}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}\.[0-9a-fA-F]\b`)

// attachPCISensors attaches NVMe/Ethernet/WiFi hwmon nodes via Driver PCI addr
// or by resolving the hwmon device symlink.
func attachPCISensors(root *domain.DeviceNode) {
	pciNodes := map[string]*domain.DeviceNode{}
	collectPCINodes(root, pciNodes)

	for _, child := range root.Children {
		if child.Class != domain.DeviceClassSensor || len(child.SensorReadings) == 0 {
			continue
		}

		target := pciNodes[child.Driver]

		if target == nil && child.Path != "" {
			if pciAddr := resolveSensorPCIAddr(child.Path); pciAddr != "" {
				target = pciNodes[pciAddr]
			}
		}

		if target == nil {
			continue
		}

		hasSensor := false
		for _, iface := range target.Interfaces {
			if iface.Type == domain.InterfaceSensor {
				hasSensor = true
				break
			}
		}
		if hasSensor {
			continue
		}

		var subtitleParts []string
		for _, s := range child.SensorReadings {
			if s.Type == "temperature" {
				subtitleParts = append(subtitleParts, fmt.Sprintf("%s: %s", s.Label, s.FormattedValue()))
				if len(subtitleParts) >= 2 {
					break
				}
			}
		}

		target.Interfaces = append(target.Interfaces, &domain.DeviceInterface{
			Type:         domain.InterfaceSensor,
			Category:     domain.DeviceClassSensor,
			SectionTitle: "Sensors",
			ScrollTag:    "sensors",
			Subtitle:     strings.Join(subtitleParts, " · "),
			SensorData:   child.SensorReadings,
		})
	}
}

func resolveSensorPCIAddr(hwmonPath string) string {
	deviceLink := filepath.Join(hwmonPath, "device")
	resolved, err := filepath.EvalSymlinks(flatpak.HostPath(deviceLink))
	if err != nil {
		// Retry without flatbridge host path translation
		resolved, err = filepath.EvalSymlinks(deviceLink)
		if err != nil {
			return ""
		}
	}

	matches := pciAddrRegex.FindAllString(resolved, -1)
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1]
}

func collectPCINodes(node *domain.DeviceNode, pci map[string]*domain.DeviceNode) {
	if node.Type == domain.DeviceTypePCI {
		pciAddr := strings.TrimPrefix(node.ID, "pci:")
		pci[pciAddr] = node
	}
	for _, child := range node.Children {
		collectPCINodes(child, pci)
	}
}

// attachMotherboardSensors moves board WMI/ACPI/EC sensor nodes onto the motherboard.
func attachMotherboardSensors(root *domain.DeviceNode) {
	var motherboard *domain.DeviceNode
	for _, child := range root.Children {
		if child.MotherboardInfo != nil {
			motherboard = child
			break
		}
	}
	if motherboard == nil {
		return
	}

	mbSensorNames := map[string]bool{
		"Gigabyte Sensors":    true,
		"ACPI Thermal Zone":   true,
		"Motherboard Sensors": true,
		"ASUS Sensors":        true,
		"ASUS EC Sensors":     true,
	}

	for _, child := range root.Children {
		if child.Class != domain.DeviceClassSensor || len(child.SensorReadings) == 0 {
			continue
		}
		if !mbSensorNames[child.Name] {
			continue
		}

		var subtitleParts []string
		for _, s := range child.SensorReadings {
			if s.Type == "temperature" {
				subtitleParts = append(subtitleParts, fmt.Sprintf("%s: %s", s.Label, s.FormattedValue()))
				if len(subtitleParts) >= 2 {
					break
				}
			}
		}

		// Unique scroll tag so multiple board sensor groups all render
		scrollTag := "sensors-" + strings.ToLower(strings.ReplaceAll(child.Name, " ", "-"))

		motherboard.Interfaces = append(motherboard.Interfaces, &domain.DeviceInterface{
			Type:         domain.InterfaceSensor,
			Category:     domain.DeviceClassSensor,
			SectionTitle: child.Name,
			ScrollTag:    scrollTag,
			Name:         child.Name, // sidebar shows source name, not motherboard
			Subtitle:     strings.Join(subtitleParts, " · "),
			SensorData:   child.SensorReadings,
		})
	}
}

// deduplicateSensors drops standalone sensor nodes already attached as interfaces.
func deduplicateSensors(root *domain.DeviceNode) {
	claimedPCIAddrs := map[string]bool{}
	collectClaimedPCIAddrs(root, claimedPCIAddrs)

	claimedNames := map[string]bool{}
	collectClaimedSensorNames(root, claimedNames)

	var filtered []*domain.DeviceNode
	for _, child := range root.Children {
		if child.Class != domain.DeviceClassSensor {
			filtered = append(filtered, child)
			continue
		}

		if claimedNames[child.Name] {
			continue
		}

		if child.Driver != "" && claimedPCIAddrs[child.Driver] {
			continue
		}
		if child.Path != "" {
			if pciAddr := resolveSensorPCIAddr(child.Path); pciAddr != "" && claimedPCIAddrs[pciAddr] {
				continue
			}
		}

		filtered = append(filtered, child)
	}
	root.Children = filtered
}

func collectClaimedPCIAddrs(node *domain.DeviceNode, claimed map[string]bool) {
	if node.Type == domain.DeviceTypePCI {
		for _, iface := range node.Interfaces {
			if iface.Type == domain.InterfaceSensor {
				pciAddr := strings.TrimPrefix(node.ID, "pci:")
				claimed[pciAddr] = true
				break
			}
		}
	}
	for _, child := range node.Children {
		collectClaimedPCIAddrs(child, claimed)
	}
}

func collectClaimedSensorNames(root *domain.DeviceNode, claimed map[string]bool) {
	cpuNames := []string{"CPU Temperature", "cpu_thermal"}
	for _, child := range root.Children {
		if child.ProcessorInfo != nil {
			for _, iface := range child.Interfaces {
				if iface.Type == domain.InterfaceSensor {
					for _, n := range cpuNames {
						claimed[n] = true
					}
					break
				}
			}
		}
	}

	mbSensorNames := []string{
		"Gigabyte Sensors", "ACPI Thermal Zone", "Motherboard Sensors",
		"ASUS Sensors", "ASUS EC Sensors",
	}
	for _, child := range root.Children {
		if child.MotherboardInfo != nil {
			for _, iface := range child.Interfaces {
				if iface.Type == domain.InterfaceSensor {
					for _, n := range mbSensorNames {
						claimed[n] = true
					}
					break
				}
			}
		}
	}
}

func extractNodeInterfaces(node *domain.DeviceNode) {
	if node.ID == "root" || node.ID == "pci" || node.ID == "usb" {
		return
	}

	var ifaces []*domain.DeviceInterface

	if node.GPUInfo != nil {
		ifaces = append(ifaces, extractGPUInterfaces(node)...)
	}

	if node.StorageInfo != nil {
		ifaces = append(ifaces, extractStorageInterfaces(node)...)
	}

	if node.NetworkInfo != nil {
		ifaces = append(ifaces, extractNetworkInterface(node))
	}

	if node.ProcessorInfo != nil {
		ifaces = append(ifaces, extractProcessorInterface(node))
	}

	if node.DisplayInfo != nil {
		ifaces = append(ifaces, extractDisplayInterface(node))
	}

	if node.MotherboardInfo != nil {
		ifaces = append(ifaces, extractMotherboardInterface(node))
	}

	if node.MemoryModule != nil {
		ifaces = append(ifaces, extractMemoryInterface(node))
	}

	if len(node.SensorReadings) > 0 && node.GPUInfo == nil && node.StorageInfo == nil {
		ifaces = append(ifaces, extractSensorInterface(node))
	}

	if node.Class == domain.DeviceClassCamera {
		ifaces = append(ifaces, extractCameraInterface(node))
	}

	if hasSerialPaths(node) {
		ifaces = append(ifaces, extractSerialInterface(node))
	}

	if node.HasDebugPort {
		ifaces = append(ifaces, extractDebugInterface(node))
	}

	if len(ifaces) == 0 {
		ifaces = append(ifaces, extractFallbackInterface(node))
	}

	node.Interfaces = ifaces
}

func extractGPUInterfaces(node *domain.DeviceNode) []*domain.DeviceInterface {
	var ifaces []*domain.DeviceInterface
	gpu := node.GPUInfo

	subtitle := node.Driver
	if vram := gpu.VRAMTotalString(); vram != "" {
		subtitle = fmt.Sprintf("%s · %s VRAM", subtitle, vram)
	}
	ifaces = append(ifaces, &domain.DeviceInterface{
		Type:         domain.InterfaceGraphics,
		Category:     domain.DeviceClassGPU,
		SectionTitle: "Graphics",
		ScrollTag:    "graphics",
		Subtitle:     subtitle,
		GPUInfo:      gpu,
	})

	if len(node.SensorReadings) > 0 {
		var subtitleParts []string
		for _, s := range node.SensorReadings {
			if s.Type == "temperature" {
				subtitleParts = append(subtitleParts, fmt.Sprintf("%s: %s", s.Label, s.FormattedValue()))
				if len(subtitleParts) >= 2 {
					break
				}
			}
		}
		ifaces = append(ifaces, &domain.DeviceInterface{
			Type:         domain.InterfaceSensor,
			Category:     domain.DeviceClassSensor,
			SectionTitle: "Sensors",
			ScrollTag:    "sensors",
			Subtitle:     strings.Join(subtitleParts, " · "),
			SensorData:   node.SensorReadings,
		})
	}

	for _, monitor := range gpu.Monitors {
		if monitor.Status != domain.ConnectionConnected {
			continue
		}
		subtitle := monitor.FormattedName()
		if monitor.CurrentMode != nil {
			subtitle = fmt.Sprintf("%s · %dx%d", subtitle,
				monitor.CurrentMode.Width, monitor.CurrentMode.Height)
		}
		ifaces = append(ifaces, &domain.DeviceInterface{
			Type:         domain.InterfaceDisplay,
			Category:     domain.DeviceClassDisplay,
			SectionTitle: fmt.Sprintf("Display Output: %s", monitor.FormattedName()),
			ScrollTag:    fmt.Sprintf("display-%s", monitor.FormattedName()),
			Subtitle:     subtitle,
		})
	}

	return ifaces
}

func extractStorageInterfaces(node *domain.DeviceNode) []*domain.DeviceInterface {
	var ifaces []*domain.DeviceInterface
	stor := node.StorageInfo

	subtitle := node.Driver
	if stor.SizeBytes > 0 {
		subtitle = fmt.Sprintf("%s · %s", subtitle, stor.SizeString())
	}
	// Keep /dev path on the interface so the paths section does not re-list it
	var storagePaths []string
	if stor.BlockDevice != "" {
		storagePaths = append(storagePaths, "/dev/"+stor.BlockDevice)
	}

	ifaces = append(ifaces, &domain.DeviceInterface{
		Type:         domain.InterfaceStorage,
		Category:     domain.DeviceClassStorage,
		SectionTitle: "Storage",
		ScrollTag:    "storage",
		Subtitle:     subtitle,
		StorageInfo:  stor,
		DevicePaths:  storagePaths,
	})

	// Temps come from attachPCISensors hwmon paths, not a Storage sensor iface.

	return ifaces
}

func extractNetworkInterface(node *domain.DeviceNode) *domain.DeviceInterface {
	net := node.NetworkInfo

	sectionTitle := "Network"
	ifaceType := domain.InterfaceNetwork
	if net.IsBluetooth {
		sectionTitle = "Bluetooth"
	} else if net.IsWireless {
		sectionTitle = "Wireless"
	}

	parts := []string{}
	if node.Driver != "" {
		parts = append(parts, node.Driver)
	}
	if net.State == "up" {
		parts = append(parts, "Connected")
	} else if net.State == "down" {
		parts = append(parts, "Disconnected")
	}
	if net.Speed != "" && net.Speed != "-1" && net.Speed != "unknown" {
		parts = append(parts, net.Speed)
	}
	subtitle := strings.Join(parts, " · ")

	return &domain.DeviceInterface{
		Type:         ifaceType,
		Category:     domain.DeviceClassNetwork, // Wireless merges into Network
		SectionTitle: sectionTitle,
		ScrollTag:    "network",
		Subtitle:     subtitle,
		NetworkInfo:  net,
	}
}

func extractProcessorInterface(node *domain.DeviceNode) *domain.DeviceInterface {
	info := node.ProcessorInfo
	subtitle := info.CoreString()
	if info.MaxFreqMHz > 0 {
		if subtitle != "" {
			subtitle += " · "
		}
		subtitle += info.FreqString()
	}

	return &domain.DeviceInterface{
		Type:          domain.InterfaceProcessor,
		Category:      domain.DeviceClassProcessor,
		SectionTitle:  "Processor",
		ScrollTag:     "processor",
		Subtitle:      subtitle,
		ProcessorInfo: info,
	}
}

func extractDisplayInterface(node *domain.DeviceNode) *domain.DeviceInterface {
	info := node.DisplayInfo
	subtitle := fmt.Sprintf("%s-%s", info.ConnectorType, info.ConnectorIndex)
	if info.CurrentMode != "" {
		subtitle += " · " + info.CurrentMode
	}
	if info.MaxRefresh > 0 {
		subtitle += fmt.Sprintf(" · %d Hz", info.MaxRefresh)
	}

	return &domain.DeviceInterface{
		Type:         domain.InterfaceDisplay,
		Category:     domain.DeviceClassDisplay,
		SectionTitle: "Display",
		ScrollTag:    "display",
		Subtitle:     subtitle,
		DisplayInfo:  info,
	}
}

func extractMotherboardInterface(node *domain.DeviceNode) *domain.DeviceInterface {
	info := node.MotherboardInfo
	subtitle := "Motherboard"
	if info.TotalMemoryBytes > 0 {
		subtitle = fmt.Sprintf("Motherboard · %s RAM", info.TotalMemoryString())
	}

	return &domain.DeviceInterface{
		Type:            domain.InterfaceSystem,
		Category:        domain.DeviceClassBridge,
		SectionTitle:    "Motherboard",
		ScrollTag:       "motherboard",
		Subtitle:        subtitle,
		MotherboardInfo: info,
	}
}

func extractMemoryInterface(node *domain.DeviceNode) *domain.DeviceInterface {
	mod := node.MemoryModule
	subtitle := mod.Label
	if mod.Speed != "" {
		subtitle += " · " + mod.Speed
	}

	return &domain.DeviceInterface{
		Type:         domain.InterfaceMemory,
		Category:     domain.DeviceClassMemory,
		SectionTitle: "Memory Module",
		ScrollTag:    "memory",
		Subtitle:     subtitle,
		MemoryModule: mod,
	}
}

func extractSensorInterface(node *domain.DeviceNode) *domain.DeviceInterface {
	var subtitleParts []string
	for _, s := range node.SensorReadings {
		if s.Type == "temperature" {
			subtitleParts = append(subtitleParts, fmt.Sprintf("%s: %s", s.Label, s.FormattedValue()))
			if len(subtitleParts) >= 2 {
				break
			}
		}
	}
	if len(subtitleParts) == 0 {
		for _, s := range node.SensorReadings {
			subtitleParts = append(subtitleParts, fmt.Sprintf("%s: %s", s.Label, s.FormattedValue()))
			if len(subtitleParts) >= 2 {
				break
			}
		}
	}

	return &domain.DeviceInterface{
		Type:         domain.InterfaceSensor,
		Category:     domain.DeviceClassSensor,
		SectionTitle: "Sensors",
		ScrollTag:    "sensors",
		Subtitle:     strings.Join(subtitleParts, " · "),
		SensorData:   node.SensorReadings,
	}
}

func extractCameraInterface(node *domain.DeviceNode) *domain.DeviceInterface {
	subtitle := node.Driver
	return &domain.DeviceInterface{
		Type:         domain.InterfaceCamera,
		Category:     domain.DeviceClassCamera,
		SectionTitle: "Camera",
		ScrollTag:    "camera",
		Subtitle:     subtitle,
		DevicePaths:  node.DevicePaths,
	}
}

func extractFallbackInterface(node *domain.DeviceNode) *domain.DeviceInterface {
	ifaceType := classToInterfaceType(node.Class)
	category := node.Class

	subtitle := node.Driver
	if subtitle == "" {
		subtitle = node.Product
	}

	return &domain.DeviceInterface{
		Type:         ifaceType,
		Category:     category,
		SectionTitle: "General",
		ScrollTag:    "general",
		Subtitle:     subtitle,
	}
}

func hasSerialPaths(node *domain.DeviceNode) bool {
	for _, p := range node.DevicePaths {
		if strings.Contains(p, "/ttyACM") || strings.Contains(p, "/ttyUSB") || strings.Contains(p, "/serial/") {
			return true
		}
	}
	return false
}

func extractSerialInterface(node *domain.DeviceNode) *domain.DeviceInterface {
	var serialPaths []string
	for _, p := range node.DevicePaths {
		if strings.Contains(p, "/ttyACM") || strings.Contains(p, "/ttyUSB") || strings.Contains(p, "/serial/") {
			serialPaths = append(serialPaths, p)
		}
	}

	subtitle := ""
	for _, p := range serialPaths {
		if strings.HasPrefix(p, "/dev/tty") {
			subtitle = p
			break
		}
	}

	return &domain.DeviceInterface{
		Type:         domain.InterfaceSerial,
		Category:     domain.DeviceClassSerial,
		SectionTitle: "Serial Device",
		ScrollTag:    "serial",
		Subtitle:     subtitle,
		DevicePaths:  serialPaths,
	}
}

func extractDebugInterface(node *domain.DeviceNode) *domain.DeviceInterface {
	subtitle := "Vendor-specific debug interface"
	if node.Driver != "" {
		subtitle = node.Driver
	}

	return &domain.DeviceInterface{
		Type:         domain.InterfaceDebug,
		Category:     domain.DeviceClassDebug,
		SectionTitle: "Debug Port",
		ScrollTag:    "debug",
		Subtitle:     subtitle,
	}
}

func classToInterfaceType(class domain.DeviceClass) domain.InterfaceType {
	switch class {
	case domain.DeviceClassGPU:
		return domain.InterfaceGraphics
	case domain.DeviceClassNetwork, domain.DeviceClassWireless:
		return domain.InterfaceNetwork
	case domain.DeviceClassStorage:
		return domain.InterfaceStorage
	case domain.DeviceClassAudio:
		return domain.InterfaceAudio
	case domain.DeviceClassUSBController, domain.DeviceClassHub:
		return domain.InterfaceUSB
	case domain.DeviceClassInput:
		return domain.InterfaceInput
	case domain.DeviceClassSerial:
		return domain.InterfaceSerial
	case domain.DeviceClassCamera:
		return domain.InterfaceCamera
	case domain.DeviceClassPrinter:
		return domain.InterfaceOther
	case domain.DeviceClassSecurity:
		return domain.InterfaceSecurity
	case domain.DeviceClassDisplay:
		return domain.InterfaceDisplay
	case domain.DeviceClassProcessor:
		return domain.InterfaceProcessor
	case domain.DeviceClassSensor:
		return domain.InterfaceSensor
	case domain.DeviceClassPower:
		return domain.InterfacePower
	case domain.DeviceClassLighting:
		return domain.InterfaceLighting
	case domain.DeviceClassMemory:
		return domain.InterfaceMemory
	case domain.DeviceClassPortable:
		return domain.InterfacePortable
	case domain.DeviceClassDebug:
		return domain.InterfaceDebug
	case domain.DeviceClassBridge:
		return domain.InterfaceSystem
	default:
		return domain.InterfaceOther
	}
}

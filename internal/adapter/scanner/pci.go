package scanner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	flatpak "gitlab.com/defenestra-software/go-flatbridge"

	"defenestra.io/chassis/assets"
	"defenestra.io/chassis/internal/core/domain"

	"github.com/jaypipes/ghw"
)

// Sourced from LACT (https://github.com/ilya-zlobintsev/LACT).
var deviceIDDB map[string]map[string]map[string]map[string]string

func init() {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(assets.DeviceIDsJSON, &raw); err != nil {
		return
	}
	deviceIDDB = make(map[string]map[string]map[string]map[string]string)
	for vendorID, vendorData := range raw {
		if vendorID == "_comment" {
			continue
		}
		var devices map[string]map[string]map[string]string
		if err := json.Unmarshal(vendorData, &devices); err != nil {
			continue
		}
		deviceIDDB[vendorID] = devices
	}
}

func readSysfsFileTrimmed(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func lookupDeviceName(vendorID, deviceID, subsysVendorID, subsysDeviceID string) string {
	if deviceIDDB == nil {
		return ""
	}
	vendorID = strings.ToLower(vendorID)
	deviceID = strings.ToLower(deviceID)
	subsysVendorID = strings.ToLower(subsysVendorID)
	subsysDeviceID = strings.ToLower(subsysDeviceID)

	if devices, ok := deviceIDDB[vendorID]; ok {
		if subsysVendors, ok := devices[deviceID]; ok {
			if subsysDevices, ok := subsysVendors[subsysVendorID]; ok {
				if name, ok := subsysDevices[subsysDeviceID]; ok {
					return name
				}
			}
		}
	}
	return ""
}

func (s *GHWScanner) scanPCIDevices(opts []*ghw.WithOption) (*domain.DeviceNode, error) {
	pciInfo, err := ghw.PCI(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to scan PCI devices: %w", err)
	}

	pciRoot := &domain.DeviceNode{
		ID:       "pci",
		ParentID: "root",
		Name:     "PCI Devices",
		Type:     domain.DeviceTypePCI,
		Class:    domain.DeviceClassOther,
		Children: make([]*domain.DeviceNode, 0),
	}

	deviceMap := make(map[string]*domain.DeviceNode)

	for _, dev := range pciInfo.Devices {
		node := s.pciDeviceToNode(dev)
		deviceMap[node.ID] = node
	}

	for _, node := range deviceMap {
		if node.ParentID != "" && node.ParentID != "pci" {
			if parent, ok := deviceMap[node.ParentID]; ok {
				parent.Children = append(parent.Children, node)
				continue
			}
		}
		node.ParentID = "pci"
		pciRoot.Children = append(pciRoot.Children, node)
	}

	return pciRoot, nil
}

func (s *GHWScanner) pciDeviceToNode(dev *ghw.PCIDevice) *domain.DeviceNode {
	id := fmt.Sprintf("pci:%s", dev.Address)
	sysfsPath := filepath.Join("/sys/bus/pci/devices", dev.Address)
	if s.chrootPath != "" {
		sysfsPath = filepath.Join(s.chrootPath, sysfsPath)
	}

	driver := s.readDriver(sysfsPath)

	iommuGroup := s.readIOMMUGroup(sysfsPath)

	classCode := s.readSysfsFile(filepath.Join(sysfsPath, "class"))
	classCode = strings.TrimPrefix(classCode, "0x")
	if len(classCode) >= 4 {
		classCode = classCode[:4] // Take first 4 hex digits (class + subclass)
	}

	name := dev.Address
	if dev.Vendor != nil && dev.Product != nil && !isUnknownName(dev.Product.Name) {
		name = fmt.Sprintf("%s %s", dev.Vendor.Name, dev.Product.Name)
	} else if dev.Vendor != nil && dev.Subclass != nil && !isUnknownName(dev.Subclass.Name) {
		name = fmt.Sprintf("%s %s", dev.Vendor.Name, dev.Subclass.Name)
	} else if dev.Vendor != nil {
		name = dev.Vendor.Name
	}

	vendor := ""
	vendorID := ""
	if dev.Vendor != nil {
		vendor = dev.Vendor.Name
		vendorID = dev.Vendor.ID
	}

	product := ""
	productID := ""
	if dev.Product != nil {
		product = dev.Product.Name
		productID = dev.Product.ID
	}

	// For GPUs, try to resolve the real marketing name using the embedded
	// device ID database (sourced from LACT), then fall back to pci.ids
	// subsystem name or sysfs subsystem IDs.
	if len(classCode) >= 2 && classCode[:2] == "03" {
		// Subsystem IDs from sysfs are more reliable than ghw here.
		subsysVendor := strings.TrimPrefix(readSysfsFileTrimmed(filepath.Join(flatpak.HostPath(sysfsPath), "subsystem_vendor")), "0x")
		subsysDevice := strings.TrimPrefix(readSysfsFileTrimmed(filepath.Join(flatpak.HostPath(sysfsPath), "subsystem_device")), "0x")

		if marketingName := lookupDeviceName(vendorID, productID, subsysVendor, subsysDevice); marketingName != "" {
			name = marketingName
		} else if dev.Subsystem != nil && dev.Subsystem.Name != "" {
			subName := strings.ToLower(dev.Subsystem.Name)
			if subName != "unknown" && !strings.Contains(subName, "unknown") {
				name = dev.Subsystem.Name
			}
		}
	} else if dev.Subsystem != nil && dev.Subsystem.Name != "" {
		subName := strings.ToLower(dev.Subsystem.Name)
		if subName != "unknown" && !strings.Contains(subName, "unknown") {
		}
	}

	caps := []string{"unbind"}
	if s.hasReset(sysfsPath) {
		caps = append(caps, "reset")
	}

	parentID := s.findPCIParent(dev.Address)

	class := s.classifyPCIDevice(classCode)

	if strings.EqualFold(vendorID, "1af4") {
		if virtioClass, ok := classifyVirtioDevice(productID); ok {
			class = virtioClass
		}
	}

	if class == domain.DeviceClassOther {
		nameLower := strings.ToLower(name)
		if strings.Contains(nameLower, "psp") || strings.Contains(nameLower, "ccp") ||
			strings.Contains(nameLower, "security processor") ||
			strings.Contains(nameLower, "trusted platform") ||
			strings.Contains(nameLower, "tpm") {
			class = domain.DeviceClassSecurity
		}
	}

	resources := s.readPCIResources(sysfsPath)

	return &domain.DeviceNode{
		ID:           id,
		ParentID:     parentID,
		Name:         name,
		Vendor:       vendor,
		Product:      product,
		VendorID:     vendorID,
		ProductID:    productID,
		Driver:       driver,
		Path:         filepath.Join("/sys/bus/pci/devices", dev.Address),
		Type:         domain.DeviceTypePCI,
		Class:        class,
		ClassCode:    classCode,
		IOMMUGroup:   iommuGroup,
		Capabilities: caps,
		Children:     make([]*domain.DeviceNode, 0),
		Resources:    resources,
	}
}

func isUnknownName(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return n == "" || n == "unknown"
}

var virtioDeviceClasses = map[string]domain.DeviceClass{
	"1000": domain.DeviceClassNetwork,  // virtio-net
	"1001": domain.DeviceClassStorage,  // virtio-blk
	"1002": domain.DeviceClassMemory,   // virtio-balloon
	"1003": domain.DeviceClassSerial,   // virtio-console
	"1004": domain.DeviceClassStorage,  // virtio-scsi
	"1005": domain.DeviceClassSecurity, // virtio-rng
	"1009": domain.DeviceClassStorage,  // virtio-9p
	"1041": domain.DeviceClassNetwork,  // virtio-net
	"1042": domain.DeviceClassStorage,  // virtio-blk
	"1043": domain.DeviceClassSerial,   // virtio-console
	"1044": domain.DeviceClassSecurity, // virtio-rng
	"1045": domain.DeviceClassMemory,   // virtio-balloon
	"1048": domain.DeviceClassStorage,  // virtio-scsi
	"1049": domain.DeviceClassStorage,  // virtio-9p
	"1050": domain.DeviceClassGPU,      // virtio-gpu
	"1052": domain.DeviceClassInput,    // virtio-input
	"1053": domain.DeviceClassNetwork,  // virtio-vsock
	"105a": domain.DeviceClassStorage,  // virtio-fs
}

func classifyVirtioDevice(deviceID string) (domain.DeviceClass, bool) {
	class, ok := virtioDeviceClasses[strings.ToLower(deviceID)]
	return class, ok
}

func (s *GHWScanner) classifyPCIDevice(classCode string) domain.DeviceClass {
	if len(classCode) < 2 {
		return domain.DeviceClassOther
	}

	// PCI class codes (first 2 hex digits)
	baseClass := classCode[:2]
	switch baseClass {
	case "03":
		return domain.DeviceClassGPU // Display controller
	case "02":
		return domain.DeviceClassNetwork // Network controller
	case "01":
		return domain.DeviceClassStorage // Mass storage
	case "04":
		return domain.DeviceClassAudio // Multimedia (audio/video)
	case "07":
		return domain.DeviceClassSerial // Simple communication controller (UART, COM ports)
	case "0c":
		if len(classCode) >= 4 {
			subClass := classCode[2:4]
			if subClass == "03" {
				return domain.DeviceClassUSBController // USB
			}
		}
		// SMBus, I2C, IPMI, FireWire are all system infrastructure
		return domain.DeviceClassBridge
	case "06":
		return domain.DeviceClassBridge // Bridge
	case "08":
		return domain.DeviceClassBridge // System peripheral (IOMMU, PIC, DMA, timer, RTC)
	case "0d":
		return domain.DeviceClassWireless // Wireless controller
	case "10":
		return domain.DeviceClassSecurity // Encryption/decryption (TPM, crypto accelerators)
	case "05":
		return domain.DeviceClassBridge // Memory controller
	case "11":
		return domain.DeviceClassBridge // Signal processing (performance counters, etc.)
	case "13":
		return domain.DeviceClassBridge // Non-essential instrumentation
	default:
		return domain.DeviceClassOther
	}
}

func (s *GHWScanner) readIOMMUGroup(sysfsPath string) int {
	iommuLink := filepath.Join(sysfsPath, "iommu_group")
	target, err := os.Readlink(iommuLink)
	if err != nil {
		return -1
	}
	groupStr := filepath.Base(target)
	group, err := strconv.Atoi(groupStr)
	if err != nil {
		return -1
	}
	return group
}

func (s *GHWScanner) hasReset(sysfsPath string) bool {
	resetPath := filepath.Join(sysfsPath, "reset")
	_, err := os.Stat(resetPath)
	return err == nil
}

func (s *GHWScanner) readPCIResources(sysfsPath string) *domain.DeviceResources {
	res := &domain.DeviceResources{
		IRQ:      -1,
		NUMANode: -1,
	}

	if irqStr := s.readSysfsFile(filepath.Join(sysfsPath, "irq")); irqStr != "" {
		if irq, err := strconv.Atoi(irqStr); err == nil && irq > 0 {
			res.IRQ = irq
		}
	}

	msiPath := filepath.Join(sysfsPath, "msi_irqs")
	if entries, err := os.ReadDir(msiPath); err == nil {
		for _, entry := range entries {
			if irq, err := strconv.Atoi(entry.Name()); err == nil {
				res.IRQs = append(res.IRQs, irq)
			}
		}
	}

	if numaStr := s.readSysfsFile(filepath.Join(sysfsPath, "numa_node")); numaStr != "" {
		if numa, err := strconv.Atoi(numaStr); err == nil {
			res.NUMANode = numa
		}
	}

	res.LocalCPUs = s.readSysfsFile(filepath.Join(sysfsPath, "local_cpulist"))

	resourceFile := filepath.Join(sysfsPath, "resource")
	if data, err := os.ReadFile(resourceFile); err == nil {
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		for i, line := range lines {
			if resource := s.parseResourceLine(line, i); resource != nil {
				if resource.Type == domain.ResourceTypeMemory {
					res.Memory = append(res.Memory, *resource)
				} else if resource.Type == domain.ResourceTypeIO {
					res.IOPorts = append(res.IOPorts, *resource)
				}
			}
		}
	}

	return res
}

func (s *GHWScanner) parseResourceLine(line string, index int) *domain.Resource {
	fields := strings.Fields(line)
	if len(fields) != 3 {
		return nil
	}

	start, err := strconv.ParseUint(strings.TrimPrefix(fields[0], "0x"), 16, 64)
	if err != nil {
		return nil
	}

	end, err := strconv.ParseUint(strings.TrimPrefix(fields[1], "0x"), 16, 64)
	if err != nil {
		return nil
	}

	flags, err := strconv.ParseUint(strings.TrimPrefix(fields[2], "0x"), 16, 64)
	if err != nil {
		return nil
	}

	if start == 0 && end == 0 {
		return nil
	}

	res := &domain.Resource{
		Start: start,
		End:   end,
		Size:  end - start + 1,
		Flags: flags,
		Index: index,
	}

	// Bit 0: 1 = I/O, 0 = Memory
	// Bit 3: 1 = Prefetchable (for memory)
	if flags&0x1 != 0 {
		res.Type = domain.ResourceTypeIO
	} else {
		res.Type = domain.ResourceTypeMemory
		res.Prefetchable = (flags & 0x8) != 0
	}

	return res
}

func (s *GHWScanner) findPCIParent(address string) string {
	// PCI address format: DDDD:BB:DD.F (domain:bus:device.function)
	// Devices on bus 00 are root devices. For other buses, find the bridge
	// that created this bus by reading the secondary bus number from sysfs.
	parts := strings.Split(address, ":")
	if len(parts) != 3 {
		return "pci"
	}

	if parts[1] == "00" {
		return "pci"
	}

	// For devices on higher buses, the parent is the bridge whose
	// secondary_bus_number matches this device's bus. We find it by
	// checking /sys/bus/pci/devices/*/secondary_bus_number.
	targetBus := parts[1]
	pciDevicesPath := "/sys/bus/pci/devices"
	if s.chrootPath != "" {
		pciDevicesPath = filepath.Join(s.chrootPath, pciDevicesPath)
	}

	entries, err := os.ReadDir(pciDevicesPath)
	if err != nil {
		return "pci"
	}

	for _, entry := range entries {
		secBus := s.readSysfsFile(filepath.Join(pciDevicesPath, entry.Name(), "secondary_bus_number"))
		if secBus != "" && strings.EqualFold(secBus, "0x"+targetBus) {
			return "pci:" + entry.Name()
		}
	}

	return "pci"
}

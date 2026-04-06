package sysinfo

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	flatpak "gitlab.com/defenestra-software/go-flatbridge"

	"defenestra.io/chassis/internal/core/domain"
)

func (s *Scanner) ScanSensors() []*domain.DeviceNode {
	var nodes []*domain.DeviceNode

	hwmonPath := flatpak.HostPath("/sys/class/hwmon")
	entries, err := os.ReadDir(hwmonPath)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "hwmon") {
			continue
		}
		chipPath := filepath.Join(hwmonPath, entry.Name())
		node := s.scanHWMonChip(chipPath, entry.Name())
		if node != nil {
			nodes = append(nodes, node)
		}
	}

	s.logger.Debug("scanned sensors", "count", len(nodes))
	return nodes
}

var hwmonFriendlyNames = map[string]string{
	"k10temp":          "CPU Temperature",
	"zenpower":         "CPU Temperature",
	"coretemp":         "CPU Temperature",
	"acpitz":           "ACPI Thermal Zone",
	"amdgpu":           "GPU Temperature",
	"nouveau":          "GPU Temperature",
	"iwlwifi_1":        "WiFi Temperature",
	"nvme":             "NVMe Temperature",
	"it87":             "Motherboard Sensors",
	"nct6775":          "Motherboard Sensors",
	"nct6776":          "Motherboard Sensors",
	"nct6779":          "Motherboard Sensors",
	"nct6791":          "Motherboard Sensors",
	"nct6792":          "Motherboard Sensors",
	"nct6795":          "Motherboard Sensors",
	"nct6796":          "Motherboard Sensors",
	"nct6798":          "Motherboard Sensors",
	"asus_wmi_sensors": "ASUS Sensors",
	"gigabyte_wmi":     "Gigabyte Sensors",
	"Gigabyte_wmi":     "Gigabyte Sensors",
	"asus-ec-sensors":  "ASUS EC Sensors",
	"nzxt-smart2":      "NZXT Smart Device",
	"r8169":            "Ethernet Temperature",
	"igb":              "Ethernet Temperature",
	"iwlwifi":          "WiFi Temperature",
	"mt7921_phy0":      "WiFi Temperature",
	"rtw89_phy0":       "WiFi Temperature",
	"zenergy":          "CPU Energy Monitor",
}

func (s *Scanner) scanHWMonChip(chipPath, hwmonName string) *domain.DeviceNode {
	chipName := readFile(filepath.Join(chipPath, "name"))
	if chipName == "" {
		return nil
	}

	// Build a human-readable name - try exact match, then prefix match
	displayName := chipName
	if friendly, ok := hwmonFriendlyNames[chipName]; ok {
		displayName = friendly
	} else {
		// Try prefix matching for names like "r8169_0_700:00"
		chipLower := strings.ToLower(chipName)
		for key, friendly := range hwmonFriendlyNames {
			if strings.HasPrefix(chipLower, strings.ToLower(key)) {
				displayName = friendly
				break
			}
		}
	}

	// Skip battery sensors (handled by battery enrichment)
	if strings.HasPrefix(chipName, "hidpp_battery") || strings.HasPrefix(chipName, "BAT") {
		return nil
	}

	// Try to find associated device for context
	deviceLink := filepath.Join(chipPath, "device")
	deviceDesc := ""
	if resolved, err := filepath.EvalSymlinks(deviceLink); err == nil {
		devName := filepath.Base(resolved)
		if devName != "" && devName != chipName {
			deviceDesc = devName
		}
	}

	// For sensors with friendly names that need disambiguation (multiple GPUs/NVMe),
	// append the device identifier in parentheses
	if deviceDesc != "" {
		switch displayName {
		case "GPU Temperature":
			displayName = fmt.Sprintf("GPU Temperature (%s)", deviceDesc)
		case "NVMe Temperature":
			displayName = fmt.Sprintf("NVMe Temperature (%s)", deviceDesc)
		case chipName:
			// No friendly name found - use chipName with device context
			displayName = fmt.Sprintf("%s (%s)", chipName, deviceDesc)
		}
	}

	// Collect sensor readings
	var sensors []*domain.SensorInfo
	sensors = append(sensors, s.readTempSensors(chipPath, chipName)...)
	sensors = append(sensors, s.readFanSensors(chipPath, chipName)...)
	sensors = append(sensors, s.readVoltageSensors(chipPath, chipName)...)
	sensors = append(sensors, s.readPowerSensors(chipPath, chipName)...)

	if len(sensors) == 0 {
		return nil
	}

	// Build subtitle with current readings summary
	var subtitleParts []string
	for _, sensor := range sensors {
		if sensor.Type == "temperature" {
			subtitleParts = append(subtitleParts, fmt.Sprintf("%s: %s", sensor.Label, sensor.FormattedValue()))
			if len(subtitleParts) >= 2 {
				break // Show max 2 temp readings in subtitle
			}
		}
	}
	if len(subtitleParts) == 0 {
		// No temps, show fan or other
		for _, sensor := range sensors {
			subtitleParts = append(subtitleParts, fmt.Sprintf("%s: %s", sensor.Label, sensor.FormattedValue()))
			if len(subtitleParts) >= 2 {
				break
			}
		}
	}
	subtitle := strings.Join(subtitleParts, " · ")

	node := &domain.DeviceNode{
		ID:             "sensor:" + hwmonName,
		Name:           displayName,
		Driver:         deviceDesc,
		Product:        subtitle,
		Type:           domain.DeviceTypeUnknown,
		Class:          domain.DeviceClassSensor,
		Path:           chipPath,
		SensorReadings: sensors,
		IOMMUGroup:     -1,
		Children:       make([]*domain.DeviceNode, 0),
	}

	return node
}

func (s *Scanner) readTempSensors(chipPath, chipName string) []*domain.SensorInfo {
	var sensors []*domain.SensorInfo
	for i := 1; i <= 20; i++ {
		inputFile := filepath.Join(chipPath, fmt.Sprintf("temp%d_input", i))
		valStr := readFile(inputFile)
		if valStr == "" {
			break
		}
		millideg, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			continue
		}

		sensor := &domain.SensorInfo{
			Name:       chipName,
			Type:       "temperature",
			Value:      millideg / 1000,
			HWMonPath:  inputFile,
			DeviceName: chipName,
		}

		// Read label
		sensor.Label = readFile(filepath.Join(chipPath, fmt.Sprintf("temp%d_label", i)))
		if sensor.Label == "" {
			sensor.Label = fmt.Sprintf("Sensor %d", i)
		}

		// Read thresholds
		if v := readFile(filepath.Join(chipPath, fmt.Sprintf("temp%d_crit", i))); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				sensor.Critical = f / 1000
			}
		}
		if v := readFile(filepath.Join(chipPath, fmt.Sprintf("temp%d_max", i))); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				sensor.Max = f / 1000
			}
		}

		sensors = append(sensors, sensor)
	}
	return sensors
}

func (s *Scanner) readFanSensors(chipPath, chipName string) []*domain.SensorInfo {
	var sensors []*domain.SensorInfo
	for i := 1; i <= 10; i++ {
		inputFile := filepath.Join(chipPath, fmt.Sprintf("fan%d_input", i))
		valStr := readFile(inputFile)
		if valStr == "" {
			break
		}
		rpm, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			continue
		}

		sensor := &domain.SensorInfo{
			Name:       chipName,
			Type:       "fan",
			Value:      rpm,
			HWMonPath:  inputFile,
			DeviceName: chipName,
		}
		sensor.Label = readFile(filepath.Join(chipPath, fmt.Sprintf("fan%d_label", i)))
		if sensor.Label == "" {
			sensor.Label = fmt.Sprintf("Fan %d", i)
		}

		sensors = append(sensors, sensor)
	}
	return sensors
}

func (s *Scanner) readVoltageSensors(chipPath, chipName string) []*domain.SensorInfo {
	var sensors []*domain.SensorInfo
	for i := 0; i <= 15; i++ {
		inputFile := filepath.Join(chipPath, fmt.Sprintf("in%d_input", i))
		valStr := readFile(inputFile)
		if valStr == "" {
			if i > 0 {
				break
			}
			continue
		}
		millivolts, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			continue
		}

		sensor := &domain.SensorInfo{
			Name:       chipName,
			Type:       "voltage",
			Value:      millivolts / 1000,
			HWMonPath:  inputFile,
			DeviceName: chipName,
		}
		sensor.Label = readFile(filepath.Join(chipPath, fmt.Sprintf("in%d_label", i)))
		if sensor.Label == "" {
			sensor.Label = fmt.Sprintf("Voltage %d", i)
		}

		sensors = append(sensors, sensor)
	}
	return sensors
}

func (s *Scanner) readPowerSensors(chipPath, chipName string) []*domain.SensorInfo {
	var sensors []*domain.SensorInfo
	for i := 1; i <= 5; i++ {
		inputFile := filepath.Join(chipPath, fmt.Sprintf("power%d_input", i))
		valStr := readFile(inputFile)
		if valStr == "" {
			break
		}
		microwatts, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			continue
		}

		sensor := &domain.SensorInfo{
			Name:       chipName,
			Type:       "power",
			Value:      microwatts / 1000000, // Convert to watts
			HWMonPath:  inputFile,
			DeviceName: chipName,
		}
		sensor.Label = readFile(filepath.Join(chipPath, fmt.Sprintf("power%d_label", i)))
		if sensor.Label == "" {
			sensor.Label = fmt.Sprintf("Power %d", i)
		}

		sensors = append(sensors, sensor)
	}
	return sensors
}

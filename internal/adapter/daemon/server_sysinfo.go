package daemon

import (
	"fmt"
	"os"
	"strings"

	"github.com/godbus/dbus/v5"

	flatpak "gitlab.com/defenestra-software/go-flatbridge"

	"defenestra.io/chassis/internal/core/domain"
)

func (s *Server) ReadSystemInfo() (string, *dbus.Error) {
	s.logger.Info("system info read requested")

	// Read board serial (root-only file)
	serial := ""
	if data, err := os.ReadFile(flatpak.HostPath("/sys/class/dmi/id/board_serial")); err == nil {
		serial = strings.TrimSpace(string(data))
	}

	// Read memory modules via dmidecode, fall back to sysfs DMI tables
	memJSON := "[]"
	output, err := runHostCommandOutput("dmidecode", "-t", "memory", "-q")
	if err == nil {
		memJSON = parseDMIMemory(string(output))
	} else {
		s.logger.Debug("dmidecode failed, trying sysfs DMI", "error", err)
		memJSON = s.readMemoryFromSysfs()
	}

	result := fmt.Sprintf(`{"board_serial":%q,"memory_modules":%s}`, serial, memJSON)
	return result, nil
}

func parseDMIMemory(output string) string {
	type memModule struct {
		Label  string `json:"label"`
		Size   string `json:"size"`
		Type   string `json:"type"`
		Speed  string `json:"speed"`
		Vendor string `json:"vendor"`
		Serial string `json:"serial"`
		Part   string `json:"part"`
		Rank   string `json:"rank"`
	}

	var modules []memModule
	var current *memModule
	inDevice := false

	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "Memory Device") {
			if current != nil && current.Size != "" && current.Size != "No Module Installed" {
				modules = append(modules, *current)
			}
			current = &memModule{}
			inDevice = true
			continue
		}

		if !inDevice || current == nil {
			continue
		}

		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch key {
		case "Size":
			current.Size = val
		case "Module Manufacturer ID":
			// JEDEC JEP106 ID like "Bank 5, Hex 0xEF"
			if jedecVendor := lookupJEDEC(val); jedecVendor != "" {
				current.Vendor = jedecVendor
			}
		case "Locator":
			current.Label = val
		case "Bank Locator":
			if val != "" && val != "Not Specified" && val != "Unknown" {
				if current.Label != "" {
					current.Label = current.Label + " / " + val
				} else {
					current.Label = val
				}
			}
		case "Type":
			current.Type = val
		case "Speed":
			current.Speed = val
		case "Configured Memory Speed", "Configured Clock Speed":
			if current.Speed == "" || current.Speed == "Unknown" {
				current.Speed = val
			}
		case "Manufacturer":
			// Only use if JEDEC lookup didn't already set it
			if current.Vendor == "" {
				current.Vendor = val
			}
		case "Serial Number":
			current.Serial = val
		case "Part Number":
			current.Part = strings.TrimSpace(val)
		case "Rank":
			current.Rank = val
		}
	}

	// Don't forget last module
	if current != nil && current.Size != "" && current.Size != "No Module Installed" {
		modules = append(modules, *current)
	}

	if len(modules) == 0 {
		return "[]"
	}

	// Build JSON manually to avoid importing encoding/json
	result := "["
	for i, m := range modules {
		if i > 0 {
			result += ","
		}
		result += fmt.Sprintf(`{"label":%q,"size":%q,"type":%q,"speed":%q,"vendor":%q,"serial":%q,"part":%q,"rank":%q}`,
			m.Label, m.Size, m.Type, m.Speed, m.Vendor, m.Serial, m.Part, m.Rank)
	}
	result += "]"
	return result
}

func (s *Server) DumpVBIOS(pciAddress string) ([]byte, *dbus.Error) {
	s.logger.Info("VBIOS dump requested", "device", pciAddress)

	if !pciAddressRegex.MatchString(pciAddress) {
		return nil, dbus.MakeFailedError(domain.ErrInvalidPath)
	}

	romPath := fmt.Sprintf("/sys/bus/pci/devices/%s/rom", pciAddress)
	romPath = flatpak.HostPath(romPath)

	if _, err := os.Stat(romPath); os.IsNotExist(err) {
		return nil, dbus.MakeFailedError(fmt.Errorf("ROM file not found for %s", pciAddress))
	}

	// Enable ROM reading
	if err := os.WriteFile(romPath, []byte("1"), 0644); err != nil {
		return nil, dbus.MakeFailedError(fmt.Errorf("failed to enable ROM: %w", err))
	}
	defer os.WriteFile(romPath, []byte("0"), 0644)

	// Read ROM data
	data, err := os.ReadFile(romPath)
	if err != nil {
		return nil, dbus.MakeFailedError(fmt.Errorf("failed to read ROM: %w", err))
	}

	if len(data) == 0 {
		return nil, dbus.MakeFailedError(fmt.Errorf("ROM data is empty for %s", pciAddress))
	}

	s.logger.Info("VBIOS dump complete", "device", pciAddress, "bytes", len(data))
	return data, nil
}

func (s *Server) ReadSMART(blockDevice string) (string, *dbus.Error) {
	s.logger.Info("SMART read requested", "device", blockDevice)

	if !blockDeviceRegex.MatchString(blockDevice) {
		return "", dbus.MakeFailedError(domain.ErrInvalidPath)
	}

	devPath := flatpak.HostPath("/dev/" + blockDevice)

	output, err := runHostCommandOutput("smartctl", "-a", devPath, "-j")
	if err != nil {
		// smartctl returns non-zero for various reasons (e.g., exit code 4 = SMART check failed)
		// but still outputs valid JSON. Only fail if output is empty.
		if len(output) == 0 {
			s.logger.Error("smartctl failed", "device", blockDevice, "error", err)
			return "", dbus.MakeFailedError(fmt.Errorf("smartctl: %w", err))
		}
		s.logger.Debug("smartctl returned non-zero but produced output", "device", blockDevice)
	}

	return string(output), nil
}

func lookupJEDEC(id string) string {
	// Parse "Bank N, Hex 0xHH"
	id = strings.TrimSpace(id)
	var bank int
	var hex string
	if _, err := fmt.Sscanf(id, "Bank %d, Hex %s", &bank, &hex); err != nil {
		return ""
	}
	hex = strings.TrimPrefix(strings.ToUpper(hex), "0X")

	key := fmt.Sprintf("%d:%s", bank, hex)
	if name, ok := jedecVendors[key]; ok {
		return name
	}
	return ""
}

// Source: JEDEC JEP106
var jedecVendors = map[string]string{
	// Bank 1
	"1:01": "AMD",
	"1:04": "Fujitsu",
	"1:07": "Hitachi",
	"1:10": "NEC",
	"1:15": "NXP",
	"1:1C": "Mitsubishi",
	"1:20": "STMicroelectronics",
	"1:2C": "Micron",
	"1:38": "Atmel",
	"1:4F": "Transcend",
	"1:89": "Intel",
	"1:97": "Texas Instruments",
	"1:98": "Toshiba",
	"1:AD": "SK Hynix",
	"1:C1": "Infineon",
	"1:CE": "Samsung",

	// Bank 2
	"2:9E": "Corsair",
	"2:43": "Ramaxel",
	"2:51": "Qimonda",
	"2:CB": "A-DATA",
	"2:F1": "Innodisk",

	// Bank 3
	"3:0B": "Nanya",
	"3:25": "Kingmax",
	"3:94": "Mushkin",
	"3:FE": "Elpida",

	// Bank 4
	"4:43": "Ramaxel",
	"4:F1": "Smart Modular",

	// Bank 5
	"5:01": "PQI",
	"5:51": "Qimonda",
	"5:EF": "TeamGroup",

	// Bank 6
	"6:02": "Patriot",
	"6:34": "Crucial",
	"6:51": "Apacer",
	"6:CB": "A-DATA",
	"6:CD": "OCZ",
	"6:F1": "G.Skill",

	// Bank 7
	"7:02": "GoodRAM",
	"7:43": "Super Talent",
	"7:AD": "XPG",
	"7:EF": "Longsys",
	"7:F1": "V-Color",

	// Bank 8
	"8:01": "Lexar",
	"8:04": "Juhor",
	"8:51": "OLOy",
	"8:AD": "Goldkey",
}

// Parses raw SMBIOS Type 17 (Memory Device) entries from the DMI table
// as a fallback when dmidecode is not available.
func (s *Server) readMemoryFromSysfs() string {
	dmiPath := flatpak.HostPath("/sys/firmware/dmi/tables/DMI")
	data, err := os.ReadFile(dmiPath)
	if err != nil {
		s.logger.Debug("could not read DMI tables", "error", err)
		return "[]"
	}

	type memModule struct {
		Label  string
		Size   string
		Type   string
		Speed  string
		Vendor string
		Serial string
		Part   string
		Rank   string
	}
	var modules []memModule

	offset := 0
	for offset+4 <= len(data) {
		smbType := data[offset]
		length := int(data[offset+1])
		if length < 4 || offset+length > len(data) {
			break
		}

		// Find the strings section start (immediately after formatted area)
		strStart := offset + length

		if smbType == 17 && length >= 0x1B {
			// Size at offset 0x0C (2 bytes)
			sizeRaw := uint16(data[offset+0x0C]) | uint16(data[offset+0x0D])<<8
			if sizeRaw != 0 && sizeRaw != 0xFFFF {
				sizeMB := int(sizeRaw & 0x7FFF)
				if sizeRaw&0x8000 != 0 {
					sizeMB = sizeMB / 1024 // KB to MB
				}
				// Extended size for modules > 32 GB
				if sizeRaw == 0x7FFF && length >= 0x20 {
					sizeMB = int(uint32(data[offset+0x1C]) | uint32(data[offset+0x1D])<<8 |
						uint32(data[offset+0x1E])<<16 | uint32(data[offset+0x1F])<<24)
				}
				sizeStr := fmt.Sprintf("%d MB", sizeMB)
				if sizeMB >= 1024 {
					sizeStr = fmt.Sprintf("%d GB", sizeMB/1024)
				}

				// Memory type at offset 0x12
				typeStr := ""
				switch data[offset+0x12] {
				case 20:
					typeStr = "DDR"
				case 21:
					typeStr = "DDR2"
				case 24:
					typeStr = "DDR3"
				case 26:
					typeStr = "DDR4"
				case 30:
					typeStr = "LPDDR4"
				case 34:
					typeStr = "DDR5"
				case 35:
					typeStr = "LPDDR5"
				}

				// Speed at offset 0x15 (2 bytes, MHz)
				speedStr := ""
				speedMHz := uint16(data[offset+0x15]) | uint16(data[offset+0x16])<<8
				if speedMHz > 0 && speedMHz != 0xFFFF {
					speedStr = fmt.Sprintf("%d MT/s", speedMHz)
				}
				// Prefer configured speed at 0x20 if available
				if length >= 0x22 {
					cfgSpeed := uint16(data[offset+0x20]) | uint16(data[offset+0x21])<<8
					if cfgSpeed > 0 && cfgSpeed != 0xFFFF {
						speedStr = fmt.Sprintf("%d MT/s", cfgSpeed)
					}
				}

				label := readSMBIOSString(data, strStart, int(data[offset+0x10]))
				if label == "" {
					label = fmt.Sprintf("DIMM %d", len(modules))
				}

				vendor := strings.TrimSpace(readSMBIOSString(data, strStart, int(data[offset+0x17])))
				serial := strings.TrimSpace(readSMBIOSString(data, strStart, int(data[offset+0x18])))
				part := strings.TrimSpace(readSMBIOSString(data, strStart, int(data[offset+0x1A])))

				rankStr := ""
				if rank := data[offset+0x1B] & 0x0F; rank > 0 && rank < 0x0F {
					rankStr = fmt.Sprintf("%d", rank)
				}

				modules = append(modules, memModule{
					Label:  label,
					Size:   sizeStr,
					Type:   typeStr,
					Speed:  speedStr,
					Vendor: vendor,
					Serial: serial,
					Part:   part,
					Rank:   rankStr,
				})
			}
		}

		// Skip past formatted area to strings section, then past double-null terminator
		strOffset := offset + length
		for strOffset+1 < len(data) {
			if data[strOffset] == 0 && data[strOffset+1] == 0 {
				strOffset += 2
				break
			}
			strOffset++
		}
		offset = strOffset
	}

	if len(modules) == 0 {
		return "[]"
	}

	result := "["
	for i, m := range modules {
		if i > 0 {
			result += ","
		}
		result += fmt.Sprintf(`{"label":%q,"size":%q,"type":%q,"speed":%q,"vendor":%q,"serial":%q,"part":%q,"rank":%q}`,
			m.Label, m.Size, m.Type, m.Speed, m.Vendor, m.Serial, m.Part, m.Rank)
	}
	result += "]"
	return result
}

func readSMBIOSString(data []byte, strStart, index int) string {
	if index <= 0 {
		return ""
	}
	cur := 1
	pos := strStart
	for pos < len(data) {
		end := pos
		for end < len(data) && data[end] != 0 {
			end++
		}
		if cur == index {
			return string(data[pos:end])
		}
		cur++
		pos = end + 1
		// Double-null means end of strings
		if pos < len(data) && data[pos] == 0 {
			break
		}
	}
	return ""
}

package sysinfo

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	flatpak "gitlab.com/defenestra-software/go-flatbridge"

	"defenestra.io/chassis/internal/core/domain"
)

// ScanDisplays builds monitor nodes from sysfs EDID (manufacturer and model).
func (s *Scanner) ScanDisplays() []*domain.DeviceNode {
	var nodes []*domain.DeviceNode

	drmPath := flatpak.HostPath("/sys/class/drm")
	entries, err := os.ReadDir(drmPath)
	if err != nil {
		return nil
	}

	connectorPattern := regexp.MustCompile(`^card(\d+)-(.+)-(\d+)$`)

	for _, entry := range entries {
		match := connectorPattern.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}

		connPath := filepath.Join(drmPath, entry.Name())
		status := readFile(filepath.Join(connPath, "status"))
		if status != "connected" {
			continue
		}

		connType := match[2]
		connIndex := match[3]
		connName := fmt.Sprintf("%s-%s", connType, connIndex)

		modesStr := readFile(filepath.Join(connPath, "modes"))
		var modeList []string
		currentMode := ""
		if modesStr != "" {
			modeList = strings.Split(strings.TrimSpace(modesStr), "\n")
			if len(modeList) > 0 {
				currentMode = modeList[0]
			}
		}

		dispInfo := s.parseFullEDID(filepath.Join(connPath, "edid"))
		dispInfo.ConnectorType = connType
		dispInfo.ConnectorIndex = connIndex
		dispInfo.GPUCard = fmt.Sprintf("card%s", match[1])
		dispInfo.CurrentMode = currentMode
		dispInfo.ModeCount = len(modeList)
		dispInfo.Modes = modeList

		displayName := connName
		if dispInfo.MonitorName != "" {
			if dispInfo.Vendor != "" {
				displayName = fmt.Sprintf("%s %s", dispInfo.Vendor, dispInfo.MonitorName)
			} else {
				displayName = dispInfo.MonitorName
			}
		}

		subtitleParts := []string{connName}
		if currentMode != "" {
			subtitleParts = append(subtitleParts, currentMode)
		}
		if dispInfo.MaxRefresh > 0 {
			subtitleParts = append(subtitleParts, fmt.Sprintf("%d Hz", dispInfo.MaxRefresh))
		}
		if sz := dispInfo.SizeString(); sz != "" {
			subtitleParts = append(subtitleParts, sz)
		}
		subtitle := strings.Join(subtitleParts, " · ")

		node := &domain.DeviceNode{
			ID:          "display:" + entry.Name(),
			Name:        displayName,
			Vendor:      dispInfo.Vendor,
			Product:     subtitle,
			Type:        domain.DeviceTypeUnknown,
			Class:       domain.DeviceClassDisplay,
			DisplayInfo: dispInfo,
			Path:        connPath,
			IOMMUGroup:  -1,
			Children:    make([]*domain.DeviceNode, 0),
		}

		nodes = append(nodes, node)
	}

	s.logger.Debug("scanned displays", "connected", len(nodes))
	return nodes
}

func (s *Scanner) parseFullEDID(edidPath string) *domain.DisplayInfo {
	info := &domain.DisplayInfo{}

	data, err := os.ReadFile(edidPath)
	if err != nil || len(data) < 128 {
		return info
	}

	header := []byte{0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x00}
	for i := 0; i < 8; i++ {
		if data[i] != header[i] {
			return info
		}
	}

	mfg1 := ((data[8] >> 2) & 0x1F) + 'A' - 1
	mfg2 := (((data[8] & 0x03) << 3) | ((data[9] >> 5) & 0x07)) + 'A' - 1
	mfg3 := (data[9] & 0x1F) + 'A' - 1
	vendorCode := string([]byte{mfg1, mfg2, mfg3})
	if v := edidVendorName(vendorCode); v != "" {
		info.Vendor = v
	} else {
		info.Vendor = vendorCode
	}

	info.ManufactureWeek = int(data[16])
	info.ManufactureYear = int(data[17]) + 1990

	info.EDIDVersion = fmt.Sprintf("%d.%d", data[18], data[19])

	if data[21] > 0 && data[22] > 0 {
		info.PhysicalWidth = int(data[21]) * 10
		info.PhysicalHeight = int(data[22]) * 10
	}

	for i := 54; i <= 108; i += 18 {
		if i+18 > len(data) {
			break
		}
		if data[i] == 0 && data[i+1] == 0 {
			tag := data[i+3]
			switch tag {
			case 0xFC: // Monitor name
				nameBytes := data[i+5 : i+18]
				info.MonitorName = strings.TrimRight(string(nameBytes), "\n\r \x0a")
			case 0xFF: // Serial number
				serialBytes := data[i+5 : i+18]
				info.Serial = strings.TrimRight(string(serialBytes), "\n\r \x0a")
			case 0xFD: // Range limits
				info.MinRefresh = int(data[i+5])
				info.MaxRefresh = int(data[i+6])
			}
		}
	}

	return info
}

func (s *Scanner) readEDID(edidPath string) (name, vendor string) {
	data, err := os.ReadFile(edidPath)
	if err != nil || len(data) < 128 {
		return "", ""
	}

	header := []byte{0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x00}
	for i := 0; i < 8; i++ {
		if data[i] != header[i] {
			return "", ""
		}
	}

	mfg1 := ((data[8] >> 2) & 0x1F) + 'A' - 1
	mfg2 := (((data[8] & 0x03) << 3) | ((data[9] >> 5) & 0x07)) + 'A' - 1
	mfg3 := (data[9] & 0x1F) + 'A' - 1
	vendorCode := string([]byte{mfg1, mfg2, mfg3})

	vendor = edidVendorName(vendorCode)
	if vendor == "" {
		vendor = vendorCode
	}

	for i := 54; i <= 108; i += 18 {
		if i+18 > len(data) {
			break
		}
		if data[i] == 0 && data[i+1] == 0 && data[i+3] == 0xFC {
			nameBytes := data[i+5 : i+18]
			nameStr := strings.TrimRight(string(nameBytes), "\n\r \x0a")
			if nameStr != "" {
				name = nameStr
			}
		}
	}

	if name == "" {
		productCode := uint16(data[9])<<8 | uint16(data[8])
		name = fmt.Sprintf("%s %04X", vendor, productCode)
	}

	return name, vendor
}

// edidVendorName maps common 3-letter EDID vendor codes to full names.
var edidVendors = map[string]string{
	"ACR": "Acer",
	"ACI": "ASUS",
	"AUS": "ASUS",
	"BNQ": "BenQ",
	"CMN": "Innolux",
	"DEL": "Dell",
	"ENC": "Eizo",
	"GBT": "Gigabyte",
	"GSM": "LG",
	"HPN": "HP",
	"HWP": "HP",
	"IVM": "Iiyama",
	"LEN": "Lenovo",
	"LGD": "LG Display",
	"MEI": "Panasonic",
	"MSI": "MSI",
	"NEC": "NEC",
	"PHL": "Philips",
	"SAM": "Samsung",
	"SEC": "Samsung",
	"SDC": "Samsung",
	"SNY": "Sony",
	"SPT": "Sceptre",
	"VSC": "ViewSonic",
	"AOC": "AOC",
	"APP": "Apple",
	"BOE": "BOE",
	"AUO": "AU Optronics",
	"SHP": "Sharp",
	"JDI": "JDI",
	"CHI": "Chi Mei",
	"HSD": "HannStar",
	"INL": "InnoLux",
	"IVO": "InfoVision",
	"LPL": "LG Philips",
}

func edidVendorName(code string) string {
	if name, ok := edidVendors[code]; ok {
		return name
	}
	return ""
}

package domain

import (
	"regexp"
	"strings"
)

var vendorShortNames = map[string]string{
	"advanced micro devices, inc.":    "AMD",
	"advanced micro devices":          "AMD",
	"amd":                             "AMD",
	"[amd]":                           "AMD",
	"[amd/ati]":                       "AMD",
	"nvidia corporation":              "NVIDIA",
	"nvidia":                          "NVIDIA",
	"intel corporation":               "Intel",
	"intel":                           "Intel",
	"realtek semiconductor co., ltd.": "Realtek",
	"realtek semiconductor":           "Realtek",
	"realtek":                         "Realtek",
	"sandisk corp":                    "SanDisk",
	"sandisk":                         "SanDisk",
	"western digital":                 "WD",
	"samsung electronics co ltd":      "Samsung",
	"samsung electronics":             "Samsung",
	"samsung":                         "Samsung",
	"micron technology inc":           "Micron",
	"micron":                          "Micron",
	"sk hynix":                        "SK Hynix",
	"kingston technology company":     "Kingston",
	"kingston":                        "Kingston",
	"broadcom inc. and subsidiaries":  "Broadcom",
	"broadcom limited":                "Broadcom",
	"broadcom":                        "Broadcom",
	"qualcomm":                        "Qualcomm",
	"marvell technology group ltd.":   "Marvell",
	"marvell":                         "Marvell",
	"mediatek inc.":                   "MediaTek",
	"mediatek":                        "MediaTek",
	"texas instruments":               "TI",
	"logitech, inc.":                  "Logitech",
	"logitech":                        "Logitech",
	"corsair":                         "Corsair",
	"razer usa, ltd":                  "Razer",
	"razer":                           "Razer",
	"steelseries aps":                 "SteelSeries",
	"steelseries":                     "SteelSeries",
	"creative technology":             "Creative",
	"creative":                        "Creative",
	"asus":                            "ASUS",
	"asustek computer":                "ASUS",
	"gigabyte technology co., ltd":    "Gigabyte",
	"gigabyte":                        "Gigabyte",
	"msi":                             "MSI",
	"micro-star international":        "MSI",
}

var redundantPhrases = []string{
	"Corporation",
	"Inc.",
	"Inc",
	"Co., Ltd.",
	"Co., Ltd",
	"Co.,Ltd.",
	"Ltd.",
	"Ltd",
	"LLC",
	"(different samples)",
	"[different samples]",
}

var bracketPattern = regexp.MustCompile(`\[([^\]]+)\]`)

// ShortenDeviceName prefers pci.ids bracket product names over long family strings.
func ShortenDeviceName(fullName, vendor, product string) string {
	if fullName == "" {
		if product != "" {
			return product
		}
		return "Unknown Device"
	}

	name := fullName
	brackets := bracketPattern.FindAllStringSubmatch(name, -1)
	shortVendor := getShortVendor(vendor, name)

	productName := ""
	for _, match := range brackets {
		content := strings.TrimSpace(match[1])
		contentLower := strings.ToLower(content)
		if contentLower == "amd" || contentLower == "amd/ati" || contentLower == "ati" {
			continue
		}
		if productName == "" {
			productName = content
		} else if strings.Contains(contentLower, "radeon") ||
			strings.Contains(contentLower, "geforce") ||
			strings.Contains(contentLower, "ethernet") ||
			strings.Contains(contentLower, "wifi") ||
			strings.Contains(contentLower, "wireless") ||
			strings.Contains(contentLower, "controller") ||
			strings.Contains(contentLower, "nvme") ||
			strings.Contains(contentLower, "ssd") {
			productName = content
		}
	}

	if productName != "" {
		productName = cleanProductName(productName)
		if shortVendor != "" && !strings.HasPrefix(strings.ToLower(productName), strings.ToLower(shortVendor)) {
			return shortVendor + " " + productName
		}
		return productName
	}

	name = cleanFullName(name, shortVendor)
	if name == "" || name == shortVendor {
		return fullName
	}
	return name
}

func getShortVendor(vendor, fullName string) string {
	if vendor != "" {
		vendorLower := strings.ToLower(vendor)
		for longName, shortName := range vendorShortNames {
			if strings.Contains(vendorLower, longName) {
				return shortName
			}
		}
	}

	fullNameLower := strings.ToLower(fullName)
	for longName, shortName := range vendorShortNames {
		if strings.Contains(fullNameLower, longName) {
			return shortName
		}
	}

	if vendor != "" {
		v := vendor
		for _, phrase := range redundantPhrases {
			v = strings.ReplaceAll(v, phrase, "")
		}
		return strings.TrimSpace(v)
	}

	return ""
}

func cleanProductName(name string) string {
	for _, phrase := range redundantPhrases {
		name = strings.ReplaceAll(name, phrase, "")
	}
	name = regexp.MustCompile(`\s*/\s*`).ReplaceAllString(name, "/")
	return strings.Join(strings.Fields(name), " ")
}

func cleanFullName(name, shortVendor string) string {
	name = regexp.MustCompile(`\s*\[(AMD|ATI|AMD/ATI)\]\s*`).ReplaceAllString(name, " ")

	nameLower := strings.ToLower(name)
	for longName := range vendorShortNames {
		if idx := strings.Index(nameLower, longName); idx != -1 {
			name = name[:idx] + name[idx+len(longName):]
			nameLower = strings.ToLower(name)
		}
	}

	for _, phrase := range redundantPhrases {
		name = strings.ReplaceAll(name, phrase, "")
		name = strings.ReplaceAll(name, strings.ToLower(phrase), "")
	}

	name = strings.Join(strings.Fields(name), " ")
	name = strings.Trim(name, " -,:")

	if shortVendor != "" && !strings.HasPrefix(strings.ToLower(name), strings.ToLower(shortVendor)) {
		name = shortVendor + " " + name
	}

	return name
}

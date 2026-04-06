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

// It preserves all variant information (e.g., "RX 7600/7600 XT").
func ShortenDeviceName(fullName, vendor, product string) string {
	if fullName == "" {
		if product != "" {
			return product
		}
		return "Unknown Device"
	}

	name := fullName

	// Extract content from brackets - often contains the actual product name
	brackets := bracketPattern.FindAllStringSubmatch(name, -1)

	// Find the short vendor name
	shortVendor := getShortVendor(vendor, name)

	// Try to extract the product name from brackets
	productName := ""
	for _, match := range brackets {
		content := strings.TrimSpace(match[1])
		// Skip vendor-only brackets like [AMD] or [AMD/ATI]
		contentLower := strings.ToLower(content)
		if contentLower == "amd" || contentLower == "amd/ati" || contentLower == "ati" {
			continue
		}
		// This is likely the product name
		if productName == "" {
			productName = content
		} else {
			// If we have multiple bracket contents, the later one is usually more specific
			// e.g., "Cezanne [Radeon Vega Series / Radeon Vega Mobile Series]"
			// We want "Radeon Vega Series / Radeon Vega Mobile Series"
			if strings.Contains(contentLower, "radeon") ||
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
	}

	// If we found a good product name in brackets, use it
	if productName != "" {
		// Clean up the product name
		productName = cleanProductName(productName)

		// Combine with vendor
		if shortVendor != "" && !strings.HasPrefix(strings.ToLower(productName), strings.ToLower(shortVendor)) {
			return shortVendor + " " + productName
		}
		return productName
	}

	// Fall back to cleaning the full name
	name = cleanFullName(name, shortVendor)

	if name == "" || name == shortVendor {
		// Last resort: use the original
		return fullName
	}

	return name
}

func getShortVendor(vendor, fullName string) string {
	// Try the vendor field first
	if vendor != "" {
		vendorLower := strings.ToLower(vendor)
		for longName, shortName := range vendorShortNames {
			if strings.Contains(vendorLower, longName) {
				return shortName
			}
		}
	}

	// Try to find vendor in the full name
	fullNameLower := strings.ToLower(fullName)
	for longName, shortName := range vendorShortNames {
		if strings.Contains(fullNameLower, longName) {
			return shortName
		}
	}

	// Return cleaned vendor if no mapping found
	if vendor != "" {
		// Remove common suffixes
		v := vendor
		for _, phrase := range redundantPhrases {
			v = strings.ReplaceAll(v, phrase, "")
		}
		return strings.TrimSpace(v)
	}

	return ""
}

func cleanProductName(name string) string {
	// Remove redundant phrases
	for _, phrase := range redundantPhrases {
		name = strings.ReplaceAll(name, phrase, "")
	}

	// Normalize slashes for variants (ensure spaces around /)
	name = regexp.MustCompile(`\s*/\s*`).ReplaceAllString(name, "/")

	// Clean up extra whitespace
	name = strings.Join(strings.Fields(name), " ")

	return name
}

func cleanFullName(name, shortVendor string) string {
	// Remove bracket contents that are just vendor names
	name = regexp.MustCompile(`\s*\[(AMD|ATI|AMD/ATI)\]\s*`).ReplaceAllString(name, " ")

	// Remove long vendor names
	nameLower := strings.ToLower(name)
	for longName := range vendorShortNames {
		if idx := strings.Index(nameLower, longName); idx != -1 {
			name = name[:idx] + name[idx+len(longName):]
			nameLower = strings.ToLower(name)
		}
	}

	// Remove redundant phrases
	for _, phrase := range redundantPhrases {
		name = strings.ReplaceAll(name, phrase, "")
		name = strings.ReplaceAll(name, strings.ToLower(phrase), "")
	}

	// Clean up
	name = strings.Join(strings.Fields(name), " ")
	name = strings.Trim(name, " -,:")

	// Prepend short vendor if not already present
	if shortVendor != "" && !strings.HasPrefix(strings.ToLower(name), strings.ToLower(shortVendor)) {
		name = shortVendor + " " + name
	}

	return name
}

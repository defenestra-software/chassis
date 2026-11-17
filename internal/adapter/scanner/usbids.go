package scanner

import (
	"encoding/json"

	"defenestra.io/chassis/assets"
	"defenestra.io/chassis/internal/core/domain"
)

type usbDeviceEntry struct {
	Name  string             `json:"name"`
	Class domain.DeviceClass `json:"class"`
}

var usbDeviceDB = loadUSBDeviceDB(assets.USBIDsJSON)

func loadUSBDeviceDB(data []byte) map[string]map[string]usbDeviceEntry {
	var db map[string]map[string]usbDeviceEntry
	if err := json.Unmarshal(data, &db); err != nil {
		return nil
	}
	return db
}

// lookupUSBDevice expects lowercase hex IDs, as sysfs reports them.
func lookupUSBDevice(vendorID, productID string) (usbDeviceEntry, bool) {
	entry, ok := usbDeviceDB[vendorID][productID]
	return entry, ok
}

package scanner

import (
	"regexp"
	"testing"

	"defenestra.io/chassis/assets"
	"defenestra.io/chassis/internal/core/domain"
)

func TestUSBDeviceDBEntries(t *testing.T) {
	db := loadUSBDeviceDB(assets.USBIDsJSON)
	if len(db) == 0 {
		t.Fatal("usb_ids.json did not parse or is empty")
	}

	classes := map[domain.DeviceClass]bool{
		"":                               true, // unset: descriptor heuristics decide
		domain.DeviceClassNetwork:        true,
		domain.DeviceClassStorage:        true,
		domain.DeviceClassAudio:          true,
		domain.DeviceClassUSBController:  true,
		domain.DeviceClassInput:          true,
		domain.DeviceClassGameController: true,
		domain.DeviceClassSerial:         true,
		domain.DeviceClassWireless:       true,
		domain.DeviceClassCamera:         true,
		domain.DeviceClassPrinter:        true,
		domain.DeviceClassHub:            true,
		domain.DeviceClassSecurity:       true,
		domain.DeviceClassDisplay:        true,
		domain.DeviceClassSensor:         true,
		domain.DeviceClassPower:          true,
		domain.DeviceClassLighting:       true,
		domain.DeviceClassPortable:       true,
		domain.DeviceClassVRHeadset:      true,
		domain.DeviceClassDebug:          true,
		domain.DeviceClassOther:          true,
	}
	hexID := regexp.MustCompile(`^[0-9a-f]{4}$`)

	for vendorID, products := range db {
		if !hexID.MatchString(vendorID) {
			t.Errorf("vendor ID %q is not 4 lowercase hex digits", vendorID)
		}
		for productID, entry := range products {
			id := vendorID + ":" + productID
			if !hexID.MatchString(productID) {
				t.Errorf("%s: product ID is not 4 lowercase hex digits", id)
			}
			if entry.Name == "" && entry.Class == "" {
				t.Errorf("%s: entry sets neither name nor class", id)
			}
			if !classes[entry.Class] {
				t.Errorf("%s: unknown class %q", id, entry.Class)
			}
		}
	}
}

func TestLookupUSBDevice(t *testing.T) {
	entry, ok := lookupUSBDevice("31e3", "1300")
	if !ok || entry.Name != "Wooting 60HE" || entry.Class != domain.DeviceClassInput {
		t.Errorf("31e3:1300 = %+v, %v", entry, ok)
	}

	// Update modes carry a name only.
	entry, ok = lookupUSBDevice("31e3", "130f")
	if !ok || entry.Class != "" {
		t.Errorf("31e3:130f = %+v, %v", entry, ok)
	}

	if _, ok := lookupUSBDevice("ffff", "ffff"); ok {
		t.Error("unknown device found")
	}
}

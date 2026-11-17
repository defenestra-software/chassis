package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"defenestra.io/chassis/internal/core/domain"
)

type fakeUSBInterface struct {
	class    string
	subClass string
	joystick string // path under the interface holding a js node
}

func writeFakeUSBDevice(t *testing.T, vendorID, productID, product string, ifaces []fakeUSBInterface) string {
	t.Helper()
	dev := filepath.Join(t.TempDir(), "1-1")
	write := func(path, content string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(dev, "idVendor"), vendorID)
	write(filepath.Join(dev, "idProduct"), productID)
	write(filepath.Join(dev, "product"), product)
	write(filepath.Join(dev, "bDeviceClass"), "00")
	for i, iface := range ifaces {
		dir := filepath.Join(dev, "1-1:1."+string(rune('0'+i)))
		write(filepath.Join(dir, "bInterfaceClass"), iface.class)
		write(filepath.Join(dir, "bInterfaceSubClass"), iface.subClass)
		if iface.joystick != "" {
			write(filepath.Join(dir, iface.joystick, "js0", "uevent"), "")
		}
	}
	return dev
}

func TestUSBDeviceClassification(t *testing.T) {
	const (
		hidJoystick  = "0003:0000:0000.0001/input/input5"
		xpadJoystick = "input/input5"
	)

	tests := []struct {
		name               string
		vendorID           string
		productID          string
		product            string
		ifaces             []fakeUSBInterface
		wantClass          domain.DeviceClass
		wantName           string
		wantDebug          bool
		wantGameController bool
	}{
		{
			name:     "HID pad with audio interfaces",
			vendorID: "054c", productID: "0ce6", product: "DualSense Wireless Controller",
			ifaces: []fakeUSBInterface{
				{class: "01"}, {class: "01"}, {class: "03", joystick: hidJoystick},
			},
			wantClass: domain.DeviceClassGameController, wantName: "DualSense Wireless Controller",
			wantGameController: true,
		},
		{
			name:     "XInput pad before the driver binds",
			vendorID: "046d", productID: "c21d", product: "Gamepad F310",
			ifaces:    []fakeUSBInterface{{class: "ff", subClass: "5d"}},
			wantClass: domain.DeviceClassGameController, wantName: "Gamepad F310",
			wantGameController: true,
		},
		{
			name:     "keyboard with XInput mode stays a keyboard",
			vendorID: "31e3", productID: "1300", product: "WootingKB",
			ifaces: []fakeUSBInterface{
				{class: "03"}, {class: "ff", subClass: "5d", joystick: xpadJoystick},
			},
			wantClass: domain.DeviceClassInput, wantName: "Wooting 60HE",
			wantGameController: true,
		},
		{
			name:     "listed keyboard with a WebUSB interface",
			vendorID: "3297", productID: "1969", product: "Moonlander Mark I",
			ifaces:    []fakeUSBInterface{{class: "03"}, {class: "ff"}},
			wantClass: domain.DeviceClassInput, wantName: "ZSA Moonlander",
		},
		{
			name:     "listed controller with only a vendor interface",
			vendorID: "18d1", productID: "9400", product: "Stadia Controller rev. A",
			ifaces:    []fakeUSBInterface{{class: "ff"}},
			wantClass: domain.DeviceClassGameController, wantName: "Stadia Controller rev. A",
		},
		{
			name:     "Arduino Leonardo is not a UPS",
			vendorID: "2341", productID: "8036", product: "Arduino Leonardo",
			ifaces:    []fakeUSBInterface{{class: "02"}, {class: "0a"}, {class: "03"}},
			wantClass: domain.DeviceClassInput, wantName: "Arduino Leonardo",
		},
		{
			name:     "listed UPS",
			vendorID: "051d", productID: "0002", product: "Back-UPS",
			ifaces:    []fakeUSBInterface{{class: "03"}},
			wantClass: domain.DeviceClassPower, wantName: "Back-UPS",
		},
		{
			name:     "listed RGB controller with a vendor interface",
			vendorID: "26ce", productID: "01a2", product: "LED Controller",
			ifaces:    []fakeUSBInterface{{class: "ff"}},
			wantClass: domain.DeviceClassLighting, wantName: "LED Controller",
		},
		{
			name:     "pid.codes joystick is not a security key",
			vendorID: "1209", productID: "4f54", product: "EdgeTX Radio",
			ifaces:    []fakeUSBInterface{{class: "03", joystick: hidJoystick}},
			wantClass: domain.DeviceClassGameController, wantName: "EdgeTX Radio",
			wantGameController: true,
		},
		{
			name:     "listed pid.codes security key",
			vendorID: "1209", productID: "beee", product: "Solo 2",
			ifaces:    []fakeUSBInterface{{class: "03"}},
			wantClass: domain.DeviceClassSecurity, wantName: "SoloKeys Solo 2",
		},
		{
			name:     "listed headset from a phone vendor",
			vendorID: "0bb4", productID: "2c87", product: "HTC Vive",
			ifaces:    []fakeUSBInterface{{class: "ff"}},
			wantClass: domain.DeviceClassVRHeadset, wantName: "HTC Vive",
		},
		{
			name:     "unlisted probe keeps its debug port",
			vendorID: "1366", productID: "0101", product: "J-Link",
			ifaces:    []fakeUSBInterface{{class: "ff"}},
			wantClass: domain.DeviceClassOther, wantName: "J-Link", wantDebug: true,
		},
	}

	s := NewGHWScanner("")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dev := writeFakeUSBDevice(t, tt.vendorID, tt.productID, tt.product, tt.ifaces)
			node := s.usbDeviceToNode(dev, "1-1")
			if node.Class != tt.wantClass {
				t.Errorf("class = %q, want %q", node.Class, tt.wantClass)
			}
			if node.Name != tt.wantName {
				t.Errorf("name = %q, want %q", node.Name, tt.wantName)
			}
			if node.HasDebugPort != tt.wantDebug {
				t.Errorf("HasDebugPort = %v, want %v", node.HasDebugPort, tt.wantDebug)
			}
			if node.HasGameController != tt.wantGameController {
				t.Errorf("HasGameController = %v, want %v", node.HasGameController, tt.wantGameController)
			}
		})
	}
}

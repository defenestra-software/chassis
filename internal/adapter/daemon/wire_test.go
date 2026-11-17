package daemon

import (
	"encoding/json"
	"strings"
	"testing"

	"defenestra.io/chassis/internal/core/domain"
)

func TestSnakeCase(t *testing.T) {
	cases := map[string]string{
		"ID":            "id",
		"ParentID":      "parent_id",
		"IOMMUGroup":    "iommu_group",
		"ClassCode":     "class_code",
		"GPUInfo":       "gpu_info",
		"VRAMTotal":     "vram_total",
		"PCIeLinkSpeed": "pcie_link_speed",
		"CacheL1d":      "cache_l1d",
		"WidthMM":       "width_mm",
		"SMART":         "smart",
		"HasDebugPort":  "has_debug_port",
		"MaxFreqMHz":    "max_freq_mhz",
		"EDIDVersion":   "edid_version",
	}
	for in, want := range cases {
		if got := snakeCase(in); got != want {
			t.Errorf("snakeCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEncodeDeviceTree(t *testing.T) {
	root := &domain.DeviceNode{
		ID:         "root",
		IOMMUGroup: -1,
		Children: []*domain.DeviceNode{{
			ID:         "pci:0000:01:00.0",
			ParentID:   "root",
			Driver:     "amdgpu",
			Type:       domain.DeviceTypePCI,
			Class:      domain.DeviceClassGPU,
			IOMMUGroup: 14,
			GPUInfo: &domain.GPUInfo{
				Monitors: []*domain.Monitor{{Type: domain.ConnectorDisplayPort, Status: domain.ConnectionConnected}},
			},
		}},
	}

	data, err := encodeDeviceTree(root, true)
	if err != nil {
		t.Fatal(err)
	}

	var got struct {
		ID       string `json:"id"`
		Children []struct {
			ParentID   string  `json:"parent_id"`
			Driver     string  `json:"driver"`
			Type       string  `json:"type"`
			Class      string  `json:"class"`
			IOMMUGroup int     `json:"iommu_group"`
			Children   []any   `json:"children"`
			Storage    *string `json:"storage_info"`
			GPUInfo    struct {
				Monitors []struct {
					Type   string `json:"type"`
					Status string `json:"status"`
				} `json:"monitors"`
			} `json:"gpu_info"`
		} `json:"children"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, data)
	}

	if len(got.Children) != 1 {
		t.Fatalf("children = %d, want 1", len(got.Children))
	}
	gpu := got.Children[0]
	if gpu.ParentID != "root" || gpu.Driver != "amdgpu" || gpu.Type != "pci" || gpu.Class != "gpu" || gpu.IOMMUGroup != 14 {
		t.Errorf("top-level fields wrong: %+v", gpu)
	}
	if gpu.Children == nil {
		t.Error("nil children should encode as []")
	}
	if gpu.Storage != nil {
		t.Error("nil StorageInfo should be omitted")
	}
	if len(gpu.GPUInfo.Monitors) != 1 || gpu.GPUInfo.Monitors[0].Type != "DisplayPort" || gpu.GPUInfo.Monitors[0].Status != "Connected" {
		t.Errorf("enum names wrong: %+v", gpu.GPUInfo.Monitors)
	}
}

func TestEncodeDeviceTreeIdentifiers(t *testing.T) {
	root := &domain.DeviceNode{
		ID:              "root",
		MotherboardInfo: &domain.MotherboardInfo{Vendor: "ASRock", SerialNumber: "M80-ABC"},
		Children: []*domain.DeviceNode{{
			ID:          "pci:0000:04:00.0",
			NetworkInfo: &domain.NetworkInfo{InterfaceName: "wlan0", MACAddress: "aa:bb:cc:dd:ee:ff", SSID: "home"},
			StorageInfo: &domain.StorageInfo{Model: "SN850", Serial: "2309A", SMART: &domain.SMARTData{Healthy: true, RawJSON: "{\"serial_number\":\"2309A\"}"}},
		}},
	}

	full, err := encodeDeviceTree(root, true)
	if err != nil {
		t.Fatal(err)
	}
	redacted, err := encodeDeviceTree(root, false)
	if err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{`"serial_number":"M80-ABC"`, `"mac_address"`, `"ssid"`, `"serial":"2309A"`, `"raw_json"`} {
		if !strings.Contains(string(full), key) {
			t.Errorf("full output missing %s", key)
		}
		if strings.Contains(string(redacted), key) {
			t.Errorf("redacted output still has %s", key)
		}
	}
	for _, key := range []string{`"vendor":"ASRock"`, `"interface_name":"wlan0"`, `"model":"SN850"`, `"healthy":true`} {
		if !strings.Contains(string(redacted), key) {
			t.Errorf("redacted output lost %s", key)
		}
	}
}

package domain

import "testing"

func TestShortenDeviceName(t *testing.T) {
	tests := []struct {
		name     string
		fullName string
		vendor   string
		product  string
		want     string
	}{
		{
			name:     "empty name returns product",
			fullName: "",
			vendor:   "",
			product:  "Some Product",
			want:     "Some Product",
		},
		{
			name:     "empty name and product returns Unknown",
			fullName: "",
			vendor:   "",
			product:  "",
			want:     "Unknown Device",
		},
		{
			name:     "AMD GPU with bracket product name",
			fullName: "Navi 33 [Radeon RX 7600/7600 XT/7600M XT/7600S]",
			vendor:   "Advanced Micro Devices, Inc.",
			product:  "",
			want:     "AMD Radeon RX 7600/7600 XT/7600M XT/7600S",
		},
		{
			name:     "Intel controller",
			fullName: "Intel Corporation Device 7a24",
			vendor:   "Intel Corporation",
			product:  "",
			want:     "Intel Device 7a24",
		},
		{
			name:     "NVIDIA GPU",
			fullName: "NVIDIA Corporation GA106 [GeForce RTX 3060]",
			vendor:   "NVIDIA Corporation",
			product:  "",
			want:     "NVIDIA GeForce RTX 3060",
		},
		{
			name:     "Realtek ethernet in brackets",
			fullName: "Realtek Semiconductor Co., Ltd. RTL8125 2.5GbE [Ethernet Controller]",
			vendor:   "Realtek Semiconductor Co., Ltd.",
			product:  "",
			want:     "Realtek Ethernet Controller",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShortenDeviceName(tt.fullName, tt.vendor, tt.product)
			if got != tt.want {
				t.Errorf("ShortenDeviceName(%q, %q, %q) = %q, want %q",
					tt.fullName, tt.vendor, tt.product, got, tt.want)
			}
		})
	}
}

func TestGetShortVendor(t *testing.T) {
	tests := []struct {
		name     string
		vendor   string
		fullName string
		want     string
	}{
		{"AMD from vendor", "Advanced Micro Devices, Inc.", "", "AMD"},
		{"Intel from vendor", "Intel Corporation", "", "Intel"},
		{"NVIDIA from vendor", "NVIDIA Corporation", "", "NVIDIA"},
		{"Realtek from vendor", "Realtek Semiconductor Co., Ltd.", "", "Realtek"},
		{"AMD from fullName", "", "Some [AMD/ATI] Device", "AMD"},
		{"unknown vendor cleaned", "Some Company Inc.", "", "Some Company"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getShortVendor(tt.vendor, tt.fullName)
			if got != tt.want {
				t.Errorf("getShortVendor(%q, %q) = %q, want %q",
					tt.vendor, tt.fullName, got, tt.want)
			}
		})
	}
}

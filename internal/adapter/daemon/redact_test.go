package daemon

import (
	"strings"
	"testing"
)

func TestWithoutIdentifiers(t *testing.T) {
	tests := []struct {
		name string
		in   string
		gone []string
		kept []string
	}{
		{
			name: "system info",
			in:   `{"board_serial":"M80-ABC","memory_modules":[{"label":"DIMM_A1","size":"16 GB","serial":"12AB34CD","part":"F5-6000"}]}`,
			gone: []string{"M80-ABC", "12AB34CD", "board_serial", `"serial"`},
			kept: []string{`"label":"DIMM_A1"`, `"size":"16 GB"`, `"part":"F5-6000"`},
		},
		{
			name: "smartctl",
			in: `{"model_name":"SN850","serial_number":"2309A","wwn":{"naa":5,"oui":6980,"id":123456},` +
				`"nvme_namespaces":[{"id":1,"eui64":{"oui":6980,"ext_id":987654}}],"logical_unit_id":"0x5000",` +
				`"temperature":{"current":41},"smart_status":{"passed":true},"data_units_written":18446744073709551615}`,
			gone: []string{"2309A", "wwn", "123456", "eui64", "987654", "logical_unit_id"},
			kept: []string{`"model_name":"SN850"`, `"current":41`, `"passed":true`, `18446744073709551615`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := withoutIdentifiers(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range tt.gone {
				if strings.Contains(out, s) {
					t.Errorf("output still has %s: %s", s, out)
				}
			}
			for _, s := range tt.kept {
				if !strings.Contains(out, s) {
					t.Errorf("output lost %s: %s", s, out)
				}
			}
		})
	}
}

func TestWithoutIdentifiersRejectsInvalidJSON(t *testing.T) {
	if out, err := withoutIdentifiers("serial_number: 2309A"); err == nil {
		t.Errorf("want error, got %q", out)
	}
}

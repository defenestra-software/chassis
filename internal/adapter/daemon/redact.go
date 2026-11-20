package daemon

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Keys in ReadSystemInfo and smartctl JSON that identify one unit of hardware.
var identifierKeys = map[string]bool{
	"board_serial":    true,
	"serial":          true,
	"serial_number":   true,
	"wwn":             true,
	"eui64":           true,
	"nguid":           true,
	"logical_unit_id": true,
}

func withoutIdentifiers(jsonData string) (string, error) {
	dec := json.NewDecoder(bytes.NewReader([]byte(jsonData)))
	dec.UseNumber() // smartctl counters exceed float64 precision
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return "", fmt.Errorf("parsing JSON to remove identifiers: %w", err)
	}
	out, err := json.Marshal(dropIdentifierKeys(doc))
	if err != nil {
		return "", fmt.Errorf("encoding JSON without identifiers: %w", err)
	}
	return string(out), nil
}

func dropIdentifierKeys(v any) any {
	switch v := v.(type) {
	case map[string]any:
		for key, child := range v {
			if identifierKeys[key] {
				delete(v, key)
				continue
			}
			v[key] = dropIdentifierKeys(child)
		}
	case []any:
		for i, child := range v {
			v[i] = dropIdentifierKeys(child)
		}
	}
	return v
}

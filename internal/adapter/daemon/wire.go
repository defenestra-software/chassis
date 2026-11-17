package daemon

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"defenestra.io/chassis/internal/core/domain"
)

// SMART RawJSON can contain a drive serial number and must be omitted with other identifiers.
var identifierFields = map[string]bool{
	"Serial":       true,
	"SerialNumber": true,
	"MACAddress":   true,
	"SSID":         true,
	"UniqueID":     true,
	"RawJSON":      true,
}

// The JSON format uses snake_case keys, omits nil info blocks and encodes nil slices as [].
func encodeDeviceTree(root *domain.DeviceNode, withIdentifiers bool) ([]byte, error) {
	enc := wireEncoder{withIdentifiers: withIdentifiers}
	return json.Marshal(enc.value(reflect.ValueOf(root)))
}

type wireEncoder struct {
	withIdentifiers bool
}

func (e wireEncoder) value(v reflect.Value) any {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return nil
		}
		return e.value(v.Elem())
	case reflect.Struct:
		t := v.Type()
		out := make(map[string]any, t.NumField())
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			if !e.withIdentifiers && identifierFields[f.Name] {
				continue
			}
			fv := v.Field(i)
			if fv.Kind() == reflect.Pointer && fv.IsNil() {
				continue
			}
			out[snakeCase(f.Name)] = e.value(fv)
		}
		return out
	case reflect.Slice, reflect.Array:
		out := make([]any, v.Len())
		for i := range out {
			out[i] = e.value(v.Index(i))
		}
		return out
	default:
		// Numeric enums with String methods are encoded as names.
		if v.Kind() != reflect.String {
			if s, ok := v.Interface().(fmt.Stringer); ok {
				return s.String()
			}
		}
		return v.Interface()
	}
}

// Acronyms stay together, including a single trailing lowercase letter (PCIe, MHz).
func snakeCase(name string) string {
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c >= 'A' && c <= 'Z' {
			if i > 0 && (isLowerOrDigit(name[i-1]) || startsWord(name[i+1:])) {
				b.WriteByte('_')
			}
			c += 'a' - 'A'
		}
		b.WriteByte(c)
	}
	return b.String()
}

func startsWord(rest string) bool {
	return len(rest) >= 2 && isLowerOrDigit(rest[0]) && isLowerOrDigit(rest[1])
}

func isLowerOrDigit(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
}

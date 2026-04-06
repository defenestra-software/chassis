package assets

import _ "embed"

// DeviceIDsJSON contains GPU marketing names keyed by PCI IDs.
// Sourced from LACT project (https://github.com/ilya-zlobintsev/LACT).
//
//go:embed device_ids.json
var DeviceIDsJSON []byte

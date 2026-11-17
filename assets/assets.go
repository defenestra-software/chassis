package assets

import _ "embed"

// DeviceIDsJSON contains GPU marketing names keyed by PCI IDs.
// Sourced from LACT project (https://github.com/ilya-zlobintsev/LACT).
//
//go:embed device_ids.json
var DeviceIDsJSON []byte

// USBIDsJSON maps USB vendor ID and product ID to a display name and an
// optional device class. Class override heuristics.
//
//go:embed usb_ids.json
var USBIDsJSON []byte

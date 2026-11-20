# Changelog

## Unreleased

- Added a per-device USB list (`assets/usb_ids.json`) that sets the name and class of known devices by vendor and product ID.
- Added Wooting keyboards, ZSA Moonlander, ErgoDox EZ, and Framework Laptop 16 input modules to the USB list.
- Added a Game Controllers category for gamepads, wheels, and joysticks, split out of Input Devices.
- Added a VR Headsets category with HTC Vive, Valve SteamVR, Bigscreen, and PlayStation VR hardware.
- Added `ListDevices` to the D-Bus interface to return the device tree as JSON, including GPU, storage, and system information without serial numbers, MAC addresses, `/dev/*/by-id` paths, and other identifying fields.
- Added `ListDevicesWithIdentifiers`, `ReadSystemInfoWithIdentifiers`, and `ReadSMARTWithIdentifiers` to return the full data with identifying fields.
- Added the polkit action `read-identifiers` for the `WithIdentifiers` methods; an active local session is not asked for a password.
- Changed `ReadSystemInfo` and `ReadSMART` to leave out serial numbers, WWN, and NVMe unique IDs.
- Moved the RGB controller, UPS, and gamepad tables into the USB list.
- Moved Rescan Devices from the sidebar header into the menu and added `F5` / `Ctrl+R` for it.
- Moved the search button into the sidebar header.
- Removed the Forum link from the menu and the About dialog.
- Fixed the window button layout being forced.
- Fixed DualSense and DualShock controllers showing up as audio devices.
- Fixed Arduino Leonardo showing up as a UPS.
- Fixed every HID device with a pid.codes vendor ID (`1209`) showing up as a security key.
- Fixed listed keyboards with a WebUSB interface showing up as debug devices.

## v0.1.6
- Added support for Logitech F310
- Added support for Stadia controllers USB and BLE interfaces

## v0.1.5
- Fixed D-Bus activation issue that resulted in the daemon not responding on first launch 

## v0.1.4
- Fixed memory size inconsistencies
- Fixed race condition in readLoop
- Fixed CHASSIS_DEBUG variable from being enabled unless actually set to true
- Fixed detail page refresh issue after operation
- Added PolicyKit policies on root operation

## v0.1.3
- Added VirtIO device classification
- Updated to go-flatbridge v0.0.4 to fix flatbox false positive detection
- Hid service setup button on RPM installations

## v0.1.2
- Added ASRock AMD Radeon RX 9070 XT Steel Legend 16GB for GPU classification
- Added ASRock LED Controller (Polychrome RGB)
- Added RPM build

## v0.1.1
- Bugfix misclassification of ASUS AURA LED Controller
- Bugfix misclassification of Cyberpower and other UPS
- Added battery information using UPower to the power device details page with live monitoring
- Clamped root device pages to make the appearance more unified
- Enable system service to be optional and skipped if the user does not want to install it.

## v0.1.0
- PCI and USB device scanning with full bus topology
- GPU details: VRAM, clocks, PCIe link speed/width, VBIOS version, VRAM vendor
- Display and monitor info from EDID: model, resolution, refresh rate, physical size
- Storage drives with capacity, model, firmware, and NVMe temperature
- SMART health data via system daemon
- Live sensor readings: temperature, fan speed, voltage, power
- CPU info: cores, threads, cache hierarchy, frequencies, flags
- Memory modules from SMBIOS with vendor inference from part numbers
- Motherboard and BIOS information
- Network interface details: link speed, state, duplex, wireless/Bluetooth detection
- Camera detection with /dev/video paths
- Serial port detection (ttyACM, ttyUSB)
- USB debug probe detection (JTAG/SWD)
- Security key recognition (YubiKey, Nitrokey, FIDO keys)
- RGB controller classification (sourced from OpenRGB)
- Portable device detection (phones, tablets)
- Bluetooth HID peripherals with battery monitoring
- IOMMU group browser
- Driver management: bind, unbind, driver probe, modprobe/rmmod
- VBIOS dump for GPU passthrough
- Live hotplug via udev/netlink
- Search across all devices
- Adaptive GTK4/Adwaita layout with split view
- Flatpak packaging with Flatbox for privileged daemon access

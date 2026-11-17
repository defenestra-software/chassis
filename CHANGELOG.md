# Changelog

## Unreleased

- Added `ListDevices` to the D-Bus interface to return the device tree as JSON, including GPU, storage, and system information without Serial numbers, MAC addresses, and other identifying fields.
- Added `ListDevicesWithIdentifiers` to return the same tree with identifying fields after polkit authorization.

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

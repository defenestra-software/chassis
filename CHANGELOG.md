# Changelog

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

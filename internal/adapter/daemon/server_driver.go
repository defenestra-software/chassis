package daemon

import (
	"fmt"
	"os"

	"github.com/godbus/dbus/v5"

	flatpak "gitlab.com/defenestra-software/go-flatbridge"

	"defenestra.io/chassis/internal/core/domain"
)

func (s *Server) Unbind(sender dbus.Sender, deviceID, busType string) *dbus.Error {
	s.logger.Info("unbind requested", "device", deviceID, "bus", busType)

	if dberr := s.authorize(sender, actionManageDrivers); dberr != nil {
		return dberr
	}

	if err := s.validateDeviceID(deviceID); err != nil {
		return dbus.MakeFailedError(err)
	}
	if err := s.validateBusType(busType); err != nil {
		return dbus.MakeFailedError(err)
	}

	driverLink := fmt.Sprintf("/sys/bus/%s/devices/%s/driver", busType, deviceID)
	driverLink = flatpak.HostPath(driverLink)
	if _, err := os.Stat(driverLink); os.IsNotExist(err) {
		return dbus.MakeFailedError(domain.ErrDriverNotBound)
	}

	path := fmt.Sprintf("/sys/bus/%s/devices/%s/driver/unbind", busType, deviceID)
	path = flatpak.HostPath(path)

	if err := s.validatePath(path); err != nil {
		return dbus.MakeFailedError(err)
	}

	if err := os.WriteFile(path, []byte(deviceID), 0200); err != nil {
		s.logger.Error("unbind failed", "error", err)
		return dbus.MakeFailedError(domain.ErrDeviceBusy)
	}

	s.logger.Info("unbind successful", "device", deviceID)
	s.EmitDeviceChanged(deviceID, "unbind")
	return nil
}

func (s *Server) Bind(sender dbus.Sender, deviceID, driverName string) *dbus.Error {
	s.logger.Info("bind requested", "device", deviceID, "driver", driverName)

	if dberr := s.authorize(sender, actionManageDrivers); dberr != nil {
		return dberr
	}

	if err := s.validateDeviceID(deviceID); err != nil {
		return dbus.MakeFailedError(err)
	}
	if err := s.validateDriverName(driverName); err != nil {
		return dbus.MakeFailedError(err)
	}

	// Auto-load the kernel module if the driver directory doesn't exist yet.
	driverDir := fmt.Sprintf("/sys/bus/pci/drivers/%s", driverName)
	driverDir = flatpak.HostPath(driverDir)
	if _, err := os.Stat(driverDir); os.IsNotExist(err) {
		s.logger.Info("auto-loading module for driver", "driver", driverName)
		if err := runHostCommand("modprobe", driverName); err != nil {
			s.logger.Warn("modprobe failed, driver may not exist as a module", "driver", driverName, "error", err)
		}
	}

	path := fmt.Sprintf("/sys/bus/pci/drivers/%s/bind", driverName)
	path = flatpak.HostPath(path)

	if err := s.validatePath(path); err != nil {
		return dbus.MakeFailedError(err)
	}

	if err := os.WriteFile(path, []byte(deviceID), 0200); err != nil {
		s.logger.Error("bind failed", "error", err)
		return dbus.MakeFailedError(domain.ErrDeviceBusy)
	}

	s.logger.Info("bind successful", "device", deviceID, "driver", driverName)
	s.EmitDeviceChanged(deviceID, "bind")
	return nil
}

func (s *Server) DriverProbe(sender dbus.Sender, deviceID string) *dbus.Error {
	s.logger.Info("driver probe requested", "device", deviceID)

	if dberr := s.authorize(sender, actionManageDrivers); dberr != nil {
		return dberr
	}

	if err := s.validateDeviceID(deviceID); err != nil {
		return dbus.MakeFailedError(err)
	}

	if pciAddressRegex.MatchString(deviceID) {
		// PCI: write to drivers_probe
		path := flatpak.HostPath("/sys/bus/pci/drivers_probe")
		if err := os.WriteFile(path, []byte(deviceID), 0200); err != nil {
			s.logger.Error("PCI driver probe failed", "error", err)
			return dbus.MakeFailedError(domain.ErrDeviceBusy)
		}
	} else if usbAddressRegex.MatchString(deviceID) {
		// USB: bind to the generic USB driver to trigger re-enumeration
		path := flatpak.HostPath("/sys/bus/usb/drivers/usb/bind")
		if err := os.WriteFile(path, []byte(deviceID), 0200); err != nil {
			s.logger.Error("USB rebind failed", "error", err)
			return dbus.MakeFailedError(domain.ErrDeviceBusy)
		}
	} else {
		return dbus.MakeFailedError(domain.ErrInvalidPath)
	}

	s.logger.Info("driver probe successful", "device", deviceID)
	s.EmitDeviceChanged(deviceID, "probe")
	return nil
}

func (s *Server) LoadModule(sender dbus.Sender, moduleName string) *dbus.Error {
	s.logger.Info("load module requested", "module", moduleName)

	if dberr := s.authorize(sender, actionManageModules); dberr != nil {
		return dberr
	}

	if err := s.validateModuleName(moduleName); err != nil {
		return dbus.MakeFailedError(err)
	}

	if err := runHostCommand("modprobe", moduleName); err != nil {
		s.logger.Error("modprobe failed", "error", err)
		return dbus.MakeFailedError(domain.ErrModuleLoadFailed)
	}

	s.logger.Info("module loaded", "module", moduleName)
	return nil
}

func (s *Server) UnloadModule(sender dbus.Sender, moduleName string) *dbus.Error {
	s.logger.Info("unload module requested", "module", moduleName)

	if dberr := s.authorize(sender, actionManageModules); dberr != nil {
		return dberr
	}

	if err := s.validateModuleName(moduleName); err != nil {
		return dbus.MakeFailedError(err)
	}

	if err := runHostCommand("rmmod", moduleName); err != nil {
		s.logger.Error("rmmod failed", "error", err)
		return dbus.MakeFailedError(domain.ErrModuleLoadFailed)
	}

	s.logger.Info("module unloaded", "module", moduleName)
	return nil
}

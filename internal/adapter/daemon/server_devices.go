package daemon

import (
	"github.com/godbus/dbus/v5"
)

// ListDevices omits identifying fields, including serial numbers and MAC addresses.
func (s *Server) ListDevices() (string, *dbus.Error) {
	s.logger.Debug("device list requested")
	return s.listDevices(false)
}

func (s *Server) ListDevicesWithIdentifiers(sender dbus.Sender) (string, *dbus.Error) {
	s.logger.Info("device list with identifiers requested")

	if dberr := s.authorize(sender, actionReadIdentifiers); dberr != nil {
		return "", dberr
	}
	return s.listDevices(true)
}

func (s *Server) listDevices(withIdentifiers bool) (string, *dbus.Error) {
	tree, err := s.devices.GetDeviceTree()
	if err != nil {
		return "", dbus.MakeFailedError(err)
	}

	data, err := encodeDeviceTree(tree, withIdentifiers)
	if err != nil {
		return "", dbus.MakeFailedError(err)
	}
	return string(data), nil
}

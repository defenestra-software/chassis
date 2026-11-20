package daemon

import (
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"

	flatpak "gitlab.com/defenestra-software/go-flatbridge"

	"defenestra.io/chassis/internal/core/domain"
	"defenestra.io/chassis/internal/core/service"
	"defenestra.io/chassis/internal/shared"
)

const (
	dbusName      = "io.defenestra.Chassis"
	dbusPath      = "/io/defenestra/Chassis"
	dbusInterface = "io.defenestra.Chassis"
)

type Server struct {
	devices *service.DeviceService
	logger  *slog.Logger

	conn *dbus.Conn
}

func NewServer(logger *slog.Logger, devices *service.DeviceService) (*Server, error) {
	return &Server{
		devices: devices,
		logger:  logger,
	}, nil
}

func (s *Server) Start() error {
	conn, err := dbus.SystemBus()
	if err != nil {
		return fmt.Errorf("connecting to system bus: %w", err)
	}
	s.conn = conn

	// Export the object before requesting the name.
	// Under D-Bus activation the bus delivers the triggering method call
	// the instant the name is acquired.
	if err := conn.Export(s, dbus.ObjectPath(dbusPath), dbusInterface); err != nil {
		return fmt.Errorf("exporting object: %w", err)
	}

	introNode := &introspect.Node{
		Name: dbusPath,
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			{
				Name: dbusInterface,
				Methods: []introspect.Method{
					{Name: "Unbind", Args: []introspect.Arg{
						{Name: "device_id", Type: "s", Direction: "in"},
						{Name: "bus_type", Type: "s", Direction: "in"},
					}},
					{Name: "Bind", Args: []introspect.Arg{
						{Name: "device_id", Type: "s", Direction: "in"},
						{Name: "driver_name", Type: "s", Direction: "in"},
					}},
					{Name: "DriverProbe", Args: []introspect.Arg{
						{Name: "device_id", Type: "s", Direction: "in"},
					}},
					{Name: "LoadModule", Args: []introspect.Arg{
						{Name: "module_name", Type: "s", Direction: "in"},
					}},
					{Name: "UnloadModule", Args: []introspect.Arg{
						{Name: "module_name", Type: "s", Direction: "in"},
					}},
					{Name: "Ping", Args: []introspect.Arg{
						{Name: "reply", Type: "s", Direction: "out"},
					}},
					{Name: "ListDevices", Args: []introspect.Arg{
						{Name: "json_data", Type: "s", Direction: "out"},
					}},
					{Name: "ListDevicesWithIdentifiers", Args: []introspect.Arg{
						{Name: "json_data", Type: "s", Direction: "out"},
					}},
					{Name: "ReadSMART", Args: []introspect.Arg{
						{Name: "block_device", Type: "s", Direction: "in"},
						{Name: "json_data", Type: "s", Direction: "out"},
					}},
					{Name: "ReadSMARTWithIdentifiers", Args: []introspect.Arg{
						{Name: "block_device", Type: "s", Direction: "in"},
						{Name: "json_data", Type: "s", Direction: "out"},
					}},
					{Name: "ReadSystemInfo", Args: []introspect.Arg{
						{Name: "json_data", Type: "s", Direction: "out"},
					}},
					{Name: "ReadSystemInfoWithIdentifiers", Args: []introspect.Arg{
						{Name: "json_data", Type: "s", Direction: "out"},
					}},
					{Name: "DumpVBIOS", Args: []introspect.Arg{
						{Name: "pci_address", Type: "s", Direction: "in"},
						{Name: "rom_data", Type: "ay", Direction: "out"},
					}},
				},
				Signals: []introspect.Signal{
					{Name: "DeviceChanged", Args: []introspect.Arg{
						{Name: "device_id", Type: "s"},
						{Name: "action", Type: "s"},
					}},
				},
			},
		},
	}

	if err := conn.Export(
		introspect.NewIntrospectable(introNode),
		dbus.ObjectPath(dbusPath),
		"org.freedesktop.DBus.Introspectable",
	); err != nil {
		return fmt.Errorf("exporting introspectable: %w", err)
	}

	reply, err := conn.RequestName(dbusName, dbus.NameFlagReplaceExisting|dbus.NameFlagAllowReplacement)
	if err != nil {
		return fmt.Errorf("requesting bus name: %w", err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner && reply != dbus.RequestNameReplyAlreadyOwner {
		return fmt.Errorf("bus name %s already taken", dbusName)
	}

	s.logger.Info("D-Bus service started", "name", dbusName, "path", dbusPath)
	return nil
}

func (s *Server) Stop() error {
	if s.conn != nil {
		_, err := s.conn.ReleaseName(dbusName)
		s.conn.Close()
		s.logger.Info("D-Bus service stopped")
		return err
	}
	return nil
}

func (s *Server) EmitDeviceChanged(deviceID, action string) {
	if s.conn == nil {
		return
	}
	s.conn.Emit(dbus.ObjectPath(dbusPath), dbusInterface+".DeviceChanged", deviceID, action)
}

func (s *Server) Ping() (string, *dbus.Error) {
	return "pong", nil
}

var (
	// PCI address: DDDD:BB:DD.F
	pciAddressRegex = regexp.MustCompile(`^[0-9a-fA-F]{4}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}\.[0-9a-fA-F]$`)
	// USB address: bus-port or bus-port.port (e.g. 1-8, 1-1.4, 2-1.3.1)
	usbAddressRegex  = regexp.MustCompile(`^[0-9]+-[0-9]+(\.[0-9]+)*$`)
	safeNameRegex    = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	blockDeviceRegex = regexp.MustCompile(`^[a-z0-9]+$`)
)

func (s *Server) validateDeviceID(deviceID string) error {
	if strings.Contains(deviceID, "..") || strings.Contains(deviceID, "/") {
		return domain.ErrInvalidPath
	}
	if pciAddressRegex.MatchString(deviceID) || usbAddressRegex.MatchString(deviceID) {
		return nil
	}
	return domain.ErrInvalidPath
}

func (s *Server) validateBusType(busType string) error {
	switch busType {
	case "pci", "usb":
		return nil
	default:
		return domain.ErrInvalidPath
	}
}

func (s *Server) validateDriverName(name string) error {
	if !safeNameRegex.MatchString(name) {
		return domain.ErrInvalidPath
	}
	return nil
}

func (s *Server) validateModuleName(name string) error {
	if !safeNameRegex.MatchString(name) {
		return domain.ErrInvalidPath
	}
	return nil
}

func (s *Server) validatePath(path string) error {
	cleanPath := filepath.Clean(path)
	cleanPath = strings.TrimPrefix(cleanPath, "/run/host/root")
	for _, allowed := range shared.AllowedSysfsPaths {
		if strings.HasPrefix(cleanPath, allowed) {
			return nil
		}
	}
	return domain.ErrInvalidPath
}

func runHostCommandOutput(name string, args ...string) ([]byte, error) {
	var cmd *exec.Cmd
	if flatpak.InFlatbox() {
		allArgs := append([]string{"--host", name}, args...)
		cmd = exec.Command("flatpak-spawn", allArgs...)
	} else {
		cmd = exec.Command(name, args...)
	}
	return cmd.CombinedOutput()
}

func runHostCommand(name string, args ...string) error {
	var cmd *exec.Cmd
	if flatpak.InFlatbox() {
		allArgs := append([]string{"--host", name}, args...)
		cmd = exec.Command("flatpak-spawn", allArgs...)
	} else {
		cmd = exec.Command(name, args...)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w (output: %s)", name, err, string(output))
	}
	return nil
}

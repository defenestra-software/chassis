package daemon

import (
	"github.com/godbus/dbus/v5"

	"defenestra.io/chassis/internal/core/domain"
)

// Polkit action IDs. These must match api/io.defenestra.Chassis.policy.
const (
	actionManageDrivers   = "io.defenestra.Chassis.manage-drivers"
	actionManageModules   = "io.defenestra.Chassis.manage-modules"
	actionDumpVBIOS       = "io.defenestra.Chassis.dump-vbios"
	actionReadIdentifiers = "io.defenestra.Chassis.read-identifiers"
)

type polkitSubject struct {
	Kind    string
	Details map[string]dbus.Variant
}

type polkitAuthResult struct {
	IsAuthorized bool
	IsChallenge  bool
	Details      map[string]string
}

func (s *Server) authorize(sender dbus.Sender, action string) *dbus.Error {
	if s.conn == nil {
		return dbus.MakeFailedError(domain.ErrPermissionDenied)
	}

	authority := s.conn.Object("org.freedesktop.PolicyKit1", "/org/freedesktop/PolicyKit1/Authority")

	subject := polkitSubject{
		Kind:    "system-bus-name",
		Details: map[string]dbus.Variant{"name": dbus.MakeVariant(string(sender))},
	}

	var result polkitAuthResult
	// flag 1 = AllowUserInteraction, so Polkit may prompt for admin credentials.
	err := authority.Call(
		"org.freedesktop.PolicyKit1.Authority.CheckAuthorization", 0,
		subject, action, map[string]string{}, uint32(1), "",
	).Store(&result)
	if err != nil {
		s.logger.Error("polkit authorization check failed", "action", action, "error", err)
		return dbus.MakeFailedError(domain.ErrPermissionDenied)
	}

	if !result.IsAuthorized {
		s.logger.Warn("authorization denied", "action", action, "sender", string(sender))
		return dbus.MakeFailedError(domain.ErrPermissionDenied)
	}

	return nil
}

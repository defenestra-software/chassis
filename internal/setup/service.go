//go:build linux

package setup

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	flatpak "gitlab.com/defenestra-software/go-flatbridge"
)

type ServiceStatus int

const (
	ServiceNotInstalled ServiceStatus = iota
	ServiceInstalled
	ServiceRunning
)

const (
	serviceName = "chassis.service"
	unitPath    = "/etc/systemd/system/" + serviceName
	dbusName    = "io.defenestra.Chassis"
	appID       = "io.defenestra.Chassis"
)

// ServiceInfo holds the status of a service and any error encountered.
type ServiceInfo struct {
	Name   string
	Status ServiceStatus
	Error  error
}

// CheckService checks the status of the system daemon.
func CheckService() ServiceInfo {
	info := ServiceInfo{Name: serviceName}

	cmd := flatpak.HostCommand("systemctl", "is-active", "--quiet", serviceName)
	if err := cmd.Run(); err == nil {
		info.Status = ServiceRunning
		return info
	}

	cmd = flatpak.HostCommand("test", "-f", unitPath)
	if err := cmd.Run(); err == nil {
		info.Status = ServiceInstalled
		return info
	}

	info.Status = ServiceNotInstalled
	return info
}

// GetFlatpakAppPath returns the Flatpak application files path, using the
// 'active' symlink so the service continues to work after updates.
func GetFlatpakAppPath() (string, error) {
	data, err := exec.Command("grep", "-oP", `app-path=\K.*`, "/.flatpak-info").Output()
	if err != nil {
		return "", fmt.Errorf("could not read /.flatpak-info: %w", err)
	}
	path := strings.TrimSpace(string(data))

	parts := strings.Split(path, "/")
	for i, part := range parts {
		if len(part) == 64 && i+1 < len(parts) && parts[i+1] == "files" {
			parts[i] = "active"
			break
		}
	}
	return strings.Join(parts, "/"), nil
}

// GetCurrentUser returns the current username.
func GetCurrentUser() string {
	if out, err := exec.Command("whoami").Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}

// InstallService installs and enables the chassis systemd service.
func InstallService() (string, error) {
	if flatpak.InFlatpakEnvironment() {
		return installFlatpakService()
	}
	cmd := exec.Command("sudo", "systemctl", "enable", "--now", serviceName)
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

func installFlatpakService() (string, error) {
	appPath, err := GetFlatpakAppPath()
	if err != nil {
		return "", fmt.Errorf("could not determine Flatpak app path: %w", err)
	}

	rootCmd, err := flatpak.PrivilegeHelper()
	if err != nil {
		return "", err
	}

	user := GetCurrentUser()
	daemonSh := appPath + "/bin/daemon.sh"
	dbusConf := appPath + "/share/dbus-1/system.d/" + dbusName + ".conf"
	polkitPolicy := appPath + "/share/polkit-1/actions/" + dbusName + ".policy"

	unitContent := fmt.Sprintf(`[Unit]
Description=Chassis Device Manager Daemon (via Flatpak)
After=dbus.service
Requires=dbus.service

[Service]
Type=dbus
BusName=%s
Environment=FLATPAK_INSTALL_USER=%s
ExecStartPre=/bin/sh -c '[ -d /var/lib/flatpak/app/%s ] || [ -d /home/%s/.local/share/flatpak/app/%s ] || { systemd-run --no-block sh -c '"'"'sleep 2; systemctl disable %s; rm -f %s /etc/dbus-1/system.d/%s.conf /etc/polkit-1/actions/%s.policy; systemctl daemon-reload; systemctl reset-failed %s'"'"'; exit 1; }'
ExecStart=%s
Restart=on-failure
RestartSec=5
StartLimitIntervalSec=60
StartLimitBurst=5

[Install]
WantedBy=multi-user.target`, dbusName, user, appID, user, appID, serviceName, unitPath, dbusName, dbusName, serviceName, daemonSh)

	installScript := fmt.Sprintf(`cat > %s << 'ENDOFUNIT'
%s
ENDOFUNIT
install -Dm644 %s /etc/dbus-1/system.d/%s.conf
install -Dm644 %s /etc/polkit-1/actions/%s.policy
chcon -t bin_t %s 2>/dev/null || true
systemctl reload dbus 2>/dev/null || true
systemctl daemon-reload
systemctl enable --now %s`, unitPath, unitContent, dbusConf, dbusName, polkitPolicy, dbusName, daemonSh, serviceName)

	rootParts := strings.Fields(rootCmd)
	cmdArgs := append(rootParts, "sh", "-c", installScript)
	cmd := flatpak.HostCommand(cmdArgs[0], cmdArgs[1:]...)
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

// WaitForService waits for the service to become active.
func WaitForService(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cmd := flatpak.HostCommand("systemctl", "is-active", "--quiet", serviceName)
		if cmd.Run() == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s", serviceName)
}

//go:build linux

package gtk

import (
	"time"

	kit "gitlab.com/defenestra-software/go-adw-kit"

	"defenestra.io/chassis/internal/adapter/config"
	"defenestra.io/chassis/internal/setup"
)

// RunSetup launches the setup window and returns 0 on success or 1 on cancellation.
func RunSetup() int {
	optional := false

	return kit.RunSetup(kit.SetupConfig{
		AppID:              "io.defenestra.Chassis.Setup",
		Title:              "Chassis Setup",
		WelcomeIcon:        "system-run-symbolic",
		WelcomeTitle:       "Welcome to Chassis",
		WelcomeDescription: "An optional system service enables driver management for your devices.",
		GroupTitle:         "Optional Service",
		OnSkip: func() {
			config.SaveSkipSetup(true)
		},
		Services: []kit.ServiceRowConfig{
			{
				Title:    "System Service",
				Subtitle: "Enables driver binding, unbinding, and module management",
				Required: &optional,
				Callbacks: kit.ServiceCallbacks{
					Check: func() kit.ServiceStatus {
						info := setup.CheckService()
						switch info.Status {
						case setup.ServiceRunning:
							return kit.ServiceRunning
						case setup.ServiceInstalled:
							return kit.ServiceInstalled
						default:
							return kit.ServiceNotInstalled
						}
					},
					Action: func() (string, error) {
						return setup.InstallService()
					},
					WaitReady: func() error {
						return setup.WaitForService(15 * time.Second)
					},
				},
			},
		},
	})
}

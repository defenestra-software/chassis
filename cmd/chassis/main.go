package main

import (
	"log/slog"
	"os"

	flatpak "gitlab.com/defenestra-software/go-flatbridge"

	"defenestra.io/chassis/internal/adapter/config"
	"defenestra.io/chassis/internal/adapter/ui/common"
	gtkui "defenestra.io/chassis/internal/adapter/ui/gtk"
	"defenestra.io/chassis/internal/setup"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--setup":
			os.Exit(gtkui.RunSetup())
		case "--check":
			os.Exit(checkDaemon())
		case "--check-skip-setup":
			os.Exit(checkSkipSetup())
		}
	}

	slog.Info("starting chassis")

	app := common.NewApp()
	defer app.Cleanup()

	ui := gtkui.NewGtkUI()
	app.WireUI(ui)

	if err := ui.Run(); err != nil {
		slog.Error("ui error", "error", err)
		os.Exit(1)
	}

	slog.Info("chassis exiting")
}

// checkDaemon checks if the system daemon is running.
// Returns 0 if running, 1 otherwise. Native (non-Flatpak) installs ship the
// daemon as a D-Bus-activated systemd service, so there is nothing to install
func checkDaemon() int {
	if !flatpak.InFlatpakEnvironment() {
		return 0
	}
	info := setup.CheckService()
	if info.Status == setup.ServiceRunning {
		return 0
	}
	return 1
}

// checkSkipSetup returns 0 if the user has opted to skip setup, 1 otherwise.
// Setup flow is Flatpak-only; outside Flatpak there is no setup to skip.
func checkSkipSetup() int {
	if !flatpak.InFlatpakEnvironment() {
		return 0
	}
	cfg, err := config.Load()
	if err != nil {
		return 1
	}
	if cfg.SkipSetup {
		return 0
	}
	return 1
}

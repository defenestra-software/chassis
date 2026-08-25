package common

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"time"

	"defenestra.io/chassis/internal/adapter/config"
	"defenestra.io/chassis/internal/adapter/daemon"
	drmscanner "defenestra.io/chassis/internal/adapter/drm"
	"defenestra.io/chassis/internal/adapter/scanner"
	storagescanner "defenestra.io/chassis/internal/adapter/storage"
	"defenestra.io/chassis/internal/adapter/sysinfo"
	"defenestra.io/chassis/internal/adapter/udev"
	"defenestra.io/chassis/internal/core/port"
	"defenestra.io/chassis/internal/core/service"
)

type App struct {
	Service        *service.DeviceService
	Mutator        port.DriverMutator
	Listener       *udev.NetlinkListener
	StorageScanner *storagescanner.Scanner
	SysInfo        *sysinfo.Scanner
	Logger         *slog.Logger
}

func NewApp() *App {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config, using defaults", "error", err)
		cfg = &config.Config{}
	}

	level := slog.LevelInfo
	if cfg.Debug {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)

	deviceScanner := scanner.NewGHWScanner(cfg.ChrootPath)

	var mutator port.DriverMutator
	if daemon.IsAvailable() {
		client, err := daemon.NewClient()
		if err != nil {
			slog.Warn("failed to connect to daemon", "error", err)
		} else {
			mutator = client
		}
	} else {
		slog.Warn("daemon not running, privileged operations disabled")
	}

	listener := udev.NewNetlinkListener(logger)
	svc := service.NewDeviceService(deviceScanner, mutator, listener, logger)

	gpuScanner := drmscanner.NewScanner(logger)
	svc.SetGPUScanner(gpuScanner)

	storScanner := storagescanner.NewScanner(logger)
	svc.SetStorageScanner(storScanner)

	sysInfoScanner := sysinfo.NewScanner(logger)
	svc.SetSysInfoScanner(sysInfoScanner)

	// Privileged SMBIOS only available when talking to the daemon
	if client, ok := mutator.(*daemon.Client); ok {
		svc.SetSystemInfoReader(client)
	}

	return &App{
		Service:        svc,
		Mutator:        mutator,
		Listener:       listener,
		StorageScanner: storScanner,
		SysInfo:        sysInfoScanner,
		Logger:         logger,
	}
}

func (a *App) WireUI(ui port.UIAdapter) {
	ui.SetDriverManagementEnabled(a.Service.HasMutator())

	ui.SetRefreshHandler(func() error {
		tree, err := a.Service.GetDeviceTree()
		if err != nil {
			return err
		}
		ui.ShowDeviceTree(tree)
		return nil
	})

	ui.SetActionHandler(func(deviceID string, action string) error {
		if !a.Service.HasMutator() {
			slog.Error("daemon not connected, driver management unavailable")
			return nil
		}

		busType, addr := parseDeviceID(deviceID)

		switch {
		case action == "disable":
			return a.Service.UnbindDriver(addr, busType)
		case action == "enable":
			return a.Service.RestoreDriver(addr, busType)
		case strings.HasPrefix(action, "bind:"):
			driverName := strings.TrimPrefix(action, "bind:")
			return a.Service.BindDriver(addr, driverName)
		case strings.HasPrefix(action, "dump-vbios:"):
			destPath := strings.TrimPrefix(action, "dump-vbios:")
			if client, ok := a.Mutator.(*daemon.Client); ok {
				data, err := client.DumpVBIOS(addr)
				if err != nil {
					return err
				}
				if err := os.WriteFile(destPath, data, 0644); err != nil {
					return fmt.Errorf("failed to save VBIOS: %w", err)
				}
				slog.Info("VBIOS saved", "path", destPath, "bytes", len(data))
				return nil
			}
			return fmt.Errorf("daemon not connected")
		}
		return nil
	})

	// Prefer netlink; Flatpak often opens the socket but filters events, so
	// fall back to polling if nothing arrives within 15s.
	netlinkAlive := make(chan struct{}, 1)

	eventChan, err := a.Listener.Subscribe()
	if err != nil {
		slog.Warn("failed to start udev listener, using polling fallback", "error", err)
		go a.pollForDeviceChanges(ui)
	} else {
		go func() {
			var debounce *time.Timer
			for event := range eventChan {
				slog.Info("hardware event", "action", event.Action, "device", event.DeviceID)
				select {
				case netlinkAlive <- struct{}{}:
				default:
				}
				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.AfterFunc(500*time.Millisecond, func() {
					tree, err := a.Service.GetDeviceTree()
					if err != nil {
						slog.Error("failed to refresh device tree", "error", err)
						return
					}
					ui.ShowDeviceTree(tree)
				})
			}
		}()

		go func() {
			select {
			case <-netlinkAlive:
				slog.Info("hotplug: netlink confirmed working, polling disabled")
			case <-time.After(15 * time.Second):
				slog.Warn("hotplug: no netlink events received, starting polling fallback")
				go a.pollForDeviceChanges(ui)
			}
		}()
	}

	a.StorageScanner.StartMonitor(5*time.Second, func() {})

	tree, err := a.Service.GetDeviceTree()
	if err != nil {
		slog.Error("failed to scan devices", "error", err)
	} else {
		ui.ShowDeviceTree(tree)
	}
}

// parseDeviceID splits "pci:0000:01:00.0" / "usb:1-8" into bus type and address.
func parseDeviceID(deviceID string) (busType, addr string) {
	if idx := strings.Index(deviceID, ":"); idx > 0 {
		return deviceID[:idx], deviceID[idx+1:]
	}
	return "pci", deviceID
}

func (a *App) pollForDeviceChanges(ui port.UIAdapter) {
	slog.Info("hotplug: polling fallback active (every 2s)")
	lastCount := countUSBDevices()
	for {
		time.Sleep(2 * time.Second)
		count := countUSBDevices()
		if count != lastCount {
			slog.Info("hotplug: device count changed", "old", lastCount, "new", count)
			lastCount = count
			tree, err := a.Service.GetDeviceTree()
			if err != nil {
				slog.Error("failed to refresh device tree", "error", err)
				continue
			}
			ui.ShowDeviceTree(tree)
		}
	}
}

func countUSBDevices() int {
	entries, err := os.ReadDir("/sys/bus/usb/devices")
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !strings.Contains(e.Name(), ":") {
			count++
		}
	}
	return count
}

func (a *App) Cleanup() {
	a.StorageScanner.Stop()
	a.Listener.Stop()
	if a.Mutator != nil {
		if err := a.Mutator.Close(); err != nil {
			a.Logger.Error("error closing daemon connection", "error", err)
		}
	}
}

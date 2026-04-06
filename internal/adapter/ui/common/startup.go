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

// App holds the application state shared between UI implementations.
type App struct {
	Service        *service.DeviceService
	Mutator        port.DriverMutator
	Listener       *udev.NetlinkListener
	StorageScanner *storagescanner.Scanner
	SysInfo        *sysinfo.Scanner
	Logger         *slog.Logger
}

// NewApp initializes the application components.
func NewApp() *App {
	// Load configuration (TOML file + env var overrides)
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config, using defaults", "error", err)
		cfg = &config.Config{}
	}

	// Initialize structured logging
	level := slog.LevelInfo
	if cfg.Debug {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)

	// Initialize scanner adapter
	deviceScanner := scanner.NewGHWScanner(cfg.ChrootPath)

	// Initialize D-Bus client for privileged operations
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

	// Initialize udev listener for hotplug events
	listener := udev.NewNetlinkListener(logger)

	// Initialize core service
	svc := service.NewDeviceService(deviceScanner, mutator, listener, logger)

	// Initialize DRM scanner for GPU/monitor info
	gpuScanner := drmscanner.NewScanner(logger)
	svc.SetGPUScanner(gpuScanner)

	// Initialize storage scanner for drive info and temps
	storScanner := storagescanner.NewScanner(logger)
	svc.SetStorageScanner(storScanner)

	// Initialize system info scanner for processors, sensors, displays
	sysInfoScanner := sysinfo.NewScanner(logger)
	svc.SetSysInfoScanner(sysInfoScanner)

	// If daemon is connected, use it for privileged SMBIOS data
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

// WireUI connects the UI to the app service.
func (a *App) WireUI(ui port.UIAdapter) {
	// Enable driver management buttons only when daemon is connected
	ui.SetDriverManagementEnabled(a.Service.HasMutator())

	// Wire up refresh handler
	ui.SetRefreshHandler(func() error {
		tree, err := a.Service.GetDeviceTree()
		if err != nil {
			return err
		}
		ui.ShowDeviceTree(tree)
		return nil
	})

	// Wire up action handler (only active when daemon is connected)
	ui.SetActionHandler(func(deviceID string, action string) error {
		if !a.Service.HasMutator() {
			slog.Error("daemon not connected, driver management unavailable")
			return nil
		}

		// Parse bus type and address from device ID (e.g. "pci:0000:01:00.0" or "usb:1-8")
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

	// Start listening for hotplug events.
	// Strategy: use netlink for instant event-driven detection. If netlink
	// fails to deliver events (common in Flatpak where the socket opens but
	// events are silently filtered), fall back to polling after a timeout.
	netlinkAlive := make(chan struct{}, 1)

	eventChan, err := a.Listener.Subscribe()
	if err != nil {
		slog.Warn("failed to start udev listener, using polling fallback", "error", err)
		// Netlink failed entirely - go straight to polling
		go a.pollForDeviceChanges(ui)
	} else {
		// Netlink opened - listen for events and signal liveness
		go func() {
			var debounce *time.Timer
			for event := range eventChan {
				slog.Info("hardware event", "action", event.Action, "device", event.DeviceID)
				// Signal that netlink is working (only need to do this once)
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

		// Wait to see if netlink delivers events. If nothing arrives within
		// 15 seconds, assume it's broken (Flatpak sandbox) and start polling.
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

	// Start storage temperature monitor (polls every 5 seconds).
	// StorageInfo objects are shared pointers - temps update in place.
	// The UI reads fresh values when displaying a storage device detail page.
	a.StorageScanner.StartMonitor(5*time.Second, func() {
		// No UI action needed - temps are updated in place on the shared objects.
		// The detail page refreshes its own temp display via its own timer.
	})

	// Initial device scan
	tree, err := a.Service.GetDeviceTree()
	if err != nil {
		slog.Error("failed to scan devices", "error", err)
	} else {
		ui.ShowDeviceTree(tree)
	}
}

// parseDeviceID splits a device ID like "pci:0000:01:00.0" or "usb:1-8"
// into bus type and address.
func parseDeviceID(deviceID string) (busType, addr string) {
	if idx := strings.Index(deviceID, ":"); idx > 0 {
		return deviceID[:idx], deviceID[idx+1:]
	}
	return "pci", deviceID
}

// pollForDeviceChanges polls USB device count every 2 seconds as a fallback
// for hotplug detection when netlink is unavailable (e.g., inside Flatpak).
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

// countUSBDevices returns the number of USB device entries in sysfs.
// Used for polling-based hotplug detection when netlink is unavailable.
func countUSBDevices() int {
	entries, err := os.ReadDir("/sys/bus/usb/devices")
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		// Count only device entries (no colon = not an interface)
		if !strings.Contains(e.Name(), ":") {
			count++
		}
	}
	return count
}

// Cleanup releases resources.
func (a *App) Cleanup() {
	a.StorageScanner.Stop()
	a.Listener.Stop()
	if a.Mutator != nil {
		if err := a.Mutator.Close(); err != nil {
			a.Logger.Error("error closing daemon connection", "error", err)
		}
	}
}

package main

import (
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"defenestra.io/chassis/internal/adapter/daemon"
	"defenestra.io/chassis/internal/adapter/drm"
	"defenestra.io/chassis/internal/adapter/scanner"
	"defenestra.io/chassis/internal/adapter/storage"
	"defenestra.io/chassis/internal/adapter/sysinfo"
	"defenestra.io/chassis/internal/core/service"
)

func main() {
	level := slog.LevelInfo
	if debug, _ := strconv.ParseBool(os.Getenv("CHASSIS_DEBUG")); debug {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)

	slog.Info("starting chassis-daemon")

	if os.Geteuid() != 0 {
		slog.Error("chassis-daemon must run as root")
		os.Exit(1)
	}

	// The daemon handles privileged writes directly; the app handles udev hotplug events.
	devices := service.NewDeviceService(scanner.NewGHWScanner(""), nil, nil, logger)
	devices.SetGPUScanner(drm.NewScanner(logger))
	storageScanner := storage.NewScanner(logger)
	devices.SetStorageScanner(storageScanner)
	devices.SetSysInfoScanner(sysinfo.NewScanner(logger))
	devices.SetSystemInfoReader(daemon.NewSMBIOSReader(logger))

	server, err := daemon.NewServer(logger, devices)
	if err != nil {
		slog.Error("failed to create server", "error", err)
		os.Exit(1)
	}

	if err := server.Start(); err != nil {
		slog.Error("failed to start server", "error", err)
		os.Exit(1)
	}
	storageScanner.StartMonitor(5*time.Second, func() {})

	slog.Info("daemon ready")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	slog.Info("received signal, shutting down", "signal", sig)

	storageScanner.Stop()
	if err := server.Stop(); err != nil {
		slog.Error("error stopping server", "error", err)
	}

	slog.Info("chassis-daemon exiting")
}

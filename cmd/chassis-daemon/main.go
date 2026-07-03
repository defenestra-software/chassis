package main

import (
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"defenestra.io/chassis/internal/adapter/daemon"
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

	// Check if running as root
	if os.Geteuid() != 0 {
		slog.Error("chassis-daemon must run as root")
		os.Exit(1)
	}

	// Create and start the D-Bus server
	server, err := daemon.NewServer(logger)
	if err != nil {
		slog.Error("failed to create server", "error", err)
		os.Exit(1)
	}

	if err := server.Start(); err != nil {
		slog.Error("failed to start server", "error", err)
		os.Exit(1)
	}

	slog.Info("daemon ready")

	// Wait for termination signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	slog.Info("received signal, shutting down", "signal", sig)

	if err := server.Stop(); err != nil {
		slog.Error("error stopping server", "error", err)
	}

	slog.Info("chassis-daemon exiting")
}

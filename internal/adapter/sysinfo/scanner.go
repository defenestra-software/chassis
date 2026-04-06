// Package sysinfo provides scanners for processors, sensors, and displays
// by reading sysfs. All data is world-readable - no root privileges needed.
package sysinfo

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// Scanner reads processor, sensor, and display info from sysfs and /proc.
type Scanner struct {
	logger *slog.Logger
}

func NewScanner(logger *slog.Logger) *Scanner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scanner{logger: logger}
}

func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func parseCacheSize(s string) uint64 {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "K") {
		v, _ := strconv.ParseUint(strings.TrimSuffix(s, "K"), 10, 64)
		return v * 1024
	}
	if strings.HasSuffix(s, "M") {
		v, _ := strconv.ParseUint(strings.TrimSuffix(s, "M"), 10, 64)
		return v * 1024 * 1024
	}
	v, _ := strconv.ParseUint(s, 10, 64)
	return v
}

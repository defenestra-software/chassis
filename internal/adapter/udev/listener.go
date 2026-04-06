package udev

import (
	"bufio"
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"syscall"

	"defenestra.io/chassis/internal/core/domain"
	"defenestra.io/chassis/internal/core/port"
)

const (
	// NETLINK_KOBJECT_UEVENT is the netlink family for kernel uevents
	NETLINK_KOBJECT_UEVENT = 15
)

type NetlinkListener struct {
	fd        int
	eventChan chan domain.HardwareEvent
	stopChan  chan struct{}
	logger    *slog.Logger
}

func NewNetlinkListener(logger *slog.Logger) *NetlinkListener {
	return &NetlinkListener{
		fd:        -1,
		eventChan: make(chan domain.HardwareEvent, 100),
		stopChan:  make(chan struct{}),
		logger:    logger,
	}
}

var _ port.EventListener = (*NetlinkListener)(nil)

func (l *NetlinkListener) Subscribe() (<-chan domain.HardwareEvent, error) {
	// Create netlink socket
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_RAW, NETLINK_KOBJECT_UEVENT)
	if err != nil {
		return nil, fmt.Errorf("failed to create netlink socket: %w", err)
	}
	l.fd = fd

	// Bind to both kernel and udev events.
	// Group 1 = kernel uevents (requires CAP_NET_ADMIN in some setups)
	// Group 2 = udev processed events (available to regular users)
	addr := &syscall.SockaddrNetlink{
		Family: syscall.AF_NETLINK,
		Groups: 1 | 2,
	}

	if err := syscall.Bind(fd, addr); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("failed to bind netlink socket: %w", err)
	}

	// Start reading events
	go l.readLoop()

	l.logger.Info("udev listener started")
	return l.eventChan, nil
}

func (l *NetlinkListener) readLoop() {
	buf := make([]byte, 8192)

	for {
		select {
		case <-l.stopChan:
			return
		default:
		}

		// Set read timeout so we can check stopChan periodically
		syscall.SetsockoptTimeval(l.fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO,
			&syscall.Timeval{Sec: 1})

		n, _, err := syscall.Recvfrom(l.fd, buf, 0)
		if err != nil {
			// Timeout is expected, continue
			if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
				continue
			}
			l.logger.Error("netlink recv error", "error", err)
			continue
		}

		if n > 0 {
			event := l.parseUevent(buf[:n])
			if event != nil {
				select {
				case l.eventChan <- *event:
				default:
					l.logger.Warn("event channel full, dropping event")
				}
			}
		}
	}
}

func (l *NetlinkListener) parseUevent(data []byte) *domain.HardwareEvent {
	// Uevent format: ACTION@DEVPATH\0KEY=VALUE\0KEY=VALUE\0...
	// First part before @ is the action, rest is devpath, then null-separated key=value pairs

	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Split(scanNullTerminated)

	var action, devpath, subsystem string

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// First line contains action@devpath
		if action == "" && strings.Contains(line, "@") {
			parts := strings.SplitN(line, "@", 2)
			if len(parts) == 2 {
				action = parts[0]
				devpath = parts[1]
			}
			continue
		}

		// Parse key=value pairs
		if strings.HasPrefix(line, "SUBSYSTEM=") {
			subsystem = strings.TrimPrefix(line, "SUBSYSTEM=")
		}
	}

	// Only handle PCI and USB events
	if subsystem != "pci" && subsystem != "usb" {
		if subsystem != "" {
			l.logger.Debug("ignoring uevent", "subsystem", subsystem, "action", action)
		}
		return nil
	}

	// Map action to our event type
	switch action {
	case "add", "remove", "bind", "unbind":
		// Extract device ID from devpath
		deviceID := extractDeviceID(devpath, subsystem)
		if deviceID == "" {
			return nil
		}

		deviceType := domain.DeviceTypeUnknown
		if subsystem == "pci" {
			deviceType = domain.DeviceTypePCI
		} else if subsystem == "usb" {
			deviceType = domain.DeviceTypeUSB
		}

		l.logger.Info("uevent received", "action", action, "device", deviceID, "subsystem", subsystem)

		return &domain.HardwareEvent{
			Action:   action,
			DeviceID: deviceID,
			Type:     deviceType,
		}
	}

	return nil
}

func extractDeviceID(devpath, subsystem string) string {
	// devpath looks like /devices/pci0000:00/0000:00:1f.3
	// We want the last component that matches the subsystem format
	parts := strings.Split(devpath, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		part := parts[i]
		if subsystem == "pci" && isPCIAddress(part) {
			return fmt.Sprintf("pci:%s", part)
		}
		if subsystem == "usb" && isUSBAddress(part) {
			return fmt.Sprintf("usb:%s", part)
		}
	}
	return ""
}

func isPCIAddress(s string) bool {
	// PCI address format: DDDD:BB:DD.F
	if len(s) < 10 {
		return false
	}
	// Simple validation: check for colons and dot in expected positions
	return len(s) >= 12 && s[4] == ':' && s[7] == ':' && s[10] == '.'
}

func isUSBAddress(s string) bool {
	// USB address format: X-Y or X-Y.Z
	return strings.Contains(s, "-") && !strings.Contains(s, ":")
}

func scanNullTerminated(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexByte(data, 0); i >= 0 {
		return i + 1, data[0:i], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func (l *NetlinkListener) Stop() error {
	close(l.stopChan)

	if l.fd >= 0 {
		syscall.Close(l.fd)
		l.fd = -1
	}

	// Don't close eventChan here - let it drain naturally
	l.logger.Info("udev listener stopped")
	return nil
}

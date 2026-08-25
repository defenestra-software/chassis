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
	NETLINK_KOBJECT_UEVENT = 15
)

type NetlinkListener struct {
	eventChan chan domain.HardwareEvent
	stopChan  chan struct{}
	logger    *slog.Logger
}

func NewNetlinkListener(logger *slog.Logger) *NetlinkListener {
	return &NetlinkListener{
		eventChan: make(chan domain.HardwareEvent, 100),
		stopChan:  make(chan struct{}),
		logger:    logger,
	}
}

var _ port.EventListener = (*NetlinkListener)(nil)

func (l *NetlinkListener) Subscribe() (<-chan domain.HardwareEvent, error) {
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_RAW, NETLINK_KOBJECT_UEVENT)
	if err != nil {
		return nil, fmt.Errorf("failed to create netlink socket: %w", err)
	}

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

	go l.readLoop(fd)

	l.logger.Info("udev listener started")
	return l.eventChan, nil
}

func (l *NetlinkListener) readLoop(fd int) {
	defer syscall.Close(fd)
	buf := make([]byte, 8192)

	for {
		select {
		case <-l.stopChan:
			return
		default:
		}

		syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO,
			&syscall.Timeval{Sec: 1})

		n, _, err := syscall.Recvfrom(fd, buf, 0)
		if err != nil {
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

	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Split(scanNullTerminated)

	var action, devpath, subsystem string

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		if action == "" && strings.Contains(line, "@") {
			parts := strings.SplitN(line, "@", 2)
			if len(parts) == 2 {
				action = parts[0]
				devpath = parts[1]
			}
			continue
		}

		if strings.HasPrefix(line, "SUBSYSTEM=") {
			subsystem = strings.TrimPrefix(line, "SUBSYSTEM=")
		}
	}

	if subsystem != "pci" && subsystem != "usb" {
		if subsystem != "" {
			l.logger.Debug("ignoring uevent", "subsystem", subsystem, "action", action)
		}
		return nil
	}

	switch action {
	case "add", "remove", "bind", "unbind":
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
	if len(s) < 10 {
		return false
	}
	return len(s) >= 12 && s[4] == ':' && s[7] == ':' && s[10] == '.'
}

func isUSBAddress(s string) bool {
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
	l.logger.Info("udev listener stopped")
	return nil
}

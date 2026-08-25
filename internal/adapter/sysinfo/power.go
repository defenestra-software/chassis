package sysinfo

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	flatpak "gitlab.com/defenestra-software/go-flatbridge"

	"defenestra.io/chassis/internal/core/domain"
)

func (s *Scanner) ScanPower() []*domain.DeviceNode {
	var nodes []*domain.DeviceNode

	psPath := flatpak.HostPath("/sys/class/power_supply")
	entries, err := os.ReadDir(psPath)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		devPath := filepath.Join(psPath, entry.Name())
		node := s.scanPowerSupply(devPath, entry.Name())
		if node != nil {
			nodes = append(nodes, node)
		}
	}

	s.logger.Debug("scanned power supplies", "count", len(nodes))
	return nodes
}

func (s *Scanner) scanPowerSupply(devPath, name string) *domain.DeviceNode {
	psType := readFile(filepath.Join(devPath, "type"))
	if psType == "" {
		return nil
	}

	// Skip peripheral batteries (mice, keyboards, headsets).
	// System batteries have scope "System" or no scope attribute.
	// Peripheral batteries have scope "Device".
	scope := readFile(filepath.Join(devPath, "scope"))
	if strings.EqualFold(scope, "Device") {
		return nil
	}

	status := readFile(filepath.Join(devPath, "status"))
	model := readFile(filepath.Join(devPath, "model_name"))
	manufacturer := readFile(filepath.Join(devPath, "manufacturer"))

	displayName := name
	subtitle := ""

	switch strings.ToLower(psType) {
	case "battery":
		if model != "" {
			displayName = model
		}
		capacity := readFile(filepath.Join(devPath, "capacity"))
		if capacity != "" {
			subtitle = fmt.Sprintf("%s%%", capacity)
		}
		if status != "" {
			if subtitle != "" {
				subtitle += " · "
			}
			subtitle += status
		}
		energyNow := readFile(filepath.Join(devPath, "energy_now"))
		energyFull := readFile(filepath.Join(devPath, "energy_full"))
		if energyNow == "" {
			energyNow = readFile(filepath.Join(devPath, "charge_now"))
			energyFull = readFile(filepath.Join(devPath, "charge_full"))
		}
		if energyFull != "" {
			if full, err := strconv.ParseFloat(energyFull, 64); err == nil && full > 0 {
				subtitle += fmt.Sprintf(" · %.1f Wh", full/1000000)
			}
		}

	case "mains", "ups":
		displayName = "AC Adapter"
		if model != "" {
			displayName = model
		}
		if status == "Charging" || status == "Not charging" || status == "Full" {
			subtitle = status
		} else {
			online := readFile(filepath.Join(devPath, "online"))
			if online == "1" {
				subtitle = "Connected"
			} else {
				subtitle = "Disconnected"
			}
		}

	default:
		return nil
	}

	node := &domain.DeviceNode{
		ID:         "power:" + name,
		Name:       displayName,
		Vendor:     manufacturer,
		Product:    subtitle,
		Type:       domain.DeviceTypeUnknown,
		Class:      domain.DeviceClassPower,
		Path:       devPath,
		IOMMUGroup: -1,
		Children:   make([]*domain.DeviceNode, 0),
	}

	return node
}

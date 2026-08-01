package sysinfo

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	flatpak "gitlab.com/defenestra-software/go-flatbridge"

	"defenestra.io/chassis/internal/core/domain"
)

// ScanProcessors returns one node per physical package (multi-socket systems included).
func (s *Scanner) ScanProcessors() []*domain.DeviceNode {
	info := s.readCPUInfo()
	if info == nil {
		return nil
	}

	subtitle := info.CoreString()
	if info.MaxFreqMHz > 0 {
		if subtitle != "" {
			subtitle += " · "
		}
		subtitle += info.FreqString()
	}

	node := &domain.DeviceNode{
		ID:            "cpu:0",
		Name:          info.ModelName,
		Vendor:        info.Vendor,
		Product:       subtitle,
		Type:          domain.DeviceTypeUnknown,
		Class:         domain.DeviceClassProcessor,
		ProcessorInfo: info,
		IOMMUGroup:    -1,
		Children:      make([]*domain.DeviceNode, 0),
	}

	return []*domain.DeviceNode{node}
}

func (s *Scanner) readCPUInfo() *domain.ProcessorInfo {
	data, err := os.ReadFile(flatpak.HostPath("/proc/cpuinfo"))
	if err != nil {
		return nil
	}

	info := &domain.ProcessorInfo{
		Architecture: runtime.GOARCH,
	}

	seen := map[string]bool{}
	physicalIDs := map[string]bool{}

	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		if seen[key] && key != "physical id" && key != "processor" {
			continue
		}

		switch key {
		case "model name":
			info.ModelName = val
			seen[key] = true
		case "vendor_id":
			info.Vendor = val
			seen[key] = true
		case "cpu family":
			info.Family = val
			seen[key] = true
		case "model":
			if !seen[key] {
				info.Model = val
				seen[key] = true
			}
		case "stepping":
			info.Stepping = val
			seen[key] = true
		case "microcode":
			info.Microcode = val
			seen[key] = true
		case "bogomips":
			info.BogoMIPS, _ = strconv.ParseFloat(val, 64)
			seen[key] = true
		case "physical id":
			physicalIDs[val] = true
		case "processor":
			info.Threads++
		}
	}

	cpuPath := flatpak.HostPath("/sys/devices/system/cpu")
	coreIDs := map[string]bool{}
	entries, _ := os.ReadDir(cpuPath)
	cpuPattern := regexp.MustCompile(`^cpu\d+$`)
	for _, entry := range entries {
		if !cpuPattern.MatchString(entry.Name()) {
			continue
		}
		coreID := readFile(filepath.Join(cpuPath, entry.Name(), "topology", "core_id"))
		pkgID := readFile(filepath.Join(cpuPath, entry.Name(), "topology", "physical_package_id"))
		if coreID != "" {
			coreIDs[pkgID+":"+coreID] = true
		}
	}
	if len(coreIDs) > 0 {
		info.Cores = len(coreIDs)
	}

	maxFreq := readFile(filepath.Join(cpuPath, "cpu0", "cpufreq", "cpuinfo_max_freq"))
	if maxFreq != "" {
		if khz, err := strconv.ParseFloat(maxFreq, 64); err == nil {
			info.MaxFreqMHz = khz / 1000
		}
	}
	minFreq := readFile(filepath.Join(cpuPath, "cpu0", "cpufreq", "cpuinfo_min_freq"))
	if minFreq != "" {
		if khz, err := strconv.ParseFloat(minFreq, 64); err == nil {
			info.MinFreqMHz = khz / 1000
		}
	}

	cachePath := filepath.Join(cpuPath, "cpu0", "cache")
	cacheEntries, _ := os.ReadDir(cachePath)
	for _, entry := range cacheEntries {
		if !strings.HasPrefix(entry.Name(), "index") {
			continue
		}
		level := readFile(filepath.Join(cachePath, entry.Name(), "level"))
		cType := readFile(filepath.Join(cachePath, entry.Name(), "type"))
		sizeStr := readFile(filepath.Join(cachePath, entry.Name(), "size"))
		size := parseCacheSize(sizeStr)

		switch {
		case level == "1" && cType == "Data":
			info.CacheL1d = size * uint64(info.Cores)
		case level == "1" && cType == "Instruction":
			info.CacheL1i = size * uint64(info.Cores)
		case level == "2":
			info.CacheL2 = size * uint64(info.Cores)
		case level == "3":
			info.CacheL3 = size // L3 is typically shared
		}
	}

	s.logger.Debug("scanned processor",
		"model", info.ModelName,
		"cores", info.Cores,
		"threads", info.Threads,
		"freq", info.FreqString())

	return info
}

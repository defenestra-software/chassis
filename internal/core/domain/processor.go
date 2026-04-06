package domain

import "fmt"

type ProcessorInfo struct {
	ModelName    string
	Vendor       string
	Family       string
	Model        string
	Stepping     string
	Microcode    string
	Cores        int
	Threads      int
	MaxFreqMHz   float64
	MinFreqMHz   float64
	CacheL1d     uint64
	CacheL1i     uint64
	CacheL2      uint64
	CacheL3      uint64
	Flags        []string
	Architecture string
	BogoMIPS     float64
}

func (p *ProcessorInfo) FreqString() string {
	if p.MaxFreqMHz >= 1000 {
		return fmt.Sprintf("%.2f GHz", p.MaxFreqMHz/1000)
	}
	return fmt.Sprintf("%.0f MHz", p.MaxFreqMHz)
}

func formatCacheSize(bytes uint64) string {
	if bytes == 0 {
		return ""
	}
	if bytes >= 1024*1024 {
		return fmt.Sprintf("%d MB", bytes/(1024*1024))
	}
	return fmt.Sprintf("%d KB", bytes/1024)
}

func (p *ProcessorInfo) CacheSummary() string {
	parts := []string{}
	if p.CacheL1d > 0 {
		parts = append(parts, fmt.Sprintf("L1d: %s", formatCacheSize(p.CacheL1d)))
	}
	if p.CacheL2 > 0 {
		parts = append(parts, fmt.Sprintf("L2: %s", formatCacheSize(p.CacheL2)))
	}
	if p.CacheL3 > 0 {
		parts = append(parts, fmt.Sprintf("L3: %s", formatCacheSize(p.CacheL3)))
	}
	if len(parts) == 0 {
		return ""
	}
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += " · "
		}
		result += p
	}
	return result
}

func (p *ProcessorInfo) CoreString() string {
	if p.Threads > p.Cores && p.Cores > 0 {
		return fmt.Sprintf("%d cores, %d threads", p.Cores, p.Threads)
	}
	if p.Cores > 0 {
		return fmt.Sprintf("%d cores", p.Cores)
	}
	return ""
}

package agent

import (
	"strconv"
	"strings"
)

// parseCPUToMillis converts a Kubernetes CPU resource string to millicores.
// Examples: "500m" → 500, "1" → 1000, "2.5" → 2500, "100m" → 100
func parseCPUToMillis(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if strings.HasSuffix(s, "m") {
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "m"), 64)
		if err != nil {
			return 0
		}
		return int64(v)
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(v * 1000)
}

// parseMemoryToBytes converts a Kubernetes memory resource string to bytes.
// Examples: "256Mi" → 268435456, "1Gi" → 1073741824, "1000" → 1000
func parseMemoryToBytes(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	suffixes := []struct {
		suffix     string
		multiplier int64
	}{
		{"Ei", 1 << 60},
		{"Pi", 1 << 50},
		{"Ti", 1 << 40},
		{"Gi", 1 << 30},
		{"Mi", 1 << 20},
		{"Ki", 1 << 10},
		{"E", 1_000_000_000_000_000_000},
		{"P", 1_000_000_000_000_000},
		{"T", 1_000_000_000_000},
		{"G", 1_000_000_000},
		{"M", 1_000_000},
		{"k", 1_000},
	}

	for _, sf := range suffixes {
		if strings.HasSuffix(s, sf.suffix) {
			v, err := strconv.ParseFloat(strings.TrimSuffix(s, sf.suffix), 64)
			if err != nil {
				return 0
			}
			return int64(v * float64(sf.multiplier))
		}
	}

	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(v)
}

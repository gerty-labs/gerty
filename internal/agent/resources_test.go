package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseCPUToMillis(t *testing.T) {
	tests := []struct {
		name string
		input string
		want  int64
	}{
		// Standard millicore values
		{"500m", "500m", 500},
		{"100m", "100m", 100},
		{"1000m", "1000m", 1000},

		// Whole cores
		{"1 core", "1", 1000},
		{"2 cores", "2", 2000},

		// Fractional cores
		{"2.5 cores", "2.5", 2500},
		{"0.5 cores", "0.5", 500},
		{"0.1 cores", "0.1", 100},

		// Fractional millicores — ParseFloat handles decimals in "m" suffix
		{"0.5m", "0.5m", 0}, // int64(0.5) truncates to 0

		// Zeroes
		{"0m", "0m", 0},
		{"0 cores", "0", 0},

		// Whitespace
		{"leading whitespace", "  500m", 500},
		{"trailing whitespace", "500m  ", 500},
		{"both whitespace", "  500m  ", 500},
		{"whitespace cores", "  2  ", 2000},

		// Empty
		{"empty string", "", 0},
		{"whitespace only", "   ", 0},

		// Garbage
		{"letters", "abc", 0},
		{"bare m suffix", "m", 0},
		{"special chars", "!@#", 0},

		// Negative values — document actual behavior (ParseFloat succeeds)
		{"negative millis", "-100m", -100},
		{"negative cores", "-1", -1000},

		// Large values
		{"large millis", "999999m", 999999},
		{"large cores", "999999999", 999999999000},

		// Case sensitivity — "M" (mega) is not a valid CPU suffix
		{"uppercase M", "500M", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCPUToMillis(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseMemoryToBytes(t *testing.T) {
	tests := []struct {
		name string
		input string
		want  int64
	}{
		// Binary suffixes
		{"256Mi", "256Mi", 256 * 1024 * 1024},
		{"1Gi", "1Gi", 1024 * 1024 * 1024},
		{"512Ki", "512Ki", 512 * 1024},
		{"1Ti", "1Ti", 1 << 40},
		{"1Pi", "1Pi", 1 << 50},
		{"1Ei", "1Ei", 1 << 60},

		// Decimal suffixes
		{"1k", "1k", 1000},
		{"1M", "1M", 1_000_000},
		{"1G", "1G", 1_000_000_000},
		{"1T", "1T", 1_000_000_000_000},
		{"1P", "1P", 1_000_000_000_000_000},
		{"1E", "1E", 1_000_000_000_000_000_000},

		// Raw bytes
		{"raw bytes", "1048576", 1048576},
		{"raw zero", "0", 0},

		// Zeroes with suffixes
		{"0Mi", "0Mi", 0},
		{"0Gi", "0Gi", 0},
		{"0Ki", "0Ki", 0},

		// Whitespace
		{"leading whitespace", "  256Mi", 256 * 1024 * 1024},
		{"trailing whitespace", "256Mi  ", 256 * 1024 * 1024},
		{"both whitespace", "  256Mi  ", 256 * 1024 * 1024},

		// Empty
		{"empty string", "", 0},
		{"whitespace only", "   ", 0},

		// Garbage
		{"letters", "abc", 0},
		{"bare Mi", "Mi", 0},
		{"bare Ki", "Ki", 0},

		// Negative values — document actual behavior
		{"negative Mi", "-256Mi", -256 * 1024 * 1024},
		{"negative Gi", "-1Gi", -1024 * 1024 * 1024},

		// Case sensitivity: "256k" (lowercase k) is valid decimal, "256Ki" is binary
		{"lowercase k", "256k", 256 * 1000},
		{"Ki vs k", "256Ki", 256 * 1024},

		// Large values
		{"large bytes", "999999999", 999999999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseMemoryToBytes(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

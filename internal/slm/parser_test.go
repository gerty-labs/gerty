package slm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRecommendation_ValidJSON(t *testing.T) {
	raw := `{
		"cpu_request": "250m",
		"cpu_limit": "500m",
		"memory_request": "256Mi",
		"memory_limit": "512Mi",
		"pattern": "steady",
		"confidence": 0.92,
		"reasoning_code": "OVER_PROVISIONED",
		"explanation": "CPU usage is consistently low relative to request",
		"risk": "LOW"
	}`

	output, err := ParseRecommendation(raw)
	require.NoError(t, err)
	assert.Equal(t, "250m", output.CPURequest)
	assert.Equal(t, "500m", output.CPULimit)
	assert.Equal(t, "256Mi", output.MemoryRequest)
	assert.Equal(t, "512Mi", output.MemoryLimit)
	assert.Equal(t, "steady", output.Pattern)
	assert.InDelta(t, 0.92, output.Confidence, 0.001)
	assert.Equal(t, "OVER_PROVISIONED", output.ReasoningCode)
	assert.Equal(t, "LOW", output.Risk)
}

func TestParseRecommendation_MarkdownBlock(t *testing.T) {
	raw := "Here is my analysis:\n```json\n" +
		`{"cpu_request":"100m","cpu_limit":"200m","memory_request":"128Mi","memory_limit":"256Mi","pattern":"idle","confidence":0.85,"reasoning_code":"IDLE","explanation":"Near-zero usage","risk":"LOW"}` +
		"\n```"

	output, err := ParseRecommendation(raw)
	require.NoError(t, err)
	assert.Equal(t, "100m", output.CPURequest)
	assert.Equal(t, "idle", output.Pattern)
}

func TestParseRecommendation_EmbeddedJSON(t *testing.T) {
	raw := `Based on the metrics, here is my recommendation: {"cpu_request":"300m","cpu_limit":"600m","memory_request":"512Mi","memory_limit":"1Gi","pattern":"burstable","confidence":0.78,"reasoning_code":"BURSTABLE","explanation":"High variance","risk":"MEDIUM"} That's my analysis.`

	output, err := ParseRecommendation(raw)
	require.NoError(t, err)
	assert.Equal(t, "300m", output.CPURequest)
	assert.Equal(t, "burstable", output.Pattern)
	assert.Equal(t, "MEDIUM", output.Risk)
}

func TestParseRecommendation_EmptyInput(t *testing.T) {
	_, err := ParseRecommendation("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestParseRecommendation_NoJSON(t *testing.T) {
	_, err := ParseRecommendation("This is just text without any JSON")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no JSON")
}

func TestParseRecommendation_MissingCPURequest(t *testing.T) {
	raw := `{"cpu_limit":"500m","memory_request":"256Mi","memory_limit":"512Mi","pattern":"steady","confidence":0.9,"risk":"LOW"}`
	_, err := ParseRecommendation(raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cpu_request")
}

func TestParseRecommendation_MissingMemoryRequest(t *testing.T) {
	raw := `{"cpu_request":"250m","cpu_limit":"500m","memory_limit":"512Mi","pattern":"steady","confidence":0.9,"risk":"LOW"}`
	_, err := ParseRecommendation(raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "memory_request")
}

func TestParseRecommendation_InvalidPattern(t *testing.T) {
	raw := `{"cpu_request":"250m","cpu_limit":"500m","memory_request":"256Mi","memory_limit":"512Mi","pattern":"unknown","confidence":0.9,"risk":"LOW"}`
	_, err := ParseRecommendation(raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid pattern")
}

func TestParseRecommendation_InvalidRisk(t *testing.T) {
	raw := `{"cpu_request":"250m","cpu_limit":"500m","memory_request":"256Mi","memory_limit":"512Mi","pattern":"steady","confidence":0.9,"risk":"EXTREME"}`
	_, err := ParseRecommendation(raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid risk")
}

func TestParseRecommendation_ConfidenceOutOfRange(t *testing.T) {
	raw := `{"cpu_request":"250m","cpu_limit":"500m","memory_request":"256Mi","memory_limit":"512Mi","pattern":"steady","confidence":1.5,"risk":"LOW"}`
	_, err := ParseRecommendation(raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "confidence")
}

func TestParseRecommendation_NormalisesCase(t *testing.T) {
	raw := `{"cpu_request":"250m","cpu_limit":"500m","memory_request":"256Mi","memory_limit":"512Mi","pattern":"STEADY","confidence":0.9,"reasoning_code":"OK","explanation":"fine","risk":"low"}`
	output, err := ParseRecommendation(raw)
	require.NoError(t, err)
	assert.Equal(t, "steady", output.Pattern)
	assert.Equal(t, "LOW", output.Risk)
}

func TestParseResourceMillis(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
		wantErr  bool
	}{
		{"500m", 500, false},
		{"250m", 250, false},
		{"1000m", 1000, false},
		{"2", 2000, false},
		{"0.5", 500, false},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseResourceMillis(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseResourceBytes(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
		wantErr  bool
	}{
		{"256Mi", 256 * 1024 * 1024, false},
		{"1Gi", 1024 * 1024 * 1024, false},
		{"512Ki", 512 * 1024, false},
		{"1048576", 1048576, false},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseResourceBytes(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseRecommendation_BackticksInValues(t *testing.T) {
	// Explanation contains backticks — should parse fine since they're inside JSON strings.
	raw := `{"cpu_request":"250m","cpu_limit":"500m","memory_request":"256Mi","memory_limit":"512Mi","pattern":"steady","confidence":0.9,"reasoning_code":"OK","explanation":"The ` + "`cpu`" + ` usage is low","risk":"LOW"}`

	output, err := ParseRecommendation(raw)
	require.NoError(t, err)
	assert.Contains(t, output.Explanation, "`cpu`")
}

func TestParseRecommendation_MultipleCodeBlocks(t *testing.T) {
	// Two code blocks — should extract the first one.
	raw := "First block:\n```json\n" +
		`{"cpu_request":"100m","cpu_limit":"200m","memory_request":"128Mi","memory_limit":"256Mi","pattern":"idle","confidence":0.85,"reasoning_code":"IDLE","explanation":"first","risk":"LOW"}` +
		"\n```\nSecond block:\n```json\n" +
		`{"cpu_request":"999m","cpu_limit":"999m","memory_request":"999Mi","memory_limit":"999Mi","pattern":"batch","confidence":0.1,"reasoning_code":"X","explanation":"second","risk":"HIGH"}` +
		"\n```"

	output, err := ParseRecommendation(raw)
	require.NoError(t, err)
	assert.Equal(t, "100m", output.CPURequest, "should extract first code block")
	assert.Equal(t, "first", output.Explanation)
}

func TestParseRecommendation_EmptyBraces(t *testing.T) {
	_, err := ParseRecommendation("{}")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cpu_request")
}

func TestParseRecommendation_ExplanationTruncation(t *testing.T) {
	longExplanation := make([]byte, 2000)
	for i := range longExplanation {
		longExplanation[i] = 'x'
	}
	raw := `{"cpu_request":"250m","cpu_limit":"500m","memory_request":"256Mi","memory_limit":"512Mi","pattern":"steady","confidence":0.9,"reasoning_code":"OK","explanation":"` + string(longExplanation) + `","risk":"LOW"}`

	output, err := ParseRecommendation(raw)
	require.NoError(t, err)
	assert.Len(t, output.Explanation, 1024, "explanation should be truncated to 1024 chars")
}

func TestParseRecommendation_WhitespaceOnly(t *testing.T) {
	_, err := ParseRecommendation("   ")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestParseResourceMillis_Extended(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{"0m", "0m", 0, false},
		{"-100m", "-100m", -100, false},
		{"123.5m decimal millis", "123.5m", 0, true}, // Sscanf %dm fails on decimal
		{"whitespace 500m", "  500m  ", 500, false},
		{"abc garbage", "abc", 0, true},
		{"bare m", "m", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseResourceMillis(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseResourceBytes_Extended(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{"1Ti", "1Ti", 1024 * 1024 * 1024 * 1024, false},
		{"0 bytes", "0", 0, false},
		{"-256Mi", "-256Mi", -256 * 1024 * 1024, false},
		{"256k lowercase", "256k", 256, false}, // Sscanf %d parses "256" from "256k"
		{"abc garbage", "abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseResourceBytes(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"shorter than max", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"truncated", "hello world", 5, "hello"},
		{"zero max", "hello", 0, ""},
		{"empty input", "", 10, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			assert.Equal(t, tt.want, got)
		})
	}
}

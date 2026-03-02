package slm

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// SLMOutput is the structured recommendation from the SLM.
type SLMOutput struct {
	CPURequest    string  `json:"cpu_request"`
	CPULimit      string  `json:"cpu_limit"`
	MemoryRequest string  `json:"memory_request"`
	MemoryLimit   string  `json:"memory_limit"`
	Pattern       string  `json:"pattern"`
	Confidence    float64 `json:"confidence"`
	ReasoningCode string  `json:"reasoning_code"`
	Explanation   string  `json:"explanation"`
	Risk          string  `json:"risk"`
}

var validPatterns = map[string]bool{
	"steady":    true,
	"burstable": true,
	"batch":     true,
	"idle":      true,
	"anomalous": true,
}

var validRisks = map[string]bool{
	"LOW":    true,
	"MEDIUM": true,
	"HIGH":   true,
}

var jsonBlockRe = regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*?\\})\\s*```")

// ParseRecommendation extracts a structured recommendation from raw SLM output.
func ParseRecommendation(raw string) (*SLMOutput, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty SLM response")
	}

	var output SLMOutput

	// Try direct JSON parse first
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		// Try extracting from markdown code block
		if matches := jsonBlockRe.FindStringSubmatch(raw); len(matches) > 1 {
			if err2 := json.Unmarshal([]byte(matches[1]), &output); err2 != nil {
				return nil, fmt.Errorf("parsing JSON from code block: %w", err2)
			}
		} else {
			// Try to find any JSON object in the text
			start := strings.Index(raw, "{")
			end := strings.LastIndex(raw, "}")
			if start >= 0 && end > start {
				if err2 := json.Unmarshal([]byte(raw[start:end+1]), &output); err2 != nil {
					return nil, fmt.Errorf("parsing SLM output as JSON: %w", err)
				}
			} else {
				return nil, fmt.Errorf("no JSON object found in SLM response")
			}
		}
	}

	// Validate required fields
	if output.CPURequest == "" {
		return nil, fmt.Errorf("missing cpu_request in SLM output")
	}
	if output.MemoryRequest == "" {
		return nil, fmt.Errorf("missing memory_request in SLM output")
	}

	// Normalise pattern
	output.Pattern = strings.ToLower(output.Pattern)
	if output.Pattern != "" && !validPatterns[output.Pattern] {
		return nil, fmt.Errorf("invalid pattern %q in SLM output", output.Pattern)
	}

	// Normalise risk
	output.Risk = strings.ToUpper(output.Risk)
	if output.Risk != "" && !validRisks[output.Risk] {
		return nil, fmt.Errorf("invalid risk %q in SLM output", output.Risk)
	}

	// Validate confidence range
	if output.Confidence < 0 || output.Confidence > 1 {
		return nil, fmt.Errorf("confidence %.2f out of range [0,1]", output.Confidence)
	}

	// Bound string field lengths to prevent abuse from malformed SLM output.
	const maxExplanationLen = 1024
	const maxReasoningCodeLen = 64
	if len(output.Explanation) > maxExplanationLen {
		output.Explanation = output.Explanation[:maxExplanationLen]
	}
	if len(output.ReasoningCode) > maxReasoningCodeLen {
		output.ReasoningCode = output.ReasoningCode[:maxReasoningCodeLen]
	}

	return &output, nil
}

// ParseResourceMillis parses a K8s resource string like "500m" or "2" to millicores.
func ParseResourceMillis(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("empty resource value")
	}

	if strings.HasSuffix(value, "m") {
		var millis int64
		_, err := fmt.Sscanf(value, "%dm", &millis)
		if err != nil {
			return 0, fmt.Errorf("parsing millicores %q: %w", value, err)
		}
		return millis, nil
	}

	// Assume whole cores
	var cores float64
	_, err := fmt.Sscanf(value, "%f", &cores)
	if err != nil {
		return 0, fmt.Errorf("parsing cores %q: %w", value, err)
	}
	return int64(cores * 1000), nil
}

// ParseResourceBytes parses a K8s memory string like "256Mi" or "1Gi" to bytes.
func ParseResourceBytes(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("empty resource value")
	}

	units := map[string]int64{
		"Ki": 1024,
		"Mi": 1024 * 1024,
		"Gi": 1024 * 1024 * 1024,
		"Ti": 1024 * 1024 * 1024 * 1024,
	}

	for suffix, multiplier := range units {
		if strings.HasSuffix(value, suffix) {
			numStr := strings.TrimSuffix(value, suffix)
			var num float64
			_, err := fmt.Sscanf(numStr, "%f", &num)
			if err != nil {
				return 0, fmt.Errorf("parsing memory %q: %w", value, err)
			}
			return int64(num * float64(multiplier)), nil
		}
	}

	// Plain bytes
	var b int64
	_, err := fmt.Sscanf(value, "%d", &b)
	if err != nil {
		return 0, fmt.Errorf("parsing bytes %q: %w", value, err)
	}
	return b, nil
}

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
	"github.com/gregorytcarroll/k8s-sage/internal/rules"
	"github.com/gregorytcarroll/k8s-sage/internal/slm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testInput() rules.AnalysisInput {
	return rules.AnalysisInput{
		Owner: models.OwnerReference{
			Kind:      "Deployment",
			Name:      "nginx",
			Namespace: "production",
		},
		ContainerName: "nginx",
		CPUUsageMillis: models.MetricAggregate{
			P50: 120, P95: 180, P99: 250, Max: 400,
		},
		CPURequestMillis: 1000,
		CPULimitMillis:   2000,
		MemUsageBytes: models.MetricAggregate{
			P50: 180 * 1024 * 1024,
			P95: 200 * 1024 * 1024,
			P99: 220 * 1024 * 1024,
			Max: 250 * 1024 * 1024,
		},
		MemRequestBytes:   512 * 1024 * 1024,
		MemLimitBytes:     1024 * 1024 * 1024,
		DataWindowMinutes: 7 * 24 * 60,
	}
}

func TestAnalyzer_L1Only(t *testing.T) {
	engine := rules.NewEngine()
	analyzer := NewAnalyzer(engine, nil)

	result := analyzer.Analyze(context.Background(), testInput())

	// Should get L1 results without error. Pattern depends on rules engine
	// classification of the test input metrics.
	assert.NotEmpty(t, result.Pattern)
	assert.NotNil(t, result.CPURecommendation)
	assert.NotNil(t, result.MemRecommendation)
}

func TestAnalyzer_L2Success(t *testing.T) {
	// Mock SLM server returning valid recommendation
	slmResponse := slm.SLMOutput{
		CPURequest:    "300m",
		CPULimit:      "600m",
		MemoryRequest: "300Mi",
		MemoryLimit:   "600Mi",
		Pattern:       "steady",
		Confidence:    0.92,
		ReasoningCode: "OVER_PROVISIONED",
		Explanation:   "CPU is significantly over-provisioned",
		Risk:          "LOW",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := slm.CompletionResponse{Content: mustJSON(t, slmResponse)}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	engine := rules.NewEngine()
	client := slm.NewClient(server.URL, 5*time.Second)
	analyzer := NewAnalyzer(engine, client)

	result := analyzer.Analyze(context.Background(), testInput())

	assert.Equal(t, models.PatternSteady, result.Pattern)
	assert.NotNil(t, result.CPURecommendation)
	// L2 confidence should be used
	assert.InDelta(t, 0.92, result.CPURecommendation.Confidence, 0.01)
	assert.Contains(t, result.CPURecommendation.Reasoning, "over-provisioned")
}

func TestAnalyzer_L2SafetyViolation(t *testing.T) {
	// SLM returns dangerously low memory recommendation
	slmResponse := slm.SLMOutput{
		CPURequest:    "200m",
		CPULimit:      "400m",
		MemoryRequest: "50Mi", // Way below P99 WS of 220Mi
		MemoryLimit:   "100Mi",
		Pattern:       "steady",
		Confidence:    0.95,
		ReasoningCode: "OPTIMIZE",
		Explanation:   "Aggressive optimization",
		Risk:          "LOW",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := slm.CompletionResponse{Content: mustJSON(t, slmResponse)}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	engine := rules.NewEngine()
	client := slm.NewClient(server.URL, 5*time.Second)
	analyzer := NewAnalyzer(engine, client)

	result := analyzer.Analyze(context.Background(), testInput())

	// Should fall back to L1 result due to safety violation
	assert.NotNil(t, result.CPURecommendation)
	// L1 confidence should be used (not L2's 0.95)
	assert.NotEqual(t, 0.95, result.CPURecommendation.Confidence)
}

func TestAnalyzer_L2Timeout(t *testing.T) {
	// SLM server that hangs
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer server.Close()

	engine := rules.NewEngine()
	client := slm.NewClient(server.URL, 5*time.Second)
	analyzer := NewAnalyzer(engine, client)

	// Use a short context timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result := analyzer.Analyze(ctx, testInput())

	// Should fall back to L1 result
	assert.NotEmpty(t, result.Pattern)
	assert.NotNil(t, result.CPURecommendation)
}

func TestAnalyzer_L2ParseFailure(t *testing.T) {
	// SLM server returns garbage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := slm.CompletionResponse{Content: "This is not JSON at all"}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	engine := rules.NewEngine()
	client := slm.NewClient(server.URL, 5*time.Second)
	analyzer := NewAnalyzer(engine, client)

	result := analyzer.Analyze(context.Background(), testInput())

	// Should fall back to L1 result
	assert.NotNil(t, result.CPURecommendation)
}

func TestAnalyzer_L2ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer server.Close()

	engine := rules.NewEngine()
	client := slm.NewClient(server.URL, 5*time.Second)
	analyzer := NewAnalyzer(engine, client)

	result := analyzer.Analyze(context.Background(), testInput())

	// Should fall back to L1 result
	assert.NotNil(t, result.CPURecommendation)
}

func TestCheckSafetyInvariants_Valid(t *testing.T) {
	output := &slm.SLMOutput{
		CPURequest:    "300m", // Above P95 (180m) × 1.10 = 198m
		MemoryRequest: "300Mi", // Above P99 (220Mi) × 1.10 = 242Mi
	}
	input := testInput()

	violations := checkSafetyInvariants(output, input)
	assert.Empty(t, violations)
}

func TestCheckSafetyInvariants_CPUBelowFloor(t *testing.T) {
	output := &slm.SLMOutput{
		CPURequest:    "150m", // Below P95 (180m) × 1.10 = 198m
		MemoryRequest: "300Mi",
	}
	input := testInput()

	violations := checkSafetyInvariants(output, input)
	assert.NotEmpty(t, violations)
	assert.Contains(t, violations[0], "cpu_request below P95")
}

func TestCheckSafetyInvariants_MemoryBelowFloor(t *testing.T) {
	output := &slm.SLMOutput{
		CPURequest:    "300m",
		MemoryRequest: "200Mi", // Below P99 (220Mi) × 1.10 = 242Mi
	}
	input := testInput()

	violations := checkSafetyInvariants(output, input)
	assert.NotEmpty(t, violations)
	assert.Contains(t, violations[0], "memory_request below P99")
}

func TestCheckSafetyInvariants_ZeroCPU(t *testing.T) {
	output := &slm.SLMOutput{
		CPURequest:    "0m",
		MemoryRequest: "300Mi",
	}
	input := testInput()

	violations := checkSafetyInvariants(output, input)
	assert.NotEmpty(t, violations)
}

func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return string(data)
}

package backtest

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
	"github.com/gregorytcarroll/k8s-sage/internal/rules"
)

// --- JSON schema types (match the fixture file's casing) ---

type backtestSuite struct {
	Scenarios []scenario `json:"scenarios"`
}

type scenario struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Category string   `json:"category"`
	Input    input    `json:"input"`
	Expected expected `json:"expected"`
}

type metricAgg struct {
	P50 float64 `json:"P50"`
	P95 float64 `json:"P95"`
	P99 float64 `json:"P99"`
	Max float64 `json:"Max"`
}

func (m metricAgg) toModel() models.MetricAggregate {
	return models.MetricAggregate{P50: m.P50, P95: m.P95, P99: m.P99, Max: m.Max}
}

type input struct {
	CPUUsage          metricAgg `json:"cpuUsage"`
	CPURequestMillis  int64     `json:"cpuRequestMillis"`
	CPULimitMillis    int64     `json:"cpuLimitMillis"`
	MemUsage          metricAgg `json:"memUsage"`
	MemRequestBytes   int64     `json:"memRequestBytes"`
	MemLimitBytes     int64     `json:"memLimitBytes"`
	DataWindowMinutes float64   `json:"dataWindowMinutes"`
}

type resourceExpected struct {
	RecommendedReq   int64   `json:"recommendedReq"`
	RecommendedLimit int64   `json:"recommendedLimit"`
	EstSavings       int64   `json:"estSavings"`
	Confidence       float64 `json:"confidence"`
	Risk             string  `json:"risk"`
}

type expected struct {
	Pattern           string            `json:"pattern"`
	CPU               *resourceExpected `json:"cpu,omitempty"`
	Memory            *resourceExpected `json:"memory,omitempty"`
	CPURecommendation *bool             `json:"cpuRecommendation,omitempty"`
	MemRecommendation *bool             `json:"memRecommendation,omitempty"`
}

// expectsCPURec returns true if the scenario expects a CPU recommendation.
// If cpuRecommendation is explicitly false, returns false.
// If cpu object exists, returns true.
// Otherwise returns false (no assertion needed).
func (e expected) expectsCPURec() (expect bool, exists bool) {
	if e.CPURecommendation != nil {
		return *e.CPURecommendation, true
	}
	if e.CPU != nil {
		return true, true
	}
	return false, false
}

// expectsMemRec returns true if the scenario expects a memory recommendation.
func (e expected) expectsMemRec() (expect bool, exists bool) {
	if e.MemRecommendation != nil {
		return *e.MemRecommendation, true
	}
	if e.Memory != nil {
		return true, true
	}
	return false, false
}

func TestBacktestScenarios(t *testing.T) {
	data, err := os.ReadFile("../fixtures/backtest_scenarios.json")
	if err != nil {
		t.Fatalf("failed to read scenarios file: %v", err)
	}

	var suite backtestSuite
	if err := json.Unmarshal(data, &suite); err != nil {
		t.Fatalf("failed to parse scenarios: %v", err)
	}

	if len(suite.Scenarios) < 50 {
		t.Fatalf("expected at least 50 scenarios, got %d", len(suite.Scenarios))
	}

	engine := rules.NewEngine()

	for _, sc := range suite.Scenarios {
		t.Run(sc.ID+"_"+sc.Name, func(t *testing.T) {
			result := engine.Analyze(rules.AnalysisInput{
				Owner:             models.OwnerReference{Kind: "Deployment", Name: "backtest-" + sc.ID, Namespace: "backtest"},
				ContainerName:     "main",
				CPUUsageMillis:    sc.Input.CPUUsage.toModel(),
				CPURequestMillis:  sc.Input.CPURequestMillis,
				CPULimitMillis:    sc.Input.CPULimitMillis,
				MemUsageBytes:     sc.Input.MemUsage.toModel(),
				MemRequestBytes:   sc.Input.MemRequestBytes,
				MemLimitBytes:     sc.Input.MemLimitBytes,
				DataWindowMinutes: sc.Input.DataWindowMinutes,
			})

			// 1. Check pattern classification.
			if string(result.Pattern) != sc.Expected.Pattern {
				t.Errorf("pattern: got %q, want %q", result.Pattern, sc.Expected.Pattern)
			}

			// 2. Check CPU recommendation.
			wantCPU, hasCPUAssertion := sc.Expected.expectsCPURec()
			if hasCPUAssertion {
				if wantCPU {
					if result.CPURecommendation == nil {
						t.Fatal("expected CPU recommendation, got nil")
					}
					checkResourceRec(t, "cpu", result.CPURecommendation, sc.Expected.CPU)
				} else {
					if result.CPURecommendation != nil {
						t.Errorf("expected no CPU recommendation, got req=%d limit=%d savings=%d",
							result.CPURecommendation.RecommendedReq,
							result.CPURecommendation.RecommendedLimit,
							result.CPURecommendation.EstSavings)
					}
				}
			}

			// 3. Check memory recommendation.
			wantMem, hasMemAssertion := sc.Expected.expectsMemRec()
			if hasMemAssertion {
				if wantMem {
					if result.MemRecommendation == nil {
						t.Fatal("expected memory recommendation, got nil")
					}
					checkResourceRec(t, "memory", result.MemRecommendation, sc.Expected.Memory)
				} else {
					if result.MemRecommendation != nil {
						t.Errorf("expected no memory recommendation, got req=%d limit=%d savings=%d",
							result.MemRecommendation.RecommendedReq,
							result.MemRecommendation.RecommendedLimit,
							result.MemRecommendation.EstSavings)
					}
				}
			}
		})
	}
}

func checkResourceRec(t *testing.T, resource string, got *models.Recommendation, want *resourceExpected) {
	t.Helper()
	if want == nil {
		return // no fields to check
	}

	// Allow ±1 tolerance on integer fields for IEEE 754 rounding in ceil().
	if !withinTolerance(got.RecommendedReq, want.RecommendedReq, 1) {
		t.Errorf("%s recommendedReq: got %d, want %d (±1)", resource, got.RecommendedReq, want.RecommendedReq)
	}
	if !withinTolerance(got.RecommendedLimit, want.RecommendedLimit, 1) {
		t.Errorf("%s recommendedLimit: got %d, want %d (±1)", resource, got.RecommendedLimit, want.RecommendedLimit)
	}
	if !withinTolerance(got.EstSavings, want.EstSavings, 1) {
		t.Errorf("%s estSavings: got %d, want %d (±1)", resource, got.EstSavings, want.EstSavings)
	}
	if want.Confidence > 0 {
		if math.Abs(got.Confidence-want.Confidence) > 0.011 {
			t.Errorf("%s confidence: got %.4f, want %.2f (±0.01)", resource, got.Confidence, want.Confidence)
		}
	}
	if want.Risk != "" {
		if string(got.Risk) != want.Risk {
			t.Errorf("%s risk: got %q, want %q", resource, got.Risk, want.Risk)
		}
	}
}

func withinTolerance(got, want int64, tol int64) bool {
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	return diff <= tol
}

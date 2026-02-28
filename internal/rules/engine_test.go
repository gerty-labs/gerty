package rules

import (
	"testing"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestEngine_Analyze_ClampNegativeValues(t *testing.T) {
	engine := NewEngine()

	input := AnalysisInput{
		Owner:         models.OwnerReference{Kind: "Deployment", Name: "test", Namespace: "default"},
		ContainerName: "main",
		CPUUsageMillis: models.MetricAggregate{
			P50: -100, P95: -200, P99: -300, Max: -400,
		},
		CPURequestMillis:  -500,
		CPULimitMillis:    -1000,
		MemUsageBytes:     models.MetricAggregate{P50: -100, P95: -200, P99: -300, Max: -400},
		MemRequestBytes:   -500,
		MemLimitBytes:     -1000,
		DataWindowMinutes: -10,
	}

	// Should not panic — negatives clamped to zero.
	result := engine.Analyze(input)

	// With everything clamped to zero, default pattern should be Steady.
	assert.Equal(t, models.PatternSteady, result.Pattern)
	// No recommendation because requests are zero.
	assert.Nil(t, result.CPURecommendation)
	assert.Nil(t, result.MemRecommendation)
}

func TestEngine_Analyze_SteadyWithClearWaste(t *testing.T) {
	engine := NewEngine()

	// CPU usage with low CV: (220-200)/200 = 0.1 < 0.3 → Steady.
	// P50/request = 200/2000 = 0.1 ≥ 0.05 → not idle.
	input := AnalysisInput{
		Owner:         models.OwnerReference{Kind: "Deployment", Name: "api", Namespace: "prod"},
		ContainerName: "main",
		CPUUsageMillis: models.MetricAggregate{
			P50: 200, P95: 220, P99: 250, Max: 300,
		},
		CPURequestMillis: 2000,
		CPULimitMillis:   4000,
		MemUsageBytes: models.MetricAggregate{
			P50: 100_000_000, P95: 200_000_000, P99: 250_000_000, Max: 300_000_000,
		},
		MemRequestBytes:   2_000_000_000,
		MemLimitBytes:     4_000_000_000,
		DataWindowMinutes: 10080, // 7 days
	}

	result := engine.Analyze(input)

	assert.Equal(t, models.PatternSteady, result.Pattern)
	assert.NotNil(t, result.CPURecommendation)
	assert.NotNil(t, result.MemRecommendation)
	assert.Equal(t, "cpu", result.CPURecommendation.Resource)
	assert.Equal(t, "memory", result.MemRecommendation.Resource)
	assert.Greater(t, result.CPURecommendation.EstSavings, int64(0))
	assert.Greater(t, result.MemRecommendation.EstSavings, int64(0))
}

func TestEngine_Analyze_SingleDataPoint(t *testing.T) {
	engine := NewEngine()

	input := AnalysisInput{
		Owner:         models.OwnerReference{Kind: "Deployment", Name: "new-app", Namespace: "staging"},
		ContainerName: "app",
		CPUUsageMillis: models.MetricAggregate{
			P50: 50, P95: 50, P99: 50, Max: 50,
		},
		CPURequestMillis:  1000,
		CPULimitMillis:    2000,
		DataWindowMinutes: 0.5, // 30 seconds
	}

	result := engine.Analyze(input)

	// Should still classify and potentially recommend, but with low confidence.
	if result.CPURecommendation != nil {
		assert.LessOrEqual(t, result.CPURecommendation.Confidence, confidenceMaxShortWindow)
	}
}

func TestEngine_Analyze_NoWaste(t *testing.T) {
	engine := NewEngine()

	input := AnalysisInput{
		Owner:         models.OwnerReference{Kind: "Deployment", Name: "tight", Namespace: "prod"},
		ContainerName: "app",
		CPUUsageMillis: models.MetricAggregate{
			P50: 450, P95: 480, P99: 490, Max: 500,
		},
		CPURequestMillis:  500,
		DataWindowMinutes: 10080,
	}

	result := engine.Analyze(input)

	// Usage is very close to request — no recommendation expected.
	assert.Nil(t, result.CPURecommendation)
}

func TestEngine_AnalyzeCluster(t *testing.T) {
	engine := NewEngine()

	report := models.ClusterReport{
		Namespaces: map[string]*models.NamespaceReport{
			"default": {
				Namespace: "default",
				Owners: []models.OwnerWaste{
					{
						Owner:    models.OwnerReference{Kind: "Deployment", Name: "web", Namespace: "default"},
						PodCount: 3,
						Containers: []models.ContainerWaste{
							{
								ContainerName:      "nginx",
								CPURequestMillis:   2000,
								CPUUsage:           models.MetricAggregate{P50: 100, P95: 200, P99: 250, Max: 300},
								MemoryRequestBytes: 2_000_000_000,
								MemoryUsage:        models.MetricAggregate{P50: 100_000_000, P95: 200_000_000, P99: 250_000_000, Max: 300_000_000},
								DataWindow:         7 * 24 * 60 * 60 * 1_000_000_000, // 7 days in ns
							},
						},
					},
				},
			},
		},
	}

	recs := engine.AnalyzeCluster(report)

	// Should get at least one CPU recommendation for the wasteful workload.
	assert.NotEmpty(t, recs)

	// Verify the target is correct.
	for _, rec := range recs {
		assert.Equal(t, "Deployment", rec.Target.Kind)
		assert.Equal(t, "web", rec.Target.Name)
		assert.Equal(t, "default", rec.Target.Namespace)
	}
}

func TestEngine_AnalyzeCluster_EmptyReport(t *testing.T) {
	engine := NewEngine()

	report := models.ClusterReport{
		Namespaces: make(map[string]*models.NamespaceReport),
	}

	recs := engine.AnalyzeCluster(report)
	assert.Empty(t, recs)
}

func TestClampAggregate(t *testing.T) {
	agg := models.MetricAggregate{P50: -10, P95: 0, P99: -5, Max: 100}
	clamped := clampAggregate(agg)

	assert.Equal(t, float64(0), clamped.P50)
	assert.Equal(t, float64(0), clamped.P95)
	assert.Equal(t, float64(0), clamped.P99)
	assert.Equal(t, float64(100), clamped.Max)
}

func TestClampZero(t *testing.T) {
	assert.Equal(t, float64(0), clampZero(-1))
	assert.Equal(t, float64(0), clampZero(0))
	assert.Equal(t, float64(42), clampZero(42))
}

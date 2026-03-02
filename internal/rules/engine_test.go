package rules

import (
	"testing"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	// Requests are zero → BestEffort recommendations (not nil).
	require.NotNil(t, result.CPURecommendation)
	assert.Contains(t, result.CPURecommendation.Reasoning, "BestEffort")
	require.NotNil(t, result.MemRecommendation)
	assert.Contains(t, result.MemRecommendation.Reasoning, "BestEffort")
}

func TestEngine_Analyze_SteadyWithClearWaste(t *testing.T) {
	engine := NewEngine()

	// CPU usage with low CV: (220-200)/200 = 0.1 < 0.3 -> Steady.
	// P50/request = 200/2000 = 0.1 >= 0.05 -> not idle.
	// CPU: recReq = P95*1.20 = 264. Conf=0.95 (>0.8 → cap=75%, floor=500).
	//   264 < 500 → capped to 500. RecLimit = max(300, 500) = 500.
	//   savings = 2000 - 500 = 1500.
	// Mem: recReq = P99*1.20 = 300M. MemConf=0.90 (>0.8 → cap=75%, floor=500M).
	//   300M < 500M → capped to 500M. RecLimit = max(360M, 500M) = 500M.
	//   savings = 2B - 500M = 1.5B.
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
	require.NotNil(t, result.CPURecommendation)
	require.NotNil(t, result.MemRecommendation)

	// CPU recommendation: capped at 75% reduction
	assert.Equal(t, "cpu", result.CPURecommendation.Resource)
	assert.Equal(t, int64(500), result.CPURecommendation.RecommendedReq)
	assert.Equal(t, int64(500), result.CPURecommendation.RecommendedLimit)
	assert.Equal(t, int64(1500), result.CPURecommendation.EstSavings)
	assert.InDelta(t, 0.95, result.CPURecommendation.Confidence, 0.01)

	// Memory recommendation: capped at 75% reduction
	assert.Equal(t, "memory", result.MemRecommendation.Resource)
	assert.Equal(t, int64(500_000_000), result.MemRecommendation.RecommendedReq)
	assert.Equal(t, int64(500_000_000), result.MemRecommendation.RecommendedLimit)
	assert.Equal(t, int64(1_500_000_000), result.MemRecommendation.EstSavings)
	assert.InDelta(t, 0.90, result.MemRecommendation.Confidence, 0.01)
}

func TestEngine_Analyze_SingleDataPoint(t *testing.T) {
	engine := NewEngine()

	// CPU: P50=P95=P99=Max=50, CV=0 -> Steady
	// RecReq = P95 * 1.20 = 60. Confidence ≈ 0.20 (<0.5 → cap=30%, floor=700).
	// 60 < 700 → capped to 700. Savings = 1000 - 700 = 300.
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

	// With waste and a clear request, recommendation produced but capped conservatively.
	require.NotNil(t, result.CPURecommendation, "waste should produce a recommendation even with short data window")
	assert.Equal(t, int64(700), result.CPURecommendation.RecommendedReq)
	assert.Equal(t, int64(700), result.CPURecommendation.RecommendedLimit)
	assert.Equal(t, int64(300), result.CPURecommendation.EstSavings)
	assert.LessOrEqual(t, result.CPURecommendation.Confidence, confidenceMaxShortWindow)
	assert.InDelta(t, 0.20, result.CPURecommendation.Confidence, 0.01)
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

	// Usage is very close to request — under-provisioned recommendation expected.
	require.NotNil(t, result.CPURecommendation)
	assert.Contains(t, result.CPURecommendation.Reasoning, "Under-provisioned")
	assert.Equal(t, int64(0), result.CPURecommendation.EstSavings)
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

	// Should get exactly 2 recommendations: 1 CPU + 1 memory.
	require.Len(t, recs, 2)

	// Verify all target the correct workload and include both resource types.
	resourcesSeen := make(map[string]bool)
	for _, rec := range recs {
		assert.Equal(t, "Deployment", rec.Target.Kind)
		assert.Equal(t, "web", rec.Target.Name)
		assert.Equal(t, "default", rec.Target.Namespace)
		assert.Greater(t, rec.EstSavings, int64(0))
		resourcesSeen[rec.Resource] = true
	}
	assert.True(t, resourcesSeen["cpu"], "expected a CPU recommendation")
	assert.True(t, resourcesSeen["memory"], "expected a memory recommendation")
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

func TestEngine_Analyze_AnomalousPattern(t *testing.T) {
	engine := NewEngine()

	// CPU is steady, but memory shows leak pattern.
	input := AnalysisInput{
		Owner:         models.OwnerReference{Kind: "Deployment", Name: "leaky-app", Namespace: "prod"},
		ContainerName: "main",
		CPUUsageMillis: models.MetricAggregate{
			P50: 200, P95: 220, P99: 250, Max: 300,
		},
		CPURequestMillis: 2000,
		CPULimitMillis:   4000,
		MemUsageBytes: models.MetricAggregate{
			P50: 10_485_760,   // 10Mi
			P95: 150_000_000,  // ~143Mi
			P99: 209_715_200,  // 200Mi
			Max: 220_200_960,  // ~210Mi — close to P99
		},
		MemRequestBytes:   1_000_000_000,
		MemLimitBytes:     2_000_000_000,
		DataWindowMinutes: 10080,
	}

	result := engine.Analyze(input)

	// Memory anomaly overrides CPU classification.
	assert.Equal(t, models.PatternAnomalous, result.Pattern)

	// Both recommendations should be investigation-type.
	require.NotNil(t, result.CPURecommendation)
	assert.Equal(t, models.PatternAnomalous, result.CPURecommendation.Pattern)
	assert.Equal(t, models.RiskHigh, result.CPURecommendation.Risk)
	assert.Equal(t, int64(0), result.CPURecommendation.EstSavings)

	require.NotNil(t, result.MemRecommendation)
	assert.Equal(t, models.PatternAnomalous, result.MemRecommendation.Pattern)
	assert.Equal(t, models.RiskHigh, result.MemRecommendation.Risk)
	assert.Equal(t, int64(0), result.MemRecommendation.EstSavings)
	assert.Contains(t, result.MemRecommendation.Reasoning, "memory leak")
}

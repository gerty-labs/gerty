package rules

import (
	"math"
	"testing"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testOwner = models.OwnerReference{
	Kind: "Deployment", Name: "api-server", Namespace: "production",
}

// assertSafetyInvariant_CPU verifies the key safety invariant: for any CPU
// recommendation, the recommended request must be >= P95 * the minimum headroom
// factor for its pattern, and >= the minimum floor.
func assertSafetyInvariant_CPU(t *testing.T, rec *models.Recommendation, usage models.MetricAggregate) {
	t.Helper()
	if rec == nil {
		return
	}
	// Minimum headroom varies by pattern; the smallest is 1.20 (steady).
	minHeadroom := 1.20
	switch rec.Pattern {
	case models.PatternBurstable, models.PatternBatch:
		// Burstable/batch request uses P50 * 1.20, but the safety invariant
		// is that the recommendation should at least cover P50 with headroom.
		minExpected := math.Ceil(usage.P50 * headroomBurstableReq)
		assert.GreaterOrEqual(t, rec.RecommendedReq, int64(minExpected),
			"safety: recommended request must be >= P50 * %.2f for %s pattern", headroomBurstableReq, rec.Pattern)
	default:
		// Steady/idle: request must be >= P95 * 1.20
		minExpected := math.Ceil(usage.P95 * minHeadroom)
		if minExpected < float64(minRecommendedCPUMillis) {
			minExpected = float64(minRecommendedCPUMillis)
		}
		assert.GreaterOrEqual(t, rec.RecommendedReq, int64(minExpected),
			"safety: recommended request must be >= max(P95 * %.2f, %d)", minHeadroom, minRecommendedCPUMillis)
	}
	assert.GreaterOrEqual(t, rec.RecommendedReq, int64(minRecommendedCPUMillis),
		"safety: recommended request must be >= minimum floor %dm", minRecommendedCPUMillis)
}

// assertSafetyInvariant_Memory verifies the key safety invariant for memory
// recommendations: recommended must be >= P99 * headroom and >= minimum floor.
func assertSafetyInvariant_Memory(t *testing.T, rec *models.Recommendation, usage models.MetricAggregate) {
	t.Helper()
	if rec == nil {
		return
	}
	minExpected := math.Ceil(usage.P99 * headroomSteady)
	if minExpected < float64(minRecommendedMemBytes) {
		minExpected = float64(minRecommendedMemBytes)
	}
	assert.GreaterOrEqual(t, rec.RecommendedReq, int64(minExpected),
		"safety: memory recommended request must be >= max(P99 * %.2f, %d)", headroomSteady, minRecommendedMemBytes)
}

func TestGenerateCPURecommendation_SteadyWorkload(t *testing.T) {
	// P50=200, P95=220 -> CV = (220-200)/200 = 0.1 < 0.3 -> Steady.
	// P50/req = 200/2000 = 0.1 >= 0.05 -> not idle.
	// RecReq = P95 * 1.20 = 220 * 1.20 = 264
	// Confidence: steady, 10080 min (>= 7d) -> 0.95 (>0.8 → cap=75%, floor=500)
	// RecReq 264 < 500 → capped to 500. RecLimit = max(240, 500) = 500.
	// Savings = 2000 - 500 = 1500.
	// Risk: recReq/P99 = 500/200 = 2.5 > 1.10 -> LOW
	usage := models.MetricAggregate{P50: 200, P95: 220, P99: 200, Max: 300}

	rec := GenerateCPURecommendation(testOwner, "main", usage, 2000, 4000, 10080)
	require.NotNil(t, rec)

	assert.Equal(t, "cpu", rec.Resource)
	assert.Equal(t, models.PatternSteady, rec.Pattern)
	assert.Equal(t, int64(500), rec.RecommendedReq)
	assert.Equal(t, int64(500), rec.RecommendedLimit)
	assert.Equal(t, int64(1500), rec.EstSavings)
	assert.Equal(t, models.RiskLow, rec.Risk)
	assert.InDelta(t, 0.95, rec.Confidence, 0.01)
	assert.Contains(t, rec.Reasoning, "Steady")
	assert.Contains(t, rec.Reasoning, "reduction capped")
	assertSafetyInvariant_CPU(t, rec, usage)
}

func TestGenerateCPURecommendation_BurstableWorkload(t *testing.T) {
	// P50/req = 200/2000 = 0.1 -> not idle. CV = (400-200)/200 = 1.0 > 0.3 -> Burstable.
	// Not batch: P99/P50 = 600/200 = 3 < 5.
	// RecReq = P50 * 1.20 = 200 * 1.20 = 240
	// RecLimit = P99 * 1.20 = 600 * 1.20 = 720
	// Confidence: burstable, 10080 min -> capped at 0.80 (not > 0.8 → cap=50%, floor=1000)
	// RecReq 240 < 1000 → capped to 1000. RecLimit = max(720, 1000) = 1000.
	// Savings = 2000 - 1000 = 1000.
	// Risk: burstable compares limit to P99: recLimit/P99 = 1000/600 = 1.67 > 1.10 -> LOW
	usage := models.MetricAggregate{P50: 200, P95: 400, P99: 600, Max: 800}

	rec := GenerateCPURecommendation(testOwner, "main", usage, 2000, 4000, 10080)
	require.NotNil(t, rec)

	assert.Equal(t, models.PatternBurstable, rec.Pattern)
	assert.Equal(t, int64(1000), rec.RecommendedReq)
	assert.Equal(t, int64(1000), rec.RecommendedLimit)
	assert.Equal(t, int64(1000), rec.EstSavings)
	assert.InDelta(t, 0.80, rec.Confidence, 0.01)
	assert.Equal(t, models.RiskLow, rec.Risk)
	assert.Contains(t, rec.Reasoning, "Burstable")
	assert.Contains(t, rec.Reasoning, "reduction capped")
	assertSafetyInvariant_CPU(t, rec, usage)
}

func TestGenerateCPURecommendation_BatchWorkload(t *testing.T) {
	// P50/request = 100/2000 = 0.05 so not idle (needs < 0.05).
	// P99/P50 = 800/100 = 8 >= 5, Max/P50 = 1500/100 = 15 >= 10 -> Batch.
	// RecReq = P50 * 1.20 = 100 * 1.20 = 120
	// RecLimit = Max * 1.20 = 1500 * 1.20 = 1800
	// Confidence: batch, 10080 min -> capped at 0.70 (>0.5 → cap=50%, floor=1000)
	// RecReq 120 < 1000 → capped to 1000. RecLimit = max(1800, 1000) = 1800.
	// Savings = 2000 - 1000 = 1000.
	// Risk: batch compares limit to Max: recLimit/Max = 1800/1500 = 1.20 >= 1.10 -> LOW
	usage := models.MetricAggregate{P50: 100, P95: 400, P99: 800, Max: 1500}

	rec := GenerateCPURecommendation(testOwner, "main", usage, 2000, 4000, 10080)
	require.NotNil(t, rec)

	assert.Equal(t, models.PatternBatch, rec.Pattern)
	assert.Equal(t, int64(1000), rec.RecommendedReq)
	assert.Equal(t, int64(1800), rec.RecommendedLimit)
	assert.Equal(t, int64(1000), rec.EstSavings)
	assert.InDelta(t, 0.70, rec.Confidence, 0.01)
	assert.Equal(t, models.RiskLow, rec.Risk)
	assert.Contains(t, rec.Reasoning, "Batch")
	assert.Contains(t, rec.Reasoning, "reduction capped")
	assertSafetyInvariant_CPU(t, rec, usage)
}

func TestGenerateCPURecommendation_IdleWorkload(t *testing.T) {
	// P50=2, P95=5, P99=8, Max=10
	// P50/req = 2/1000 = 0.002 < 0.05, dataWindow=5000 > 2880 -> Idle.
	// RecReq = max(P95 * 1.20, 50) = max(6, 50) = 50
	// Confidence: idle, 5000 min -> 0.86 (>0.8 → cap=75%, floor=250)
	// RecReq 50 < 250 → capped to 250. RecLimit = max(50, 250) = 250.
	// Savings = 1000 - 250 = 750.
	// Risk: recReq/P99 = 250/8 = 31.25 > 1.10 -> LOW
	usage := models.MetricAggregate{P50: 2, P95: 5, P99: 8, Max: 10}

	rec := GenerateCPURecommendation(testOwner, "main", usage, 1000, 2000, 5000)
	require.NotNil(t, rec)

	assert.Equal(t, models.PatternIdle, rec.Pattern)
	assert.Equal(t, int64(250), rec.RecommendedReq)
	assert.Equal(t, int64(250), rec.RecommendedLimit)
	assert.Equal(t, int64(750), rec.EstSavings)
	assert.Equal(t, models.RiskLow, rec.Risk)
	assert.InDelta(t, 0.86, rec.Confidence, 0.01)
	assert.Contains(t, rec.Reasoning, "idle")
	assert.Contains(t, rec.Reasoning, "reduction capped")
	assertSafetyInvariant_CPU(t, rec, usage)
}

func TestGenerateCPURecommendation_ZeroRequest_BestEffort(t *testing.T) {
	usage := models.MetricAggregate{P50: 100, P95: 200, P99: 250, Max: 300}

	rec := GenerateCPURecommendation(testOwner, "main", usage, 0, 0, 10080)
	require.NotNil(t, rec, "best-effort pod should get a recommendation")
	assert.Equal(t, models.RiskHigh, rec.Risk)
	assert.Contains(t, rec.Reasoning, "BestEffort")
	assert.Equal(t, int64(0), rec.CurrentRequest)
	assert.Greater(t, rec.RecommendedReq, int64(0))
}

func TestGenerateCPURecommendation_NegativeRequest_BestEffort(t *testing.T) {
	usage := models.MetricAggregate{P50: 100, P95: 200, P99: 250, Max: 300}

	rec := GenerateCPURecommendation(testOwner, "main", usage, -100, 0, 10080)
	require.NotNil(t, rec, "negative request treated as best-effort")
	assert.Contains(t, rec.Reasoning, "BestEffort")
}

func TestGenerateCPURecommendation_UsageExceedsRequest(t *testing.T) {
	// P95 > request. Burstable: recReq = P50*1.20 = 960. Savings = 1000-960 = 40 (4% < 10% → well-sized).
	usage := models.MetricAggregate{P50: 800, P95: 1200, P99: 1500, Max: 2000}

	rec := GenerateCPURecommendation(testOwner, "main", usage, 1000, 2000, 10080)
	require.NotNil(t, rec, "should get well-sized recommendation")
	assert.Contains(t, rec.Reasoning, "Well-sized")
	assert.Equal(t, int64(0), rec.EstSavings)
	assert.Equal(t, models.RiskLow, rec.Risk)
}

func TestGenerateCPURecommendation_AlreadyRightSized(t *testing.T) {
	// Waste < 10% -- below wasteThresholdPercent.
	usage := models.MetricAggregate{P50: 400, P95: 450, P99: 470, Max: 490}

	// Request = 500, P95 = 450, headroom = 540. Savings = 500 - 540 < 0 → under-provisioned.
	rec := GenerateCPURecommendation(testOwner, "main", usage, 500, 1000, 10080)
	require.NotNil(t, rec, "under-provisioned workload should get recommendation")
	assert.Equal(t, int64(0), rec.EstSavings)
}

func TestGenerateCPURecommendation_MinimumFloor(t *testing.T) {
	// Steady workload with very low usage. Floor test now interacts with cap.
	// P50=30, P95=35 -> CV = (35-30)/30 = 0.167 < 0.3 -> Steady.
	// P50=30 >= lowUsageP50Floor=25, so near-zero guard doesn't fire.
	// P50/req = 30/1000 = 0.03 < 0.05 but dataWindow=1000 < 2880 -> not idle.
	// Steady: RecReq = P95 * 1.20 = 42.
	// Confidence: steady, 1000 min (<3d) -> 0.20 + (1000/4320)*0.30 ≈ 0.27 (<0.5 → cap=30%, floor=700)
	// RecReq 42 < 700 → capped to 700. Savings = 1000-700 = 300.
	usage := models.MetricAggregate{P50: 30, P95: 35, P99: 40, Max: 45}

	rec := GenerateCPURecommendation(testOwner, "main", usage, 1000, 2000, 1000)
	require.NotNil(t, rec)

	assert.Equal(t, int64(700), rec.RecommendedReq)
	assert.Equal(t, int64(700), rec.RecommendedLimit)
	assert.Equal(t, int64(300), rec.EstSavings)
	assert.Equal(t, models.PatternSteady, rec.Pattern)
	assert.Contains(t, rec.Reasoning, "reduction capped")
	assertSafetyInvariant_CPU(t, rec, usage)
}

func TestGenerateCPURecommendation_ZeroUsage_WithEnoughData(t *testing.T) {
	// Zero usage with request=1000 and 83h data window (> 48h) -> Idle.
	// RecReq = max(0, 50) = 50. Confidence=0.86 (>0.8 → cap=75%, floor=250).
	// RecReq 50 < 250 → capped to 250. Savings = 1000 - 250 = 750.
	usage := models.MetricAggregate{P50: 0, P95: 0, P99: 0, Max: 0}

	rec := GenerateCPURecommendation(testOwner, "main", usage, 1000, 2000, 5000)
	require.NotNil(t, rec)

	assert.Equal(t, int64(250), rec.RecommendedReq)
	assert.Equal(t, models.PatternIdle, rec.Pattern)
	assert.Equal(t, int64(750), rec.EstSavings)
	assertSafetyInvariant_CPU(t, rec, usage)
}

func TestGenerateCPURecommendation_ZeroUsage_ShortWindow(t *testing.T) {
	// Zero usage but only 1h data -> not enough for idle, defaults to Steady.
	// RecReq = max(0, 50) = 50. Confidence ≈ 0.20 (<0.5 → cap=30%, floor=700).
	// RecReq 50 < 700 → capped to 700. Savings = 1000 - 700 = 300.
	usage := models.MetricAggregate{P50: 0, P95: 0, P99: 0, Max: 0}

	rec := GenerateCPURecommendation(testOwner, "main", usage, 1000, 2000, 60)
	require.NotNil(t, rec)

	assert.Equal(t, int64(700), rec.RecommendedReq)
	assert.Equal(t, models.PatternSteady, rec.Pattern)
	assert.Equal(t, int64(300), rec.EstSavings)
	assertSafetyInvariant_CPU(t, rec, usage)
}

func TestGenerateMemoryRecommendation_SteadyWorkload(t *testing.T) {
	// Steady memory: RecReq = P99 * 1.20 = 250_000_000 * 1.20 = 300_000_000
	// RecLimit = Max * 1.20 = 300_000_000 * 1.20 = 360_000_000
	// Savings = 1_000_000_000 - 300_000_000 = 700_000_000
	// Confidence: 0.95 * 0.95 = 0.9025, round -> 0.90
	// Risk: recReq/Max = 300M/300M = 1.0, >= 1.0 but < 1.10 -> MEDIUM
	memUsage := models.MetricAggregate{
		P50: 100_000_000,
		P95: 200_000_000,
		P99: 250_000_000,
		Max: 300_000_000,
	}

	rec := GenerateMemoryRecommendation(
		testOwner, "main", memUsage,
		1_000_000_000, 2_000_000_000,
		10080, models.PatternSteady,
	)
	require.NotNil(t, rec)

	assert.Equal(t, "memory", rec.Resource)
	assert.Equal(t, int64(300_000_000), rec.RecommendedReq)
	assert.Equal(t, int64(360_000_000), rec.RecommendedLimit)
	assert.Equal(t, int64(700_000_000), rec.EstSavings)
	assert.InDelta(t, 0.90, rec.Confidence, 0.01)
	assert.Equal(t, models.RiskMedium, rec.Risk)
	assert.Contains(t, rec.Reasoning, "Steady")
	assertSafetyInvariant_Memory(t, rec, memUsage)
}

func TestGenerateMemoryRecommendation_ZeroRequest_BestEffort(t *testing.T) {
	memUsage := models.MetricAggregate{P50: 100, P95: 200, P99: 250, Max: 300}

	rec := GenerateMemoryRecommendation(
		testOwner, "main", memUsage,
		0, 0, 10080, models.PatternSteady,
	)
	require.NotNil(t, rec, "best-effort pod should get memory recommendation")
	assert.Equal(t, models.RiskHigh, rec.Risk)
	assert.Contains(t, rec.Reasoning, "BestEffort")
	assert.Equal(t, int64(0), rec.CurrentRequest)
}

func TestGenerateMemoryRecommendation_MinimumFloor(t *testing.T) {
	// Steady: RecReq = P99 * 1.20 = 300 * 1.20 = 360
	// Confidence = 0.95 * 0.95 = 0.9025 ≈ 0.90 (>0.8 → cap=75%, floor=250_000_000)
	// RecReq 360 < 250_000_000 → capped to 250_000_000. Floor (64MiB) not needed.
	memUsage := models.MetricAggregate{P50: 100, P95: 200, P99: 300, Max: 500}

	rec := GenerateMemoryRecommendation(
		testOwner, "main", memUsage,
		1_000_000_000, 2_000_000_000,
		10080, models.PatternSteady,
	)
	require.NotNil(t, rec)
	assert.Equal(t, int64(250_000_000), rec.RecommendedReq)
	assert.Contains(t, rec.Reasoning, "reduction capped")
	assertSafetyInvariant_Memory(t, rec, memUsage)
}

func TestComputeConfidence(t *testing.T) {
	tests := []struct {
		name string
		stats WorkloadStats
		want  float64
	}{
		{
			name:  "7+ days steady -- highest confidence",
			stats: WorkloadStats{Pattern: models.PatternSteady, DataWindowMinutes: 10080},
			want:  0.95,
		},
		{
			name:  "3-7 days steady -- interpolated confidence",
			stats: WorkloadStats{Pattern: models.PatternSteady, DataWindowMinutes: 5000},
			want:  0.86,
		},
		{
			name:  "less than 3 days -- low confidence",
			stats: WorkloadStats{Pattern: models.PatternSteady, DataWindowMinutes: 1440},
			want:  0.30,
		},
		{
			name:  "zero data window",
			stats: WorkloadStats{Pattern: models.PatternSteady, DataWindowMinutes: 0},
			want:  0.20,
		},
		{
			name:  "burstable capped at 0.80",
			stats: WorkloadStats{Pattern: models.PatternBurstable, DataWindowMinutes: 10080},
			want:  0.80,
		},
		{
			name:  "batch capped at 0.70",
			stats: WorkloadStats{Pattern: models.PatternBatch, DataWindowMinutes: 10080},
			want:  0.70,
		},
		{
			name:  "idle with good data -- high confidence",
			stats: WorkloadStats{Pattern: models.PatternIdle, DataWindowMinutes: 10080},
			want:  0.95,
		},
		{
			name:  "exactly 3 days boundary",
			stats: WorkloadStats{Pattern: models.PatternSteady, DataWindowMinutes: 4320},
			want:  0.85,
		},
		{
			name:  "exactly 1 day boundary",
			stats: WorkloadStats{Pattern: models.PatternSteady, DataWindowMinutes: 1440},
			want:  0.30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeConfidence(tt.stats)
			assert.InDelta(t, tt.want, got, 0.01, "confidence mismatch")
		})
	}
}

func TestComputeRisk(t *testing.T) {
	tests := []struct {
		name   string
		recReq float64
		p99    float64
		want   models.RiskLevel
	}{
		{
			name:   "well above P99 -- LOW",
			recReq: 500,
			p99:    200,
			want:   models.RiskLow,
		},
		{
			name:   "close to P99 -- MEDIUM",
			recReq: 210,
			p99:    200,
			want:   models.RiskMedium, // 210/200 = 1.05 < 1.10
		},
		{
			name:   "at or below P99 -- HIGH",
			recReq: 190,
			p99:    200,
			want:   models.RiskHigh, // 190/200 = 0.95 < 1.0
		},
		{
			name:   "exactly at P99 -- MEDIUM",
			recReq: 200,
			p99:    200,
			want:   models.RiskMedium, // 200/200 = 1.0, not < 1.0 so not HIGH, but < 1.10 so MEDIUM
		},
		{
			name:   "exactly at 1.10 threshold -- LOW",
			recReq: 220,
			p99:    200,
			want:   models.RiskLow, // 220/200 = 1.10, not < 1.10 so LOW
		},
		{
			name:   "zero P99 -- LOW",
			recReq: 100,
			p99:    0,
			want:   models.RiskLow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := WorkloadStats{Pattern: models.PatternSteady}
			got := computeRisk(tt.recReq, tt.p99, stats)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestComputeMemoryRisk(t *testing.T) {
	tests := []struct {
		name   string
		recReq float64
		max    float64
		want   models.RiskLevel
	}{
		{
			name:   "well above max -- LOW",
			recReq: 500_000_000,
			max:    200_000_000,
			want:   models.RiskLow,
		},
		{
			name:   "close to max -- MEDIUM",
			recReq: 210_000_000,
			max:    200_000_000,
			want:   models.RiskMedium,
		},
		{
			name:   "below max -- HIGH",
			recReq: 190_000_000,
			max:    200_000_000,
			want:   models.RiskHigh,
		},
		{
			name:   "zero max -- LOW",
			recReq: 100,
			max:    0,
			want:   models.RiskLow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			memUsage := models.MetricAggregate{Max: tt.max}
			got := computeMemoryRisk(tt.recReq, memUsage)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCapReduction(t *testing.T) {
	tests := []struct {
		name        string
		current     int64
		recommended float64
		confidence  float64
		wantValue   float64
		wantCapped  bool
	}{
		{
			name:        "low confidence — cap at 30%",
			current:     1000,
			recommended: 100,
			confidence:  0.24,
			wantValue:   700, // 1000 * (1-0.30) = 700
			wantCapped:  true,
		},
		{
			name:        "medium confidence — cap at 50%",
			current:     2000,
			recommended: 500,
			confidence:  0.60,
			wantValue:   1000, // 2000 * (1-0.50) = 1000
			wantCapped:  true,
		},
		{
			name:        "high confidence — cap at 75%",
			current:     2000,
			recommended: 400,
			confidence:  0.95,
			wantValue:   500, // 2000 * (1-0.75) = 500
			wantCapped:  true,
		},
		{
			name:        "high confidence — recommendation above cap floor",
			current:     2000,
			recommended: 600,
			confidence:  0.95,
			wantValue:   600, // 600 >= 500, no cap applied
			wantCapped:  false,
		},
		{
			name:        "medium confidence — no cap needed",
			current:     2000,
			recommended: 1500,
			confidence:  0.60,
			wantValue:   1500, // 1500 >= 1000, no cap applied
			wantCapped:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, capped := capReduction(tt.current, tt.recommended, tt.confidence)
			assert.InDelta(t, tt.wantValue, got, 0.01)
			assert.Equal(t, tt.wantCapped, capped)
		})
	}
}

func TestGenerateMemoryRecommendation_AnomalousPattern(t *testing.T) {
	// Anomalous pattern: recommendation preserves current resources with risk=HIGH.
	memUsage := models.MetricAggregate{
		P50: 10_485_760, P99: 209_715_200, Max: 220_200_960,
	}

	rec := GenerateMemoryRecommendation(
		testOwner, "main", memUsage,
		1_000_000_000, 2_000_000_000,
		10080, models.PatternAnomalous,
	)
	require.NotNil(t, rec)

	assert.Equal(t, models.PatternAnomalous, rec.Pattern)
	assert.Equal(t, int64(1_000_000_000), rec.RecommendedReq)
	assert.Equal(t, int64(2_000_000_000), rec.RecommendedLimit)
	assert.Equal(t, int64(0), rec.EstSavings)
	assert.Equal(t, models.RiskHigh, rec.Risk)
	assert.Equal(t, float64(0), rec.Confidence)
	assert.Contains(t, rec.Reasoning, "memory leak")
}

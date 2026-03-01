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
	// RecLimit = P99 * 1.20 = 200 * 1.20 = 240, but max(240, recReq=264) = 264
	// Savings = 2000 - 264 = 1736
	// Risk: recReq/P99 = 264/200 = 1.32 > 1.10 -> LOW
	// Confidence: steady, 10080 min (>= 7d) -> 0.95
	usage := models.MetricAggregate{P50: 200, P95: 220, P99: 200, Max: 300}

	rec := GenerateCPURecommendation(testOwner, "main", usage, 2000, 4000, 10080)
	require.NotNil(t, rec)

	assert.Equal(t, "cpu", rec.Resource)
	assert.Equal(t, models.PatternSteady, rec.Pattern)
	assert.Equal(t, int64(264), rec.RecommendedReq)
	assert.Equal(t, int64(264), rec.RecommendedLimit)
	assert.Equal(t, int64(1736), rec.EstSavings)
	assert.Equal(t, models.RiskLow, rec.Risk)
	assert.InDelta(t, 0.95, rec.Confidence, 0.01)
	assert.Contains(t, rec.Reasoning, "Steady")
	assertSafetyInvariant_CPU(t, rec, usage)
}

func TestGenerateCPURecommendation_BurstableWorkload(t *testing.T) {
	// P50/req = 200/2000 = 0.1 -> not idle. CV = (400-200)/200 = 1.0 > 0.3 -> Burstable.
	// Not batch: P99/P50 = 600/200 = 3 < 5.
	// RecReq = P50 * 1.20 = 200 * 1.20 = 240
	// RecLimit = P99 * 1.20 = 600 * 1.20 = 720
	// Savings = 2000 - 240 = 1760
	// Risk: burstable compares limit to P99: recLimit/P99 = 720/600 = 1.20 > 1.10 -> LOW
	// Confidence: burstable, 10080 min -> capped at 0.80
	usage := models.MetricAggregate{P50: 200, P95: 400, P99: 600, Max: 800}

	rec := GenerateCPURecommendation(testOwner, "main", usage, 2000, 4000, 10080)
	require.NotNil(t, rec)

	assert.Equal(t, models.PatternBurstable, rec.Pattern)
	assert.Equal(t, int64(240), rec.RecommendedReq)
	assert.Equal(t, int64(720), rec.RecommendedLimit)
	assert.Equal(t, int64(1760), rec.EstSavings)
	assert.InDelta(t, 0.80, rec.Confidence, 0.01)
	assert.Equal(t, models.RiskLow, rec.Risk)
	assert.Contains(t, rec.Reasoning, "Burstable")
	assertSafetyInvariant_CPU(t, rec, usage)
}

func TestGenerateCPURecommendation_BatchWorkload(t *testing.T) {
	// P50/request = 100/2000 = 0.05 so not idle (needs < 0.05).
	// P99/P50 = 800/100 = 8 >= 5, Max/P50 = 1500/100 = 15 >= 10 -> Batch.
	// RecReq = P50 * 1.20 = 100 * 1.20 = 120
	// RecLimit = Max * 1.20 = 1500 * 1.20 = 1800
	// Savings = 2000 - 120 = 1880
	// Risk: batch compares limit to Max: recLimit/Max = 1800/1500 = 1.20 >= 1.10 -> LOW
	// Confidence: batch, 10080 min -> capped at 0.70
	usage := models.MetricAggregate{P50: 100, P95: 400, P99: 800, Max: 1500}

	rec := GenerateCPURecommendation(testOwner, "main", usage, 2000, 4000, 10080)
	require.NotNil(t, rec)

	assert.Equal(t, models.PatternBatch, rec.Pattern)
	assert.Equal(t, int64(120), rec.RecommendedReq)
	assert.Equal(t, int64(1800), rec.RecommendedLimit)
	assert.Equal(t, int64(1880), rec.EstSavings)
	assert.InDelta(t, 0.70, rec.Confidence, 0.01)
	assert.Equal(t, models.RiskLow, rec.Risk)
	assert.Contains(t, rec.Reasoning, "Batch")
	assertSafetyInvariant_CPU(t, rec, usage)
}

func TestGenerateCPURecommendation_IdleWorkload(t *testing.T) {
	// P50=2, P95=5, P99=8, Max=10
	// P50/req = 2/1000 = 0.002 < 0.05, dataWindow=5000 > 2880 -> Idle.
	// RecReq = max(P95 * 1.20, 10) = max(6, 10) = 10
	// RecLimit = max(P99 * 1.20, recReq) = max(9.6, 10) = 10
	// Savings = 1000 - 10 = 990
	// Risk: recReq/P99 = 10/8 = 1.25 > 1.10 -> LOW
	// Confidence: idle, 5000 min (3-7d range),
	//   ratio = (5000-4320)/(10080-4320) = 680/5760 ~ 0.118,
	//   base = 0.85 + 0.118*0.10 = 0.8618, cap 0.95, round -> 0.86
	usage := models.MetricAggregate{P50: 2, P95: 5, P99: 8, Max: 10}

	rec := GenerateCPURecommendation(testOwner, "main", usage, 1000, 2000, 5000)
	require.NotNil(t, rec)

	assert.Equal(t, models.PatternIdle, rec.Pattern)
	assert.Equal(t, int64(minRecommendedCPUMillis), rec.RecommendedReq)
	assert.Equal(t, int64(minRecommendedCPUMillis), rec.RecommendedLimit)
	assert.Equal(t, int64(990), rec.EstSavings)
	assert.Equal(t, models.RiskLow, rec.Risk)
	assert.InDelta(t, 0.86, rec.Confidence, 0.01)
	assert.Contains(t, rec.Reasoning, "idle")
	assert.Contains(t, rec.Reasoning, "Consider scaling to zero")
	assertSafetyInvariant_CPU(t, rec, usage)
}

func TestGenerateCPURecommendation_ZeroRequest(t *testing.T) {
	usage := models.MetricAggregate{P50: 100, P95: 200, P99: 250, Max: 300}

	rec := GenerateCPURecommendation(testOwner, "main", usage, 0, 0, 10080)
	assert.Nil(t, rec, "should not recommend when current request is zero")
}

func TestGenerateCPURecommendation_NegativeRequest(t *testing.T) {
	usage := models.MetricAggregate{P50: 100, P95: 200, P99: 250, Max: 300}

	rec := GenerateCPURecommendation(testOwner, "main", usage, -100, 0, 10080)
	assert.Nil(t, rec, "should not recommend with negative request")
}

func TestGenerateCPURecommendation_UsageExceedsRequest(t *testing.T) {
	// P95 > request = workload is under-provisioned.
	usage := models.MetricAggregate{P50: 800, P95: 1200, P99: 1500, Max: 2000}

	rec := GenerateCPURecommendation(testOwner, "main", usage, 1000, 2000, 10080)
	assert.Nil(t, rec, "should not recommend when usage exceeds request")
}

func TestGenerateCPURecommendation_AlreadyRightSized(t *testing.T) {
	// Waste < 10% -- below wasteThresholdPercent.
	usage := models.MetricAggregate{P50: 400, P95: 450, P99: 470, Max: 490}

	// Request = 500, P95 = 450, headroom = 540. Savings = 500 - 540 < 0.
	rec := GenerateCPURecommendation(testOwner, "main", usage, 500, 1000, 10080)
	assert.Nil(t, rec, "should not recommend when already right-sized")
}

func TestGenerateCPURecommendation_MinimumFloor(t *testing.T) {
	// Very low usage -- should not recommend below 10m CPU.
	// P50=1, P95=2, P99=3, Max=5 -> Steady (CV = (2-1)/1 = 1.0 -> Burstable actually)
	// Check: CV = (P95-P50)/P50 = (2-1)/1 = 1.0 > 0.3 -> Burstable
	// P99/P50 = 3, Max/P50 = 5 -> not batch (3 < 5 threshold)
	// RecReq = P50 * 1.20 = 1.2, floor to 10
	// RecLimit = max(P99 * 1.20, 10) = max(3.6, 10) = 10
	// Savings = 1000 - 10 = 990
	usage := models.MetricAggregate{P50: 1, P95: 2, P99: 3, Max: 5}

	rec := GenerateCPURecommendation(testOwner, "main", usage, 1000, 2000, 10080)
	require.NotNil(t, rec)

	assert.Equal(t, int64(minRecommendedCPUMillis), rec.RecommendedReq)
	assert.Equal(t, int64(minRecommendedCPUMillis), rec.RecommendedLimit)
	assert.Equal(t, int64(990), rec.EstSavings)
	assertSafetyInvariant_CPU(t, rec, usage)
}

func TestGenerateCPURecommendation_ZeroUsage_WithEnoughData(t *testing.T) {
	// Zero usage with request=1000 and 83h data window (> 48h) -> Idle.
	usage := models.MetricAggregate{P50: 0, P95: 0, P99: 0, Max: 0}

	rec := GenerateCPURecommendation(testOwner, "main", usage, 1000, 2000, 5000)
	require.NotNil(t, rec)

	assert.Equal(t, int64(minRecommendedCPUMillis), rec.RecommendedReq)
	assert.Equal(t, models.PatternIdle, rec.Pattern)
	assert.Equal(t, int64(990), rec.EstSavings)
	assertSafetyInvariant_CPU(t, rec, usage)
}

func TestGenerateCPURecommendation_ZeroUsage_ShortWindow(t *testing.T) {
	// Zero usage but only 1h data -> not enough for idle, defaults to Steady.
	usage := models.MetricAggregate{P50: 0, P95: 0, P99: 0, Max: 0}

	rec := GenerateCPURecommendation(testOwner, "main", usage, 1000, 2000, 60)
	require.NotNil(t, rec)

	assert.Equal(t, int64(minRecommendedCPUMillis), rec.RecommendedReq)
	assert.Equal(t, models.PatternSteady, rec.Pattern)
	assert.Equal(t, int64(990), rec.EstSavings)
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

func TestGenerateMemoryRecommendation_ZeroRequest(t *testing.T) {
	memUsage := models.MetricAggregate{P50: 100, P95: 200, P99: 250, Max: 300}

	rec := GenerateMemoryRecommendation(
		testOwner, "main", memUsage,
		0, 0, 10080, models.PatternSteady,
	)
	assert.Nil(t, rec)
}

func TestGenerateMemoryRecommendation_MinimumFloor(t *testing.T) {
	// Steady: RecReq = P99 * 1.20 = 300 * 1.20 = 360
	// Floor = 4 * 1024 * 1024 = 4_194_304
	// max(360, 4_194_304) = 4_194_304
	memUsage := models.MetricAggregate{P50: 100, P95: 200, P99: 300, Max: 500}

	rec := GenerateMemoryRecommendation(
		testOwner, "main", memUsage,
		1_000_000_000, 2_000_000_000,
		10080, models.PatternSteady,
	)
	require.NotNil(t, rec)
	assert.Equal(t, int64(minRecommendedMemBytes), rec.RecommendedReq)
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

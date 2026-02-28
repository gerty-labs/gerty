package rules

import (
	"testing"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testOwner = models.OwnerReference{
	Kind: "Deployment", Name: "api-server", Namespace: "production",
}

func TestGenerateCPURecommendation_SteadyWorkload(t *testing.T) {
	// P50=200, P95=220 → CV = (220-200)/200 = 0.1 < 0.3 → Steady.
	// P50/req = 200/2000 = 0.1 ≥ 0.05 → not idle.
	// Risk: recReq=264, P99=200 → ratio=264/200=1.32 > 1.10 → LOW.
	usage := models.MetricAggregate{P50: 200, P95: 220, P99: 200, Max: 300}

	rec := GenerateCPURecommendation(testOwner, "main", usage, 2000, 4000, 10080)
	require.NotNil(t, rec)

	assert.Equal(t, "cpu", rec.Resource)
	assert.Equal(t, models.PatternSteady, rec.Pattern)
	// Request should be P95 * 1.20 = 264
	assert.Equal(t, int64(264), rec.RecommendedReq)
	// Savings = 2000 - 264 = 1736
	assert.Equal(t, int64(1736), rec.EstSavings)
	assert.Contains(t, rec.Reasoning, "Steady")
	assert.Equal(t, models.RiskLow, rec.Risk)
}

func TestGenerateCPURecommendation_BurstableWorkload(t *testing.T) {
	// P50/req = 200/2000 = 0.1 → not idle. CV = (400-200)/200 = 1.0 > 0.3 → Burstable.
	// Not batch: P99/P50 = 600/200 = 3 < 5.
	usage := models.MetricAggregate{P50: 200, P95: 400, P99: 600, Max: 800}

	rec := GenerateCPURecommendation(testOwner, "main", usage, 2000, 4000, 10080)
	require.NotNil(t, rec)

	assert.Equal(t, models.PatternBurstable, rec.Pattern)
	// Request = P50 * 1.30 = 260
	assert.Equal(t, int64(260), rec.RecommendedReq)
	// Limit = P99 * 1.25 = 750
	assert.Equal(t, int64(750), rec.RecommendedLimit)
	assert.Contains(t, rec.Reasoning, "Burstable")
}

func TestGenerateCPURecommendation_BatchWorkload(t *testing.T) {
	// P50/request = 100/2000 = 0.05 so not idle (needs < 0.05).
	// P99/P50 = 800/100 = 8 >= 5, Max/P50 = 1500/100 = 15 >= 10 → Batch.
	usage := models.MetricAggregate{P50: 100, P95: 400, P99: 800, Max: 1500}

	rec := GenerateCPURecommendation(testOwner, "main", usage, 2000, 4000, 10080)
	require.NotNil(t, rec)

	assert.Equal(t, models.PatternBatch, rec.Pattern)
	// Request = P50 * 1.30 = 130
	assert.Equal(t, int64(130), rec.RecommendedReq)
	// Limit = Max * 1.10 = 1650.0 (floating point: math.Ceil may round up to 1651)
	assert.Equal(t, int64(1651), rec.RecommendedLimit)
	assert.Contains(t, rec.Reasoning, "Batch")
}

func TestGenerateCPURecommendation_IdleWorkload(t *testing.T) {
	usage := models.MetricAggregate{P50: 2, P95: 5, P99: 8, Max: 10}

	rec := GenerateCPURecommendation(testOwner, "main", usage, 1000, 2000, 5000)
	require.NotNil(t, rec)

	assert.Equal(t, models.PatternIdle, rec.Pattern)
	assert.Contains(t, rec.Reasoning, "idle")
	assert.Contains(t, rec.Reasoning, "Consider scaling to zero")
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
	// Waste < 10% — below wasteThresholdPercent.
	usage := models.MetricAggregate{P50: 400, P95: 450, P99: 470, Max: 490}

	// Request = 500, P95 = 450, headroom = 540. Savings = 500 - 540 < 0.
	rec := GenerateCPURecommendation(testOwner, "main", usage, 500, 1000, 10080)
	assert.Nil(t, rec, "should not recommend when already right-sized")
}

func TestGenerateCPURecommendation_MinimumFloor(t *testing.T) {
	// Very low usage — should not recommend below 10m CPU.
	usage := models.MetricAggregate{P50: 1, P95: 2, P99: 3, Max: 5}

	rec := GenerateCPURecommendation(testOwner, "main", usage, 1000, 2000, 10080)
	require.NotNil(t, rec)

	assert.GreaterOrEqual(t, rec.RecommendedReq, int64(minRecommendedCPUMillis))
}

func TestGenerateCPURecommendation_ZeroUsage_WithEnoughData(t *testing.T) {
	// Zero usage with request=1000 and 83h data window (> 48h) → Idle.
	usage := models.MetricAggregate{P50: 0, P95: 0, P99: 0, Max: 0}

	rec := GenerateCPURecommendation(testOwner, "main", usage, 1000, 2000, 5000)
	require.NotNil(t, rec)

	assert.Equal(t, int64(minRecommendedCPUMillis), rec.RecommendedReq)
	assert.Equal(t, models.PatternIdle, rec.Pattern)
}

func TestGenerateCPURecommendation_ZeroUsage_ShortWindow(t *testing.T) {
	// Zero usage but only 1h data → not enough for idle, defaults to Steady.
	usage := models.MetricAggregate{P50: 0, P95: 0, P99: 0, Max: 0}

	rec := GenerateCPURecommendation(testOwner, "main", usage, 1000, 2000, 60)
	require.NotNil(t, rec)

	assert.Equal(t, int64(minRecommendedCPUMillis), rec.RecommendedReq)
	assert.Equal(t, models.PatternSteady, rec.Pattern)
}

func TestGenerateMemoryRecommendation_SteadyWorkload(t *testing.T) {
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
	// Request = P99 * 1.20 = 300_000_000
	assert.Equal(t, int64(300_000_000), rec.RecommendedReq)
	assert.Contains(t, rec.Reasoning, "Steady")
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
	memUsage := models.MetricAggregate{P50: 100, P95: 200, P99: 300, Max: 500}

	rec := GenerateMemoryRecommendation(
		testOwner, "main", memUsage,
		1_000_000_000, 2_000_000_000,
		10080, models.PatternSteady,
	)
	require.NotNil(t, rec)
	assert.GreaterOrEqual(t, rec.RecommendedReq, int64(minRecommendedMemBytes))
}

func TestComputeConfidence(t *testing.T) {
	tests := []struct {
		name    string
		stats   WorkloadStats
		wantMin float64
		wantMax float64
	}{
		{
			name:    "7+ days steady — highest confidence",
			stats:   WorkloadStats{Pattern: models.PatternSteady, DataWindowMinutes: 10080},
			wantMin: 0.90,
			wantMax: 0.95,
		},
		{
			name:    "3–7 days steady — medium confidence",
			stats:   WorkloadStats{Pattern: models.PatternSteady, DataWindowMinutes: 5000},
			wantMin: 0.70,
			wantMax: 0.95,
		},
		{
			name:    "less than 3 days — low confidence",
			stats:   WorkloadStats{Pattern: models.PatternSteady, DataWindowMinutes: 1440},
			wantMin: 0.20,
			wantMax: 0.50,
		},
		{
			name:    "zero data window",
			stats:   WorkloadStats{Pattern: models.PatternSteady, DataWindowMinutes: 0},
			wantMin: 0.19,
			wantMax: 0.21,
		},
		{
			name:    "burstable capped at 0.80",
			stats:   WorkloadStats{Pattern: models.PatternBurstable, DataWindowMinutes: 10080},
			wantMin: 0.79,
			wantMax: 0.81,
		},
		{
			name:    "batch capped at 0.70",
			stats:   WorkloadStats{Pattern: models.PatternBatch, DataWindowMinutes: 10080},
			wantMin: 0.69,
			wantMax: 0.71,
		},
		{
			name:    "idle with good data — high confidence",
			stats:   WorkloadStats{Pattern: models.PatternIdle, DataWindowMinutes: 10080},
			wantMin: 0.90,
			wantMax: 0.96,
		},
		{
			name:    "exactly 3 days boundary",
			stats:   WorkloadStats{Pattern: models.PatternSteady, DataWindowMinutes: 4320},
			wantMin: 0.49,
			wantMax: 0.86,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeConfidence(tt.stats)
			assert.GreaterOrEqual(t, got, tt.wantMin, "confidence too low")
			assert.LessOrEqual(t, got, tt.wantMax, "confidence too high")
		})
	}
}

func TestComputeRisk(t *testing.T) {
	tests := []struct {
		name  string
		recReq float64
		p99   float64
		want  models.RiskLevel
	}{
		{
			name:   "well above P99 — LOW",
			recReq: 500,
			p99:    200,
			want:   models.RiskLow,
		},
		{
			name:   "close to P99 — MEDIUM",
			recReq: 210,
			p99:    200,
			want:   models.RiskMedium, // 210/200 = 1.05 < 1.10
		},
		{
			name:   "at or below P99 — HIGH",
			recReq: 190,
			p99:    200,
			want:   models.RiskHigh, // 190/200 = 0.95 < 1.0
		},
		{
			name:   "exactly at P99 — MEDIUM",
			recReq: 200,
			p99:    200,
			want:   models.RiskMedium, // 200/200 = 1.0, not < 1.0 so not HIGH, but < 1.10 so MEDIUM
		},
		{
			name:   "zero P99 — LOW",
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
			name:   "well above max — LOW",
			recReq: 500_000_000,
			max:    200_000_000,
			want:   models.RiskLow,
		},
		{
			name:   "close to max — MEDIUM",
			recReq: 210_000_000,
			max:    200_000_000,
			want:   models.RiskMedium,
		},
		{
			name:   "below max — HIGH",
			recReq: 190_000_000,
			max:    200_000_000,
			want:   models.RiskHigh,
		},
		{
			name:   "zero max — LOW",
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

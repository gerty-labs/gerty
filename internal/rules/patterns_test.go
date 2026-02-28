package rules

import (
	"testing"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestClassifyWorkload(t *testing.T) {
	tests := []struct {
		name              string
		cpuUsage          models.MetricAggregate
		cpuRequestMillis  int64
		dataWindowMinutes float64
		want              models.WorkloadPattern
	}{
		{
			name:              "steady — low CV",
			cpuUsage:          models.MetricAggregate{P50: 100, P95: 120, P99: 130, Max: 140},
			cpuRequestMillis:  500,
			dataWindowMinutes: 10080, // 7 days
			want:              models.PatternSteady, // CV = (120-100)/100 = 0.2 < 0.3
		},
		{
			name:              "steady — CV just below threshold (0.29)",
			cpuUsage:          models.MetricAggregate{P50: 100, P95: 129, P99: 150, Max: 160},
			cpuRequestMillis:  500,
			dataWindowMinutes: 10080,
			want:              models.PatternSteady, // CV = (129-100)/100 = 0.29 < 0.3
		},
		{
			name:              "burstable — CV exactly at threshold (0.30)",
			cpuUsage:          models.MetricAggregate{P50: 100, P95: 130, P99: 150, Max: 160},
			cpuRequestMillis:  500,
			dataWindowMinutes: 10080,
			want:              models.PatternBurstable, // CV = (130-100)/100 = 0.30, NOT < 0.3 -> burstable
		},
		{
			name:              "burstable — moderate CV",
			cpuUsage:          models.MetricAggregate{P50: 200, P95: 400, P99: 500, Max: 600},
			cpuRequestMillis:  1000,
			dataWindowMinutes: 10080,
			want:              models.PatternBurstable, // CV = (400-200)/200 = 1.0 > 0.3; P50/req = 0.2 >= 0.05
		},
		{
			name:              "burstable — just above threshold",
			cpuUsage:          models.MetricAggregate{P50: 100, P95: 131, P99: 150, Max: 160},
			cpuRequestMillis:  500,
			dataWindowMinutes: 10080,
			want:              models.PatternBurstable, // CV = (131-100)/100 = 0.31 > 0.3
		},
		{
			name:              "idle — very low utilisation with enough data",
			cpuUsage:          models.MetricAggregate{P50: 2, P95: 5, P99: 8, Max: 10},
			cpuRequestMillis:  1000,
			dataWindowMinutes: 3000, // 50 hours > 48h threshold
			want:              models.PatternIdle, // P50/req = 0.002 < 0.05
		},
		{
			name:              "idle — P50/req just below 5% (0.049)",
			cpuUsage:          models.MetricAggregate{P50: 49, P95: 50, P99: 55, Max: 60},
			cpuRequestMillis:  1000,
			dataWindowMinutes: 3000,
			want:              models.PatternIdle, // P50/req = 0.049 < 0.05
		},
		{
			name:              "not idle — P50/req exactly at 5% (0.050)",
			cpuUsage:          models.MetricAggregate{P50: 50, P95: 60, P99: 70, Max: 80},
			cpuRequestMillis:  1000,
			dataWindowMinutes: 3000,
			want:              models.PatternSteady, // P50/req = 0.050, NOT < 0.05 -> not idle; CV = (60-50)/50 = 0.2 < 0.3 -> steady
		},
		{
			name:              "not idle — just above 5% utilisation",
			cpuUsage:          models.MetricAggregate{P50: 51, P95: 60, P99: 70, Max: 80},
			cpuRequestMillis:  1000,
			dataWindowMinutes: 3000,
			want:              models.PatternSteady, // P50/req = 0.051 >= 0.05; CV = (60-51)/51 = 0.18 < 0.3
		},
		{
			name:              "not idle — insufficient data window",
			cpuUsage:          models.MetricAggregate{P50: 2, P95: 5, P99: 8, Max: 10},
			cpuRequestMillis:  1000,
			dataWindowMinutes: 2000, // ~33 hours < 48h
			want:              models.PatternBurstable, // Can't check idle; CV = (5-2)/2 = 1.5 > 0.3
		},
		{
			name:              "batch — extreme spikes, non-idle baseline",
			cpuUsage:          models.MetricAggregate{P50: 100, P95: 400, P99: 800, Max: 1500},
			cpuRequestMillis:  2000,
			dataWindowMinutes: 10080,
			want:              models.PatternBatch, // P50/req = 0.05 so not idle; P99/P50=8 >= 5, Max/P50=15 >= 10
		},
		{
			name:              "batch — exactly at spike ratio thresholds",
			cpuUsage:          models.MetricAggregate{P50: 100, P95: 300, P99: 500, Max: 1000},
			cpuRequestMillis:  2000,
			dataWindowMinutes: 10080,
			want:              models.PatternBatch, // P99/P50 = 5, Max/P50 = 10 (exactly at thresholds)
		},
		{
			name:              "not batch — spike ratios just below thresholds",
			cpuUsage:          models.MetricAggregate{P50: 100, P95: 300, P99: 490, Max: 990},
			cpuRequestMillis:  2000,
			dataWindowMinutes: 10080,
			want:              models.PatternBurstable, // P99/P50 = 4.9 < 5
		},
		{
			name:              "zero requests — classify by shape only, not idle",
			cpuUsage:          models.MetricAggregate{P50: 100, P95: 120, P99: 130, Max: 140},
			cpuRequestMillis:  0,
			dataWindowMinutes: 5000,
			want:              models.PatternSteady, // CV = 0.2 < 0.3
		},
		{
			name:              "zero usage and zero request — default steady",
			cpuUsage:          models.MetricAggregate{P50: 0, P95: 0, P99: 0, Max: 0},
			cpuRequestMillis:  0,
			dataWindowMinutes: 10080,
			want:              models.PatternSteady,
		},
		{
			name:              "zero usage with request and enough data — idle",
			cpuUsage:          models.MetricAggregate{P50: 0, P95: 0, P99: 0, Max: 0},
			cpuRequestMillis:  1000,
			dataWindowMinutes: 5000,
			want:              models.PatternIdle,
		},
		{
			name:              "zero usage with request but short window — steady default",
			cpuUsage:          models.MetricAggregate{P50: 0, P95: 0, P99: 0, Max: 0},
			cpuRequestMillis:  1000,
			dataWindowMinutes: 1000, // < 48h
			want:              models.PatternSteady,
		},
		{
			name:              "single data point — short window, all same value",
			cpuUsage:          models.MetricAggregate{P50: 100, P95: 100, P99: 100, Max: 100},
			cpuRequestMillis:  500,
			dataWindowMinutes: 1,
			want:              models.PatternSteady, // CV = 0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyWorkload(tt.cpuUsage, tt.cpuRequestMillis, tt.dataWindowMinutes)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEstimateCV(t *testing.T) {
	tests := []struct {
		name string
		agg  models.MetricAggregate
		want float64
	}{
		{
			name: "zero P50 with P95 — high CV",
			agg:  models.MetricAggregate{P50: 0, P95: 100},
			want: 1.0,
		},
		{
			name: "both zero — no CV",
			agg:  models.MetricAggregate{P50: 0, P95: 0},
			want: 0.0,
		},
		{
			name: "equal P50 and P95 — zero CV",
			agg:  models.MetricAggregate{P50: 100, P95: 100},
			want: 0.0,
		},
		{
			name: "P95 double P50 — CV=1.0",
			agg:  models.MetricAggregate{P50: 100, P95: 200},
			want: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateCV(tt.agg)
			assert.InDelta(t, tt.want, got, 0.01)
		})
	}
}

func TestIsBatchPattern(t *testing.T) {
	tests := []struct {
		name string
		agg  models.MetricAggregate
		want bool
	}{
		{
			name: "typical batch",
			agg:  models.MetricAggregate{P50: 10, P95: 40, P99: 80, Max: 200},
			want: true,
		},
		{
			name: "not batch — ratios too low",
			agg:  models.MetricAggregate{P50: 100, P95: 150, P99: 200, Max: 250},
			want: false,
		},
		{
			name: "zero P50",
			agg:  models.MetricAggregate{P50: 0, P95: 40, P99: 80, Max: 200},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBatchPattern(tt.agg)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestComputeWorkloadStats(t *testing.T) {
	usage := models.MetricAggregate{P50: 100, P95: 120, P99: 130, Max: 140}
	stats := ComputeWorkloadStats(usage, 500, 10080)

	assert.Equal(t, models.PatternSteady, stats.Pattern)
	assert.InDelta(t, 0.2, stats.CV, 0.01) // (120-100)/100 = 0.2
	assert.InDelta(t, 0.2, stats.UtilisationP50, 0.01)
	assert.InDelta(t, 0.24, stats.UtilisationP95, 0.01)
}

func TestComputeWorkloadStats_CapsUtilisation(t *testing.T) {
	// Usage exceeds request — utilisation capped at 1.0 for P50/P95.
	usage := models.MetricAggregate{P50: 600, P95: 700, P99: 800, Max: 900}
	stats := ComputeWorkloadStats(usage, 500, 10080)

	assert.Equal(t, 1.0, stats.UtilisationP50)
	assert.Equal(t, 1.0, stats.UtilisationP95)
	assert.InDelta(t, 1.6, stats.UtilisationP99, 0.01) // P99 not capped
}

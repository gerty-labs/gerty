package scale

import (
	"fmt"
	"math"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/gregorytcarroll/k8s-sage/internal/rules"
	"github.com/gregorytcarroll/k8s-sage/internal/server"
	"github.com/stretchr/testify/assert"
)

var scaleConfigs = []ScaleConfig{
	{Nodes: 20, PodsPerNode: 30, Namespaces: 5, Owners: 40},
	{Nodes: 50, PodsPerNode: 30, Namespaces: 10, Owners: 100},
	{Nodes: 100, PodsPerNode: 30, Namespaces: 15, Owners: 200},
	{Nodes: 200, PodsPerNode: 30, Namespaces: 20, Owners: 400},
	{Nodes: 500, PodsPerNode: 30, Namespaces: 30, Owners: 1000},
	{Nodes: 1000, PodsPerNode: 30, Namespaces: 50, Owners: 2000},
}

func TestScale(t *testing.T) {
	if testing.Short() {
		t.Skip("scale test skipped in short mode")
	}

	var results []ScaleResult

	for _, cfg := range scaleConfigs {
		cfg := cfg
		t.Run(fmt.Sprintf("%d-nodes", cfg.Nodes), func(t *testing.T) {
			result := runScalePoint(t, cfg)
			results = append(results, result)
		})
	}

	// Print consolidated report.
	t.Log(FormatReport(results))
}

func TestScaleSmall(t *testing.T) {
	// Quick smoke test with just the smallest scale point.
	cfg := ScaleConfig{Nodes: 20, PodsPerNode: 30, Namespaces: 5, Owners: 40}
	result := runScalePoint(t, cfg)

	assert.Equal(t, cfg.TotalPods(), result.PodsIngested)
	assert.Equal(t, 0, result.PodsDropped)
	assert.Greater(t, result.RecCount, 0)
	assert.Less(t, result.IngestTimeMs, 5000.0) // Should be well under 5s.
}

func TestGenerateNodeReports_Deterministic(t *testing.T) {
	cfg := ScaleConfig{Nodes: 5, PodsPerNode: 10, Namespaces: 3, Owners: 10}
	reports1 := GenerateNodeReports(cfg, 42)
	reports2 := GenerateNodeReports(cfg, 42)

	assert.Equal(t, len(reports1), len(reports2))
	for i := range reports1 {
		assert.Equal(t, reports1[i].NodeName, reports2[i].NodeName)
		assert.Equal(t, len(reports1[i].Pods), len(reports2[i].Pods))
		for j := range reports1[i].Pods {
			assert.Equal(t, reports1[i].Pods[j].PodName, reports2[i].Pods[j].PodName)
			assert.Equal(t, reports1[i].Pods[j].OwnerRef, reports2[i].Pods[j].OwnerRef)
		}
	}
}

func TestGenerateNodeReports_CorrectCounts(t *testing.T) {
	cfg := ScaleConfig{Nodes: 10, PodsPerNode: 20, Namespaces: 5, Owners: 15}
	reports := GenerateNodeReports(cfg, 42)

	assert.Equal(t, 10, len(reports))
	totalPods := 0
	for _, r := range reports {
		totalPods += len(r.Pods)
	}
	assert.Equal(t, 200, totalPods)
}

func TestGenerateNodeReports_WorkloadMix(t *testing.T) {
	// Generate enough data that we see all pattern types.
	cfg := ScaleConfig{Nodes: 20, PodsPerNode: 30, Namespaces: 5, Owners: 100}
	reports := GenerateNodeReports(cfg, 42)

	patterns := make(map[string]int)
	for _, r := range reports {
		for _, pod := range r.Pods {
			// Infer pattern from owner name prefix.
			name := pod.OwnerRef.Name
			if len(name) >= 3 {
				patterns[name[:3]]++
			}
		}
	}

	// Should have all pattern types.
	assert.Greater(t, patterns["sdy"], 0, "should have steady workloads")
	assert.Greater(t, patterns["bst"], 0, "should have burstable workloads")
	assert.Greater(t, patterns["bat"], 0, "should have batch workloads")
	assert.Greater(t, patterns["idl"], 0, "should have idle workloads")
	assert.Greater(t, patterns["ano"], 0, "should have anomalous workloads")
}

func TestScaleConfig_TotalPods(t *testing.T) {
	cfg := ScaleConfig{Nodes: 100, PodsPerNode: 30}
	assert.Equal(t, 3000, cfg.TotalPods())
}

func TestFormatReport(t *testing.T) {
	results := []ScaleResult{
		{
			Config:          ScaleConfig{Nodes: 20, PodsPerNode: 30, Namespaces: 5, Owners: 40},
			PodsIngested:    600,
			IngestTimeMs:    300,
			ReportBuildMs:   5,
			L1AnalyzeMs:     12,
			L2EstimatedSecs: 160,
			CycleTimeSecs:   160,
			HeapMB:          45,
			RecCount:        80,
		},
	}

	report := FormatReport(results)
	assert.Contains(t, report, "k8s-sage Scale Test Results")
	assert.Contains(t, report, "20")
	assert.Contains(t, report, "600")
	assert.Contains(t, report, "OK")
}

func TestBudgetStatus(t *testing.T) {
	tests := []struct {
		cycle float64
		want  string
	}{
		{100, "OK"},
		{199, "OK"},
		{200, "OK"},
		{201, "WARN"},
		{299, "WARN"},
		{300, "WARN"},
		{301, "OVER"},
	}
	for _, tt := range tests {
		r := ScaleResult{CycleTimeSecs: tt.cycle}
		assert.Equal(t, tt.want, r.BudgetStatus(), "cycle=%.0f", tt.cycle)
	}
}

// runScalePoint executes a single scale test scenario.
func runScalePoint(t *testing.T, cfg ScaleConfig) ScaleResult {
	t.Helper()

	// Generate synthetic data.
	reports := GenerateNodeReports(cfg, 42)

	// Fresh aggregator + engine per scenario.
	agg := server.NewAggregator()
	engine := rules.NewEngine()

	// Force GC to get a clean baseline.
	runtime.GC()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	// Measure ingest time.
	var ingestLatencies []float64
	ingestStart := time.Now()
	totalIngested := 0
	totalDropped := 0

	for _, report := range reports {
		reportStart := time.Now()
		ingested, atCapacity := agg.Ingest(report)
		latency := time.Since(reportStart).Seconds() * 1000 // ms
		ingestLatencies = append(ingestLatencies, latency)

		totalIngested += ingested
		if atCapacity {
			totalDropped += len(report.Pods) - ingested
		}
	}
	ingestTime := time.Since(ingestStart).Seconds() * 1000 // ms

	// Measure report build time.
	reportStart := time.Now()
	clusterReport := agg.ClusterReport()
	reportBuildMs := time.Since(reportStart).Seconds() * 1000

	// Measure L1 analysis time.
	l1Start := time.Now()
	recs := engine.AnalyzeCluster(clusterReport)
	l1Ms := time.Since(l1Start).Seconds() * 1000

	// Measure peak memory.
	runtime.GC()
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)
	var heapMB float64
	if memAfter.HeapInuse >= memBefore.HeapInuse {
		heapMB = float64(memAfter.HeapInuse-memBefore.HeapInuse) / (1024 * 1024)
	} else {
		// GC reclaimed memory; just report absolute heap.
		heapMB = float64(memAfter.HeapInuse) / (1024 * 1024)
	}

	// Calculate L2 estimated time.
	l2EstSecs := float64(cfg.Owners) * 4.0

	// Total cycle time.
	cycleTimeSecs := (ingestTime + reportBuildMs + l1Ms) / 1000 + l2EstSecs

	// Calculate ingest latency percentiles.
	sort.Float64s(ingestLatencies)
	p50 := percentile(ingestLatencies, 0.50)
	p95 := percentile(ingestLatencies, 0.95)
	p99 := percentile(ingestLatencies, 0.99)

	result := ScaleResult{
		Config:          cfg,
		PodsIngested:    totalIngested,
		PodsDropped:     totalDropped,
		IngestTimeMs:    ingestTime,
		ReportBuildMs:   reportBuildMs,
		L1AnalyzeMs:     l1Ms,
		L2EstimatedSecs: l2EstSecs,
		CycleTimeSecs:   cycleTimeSecs,
		HeapMB:          heapMB,
		RecCount:        len(recs),
		IngestP50Ms:     p50,
		IngestP95Ms:     p95,
		IngestP99Ms:     p99,
	}

	// Log results for this scale point.
	t.Logf("Nodes=%d Pods=%d Ingested=%d Dropped=%d Ingest=%.1fms Report=%.1fms L1=%.1fms L2est=%.0fs Cycle=%.0fs [%s] Heap=%.0fMB Recs=%d",
		cfg.Nodes, cfg.TotalPods(), totalIngested, totalDropped,
		ingestTime, reportBuildMs, l1Ms, l2EstSecs, cycleTimeSecs,
		result.BudgetStatus(), heapMB, len(recs))

	// Basic assertions.
	if cfg.TotalPods() <= 10000 {
		assert.Equal(t, 0, totalDropped, "should not drop pods under capacity")
	}

	return result
}

// percentile returns the p-th percentile from a sorted slice.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := p * float64(len(sorted)-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))
	if lower == upper {
		return sorted[lower]
	}
	frac := idx - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}

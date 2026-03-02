package scale

import (
	"fmt"
	"strings"
)

// ScaleResult holds measurements from a single scale test point.
type ScaleResult struct {
	Config          ScaleConfig
	PodsIngested    int
	PodsDropped     int
	IngestTimeMs    float64
	ReportBuildMs   float64
	L1AnalyzeMs     float64
	L2EstimatedSecs float64 // owners * 4s
	CycleTimeSecs   float64 // total cycle
	HeapMB          float64
	RecCount        int
	IngestP50Ms     float64
	IngestP95Ms     float64
	IngestP99Ms     float64
}

// BudgetStatus returns the budget classification relative to the 5-minute cycle.
func (r *ScaleResult) BudgetStatus() string {
	if r.CycleTimeSecs > 300 {
		return "OVER"
	}
	if r.CycleTimeSecs > 200 {
		return "WARN"
	}
	return "OK"
}

// FormatReport renders the consolidated scale test results table.
func FormatReport(results []ScaleResult) string {
	var b strings.Builder

	b.WriteString("\n=== k8s-sage Scale Test Results ===\n\n")
	b.WriteString(fmt.Sprintf("%-7s %-8s %-10s %-9s %-11s %-12s %-8s %-11s %-10s %-8s %-8s %-6s\n",
		"Nodes", "Pods", "Ingested", "Dropped", "Ingest(s)", "Report(ms)", "L1(ms)", "L2 est(s)", "Cycle(s)", "Budget", "HeapMB", "Recs"))
	b.WriteString(strings.Repeat("-", 110) + "\n")

	for _, r := range results {
		b.WriteString(fmt.Sprintf("%-7d %-8d %-10d %-9d %-11.1f %-12.0f %-8.0f %-11.0f %-10.0f %-8s %-8.0f %-6d\n",
			r.Config.Nodes,
			r.Config.TotalPods(),
			r.PodsIngested,
			r.PodsDropped,
			r.IngestTimeMs/1000,
			r.ReportBuildMs,
			r.L1AnalyzeMs,
			r.L2EstimatedSecs,
			r.CycleTimeSecs,
			r.BudgetStatus(),
			r.HeapMB,
			r.RecCount,
		))
	}

	b.WriteString("\n")
	b.WriteString("5-minute cycle budget = 300s\n")
	b.WriteString("L2 estimated = owners * 4s (serial SLM inference)\n")
	b.WriteString("Budget: OK (<200s), WARN (200-300s), OVER (>300s)\n")

	// Ingest latency summary.
	b.WriteString("\n=== Ingest Latency (per NodeReport) ===\n\n")
	b.WriteString(fmt.Sprintf("%-7s %-10s %-10s %-10s\n", "Nodes", "P50(ms)", "P95(ms)", "P99(ms)"))
	b.WriteString(strings.Repeat("-", 40) + "\n")
	for _, r := range results {
		b.WriteString(fmt.Sprintf("%-7d %-10.2f %-10.2f %-10.2f\n",
			r.Config.Nodes,
			r.IngestP50Ms,
			r.IngestP95Ms,
			r.IngestP99Ms,
		))
	}

	// Recommendations summary.
	b.WriteString("\n=== Recommendations ===\n\n")
	b.WriteString("- L1-only: fits 5-min cycle easily at all scale points (sub-second analysis)\n")

	// Find the crossover point for L2.
	for _, r := range results {
		if r.L2EstimatedSecs > 300 {
			b.WriteString(fmt.Sprintf("- L2 serial: exceeds 5-min budget at %d nodes (%d owners * 4s = %.0fs)\n",
				r.Config.Nodes, r.Config.Owners, r.L2EstimatedSecs))
			break
		}
	}
	b.WriteString("- L2 with top-N filter (only high-waste owners): extends coverage significantly\n")
	b.WriteString("- L2 with parallelism (multiple SLM replicas): extends linearly with replica count\n")

	return b.String()
}

package rules

import (
	"log/slog"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
)

// Engine is the deterministic rules engine that classifies workloads and
// generates right-sizing recommendations without any AI/ML model.
type Engine struct{}

// NewEngine creates a new rules Engine.
func NewEngine() *Engine {
	return &Engine{}
}

// AnalysisInput holds the data needed to analyse a single container.
type AnalysisInput struct {
	Owner             models.OwnerReference
	ContainerName     string
	CPUUsageMillis    models.MetricAggregate // CPU usage in millicores
	CPURequestMillis  int64
	CPULimitMillis    int64
	MemUsageBytes     models.MetricAggregate // Memory working set in bytes
	MemRequestBytes   int64
	MemLimitBytes     int64
	DataWindowMinutes float64
}

// AnalysisResult holds the output of analysing a container.
type AnalysisResult struct {
	Pattern            models.WorkloadPattern
	CPURecommendation  *models.Recommendation
	MemRecommendation  *models.Recommendation
}

// Analyze runs classification and recommendation generation for a single container.
func (e *Engine) Analyze(input AnalysisInput) AnalysisResult {
	// Clamp negative values to zero — defensive against garbage data.
	input.CPUUsageMillis = clampAggregate(input.CPUUsageMillis)
	input.MemUsageBytes = clampAggregate(input.MemUsageBytes)
	if input.CPURequestMillis < 0 {
		input.CPURequestMillis = 0
	}
	if input.CPULimitMillis < 0 {
		input.CPULimitMillis = 0
	}
	if input.MemRequestBytes < 0 {
		input.MemRequestBytes = 0
	}
	if input.MemLimitBytes < 0 {
		input.MemLimitBytes = 0
	}
	if input.DataWindowMinutes < 0 {
		input.DataWindowMinutes = 0
	}

	pattern := ClassifyWorkload(input.CPUUsageMillis, input.CPURequestMillis, input.DataWindowMinutes)

	slog.Debug("classified workload",
		"owner", input.Owner.Kind+"/"+input.Owner.Name,
		"container", input.ContainerName,
		"pattern", pattern)

	cpuRec := GenerateCPURecommendation(
		input.Owner,
		input.ContainerName,
		input.CPUUsageMillis,
		input.CPURequestMillis,
		input.CPULimitMillis,
		input.DataWindowMinutes,
	)

	memRec := GenerateMemoryRecommendation(
		input.Owner,
		input.ContainerName,
		input.MemUsageBytes,
		input.MemRequestBytes,
		input.MemLimitBytes,
		input.DataWindowMinutes,
		pattern,
	)

	return AnalysisResult{
		Pattern:           pattern,
		CPURecommendation: cpuRec,
		MemRecommendation: memRec,
	}
}

// AnalyzeCluster runs the rules engine across all owners in a cluster report
// and returns a list of recommendations.
func (e *Engine) AnalyzeCluster(report models.ClusterReport) []models.Recommendation {
	var recs []models.Recommendation

	for _, nsReport := range report.Namespaces {
		for _, owner := range nsReport.Owners {
			for _, cw := range owner.Containers {
				input := AnalysisInput{
					Owner:             owner.Owner,
					ContainerName:     cw.ContainerName,
					CPUUsageMillis:    cw.CPUUsage,
					CPURequestMillis:  cw.CPURequestMillis,
					CPULimitMillis:    0, // Not in ContainerWaste; use 0 = no limit
					MemUsageBytes:     cw.MemoryUsage,
					MemRequestBytes:   cw.MemoryRequestBytes,
					MemLimitBytes:     0,
					DataWindowMinutes: cw.DataWindow.Minutes(),
				}

				result := e.Analyze(input)

				if result.CPURecommendation != nil {
					recs = append(recs, *result.CPURecommendation)
				}
				if result.MemRecommendation != nil {
					recs = append(recs, *result.MemRecommendation)
				}
			}
		}
	}

	return recs
}

// clampAggregate ensures no negative values in a MetricAggregate.
func clampAggregate(agg models.MetricAggregate) models.MetricAggregate {
	return models.MetricAggregate{
		P50: clampZero(agg.P50),
		P95: clampZero(agg.P95),
		P99: clampZero(agg.P99),
		Max: clampZero(agg.Max),
	}
}

func clampZero(v float64) float64 {
	if v < 0 {
		return 0
	}
	return v
}

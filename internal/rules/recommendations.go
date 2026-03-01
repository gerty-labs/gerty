package rules

import (
	"fmt"
	"math"
	"time"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
)

const (
	// headroomSteady is the headroom multiplier above P95 for steady workloads.
	// Request = P95 * 1.20 (20% headroom).
	headroomSteady = 1.20

	// headroomBurstableReq is the headroom for burstable workload requests.
	// Request = P50 * 1.20 (20% headroom above baseline, consistent with steady).
	headroomBurstableReq = 1.20

	// headroomBurstableLimit is the headroom for burstable workload limits.
	// Limit = P99 * 1.20 (20% headroom above peak).
	headroomBurstableLimit = 1.20

	// headroomBatchLimit is the headroom for batch workload limits.
	// Limit = Max * 1.20 (20% headroom — consistent across all patterns).
	headroomBatchLimit = 1.20

	// confidenceMaxSteady7d is the maximum confidence for steady patterns with 7+ days data.
	confidenceMaxSteady7d = 0.95

	// confidenceMaxSteady3to7d is the max confidence for steady with 3–7 days data.
	confidenceMaxSteady3to7d = 0.85

	// confidenceMaxShortWindow is the max confidence for <3 days of data.
	confidenceMaxShortWindow = 0.50

	// confidenceCapBurstable caps confidence for burstable workloads.
	confidenceCapBurstable = 0.80

	// confidenceCapBatch caps confidence for batch workloads.
	confidenceCapBatch = 0.70

	// riskMediumP99Proximity is how close the recommendation can be to P99
	// before risk moves from LOW to MEDIUM. 1.10 = within 10% of P99.
	riskMediumP99Proximity = 1.10

	// riskHighP99Proximity is the threshold for HIGH risk.
	// Recommendation is at or below P99.
	riskHighP99Proximity = 1.0

	// minRecommendedCPUMillis is the floor for CPU recommendations.
	// Never recommend less than 50m CPU.
	minRecommendedCPUMillis = 50

	// minRecommendedMemBytes is the floor for memory recommendations.
	// Never recommend less than 64 MiB.
	minRecommendedMemBytes = 64 * 1024 * 1024

	// dataWindow3Days in minutes.
	dataWindow3Days = 3 * 24 * 60

	// dataWindow7Days in minutes.
	dataWindow7Days = 7 * 24 * 60

	// wasteThresholdPercent is the minimum waste percentage before generating
	// a recommendation. Below this, the workload is considered well-sized.
	wasteThresholdPercent = 10.0
)

// GenerateCPURecommendation produces a right-sizing recommendation for CPU.
// Returns nil if the workload is already well-sized.
func GenerateCPURecommendation(
	target models.OwnerReference,
	containerName string,
	cpuUsage models.MetricAggregate,
	currentReqMillis int64,
	currentLimitMillis int64,
	dataWindowMinutes float64,
) *models.Recommendation {
	if currentReqMillis <= 0 {
		return nil // Can't recommend without a current request
	}

	stats := ComputeWorkloadStats(cpuUsage, currentReqMillis, dataWindowMinutes)

	// Calculate recommended request based on pattern.
	var recReq, recLimit float64
	var reasoning string

	switch stats.Pattern {
	case models.PatternIdle:
		reasoning = fmt.Sprintf(
			"Workload is idle: P50 CPU usage is %.0fm (%.1f%% of %dm request) sustained over %.0f hours. "+
				"Consider scaling to zero or removing this workload.",
			cpuUsage.P50, stats.UtilisationP50*100, currentReqMillis, dataWindowMinutes/60)
		recReq = math.Max(cpuUsage.P95*headroomSteady, float64(minRecommendedCPUMillis))
		recLimit = math.Max(cpuUsage.P99*headroomBurstableLimit, recReq)

	case models.PatternSteady:
		recReq = cpuUsage.P95 * headroomSteady
		recLimit = cpuUsage.P99 * headroomBurstableLimit
		reasoning = fmt.Sprintf(
			"Steady workload (CV=%.2f): P95 CPU is %.0fm against %dm request. "+
				"Recommend %.0fm (P95 + %.0f%% headroom).",
			stats.CV, cpuUsage.P95, currentReqMillis, recReq, (headroomSteady-1)*100)

	case models.PatternBurstable:
		recReq = cpuUsage.P50 * headroomBurstableReq
		recLimit = cpuUsage.P99 * headroomBurstableLimit
		reasoning = fmt.Sprintf(
			"Burstable workload (CV=%.2f): baseline P50 is %.0fm, spikes to P99 %.0fm. "+
				"Request set to %.0fm (P50 + %.0f%% headroom), limit to %.0fm (P99 + %.0f%% headroom).",
			stats.CV, cpuUsage.P50, cpuUsage.P99,
			recReq, (headroomBurstableReq-1)*100,
			recLimit, (headroomBurstableLimit-1)*100)

	case models.PatternBatch:
		recReq = cpuUsage.P50 * headroomBurstableReq
		recLimit = cpuUsage.Max * headroomBatchLimit
		reasoning = fmt.Sprintf(
			"Batch workload: mostly idle at %.0fm (P50), peaks to %.0fm (max). "+
				"Request set to %.0fm for scheduling, limit to %.0fm to allow full burst.",
			cpuUsage.P50, cpuUsage.Max, recReq, recLimit)
	}

	// Apply confidence-gated reduction cap before floors.
	confidence := computeConfidence(stats)
	recReq, capApplied := capReduction(currentReqMillis, recReq, confidence)
	recLimit = math.Max(recLimit, recReq)
	if capApplied {
		var maxPct float64
		switch {
		case confidence > 0.8:
			maxPct = 75
		case confidence > 0.5:
			maxPct = 50
		default:
			maxPct = 30
		}
		reasoning += fmt.Sprintf(" (reduction capped at %.0f%% due to confidence level)", maxPct)
	}

	// Enforce floors.
	floorApplied := recReq < float64(minRecommendedCPUMillis)
	recReq = math.Max(recReq, float64(minRecommendedCPUMillis))
	recLimit = math.Max(recLimit, recReq)
	if floorApplied {
		reasoning += fmt.Sprintf(" (minimum floor of %dm applied — usage below threshold)", minRecommendedCPUMillis)
	}

	// Check if the recommendation actually reduces waste meaningfully.
	savings := float64(currentReqMillis) - recReq
	if savings <= 0 {
		return nil // Already right-sized or under-provisioned
	}
	wastePercent := (savings / float64(currentReqMillis)) * 100
	if wastePercent < wasteThresholdPercent {
		return nil // Not enough waste to warrant a recommendation
	}

	// Risk depends on pattern: for burst/batch patterns, the limit (not the
	// request) provides the safety margin — the request is intentionally low
	// for scheduling while the limit covers execution peaks.
	var risk models.RiskLevel
	switch stats.Pattern {
	case models.PatternBatch:
		risk = computeRisk(recLimit, cpuUsage.Max, stats)
	case models.PatternBurstable:
		risk = computeRisk(recLimit, cpuUsage.P99, stats)
	default:
		risk = computeRisk(recReq, cpuUsage.P99, stats)
	}

	return &models.Recommendation{
		Target:           target,
		Container:        containerName,
		Resource:         "cpu",
		CurrentRequest:   currentReqMillis,
		CurrentLimit:     currentLimitMillis,
		RecommendedReq:   int64(math.Ceil(recReq)),
		RecommendedLimit: int64(math.Ceil(recLimit)),
		Pattern:          stats.Pattern,
		Confidence:       confidence,
		Reasoning:        reasoning,
		EstSavings:       int64(savings),
		Risk:             risk,
		DataWindow:       time.Duration(dataWindowMinutes) * time.Minute,
	}
}

// GenerateMemoryRecommendation produces a right-sizing recommendation for memory.
// Returns nil if the workload is already well-sized.
func GenerateMemoryRecommendation(
	target models.OwnerReference,
	containerName string,
	memUsage models.MetricAggregate,
	currentReqBytes int64,
	currentLimitBytes int64,
	dataWindowMinutes float64,
	cpuPattern models.WorkloadPattern,
) *models.Recommendation {
	if currentReqBytes <= 0 {
		return nil
	}

	// Memory recommendations are more conservative — OOMKill is worse than
	// CPU throttling. Always use P99 as the baseline with extra headroom.
	var recReq, recLimit float64
	var reasoning string

	switch cpuPattern {
	case models.PatternIdle:
		recReq = math.Max(memUsage.P99*headroomSteady, float64(minRecommendedMemBytes))
		recLimit = math.Max(memUsage.Max*headroomBurstableLimit, recReq)
		reasoning = fmt.Sprintf(
			"Idle workload: P99 memory working set is %.1fMi against %.1fMi request. "+
				"Recommend %.1fMi (P99 + %.0f%% headroom).",
			memUsage.P99/1048576, float64(currentReqBytes)/1048576,
			recReq/1048576, (headroomSteady-1)*100)

	case models.PatternSteady:
		recReq = memUsage.P99 * headroomSteady
		recLimit = memUsage.Max * headroomBurstableLimit
		reasoning = fmt.Sprintf(
			"Steady workload: P99 memory is %.1fMi against %.1fMi request. "+
				"Recommend %.1fMi (P99 + %.0f%% headroom). Memory is predictable for this pattern.",
			memUsage.P99/1048576, float64(currentReqBytes)/1048576,
			recReq/1048576, (headroomSteady-1)*100)

	case models.PatternBurstable, models.PatternBatch:
		// For spiky workloads, use P99 for request and max with headroom for limit.
		recReq = memUsage.P99 * headroomBurstableLimit
		recLimit = memUsage.Max * headroomBurstableLimit
		reasoning = fmt.Sprintf(
			"Burstable/batch workload: P99 memory is %.1fMi, max is %.1fMi, request is %.1fMi. "+
				"Recommend %.1fMi request (P99 + %.0f%% headroom) to handle spikes safely.",
			memUsage.P99/1048576, memUsage.Max/1048576, float64(currentReqBytes)/1048576,
			recReq/1048576, (headroomBurstableLimit-1)*100)
	}

	// Memory confidence follows CPU pattern but is slightly more conservative.
	memStats := WorkloadStats{
		Pattern:           cpuPattern,
		DataWindowMinutes: dataWindowMinutes,
	}
	confidence := computeConfidence(memStats) * 0.95 // 5% less confident for memory

	// Apply confidence-gated reduction cap before floors.
	recReq, capApplied := capReduction(currentReqBytes, recReq, confidence)
	recLimit = math.Max(recLimit, recReq)
	if capApplied {
		var maxPct float64
		switch {
		case confidence > 0.8:
			maxPct = 75
		case confidence > 0.5:
			maxPct = 50
		default:
			maxPct = 30
		}
		reasoning += fmt.Sprintf(" (reduction capped at %.0f%% due to confidence level)", maxPct)
	}

	floorApplied := recReq < float64(minRecommendedMemBytes)
	recReq = math.Max(recReq, float64(minRecommendedMemBytes))
	recLimit = math.Max(recLimit, recReq)
	if floorApplied {
		reasoning += fmt.Sprintf(" (minimum floor of %.0fMi applied — usage below threshold)", float64(minRecommendedMemBytes)/1048576)
	}

	savings := float64(currentReqBytes) - recReq
	if savings <= 0 {
		return nil
	}
	wastePercent := (savings / float64(currentReqBytes)) * 100
	if wastePercent < wasteThresholdPercent {
		return nil
	}

	risk := computeMemoryRisk(recReq, memUsage)

	return &models.Recommendation{
		Target:           target,
		Container:        containerName,
		Resource:         "memory",
		CurrentRequest:   currentReqBytes,
		CurrentLimit:     currentLimitBytes,
		RecommendedReq:   int64(math.Ceil(recReq)),
		RecommendedLimit: int64(math.Ceil(recLimit)),
		Pattern:          cpuPattern,
		Confidence:       math.Round(confidence*100) / 100,
		Reasoning:        reasoning,
		EstSavings:       int64(savings),
		Risk:             risk,
		DataWindow:       time.Duration(dataWindowMinutes) * time.Minute,
	}
}

// computeConfidence determines how confident we are in a recommendation based
// on data window duration and workload pattern stability.
func computeConfidence(stats WorkloadStats) float64 {
	var base float64

	switch {
	case stats.DataWindowMinutes >= dataWindow7Days:
		base = confidenceMaxSteady7d
	case stats.DataWindowMinutes >= dataWindow3Days:
		// Linear interpolation between 3d and 7d.
		ratio := (stats.DataWindowMinutes - dataWindow3Days) / (dataWindow7Days - dataWindow3Days)
		base = confidenceMaxSteady3to7d + ratio*(confidenceMaxSteady7d-confidenceMaxSteady3to7d)
	default:
		// Linear scale from 0.2 (no data) to 0.5 (3 days).
		ratio := stats.DataWindowMinutes / dataWindow3Days
		base = 0.20 + ratio*(confidenceMaxShortWindow-0.20)
	}

	// Apply pattern-specific caps.
	switch stats.Pattern {
	case models.PatternBurstable:
		base = math.Min(base, confidenceCapBurstable)
	case models.PatternBatch:
		base = math.Min(base, confidenceCapBatch)
	case models.PatternIdle:
		// Idle is highly confident — if it's been idle for 48h+ we know.
		base = math.Min(base, confidenceMaxSteady7d)
	}

	return math.Round(base*100) / 100
}

// computeRisk determines the risk level based on how close the recommended
// request is to the P99 usage.
func computeRisk(recommendedReq float64, p99 float64, stats WorkloadStats) models.RiskLevel {
	if p99 <= 0 {
		return models.RiskLow
	}

	ratio := recommendedReq / p99

	if ratio < riskHighP99Proximity {
		return models.RiskHigh
	}
	if ratio < riskMediumP99Proximity {
		return models.RiskMedium
	}
	return models.RiskLow
}

// capReduction limits how much a single recommendation can reduce a resource,
// based on confidence. Prevents cliff-drop reductions on low-confidence data.
func capReduction(current int64, recommended float64, confidence float64) (float64, bool) {
	var maxReductionPct float64
	switch {
	case confidence > 0.8:
		maxReductionPct = 0.75
	case confidence > 0.5:
		maxReductionPct = 0.50
	default:
		maxReductionPct = 0.30
	}
	floor := float64(current) * (1.0 - maxReductionPct)
	if recommended < floor {
		return floor, true
	}
	return recommended, false
}

// computeMemoryRisk determines risk for memory recommendations. Memory risk is
// higher than CPU because OOMKill is more disruptive than CPU throttling.
func computeMemoryRisk(recommendedReq float64, memUsage models.MetricAggregate) models.RiskLevel {
	if memUsage.Max <= 0 {
		return models.RiskLow
	}

	// For memory, we compare against max (not P99) because a single spike
	// above the limit means OOMKill.
	ratio := recommendedReq / memUsage.Max

	if ratio < riskHighP99Proximity {
		return models.RiskHigh
	}
	if ratio < riskMediumP99Proximity {
		return models.RiskMedium
	}
	return models.RiskLow
}

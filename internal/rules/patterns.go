package rules

import (
	"math"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
)

const (
	// cvSteadyThreshold is the coefficient of variation below which a workload
	// is classified as steady. CV = stddev / mean; CV < 0.3 means low variance.
	cvSteadyThreshold = 0.3

	// idleUtilisationThreshold is the fraction of CPU request below which a
	// workload is considered idle. 0.05 = less than 5% of requested CPU.
	idleUtilisationThreshold = 0.05

	// idleMinDataWindow is the minimum observation window required before
	// classifying a workload as idle (48 hours).
	idleMinDataWindow = 48 * 60 // minutes

	// batchSpikeRatio is the minimum ratio of P99/P50 to detect periodic
	// spikes characteristic of batch workloads.
	batchSpikeRatio = 5.0

	// batchIdleRatio is the minimum ratio of max/P50 for batch detection.
	// Batch workloads are mostly idle with extreme spikes during execution.
	batchIdleRatio = 10.0
)

// ClassifyWorkload determines the workload pattern based on CPU usage metrics
// and resource requests. It uses the coefficient of variation, utilisation
// ratio, and spike analysis to categorise the workload.
func ClassifyWorkload(cpuUsage models.MetricAggregate, cpuRequestMillis int64, dataWindowMinutes float64) models.WorkloadPattern {
	// Check for idle first: mean usage < 5% of request for 48h+.
	// This must come before the "no data" check because zero usage with a
	// real request and sufficient data window IS idle, not "no data".
	if cpuRequestMillis > 0 && dataWindowMinutes >= idleMinDataWindow {
		utilisation := cpuUsage.P50 / float64(cpuRequestMillis)
		if utilisation < idleUtilisationThreshold {
			return models.PatternIdle
		}
	}

	// Cannot classify shape with no data.
	if cpuUsage.Max == 0 && cpuUsage.P50 == 0 {
		return models.PatternSteady // default to steady with no data
	}

	// Compute coefficient of variation from available percentiles.
	// We approximate stddev from the spread between P50 and P95.
	cv := estimateCV(cpuUsage)

	if cv < cvSteadyThreshold {
		return models.PatternSteady
	}

	// Check for batch pattern: extreme spikes with low baseline.
	if isBatchPattern(cpuUsage) {
		return models.PatternBatch
	}

	return models.PatternBurstable
}

// estimateCV approximates the coefficient of variation from percentile data.
// True CV requires raw samples, but we can estimate from the spread between
// P50 (median ≈ mean for symmetric distributions) and P95.
// CV = stddev / mean ≈ (P95 - P50) / (1.645 * P50) for normal-like distributions.
func estimateCV(agg models.MetricAggregate) float64 {
	if agg.P50 <= 0 {
		if agg.P95 > 0 {
			return 1.0 // high variation: P50 is zero but P95 is not
		}
		return 0 // no data
	}

	// Use interquartile-like spread normalised by the median.
	// (P95 - P50) represents the upper-tail spread.
	spread := agg.P95 - agg.P50
	return spread / agg.P50
}

// isBatchPattern detects batch workloads characterised by extreme spikes
// against an otherwise low baseline. Batch jobs show: high P99/P50 ratio
// (spiking), high Max/P50 ratio (job execution peaks), and significant
// gap between P50 and P95.
func isBatchPattern(agg models.MetricAggregate) bool {
	if agg.P50 <= 0 {
		return false
	}

	p99Ratio := agg.P99 / agg.P50
	maxRatio := agg.Max / agg.P50

	return p99Ratio >= batchSpikeRatio && maxRatio >= batchIdleRatio
}

// WorkloadStats holds derived statistics used for recommendation generation.
type WorkloadStats struct {
	Pattern           models.WorkloadPattern
	CV                float64
	UtilisationP50    float64 // P50 usage / request
	UtilisationP95    float64 // P95 usage / request
	UtilisationP99    float64 // P99 usage / request
	DataWindowMinutes float64
}

// ComputeWorkloadStats calculates derived statistics for a container's metrics.
func ComputeWorkloadStats(cpuUsage models.MetricAggregate, cpuRequestMillis int64, dataWindowMinutes float64) WorkloadStats {
	pattern := ClassifyWorkload(cpuUsage, cpuRequestMillis, dataWindowMinutes)
	cv := estimateCV(cpuUsage)

	var utilP50, utilP95, utilP99 float64
	if cpuRequestMillis > 0 {
		req := float64(cpuRequestMillis)
		utilP50 = cpuUsage.P50 / req
		utilP95 = cpuUsage.P95 / req
		utilP99 = cpuUsage.P99 / req
	}

	return WorkloadStats{
		Pattern:           pattern,
		CV:                cv,
		UtilisationP50:    math.Min(utilP50, 1.0),
		UtilisationP95:    math.Min(utilP95, 1.0),
		UtilisationP99:    utilP99,
		DataWindowMinutes: dataWindowMinutes,
	}
}

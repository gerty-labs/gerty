# L2 Rules Engine Fixes — Agreed Implementation Plan

**Date**: 2026-03-01
**Status**: Ready for implementation
**Based on**: L1 dogfooding report, code review of rules engine, live cluster data (21h Kind cluster)

## Context

k8s-sage has been running on a Kind cluster (3 workers + control plane) for ~21 hours with 8 synthetic dogfood workloads + k8s-sage's own components. The L1 rules engine produces recommendations but dogfooding revealed several issues. This document captures the **agreed fixes** after reviewing and challenging the original L1 proposals.

## Priority Order

1. Fix classification near-zero P50 explosion (root cause of most issues)
2. Raise global resource floors (eliminates dangerous minimum recommendations)
3. Add memory leak / anomaly detection (safety-critical)
4. Add missing workload visibility (best-effort handling + well-sized reporting)
5. Investigate duplicate per-replica entries (existing aggregation code may just need wiring fix)
6. Confidence-gated reduction caps, reasoning text cleanup, further refinements

---

## Fix 1: Classification — Absolute Threshold Guard on Near-Zero P50

**Root cause**: The CV estimation (`(P95 - P50) / P50`) and batch detection ratios (`P99/P50`, `Max/P50`) explode when P50 approaches zero. A daemon with P50=1m and Max=10m gets ratios of 10x, triggering batch classification. This is the root cause of k8s-sage server, agents, and kindnet all being misclassified as batch.

**Fix**: Add an absolute-value guard before computing ratios. If P50 is below an absolute floor AND the spike amplitude is small in absolute terms, classify based on absolute values, not ratios.

```
IF P50 < 25m CPU AND Max < 100m CPU:
    → classify as STEADY (low-usage daemon, ratio math is meaningless)
IF P50 < 25m CPU AND Max >= 100m CPU:
    → classify as BURSTABLE (low baseline but real spikes)
ELSE:
    → proceed with existing CV / ratio classification
```

**Where**: `internal/rules/patterns.go`, `ClassifyWorkload()` (lines 34-64). Add guard before the CV calculation at line ~45.

**New constants**:
```go
const (
    lowUsageP50Floor    = 25   // millicores — below this, ratio math is unreliable
    lowUsageSpikeFloor  = 100  // millicores — spikes below this are noise, not batch
)
```

**Classification hysteresis** (future refinement): Use exponential moving average of CV across analysis cycles to smooth transitions. Not required for initial fix — the absolute guard eliminates the observed flip-flopping.

**Workload metadata as soft priors** (deferred): DaemonSet/Deployment type could bias classification slightly toward steady, but only as a tiebreaker overridden by strong usage evidence. The absolute threshold guard eliminates the need for this now. Revisit if misclassifications persist after Fix 1.

**Tests to add**:
- Near-zero P50 daemon (P50=1m, P95=3m, Max=10m) → steady, not batch
- Low-baseline high-spike (P50=5m, P95=50m, Max=500m) → burstable
- Genuine batch (P50=200m, P95=1500m, Max=2000m) → batch (unchanged)
- Existing classification tests still pass

---

## Fix 2: Raise Global Resource Floors

**Problem**: Current floors (10m CPU, 4Mi memory) are below the minimum viable resources for any real container. Multiple workloads hit the 10m CPU floor, making the floor look like the recommendation.

**Fix**: Raise global defaults to reasonable minimums.

```go
const (
    minRecommendedCPUMillis = 50                    // was 10
    minRecommendedMemBytes  = 64 * 1024 * 1024      // was 4 MiB → now 64 MiB
)
```

**Where**: `internal/rules/recommendations.go` (lines 51-57).

**Reasoning text fix**: When a recommendation hits the floor, the reasoning should say so explicitly. Currently says "Request set to 0m" when the actual recommendation is the floor value. Fix the reasoning template to append "(minimum floor applied)" when `calculatedValue < floor`.

**Where**: `GenerateCPURecommendation()` (lines 72-170) and `GenerateMemoryRecommendation()` (lines 174-258) — update the `Reasoning` string construction.

**Per-runtime floors** (deferred): JVM 128Mi, nginx 32Mi, Python/Node 64Mi etc. Requires image name / env var detection which is fragile. Revisit after global floors are validated. The 64Mi global floor covers the common case.

**Tests to add**:
- Workload with usage below floor → recommendation equals floor, not usage
- Reasoning text contains "minimum floor" when floor is applied
- Existing recommendation tests updated for new floor values

---

## Fix 3: Memory Leak / Anomaly Detection

**Problem**: The `memory-leak` workload shows monotonic memory growth but is classified as batch and gets a memory *reduction* recommendation. This is actively harmful.

**Fix**: Add monotonic growth detection before classification. If detected, classify as `anomalous` and emit an investigation recommendation instead of sizing.

**New pattern**: Add `PatternAnomalous` to the WorkloadPattern enum.

**Where**: `internal/models/recommendation.go` (line 12, add after PatternIdle).

**Detection algorithm**: Rather than the fragile "80% of buckets are higher" approach, use growth rate projection:

```
1. Divide data window into 4 equal segments
2. Calculate average memory usage per segment
3. If each segment's average is higher than the previous (3/3 increases):
   AND the growth from segment 1 → segment 4 is > 20% of segment 1:
   → Flag as anomalous (monotonic growth)
4. Project: if growth rate continues, would memory exceed current request
   within 2x the current data window?
   → If yes, risk = HIGH
```

This catches real leaks (linear/exponential growth toward OOM) while ignoring normal warm-up that plateaus (warm-up shows growth in segments 1-2 but stabilises in 3-4).

**Where**: `internal/rules/patterns.go` — new function `detectMonotonicGrowth()` called before `ClassifyWorkload()`. Requires memory time-series data (segment averages), not just percentiles. May need the agent store to expose segment summaries.

**Safety invariant**: Never recommend memory reduction on a workload with rising memory trend, regardless of classification.

**Recommendation output for anomalous**:
```
Pattern:     anomalous
Reasoning:   "Monotonic memory growth detected — usage increased X% over Y hours
              with no significant drops. Possible memory leak. Do not resize."
Risk:        HIGH
Confidence:  0.0 (not applicable — investigation needed, not sizing)
Action:      INVESTIGATE (new field, or encode in reasoning)
```

**Data dependency**: The current `MetricAggregate` struct only has P50/P95/P99/Max — no time-series segments. Options:
- A) Add segment averages to the aggregate (agent-side change)
- B) Use P50 vs P95 vs P99 spread as a proxy (less accurate but no agent change)
- C) Add a separate `MemoryTrend` field to the report

Option A is cleanest. The agent store already has time-bucketed data (`fine` and `coarse` buckets in `store.go`) — expose 4-segment averages in the report.

**Tests to add**:
- Monotonic growth (each segment higher) → anomalous
- Normal warm-up (growth then plateau) → not anomalous
- Sawtooth pattern (GC cycles) → not anomalous
- Anomalous workload gets investigation recommendation, not sizing
- Memory reduction blocked when trend is rising

---

## Fix 4: Missing Workload Visibility

### 4a: Best-Effort Pods (no resource requests)

**Problem**: Pods with no resource requests get `nil` from the recommendation functions (`currentReq <= 0 → return nil`). They silently disappear from the report. These are the most at-risk pods — first to be OOM-killed under node pressure.

**Fix**: Add a new recommendation type for best-effort pods: "ADD resource requests based on observed usage."

**Where**: `internal/rules/recommendations.go` — both `GenerateCPURecommendation()` and `GenerateMemoryRecommendation()`. Instead of returning nil when `currentReq <= 0`, generate a recommendation with:
```
CurrentRequest:    0
RecommendedReq:    P95 * 1.20 (or P99 * 1.20 for memory)
RecommendedLimit:  P99 * 1.20 (or Max * 1.20 for memory)
Reasoning:         "No resource requests set (BestEffort QoS). This pod is at risk
                    of eviction under node pressure. Based on observed usage,
                    recommend adding requests."
Risk:              HIGH (BestEffort is always high risk)
```

### 4b: Well-Sized Workloads

**Problem**: Workloads where waste is < 10% (the `wasteThresholdPercent`) are filtered out. The `right-sized` dogfood workload doesn't appear in recommendations at all.

**Fix**: Instead of returning nil, return a recommendation with zero savings and a positive message:

```
EstSavings:  0
Reasoning:   "Workload is well-sized — current requests are within 10% of observed
              usage. No changes recommended."
Risk:        LOW
```

**Where**: `internal/rules/recommendations.go` — the waste threshold check (around line 100 for CPU, line 210 for memory). Return a "no change" recommendation instead of nil.

**Tests to add**:
- Best-effort pod (0 requests) → gets ADD recommendation with observed-usage-based values
- Well-sized pod (waste < 10%) → appears in report with "no changes" message
- Both types visible in /api/v1/recommendations output

---

## Fix 5: Investigate Duplicate Per-Replica Entries

**Problem**: Workloads with multiple replicas (nginx x3, api-bursty x2, agent x3) show separate entries per pod rather than one aggregated recommendation.

**Investigation needed**: The aggregation code already exists (`aggregateByOwner()` in `internal/server/aggregator.go`, lines 201-264) with conservative max strategy for percentile merging. Before writing new code, determine:

1. Is `HandleRecommendations` in `api.go` (line 252) calling `AnalyzeCluster` with already-aggregated data?
2. Are pods missing OwnerReferences? (unlikely for Deployments, but check Kind-specific behaviour)
3. Is the aggregation path wired correctly end-to-end: ingest → store → aggregate → analyze → API?

**Where to look**: `internal/server/api.go:252` (HandleRecommendations) → trace the data flow back to aggregation.

**Fix**: Likely a wiring issue, not a missing feature. Fix the data flow so `AnalyzeCluster` receives owner-aggregated data.

**Savings display**: Once deduplication works, include replica count in reasoning: "3 replicas × 768Mi saving = 2.3Gi total memory freed."

---

## Fix 6: Confidence-Gated Reduction Caps

**Problem**: Even with correct classification and sensible floors, a recommendation based on 9 hours of data shouldn't suggest drastic reductions. High-confidence data can still be unrepresentative (maintenance windows, traffic rerouting, feature flags off).

**Fix**: Tie maximum reduction percentage to confidence score. Always enforce a cap — never allow unlimited reduction in a single cycle.

| Confidence | Max Reduction Per Cycle |
|------------|------------------------|
| < 0.5      | 30%                    |
| 0.5 – 0.8  | 50%                    |
| > 0.8      | 75%                    |

**Where**: `internal/rules/recommendations.go` — apply after recommendation calculation but before returning, in both `GenerateCPURecommendation()` and `GenerateMemoryRecommendation()`.

```go
func capReduction(current, recommended int64, confidence float64) int64 {
    var maxReductionPct float64
    switch {
    case confidence > 0.8:
        maxReductionPct = 0.75
    case confidence > 0.5:
        maxReductionPct = 0.50
    default:
        maxReductionPct = 0.30
    }
    floor := int64(float64(current) * (1.0 - maxReductionPct))
    if recommended < floor {
        return floor
    }
    return recommended
}
```

**Interaction with global floors**: The reduction cap is applied first, then the global floor. So a workload at 100m CPU with confidence 0.3 gets capped at 70m (30% max reduction), then checked against 50m floor — result is 70m. The floor only kicks in if the cap still results in something below it.

**Tests to add**:
- Low confidence (0.24) + large waste → capped at 30% reduction
- Medium confidence (0.65) → capped at 50%
- High confidence (0.90) → capped at 75%
- Cap applied before floor check
- Reasoning text notes when cap is applied: "(reduction capped at X% due to confidence level)"

---

## Validation Checklist (Post-Implementation)

Re-run against the same dogfood cluster and verify:

- [ ] k8s-sage server, agents, kindnet classified as steady (not batch)
- [ ] api-bursty classified as burstable consistently
- [ ] No recommendation below 50m CPU or 64Mi memory
- [ ] Reasoning text says "minimum floor applied" when floor is hit
- [ ] memory-leak classified as anomalous with investigation recommendation (not reduction)
- [ ] No memory reduction on workloads with rising memory trend
- [ ] right-sized workload appears in report with "no changes recommended"
- [ ] best-effort workload appears with "add resource requests" recommendation
- [ ] Recommendations aggregated per Deployment/DaemonSet (one entry, not per-pod)
- [ ] No recommendation reduces any resource by more than 75% in a single cycle
- [ ] Low-confidence (< 0.5) recommendations capped at 30% reduction
- [ ] All existing unit tests pass (updated for new floor values)
- [ ] New tests cover all six fixes

## File Reference

| File | Key Lines | What Changes |
|------|-----------|-------------|
| `internal/models/recommendation.go` | 5-13 | Add `PatternAnomalous` |
| `internal/rules/patterns.go` | 9-29, 34-64 | Add absolute threshold guard, new constants, `detectMonotonicGrowth()` |
| `internal/rules/recommendations.go` | 51-57, 72-170, 174-258, 262-290 | New floors, reduction cap, best-effort handling, well-sized reporting, reasoning text |
| `internal/rules/engine.go` | 39-90 | Pass memory trend data to classification |
| `internal/server/api.go` | 252-286 | Verify aggregation wiring |
| `internal/server/aggregator.go` | 201-264 | Debug why aggregation isn't reaching API |
| `internal/agent/store.go` | (TBD) | Expose memory segment averages for trend detection |
| `internal/rules/patterns_test.go` | (new tests) | Near-zero P50, anomaly detection |
| `internal/rules/recommendations_test.go` | (update + new) | New floors, caps, best-effort, well-sized |
| `internal/rules/engine_test.go` | (update + new) | End-to-end with new patterns |

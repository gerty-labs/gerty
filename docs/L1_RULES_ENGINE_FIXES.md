# L1 Rules Engine Fixes — Dogfooding Issues

## Context

We've been running k8s-sage on a Kind cluster (3 workers + control plane) for ~9 hours with 11 synthetic workloads. The L1 rules engine is producing recommendations but dogfooding has revealed several issues that need fixing before L2 integration.

**Dogfood report is in the repo — reference it for full data.**

## Issue 1: Recommendations Are Dangerously Aggressive

**Problem:** The engine is recommending drastic reductions that would cause outages in production.

Examples from dogfooding:
- `idle-dev`: 512Mi → 4Mi memory (99% reduction)
- `memory-leak`: 256Mi → 4Mi memory (98% reduction) 
- `nginx-overprovisioned`: 1024Mi → 13Mi memory (99% reduction)
- Nearly every workload gets CPU recommended to exactly 10m

A 99% reduction is almost never safe. Even an idle nginx container needs more than 13Mi to function. The engine is looking at current usage and recommending based on that with minimal headroom.

**Fix needed:**

1. **Increase headroom multipliers.** The safety margin between observed usage and recommendation is too thin. Recommendations should include meaningful headroom above P95/P99 observed usage — not just 5-10% above current, but enough to handle normal variance. A common pattern is `max(observed_p99 * 1.3, runtime_minimum)`.

2. **Implement per-runtime minimum floors.** Different workloads have different minimum viable resources regardless of current usage:
   - JVM containers: minimum 128Mi memory (heap + metaspace + thread stacks + OS overhead), minimum 100m CPU (GC needs CPU)
   - nginx/web servers: minimum 32Mi memory, minimum 10m CPU
   - Python/Node apps: minimum 64Mi memory, minimum 50m CPU  
   - Generic/unknown: minimum 64Mi memory, minimum 25m CPU
   
   These should be configurable but have sane defaults. The engine should detect runtime from container image name, command, and environment variables (JAVA_OPTS, -Xmx, NODE_OPTIONS, etc.) and apply the appropriate floor.

3. **Cap maximum reduction percentage per cycle.** No single recommendation should reduce a resource by more than 50% in one step, regardless of what the math says. If the engine thinks nginx should go from 1024Mi to 13Mi, the first recommendation should be 1024Mi → 512Mi. Next cycle, if usage confirms, 512Mi → 256Mi. Step-down, not cliff-drop. This protects against bad data windows and gives the workload time to demonstrate real needs at each level.

4. **The 10m CPU floor is a clamp, not a recommendation.** If multiple workloads with very different characteristics all get the same CPU recommendation, that's a floor value being hit, not intelligence. Review the minimum CPU logic — if observed CPU is below the floor, the recommendation should be the floor with a note like "usage below minimum threshold" rather than presenting the floor as a calculated recommendation.

## Issue 2: Memory Leak Detection Missing

**Problem:** The `memory-leak` workload is classified as `batch` and gets a memory *reduction* recommendation (256Mi → 4Mi). This is the opposite of correct — a leaking workload should never get a memory reduction.

**What's happening:** The engine sees memory that rises over time and classifies it as batch (because it's not steady). But a batch workload spikes up during processing then drops back down. A memory leak goes up and never comes back down.

**Fix needed:**

1. **Add monotonic growth detection.** Before classifying a workload, check if memory usage is trending upward without significant drops:
   - Divide the data window into segments (e.g., 30-minute buckets)
   - If average memory in each successive bucket is higher than the previous (with some tolerance, say 80% of buckets are higher than their predecessor)
   - And there are no significant drops (>20% decrease from any peak)
   - Then flag as `anomalous` pattern, not `batch`

2. **Anomalous workloads should get investigation recommendations, not sizing recommendations.** The output for memory-leak should be something like:
   ```
   Pattern: anomalous (monotonic memory growth detected)
   Recommendation: INVESTIGATE — memory usage growing linearly without release, possible memory leak
   Action: Do not resize. Examine application for memory leak.
   Risk: HIGH
   Confidence: N/A (anomalous behaviour detected)
   ```

3. **Never recommend memory reduction on a workload with rising memory trend.** Even if current usage is low (early in the leak cycle), if the trend is upward, reducing memory will just accelerate the OOM kill.

## Issue 3: Classification Mismatches

**Problem:** Several workloads are misclassified:
- `k8s-sage/server` → classified as `batch`, should be `steady` (it's a long-running HTTP server)
- `k8s-sage/agent` (x3) → classified as `batch`, should be `steady` (long-running DaemonSet agents)
- `kube-system/kindnet` (x3) → classified as `batch`, should be `steady` (CNI plugin, always running)
- `api-bursty` → fluctuates between `burstable` and `steady` depending on data window

**What's happening:** The classification logic is probably seeing periodic small spikes (metric collection intervals, health checks, periodic scrapes) and interpreting those as batch processing patterns. But batch workloads have a fundamentally different shape — they start, process intensively, then complete or go idle. A daemon with periodic 5-second spikes is not batch.

**Fix needed:**

1. **Distinguish periodic micro-spikes from batch processing.** Batch workloads have:
   - High amplitude spikes (>50% of limit or request)
   - Clear start/stop cycles
   - Extended idle periods between cycles
   
   Long-running daemons with periodic spikes have:
   - Low amplitude spikes (<20% of request)
   - Regular/predictable intervals
   - Never truly idle (baseline always present)

2. **Use uptime/restart count as a signal.** If a pod has been running continuously for hours with no restarts, it's much more likely steady or burstable than batch. Batch workloads typically have shorter pod lifetimes or are Jobs/CronJobs.

3. **Consider workload type metadata.** DaemonSets are almost never batch. Deployments with `replicas > 1` are rarely batch. Jobs and CronJobs are batch by definition. This metadata should weight the classification.

4. **Classification stability.** If a workload was classified as `burstable` 2 hours ago and is now `steady`, the engine shouldn't flip immediately. Add hysteresis — require sustained evidence (e.g., 3+ consecutive analysis cycles agreeing) before changing a classification. This prevents the api-bursty flip-flopping issue.

## Issue 4: Missing Workloads

**Problem:** The `right-sized` and `best-effort` workloads from the dogfood manifest are not appearing in recommendations at all.

**Fix needed:**

1. **Check if they're being analysed and deliberately filtered out** (correct behaviour if right-sized workload has good resource fit and best-effort has no requests to adjust) **or silently dropped** (bug).

2. **If deliberately filtered:** Add them to the report anyway with a status like "analysed — no changes recommended" so the operator knows Sage has seen them. Silent omission makes it look like Sage missed them.

3. **If best-effort (no resource requests/limits set):** The recommendation should be to ADD requests/limits, not skip the workload. A best-effort workload is the most at-risk in the cluster — it'll be first to get OOM-killed under pressure. Sage should recommend initial sizing based on observed usage.

## Issue 5: Duplicate Per-Replica Entries

**Problem:** Workloads with multiple replicas (nginx-overprovisioned x3, api-bursty x2, agent x3, kindnet x3) show separate entries per pod rather than one aggregated recommendation per Deployment/DaemonSet.

**Fix needed:**

1. **Aggregate at the workload controller level** (Deployment, StatefulSet, DaemonSet), not at the pod level. All replicas of `nginx-overprovisioned` share the same pod spec — they should get one recommendation.

2. **Use the highest resource usage across replicas** for the recommendation calculation (not average). If one replica is using 200Mi and another is using 50Mi, the recommendation should be based on the 200Mi replica plus headroom. Averaging down would under-provision the busy replica.

3. **Report replica count** in the recommendation so the operator understands the cluster-wide impact (e.g., "3 replicas × 768Mi saving = 2.3Gi total memory freed").

## Priority Order

1. **Issue 1 (aggressive recommendations)** — highest priority, these would cause outages if applied
2. **Issue 2 (memory leak detection)** — recommending reduction on a leaking workload is actively harmful  
3. **Issue 3 (classification)** — affects recommendation quality but less dangerous than 1 and 2
4. **Issue 5 (deduplication)** — UX issue, important but not safety-critical
5. **Issue 4 (missing workloads)** — investigate whether bug or expected behaviour, then fix accordingly

## Validation

After fixes, re-run against the same dogfood cluster and verify:
- [ ] No recommendation reduces any resource by more than 50% in a single step
- [ ] No workload gets recommended below its runtime minimum floor
- [ ] memory-leak is classified as anomalous, not batch
- [ ] memory-leak gets an investigation recommendation, not a reduction
- [ ] k8s-sage server, agents, and kindnet are classified as steady, not batch
- [ ] api-bursty maintains burstable classification across data windows
- [ ] Recommendations are aggregated per Deployment/DaemonSet, not per pod
- [ ] right-sized and best-effort workloads appear in the report (even if "no changes needed")
- [ ] best-effort workloads get "add resource requests" recommendation

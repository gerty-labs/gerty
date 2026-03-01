# K8s Documentation Instruction Pairs -- Generation Report

**Generated**: 2026-03-01
**Output file**: `ml/dataset/raw/k8s_docs_pairs.jsonl`
**Licence**: Kubernetes documentation is licenced under Apache 2.0. Provenance URLs are included in every pair's metadata.

## Source Pages

| # | Page | Pairs | Status |
|---|------|-------|--------|
| 1 | [Resource Management for Containers](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/) | 5 | Conceptual + applied + operational + units + BestEffort edge case |
| 2 | [Resource Quotas](https://kubernetes.io/docs/concepts/policy/resource-quotas/) | 3 | Conceptual + applied (quota-constrained right-sizing) + operational gotchas |
| 3 | [Limit Ranges](https://kubernetes.io/docs/concepts/policy/limit-range/) | 3 | Conceptual + applied (LimitRange defaults) + edge cases |
| 4 | [Assign CPU Resources](https://kubernetes.io/docs/tasks/configure-pod-container/assign-cpu-resource/) | 3 | CFS throttling deep-dive + applied (Go GOMAXPROCS) + CPU gotchas |
| 5 | [Assign Memory Resources](https://kubernetes.io/docs/tasks/configure-pod-container/assign-memory-resource/) | 3 | Memory/OOM mechanics + applied (JVM right-sizing) + memory gotchas |
| 6 | [Quality of Service for Pods](https://kubernetes.io/docs/tasks/configure-pod-container/quality-service-pod/) | 3 | QoS class explanation + applied (payment gateway) + QoS gotchas |
| 7 | [Pod Overhead](https://kubernetes.io/docs/concepts/scheduling-eviction/pod-overhead/) | 2 | Conceptual + applied (Kata Containers overhead accounting) |
| 8 | [Horizontal Pod Autoscaler](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/) | 3 | Conceptual (HPA mechanics) + applied (HPA + right-sizing) + HPA gotchas |
| 9 | [Autoscaling](https://kubernetes.io/docs/concepts/workloads/autoscaling/) | 2 | HPA/VPA/CA overview + applied (well-sized ETL steady workload) |
| -- | [Configure Pod ConfigMap](https://kubernetes.io/docs/tasks/configure-pod-container/configure-pod-configmap/) | 0 | Skipped -- wrong topic (not resource-related) |

## Summary Statistics

| Metric | Count |
|--------|-------|
| **Total pairs** | **27** |
| Pages successfully used | 9 of 10 (1 skipped: ConfigMap) |
| Average pairs per page | 3.0 |

## Category Breakdown

| Category | Count | Percentage |
|----------|-------|------------|
| right-sizing | 15 | 55.6% |
| edge-case | 6 | 22.2% |
| classification | 4 | 14.8% |
| runtime-specific | 2 | 7.4% |

## Pair Type Breakdown

| Type | Description | Count |
|------|-------------|-------|
| Conceptual | "What is X and how does it work?" | 9 |
| Applied | Realistic metrics scenario with right-sizing recommendations | 9 |
| Operational | Common mistakes, gotchas, edge cases | 9 |

## Data Quality Notes

- All `id` fields match pattern `^[a-z0-9-]+$` in format `{source}-{category}-{sequence}`
- All `source` fields are `"k8s-docs"`
- All `system` fields use the canonical k8s-sage system prompt
- All `assistant` fields exceed 50 characters (most exceed 1000 characters)
- All `metadata.needs_review` fields are `true`
- All `metadata.provenance` fields are valid URLs to the corresponding K8s docs page

### Metric consistency checks

All applied pairs use realistic metrics where P50 <= P95 <= P99 <= Max:

| Pair ID | CPU (P50/P95/P99/Max) | Memory (P50/P95/P99/Max) | Valid |
|---------|----------------------|--------------------------|-------|
| right-sizing-002 | 120m/350m/480m/750m | 512Mi/800Mi/950Mi/1100Mi | Yes |
| right-sizing-006 | 60m/110m/150m/200m | 300Mi/450Mi/500Mi/550Mi | Yes |
| right-sizing-008 | 15m/30m/45m/80m | 32Mi/48Mi/55Mi/60Mi | Yes |
| right-sizing-010 | 180m/450m/700m/1200m | n/a | Yes |
| right-sizing-012 | n/a | 620Mi/740Mi/780Mi/830Mi | Yes |
| right-sizing-014 (steady) | 680m/850m/920m/1100m | 400Mi/550Mi/600Mi/650Mi | Yes |
| right-sizing-014 (off-peak) | 180m/280m/350m/400m | 350Mi/450Mi/500Mi/520Mi | Yes |
| classification-002 | 80m/150m/200m/280m | 400Mi/600Mi/700Mi/780Mi | Yes |
| classification-004 | 3200m/3600m/3750m/3900m | 5800Mi/6400Mi/6700Mi/7100Mi | Yes |
| runtime-specific-002 | 100m/250m/320m/450m | 200Mi/300Mi/340Mi/380Mi | Yes |

### Recommendation formula compliance

All CPU recommendations satisfy `recommended >= P95 * 1.20` (for steady/burstable) or `recommended >= P50 * 1.20` where noted.
All memory recommendations satisfy `recommended >= P99 * 1.10`.
CPU floor of 10m and memory floor of 4Mi are respected in all pairs.

## Licence and Provenance

The Kubernetes documentation is licenced under the [Apache License 2.0](https://github.com/kubernetes/website/blob/main/LICENSE). Content was used to inform instruction pair generation. Each pair includes a `metadata.provenance` URL pointing to the specific documentation page that informed the pair's content. All pairs are marked `needs_review: true` for human review before use in training.

## Sample Pairs for Quality Review

### Sample 1: Conceptual (right-sizing-001)

**User**: What are Kubernetes resource requests and limits, and how do they affect pod scheduling and runtime behaviour?

**Assistant** (truncated): Kubernetes resource requests and limits are the two levers for controlling how much CPU and memory a container can use. **Requests** are what the scheduler uses to decide where to place a pod. [...] **Limits** are the hard ceiling enforced at runtime: CPU limits are enforced via CFS quota (throttling), memory limits via the OOM killer. [...]

**Category**: right-sizing | **Provenance**: manage-resources-containers

---

### Sample 2: Applied (right-sizing-010)

**User**: I have a Go microservice deployment `api-gateway` with 6 replicas. Current config: requests 2 CPU, limits 2 CPU. Metrics over 10 days: CPU: P50=180m, P95=450m, P99=700m, Max=1200m. The service is multi-threaded (GOMAXPROCS not set). I see `container_cpu_cfs_throttled_periods_total` is 40% of total periods. What should I do?

**Assistant** (truncated): You have a serious CFS throttling problem. [...] Go runtime defaults GOMAXPROCS to the number of CPU cores on the *node* not the container limit. On an 8-core node, 8 threads can consume the 200ms CFS quota in 25ms of wall-clock time, then the container is throttled for 75ms. [...] Phase 1: Set GOMAXPROCS=2. Phase 2: Switch to Burstable with 550m request and no CPU limit.

**Category**: right-sizing | **Provenance**: assign-cpu-resource

---

### Sample 3: Operational (edge-case-005)

**User**: What are common HPA configuration mistakes that lead to scaling problems or resource waste?

**Assistant** (truncated): 1. CPU requests are too high, so HPA never triggers. 2. CPU limits cause throttling that masks the need to scale. 3. minReplicas too high -- paying for idle capacity. [...] 9. Conflicting HPA and VPA -- they can oscillate. Use VPA in recommend-only mode alongside HPA.

**Category**: edge-case | **Provenance**: horizontal-pod-autoscale

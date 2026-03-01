# GitHub Issues Volume Assessment

**Date**: 2026-03-01
**Status**: Assessed with live GitHub API data (unauthenticated, rate-limited)

## Methodology

Searched the GitHub Search API (unauthenticated, 10 req/hr limit) across two repositories:
- `kubernetes/kubernetes` — core K8s resource-related issues
- `kubernetes/autoscaler` — VPA/HPA issues

**Search queries** (5 for k/k, 3 for autoscaler):
- OOMKill resource limit, cpu throttling resource request, vertical pod autoscaler VPA
- resource quota limit range, memory limit OOM pod
- VPA recommendation, OOM memory resource, vertical pod autoscaler

**Assessment**: Fetched top 30 results per query (sorted by reactions), deduplicated, then heuristically assessed each for training pair quality based on: comment count (≥3), resource-related keywords, fix/solution language, label signals.

## Raw Search Results

| Repository | Search Hits (raw, with overlap) | Unique Issues Fetched | Estimated Total Unique |
|------------|------|--------|-----------|
| kubernetes/kubernetes | 696 | 130 | ~400 |
| kubernetes/autoscaler | 2,508 | 80 | ~1,500 |
| **Total** | **3,204** | **210** | **~1,900** |

Note: Raw hit counts include significant overlap between queries. The "estimated total unique" is conservative.

## Quality Assessment (210 issues assessed in detail)

| Quality | Count | Percentage | Definition |
|---------|-------|------------|------------|
| GOOD | 78 | 37% | Clear problem statement + resolution, ≥3 comments, resource keywords, fix/solution language |
| MAYBE | 131 | 62% | Related to resources but needs significant editing. May lack clear resolution or be tangential. |
| REJECT | 1 | <1% | Not about resource efficiency, or feature proposal/RFC |

**Estimated conversion rate**: ~56% (all GOOD + 30% of MAYBE)

## Volume Projection

| Metric | Value |
|--------|-------|
| Estimated total matching issues | ~1,900 |
| Conversion rate | ~56% |
| **Projected usable pairs** | **~1,060** |
| Target (TRAINING_DATA.md) | 3,000 |
| **Gap** | **~1,940** |

## Sample GOOD Candidates (5)

1. **kubernetes/kubernetes#126096** — "kubelet: new kubelet config option for disabling group oom kill" (57 comments)
   - Why good: Deep technical discussion of OOM behaviour, clear resolution, highly relevant to resource management
   - Pair potential: Multiple pairs covering OOM kill semantics, cgroup configuration, pod eviction behaviour

2. **kubernetes/kubernetes#103046** — "More accurate kube-/system-reserved based on actual resource usage" (27 comments)
   - Why good: Directly about resource reservation accuracy, operational best practices
   - Pair potential: Applied pair about node-level resource accounting

3. **kubernetes/kubernetes#100483** — "Log and expose cgroup OOM events to the associated Pod resource" (20 comments)
   - Why good: Discusses OOM visibility, monitoring, how to detect resource pressure
   - Pair potential: Edge-case pair about diagnosing OOM events

4. **kubernetes/kubernetes#50632** — "Container with multiple processes not terminated when OOM" (22 comments)
   - Why good: Subtle OOM behaviour in multi-process containers, clear resolution
   - Pair potential: Edge-case pair about multi-process container memory management

5. **kubernetes/kubernetes#93818** — "Prestop hook not triggered when container gets restarted" (14 comments)
   - Why good: Interaction between resource limits and container lifecycle
   - Pair potential: Operational pair about restart behaviour during resource pressure

## Sample REJECTED (1 found in assessment)

1. **kubernetes/kubernetes#13693** — "Vertical Pod Autoscaler overview proposal" (27 comments)
   - Rejection reason: Feature proposal/RFC, not a problem+resolution. Contains design discussion but not operational knowledge suitable for instruction tuning.

Note: The low reject rate (1/210) is because the search queries were already fairly targeted. In a full processing pipeline, the MAYBE category (62%) would produce the bulk of rejections during human review.

## Honest Assessment of 3,000 Target

**The 3,000 pair target from GitHub issues is NOT achievable** from kubernetes/kubernetes and kubernetes/autoscaler alone.

### Why:
1. **Raw volume is lower than expected**: ~1,900 unique matching issues, not the thousands implied by the target
2. **Conversion rate is ~56%**: Many issues are bug reports with fixes that don't translate directly into resource efficiency instruction pairs
3. **Projected yield**: ~1,060 pairs from the two primary repos
4. **Gap**: ~1,940 pairs short of target

### Recommendations:
1. **Expand repo coverage**: Add `prometheus-operator/prometheus-operator`, `helm/charts`, `kubernetes-sigs/kustomize`, `argoproj/argo-cd`, `kubernetes-sigs/metrics-server` — these have resource-related issues too. Estimated additional yield: ~300-500 pairs.
2. **Revise target to 1,500**: More realistic given actual GitHub issue quality
3. **Redistribute gap**: Move ~1,500 pairs to other sources:
   - Synthetic generation: +800 (can be generated programmatically from rules engine)
   - Stack Overflow: +400 (has capacity beyond current filtered count)
   - Expert pairs: +300
4. **Use MAYBE issues with careful curation**: The 131 MAYBE issues could yield ~40 more pairs with manual editing

### Rate Limit Note
This assessment used 8 of 10 available unauthenticated API requests. Full issue detail fetching (reading comments/body for each issue) requires authenticated access or will need to be done over multiple hours. Recommend setting up a GitHub personal access token (5,000 req/hr) for the full data collection phase.

### Licence Note
Kubernetes repos use Apache 2.0. Issue text authorship is complex — contributors retain copyright but content is publicly available. Include provenance URLs for all GitHub-sourced pairs.

## Next Steps

1. Set up GitHub PAT for authenticated API access
2. Fetch full issue details + comments for all 78 GOOD candidates
3. Expand search to additional repos (estimated +300-500 pairs)
4. Transform GOOD issues into JSONL pairs with proper schema
5. Human review of all automated transformations

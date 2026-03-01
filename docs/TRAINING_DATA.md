# k8s-sage Training Data Strategy

## Overview

The k8s-sage SLM requires a curated instruction-tuning dataset of Kubernetes resource efficiency knowledge. No such dataset exists today — general-purpose LLM training data contains K8s documentation but lacks the operational depth needed for right-sizing decisions.

**Target**: 12,000 instruction pairs, 85% derived from real sources, 15% synthetic gap-fillers.

---

## Source Breakdown

| Source | Type | Volume (est.) | Content | Licence |
|--------|------|---------------|---------|---------|
| K8s official docs + GKE/EKS/AKS | Extracted + transformed | ~3,500 pairs | Resource management, QoS classes, VPA/HPA, LimitRange, ResourceQuota, provider-specific best practices | Apache 2.0 (K8s), proprietary (provider docs — fair use for training) |
| GitHub issues | Filtered + curated | ~3,000 pairs | Real-world resource problems from kubernetes/kubernetes, helm charts, operator repos | Varies by repo — check each repo's licence |
| Stack Overflow | Filtered for quality | ~2,000 pairs | Community Q&A on K8s resource configuration, OOMKill debugging, CPU throttling | CC BY-SA 4.0 — attribution required |
| VPA recommender + Goldilocks + Kubecost | Logic extraction | ~800 pairs | How existing tools calculate recommendations, translated to natural language explanations | Apache 2.0 (VPA, Goldilocks), check Kubecost |
| Expert operational knowledge | Hand-written | ~1,000 pairs | Runtime-specific guidance (JVM, Go, Python, Node.js), edge cases, anti-patterns | Original — proprietary |
| Synthetic generation | Programmatic gap-fill | ~1,500 pairs | Rare edge cases, unusual metric patterns, cross-cutting scenarios not covered by real data | Original — proprietary |

### Source Details

#### K8s Official Docs + Provider Docs (~3,500 pairs)

Target sections from kubernetes.io:
- Resource Management for Pods and Containers
- Quality of Service for Pods (Guaranteed, Burstable, BestEffort)
- Vertical Pod Autoscaler
- Horizontal Pod Autoscaler
- LimitRange and ResourceQuota
- Node-pressure eviction
- Pod overhead and init containers

Provider-specific:
- GKE: Autopilot resource recommendations, cost optimisation guides
- EKS: Right Sizing Recommendations, Compute Optimizer integration
- AKS: Resource recommendations, cluster advisor

Each doc section is transformed into 3-5 instruction pairs covering:
1. "What does this feature do?" (conceptual)
2. "Given these metrics, how would this feature apply?" (applied)
3. "What are the gotchas?" (operational)

#### GitHub Issues (~3,000 pairs)

Repositories:
- `kubernetes/kubernetes` — core resource issues
- `kubernetes/autoscaler` — VPA/HPA issues
- Popular operator repos with resource-related issues

Filters:
- Labels containing: `resource`, `oom`, `memory`, `cpu`, `right-size`, `limit`, `request`, `throttl`
- Closed issues with resolution (we want problems with solutions)
- Minimum 2 comments (indicates community engagement)

Each issue is transformed into an instruction pair where the user message describes the symptoms and the assistant explains the root cause and fix.

#### Stack Overflow (~2,000 pairs)

Query tags: `kubernetes` AND (`memory` OR `cpu` OR `resources` OR `oom` OR `limits` OR `requests`)

Filters:
- Score > 5
- Has accepted answer
- Answer score > 3

Quality checks:
- Remove answers that are outdated (pre-K8s 1.20 if API has changed)
- Remove answers that recommend deprecated approaches
- Prioritise answers with concrete metric examples

#### VPA/Goldilocks/Kubecost Logic (~800 pairs)

Extract the recommendation algorithms from source code and translate into natural language:
- VPA recommender: how it computes target, lower bound, upper bound
- Goldilocks: how it maps VPA output to recommendations
- Kubecost: allocation model and efficiency scoring

Each algorithm step becomes an instruction pair explaining the logic and when it applies or fails.

#### Expert Knowledge (~1,000 pairs)

Hand-written by engineers with production K8s experience. Covers knowledge that doesn't appear in docs or Q&A:

- **Runtime-specific**: JVM GC heap behaviour, Go memory model, Python GC fragmentation, Node.js event loop patterns
- **Anti-patterns**: Init containers skewing metrics, sidecar resources being analysed as primary workload, CrashLoopBackOff producing misleading spikes
- **Workload-type nuance**: DaemonSet vs Deployment sizing, StatefulSet conservative requirements, Job/CronJob burst patterns
- **Operational wisdom**: Why high memory isn't always waste (JVM -Xmx), why low CPU doesn't mean idle (event-driven), when to ignore recommendations

See `ml/dataset/examples/` for seed examples.

#### Synthetic Generation (~1,500 pairs)

Generated programmatically from the rules engine to fill coverage gaps:
- Unusual metric combinations not seen in real data
- Edge cases: zero usage, extreme spikes, very short data windows
- Cross-pattern scenarios: workload transitioning between patterns
- Multi-container pod scenarios

Each synthetic pair passes the same safety invariants as the rules engine:
- `recommended >= P95 * headroom`
- `memory_recommended >= P99 * 1.10`
- `P50 <= P95 <= P99 <= Max`

Synthetic pairs are labelled as `source: "synthetic"` and capped at 15% of total dataset.

---

## Instruction-Tuning JSONL Schema

Each training example is a JSON object on a single line:

```json
{
  "id": "expert-jvm-gc-001",
  "source": "expert",
  "system": "You are k8s-sage, a Kubernetes resource efficiency specialist. Analyse the provided workload metrics and give actionable right-sizing recommendations. Be specific about numbers, explain your reasoning, and flag risks.",
  "user": "Workload: deployment/payment-service in namespace production\nReplicas: 2\nContainer: app (eclipse-temurin:17-jre)\nCPU Request: 2000m, Limit: 4000m\nMemory Request: 4Gi, Limit: 4Gi\nCPU Usage (7d): P50=180m, P95=420m, P99=680m, Max=1100m\nMemory Usage (7d): P50=2.8Gi, P95=3.1Gi, P99=3.4Gi, Max=3.6Gi\nMemory Working Set (7d): P50=2.6Gi, P95=2.9Gi, P99=3.2Gi, Max=3.4Gi\nPattern: Steady\nRestarts (7d): 0\nQoS: Burstable\nJVM flags: -Xmx3g -Xms1g -XX:+UseG1GC",
  "assistant": "## Analysis\n\n**CPU**: Requesting 2000m but P95 is 420m — 79% waste. The steady pattern with occasional spikes to 1100m suggests this handles synchronous request processing with periodic batch operations.\n\n**Memory**: This is a JVM workload with -Xmx3g. The high memory usage (P50=2.8Gi) is expected — the JVM reserves heap up to the -Xmx value and the G1GC collector will use most of it. This is NOT waste. The working set closely tracks total usage, confirming heap is actively used.\n\n## Recommendations\n\n**CPU**: Reduce request to 510m (P95 + 20% headroom). Keep limit at 1500m to handle the P99 spikes safely.\n- Savings per replica: 1490m\n- Risk: LOW (limit well above P99)\n\n**Memory**: Do NOT reduce memory request. The 4Gi request matches the JVM's -Xmx3g + ~1Gi for metaspace, thread stacks, and native memory. Reducing below 3.5Gi risks OOMKill during full GC cycles when both old and young generation are in memory.\n- Recommendation: Keep at 4Gi\n- Risk of reducing: HIGH (JVM OOMKill)\n\n## Key Insight\n\nFor JVM workloads, always check -Xmx before recommending memory reductions. The difference between memory usage and -Xmx is not waste — it's GC headroom the JVM needs to function correctly.",
  "metadata": {
    "category": "runtime-specific",
    "runtime": "jvm",
    "pattern": "steady",
    "provenance": "Expert knowledge: JVM GC heap reservation behaviour with G1GC"
  }
}
```

### Field Definitions

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique identifier: `{source}-{category}-{sequence}` |
| `source` | enum | yes | One of: `k8s-docs`, `github`, `stackoverflow`, `vpa-source`, `expert`, `synthetic` |
| `system` | string | yes | System prompt — consistent across all pairs |
| `user` | string | yes | Metrics context + question/scenario |
| `assistant` | string | yes | Analysis + recommendation (min 50 chars) |
| `metadata.category` | enum | yes | `right-sizing`, `classification`, `runtime-specific`, `edge-case` |
| `metadata.runtime` | enum | no | `jvm`, `go`, `python`, `node`, `generic` |
| `metadata.pattern` | enum | no | `steady`, `burstable`, `batch`, `idle` |
| `metadata.provenance` | string | yes | URL or description of the source material |
| `metadata.needs_review` | bool | no | `true` if automated extraction confidence is low |

---

## Data Quality Criteria

### Metric Plausibility

Every training pair must have physically plausible metrics:
- CPU usage cannot exceed CPU limit
- Memory working set cannot exceed total memory usage
- `P50 <= P95 <= P99 <= Max` — always
- Restart count is a non-negative integer
- Data window is a positive duration

### Recommendation Safety

Every recommendation in a training pair must satisfy:
- CPU: `recommended_request >= P95 * 1.20` (steady) or `recommended_request >= P50 * 1.20` (burstable/batch)
- Memory: `recommended_request >= P99 * 1.10`
- Never recommend 0 CPU or 0 memory
- CPU floor: 10m, Memory floor: 4Mi
- `recommended_limit >= recommended_request`

### Content Quality

- Assistant responses must be at least 50 characters
- Responses must contain specific numbers, not just qualitative statements
- Responses must explain reasoning, not just state conclusions
- No hallucinated tool versions, kernel parameters, or runtime internals
- Runtime-specific claims must be grounded in documented behaviour

### Deduplication Strategy

1. **Exact dedup**: Remove pairs where user+assistant text is identical
2. **Semantic dedup**: Cluster pairs by embedding similarity (sentence-transformers). Within each cluster, keep the highest-quality pair (longest assistant response with concrete numbers)
3. **Source dedup**: If the same K8s doc section produces multiple overlapping pairs, keep the most operationally useful one
4. **Cross-source dedup**: If a GitHub issue and SO question cover the same scenario, keep the one with more detail

Target: <5% redundancy in final dataset.

---

## Provenance Tracking

Every training pair must be traceable to its source:

| Source Type | Provenance Format |
|-------------|-------------------|
| K8s docs | URL to specific docs page |
| GitHub issues | `github.com/{org}/{repo}/issues/{number}` |
| Stack Overflow | `stackoverflow.com/questions/{id}` |
| VPA source | File path + function in kubernetes/autoscaler repo |
| Expert | Description of the operational scenario |
| Synthetic | Rules engine config that generated it |

Provenance is stored in `metadata.provenance` and is never stripped during processing.

---

## Pipeline Overview

```
collect_k8s_docs.py  ──┐
collect_gh_issues.py ──┤
collect_so.py        ──┼──► format_instruct.py ──► training_data.jsonl
generate_synthetic.py──┤     (validate + dedup)
examples/*.jsonl     ──┘
```

Each collection script outputs intermediate JSONL matching the schema above. `format_instruct.py` validates all pairs against the schema, runs deduplication, and produces the final training dataset.

---

## Dataset Versioning

Datasets are versioned with date stamps: `training_data_v{YYYY-MM-DD}.jsonl`

Each version includes a manifest:
```json
{
  "version": "2026-03-15",
  "total_pairs": 12000,
  "sources": {
    "k8s-docs": 3500,
    "github": 3000,
    "stackoverflow": 2000,
    "vpa-source": 800,
    "expert": 1000,
    "synthetic": 1500
  },
  "quality_metrics": {
    "mean_assistant_length": 450,
    "dedup_rate": 0.03,
    "review_flagged": 120
  }
}
```

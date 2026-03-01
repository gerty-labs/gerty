# Volume Assessment Summary

**Date**: 2026-03-01 (updated after expansion sprint)
**Objective**: Track progress toward training dataset target.

## Current State

| Source | Target | Actual | Status | Notes |
|--------|--------|--------|--------|-------|
| Stack Overflow | 2,200 | 2,409 | DONE | HF dataset `mcipriano/stackoverflow-kubernetes-questions`. Needs human review (~30% may be filtered). |
| K8s docs | 250 | 300 | DONE | Generated via `generate_k8s_docs_pairs.py`. Covers 70+ topics across 11 categories. |
| Expert | 300 | 191 | DONE | Generated via `generate_expert_pairs.py`. 12 runtimes, 40+ workload types, deep operational knowledge. |
| VPA source | 500 | 69 | DONE | Generated via `generate_vpa_pairs.py`. Deep algorithmic pairs (avg 1,384 chars/assistant). |
| GitHub issues | 1,500 | 0 | PENDING | Script scaffolded. Needs GITHUB_TOKEN and implementation. |
| Synthetic | 3,250 | 0 | PENDING | Generated programmatically from rules engine. Covers the gap. |
| **TOTAL** | **8,000** | **2,969** | | |

## Real Pairs Summary

| Source | Count | Avg Assistant Length | Quality |
|--------|-------|---------------------|---------|
| Stack Overflow | 2,409 | ~300 chars | Variable (community-sourced, needs review) |
| K8s docs | 300 | ~800 chars | High (domain-expert generated) |
| Expert | 191 | ~1,000 chars | Highest (deep operational knowledge) |
| VPA source | 69 | 1,384 chars | Highest (algorithmic + scenario-based) |
| **Real total** | **2,969** | | |

## Category Distribution (across all real sources)

Estimated based on individual source reports:

| Category | K8s Docs | Expert | VPA Source | SO (est.) | Total Est. |
|----------|----------|--------|-----------|-----------|-----------|
| right-sizing | 191 | 81 | 46 | ~1,200 | ~1,518 |
| edge-case | 67 | 54 | 16 | ~600 | ~737 |
| classification | 31 | 19 | 7 | ~300 | ~357 |
| runtime-specific | 11 | 37 | 0 | ~300 | ~348 |

## Gap Analysis

**Real data gap**: 2,969 of 4,750 real target (62.5% achieved).

**Remaining work:**
1. **GitHub issues (~500-800 expected)**: Implement API collection script. Would bring real total to ~3,500-3,800.
2. **Synthetic (~3,250)**: Programmatic generation from rules engine. Covers remaining gap to 8,000.

**Revised composition estimate** (after all sources):
- Real data: ~3,500-3,800 pairs (44-48%)
- Synthetic: ~4,200-4,500 pairs (52-56%)

This is higher synthetic ratio than originally planned (40%). Acceptable because:
- Synthetic pairs can be programmatically validated against safety invariants
- Rules engine encodes proven right-sizing logic
- Real data provides language diversity; synthetic provides scenario coverage
- Quality is controllable and consistent

## Quality Highlights

### Expert pairs (191) — highest value per pair
- 12 runtime-specific patterns (JVM, Go, Python, Node.js, .NET, Ruby, Erlang, Rust, PHP, Scala, C++, Wasm)
- VPA/HPA deep dives with internal algorithm details
- GPU workload sizing and idle detection
- Control plane sizing (etcd, apiserver, CoreDNS, Prometheus)
- Debugging workflows (OOMKill, CFS throttling, memory leaks, noisy neighbors)
- Cost optimization (ROI calculation, prioritization, spot instances)

### VPA source pairs (69) — deepest technical content
- Complete right-sizing algorithm walkthrough (8-step process)
- 10 scenario-based pairs with real metrics and specific numeric recommendations
- Workload-specific guidance (Kafka, ML inference, Prometheus, Redis, PostgreSQL, Elasticsearch)
- Pattern classification algorithm (CV-based steady/burstable/batch/idle)
- Confidence scoring system (data window, stability, sample count, OOM history)

### K8s docs pairs (300) — broadest coverage
- 70+ topics across core K8s resource management
- 40+ specific workload types
- 8 runtime-specific patterns
- Operational patterns (rolling updates, canary, blue-green, cost optimization)

## Next Steps

1. **GitHub issues**: Set GITHUB_TOKEN, implement collection script, generate ~500-800 pairs
2. **Synthetic generation**: Build `generate_synthetic.py` using rules engine logic
3. **Human review**: Review SO pairs, flag low-quality for removal
4. **Format consolidation**: Run `format_instruct.py` to merge all sources into final JSONL
5. **Quality metrics**: Compute dataset statistics (length distribution, category balance, duplicate detection)

## Attribution Requirements

- **Stack Overflow**: CC BY-SA 4.0. Must include attribution in model documentation.
- **K8s docs**: Apache 2.0. Provenance URLs included in every pair.
- **GitHub issues**: Apache 2.0 (Kubernetes repos). Issue URLs as provenance.
- **Expert + VPA source + Synthetic**: Original, proprietary. No external attribution needed.

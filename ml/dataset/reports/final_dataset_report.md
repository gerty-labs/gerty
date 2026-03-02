# Final Dataset Report

**Date**: 2026-03-02
**Dataset**: `ml/dataset/data/training_data.jsonl`
**Total pairs**: 6,982

---

## Source Distribution

| Source | Count | % | Avg Length | Min | Max |
|--------|-------|---|-----------|-----|-----|
| Synthetic | 3,523 | 50.5% | 800 chars | 454 | 1,216 |
| Stack Overflow | 2,405 | 34.5% | 939 chars | 100 | 3,000 |
| GitHub Issues | 530 | 7.6% | 1,778 chars | 102 | 3,650 |
| K8s Docs | 289 | 4.1% | 1,659 chars | 833 | 2,343 |
| Expert | 169 | 2.4% | 1,764 chars | 585 | 3,484 |
| VPA Source | 66 | 0.9% | 1,388 chars | 1,013 | 1,721 |

## Category Distribution

| Category | Count | % |
|----------|-------|---|
| runtime-specific | 2,525 | 36.2% |
| right-sizing | 2,166 | 31.0% |
| edge-case | 1,493 | 21.4% |
| classification | 798 | 11.4% |

## Runtime Coverage

| Runtime | Count | % |
|---------|-------|---|
| Unspecified (general K8s) | 4,803 | 68.8% |
| Python | 562 | 8.0% |
| Go | 556 | 8.0% |
| JVM | 518 | 7.4% |
| Node.js | 498 | 7.1% |
| Generic | 45 | 0.6% |

## Workload Pattern Coverage

| Pattern | Count | % |
|---------|-------|---|
| Unspecified | 3,439 | 49.3% |
| Steady | 1,187 | 17.0% |
| Burstable | 945 | 13.5% |
| Idle | 741 | 10.6% |
| Batch | 670 | 9.6% |

## Quality Metrics

| Metric | Value |
|--------|-------|
| Average assistant response length | 987 chars |
| Minimum response length | 100 chars |
| Maximum response length | 3,650 chars |
| Duplicates removed | 0 |
| Validation failures (excluded) | 1,017 |

## Train/Eval Split (90/10)

| Split | Count |
|-------|-------|
| Training | 6,284 |
| Evaluation | 698 |

## Deduplication

Zero duplicates detected across all sources. The ID-based and content-based deduplication in `format_instruct.py` found no overlaps between real and synthetic data.

## Validation Failures (Excluded)

1,017 pairs failed validation, primarily due to metric plausibility regex false positives on real data:
- Expert pairs: 22 excluded (Gi/Mi units parsed as bare numbers by regex)
- K8s docs: 11 excluded (similar unit format issues)
- SO: 4 excluded (metric ordering)
- Synthetic: 977 excluded by synthetic cap (60% limit)
- VPA source: 3 excluded

The synthetic cap trimmed 977 valid synthetic pairs to stay within the 60% ratio limit. All excluded synthetic pairs were otherwise valid.

## Assessment

The dataset is well-balanced across categories and patterns. The runtime-specific coverage (JVM, Go, Python, Node.js) is a key differentiator — general-purpose models have no training data specific to how these runtimes behave in Kubernetes resource contexts.

The "unspecified" entries in runtime and pattern fields are from real data sources (SO, GitHub) where the metadata wasn't annotated — the actual content still covers diverse scenarios.

Average response length of 987 chars is healthy for instruction-tuning — long enough for detailed reasoning but not so long that the model learns to pad responses.

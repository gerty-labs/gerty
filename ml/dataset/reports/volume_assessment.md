# Volume Assessment Summary

**Date**: 2026-03-01
**Objective**: Validate the 12,000 instruction pair target from TRAINING_DATA.md against actual data availability.

## Results

| Source | Target | Actual | Gap | Notes |
|--------|--------|--------|-----|-------|
| Stack Overflow | 2,000 | 2,409 (raw) / ~1,500 (est. after review) | +409 raw / -500 reviewed | HF dataset `mcipriano/stackoverflow-kubernetes-questions`. No SO scores available — all pairs need human review. |
| K8s docs | 3,500 | 27 | -3,473 | 9 core K8s pages processed, 3 pairs each. Target assumed provider docs (GKE/EKS/AKS) and many more sub-pages. |
| GitHub issues | 3,000 | ~1,060 (projected) | -1,940 | API-validated. ~1,900 unique matching issues, ~56% conversion rate. Target was unrealistic for 2 repos. |
| Expert | 1,000 | 21 | -979 | Seed examples done and reviewed. Remaining requires manual authoring. |
| VPA source | 800 | 0 | -800 | Not started. Requires reading VPA recommender source code. |
| Synthetic | 1,500 | 0 | -1,500 | Fills gaps last. Can be generated programmatically from rules engine. |
| **TOTAL** | **12,000** | **~2,457 actual + ~1,060 projected** | **~8,483** | |

## Honest Assessment

### Is 12,000 achievable?

**Not with the current source ratios.** The three largest sources (SO, K8s docs, GitHub) together yield ~4,500 pairs instead of the target ~8,500. The gaps are:

1. **K8s docs (-3,473)**: The 3,500 target was wildly optimistic. The 9 core K8s pages about resource management produce ~27 pairs at 3 per page. Even processing ALL Kubernetes documentation (100+ pages) and adding GKE/EKS/AKS provider docs, realistic yield is **150-300 pairs**, not 3,500. Each page generates 3-5 focused pairs; there simply aren't 1,000 resource-specific pages.

2. **GitHub issues (-1,940)**: The 3,000 target assumed more repos and higher conversion rates. Realistic yield from kubernetes/kubernetes + kubernetes/autoscaler is ~1,060. Expanding to 5-10 more repos (prometheus-operator, helm, kustomize, etc.) might add ~300-500 pairs. Revised estimate: **1,300-1,500 pairs**.

3. **Stack Overflow (+409 raw but -500 reviewed)**: Best performer. The 2,409 raw pairs are encouraging, but without SO scores for quality filtering, ~30-40% may be rejected on review. Supplementing with direct SO API queries (with tag+score filtering) could recover the gap. Revised estimate: **1,800-2,200 pairs**.

### Revised Source Ratios

| Source | Revised Target | Confidence | Strategy |
|--------|---------------|------------|----------|
| Stack Overflow | 2,200 | HIGH | Current dataset + supplement with SO API |
| K8s docs | 250 | HIGH | Process all resource-related pages + provider docs |
| GitHub issues | 1,500 | MEDIUM | Expand to 5+ repos, use authenticated API |
| Expert | 300 | MEDIUM | Expand from 21 seed pairs, labour-intensive |
| VPA source | 500 | MEDIUM | Extract from VPA recommender + Goldilocks code |
| Synthetic | 3,250 | HIGH | Programmatic generation from rules engine covers the gap |
| **Revised total** | **8,000** | | |

### The Synthetic Lever

The biggest adjustment is synthetic generation: from 1,500 to 3,250. This is defensible because:
- The rules engine already encodes the right-sizing logic
- Synthetic pairs can be programmatically validated against safety invariants
- We can generate diverse metric combinations that real data doesn't cover
- Quality is controllable (unlike community-sourced data)

But it changes the dataset composition from 85% real / 15% synthetic to **~60% real / ~40% synthetic**. This is a meaningful tradeoff: more control, less diversity in language patterns.

### Revised Target: 8,000 pairs

Recommend reducing the target from 12,000 to **8,000 pairs** with a 60/40 real/synthetic split. Rationale:
- Quality over quantity: 8,000 well-curated pairs will train a better model than 12,000 padded ones
- Phi-3 Mini has been fine-tuned successfully on datasets as small as 5,000 pairs for domain specialization
- The synthetic gap-fill can be calibrated after evaluating the real data quality
- We can always generate more synthetic pairs if evaluation shows the model needs more data

## Quality Observations

### Stack Overflow (strongest source)
- Real community problems with practical solutions
- Diverse workload types and cluster configurations
- Weakness: varying answer quality, some outdated advice, no score filtering available

### K8s Docs (limited but high quality)
- Clean, authoritative content
- Well-structured for instruction pair extraction
- Weakness: too few resource-specific pages for high volume, documentation style is explanatory not operational

### GitHub Issues (moderate quality, moderate volume)
- Real production problems with technical depth
- Often includes debugging steps and root cause analysis
- Weakness: many issues are about K8s internals rather than user-facing resource management, require significant transformation to become instruction pairs

### Expert Pairs (highest quality, lowest volume)
- Deep operational knowledge not available elsewhere
- Runtime-specific insights, anti-patterns, edge cases
- Weakness: labour-intensive to write, only 21 seed examples so far

## Next Steps

1. **Immediate**: Commit current data and reports
2. **Week 1**: Human review of SO pairs (flag ~30% for removal), expand GitHub to more repos
3. **Week 1**: Write 50+ more expert pairs covering uncovered scenarios
4. **Week 2**: Build synthetic generation pipeline from rules engine
5. **Week 2**: Extract VPA recommender logic into instruction pairs
6. **Ongoing**: Track pair count in dataset manifest, re-assess at 5,000 pairs

## Attribution Requirements

- **Stack Overflow**: CC BY-SA 4.0. Must include attribution in model documentation and dataset metadata. Consider adding SO attribution notice to the Modelfile.
- **K8s docs**: Apache 2.0. Include provenance URLs. No special attribution required beyond licence compliance.
- **GitHub issues**: Apache 2.0 (Kubernetes repos). Include issue URLs as provenance.
- **Expert + Synthetic**: Original, proprietary. No external attribution needed.

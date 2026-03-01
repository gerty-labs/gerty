# GitHub Issues Instruction Pairs -- Collection Report

**Generated**: 2026-03-01
**Output file**: `ml/dataset/raw/github_issues_pairs.jsonl`
**Collection script**: `ml/dataset/collect_gh_issues.py`

## Summary Statistics

| Metric | Count |
|--------|-------|
| **Total pairs** | **530** |
| Collection method | GitHub Search API + issue detail + comment extraction |
| API calls used | 1,210 (of 5,000/hr limit) |
| Average assistant length | 1,777 characters |
| Unique IDs | 530 (zero duplicates) |

## Repository Breakdown

| Repository | Pairs | Notes |
|-----------|-------|-------|
| kubernetes/kubernetes | 246 | Core K8s issues: OOMKill, throttling, resource management |
| kubernetes/autoscaler | 198 | VPA/HPA/CA issues: recommendations, scaling behavior |
| kubernetes-sigs/descheduler | 50 | Pod eviction, resource rebalancing |
| FairwindsOps/goldilocks | 36 | VPA dashboard, resource recommendations |
| **Total** | **530** | |

## Category Breakdown

| Category | Count | Percentage |
|----------|-------|------------|
| edge-case | 238 | 44.9% |
| right-sizing | 157 | 29.6% |
| runtime-specific | 108 | 20.4% |
| classification | 27 | 5.1% |

### Per-Repository Category Distribution

| Repository | right-sizing | edge-case | runtime-specific | classification |
|-----------|-------------|-----------|-----------------|---------------|
| kubernetes/kubernetes | 56 | 120 | 58 | 12 |
| kubernetes/autoscaler | 70 | 78 | 39 | 11 |
| kubernetes-sigs/descheduler | 10 | 36 | 3 | 1 |
| FairwindsOps/goldilocks | 21 | 4 | 8 | 3 |

## Assistant Response Length Distribution

| Range | Count | Percentage |
|-------|-------|------------|
| 0-200 chars | 6 | 1.1% |
| 200-500 chars | 19 | 3.6% |
| 500-1,000 chars | 74 | 14.0% |
| 1,000-2,000 chars | 222 | 41.9% |
| 2,000-5,000 chars | 209 | 39.4% |

- **Min**: 102 chars
- **P25**: 1,206 chars
- **Median**: 1,741 chars
- **P75**: 2,351 chars
- **Max**: 3,650 chars

## Collection Pipeline

1. **Search**: 15 queries per repo across 4 repositories (60 total search API calls)
2. **Fetch**: Full issue body + top 20 comments for each unique issue
3. **Transform**: Issue body → user prompt, best resolution comment → assistant response
4. **Quality filter**: Minimum 3 comments, resolution ≥100 chars, ≥2 resource keywords
5. **Category assignment**: Keyword-based (OOM → edge-case, throttle → right-sizing, VPA → right-sizing, quota → classification)
6. **Deduplication**: By issue ID (cross-query) and content hash (cross-issue)

### Search Queries

```
OOMKill, OOMKilled, cpu throttling, resource requests limits,
right-sizing resources, over-provisioned, under-provisioned,
memory leak pod, VPA recommendation, vertical pod autoscaler,
CPU limit throttle, resource quota exceeded, LimitRange,
container memory, HPA scaling
```

## Data Quality Notes

- All `id` fields match pattern `^github-[a-zA-Z]+-[a-zA-Z]+-\d+$`
- All `source` fields are `"github"`
- All `system` fields use the canonical k8s-sage system prompt
- All `assistant` fields exceed 50 characters (average 1,777 chars)
- All `metadata.category` fields are valid categories
- All pairs include GitHub issue URL as provenance
- 530 unique IDs, zero duplicates
- All pairs marked `needs_review: true` for human QA

## Licensing

GitHub issues are public user-contributed content under each repository's license:
- kubernetes/kubernetes: Apache 2.0
- kubernetes/autoscaler: Apache 2.0
- kubernetes-sigs/descheduler: Apache 2.0
- FairwindsOps/goldilocks: Apache 2.0

Transformation into instruction pairs includes the original issue URL as provenance.

# Stack Overflow Dataset Report

## Source

- **Dataset**: `mcipriano/stackoverflow-kubernetes-questions` (HuggingFace)
- **Licence**: CC BY-SA 4.0 — attribution required for all derived training pairs
- **Schema**: 4 columns: Question (HTML), QuestionAuthor, Answer (HTML), AnswerAuthor
- **Limitation**: No question scores, tags, accepted-answer flags, or SO question IDs available in this dataset. Provenance tracks the dataset row index only.

## Volume Pipeline

| Stage | Count | Notes |
|-------|-------|-------|
| Total questions in dataset | 30,044 | All K8s-tagged SO questions |
| Keyword-matched (≥2 hits) | 8,509 | Broad keyword filter |
| Strict relevance filter | 2,511 | Requires strong resource keyword + K8s context |
| After exact dedup | 2,409 | MD5 hash on question text |
| **Final output** | **2,409** | Written to `stackoverflow_filtered.jsonl` |

### Filter Details

**Broad filter** (≥2 keyword hits from): memory, cpu, oom, OOMKill, limit, request, resources, right-siz*, throttl*, over-provision*, under-provision*, resource quota, limit range, vertical pod autoscaler, VPA, resource efficiency, millicores, milli, Gi, Mi, memory leak, cpu throttl*, QoS, HPA, Burstable, Guaranteed, BestEffort, eviction, node-pressure

**Strict filter** (requires at least one strong signal): memory limit, cpu limit, memory request, cpu request, OOMKill, resource quota, limit range, resource management/efficiency, cpu/memory usage/allocation, right-sizing, throttling, over/under-provision, VPA, QoS class, millicores, resources.limits/requests

**Rejections**: 5,978 had no strong resource keyword (mostly generic K8s questions mentioning "resources" in a non-resource-management context), 20 had no K8s context.

## Category Breakdown

| Category | Count | Percentage |
|----------|-------|------------|
| right-sizing | 945 | 39.2% |
| edge-case | 547 | 22.7% |
| runtime-specific | 543 | 22.5% |
| classification | 374 | 15.5% |

## Quality Assessment

### Strengths
- Real community Q&A with practical problems and solutions
- Good coverage of OOMKill debugging, resource limit configuration, QoS classes
- Includes VPA/HPA questions with real-world operational answers
- Diverse workload types and cluster configurations

### Weaknesses
- **No score/quality filtering**: This dataset lacks SO scores, so we can't filter for high-quality accepted answers (TRAINING_DATA.md specifies score > 5, accepted answer, answer score > 3). All pairs have `needs_review: true`.
- **HTML artifacts**: Questions and answers are stripped from HTML but may contain formatting artifacts
- **Varying quality**: Some answers are brief or tangential. Human review needed to cull low-quality pairs.
- **No SO question IDs**: Provenance can only reference the HF dataset row index, not the original SO URL. This makes CC BY-SA 4.0 attribution harder.
- **Potential staleness**: Some answers may reference deprecated K8s APIs (pre-1.20)

### Estimated Usable After Review
With human review, estimate **60-70% pass rate** → **~1,450-1,690 usable pairs**. The 2,000 target may require supplementing with direct SO API queries using proper score/tag filtering.

## Sample Pairs (5 for Quality Review)

### Sample 1: `stackoverflow-classification-0076`
**Category**: classification
**Q**: In grafana dashboard, I see the memory request(2GB) and limit(4GB) lines. The current base which I think is the current usage consumption looks steady...
**A**: Its page cache. Under Linux, the Page Cache accelerates many accesses to files on non volatile storage...
**Assessment**: Good — explains page cache vs actual memory usage, relevant to right-sizing decisions.

### Sample 2: `stackoverflow-runtime-specific-0022`
**Category**: runtime-specific
**Q**: What is the correct way of memory handling in OpenShift/Kubernetes? If I create a project in OKD, how can I determine optimal memory usage of pods?
**A**: Is it able to give extra memory but only for the first X minutes for each pod start? You do get this behavior when you set the limit to a higher value than the request. This allows pods to burst...
**Assessment**: Good — explains request vs limit burst behaviour, directly applicable to resource configuration.

### Sample 3: `stackoverflow-runtime-specific-0271`
**Category**: runtime-specific
**Q**: With my team, we're currently building an API using FastAPI and we're really struggling to get good performances out of it once deployed to Kubernetes...
**A**: It finally appeared that our performance issues were caused by the non-usage of gunicorn...
**Assessment**: Moderate — runtime-specific (Python ASGI), but the K8s resource angle is tangential. May need reframing.

### Sample 4: `stackoverflow-right-sizing-0367`
**Category**: right-sizing
**Q**: There are already a million questions and answers stating you cannot run a VPA+HPA at the same time...
**A**: Well I just took the plunge and deployed it. So far I'm not having any issues. So it appears to be safe to deploy a VPA+HPA both watching CPU and Memory but have the VPA set with updateMode: "Off"...
**Assessment**: Good — practical VPA+HPA coexistence advice, directly relevant to recommendation strategy.

### Sample 5: `stackoverflow-edge-case-0213`
**Category**: edge-case
**Q**: I'm using microk8s on Ubuntu but I have a problem with the coredns pod which fails to start...
**A**: Resolved! Within the config map there should be a carriage return before the word 'reload'...
**Assessment**: Poor — this is a CoreDNS config issue, not a resource efficiency problem. False positive from keyword filter. Should be removed in review.

## Conclusion

The SO dataset yields **2,409 pairs** against a 2,000 target, which is encouraging. However:
- Quality filtering will likely reduce this to ~1,500-1,700 usable pairs
- The lack of SO scores means we can't pre-filter for quality automatically
- All pairs are flagged `needs_review: true`
- To reach the 2,000 target reliably, supplement with direct SO API queries using proper tag+score filtering

**Honest assessment**: We'll likely land at **~1,500 high-quality pairs** from this source after human review. The 2,000 target is achievable but requires either (a) aggressive review accepting 80%+ of pairs or (b) supplementing with ~300-500 pairs from direct SO API with score filtering.

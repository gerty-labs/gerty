# K8s Documentation Instruction Pairs -- Generation Report

**Generated**: 2026-03-01 (updated)
**Output file**: `ml/dataset/raw/k8s_docs_pairs.jsonl`
**Generation script**: `ml/dataset/generate_k8s_docs_pairs.py`
**Licence**: Kubernetes documentation is licenced under Apache 2.0. Provenance URLs are included in every pair's metadata.

## Summary Statistics

| Metric | Count |
|--------|-------|
| **Total pairs** | **300** |
| Generation method | Python script with domain-expert knowledge |
| Pair types | Conceptual, Applied (with metrics), Operational (gotchas) |

## Category Breakdown

| Category | Count | Percentage |
|----------|-------|------------|
| right-sizing | 191 | 63.7% |
| edge-case | 67 | 22.3% |
| classification | 31 | 10.3% |
| runtime-specific | 11 | 3.7% |

## Topic Coverage

### Core Kubernetes Resource Management
- Requests/limits, resource units, QoS classes, pod overhead
- Init container resource model, sidecar resource attribution
- Ephemeral storage management

### Scheduling & Placement
- Scheduler bin-packing with requests, node allocatable
- Topology spread constraints, node affinity interactions
- Pod priority and preemption resource implications

### Autoscaling
- HPA (CPU, custom metrics, behaviour tuning, interaction with right-sizing)
- VPA (modes, policies, limitations, troubleshooting)
- KEDA, cluster autoscaler, Karpenter interactions
- In-place pod vertical scaling (KEP-1287)

### Eviction & Disruption
- Node-pressure eviction order by QoS class
- PodDisruptionBudget interaction with right-sizing
- Termination grace period resource implications

### Policy
- ResourceQuota (per-namespace limits, quota exhaustion diagnosis)
- LimitRange (defaults, min/max, interaction with right-sizing)
- Admission webhooks that modify resources

### Monitoring & Observability
- container_memory_usage_bytes vs working_set_bytes vs RSS
- CFS throttling metrics and interpretation
- Prometheus recording rules for right-sizing
- Building efficiency scores

### Workload-Specific Right-Sizing (40+ specific workloads)
- Databases: PostgreSQL, MongoDB, Elasticsearch, ClickHouse, Redis
- Messaging: Kafka, RabbitMQ, NATS
- ML/AI: inference servers, GPU workloads
- Infrastructure: CoreDNS, nginx ingress, cert-manager, Vault, Prometheus, Grafana
- CI/CD: Argo CD, Temporal workers
- Web: WordPress/PHP-FPM, Node.js, static sites
- Security: Falco, Keycloak
- Observability: Jaeger, Vector, node_exporter
- Storage: MinIO, Harbor

### Runtime-Specific Patterns
- JVM: GC behaviour, heap sizing, container awareness
- Go: GOGC, GOMEMLIMIT, goroutine memory, invisible CPU spikes
- Python: memory never released, multiprocessing, ML inference
- Node.js: event loop blocking, V8 heap, libuv threads
- .NET: GC heap management, Server vs Workstation GC
- PHP-FPM: worker-based memory model
- Erlang/BEAM: scheduler pinning, binary leaks, atom table
- Rust: deterministic memory, minimal runtime overhead

### Operational Patterns
- Rolling update resource overhead
- Canary deployment capacity planning
- Cold start / startup resource spikes
- Right-sizing across dev/staging/prod environments
- VM-to-Kubernetes migration sizing
- Air-gapped and edge cluster right-sizing
- Spot vs on-demand node pool strategies
- FinOps integration and ROI calculation

### Edge Cases & Troubleshooting
- OOMKill despite generous limits (kernel memory, tmpfs, metric gaps)
- Cluster autoscaler flapping after right-sizing
- VPA not applying recommendations
- Pending pods with "Insufficient cpu" despite available resources
- Memory leaks vs legitimate usage classification
- Shared memory (/dev/shm) right-sizing
- Network policy CPU overhead
- Seccomp profile resource impact
- CronJob overlap resource planning
- Kubernetes version upgrade impact on resource usage

## Data Quality Notes

- All `id` fields match pattern `^[a-z0-9-]+$` in format `k8s-docs-{category}-{NNN}`
- All `source` fields are `"k8s-docs"`
- All `system` fields use the canonical k8s-sage system prompt
- All `assistant` fields exceed 50 characters (most exceed 500 characters)
- All `metadata.needs_review` fields are `true`
- All `metadata.provenance` fields are valid URLs to K8s docs or related sources
- All metric scenarios use realistic values where P50 <= P95 <= P99 <= Max
- All CPU/memory recommendations include specific numbers with reasoning

## Licence and Provenance

The Kubernetes documentation is licenced under the [Apache License 2.0](https://github.com/kubernetes/website/blob/main/LICENSE). Content was generated from domain knowledge informed by official K8s documentation. Each pair includes a `metadata.provenance` URL. All pairs are marked `needs_review: true` for human review before use in training.

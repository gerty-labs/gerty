# k8s-sage — Architecture

## Problem Statement

Kubernetes clusters are chronically over-provisioned. Engineering teams set resource requests high "just in case" and never revisit them. Studies consistently show 50-70% of allocated CPU and memory goes unused. This waste costs money and consumes energy unnecessarily.

Existing solutions fall into two camps:

1. **Rules-based tools** (VPA, Kubecost, PerfectScale) — provide formulaic recommendations but lack context. They can't distinguish a workload that's idle because it's a cron job from one that's genuinely over-provisioned.

2. **LLM-augmented tools** (K8sGPT) — use general-purpose models for troubleshooting, not efficiency. They call out to external APIs (OpenAI, Cohere) or run large local models. None have a model that actually understands K8s resource patterns.

**k8s-sage fills the gap**: a purpose-built small language model, fine-tuned on Kubernetes operational data, that runs locally inside your cluster with minimal overhead and provides intelligent right-sizing recommendations.

---

## Design Principles

1. **The tool must cost less than the waste it identifies.** If the agent uses 50MB per node and identifies 2GB of waste, that's a 40:1 return. If it uses 2GB itself, it's useless.

2. **Work without AI, be better with AI.** The rules engine is the MVP. The SLM is a force multiplier. No cluster should require a model to get value from k8s-sage.

3. **Data never leaves the cluster by default.** No external API calls. The model runs locally. Metrics stay in-cluster.

4. **Opinionated defaults, configurable everything.** Sensible out-of-the-box behaviour, but every threshold and interval is tuneable.

---

## Components

### 1. sage-agent (DaemonSet)

A lightweight Go binary that runs on every node. Its only job is to collect resource metrics and make them available.

#### Resource Budget

| Resource | Request | Limit |
|----------|---------|-------|
| CPU | 50m | 100m |
| Memory | 50Mi | 100Mi |

These are hard constraints. Any code change that risks exceeding them must be flagged.

#### What It Collects

The agent scrapes the kubelet Summary API (`/stats/summary`) at a configurable interval (default 30s) for usage metrics, and the kubelet `/pods` endpoint for pod spec data (resource requests, limits, QoS class, restart counts).

Per container, per scrape:
- CPU usage (nanocores) — from `/stats/summary`
- Memory usage (bytes) and working set (bytes) — from `/stats/summary`
- CPU request, CPU limit — from `/pods` pod spec
- Memory request, memory limit — from `/pods` pod spec
- Restart count — from `/pods` container status
- Pod QoS class (Guaranteed, Burstable, BestEffort) — from `/pods` pod status

#### In-Memory Store

Metrics are stored in a rolling window with aggressive downsampling to stay within the memory budget:

```
Age 0–24h:    5-minute aggregates (P50, P95, P99, max)
Age 24h–7d:   1-hour aggregates
```

Per-pod storage cost: ~500 data points × 6 metrics × ~40 bytes = ~120KB per pod.
At 50Mi budget with 15Mi for Go runtime/HTTP: supports ~290 pods per node.

For dense nodes (>300 pods), reduce retention or increase memory limit via Helm values.

#### Outputs

- **Pull**: `/report` endpoint (port 9101) returns JSON waste analysis per pod on this node
- **Push**: The Pusher component (`internal/agent/pusher.go`) POSTs aggregated `NodeReport` JSON to the server's `POST /api/v1/ingest` endpoint every push interval (default 5 minutes, configurable via `PUSH_INTERVAL` env var). The server URL is set via `SERVER_URL` env var (default `http://k8s-sage-server:8080`).
- **Prometheus**: `/metrics` endpoint exposes standard Prometheus metrics for integration with existing monitoring

#### RBAC

The agent needs a minimal ClusterRole:
```yaml
rules:
  - apiGroups: [""]
    resources: ["pods", "nodes"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["nodes/stats", "nodes/proxy"]
    verbs: ["get"]
  - apiGroups: ["metrics.k8s.io"]
    resources: ["pods", "nodes"]
    verbs: ["get", "list"]
```

The `nodes/stats` and `nodes/proxy` resources are required for the agent to access the kubelet `/stats/summary` and `/pods` endpoints via the API server proxy. No write access. No secrets access. Principle of least privilege.

---

### 2. sage-server (Deployment)

Central service that aggregates data from all agents, runs the rules engine, optionally invokes the SLM, and serves the API.

#### Aggregation

The server maintains a cluster-wide view built from agent push reports. Data is keyed by `namespace/pod/container` with per-node provenance.

```go
type ClusterState struct {
    Namespaces map[string]*NamespaceState
    LastUpdate time.Time
}

type NamespaceState struct {
    Pods       map[string]*PodState
    TotalWaste ResourceWaste
}

type PodState struct {
    Containers []ContainerState
    Node       string
    QoSClass   string
    Labels     map[string]string
    OwnerRef   OwnerReference  // Deployment, StatefulSet, DaemonSet, Job, etc.
}

type ContainerState struct {
    Name       string
    CPURequest resource.Quantity
    CPUUsage   TimeSeriesSummary  // P50, P95, P99, max over window
    MemRequest resource.Quantity
    MemUsage   TimeSeriesSummary
    Restarts   int
}
```

#### Owner-Level Aggregation

Recommendations are made at the owner level (Deployment, StatefulSet, etc.), not the pod level. If a Deployment has 10 replicas all showing the same waste pattern, we generate one recommendation for the Deployment, not 10 for individual pods.

```
Pod metrics → Group by ownerReference → Aggregate across replicas → Recommend
```

---

### 3. Rules Engine

Deterministic logic that classifies workloads and generates right-sizing recommendations without any AI.

#### Workload Classification

The rules engine classifies each workload into one of four patterns based on CPU/memory time series:

| Pattern | Characteristics | Right-sizing Strategy |
|---------|----------------|----------------------|
| **Steady** | Low variance, consistent usage | Set request to P95 + 20% headroom |
| **Burstable** | Periodic spikes, low baseline | Set request to P50 + 20%, limit to P99 + 20% |
| **Batch** | High usage during execution, idle otherwise | Set request to P50 + 20%, limit to Max + 20% |
| **Idle** | <5% utilisation sustained over 48h+ | Flag for removal or investigation |

Classification algorithm:
```
coefficient_of_variation = stddev(cpu_usage) / mean(cpu_usage)

if mean(cpu_usage) < 0.05 * cpu_request for >48h:
    pattern = IDLE
elif cv < 0.3:
    pattern = STEADY
elif P99/P50 >= 5 AND Max/P50 >= 10:
    pattern = BATCH
else:
    pattern = BURSTABLE
```

#### Recommendation Generation

Each recommendation includes:
```go
type Recommendation struct {
    Target        OwnerReference        // What to change
    Container     string                // Which container
    Resource      string                // "cpu" or "memory"
    CurrentReq    resource.Quantity      // Current request
    CurrentLimit  resource.Quantity      // Current limit
    RecommendedReq   resource.Quantity   // Suggested request
    RecommendedLimit resource.Quantity   // Suggested limit
    Pattern       WorkloadPattern       // Classification
    Confidence    float64               // 0.0–1.0
    Reasoning     string                // Human-readable explanation
    EstSavings    ResourceSavings       // CPU cores and memory freed
    Risk          RiskLevel             // LOW, MEDIUM, HIGH
    DataWindow    time.Duration         // How much data this is based on
}
```

**Confidence scoring**:
- 7+ days of data, steady pattern → 0.9+
- 3–7 days, steady → 0.7–0.9
- <3 days → 0.5 max (flag as low confidence)
- Burstable patterns cap at 0.8 (inherently less predictable)
- Batch workloads cap at 0.7 (need multiple execution cycles)

**Risk levels**:
- LOW: Recommendation reduces request but stays well above P99
- MEDIUM: Recommendation is close to P99, minor risk under unusual load
- HIGH: Workload shows erratic patterns, recommendation may cause OOMKill or throttling

---

### 4. K8s-Sage SLM (The Model)

This is the long-term differentiator. A small language model fine-tuned specifically on Kubernetes operational knowledge.

#### Why a Specialist Model

General-purpose LLMs (GPT-4, Llama, Phi-3) know about Kubernetes from their pretraining data, but they lack:

- **Metric interpretation skills**: They can't look at a CPU time series and recognise a memory leak pattern vs. legitimate growth vs. a cron spike
- **Right-sizing intuition**: They don't know that a JVM workload needs memory headroom for GC, or that a Go service can safely run closer to its P99
- **K8s-specific context**: They don't know that DaemonSets shouldn't be right-sized the same way as Deployments, or that init containers don't need sustained resources

A specialist model trained on this domain knowledge will be smaller, faster, and more accurate than a general model prompted with the same questions.

#### Base Model Selection

| Candidate | Params | Q4 Size | CPU RAM | Rationale |
|-----------|--------|---------|---------|-----------|
| **Phi-3 Mini** | 3.8B | ~2.2GB | ~3GB | Best reasoning for size, ONNX CPU support, MIT licence |
| TinyLlama 1.1B | 1.1B | ~0.7GB | ~1GB | Smallest viable, faster inference, but weaker reasoning |
| Phi-3.5 Mini | 3.8B | ~2.2GB | ~3GB | Improved over Phi-3, if available in GGUF |
| Qwen2.5 3B | 3B | ~1.8GB | ~2.5GB | Strong multilingual, Apache 2.0 |

**Current recommendation**: Start with **Phi-3 Mini 4K Instruct** as the base. It's the best balance of reasoning capability, size, licence (MIT), and CPU inference performance. Fine-tune with LoRA, merge, quantize to Q4 GGUF, serve via Ollama.

If Phi-3 proves too large for customer environments, fall back to TinyLlama with a more focused training dataset.

#### Training Data and Fine-Tuning

The training dataset targets 12,000 instruction pairs (85% real sources, 15% synthetic gap-fillers) and the model is fine-tuned via LoRA on Phi-3 Mini 4K Instruct, then quantised to GGUF Q4_K_M for CPU-only serving via Ollama. Full details are maintained in dedicated documents to avoid drift:

- **[TRAINING_DATA.md](TRAINING_DATA.md)** — Source breakdown, JSONL schema, quality criteria, deduplication strategy, and provenance tracking. The canonical system prompt for all training pairs is defined here.
- **[MODEL_DESIGN.md](MODEL_DESIGN.md)** — Base model rationale, LoRA configuration, post-training pipeline, Ollama Modelfile, serving architecture, and evaluation plan.

#### SLM Integration in the Server

```go
// internal/slm/client.go
type SLMClient interface {
    Analyze(ctx context.Context, req AnalysisRequest) (*AnalysisResponse, error)
    HealthCheck(ctx context.Context) error
}

// The server checks if SLM is available and uses it to enhance
// rules engine output. If SLM is unavailable, rules engine output
// is returned as-is.
func (a *Analyzer) Analyze(ctx context.Context, state *ClusterState) (*Report, error) {
    report := a.rules.Analyze(state)  // Always runs
    
    if a.slm != nil {
        enhanced, err := a.slm.Analyze(ctx, report.ToPrompt())
        if err != nil {
            slog.Warn("SLM unavailable, using rules-only", "error", err)
            return report, nil  // Graceful degradation
        }
        report.EnrichWith(enhanced)
    }
    
    return report, nil
}
```

---

### 5. sage-cli

Command-line interface for interacting with k8s-sage.

```bash
# Cluster-wide efficiency report
sage report

# Namespace drill-down
sage report -n production

# Specific workload
sage report -n production deployment/api-gateway

# With AI-powered explanations (requires SLM)
sage report --explain

# Output as JSON for piping
sage report -o json

# Recommendations only
sage recommend

# Apply a recommendation (generates kubectl patch)
sage recommend apply deployment/api-gateway --dry-run
```

---

## API Design

### Server REST API (port 8080)

```
GET  /healthz                           Health check
GET  /api/v1/report                     Cluster-wide report
GET  /api/v1/report?namespace={ns}      Namespace report
GET  /api/v1/workloads                  All workloads with waste summary
GET  /api/v1/workloads/{ns}/{kind}/{name}  Specific workload detail
GET  /api/v1/recommendations            All recommendations
GET  /api/v1/recommendations?risk=low   Filtered recommendations
POST /api/v1/ingest                     Agent push endpoint (NodeReport JSON)
POST /api/v1/analyze                    Trigger on-demand analysis
POST /api/v1/explain                    SLM-powered explanation (requires model)
```

All responses are JSON. The `Accept: text/plain` header returns human-readable output.

---

## Deployment

### Helm Chart

```bash
helm repo add k8s-sage https://k8s-sage.github.io/charts
helm install k8s-sage k8s-sage/k8s-sage

# With SLM enabled
helm install k8s-sage k8s-sage/k8s-sage --set slm.enabled=true

# Custom resource budgets for large clusters
helm install k8s-sage k8s-sage/k8s-sage \
  --set agent.resources.requests.memory=100Mi \
  --set agent.store.retentionHours=336
```

### Minimal Deployment (no model)

Agent DaemonSet + Server Deployment + CLI. Rules engine only. Total cluster overhead: ~50Mi per node + 256Mi for server.

### Full Deployment (with model)

Above + Ollama Deployment running k8s-sage SLM. Additional overhead: ~3Gi for model serving pod.

---

## Future Roadmap

- **v0.1**: Agent + rules engine + CLI (pre-March 16 target)
- **v0.2**: SLM fine-tuned and integrated via Ollama
- **v0.3**: Training data pipeline for continuous improvement from anonymised cluster data
- **v0.4**: Web dashboard
- **v0.5**: Prometheus/Grafana integration (native dashboards)
- **v1.0**: Stable API, published model on HuggingFace, Helm chart in Artifact Hub

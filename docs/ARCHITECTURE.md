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

2. **Work without AI, be better with AI.** The rules engine (L1) is the MVP. The SLM (L2) is a force multiplier. No cluster should require a model to get value from k8s-sage.

3. **L1 is the safety floor.** The SLM can enhance recommendations but never override safety invariants enforced by the rules engine. If L2 fails, times out, or violates safety, L1 stands.

4. **Data never leaves the cluster by default.** No external API calls. The model runs locally. Metrics stay in-cluster.

5. **Opinionated defaults, configurable everything.** Sensible out-of-the-box behaviour, but every threshold and interval is tuneable.

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

Per-pod storage cost: ~500 data points x 6 metrics x ~40 bytes = ~120KB per pod.
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

No write access. No secrets access. Principle of least privilege.

---

### 2. sage-server (Deployment)

Central service that aggregates data from all agents, runs the L1 rules engine, optionally invokes the L2 SLM, and serves the API.

#### Aggregation

The server maintains a cluster-wide view built from agent push reports. Data is keyed by `namespace/pod/container` with per-node provenance.

#### Owner-Level Aggregation

Recommendations are made at the owner level (Deployment, StatefulSet, etc.), not the pod level. If a Deployment has 10 replicas all showing the same waste pattern, we generate one recommendation for the Deployment, not 10 for individual pods. The highest resource usage across replicas drives the recommendation.

```
Pod metrics -> Group by ownerReference -> Aggregate across replicas -> Recommend
```

---

### 3. Rules Engine (L1)

Deterministic logic that classifies workloads and generates right-sizing recommendations without any AI. This is the safety-critical path — all recommendations must pass L1 invariants regardless of whether L2 is enabled.

#### Workload Classification

The rules engine classifies each workload into one of five patterns:

| Pattern | Characteristics | Right-sizing Strategy |
|---------|----------------|----------------------|
| **Steady** | Low variance, CV < 0.3 | Request = P95 x 1.20 headroom |
| **Burstable** | Periodic spikes, moderate variance | Request = P50 x 1.20, Limit = P99 x 1.20 |
| **Batch** | High amplitude spikes, idle between | Request = P50 x 1.20, Limit = Max x 1.20 |
| **Idle** | <5% utilisation sustained over 48h+ | Flag for removal or investigation |
| **Anomalous** | Monotonic memory growth, no release | Flag for investigation (possible memory leak) |

#### Classification Algorithm

```
# Near-zero guard: ratio math is unreliable when P50 is tiny
IF P50 < 25m CPU AND Max < 100m CPU:
    -> STEADY (low-usage daemon)
IF P50 < 25m CPU AND Max >= 100m CPU:
    -> BURSTABLE (low baseline, real spikes)

# Idle detection
IF mean(cpu_usage) < 0.05 * cpu_request for >48h:
    -> IDLE

# Anomaly detection (memory only)
IF memory usage is monotonically increasing across 4 time segments
   AND growth from segment 1 -> segment 4 > 20%:
    -> ANOMALOUS

# Standard classification
coefficient_of_variation = (P95 - P50) / P50
IF cv < 0.3:
    -> STEADY
ELIF P99/P50 >= 5 AND Max/P50 >= 10:
    -> BATCH
ELSE:
    -> BURSTABLE
```

The near-zero P50 guard was added after dogfooding revealed that low-usage daemons (agents, CNI plugins, HTTP servers with <25m baseline) were being misclassified as batch due to ratio explosion when P50 approaches zero.

#### Safety Invariants

These are enforced on every recommendation, regardless of L1 or L2 origin:

| Invariant | Value | Rationale |
|-----------|-------|-----------|
| CPU floor | 50m | No real container runs below this |
| Memory floor | 64Mi | Minimum viable for any runtime |
| Memory recommendation | >= P99 working set x 1.10 | Prevent OOM kills |
| CPU recommendation | >= P95 x headroom | Prevent throttling |
| No zero recommendations | Always enforced | Zero = eviction risk |

When a recommendation hits the floor, the reasoning text explicitly notes "minimum floor applied".

#### Confidence-Gated Reduction Caps

No single recommendation can reduce resources by an unbounded amount. The maximum reduction per cycle is tied to confidence:

| Confidence | Max Reduction |
|------------|--------------|
| < 0.5 | 30% |
| 0.5 - 0.8 | 50% |
| > 0.8 | 75% |

This prevents cliff-drop recommendations (e.g., 1024Mi -> 13Mi) even when the math suggests it. Step-down gradually, validate at each level.

**Confidence scoring**:
- 7+ days of data, steady pattern -> 0.95 cap
- 3-7 days, steady -> 0.7-0.9
- <3 days -> 0.5 max (low confidence)
- Burstable patterns cap at 0.80
- Batch workloads cap at 0.70

#### Special Cases

**Best-effort pods** (no resource requests): Instead of skipping, the engine recommends adding requests based on observed usage with a HIGH risk flag.

**Well-sized workloads** (waste < 10%): Appear in reports with "no changes recommended" rather than being silently omitted.

**Anomalous workloads**: Get investigation recommendations, never sizing reductions. Memory reduction is blocked on any workload with a rising memory trend.

---

### 4. K8s-Sage SLM (L2)

A small language model fine-tuned specifically on Kubernetes operational knowledge.

#### Why a Specialist Model

General-purpose LLMs (GPT-4, Llama) know about Kubernetes from pretraining but lack:

- **Metric interpretation**: Can't distinguish a memory leak from legitimate growth from a cron spike
- **Right-sizing intuition**: Don't know that JVM needs GC headroom, or that Go services can run closer to P99
- **K8s-specific context**: Don't know that DaemonSets shouldn't be right-sized like Deployments, or that init containers don't need sustained resources

#### Model

| Property | Value |
|----------|-------|
| Base model | AI21 Jamba Reasoning 3B (hybrid Mamba-Transformer) |
| Fine-tuning | QLoRA (r=8, alpha=16, NF4 4-bit) via SFTTrainer |
| Quantisation | GGUF Q4_K_M (~1.8GB) |
| Serving | llama.cpp (CPU-only, ~2.5Gi RAM) |
| Inference | Single HTTP POST to `/completion`, <5s latency |

Jamba's hybrid Mamba-Transformer architecture was chosen for linear-time inference and strong structured output (JSON) capabilities. See [MODEL_DESIGN.md](MODEL_DESIGN.md) for full rationale.

#### Training Data

6,982 validated instruction pairs from 6 sources:

| Source | Count | % |
|--------|-------|---|
| Synthetic (mirrors rules engine) | 3,523 | 50.5% |
| Stack Overflow | 2,405 | 34.5% |
| GitHub Issues | 530 | 7.6% |
| K8s Docs | 289 | 4.1% |
| Expert pairs | 169 | 2.4% |
| VPA source analysis | 66 | 0.9% |

The synthetic data generator (`ml/dataset/generate_synthetic.py`) produces metric-to-recommendation pairs that mirror the Go rules engine exactly, covering 7 scenario categories with runtime-specific variants (JVM, Go, Python, Node.js, .NET, Ruby).

See [TRAINING_DATA.md](TRAINING_DATA.md) for the full dataset methodology.

#### L1 + L2 Orchestration

The analyzer always runs both layers, with L1 as the safety floor:

```
1. Run L1 rules engine (< 1ms, always runs)
2. If SLM not configured -> return L1 result
3. Call L2 with timeout (10s default)
4. If L2 succeeds -> validate against L1 safety invariants
5. If L2 passes safety -> merge (L2 values, L1 safety as floor)
6. If L2 violates safety -> use L1 result, log violation
7. If L2 fails/times out -> use L1 result, log error
```

L2 adds nuance that L1 cannot: natural language explanations, runtime-specific tuning advice, pattern recognition beyond simple thresholds. But L2 can never recommend below L1's safety floors.

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
helm install k8s-sage deploy/helm/k8s-sage

# With SLM enabled
helm install k8s-sage deploy/helm/k8s-sage --set slm.enabled=true

# Custom resource budgets for large clusters
helm install k8s-sage deploy/helm/k8s-sage \
  --set agent.resources.requests.memory=100Mi \
  --set agent.store.retentionHours=336
```

### Minimal Deployment (no model)

Agent DaemonSet + Server Deployment + CLI. L1 rules engine only. Total cluster overhead: ~50Mi per node + 256Mi for server.

### Full Deployment (with model)

Above + llama.cpp Deployment running k8s-sage SLM. Additional overhead: ~2.5Gi for model serving pod (CPU-only).

---

## Project Status

### Complete
- In-cluster infrastructure: agent, server, rules engine, CLI, Helm chart, CI/CD
- L1 rules engine with safety invariants (floors, caps, anomaly detection, classification guards)
- ML pipeline: 6,982 training pairs, QLoRA training script, merge/quantise, evaluation, serving
- Go SLM integration: client, prompts, parser (21 tests), L1+L2 analyzer orchestrator
- 8 dogfood workload archetypes with validation scripts
- CI: Python ruff lint + Helm lint in CI pipeline
- Grafana dashboard (Infinity datasource, 5 panels) + Helm ConfigMap for sidecar auto-import
- `sage annotate` CLI subcommand for GitOps source annotations
- Slack integration scaffold: ticker-based notifier, Block Kit messages, severity/dedup, Helm config
- GitOps discovery: ArgoCD Application + Flux Kustomization parsing, `sage discover` CLI subcommand
- Models package test suite (30 tests)
- PR creation flow: `sage pr` CLI subcommand (gh CLI + kubectl, dry-run support)
- Security pipeline: SonarCloud (A rating, zero security issues), Semgrep CI, Gremlins mutation testing (60% threshold), SHA-pinned GitHub Actions, kubelet TLS CA chain verification, regex DoS hardening
- Test coverage push: agent 83.8%, server 92.8%, slm 91.3%, pr 70.7% — 49 new test functions across runtime-critical packages
- Marketplace readiness: base image digest pinning, Helm image digest override support (agent/server/slm), GCP Application CR (gated), deployment guide for AWS/Azure/GCP (`docs/MARKETPLACE_DEPLOYMENT.md`)

### Ready (blocked on GPU)
- Fine-tune Jamba 3B on Threadripper + dual RTX 3090 (`./scripts/train.sh`)
- Merge + GGUF quantise + evaluate (`./scripts/eval_and_deploy.sh`)
- Dogfood v2 (L1 with fixes) and v3 (with L2)

### Remaining
- KWOK scale testing
- Marketplace submission: AWS (Helm chart + EKS add-on), Azure (CNAB packaging), GCP (deployer image). Technical code changes complete — remaining work is packaging artifacts and seller account onboarding. See `docs/MARKETPLACE_DEPLOYMENT.md` for full checklists.

---

## Key Files

| Path | Purpose |
|------|---------|
| `internal/rules/patterns.go` | Workload classification (steady/burstable/batch/idle/anomalous) |
| `internal/rules/recommendations.go` | Right-sizing logic, safety floors, reduction caps |
| `internal/rules/engine.go` | Orchestrates classification + recommendation |
| `internal/server/analyzer.go` | L1+L2 orchestrator with safety fallback |
| `internal/slm/client.go` | llama.cpp HTTP client |
| `internal/slm/prompts.go` | Prompt construction from workload metrics |
| `internal/slm/parser.go` | Structured JSON response parsing |
| `internal/slack/notifier.go` | Slack digest notifier (ticker loop) |
| `internal/slack/messages.go` | Block Kit message builders |
| `internal/gitops/discover.go` | ArgoCD + Flux workload discovery |
| `cmd/cli/annotate.go` | `sage annotate` — GitOps source annotations |
| `cmd/cli/discover.go` | `sage discover` — auto-detect GitOps mappings |
| `cmd/cli/pr.go` | `sage pr` — automated PR creation for right-sizing |
| `internal/pr/creator.go` | PR creation logic (gh CLI + kubectl, manifest/values modification) |
| `deploy/helm/k8s-sage/files/k8s-sage-dashboard.json` | Grafana dashboard |
| `deploy/helm/k8s-sage/templates/application.yaml` | GCP Marketplace Application CR (gated) |
| `ml/training/finetune_lora.py` | QLoRA fine-tuning (SFTTrainer) |
| `ml/dataset/generate_synthetic.py` | Synthetic training pair generator |
| `deploy/helm/k8s-sage/values.yaml` | Deployment configuration |
| `docs/MODEL_DESIGN.md` | Model selection, training config, evaluation targets |
| `docs/TRAINING_DATA.md` | Dataset methodology and provenance |
| `docs/MARKETPLACE_DEPLOYMENT.md` | Cloud marketplace listing guide (AWS, Azure, GCP) |

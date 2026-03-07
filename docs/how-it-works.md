# Architecture

## Problem

Kubernetes clusters are chronically over-provisioned. Engineering teams set resource requests high "just in case" and never revisit them. Studies consistently show 50-70% of allocated CPU and memory goes unused.

Existing solutions either give formulaic recommendations without context, or rely on external APIs and general-purpose models that don't understand Kubernetes resource patterns.

Gerty is purpose-built for Kubernetes right-sizing. It runs entirely inside your cluster, understands workload patterns, and delivers recommendations through your existing GitOps workflow.

---

## How It Works

### 1. Lightweight Agents

A tiny agent runs on every node as a DaemonSet. It collects resource usage metrics from the kubelet and pushes them to the Gerty server.

| Resource | Request | Limit |
|----------|---------|-------|
| CPU | 50m | 100m |
| Memory | 50Mi | 100Mi |

The agent collects per-container CPU and memory usage, requests, limits, and restart counts. Metrics are stored in a rolling window and pushed to the server every 5 minutes.

No write access to your cluster. No secrets access. Read-only metrics only.

### 2. Rules Engine

A deterministic rules engine classifies every workload and generates right-sizing recommendations. This runs without any AI and provides the safety floor for all recommendations.

#### Workload Classification

| Pattern | What It Means | Right-Sizing Strategy |
|---------|--------------|----------------------|
| **Steady** | Consistent, predictable usage | Set requests close to observed usage with headroom |
| **Burstable** | Periodic spikes above baseline | Lower requests, higher limits to accommodate spikes |
| **Batch** | High spikes with idle periods | Conservative requests, generous limits |
| **Idle** | Near-zero utilisation for 48h+ | Flagged for investigation |
| **Anomalous** | Monotonic memory growth | Flagged for investigation (possible memory leak) |

#### Safety Invariants

Every recommendation, regardless of source, must pass these checks:

| Invariant | Value | Rationale |
|-----------|-------|-----------|
| CPU floor | 50m | No real container runs below this |
| Memory floor | 64Mi | Minimum viable for any runtime |
| Memory recommendation | >= P99 working set x 1.10 | Prevent OOM kills |
| No zero recommendations | Always enforced | Zero = eviction risk |

Reduction caps are tied to confidence: higher confidence allows larger reductions, lower confidence is more conservative. This prevents cliff-drop recommendations.

### 3. AI Reasoning (Optional)

When enabled, Gerty's on-cluster AI adds deeper analysis that rules alone can't provide:

- **Runtime-aware sizing**: Understands JVM heap, Go GOGC, .NET GC, connection pools
- **Hold decisions**: Recognises when a workload shouldn't be resized and explains why
- **Temporal patterns**: Identifies batch, seasonal, and event-driven patterns
- **Partial recommendations**: Can recommend reducing CPU but not memory (or vice versa) with reasoning

The AI handles the majority of workloads quickly. When it encounters complex cases, a deeper analysis layer activates automatically, processes the queue, then scales back to zero. Gerty checks available cluster headroom before scaling and will never starve your workloads.

Choose your intelligence tier:

| Tier | What You Get |
|------|-------------|
| **Lite** | Fast scanning with good recommendations |
| **Standard** | Deeper reasoning for complex workloads — JVM heap, temporal patterns, blast radius |
| **Premium** | Maximum reasoning quality for heterogeneous environments |

All AI runs on CPU inside your cluster. No GPUs required. No external API calls. Your metadata never leaves the VPC.

### 4. CLI

```bash
gerty report                              # Cluster-wide efficiency report
gerty report -n production                # Namespace drill-down
gerty recommend                           # Right-sizing recommendations
gerty recommend --risk LOW                # Filter by risk level
gerty workloads                           # List all workloads
gerty annotate deployment/api \           # Set GitOps source annotations
  --repo github.com/acme/manifests \
  --path apps/api/values.yaml
gerty discover                            # Auto-detect ArgoCD/Flux mappings
gerty pr deployment/api-gateway           # Create right-sizing PR
gerty report -o json                      # JSON output for pipelines
```

---

## API

```
GET  /healthz                           Health check
GET  /api/v1/report                     Cluster-wide report
GET  /api/v1/report?namespace={ns}      Namespace report
GET  /api/v1/workloads                  All workloads
GET  /api/v1/workloads/{ns}/{kind}/{name}  Workload detail
GET  /api/v1/recommendations            All recommendations
GET  /api/v1/recommendations?risk=low   Filtered recommendations
POST /api/v1/ingest                     Agent push endpoint
POST /api/v1/analyze                    Trigger on-demand analysis
```

All responses are JSON. The `Accept: text/plain` header returns human-readable output.

---

## Deployment

### Minimal (no AI)

Agent DaemonSet + Server Deployment + CLI. Rules engine only. Total cluster overhead: ~50Mi per node + 256Mi for server.

### Full (with AI)

Everything above, plus the AI reasoning layer. The deeper analysis tier scales from zero when needed and scales back to zero when done. Gerty checks cluster headroom before scaling and degrades gracefully if resources are tight.

```bash
helm install gerty gerty/gerty
helm install gerty gerty/gerty --set slm.enabled=true --set slm.tier=standard
```

---

## Integrations

- **GitOps**: GitHub and GitLab. Auto-detected from repo URL. Gerty opens PRs — you review and merge.
- **Slack**: Periodic digest messages with grouped recommendations. Configurable severity filter and dedup.
- **Grafana**: Dashboard shipped as a ConfigMap for sidecar auto-import.
- **Prometheus**: Agent exposes standard `/metrics` endpoint.
- **Marketplace**: AWS, GCP, and Azure Marketplace (coming soon).

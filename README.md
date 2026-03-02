# k8s-sage

Kubernetes resource efficiency platform. Lightweight in-cluster agents collect metrics and identify waste. A deterministic rules engine classifies workloads and generates right-sizing recommendations. An optional fine-tuned SLM adds context-aware explanations.

```
┌─────────────────────────────────────┐
│         Kubernetes Cluster          │
│                                     │
│  ┌─────────┐ ┌─────────┐           │
│  │ Agent   │ │ Agent   │ DaemonSet  │
│  │ Node 1  │ │ Node N  │ per node   │
│  └────┬────┘ └────┬────┘           │
│       └─────┬─────┘                 │
│             ▼                       │
│  ┌──────────────────┐               │
│  │   sage-server    │               │
│  │  ┌────────────┐  │               │
│  │  │Rules Engine│  │               │
│  │  └────────────┘  │               │
│  │  ┌────────────┐  │               │
│  │  │ SLM (opt.) │  │               │
│  │  └────────────┘  │               │
│  └────────┬─────────┘               │
│           ▼                         │
│       REST API (:8080)              │
└───────────┬─────────────────────────┘
            ▼
     ┌────────────┐
     │  sage-cli   │
     └────────────┘
```

## Components

**Agents** (DaemonSet, 50Mi/50m per node) scrape the kubelet Summary API every 30s, compute per-container waste, and push `NodeReport` JSON to the server every 5 minutes.

**Server** aggregates reports cluster-wide, groups by owner (Deployment, StatefulSet, etc.), and runs the rules engine. Classifies workloads as steady, burstable, batch, idle, or anomalous, then generates right-sizing recommendations with confidence scores and risk levels.

**CLI** queries the server API. Subcommands: `report`, `recommend`, `workloads`, `annotate`, `discover`.

**Slack notifier** (optional) sends periodic digest messages with grouped recommendations to a webhook. Configurable severity filter and 7-day dedup.

**Grafana dashboard** (optional) visualises cluster waste, namespace breakdown, top wasting workloads, and recommendations via the Infinity datasource.

## Quick Start

Prerequisites: Go 1.22+, Docker, kind + kubectl, Helm 3.

```bash
make build                  # Go binaries
make docker-build           # Container images
./scripts/kind-cluster.sh   # Create kind cluster
helm install k8s-sage deploy/helm/k8s-sage --set image.pullPolicy=Never
```

## CLI

```bash
sage report                              # Cluster-wide waste report
sage report --namespace production       # Namespace drill-down
sage recommend                           # Right-sizing recommendations
sage recommend --risk LOW                # Filter by risk
sage workloads                           # List workloads
sage workloads production/Deployment/api # Workload detail
sage annotate deployment/api \           # Set GitOps source annotations
  --repo github.com/acme/manifests \
  --path apps/api/values.yaml
sage discover                            # Auto-detect ArgoCD/Flux mappings
sage report -o json                      # JSON output
```

Server address: `--server` flag > `SAGE_SERVER` env > `http://localhost:8080`.

## API

All responses wrapped in `{"status": "ok"|"error", "data": ..., "timestamp": "..."}`.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/healthz` | Health check |
| GET | `/readyz` | Ready check |
| POST | `/api/v1/ingest` | Agent pushes NodeReport |
| GET | `/api/v1/report` | Cluster report (optional `?namespace=`) |
| GET | `/api/v1/workloads` | All workloads |
| GET | `/api/v1/workloads/{ns}/{kind}/{name}` | Workload detail |
| GET | `/api/v1/recommendations` | Recommendations (optional `?risk=&namespace=`) |
| POST | `/api/v1/analyze` | On-demand analysis |

## Helm

```bash
helm install k8s-sage deploy/helm/k8s-sage
helm install k8s-sage deploy/helm/k8s-sage --set slm.enabled=true
helm install k8s-sage deploy/helm/k8s-sage --set grafana.dashboards.enabled=true
helm install k8s-sage deploy/helm/k8s-sage --set slack.enabled=true --set slack.webhookURL=https://hooks.slack.com/...
```

| Value | Default | Description |
|-------|---------|-------------|
| `agent.resources.requests.cpu` | `50m` | Agent CPU request |
| `agent.resources.requests.memory` | `50Mi` | Agent memory request |
| `agent.pushInterval` | `5m` | Report push interval |
| `server.replicas` | `1` | Server replicas |
| `slm.enabled` | `false` | Enable SLM serving |
| `grafana.dashboards.enabled` | `false` | Deploy Grafana dashboard ConfigMap |
| `slack.enabled` | `false` | Enable Slack notifications |
| `slack.webhookURL` | `""` | Slack incoming webhook URL |
| `slack.channel` | `#k8s-sage` | Slack channel |
| `slack.digestInterval` | `1h` | Notification interval |
| `slack.minSeverity` | `optimisation` | Minimum severity to notify |

## How Recommendations Work

| Pattern | Trigger | Strategy |
|---------|---------|----------|
| Steady | CV < 0.3 | Request = P95 * 1.20 |
| Burstable | Periodic spikes | Request = P50 * 1.20, Limit = P99 * 1.20 |
| Batch | Extreme spike ratios | Request = P50 * 1.20, Limit = Max * 1.20 |
| Idle | < 5% utilisation for 48h+ | Flag for investigation |
| Anomalous | Monotonic memory growth | Flag for investigation |

Safety invariants: CPU floor 50m, memory floor 64Mi, memory >= P99 working set * 1.10. Reduction caps: 30% (low confidence), 50% (medium), 75% (high).

## Project Structure

```
k8s-sage/
├── cmd/
│   ├── agent/              # DaemonSet entrypoint
│   ├── server/             # Server entrypoint
│   └── cli/                # CLI (report, recommend, workloads, annotate, discover)
├── internal/
│   ├── agent/              # Collector, store, reporter, pusher
│   ├── server/             # Aggregator, API, analyzer
│   ├── rules/              # Classification + recommendation engine
│   ├── models/             # Shared types
│   ├── slm/                # SLM client (llama.cpp)
│   ├── slack/              # Slack webhook notifier
│   └── gitops/             # ArgoCD + Flux discovery
├── ml/
│   ├── dataset/            # Training data (6,982 pairs) + generators
│   ├── training/           # QLoRA fine-tuning, eval, merge/quantise
│   └── serving/            # llama.cpp config + smoke tests
├── deploy/
│   ├── helm/k8s-sage/      # Helm chart
│   └── grafana/            # Standalone Grafana dashboard
├── test/
│   ├── backtest/           # 52 scenario regression tests
│   ├── safety/             # Safety invariant tests
│   ├── integration/        # E2E tests
│   └── dogfood/            # 8 workload archetypes + validation
├── scripts/                # Dev + training scripts
└── docs/                   # Architecture, model design, training data
```

## Build and Test

```bash
make build            # Build all Go binaries
make test             # Unit tests (go test -p 2 -timeout 120s ./...)
make lint             # go vet + staticcheck + ruff + helm lint
make lint-python      # ruff check ml/
make lint-helm        # helm lint deploy/helm/k8s-sage/
make docker-build     # Container images
make dev-cluster      # kind cluster
make dev-deploy       # Helm install to kind
make backtest         # 52 scenario regression tests
make test-safety      # Safety invariant tests
make test-integration # E2E (requires running cluster)
```

## Limitations

- No persistent storage. Restarting loses historical data. Agents rebuild within 30s; server rebuilds as agents push.
- No authentication on the API. Deploy behind network policy or service mesh.
- Single-server, no HA. Sufficient for clusters up to 10k pods.
- SLM requires separate training run on GPU hardware before use.

## Licence

Copyright (c) 2026 Gregory Carroll. All rights reserved. See [COPYRIGHT](./COPYRIGHT).

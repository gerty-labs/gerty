# k8s-sage

Kubernetes resource efficiency platform. Lightweight in-cluster agents identify waste; a deterministic rules engine generates right-sizing recommendations.

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
│  └────────┬─────────┘               │
│           ▼                         │
│       REST API (:8080)              │
└───────────┬─────────────────────────┘
            ▼
     ┌────────────┐
     │  sage-cli   │
     └────────────┘
```

## What It Does

- **Agents** (DaemonSet, 50Mi/50m per node) scrape the kubelet Summary API every 30s, compute waste per container (`request - P95 usage`), and push reports to the server.
- **Server** aggregates reports cluster-wide, groups by owner (Deployment, StatefulSet, etc.), and runs a deterministic rules engine that classifies workloads (steady, burstable, batch, idle) and generates right-sizing recommendations with confidence scores and risk levels.
- **CLI** queries the server API and displays reports and recommendations as tables or JSON.

## Quick Start

### Prerequisites

- Go 1.22+
- Docker (for container images)
- kind + kubectl (for local testing)
- Helm 3 (for deployment)

### Build

```bash
# Go binaries
go build ./cmd/agent
go build ./cmd/server
go build ./cmd/cli

# Container images
docker build -t k8s-sage-agent:latest -f Dockerfile.agent .
docker build -t k8s-sage-server:latest -f Dockerfile.server .
```

### Deploy to kind

```bash
# Create cluster and load images
./scripts/kind-cluster.sh

# Install via Helm
helm install k8s-sage ./deploy/helm/k8s-sage/ --set image.pullPolicy=Never

# Deploy test workloads
./test/dogfood/setup-workloads.sh
```

### Run the CLI

```bash
# Cluster-wide waste report
sage report

# Namespace drill-down
sage report --namespace production

# Right-sizing recommendations
sage recommend

# Filter by risk level
sage recommend --risk LOW

# List workloads
sage workloads

# Workload detail
sage workloads production/Deployment/api-server

# JSON output (pipe to jq, etc.)
sage report -o json
```

### Configuration

The CLI resolves the server address in order: `--server` flag > `SAGE_SERVER` env var > `http://localhost:8080`.

```bash
export SAGE_SERVER=http://sage-server.k8s-sage.svc:8080
sage report
```

## API

All responses are wrapped in an envelope:

```json
{"status": "ok", "data": {...}, "timestamp": "2026-03-01T12:00:00Z"}
```

| Method | Path | Description |
|--------|------|-------------|
| GET | `/healthz` | Health check |
| GET | `/readyz` | Ready check (503 until agent data received) |
| POST | `/api/v1/ingest` | Agent pushes NodeReport |
| GET | `/api/v1/report` | Cluster-wide waste report |
| GET | `/api/v1/report?namespace=ns` | Namespace-scoped report |
| GET | `/api/v1/workloads` | All workloads with waste summary |
| GET | `/api/v1/workloads/{ns}/{kind}/{name}` | Single workload detail |
| GET | `/api/v1/recommendations` | Right-sizing recommendations |
| GET | `/api/v1/recommendations?risk=LOW&namespace=ns` | Filtered recommendations |
| POST | `/api/v1/analyze` | Analyze a specific namespace |

## Helm Values

Key configuration options:

| Value | Default | Description |
|-------|---------|-------------|
| `agent.resources.requests.cpu` | `50m` | Agent CPU request (hard constraint) |
| `agent.resources.requests.memory` | `50Mi` | Agent memory request (hard constraint) |
| `server.replicas` | `1` | Server replica count |
| `server.resources.requests.memory` | `256Mi` | Server memory request |
| `server.service.type` | `ClusterIP` | Server service type |
| `server.service.port` | `8080` | Server service port |
| `slm.enabled` | `false` | Enable SLM model serving (Phase 2) |

## How Recommendations Work

The rules engine classifies each workload into one of four patterns:

| Pattern | Trigger | Strategy |
|---------|---------|----------|
| **Steady** | Low variance (CV < 0.3) | Request = P95 + 20% headroom |
| **Burstable** | Periodic spikes | Request = P50 + 30%, Limit = P99 + 25% |
| **Batch** | Extreme spike ratios | Request = P50 + 30%, Limit = Max + 10% |
| **Idle** | < 5% utilisation for 48h+ | Flag for investigation |

Confidence scoring accounts for data window duration (7+ days = high confidence) and pattern stability. Risk levels reflect how close recommendations are to observed peaks (P99 for CPU, Max for memory).

Safety invariants enforced:
- Memory recommendations never go below P99 working set
- CPU floor: 10m, Memory floor: 4Mi
- Waste must exceed 10% of request before recommending changes

## Limitations

- **No persistent storage**: Agent and server are in-memory only. Restarting loses historical data. Agents rebuild from the kubelet within 30s; server rebuilds as agents push.
- **No SLM yet**: The fine-tuned small language model is Phase 2. Current recommendations come from the deterministic rules engine only.
- **No push from agent to server**: Agents currently expose `/report` for pull. Push-based reporting to the server is not yet implemented — the server aggregates what it receives via the ingest API.
- **No authentication**: The API has no auth. Deploy behind a network policy or service mesh in production.
- **Single-server**: No HA for the server. A single replica is sufficient for clusters up to 10k pods.
- **Owner detection**: Relies on `ownerReference` in pod waste reports. Standalone pods (no owner) are treated as their own owner.

## Project Structure

```
k8s-sage/
├── cmd/
│   ├── agent/          # DaemonSet entrypoint (port 9101)
│   ├── server/         # Server entrypoint (port 8080)
│   └── cli/            # CLI (cobra)
├── internal/
│   ├── agent/          # Collector, store, reporter
│   ├── server/         # Aggregator, API handlers
│   ├── rules/          # Classification + recommendation engine
│   ├── models/         # Shared types
│   └── slm/            # SLM client (placeholder)
├── deploy/helm/        # Helm chart
├── scripts/            # Dev scripts (kind cluster)
├── test/dogfood/       # Synthetic workload scripts
├── Dockerfile.agent    # ~6MB scratch image
└── Dockerfile.server   # ~6MB scratch image
```

## Licence

Copyright (c) 2026 Gregory Carroll. All rights reserved.
See [COPYRIGHT](./COPYRIGHT) for details.

# Architecture

```
Kubernetes Cluster
 +-- gerty-agent (DaemonSet, per node)
 |     Collects metrics from kubelet
 |     Reports to gerty-server
 |
 +-- gerty-server
 |     L1: Deterministic rules engine
 |     L2: Local SLM (optional)
 |     Serves REST API
 |
 +-- gerty-slm (optional Deployment)
       llama.cpp server
       CPU-only inference

gerty CLI (your machine)
  kubectl plugin
  Talks to gerty-server REST API
  Opens PRs in your GitOps repo
```

## Agent

The agent runs as a DaemonSet with one pod per node. It collects per-pod CPU and memory usage directly from the kubelet metrics endpoint, avoiding any dependency on Prometheus or metrics-server.

Resource budget: **50MB RAM, 0.05 CPU**. The agent is invisible. If the efficiency tool uses meaningful resources, it has failed.

Metrics are pushed to the server at a configurable interval (default 5 minutes).

## Server

The server processes metrics through two layers:

**L1 -- Rules Engine (deterministic, always on).** Classifies workloads (steady, burstable, batch, idle), computes right-sizing recommendations with confidence scoring, and enforces safety floors. L1 is the safety floor: it never recommends below 50m CPU / 64Mi memory, and caps reductions based on confidence tiers.

**L2 -- SLM (optional enhancement).** A Small Language Model that generates human-readable explanations and can refine recommendations with workload-specific context. L2 enhances but never overrides L1 safety constraints.

The server exposes a REST API consumed by the CLI.

## CLI

The `gerty` CLI is a kubectl plugin with six subcommands:

| Command | Purpose |
|---------|---------|
| `report` | Cluster-wide efficiency report |
| `workloads` | List workloads with waste metrics |
| `recommend` | Right-sizing recommendations |
| `pr` | Open a PR with recommendations |
| `annotate` | Link workloads to GitOps source files |
| `discover` | Auto-discover ArgoCD/Flux mappings |

See the [CLI Reference](/cli) for full usage.

## SLM

When enabled, the SLM runs as a separate Deployment using [llama.cpp](https://github.com/ggerganov/llama.cpp). The model is delivered via an init container and served CPU-only -- no GPU required, no external API calls.

Typical resource usage: ~2.5GB RAM, 1-2 CPU cores. The SLM is optional; L1 provides full functionality without it.

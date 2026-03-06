# Gerty

> Efficiency is an engineering problem. Gerty is the engineer.

**Sovereign Kubernetes right-sizing.** No SaaS. No telemetry egress. No "success tax."

Gerty is a self-hosted efficiency assistant for Kubernetes clusters. She monitors real-world resource utilisation, classifies workload patterns, and delivers right-sizing recommendations -- all without your metadata leaving the VPC.

---

## What Gerty Does

- **Collects** per-pod CPU and memory usage via kubelet metrics (DaemonSet, <50MB RAM)
- **Classifies** workload patterns: steady-state, burstable, batch, idle
- **Recommends** right-sized resource requests and limits with confidence scoring
- **Opens PRs** directly in your GitOps repository -- no live-patching, no drift
- **Explains** every recommendation with technical justification

## What Gerty Doesn't Do

- Phone home
- Send your cluster metadata to a third-party SaaS
- Charge a percentage of your "savings"
- Auto-pilot your API server

## Architecture

```
Kubernetes Cluster
 +-- gerty-agent (DaemonSet, per node)
 |     Collects metrics from kubelet
 |     Reports to gerty-server
 |
 +-- gerty-server
       L1: Deterministic rules engine
       L2: Local SLM (Small Language Model, optional)
       Serves REST API

gerty-cli
  CLI for reports, recommendations, PR creation
```

The agent is invisible: 50MB RAM, 0.05 CPU. If the efficiency tool uses meaningful resources, it has failed.

The SLM runs locally on your nodes via llama.cpp. CPU-only, ~2.5GB RAM. No GPU required. No API calls to external services.

## Install

```bash
helm repo add gerty https://gerty-labs.github.io/gerty
helm install gerty gerty/gerty
```

## CLI

```bash
gerty report                          # Cluster-wide efficiency report
gerty workloads                       # List all workloads with waste metrics
gerty recommend -n production         # Right-sizing recommendations
gerty pr deployment/api-gateway       # Open a PR with the recommendation
gerty annotate deployment/api -n prod \
  --repo https://github.com/acme/k8s  \
  --path apps/api/deployment.yaml     # Link workloads to GitOps source
gerty discover                        # Auto-discover ArgoCD/Flux mappings
```

## Pricing

**First 10 nodes free.** No credit card. No trial period. No feature gates.

| Total Nodes | Rate |
|-------------|------|
| 1--10 | Free |
| 11--50 | £5/node/month |
| 51--100 | £4.50/node/month |
| 101+ | £4/node/month |

All model tiers included -- pick Lite (2B), Standard (4B), or Premium (9B) based on your resource budget, not pricing.

Full pricing details at [gerty.io/pricing](https://gerty.io/pricing).

## The Manifesto

[The Sovereign SRE Manifesto](https://gerty.io/manifesto)

---

Copyright 2026 Gregory Carroll. Licensed under [Apache 2.0](LICENSE).

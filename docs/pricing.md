# Pricing

Flat per-node pricing. No "savings tax." No per-recommendation fees. No surprise invoices.

## Free Tier

Your first **10 nodes are always free**. No credit card. No trial period. No feature gates.

Every feature - including the SLM, GitOps PRs, and all model tiers - is available on the free tier.

## Volume Pricing

Tier is determined by your **total node count**. The first 10 nodes are free in every tier. Remaining nodes are billed at a flat rate for that tier.

| Total Nodes | Free | Billed | Rate | Example Monthly |
|-------------|------|--------|------|-----------------|
| 1-10 | All free | 0 | - | £0 |
| 11--50 | 10 | Rest | £5/node | 30 nodes: 20 x £5 = **£100** |
| 51--100 | 10 | Rest | £4.50/node | 60 nodes: 50 x £4.50 = **£225** |
| 101+ | 10 | Rest | £4/node | 150 nodes: 140 x £4 = **£560** |

## Model Tiers (All Included)

All model tiers are included at every pricing level. Gerty automatically detects your cluster shape on install and recommends the right tier.

| Tier | Workloads | GGUF Size | RAM | Typical Cluster |
|------|-----------|-----------|-----|-----------------|
| Lite | Up to ~150 | 1.3 GB | ~1.5 GB | 10-25 small nodes |
| Standard | Up to ~500 | 2.7 GB | ~3 GB | 25-75 mixed nodes |
| Premium | Up to ~1,000 | ~5.5 GB | ~6 GB | 75-150 nodes |
| Premium + replicas | 1,000+ | ~5.5 GB x 2-3 | ~6 GB x 2-3 | 150+ nodes |

The tier isn't about node count - it's about how many workloads the model needs to reason over. A dense 25-node cluster running 1,250 workloads needs Premium, and Gerty will tell you that.

### Auto-Detect

On install, Gerty counts your workloads and recommends the appropriate tier:

```
Gerty detected 340 workloads across 45 nodes.
Recommend: Standard model.
Your current Lite model may result in slower analysis cycles.
```

Override manually if needed:

```bash
helm install gerty gerty/gerty --set slm.modelSize=premium
```

## Marketplace

Gerty will be available on **AWS Marketplace**, **GCP Marketplace**, and **Azure Marketplace**. Billing handled through your existing cloud account - no separate invoicing.

## Why Per-Node

We charge per node, not per "saving." Our incentives are aligned with your cluster's health, not your cloud provider's invoice. If Gerty isn't saving you more than it costs, cancel it.

Some tools charge per vCPU - fair enough if they're live-patching every core. Gerty observes and recommends, so we charge per node. A 3-node cluster is 3 units, not 288. The model tier handles performance scaling separately - they're different concerns.

# Pricing

Flat per-node pricing. No "savings tax." No per-recommendation fees. No surprise invoices.

## Free Tier

Your first **10 nodes are always free**. No credit card. No trial period. No feature gates.

Every feature -- including the SLM, GitOps PRs, and all model tiers -- is available on the free tier.

## Volume Pricing

Tier is determined by your **total node count**. The first 10 nodes are free in every tier. Remaining nodes are billed at a flat rate for that tier.

| Total Nodes | Free | Billed | Rate | Example Monthly |
|-------------|------|--------|------|-----------------|
| 1--10 | All free | 0 | -- | £0 |
| 11--50 | 10 | Rest | £5/node | 30 nodes: 20 x £5 = **£100** |
| 51--100 | 10 | Rest | £4.50/node | 60 nodes: 50 x £4.50 = **£225** |
| 101+ | 10 | Rest | £4/node | 150 nodes: 140 x £4 = **£560** |

## Model Tiers (All Included)

All model tiers are included at every pricing level. Pick the one that fits your resource budget.

| Tier | Model | GGUF Size | RAM | Best For |
|------|-------|-----------|-----|----------|
| Lite | Qwen 3.5 2B | 1.3 GB | ~1.5 GB | Small clusters, tight resources |
| Standard | Qwen 3.5 4B | 2.7 GB | ~3 GB | Most clusters (default) |
| Premium | Qwen 3.5 9B | ~5.5 GB | ~6 GB | Large clusters, best reasoning |

Select via Helm:

```bash
# Lite (2B)
helm install gerty gerty/gerty --set slm.modelSize=2b

# Standard (4B, default)
helm install gerty gerty/gerty --set slm.enabled=true

# Premium (9B)
helm install gerty gerty/gerty --set slm.modelSize=9b
```

## Why Per-Node

We charge per node, not per "saving." Our incentives are aligned with your cluster's health, not your cloud provider's invoice. If Gerty isn't saving you more than it costs, cancel it.

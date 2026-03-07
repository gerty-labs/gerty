# Pricing

Flat per-node pricing. No "savings tax." No per-recommendation fees. No surprise invoices.

## Free Tier

Your first **10 nodes are always free**. No credit card. No trial period. No feature gates.

Every feature - including AI reasoning, GitOps PRs, and all tiers - is available on the free tier.

## Volume Pricing

Tier is determined by your **total node count**. The first 10 nodes are free in every tier. Remaining nodes are billed at a flat rate for that tier.

| Total Nodes | Free | Billed | Rate | Example Monthly |
|-------------|------|--------|------|-----------------|
| 1-10 | All free | 0 | - | £0 |
| 11--50 | 10 | Rest | £5/node | 30 nodes: 20 x £5 = **£100** |
| 51--100 | 10 | Rest | £4.50/node | 60 nodes: 50 x £4.50 = **£225** |
| 101+ | 10 | Rest | £4/node | 150 nodes: 140 x £4 = **£560** |

## Intelligence Tiers (All Included)

All tiers are included at every pricing level. The tier determines reasoning depth, not capacity. Every tier handles any cluster size.

| Tier | What You Get |
|------|-------------|
| **Lite** | Fast scanning with good recommendations. Minimal cluster footprint. Right for most clusters. |
| **Standard** | Everything in Lite, plus deeper reasoning for the hard cases: JVM heap sizing, temporal patterns, blast radius awareness. |
| **Premium** | Everything in Standard, with the best possible reasoning quality on complex workloads. |

Gerty's AI scales from zero when needed and scales back down when done. It checks available cluster headroom before scaling and will never starve your workloads. If headroom is tight, it runs leaner (slower but safe) and notifies you via Slack.

### Choosing a Tier

- **Lite** — fast, lightweight, handles 60-80% of right-sizing decisions well.
- **Standard** — adds deeper analysis for runtime-specific ceilings, hold decisions, and temporal patterns.
- **Premium** — maximum reasoning depth for teams with complex, heterogeneous workloads.

Set your tier:

```bash
helm install gerty gerty/gerty --set slm.tier=premium
```

## Marketplace

Gerty will be available on **AWS Marketplace**, **GCP Marketplace**, and **Azure Marketplace**. Billing handled through your existing cloud account - no separate invoicing.

## Why Per-Node

We charge per node, not per "saving." Our incentives are aligned with your cluster's health, not your cloud provider's invoice. If Gerty isn't saving you more than it costs, cancel it.

Some tools charge per vCPU - fair enough if they're live-patching every core. Gerty observes and recommends, so we charge per node. A 3-node cluster is 3 units, not 288. The model tier handles performance scaling separately - they're different concerns.

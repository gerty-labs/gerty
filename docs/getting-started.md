# Getting Started

## Prerequisites

- A Kubernetes cluster (1.27+) with managed node pools (EKS, GKE Standard, AKS)
- Helm 3.x

::: info
Gerty v1 supports clusters with managed node pools. GKE Autopilot and AWS Fargate support is on the roadmap.
:::

## Install

```bash
helm repo add gerty https://gerty-labs.github.io/gerty
helm install gerty gerty/gerty
```

## Verify

Check that the agent and server pods are running:

```bash
kubectl get pods -l app.kubernetes.io/name=gerty
```

You should see a `gerty-agent` pod on each node (DaemonSet) and one `gerty-server` pod.

## First Report

Generate a cluster-wide efficiency report:

```bash
gerty report
```

This shows waste metrics across all namespaces, with workload classification (steady, burstable, batch, idle) and potential savings.

## Next Steps

- [Configuration](/configuration) - customise agent intervals, AI reasoning, integrations
- [CLI Reference](/cli) - all commands and flags
- [Architecture](/how-it-works) - how the components fit together

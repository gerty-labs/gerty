# Getting Started

## Prerequisites

- A Kubernetes cluster (1.27+)
- Helm 3.x

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

- [Configuration](/configuration) - customise agent intervals, SLM, integrations
- [CLI Reference](/cli) - all commands and flags
- [Architecture](/architecture) - how the components fit together

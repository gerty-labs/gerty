# Configuration

All configuration is via Helm values. Override defaults with `--set` or a custom values file:

```bash
helm install gerty gerty/gerty -f my-values.yaml
```

## Global

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `nameOverride` | string | `""` | Override the chart name |
| `fullnameOverride` | string | `""` | Override the full release name |
| `image.registry` | string | `ghcr.io/gerty-labs` | Container image registry |
| `image.tag` | string | Chart appVersion | Image tag for all components |
| `image.pullPolicy` | string | `IfNotPresent` | Image pull policy |

## Agent

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `agent.image.repository` | string | `gerty-agent` | Agent image name |
| `agent.image.digest` | string | `""` | Image digest override |
| `agent.resources.requests.cpu` | string | `50m` | CPU request |
| `agent.resources.requests.memory` | string | `50Mi` | Memory request |
| `agent.resources.limits.cpu` | string | `100m` | CPU limit |
| `agent.resources.limits.memory` | string | `100Mi` | Memory limit |
| `agent.scrapeInterval` | string | `30s` | Kubelet scrape interval |
| `agent.pushInterval` | string | `5m` | Server push interval |
| `agent.nodeSelector` | object | `{}` | Node selector |
| `agent.tolerations` | list | `[]` | Tolerations |

## Server

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `server.image.repository` | string | `gerty-server` | Server image name |
| `server.image.digest` | string | `""` | Image digest override |
| `server.replicas` | int | `1` | Replica count |
| `server.resources.requests.cpu` | string | `250m` | CPU request |
| `server.resources.requests.memory` | string | `256Mi` | Memory request |
| `server.resources.limits.cpu` | string | `500m` | CPU limit |
| `server.resources.limits.memory` | string | `512Mi` | Memory limit |
| `server.service.type` | string | `ClusterIP` | Service type |
| `server.service.port` | int | `8080` | Service port |
| `server.nodeSelector` | object | `{}` | Node selector |
| `server.tolerations` | list | `[]` | Tolerations |

## Server Persistence

Gerty persists aggregator state so recommendations survive pod restarts. By default, it uses an embedded database backed by a PVC. For multi-AZ clusters, an external PostgreSQL database can be used instead.

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `server.persistence.enabled` | bool | `true` | Enable state persistence |
| `server.persistence.storageClass` | string | `""` | PVC storage class (empty = cluster default) |
| `server.persistence.size` | string | `1Gi` | PVC size |
| `server.persistence.accessModes` | list | `[ReadWriteOnce]` | PVC access modes |
| `server.externalDatabase.enabled` | bool | `false` | Use external PostgreSQL instead of embedded storage |
| `server.externalDatabase.url` | string | `""` | PostgreSQL connection string |

::: tip
PVCs bind to a single availability zone. In multi-AZ clusters, either pin the server to an AZ via `nodeAffinity`, use a cross-AZ StorageClass, or set `server.externalDatabase.enabled=true` with a PostgreSQL URL.
:::

## AI Reasoning

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `slm.enabled` | bool | `false` | Enable AI reasoning |
| `slm.tier` | string | `standard` | Intelligence tier (`lite`, `standard`, or `premium`) |
| `slm.scaling.maxMemoryBudget` | string | `12Gi` | Max total RAM Gerty can consume for AI reasoning |
| `slm.scaling.maxCpuBudget` | string | `4` | Max total CPU cores for AI reasoning |
| `slm.scaling.minClusterHeadroom` | string | `20%` | Never consume more than this % of free cluster resources |
| `slm.persistence.enabled` | bool | `false` | Enable persistent model storage |
| `slm.persistence.size` | string | `5Gi` | PVC size |
| `slm.persistence.storageClass` | string | `""` | Storage class |

### Intelligence Tiers

All tiers are included at every pricing level. The tier determines reasoning depth, not workload capacity.

| `slm.tier` | Reasoning Depth | Best For |
|-------------|----------------|----------|
| `lite` | Fast scanning, good recommendations | Most clusters, tight resource budgets |
| `standard` | Deeper analysis for complex workloads | Production clusters with mixed workload types |
| `premium` | Maximum reasoning quality | Large clusters with complex, heterogeneous workloads |

Gerty's AI scales automatically based on demand. Standard and Premium tiers include a deeper analysis layer that scales from zero when needed, then scales back down. Gerty checks cluster headroom before scaling and degrades gracefully if resources are tight.

## GitOps

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `gitops.provider` | string | `""` | Git provider (`github` or `gitlab`). Auto-detected from repo URL if not set |
| `gitops.token` | string | `""` | Personal access token or deploy token |
| `gitops.tokenSecretRef` | string | `""` | Reference to an existing Secret containing the token |
| `gitops.gitlab.url` | string | `https://gitlab.com` | GitLab instance URL (for self-hosted) |
| `gitops.prTemplate` | string | `""` | Custom PR/MR description template ([Go text/template](https://pkg.go.dev/text/template)). Uses built-in default if empty |

### PR Template

The default PR template includes workload name, namespace, pattern classification, confidence score, resource changes table, metrics summary, and risk assessment. All values are populated from the rules engine. If AI reasoning is enabled, an optional reasoning section is appended.

Override with a custom [Go text/template](https://pkg.go.dev/text/template). Available fields:

| Field | Type | Description |
|-------|------|-------------|
| `.Workload` | string | Workload name (e.g. `deployment/api-gateway`) |
| `.Namespace` | string | Kubernetes namespace |
| `.Pattern` | string | Classification (`steady`, `burstable`, `batch`) |
| `.Confidence` | float64 | Confidence score (0.0 - 1.0) |
| `.ObservationDays` | int | Number of days of metrics data |
| `.Changes` | []Change | List of resource changes (`.Resource`, `.Current`, `.Recommended`, `.Delta`) |
| `.Metrics` | Metrics | Summary metrics (`.CPUP95`, `.MemP95`, `.Samples`) |
| `.Risk` | string | Risk level (`LOW`, `MEDIUM`, `HIGH`) |
| `.Reasoning` | string | AI explanation (empty if AI reasoning disabled) |

## Integrations

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `gcpMarketplace.enabled` | bool | `false` | Enable GCP Marketplace integration |
| `serviceAccount.create` | bool | `true` | Create a ServiceAccount |
| `serviceAccount.name` | string | `""` | ServiceAccount name override |
| `serviceAccount.annotations` | object | `{}` | ServiceAccount annotations |
| `grafana.dashboards.enabled` | bool | `false` | Deploy Grafana dashboard ConfigMap |
| `networkPolicy.enabled` | bool | `true` | Enable NetworkPolicy |
| `networkPolicy.allowExternalIngress` | bool | `true` | Allow external ingress to server |
| `slack.enabled` | bool | `false` | Enable Slack notifications |
| `slack.webhookURL` | string | `""` | Slack webhook URL |
| `slack.channel` | string | `#gerty` | Slack channel |
| `slack.digestInterval` | string | `1h` | Digest send interval |
| `slack.minSeverity` | string | `optimisation` | Minimum severity to notify |

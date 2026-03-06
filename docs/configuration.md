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

## SLM

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `slm.enabled` | bool | `false` | Enable the SLM deployment |
| `slm.image.repository` | string | `ghcr.io/ggerganov/llama.cpp` | llama.cpp image |
| `slm.image.tag` | string | `server` | Image tag |
| `slm.image.digest` | string | `""` | Image digest override |
| `slm.modelSize` | string | `standard` | Model tier (`lite`, `standard`, or `premium`) |
| `slm.model.path` | string | `/models/gerty.gguf` | Model file path in container |
| `slm.model.repository` | string | `gerty-model` | Model init container image |
| `slm.model.tag` | string | Chart appVersion | Model image tag |
| `slm.model.digest` | string | `""` | Model image digest override |
| `slm.args` | list | See values.yaml | llama.cpp server arguments |
| `slm.resources.requests.cpu` | string | `1` | CPU request |
| `slm.resources.requests.memory` | string | `2.5Gi` | Memory request (auto-adjusted by model size) |
| `slm.resources.limits.cpu` | string | `2` | CPU limit |
| `slm.resources.limits.memory` | string | `3Gi` | Memory limit (auto-adjusted by model size) |


### Model Tiers

All model tiers are included at every pricing level. Choose based on your resource budget:

| `modelSize` | Tier | GGUF Size | RAM Required | Best For |
|-------------|------|-----------|--------------|----------|
| `lite` | Lite | 1.3 GB | ~1.5 GB | Small clusters, tight resources |
| `standard` | Standard | 2.7 GB | ~3 GB | Most clusters (default) |
| `premium` | Premium | ~5.5 GB | ~6 GB | Large clusters, best reasoning |

| `slm.persistence.enabled` | bool | `false` | Enable persistent model storage |
| `slm.persistence.size` | string | `5Gi` | PVC size |
| `slm.persistence.storageClass` | string | `""` | Storage class |

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

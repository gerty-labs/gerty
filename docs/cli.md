# CLI Reference

The `gerty` CLI is a kubectl plugin that communicates with the gerty-server REST API.

## Global Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--server` | | gerty-server address |
| `-o, --output` | `table` | Output format (`table` or `json`) |

## report

Show a cluster-wide or namespace-scoped efficiency report.

```bash
gerty report
gerty report -n production
```

| Flag | Description |
|------|-------------|
| `-n, --namespace` | Scope to a single namespace |

## workloads

List all workloads with waste metrics, or show detail for a specific workload.

```bash
gerty workloads
gerty workloads production/deployment/api-gateway
```

The positional argument accepts `namespace/kind/name` format.

## recommend

Show right-sizing recommendations.

```bash
gerty recommend
gerty recommend -n production
gerty recommend --risk HIGH
```

| Flag | Description |
|------|-------------|
| `-n, --namespace` | Scope to a single namespace |
| `--risk` | Filter by risk level (`LOW`, `MEDIUM`, `HIGH`) |

## pr

Create a Pull Request (GitHub) or Merge Request (GitLab) in your GitOps repository with right-sizing recommendations.

```bash
gerty pr deployment/api-gateway
gerty pr deployment/api-gateway -n production --dry-run
```

| Flag | Description |
|------|-------------|
| `-n, --namespace` | Target namespace |
| `--branch-prefix` | PR/MR branch prefix |
| `--dry-run` | Show what would be changed without creating a PR/MR |

Gerty auto-detects your Git provider from the repository URL. GitHub and GitLab (including self-hosted) are supported. Configure access via Helm values:

```yaml
gitops:
  provider: github   # or gitlab
  token: ""          # PAT or deploy token (or reference a Secret)
```

The PR/MR description uses a built-in template with resource changes, metrics, and risk assessment. Customise it with `gitops.prTemplate` in your values file. See [Configuration](/configuration#pr-template) for available template fields.

## annotate

Add GitOps source annotations to a Kubernetes resource, linking it to its manifest in a Git repository.

```bash
gerty annotate deployment/api -n prod \
  --repo https://github.com/acme/k8s \
  --path apps/api/deployment.yaml
```

| Flag | Description |
|------|-------------|
| `-n, --namespace` | Target namespace |
| `--repo` | Git repository URL (required) |
| `--path` | Path to manifest in repo (required) |
| `--field` | Specific field path |
| `--apply` | Apply annotations immediately via kubectl |

## discover

Auto-discover workloads managed by ArgoCD or Flux and display their GitOps mappings.

```bash
gerty discover
gerty discover -o json
```

| Flag | Description |
|------|-------------|
| `-o, --output` | Output format (`text` or `json`) |

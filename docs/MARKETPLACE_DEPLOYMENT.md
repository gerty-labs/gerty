# k8s-sage — Cloud Marketplace Deployment Guide

Technical considerations and code changes required to list k8s-sage on AWS Marketplace, Azure Marketplace, and GCP Marketplace as a Kubernetes-native container product.

---

## Shared Requirements (All Marketplaces)

These apply universally regardless of platform.

### Container Image Hygiene

All three marketplaces scan images for CVEs during certification. Prepare for this now rather than at submission time.

- Pin all base images to digest, not tag (e.g., `golang:1.22@sha256:abc...`, not `golang:latest`)
- Run as non-root user in all containers (`sage-agent`, `sage-server`, `sage-slm`)
- No hardcoded secrets, passwords, or credentials anywhere in image layers
- No deprecated or end-of-life packages — run `trivy image` or `grype` locally before submission
- Multi-arch builds recommended (amd64 + arm64) — AWS and GCP both support Graviton/Arm nodes
- Ensure `HEALTHCHECK` or liveness/readiness probes are defined in the Helm chart (already present via `/healthz`)

```dockerfile
# Example: enforce non-root in sage-agent
FROM golang:1.22-alpine AS builder
# ... build steps ...

FROM alpine:3.19
RUN addgroup -S sage && adduser -S sage -G sage
COPY --from=builder /app/sage-agent /usr/local/bin/sage-agent
USER sage
ENTRYPOINT ["sage-agent"]
```

### Helm Chart Hardening

All three platforms deploy via Helm (or Helm-derived packaging). These adjustments ensure compatibility.

- Remove any Helm hooks (`pre-install`, `post-install`, etc.) — AWS EKS add-on framework does not support them, and they complicate CNAB packaging on Azure
- All dependent charts must be vendored locally (`file://` references, no remote repository pulls during install)
- Chart must pass `helm lint` and `helm template` with zero errors and zero warnings
- All images must be referenced via a single configurable registry prefix (marketplaces replace your registry with their own at publish time)
- Use `{{ .Release.Name }}` and `{{ .Release.Namespace }}` only — avoid `.Capabilities.APIVersions` for non-built-in APIs
- Ensure all CRDs (if any) are in `crds/` directory, not installed via hooks

```yaml
# values.yaml — registry abstraction for marketplace image replacement
global:
  imageRegistry: ""  # Overridden by marketplace at deploy time
  imagePullSecrets: []

agent:
  image:
    repository: "k8s-sage/sage-agent"
    tag: "v0.1.0"
    pullPolicy: IfNotPresent

server:
  image:
    repository: "k8s-sage/sage-server"
    tag: "v0.1.0"
    pullPolicy: IfNotPresent

slm:
  enabled: false
  image:
    repository: "k8s-sage/sage-slm"
    tag: "v0.1.0"
    pullPolicy: IfNotPresent
```

All image references in templates should use the global registry prefix:

```yaml
# templates/agent-daemonset.yaml
image: "{{ .Values.global.imageRegistry }}{{ .Values.agent.image.repository }}:{{ .Values.agent.image.tag }}"
```

### Licensing & Pricing Model

For initial listing, BYOL (Bring Your Own License) or Free is the simplest path on all three platforms. It avoids metering/billing integration entirely and gets you listed fastest.

- **Free tier**: L1-only (rules engine, no SLM) — open source, no billing integration needed
- **Paid tier**: L1 + L2 (with SLM) — BYOL initially, consider marketplace-native billing later
- Marketplace-native metering (per-node, per-cluster) requires platform-specific API integration — defer this to v2

### Semantic Versioning

All platforms require SemVer 2.0 (`MAJOR.MINOR.PATCH`). Tag images and Helm chart consistently:

```bash
# Image tags
k8s-sage/sage-agent:1.0.0
k8s-sage/sage-server:1.0.0
k8s-sage/sage-slm:1.0.0

# Helm chart version in Chart.yaml
version: 1.0.0
appVersion: "1.0.0"
```

---

## AWS Marketplace

### Listing Type

Container product with Helm chart delivery option, targeting EKS and any self-managed Kubernetes cluster.

### Image Registry

AWS copies your images into an AWS-owned ECR registry. Buyers pull from ECR, not from your registry. This means:

- All image references in the Helm chart must be parameterised (see shared section above)
- AWS provides a mapping file at publish time — your CI should support overriding image URIs
- Images must not reference external registries at runtime (init containers, sidecars, etc.)

### Helm Chart Constraints (EKS Add-on Framework)

If listing as an EKS add-on (recommended for discoverability), additional constraints apply beyond standard Helm:

- No `helm.sh/hook` annotations — the EKS add-on framework does not support hooks
- No `lookup` function in templates
- Only `Release.Name` and `Release.Namespace` built-in objects — no `.Capabilities.APIVersions` for custom APIs
- All sub-charts must use `file://` repository paths (no remote chart dependencies)
- Chart must pass with Helm v3.19.0+

Validation:

```bash
helm lint deploy/helm/k8s-sage/
helm template k8s-sage deploy/helm/k8s-sage/ --set slm.enabled=true
```

### RBAC

The existing ClusterRole is minimal and marketplace-compatible. No changes needed — read-only access to pods, nodes, and metrics is standard for observability tooling.

### Quick Launch Deprecation

As of March 1, 2026, AWS has discontinued Quick Launch for Helm chart deployments on EKS. Buyers still deploy via standard `helm install` or the EKS add-on mechanism. No code change needed, but deployment documentation should reflect this — do not reference Quick Launch in listing materials.

### Pricing Integration (If Not BYOL)

For usage-based pricing, integrate with the AWS Marketplace Metering API:

- `RegisterUsage` — called once at container startup to confirm entitlement
- `MeterUsage` — called periodically to report consumption (e.g., number of nodes monitored)

This would live as a sidecar or init step in `sage-server`. Defer unless pursuing paid listing from day one.

### Checklist

```
[ ] Images build and run as non-root
[ ] Images pass Trivy/Grype scan with no critical/high CVEs
[ ] Helm chart passes lint and template with zero errors
[ ] No Helm hooks in any templates
[ ] All image refs use global registry prefix
[ ] All sub-charts vendored locally
[ ] AWS seller account created, tax forms submitted
[ ] Foundational Technical Review (FTR) completed
[ ] Listing metadata prepared (description, logo, screenshots, usage docs)
```

---

## Azure Marketplace

### Listing Type

Container offer for Kubernetes Applications, deployed as an AKS cluster extension.

### Key Difference: CNAB Packaging

Azure requires your application to be packaged as a Cloud Native Application Bundle (CNAB). This is the main additional work compared to AWS/GCP.

The CNAB bundles your Helm chart, container images, and an ARM template into a single deployable artifact. The process:

1. Push all container images to a private Azure Container Registry (ACR)
2. Grant the marketplace `AcrPull` permission on your ACR
3. Create the CNAB bundle using Microsoft's bundling tool
4. Push the CNAB to your ACR
5. Reference it in Partner Center during offer creation

```bash
# Pull the CNAB bundler
docker pull mcr.microsoft.com/container-package-app:latest

# Validate bundle artifacts
docker run -v $(pwd)/bundle:/data \
  mcr.microsoft.com/container-package-app:latest \
  /bin/bash -c "cpa verify"

# Build and push CNAB
docker run -v $(pwd)/bundle:/data \
  mcr.microsoft.com/container-package-app:latest \
  /bin/bash -c "cpa buildbundle"
```

### CNAB Artifact Structure

Create a bundle directory alongside your Helm chart:

```
bundle/
├── manifest.yaml          # CNAB manifest (image refs, version, metadata)
├── createUiDefinition.json  # Azure portal deployment wizard UI
├── mainTemplate.json      # ARM template for cluster extension deployment
└── helm/
    └── k8s-sage/          # Your existing Helm chart (copied or symlinked)
```

The `createUiDefinition.json` defines what the Azure portal shows buyers when they click "Create". For k8s-sage, this would include:

- Cluster selection (existing AKS cluster)
- Namespace
- Whether to enable SLM (L2)
- Agent memory/retention configuration

### Cluster Extension Type

Azure deploys K8s marketplace apps as cluster extensions. You need to define a unique extension type name:

```
Format: PublisherName.ApplicationName
Example: k8ssage.k8s-sage
```

This value cannot be changed after publishing to preview.

### Security Scanning

Azure uses Microsoft Defender for Cloud to scan container images. Requirements:

- CVSS 3.0 score must be under 7.0 for all vulnerabilities
- Enable Defender on your ACR before submission to pre-check
- No end-of-life base images or packages

```bash
# Pre-check locally
trivy image --severity HIGH,CRITICAL k8s-sage/sage-agent:1.0.0
trivy image --severity HIGH,CRITICAL k8s-sage/sage-server:1.0.0
```

### Pricing Models

Azure supports several K8s-native billing models out of the box:

- Free
- BYOL
- Per core in cluster
- Per node in cluster
- Per cluster
- Per pod
- Custom meters

For k8s-sage, **per node** or **per cluster** maps most naturally to the value model (waste identified scales with cluster size). BYOL is simplest for initial listing.

### Checklist

```
[ ] Azure Container Registry created and images pushed
[ ] AcrPull permission granted to marketplace service principal
[ ] CNAB bundle created and validated
[ ] createUiDefinition.json authored for Azure portal wizard
[ ] mainTemplate.json (ARM template) authored
[ ] Cluster extension type name chosen and registered
[ ] Images pass Defender scan (CVSS < 7.0)
[ ] Partner Center account created
[ ] Listing metadata and marketing collateral prepared
[ ] Certification review submitted (~2 days)
```

---

## GCP Marketplace

### Listing Type

Kubernetes app for GKE and GKE Enterprise.

### Key Difference: Deployer Image

GCP requires a **deployer container image** that handles UI-based installation. This is a container that runs the Helm/kubectl deployment when a buyer clicks "Deploy" in the Cloud Marketplace console.

Google provides a base deployer image you extend:

```dockerfile
FROM gcr.io/cloud-marketplace-tools/k8s/deployer_helm:latest

COPY chart/k8s-sage /data/chart/k8s-sage
COPY schema.yaml /data/

# The deployer image runs helm install with parameters from the schema
```

### Application Custom Resource

GCP requires an `Application` CR (from the Kubernetes Application SIG) that aggregates all resources. Add this to your Helm chart:

```yaml
# templates/application.yaml
apiVersion: app.kubernetes.io/v1beta1
kind: Application
metadata:
  name: {{ .Release.Name }}
  namespace: {{ .Release.Namespace }}
  labels:
    app.kubernetes.io/name: {{ .Release.Name }}
  annotations:
    marketplace.cloud.google.com/deploy-info: '{"partner_id": "k8s-sage", "product_id": "k8s-sage"}'
spec:
  descriptor:
    type: k8s-sage
    version: "{{ .Chart.AppVersion }}"
    description: "Kubernetes resource right-sizing with rules engine and optional SLM"
    links:
      - description: Documentation
        url: https://github.com/yourusername/k8s-sage
    notes: |
      k8s-sage is deployed. Run `sage report` to see cluster-wide recommendations.
  selector:
    matchLabels:
      app.kubernetes.io/name: {{ .Release.Name }}
  componentKinds:
    - group: apps
      kind: DaemonSet
    - group: apps
      kind: Deployment
    - group: ""
      kind: Service
    - group: ""
      kind: ServiceAccount
    - group: rbac.authorization.k8s.io
      kind: ClusterRole
    - group: rbac.authorization.k8s.io
      kind: ClusterRoleBinding
```

### Schema File

The deployer uses a `schema.yaml` to define configurable parameters shown in the GCP console:

```yaml
# schema.yaml
x-google-marketplace:
  schemaVersion: v2
  applicationApiVersion: v1beta1
  publishedVersion: "1.0.0"
  publishedVersionMetadata:
    releaseNote: "Initial release"
  images:
    sage-agent:
      properties:
        agent.image.repository:
          type: REPO_WITH_REGISTRY
        agent.image.tag:
          type: TAG
    sage-server:
      properties:
        server.image.repository:
          type: REPO_WITH_REGISTRY
        server.image.tag:
          type: TAG

properties:
  name:
    type: string
    x-google-marketplace:
      type: NAME
  namespace:
    type: string
    x-google-marketplace:
      type: NAMESPACE
  slm.enabled:
    type: boolean
    title: Enable AI-powered recommendations (SLM)
    description: Deploys the k8s-sage SLM for enhanced analysis. Requires ~2.5Gi additional memory.
    default: false
  agent.resources.requests.memory:
    type: string
    title: Agent memory request
    description: Memory request per agent pod (increase for dense nodes >300 pods)
    default: "50Mi"

required:
  - name
  - namespace
```

### Public Git Repository

GCP requires a public Git repository containing:

- `LICENSE` file
- Helm chart or Kubernetes manifests
- Application CR
- Deployment documentation

If the main repo is private, create a separate public repo for the marketplace-specific packaging:

```
k8s-sage-marketplace/
├── LICENSE
├── README.md
├── chart/
│   └── k8s-sage/        # Marketplace-ready Helm chart
├── deployer/
│   ├── Dockerfile        # Deployer image
│   └── schema.yaml       # GCP schema
└── scripts/
    └── verify.sh         # Pre-submission verification
```

### Billing Agent (Paid Listings Only)

For commercial (non-BYOL) listings, GCP requires a billing agent sidecar that reports usage. This runs alongside `sage-server`:

```yaml
# Billing agent sidecar in sage-server deployment (paid tier only)
- name: ubbagent
  image: gcr.io/cloud-marketplace-tools/metering/ubbagent
  env:
    - name: AGENT_CONFIG_FILE
      value: /etc/ubbagent/config.yaml
    - name: AGENT_STATE_DIR
      value: /var/lib/ubbagent
    - name: AGENT_REPORT_DIR
      value: /var/lib/ubbagent/reports
  volumeMounts:
    - name: ubbagent-config
      mountPath: /etc/ubbagent
    - name: ubbagent-state
      mountPath: /var/lib/ubbagent
```

Defer this entirely for BYOL/free listings.

### Image Annotations (Required Since January 2025)

All container images must include a manifest annotation identifying the product:

```bash
# When building images, add the annotation
docker buildx build \
  --annotation "com.google.cloud.marketplace.product=k8s-sage" \
  -t k8s-sage/sage-agent:1.0.0 .
```

### Verification

Use Google's `mpdev` tool to validate before submission:

```bash
# Install mpdev
docker pull gcr.io/cloud-marketplace-tools/k8s/mpdev

# Verify the application installs and uninstalls cleanly
mpdev verify \
  --deployer=gcr.io/your-project/k8s-sage/deployer:1.0.0
```

### Checklist

```
[ ] Public Git repo created with LICENSE, chart, and deployer
[ ] Application CR added to Helm chart
[ ] Deployer image built and tested
[ ] schema.yaml defines all configurable parameters
[ ] Image annotations added (marketplace product identifier)
[ ] Images pushed to GCR/Artifact Registry
[ ] mpdev verify passes (install + uninstall)
[ ] Partner Advantage joined, Build partner status achieved
[ ] Marketplace Vendor Agreement signed
[ ] Producer Portal access configured
[ ] Listing metadata and documentation submitted
```

---

## Platform Comparison Summary

| Consideration | AWS | Azure | GCP |
|---|---|---|---|
| Packaging format | Helm chart (direct) | CNAB bundle wrapping Helm | Deployer image + Helm |
| Image registry | Auto-copied to ECR | Push to ACR, copied by MS | Push to GCR/Artifact Registry |
| Extra artifacts needed | None | CNAB, ARM template, UI definition | Deployer Dockerfile, schema.yaml, Application CR |
| Billing integration (paid) | Metering API | Custom meters | Billing agent sidecar |
| Security scanning | FTR review | Defender (CVSS < 7.0) | Artifact Analysis |
| Certification timeline | 4-8 weeks end-to-end | ~2 days cert + 4-6 weeks total | Up to 2 weeks review |
| Seller fees | 3% (public), 1.5-3% (private) | ~3% | ~3% |
| Public repo required | No | No | Yes |
| Simplest BYOL/Free path | Easiest | Medium | Medium |

---

## Recommended Listing Order

1. **AWS Marketplace** — least additional work, largest K8s audience, Helm chart deploys natively
2. **Azure Marketplace** — CNAB packaging is extra work but AKS enterprise audience is valuable
3. **GCP Marketplace** — deployer image and Partner Advantage onboarding add overhead, but GKE audience is strong

For all three: start with a Free or BYOL listing for L1-only (rules engine). Add the paid SLM tier once the listing pipeline is proven and metering integration is built.

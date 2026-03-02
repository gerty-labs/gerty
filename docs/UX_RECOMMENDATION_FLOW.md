# k8s-sage — Recommendation Delivery UX

## Overview

k8s-sage operates as an **advisory-first** tool. Recommendations are delivered to engineers where they already work — Slack — with actionable buttons that integrate into existing GitOps workflows. The tool never silently modifies running workloads.

The goal: an engineer sees a recommendation, understands *why* in seconds, and can act on it without leaving the conversation.

---

## Slack Message Anatomy

Every recommendation is delivered as a structured Slack message to a configured channel (e.g. `#k8s-sage` or per-namespace channels).

```
┌──────────────────────────────────────────────────────────┐
│  💰 k8s-sage recommendation                              │
│                                                          │
│  payment-service │ prod │ 3 replicas                     │
│                                                          │
│  Memory request: 2Gi → 768Mi                             │
│  Estimated saving: ~£135/mo across replicas              │
│  Confidence: 0.85                                        │
│                                                          │
│  Reason: JVM detected (-Xmx512m). Container memory at   │
│  94% is expected GC behaviour — the JVM reserves the     │
│  full heap on startup. 768Mi provides 256Mi headroom     │
│  above heap max for metaspace, thread stacks, and OS     │
│  overhead.                                               │
│                                                          │
│  ┌──────────┐  ┌───────────┐  ┌─────────────┐           │
│  │ Create PR│  │ 🚨 Hotfix │  │ Acknowledge │           │
│  └──────────┘  └───────────┘  └─────────────┘           │
│                                                          │
│  L2 analysis • 01 Mar 2026 09:14 UTC                     │
└──────────────────────────────────────────────────────────┘
```

### Message Fields

| Field | Description |
|---|---|
| **Workload** | Deployment/StatefulSet name, namespace, replica count |
| **Change** | Current → recommended value, resource type (CPU/memory) |
| **Estimated saving** | Monthly cost impact across all replicas |
| **Confidence** | 0.0–1.0 score from the L2 model. Below 0.7, the message is flagged as "low confidence — review carefully" |
| **Reason** | Plain-english explanation from L2. This is the key differentiator — not just *what* to change, but *why* it's safe |
| **Source** | Whether the recommendation came from L1 (rules engine) or L2 (model analysis) |

---

## Button Actions

### Create PR

The default, recommended action. Opens a pull/merge request against the workload's source GitOps repository.

**What happens:**

1. Engineer clicks **Create PR**
2. Sage resolves the workload's source repo and file path (see [GitOps Discovery](#gitops-discovery) below)
3. A PR/MR is opened on a branch named `k8s-sage/<workload>-<short-hash>`
4. PR body contains:
   - The specific value change (e.g. `resources.requests.memory: 2Gi → 768Mi`)
   - Full L2 explanation
   - Confidence score
   - Link back to Sage dashboard with supporting metrics
5. Slack message updates with a link to the PR
6. ArgoCD/Flux picks up the merge through the normal sync pipeline

**PR title format:**
```
k8s-sage: reduce memory request for payment-service (prod) — 768Mi, confidence 0.85
```

**No drift.** The change flows through the same IaC pipeline as every other infrastructure change. Full audit trail in Git.

### 🚨 Hotfix

For active incidents only. Patches the running workload directly via the Kubernetes API, bypassing Git. **Upscale-only by default** — hotfix can only *increase* resource requests/limits, never reduce them. In an active incident, the answer is always "give it more." Downscale hotfix is available behind `hotfix.allowDownscale: true` for teams that explicitly want it.

**What happens:**

1. Engineer clicks **🚨 Hotfix**
2. Sage responds with a dry-run preview and confirmation prompt in-thread:
   ```
   ⚠️ Hotfix preview — payment-service (prod)

   memory request: 256Mi → 512Mi  (⬆ upscale)
   memory limit:   512Mi → 1Gi    (⬆ upscale)

   This will patch the Deployment directly in the cluster,
   bypassing GitOps. Use only for active incidents.

   A backfill PR will be opened automatically to prevent
   ArgoCD reverting this change on next sync.

   [Confirm hotfix]  [Cancel]
   ```
3. On confirm:
   - Sage patches the Deployment via Kubernetes API
   - A backfill PR is opened against the GitOps repo to align the source of truth
   - Slack message updates: "Hotfix applied. Backfill PR: [link]"
   - Sage monitors pod health for 60 seconds post-patch
4. **Automatic rollback:** If the workload's health checks fail within 60 seconds of the hotfix (pod still crash-looping, readiness probe failing), Sage automatically reverts the patch and notifies the channel: "Hotfix did not resolve the issue — reverted. Escalating to engineer."
5. If backfill PR is not merged, ArgoCD will detect drift and eventually revert — this is by design (forces the engineer to complete the loop)

**When to use:** OOM kills in a loop, sustained CPU throttling, pod crash loops — situations where waiting for a PR review cycle costs more than the process bypass.

**Why upscale-only:** There is no active incident scenario where reducing resources on a crashing service is the correct response. Restricting hotfix to upscaling eliminates the most dangerous footgun. Teams that need downscale hotfix for specific workflows can opt in via Helm values.

### Acknowledge

The engineer has seen the recommendation and will handle it on their own terms (or chooses not to act).

**What happens:**

1. Engineer clicks **Acknowledge**
2. Sage logs the recommendation as "seen, not actioned"
3. Slack message updates with "👀 Acknowledged by @engineer"
4. Recommendation stays visible in the Sage dashboard for future reference
5. If the same recommendation persists for a configurable period (default: 7 days), Sage sends a single follow-up reminder, then stops

**No nagging.** One reminder, then silence. Engineers trust tools that respect their judgement.

---

## GitOps Discovery

Sage needs to know *where* a workload's resource values live in Git in order to open PRs. This is resolved through a priority chain — Sage tries each method in order until it finds a match.

### Priority 1 — ArgoCD / Flux Auto-Detection (Zero Config)

If the cluster runs ArgoCD or Flux, every synced resource already carries metadata linking back to its source.

**ArgoCD:**
Sage queries the ArgoCD Application resource for the workload, which contains:
- `spec.source.repoURL` → Git repository
- `spec.source.path` → directory within the repo
- `spec.source.targetRevision` → branch
- Tracking annotations on the resource itself to map workload → Application

**Flux:**
Sage reads the Kustomization and GitRepository resources:
- `GitRepository.spec.url` → Git repository
- `Kustomization.spec.path` → directory within the repo
- `Kustomization.spec.sourceRef` → links to the GitRepository

From the repo and path, Sage scans for the relevant values file (Helm `values.yaml`, Kustomize patches, or raw manifests) and locates the specific resource field to modify.

**This covers the majority of teams.** If you're running GitOps, Sage just reads what's already there. No annotations, no config.

**Known limitation — Kustomize overlays:** When a team uses Kustomize with base manifests in one repo and overlay patches in another, auto-detection resolves to whichever repo/path the ArgoCD Application or Flux Kustomization points to (typically the overlay). If the team wants Sage to PR against the base repo instead, they should use explicit annotations. Auto-detection does not attempt to resolve across multiple layers — this is intentional to avoid surprising PRs against the wrong repo.

### Priority 2 — Explicit Annotations

For workloads not managed by ArgoCD/Flux, or where auto-detection resolves incorrectly, teams can annotate directly:

```yaml
metadata:
  annotations:
    sage.io/repo: "github.com/acme/k8s-manifests"
    sage.io/path: "apps/payment-service/values.yaml"
    sage.io/field: "resources.requests.memory"
```

This overrides any auto-detected source. Useful for:
- Workloads deployed by CI/CD pipelines without a GitOps controller
- Monorepos where path inference is ambiguous
- Teams that want explicit control over where PRs land

**CLI helper:**
```bash
sage annotate deployment/payment-service \
  --repo github.com/acme/k8s-manifests \
  --scan  # walks the repo, fuzzy-matches deployment to file path
```

The `--scan` flag searches the repo for files that define `payment-service` and suggests the correct `path` and `field` annotations. The engineer confirms before they're applied.

### Priority 3 — Global Config (Helm Values)

For teams with a single GitOps repo or a consistent repo structure:

```yaml
# k8s-sage Helm values
gitops:
  provider: github          # github | gitlab | bitbucket
  defaultRepo: "acme/k8s-manifests"
  defaultBranch: "main"
  basePath: "apps/"         # sage looks for apps/<workload-name>/
  auth:
    secretRef: sage-git-token
```

Sage convention-matches workload names to paths within the repo:
- `payment-service` → `apps/payment-service/values.yaml`
- `api-gateway` → `apps/api-gateway/values.yaml`

**Least precise, but covers the simple case.** Teams with one repo and consistent naming get PR support with three lines of config.

### When Discovery Fails

If Sage cannot resolve a workload's source repo through any method:

- The **Create PR** button is replaced with **Copy recommendation** (copies a YAML patch to clipboard)
- The Slack message includes a note: "💡 Annotate this workload to enable automatic PRs — `sage annotate deployment/payment-service --scan`"
- Recommendation is still fully visible and actionable — the engineer just applies it manually

**Sage never blocks the recommendation because it can't find the repo.** The GitOps integration is a convenience layer, not a gate.

---

## Message Grouping & Noise Control

### Batching

Sage runs analysis cycles every 5 minutes. Rather than sending one message per workload, recommendations from the same cycle are grouped:

```
┌──────────────────────────────────────────────────────────┐
│  💰 k8s-sage — 4 recommendations (prod)                  │
│  Estimated total saving: ~£380/mo                        │
│                                                          │
│  ▸ payment-service    memory 2Gi → 768Mi     conf: 0.85  │
│  ▸ api-gateway        cpu 1000m → 200m       conf: 0.91  │
│  ▸ user-service       memory 1Gi → 512Mi     conf: 0.78  │
│  ▸ notifications      cpu 500m → 100m        conf: 0.88  │
│                                                          │
│  Expand any workload for details and actions.             │
└──────────────────────────────────────────────────────────┘
```

Each workload row expands (Slack collapsible section) to show the full explanation and action buttons.

### Deduplication

Sage does not re-send recommendations that have already been delivered and are either:
- Pending in an open PR
- Acknowledged within the last 7 days
- Already applied (values match recommendation)

### Severity Tiers

| Tier | Trigger | Behaviour |
|---|---|---|
| **🔴 Critical** | Active OOM kills, crash loops, sustained throttling | Sent immediately, outside batch cycle. Engineer must still confirm hotfix — Sage never auto-applies changes without human approval. |
| **🟡 Optimisation** | Over-provisioned resources, cost savings | Batched per cycle, grouped by namespace |
| **🟢 Informational** | Minor tweaks, low confidence suggestions | Rolled into a daily or weekly digest (configurable) |

Engineers configure their noise tolerance:

```yaml
notifications:
  channel: "#k8s-sage"
  critical: immediate       # always
  optimisation: batched      # per-cycle or daily
  informational: weekly      # digest
  quietHours:
    enabled: true
    start: "22:00"
    end: "08:00"
    timezone: "Europe/London"
```

During quiet hours, only critical alerts are delivered. Everything else queues for the morning.

---

## Dashboard (Complementary)

Slack is the primary interaction surface, but a lightweight Grafana dashboard provides:

- **Recommendation history** — what was suggested, when, outcome (applied/ignored/dismissed)
- **Cluster savings tracker** — cumulative £ saved from applied recommendations
- **Confidence trends** — model accuracy over time as recommendations are validated
- **Pending recommendations** — anything not yet actioned, sortable by potential saving

The dashboard is read-only context. Engineers act in Slack, review trends in Grafana.

---

## User Journey Summary

```
                    ┌─────────────────────┐
                    │   Sage analyses      │
                    │   workloads every    │
                    │   5 minutes          │
                    └─────────┬───────────┘
                              │
                              ▼
                    ┌─────────────────────┐
                    │  Recommendation      │
                    │  delivered to Slack   │
                    └─────────┬───────────┘
                              │
                ┌─────────────┼─────────────┐
                ▼             ▼             ▼
         ┌────────────┐ ┌──────────┐ ┌─────────────┐
         │ Create PR  │ │ 🚨Hotfix │ │ Acknowledge │
         └─────┬──────┘ └────┬─────┘ └──────┬──────┘
               │             │              │
               ▼             ▼              ▼
        ┌────────────┐ ┌───────────┐ ┌───────────────┐
        │ PR opened  │ │ Confirm?  │ │ Logged, one   │
        │ against    │ │           │ │ reminder in   │
        │ GitOps     │ │ Yes → K8s │ │ 7 days, then  │
        │ repo       │ │ patched + │ │ silence       │
        │            │ │ backfill  │ │               │
        │ Engineer   │ │ PR opened │ └───────────────┘
        │ reviews &  │ │           │
        │ merges     │ └───────────┘
        │            │
        │ ArgoCD/    │
        │ Flux syncs │
        └────────────┘
```

**The engineer is always in control.** Sage advises, explains, and makes acting easy — but never decides for the team.

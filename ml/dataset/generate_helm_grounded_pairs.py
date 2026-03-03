#!/usr/bin/env python3
"""Generate Helm chart grounded training pairs for k8s-sage.

Each pair references REAL default resource values from popular Helm chart
values.yaml files. Provenance links point to the actual source files on GitHub.

Output: ml/dataset/raw/helm_grounded_pairs.jsonl
"""

import json
from pathlib import Path

SYSTEM_PROMPT = (
    "You are k8s-sage, a Kubernetes resource efficiency specialist. "
    "Analyse the provided workload metrics and give actionable right-sizing "
    "recommendations. Be specific about numbers, explain your reasoning, "
    "and flag risks."
)

OUTPUT_PATH = Path(__file__).parent / "raw" / "helm_grounded_pairs.jsonl"

# ── Raw GitHub URLs for provenance ────────────────────────────────────

URLS = {
    "kube-prometheus-stack": "https://raw.githubusercontent.com/prometheus-community/helm-charts/main/charts/kube-prometheus-stack/values.yaml",
    "prometheus": "https://raw.githubusercontent.com/prometheus-community/helm-charts/main/charts/prometheus/values.yaml",
    "redis": "https://raw.githubusercontent.com/bitnami/charts/main/bitnami/redis/values.yaml",
    "postgresql": "https://raw.githubusercontent.com/bitnami/charts/main/bitnami/postgresql/values.yaml",
    "mongodb": "https://raw.githubusercontent.com/bitnami/charts/main/bitnami/mongodb/values.yaml",
    "mysql": "https://raw.githubusercontent.com/bitnami/charts/main/bitnami/mysql/values.yaml",
    "kafka": "https://raw.githubusercontent.com/bitnami/charts/main/bitnami/kafka/values.yaml",
    "rabbitmq": "https://raw.githubusercontent.com/bitnami/charts/main/bitnami/rabbitmq/values.yaml",
    "elasticsearch": "https://raw.githubusercontent.com/bitnami/charts/main/bitnami/elasticsearch/values.yaml",
    "ingress-nginx": "https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/charts/ingress-nginx/values.yaml",
    "cert-manager": "https://raw.githubusercontent.com/cert-manager/cert-manager/master/deploy/charts/cert-manager/values.yaml",
    "argo-cd": "https://raw.githubusercontent.com/argoproj/argo-helm/main/charts/argo-cd/values.yaml",
    "grafana": "https://raw.githubusercontent.com/grafana/helm-charts/grafana-8.8.4/charts/grafana/values.yaml",
    "vault": "https://raw.githubusercontent.com/hashicorp/vault-helm/main/values.yaml",
    "istio": "https://raw.githubusercontent.com/istio/istio/master/manifests/charts/istio-control/istio-discovery/values.yaml",
    "linkerd": "https://raw.githubusercontent.com/linkerd/linkerd2/main/charts/linkerd-control-plane/values.yaml",
    "traefik": "https://raw.githubusercontent.com/traefik/traefik-helm-chart/master/traefik/values.yaml",
    "jenkins": "https://raw.githubusercontent.com/jenkinsci/helm-charts/main/charts/jenkins/values.yaml",
    "airflow": "https://raw.githubusercontent.com/apache/airflow/main/chart/values.yaml",
    "spark-operator": "https://raw.githubusercontent.com/GoogleCloudPlatform/spark-on-k8s-operator/master/charts/spark-operator-chart/values.yaml",
}


def make_pair(
    id_str: str, user: str, assistant: str, category: str,
    provenance: str, **extra_meta,
) -> dict:
    """Create a validated training pair."""
    assert len(assistant) >= 50, f"{id_str}: assistant too short ({len(assistant)} chars)"
    assert category in ("right-sizing", "classification", "runtime-specific", "edge-case")
    meta = {"category": category, "provenance": provenance}
    meta.update(extra_meta)
    return {
        "id": id_str,
        "source": "expert",
        "system": SYSTEM_PROMPT,
        "user": user,
        "assistant": assistant,
        "metadata": meta,
    }


# ── kube-prometheus-stack (7 pairs) ──────────────────────────────────


def kube_prometheus_stack_pairs() -> list[dict]:
    url = URLS["kube-prometheus-stack"]
    pairs = []

    pairs.append(make_pair(
        "helm-kube-prometheus-stack-right-sizing-001",
        "What are the default resource settings in the kube-prometheus-stack Helm chart?",
        "## kube-prometheus-stack Default Resource Settings\n\n"
        "The kube-prometheus-stack chart ships with the following default resource "
        "configurations across its components:\n\n"
        "**Prometheus Server (prometheusSpec):**\n"
        "- requests: not set (empty `{}`)\n"
        "- limits: not set\n"
        "- The chart relies on you setting `prometheusSpec.resources` explicitly.\n\n"
        "**Alertmanager:**\n"
        "- requests: not set (empty `{}`)\n"
        "- limits: not set\n\n"
        "**Prometheus Operator:**\n"
        "- requests: not set (empty `{}`)\n"
        "- limits: not set\n\n"
        "**kube-state-metrics (subchart):**\n"
        "- requests: not set by default\n"
        "- limits: not set by default\n\n"
        "**prometheus-node-exporter (subchart):**\n"
        "- requests: not set by default\n"
        "- limits: not set by default\n\n"
        "**Grafana (subchart):**\n"
        "- Uses the grafana subchart defaults which are typically empty `{}`.\n\n"
        "**Thanos Ruler (if enabled):**\n"
        "- requests: not set\n"
        "- limits: not set\n\n"
        "In summary, kube-prometheus-stack sets virtually no resource requests or "
        "limits by default. Every component ships with `resources: {}`. This means "
        "all pods run as BestEffort QoS and will be evicted first under node pressure. "
        "For production, you must configure resources for every component explicitly.",
        "right-sizing", url,
    ))

    pairs.append(make_pair(
        "helm-kube-prometheus-stack-right-sizing-002",
        "Are the default kube-prometheus-stack resources appropriate for production? "
        "We have a 50-node cluster with 2000 pods.",
        "## Production Assessment: kube-prometheus-stack\n\n"
        "**No. The defaults are not production-ready.** All components ship with "
        "`resources: {}`, meaning BestEffort QoS. For a 50-node, 2000-pod cluster, "
        "here is what you actually need:\n\n"
        "**Prometheus Server:**\n"
        "A 50-node cluster with 2000 pods typically generates 1-2 million active time "
        "series. Prometheus memory scales at approximately 4KB per series in the TSDB "
        "head block.\n"
        "- 1.5M series * 4KB = ~6Gi baseline\n"
        "- Add 30% for queries, WAL, and compaction headroom\n"
        "- Recommended: requests 2000m CPU / 8Gi memory, limits 4000m / 12Gi\n\n"
        "**Alertmanager:**\n"
        "Alertmanager is lightweight unless you have thousands of active alerts.\n"
        "- Recommended: requests 100m / 128Mi, limits 500m / 256Mi\n\n"
        "**Prometheus Operator:**\n"
        "Watches CRDs (ServiceMonitor, PodMonitor, PrometheusRule). With 50 nodes:\n"
        "- Recommended: requests 100m / 256Mi, limits 500m / 512Mi\n\n"
        "**kube-state-metrics:**\n"
        "Memory scales with object count. 2000 pods + associated objects:\n"
        "- Recommended: requests 100m / 256Mi, limits 500m / 512Mi\n\n"
        "**node-exporter (DaemonSet, 50 pods):**\n"
        "Per-node overhead is small but adds up across the fleet.\n"
        "- Recommended: requests 50m / 64Mi, limits 200m / 128Mi per pod\n"
        "- Fleet total: 2500m / 3.2Gi requests\n\n"
        "**Total cluster overhead for kube-prometheus-stack (production-sized):**\n"
        "- Requests: ~5000m CPU, ~12Gi memory (excluding node-exporter fleet)\n"
        "- With node-exporter: ~7500m CPU, ~15Gi memory\n\n"
        "The monitoring stack itself consumes meaningful resources. Factor this into "
        "cluster capacity planning.",
        "right-sizing", url,
    ))

    pairs.append(make_pair(
        "helm-kube-prometheus-stack-right-sizing-003",
        "My Prometheus server from kube-prometheus-stack is OOMKilled. It was running "
        "with default resources (no requests/limits set). The cluster has 100 nodes.",
        "## Prometheus OOMKill with Default Resources\n\n"
        "**Root cause:** kube-prometheus-stack ships with `prometheusSpec.resources: {}` "
        "(no requests, no limits). Without a memory limit, Prometheus grows until the "
        "node runs out of memory or the kubelet evicts it.\n\n"
        "**Why it OOMKilled on a 100-node cluster:**\n"
        "- 100 nodes with default scrape targets generates 3-5 million active series\n"
        "- At 4KB per series: 12-20Gi just for TSDB head block\n"
        "- Plus WAL replay on restart, compaction memory, query allocations\n"
        "- Total memory demand: 15-25Gi\n\n"
        "**Immediate fix -- set resources in values.yaml:**\n"
        "```yaml\n"
        "prometheus:\n"
        "  prometheusSpec:\n"
        "    resources:\n"
        "      requests:\n"
        "        cpu: 4000m\n"
        "        memory: 16Gi\n"
        "      limits:\n"
        "        cpu: 8000m\n"
        "        memory: 24Gi\n"
        "```\n\n"
        "**Why the limit needs headroom above request:**\n"
        "- TSDB compaction temporarily doubles memory usage (reads old block + writes "
        "new block)\n"
        "- Large PromQL queries allocate per-query buffers\n"
        "- WAL replay on restart loads all samples since last checkpoint\n\n"
        "**Longer-term:** If Prometheus needs >16Gi, consider:\n"
        "1. Reducing cardinality (drop high-cardinality labels via relabeling)\n"
        "2. Shorter retention (default 15d, reduce to 7d halves memory)\n"
        "3. Remote write to Thanos/Mimir/VictoriaMetrics for long-term storage\n"
        "4. Federation or sharding to split the scrape load\n\n"
        "**Never run Prometheus as BestEffort in production.** An OOMKilled Prometheus "
        "loses WAL data and can take 10-30 minutes to replay on restart, creating a "
        "monitoring blackout.",
        "right-sizing", url,
    ))

    pairs.append(make_pair(
        "helm-kube-prometheus-stack-classification-004",
        "How should I classify the different kube-prometheus-stack components for "
        "right-sizing purposes?",
        "## Workload Classification: kube-prometheus-stack Components\n\n"
        "Each component has a distinct resource pattern:\n\n"
        "**Prometheus Server -- STEADY with compaction spikes:**\n"
        "- CPU: Steady scraping load (proportional to series count and scrape interval) "
        "with periodic spikes every 2 hours during TSDB compaction.\n"
        "- Memory: Grows gradually as new series appear, drops during compaction. "
        "Very stable once series count stabilises.\n"
        "- Classification: Steady. Use P95 for requests, Max * 1.25 for limits.\n\n"
        "**Alertmanager -- STEADY (very low utilisation):**\n"
        "- Processes alerts and sends notifications. CPU/memory essentially flat.\n"
        "- Unless you have alert storms (thousands of alerts firing), this is idle.\n"
        "- Classification: Steady. Safe to right-size aggressively.\n\n"
        "**Prometheus Operator -- BURSTABLE:**\n"
        "- Idle most of the time. Spikes when CRDs change (ServiceMonitor updates, "
        "PrometheusRule changes) as it reconciles and regenerates Prometheus config.\n"
        "- Classification: Burstable. Low request, moderate limit.\n\n"
        "**kube-state-metrics -- STEADY:**\n"
        "- Reflects K8s API objects as Prometheus metrics. CPU/memory proportional "
        "to object count, very predictable.\n"
        "- Classification: Steady. Right-size based on cluster object count.\n\n"
        "**node-exporter -- STEADY (per-node DaemonSet):**\n"
        "- Exposes host metrics. Fixed resource footprint per node.\n"
        "- Classification: Steady. 50m/64Mi request is typical.\n\n"
        "**Grafana -- BURSTABLE:**\n"
        "- Idle when nobody is viewing dashboards. Spikes with concurrent viewers "
        "and complex queries.\n"
        "- Classification: Burstable. Set request for idle + 1-2 users, limit for "
        "peak concurrent users.",
        "classification", url,
    ))

    pairs.append(make_pair(
        "helm-kube-prometheus-stack-right-sizing-005",
        "How should I size kube-prometheus-stack for a large cluster with 500 nodes "
        "and 20,000 pods?",
        "## Sizing kube-prometheus-stack for 500 Nodes / 20K Pods\n\n"
        "At this scale, the monitoring stack itself becomes a significant workload.\n\n"
        "**Expected time series count:** 10-20 million active series.\n"
        "- Each pod generates ~100-200 metrics\n"
        "- Each node generates ~500 metrics (node-exporter)\n"
        "- kube-state-metrics generates ~50 metrics per object\n"
        "- ServiceMonitors for application metrics add more\n\n"
        "**Prometheus Server (likely needs sharding):**\n"
        "- 15M series * 4KB = 60Gi baseline memory\n"
        "- Single Prometheus cannot realistically handle this\n"
        "- Use Prometheus sharding via `prometheusSpec.shards: 4`\n"
        "- Per-shard: requests 4000m / 16Gi, limits 8000m / 24Gi\n"
        "- Total Prometheus: 16,000m / 64Gi requests\n\n"
        "**Alertmanager (3 replicas for HA):**\n"
        "- requests: 200m / 256Mi per replica\n"
        "- Total: 600m / 768Mi\n\n"
        "**Prometheus Operator:**\n"
        "- At 500 nodes, many CRDs to watch\n"
        "- requests: 500m / 512Mi, limits 1000m / 1Gi\n\n"
        "**kube-state-metrics:**\n"
        "- 20K pods + deployments + services + nodes = 50K+ objects\n"
        "- requests: 500m / 1Gi, limits 1000m / 2Gi\n"
        "- Consider kube-state-metrics sharding for >30K objects\n\n"
        "**node-exporter (500 pods):**\n"
        "- Per pod: 50m / 64Mi\n"
        "- Fleet total: 25,000m / 32Gi\n\n"
        "**Total monitoring overhead:**\n"
        "- Requests: ~42,600m CPU, ~98Gi memory\n"
        "- This is equivalent to ~5-10 nodes dedicated to monitoring\n"
        "- At 500 nodes, that is 1-2% of cluster capacity for monitoring\n\n"
        "**Alternative:** At this scale, consider Thanos or Mimir with object "
        "storage backend. They separate ingestion from storage and query, "
        "allowing independent scaling of each concern.",
        "right-sizing", url,
    ))

    pairs.append(make_pair(
        "helm-kube-prometheus-stack-runtime-specific-006",
        "What is the total resource overhead of deploying kube-prometheus-stack "
        "with default values on a 20-node cluster?",
        "## Resource Overhead: kube-prometheus-stack on 20 Nodes\n\n"
        "**With default values (all `resources: {}`), the actual resource consumption "
        "will be:**\n\n"
        "The chart sets no requests or limits by default, so pods run as BestEffort. "
        "But they still consume actual resources:\n\n"
        "**Prometheus Server (1 replica):**\n"
        "- Actual usage with 20 nodes: ~500m CPU, ~3-4Gi memory\n"
        "- 20 nodes typically produce ~500K series\n\n"
        "**Alertmanager (1 replica):**\n"
        "- Actual usage: ~10m CPU, ~50Mi memory\n\n"
        "**Prometheus Operator (1 replica):**\n"
        "- Actual usage: ~20m CPU, ~100Mi memory\n\n"
        "**kube-state-metrics (1 replica):**\n"
        "- Actual usage: ~50m CPU, ~100Mi memory\n\n"
        "**node-exporter (20 pods, DaemonSet):**\n"
        "- Actual usage per pod: ~10m CPU, ~30Mi memory\n"
        "- Fleet total: ~200m CPU, ~600Mi memory\n\n"
        "**Grafana (1 replica, if enabled):**\n"
        "- Actual usage: ~50m CPU, ~150Mi memory\n\n"
        "**Total actual consumption:**\n"
        "- CPU: ~830m\n"
        "- Memory: ~4-5Gi\n\n"
        "**The problem is not the actual usage -- it is the QoS class.** With no "
        "requests set, all monitoring pods are BestEffort. Under node memory pressure, "
        "the kubelet evicts BestEffort pods first. Your monitoring stack goes down "
        "exactly when you need it most -- during a resource crisis.\n\n"
        "**Minimum production configuration:** Set at least memory requests on "
        "Prometheus, Alertmanager, and kube-state-metrics to ensure Burstable QoS.",
        "runtime-specific", url,
    ))

    pairs.append(make_pair(
        "helm-kube-prometheus-stack-edge-case-007",
        "We upgraded kube-prometheus-stack and Prometheus memory doubled overnight. "
        "No new ServiceMonitors were added. What happened?",
        "## Edge Case: Memory Doubling After kube-prometheus-stack Upgrade\n\n"
        "**Most likely cause: new default scrape targets or metric relabeling changes "
        "in the upgraded chart version.**\n\n"
        "kube-prometheus-stack bundles default ServiceMonitors, PodMonitors, and "
        "PrometheusRules. Chart upgrades frequently add new scrape targets:\n\n"
        "**Common culprits after upgrade:**\n"
        "1. **New ServiceMonitors enabled by default**: The chart may enable scraping "
        "of additional kube-system components (etcd, kube-scheduler, kube-controller-manager) "
        "that were previously disabled.\n"
        "2. **Changed metric relabeling**: If `metricRelabelings` that were dropping "
        "high-cardinality metrics got reset, series count explodes.\n"
        "3. **kube-state-metrics major version bump**: New KSM versions expose "
        "additional metrics by default (e.g., pod labels as metrics).\n"
        "4. **Grafana sidecar**: The chart may now enable a sidecar that watches "
        "ConfigMaps for dashboards, adding memory overhead.\n\n"
        "**How to diagnose:**\n"
        "```promql\n"
        "# Check series count before and after\n"
        "prometheus_tsdb_head_series\n\n"
        "# Find which jobs added the most series\n"
        "topk(10, count by (job) ({__name__=~\".+\"}))\n"
        "```\n\n"
        "**How to fix:**\n"
        "1. Compare `values.yaml` between chart versions for new `enabled: true` flags\n"
        "2. Add explicit metric relabeling to drop unwanted series:\n"
        "```yaml\n"
        "prometheus:\n"
        "  prometheusSpec:\n"
        "    additionalScrapeConfigs: []\n"
        "    # Pin enabled ServiceMonitors explicitly\n"
        "```\n"
        "3. Disable metrics you do not need in kube-state-metrics:\n"
        "```yaml\n"
        "kube-state-metrics:\n"
        "  metricLabelsAllowlist: []  # Prevent pod label metrics\n"
        "```\n\n"
        "**Prevention:** Always diff the chart values between versions before upgrading. "
        "Pin resource requests to accommodate the expected series count after upgrade.",
        "edge-case", url,
    ))

    return pairs


# ── prometheus (5 pairs) ─────────────────────────────────────────────


def prometheus_pairs() -> list[dict]:
    url = URLS["prometheus"]
    pairs = []

    pairs.append(make_pair(
        "helm-prometheus-right-sizing-001",
        "What are the default resource settings in the standalone Prometheus Helm chart?",
        "## Standalone Prometheus Chart Default Resources\n\n"
        "The prometheus-community/prometheus chart (not kube-prometheus-stack) has "
        "the following defaults:\n\n"
        "**Prometheus Server:**\n"
        "- requests: not set (empty `{}`)\n"
        "- limits: not set\n"
        "- Runs as a Deployment or StatefulSet (configurable)\n\n"
        "**Alertmanager:**\n"
        "- requests: not set\n"
        "- limits: not set\n"
        "- Disabled by default in newer chart versions\n\n"
        "**Pushgateway:**\n"
        "- requests: not set\n"
        "- limits: not set\n"
        "- Disabled by default in newer chart versions\n\n"
        "**configmap-reload sidecar:**\n"
        "- requests: not set\n"
        "- limits: not set\n"
        "- Very lightweight Go binary that watches for ConfigMap changes\n\n"
        "Like kube-prometheus-stack, the standalone chart ships with no resource "
        "defaults. The philosophy is that resource needs vary too widely between "
        "deployments (10-node dev cluster vs 500-node production) to set meaningful "
        "defaults. You must set resources explicitly based on your cluster size.",
        "right-sizing", url,
    ))

    pairs.append(make_pair(
        "helm-prometheus-right-sizing-002",
        "Are the standalone Prometheus chart defaults suitable for a 30-node production cluster?",
        "## Production Assessment: Standalone Prometheus Chart\n\n"
        "**No.** The chart ships with `resources: {}` for all components. For a "
        "30-node cluster:\n\n"
        "**Prometheus Server sizing:**\n"
        "- Expected series: 700K-1.5M (30 nodes, typical workloads)\n"
        "- Memory need: 3-6Gi (series * 4KB + overhead)\n"
        "- CPU: 1000-2000m (scraping 30 targets at 15s intervals + queries)\n\n"
        "**Recommended values.yaml:**\n"
        "```yaml\n"
        "server:\n"
        "  resources:\n"
        "    requests:\n"
        "      cpu: 1000m\n"
        "      memory: 4Gi\n"
        "    limits:\n"
        "      cpu: 2000m\n"
        "      memory: 8Gi\n"
        "  persistentVolume:\n"
        "    size: 50Gi  # default 8Gi is too small for 30 nodes\n"
        "```\n\n"
        "**configmap-reload sidecar:**\n"
        "```yaml\n"
        "configmapReload:\n"
        "  prometheus:\n"
        "    resources:\n"
        "      requests: { cpu: 10m, memory: 32Mi }\n"
        "      limits: { cpu: 50m, memory: 64Mi }\n"
        "```\n\n"
        "**Key difference vs kube-prometheus-stack:** The standalone chart does not "
        "bundle Grafana, kube-state-metrics, or node-exporter. If you need those, "
        "deploy them separately and account for their resource overhead.",
        "right-sizing", url,
    ))

    pairs.append(make_pair(
        "helm-prometheus-right-sizing-003",
        "My standalone Prometheus is using 12Gi memory on a 50-node cluster. "
        "I deployed with chart defaults. How should I set resource limits?",
        "## Right-Sizing Prometheus at 12Gi\n\n"
        "**With the chart defaulting to `resources: {}`, your Prometheus is running "
        "without any memory limit.** At 12Gi, you have approximately 3 million active "
        "time series (12Gi / 4KB per series).\n\n"
        "**Step 1 -- Verify your series count:**\n"
        "```promql\n"
        "prometheus_tsdb_head_series\n"
        "```\n\n"
        "**Step 2 -- Set appropriate resources:**\n"
        "```yaml\n"
        "server:\n"
        "  resources:\n"
        "    requests:\n"
        "      cpu: 2000m\n"
        "      memory: 12Gi   # Current usage as baseline\n"
        "    limits:\n"
        "      cpu: 4000m\n"
        "      memory: 18Gi   # 50% headroom for compaction\n"
        "```\n\n"
        "**Why 18Gi limit when current usage is 12Gi:**\n"
        "- TSDB compaction (every 2h) temporarily doubles block memory\n"
        "- Large range queries allocate intermediate result buffers\n"
        "- WAL replay on restart needs memory proportional to WAL size\n"
        "- Without headroom, compaction triggers OOMKill, corrupting WAL\n\n"
        "**Step 3 -- Reduce series count if 12Gi is too expensive:**\n"
        "- Drop unused metrics via `metric_relabel_configs`\n"
        "- Reduce scrape interval from 15s to 30s (halves ingestion CPU, not memory)\n"
        "- Reduce retention from default 15d to 7d\n"
        "- Check for cardinality bombs: `topk(10, count by (__name__)({__name__=~\".+\"}))`",
        "right-sizing", url,
    ))

    pairs.append(make_pair(
        "helm-prometheus-classification-004",
        "How should I classify Prometheus server workload for right-sizing?",
        "## Prometheus Server Workload Classification\n\n"
        "**Prometheus is a STEADY workload with periodic compaction spikes.**\n\n"
        "**Steady characteristics:**\n"
        "- CPU usage is proportional to scrape target count and scrape interval. "
        "With fixed targets and 15s interval, CPU is highly predictable.\n"
        "- Memory is proportional to active series count. Series count changes slowly "
        "(new deployments add series, removed deployments eventually age out).\n"
        "- Coefficient of variation (CV) for CPU is typically < 0.3 outside compaction.\n\n"
        "**Compaction spikes (every ~2 hours):**\n"
        "- CPU spikes 2-5x during TSDB block compaction\n"
        "- Memory spikes by 20-50% (reading old block + writing new block)\n"
        "- These are predictable and periodic, not anomalous\n\n"
        "**Right-sizing approach for steady-with-spikes:**\n"
        "- CPU request: P95 * 1.20 (captures scraping load, not compaction)\n"
        "- CPU limit: Max * 1.25 (accommodates compaction peaks)\n"
        "- Memory request: P95 * 1.20 (stable memory footprint)\n"
        "- Memory limit: P99 * 1.50 (compaction + query headroom)\n"
        "- Confidence: 0.90 for CPU (predictable), 0.85 for memory (query-dependent)\n\n"
        "**Do not classify Prometheus as burstable** despite the compaction spikes. "
        "The baseline is high and consistent. Burstable classification would set "
        "requests too low, causing throttling during normal scraping.",
        "classification", url,
    ))

    pairs.append(make_pair(
        "helm-prometheus-runtime-specific-005",
        "What is the total resource footprint of a standalone Prometheus deployment "
        "with alertmanager and pushgateway enabled?",
        "## Total Resource Footprint: Standalone Prometheus Stack\n\n"
        "**With all components enabled (server + alertmanager + pushgateway + "
        "configmap-reload sidecars), the chart deploys 3 pods plus sidecars.**\n\n"
        "**On a 20-node cluster (typical small-medium production):**\n\n"
        "| Component | CPU actual | Memory actual | Recommended request |\n"
        "|-----------|-----------|--------------|--------------------|\n"
        "| Server | 500m | 3Gi | 750m / 4Gi |\n"
        "| Server configmap-reload | 5m | 20Mi | 10m / 32Mi |\n"
        "| Alertmanager | 10m | 40Mi | 50m / 64Mi |\n"
        "| Alertmanager configmap-reload | 5m | 20Mi | 10m / 32Mi |\n"
        "| Pushgateway | 5m | 30Mi | 50m / 64Mi |\n"
        "| **Total** | **525m** | **~3.1Gi** | **870m / 4.2Gi** |\n\n"
        "**Important caveats:**\n"
        "1. These are actual usage numbers, not chart defaults (which are empty).\n"
        "2. Memory grows with series count. At 50 nodes, double the server memory.\n"
        "3. Pushgateway memory grows with pushed metric count. If applications push "
        "thousands of unique metrics, it can grow to 500Mi+.\n"
        "4. The configmap-reload sidecars are negligible (<10m CPU, <32Mi memory) "
        "but still need requests set to avoid BestEffort QoS on the parent pod.\n\n"
        "**Chart defaults leave everything at `resources: {}`.** The total overhead "
        "is modest for small clusters but the BestEffort QoS risk makes it essential "
        "to set explicit requests.",
        "runtime-specific", url,
    ))

    return pairs


# ── redis (6 pairs) ──────────────────────────────────────────────────


def redis_pairs() -> list[dict]:
    url = URLS["redis"]
    pairs = []

    pairs.append(make_pair(
        "helm-redis-right-sizing-001",
        "What are the default resource settings in the Bitnami Redis Helm chart?",
        "## Bitnami Redis Chart Default Resources\n\n"
        "The Bitnami Redis chart sets explicit resource defaults for all containers:\n\n"
        "**Master container:**\n"
        "- requests: cpu=250m, memory=256Mi\n"
        "- limits: cpu=not set (no CPU limit by default), memory=256Mi\n\n"
        "**Replica container:**\n"
        "- requests: cpu=250m, memory=256Mi\n"
        "- limits: cpu=not set, memory=256Mi\n\n"
        "**Sentinel container (if enabled):**\n"
        "- requests: cpu=250m, memory=256Mi\n"
        "- limits: cpu=not set, memory=256Mi\n\n"
        "**Metrics exporter sidecar (if enabled):**\n"
        "- requests: cpu=100m, memory=256Mi\n"
        "- limits: cpu=not set, memory=256Mi\n\n"
        "**Key observation:** The chart uses `resourcesPreset: nano` by default, which "
        "maps to the values above. The memory request equals the memory limit (256Mi), "
        "creating Guaranteed QoS for memory. This means any Redis instance holding more "
        "than ~200Mi of data will be OOMKilled immediately.\n\n"
        "**The 256Mi limit is only appropriate for:**\n"
        "- Session caches with small data volume (<100Mi)\n"
        "- Rate limiter counters\n"
        "- Development/testing environments\n\n"
        "For any production Redis with meaningful data, these defaults must be increased.",
        "right-sizing", url,
    ))

    pairs.append(make_pair(
        "helm-redis-right-sizing-002",
        "Are the default Bitnami Redis resources appropriate for production? "
        "We store 2Gi of session data.",
        "## Production Assessment: Bitnami Redis with 2Gi Dataset\n\n"
        "**No. The defaults (256Mi memory limit) will cause immediate OOMKill.** "
        "Redis holds the entire dataset in memory. With 2Gi of data, you need:\n\n"
        "**Memory calculation:**\n"
        "- Dataset: 2Gi\n"
        "- Redis overhead (per-key metadata, ~80 bytes/key): ~200Mi for 2M keys\n"
        "- Replication output buffer (repl-backlog-size): 1Mi default\n"
        "- Client output buffers: ~50Mi (typical 200 connections)\n"
        "- jemalloc fragmentation: ~10% = 200Mi\n"
        "- AOF rewrite buffer (worst case): up to 2Gi during background rewrite\n"
        "- Total steady: ~2.5Gi, peak during AOF rewrite: ~4.5Gi\n\n"
        "**Recommended values.yaml:**\n"
        "```yaml\n"
        "master:\n"
        "  resources:\n"
        "    requests:\n"
        "      cpu: 250m\n"
        "      memory: 3Gi\n"
        "    limits:\n"
        "      cpu: 1000m\n"
        "      memory: 5Gi    # Headroom for AOF rewrite\n"
        "  persistence:\n"
        "    size: 8Gi        # Default 8Gi is fine for 2Gi dataset\n"
        "replica:\n"
        "  resources:\n"
        "    requests:\n"
        "      cpu: 250m\n"
        "      memory: 3Gi\n"
        "    limits:\n"
        "      cpu: 500m\n"
        "      memory: 5Gi\n"
        "```\n\n"
        "**Also set maxmemory in Redis config:**\n"
        "```yaml\n"
        "commonConfiguration: |-\n"
        "  maxmemory 3500mb\n"
        "  maxmemory-policy allkeys-lru\n"
        "```\n"
        "Set maxmemory to ~70% of the container memory limit. This ensures Redis "
        "evicts keys before the container is OOMKilled.",
        "right-sizing", url,
    ))

    pairs.append(make_pair(
        "helm-redis-right-sizing-003",
        "My Redis pod from the Bitnami chart keeps getting OOMKilled. "
        "It is running with the default 256Mi memory limit.",
        "## Redis OOMKill with Default 256Mi Limit\n\n"
        "**The Bitnami Redis chart defaults to 256Mi memory limit via the `nano` "
        "resource preset.** This is the most common cause of Redis OOMKill in "
        "Kubernetes.\n\n"
        "**Why it OOMKills:**\n"
        "- Redis holds all data in memory\n"
        "- 256Mi allows for approximately 200Mi of actual data (after Redis overhead)\n"
        "- Even an empty Redis uses ~30Mi for the server process\n"
        "- Any meaningful dataset exceeds the 256Mi limit\n\n"
        "**Diagnosis -- check your dataset size:**\n"
        "```bash\n"
        "redis-cli INFO memory | grep used_memory_human\n"
        "redis-cli DBSIZE\n"
        "```\n\n"
        "**Fix -- increase resources based on dataset size:**\n"
        "```yaml\n"
        "master:\n"
        "  resourcesPreset: \"none\"  # Override the nano preset\n"
        "  resources:\n"
        "    requests:\n"
        "      cpu: 250m\n"
        "      memory: 1Gi     # Adjust based on INFO memory output\n"
        "    limits:\n"
        "      memory: 2Gi     # 2x dataset for AOF rewrite headroom\n"
        "```\n\n"
        "**Sizing formula:**\n"
        "- memory_request = used_memory * 1.3 (fragmentation + overhead)\n"
        "- memory_limit = memory_request * 2.0 (AOF/RDB fork headroom)\n"
        "- Set maxmemory = memory_limit * 0.7\n\n"
        "**Critical detail with Bitnami chart:** You must set `resourcesPreset: \"none\"` "
        "when providing custom `resources:` values, otherwise the preset overrides your "
        "custom settings. This is a common misconfiguration -- users set resources but "
        "the nano preset still applies.",
        "right-sizing", url,
    ))

    pairs.append(make_pair(
        "helm-redis-classification-004",
        "How should I classify a Bitnami Redis deployment for workload pattern analysis?",
        "## Redis Workload Classification\n\n"
        "**Redis is almost always a STEADY workload for memory, and varies for CPU.**\n\n"
        "**Memory pattern -- Steady:**\n"
        "- Redis memory is deterministic: it equals the dataset size plus overhead.\n"
        "- Memory does not fluctuate with request rate (unlike JVM apps).\n"
        "- The only memory spikes occur during AOF rewrite or RDB save (fork doubles "
        "memory temporarily via copy-on-write).\n"
        "- CV for memory is typically < 0.1 (very stable).\n\n"
        "**CPU pattern depends on usage:**\n"
        "- Cache workload (GET/SET): Steady CPU proportional to ops/sec.\n"
        "- Pub/Sub: Burstable -- spikes with message volume.\n"
        "- Lua scripts: Burstable -- CPU spikes during script execution.\n"
        "- Sorted set operations (ZRANGEBYSCORE on large sets): Burstable.\n\n"
        "**Right-sizing approach:**\n"
        "- Memory: Use the dataset size formula, not P95-based sizing. Redis memory "
        "is configuration-bound, not traffic-bound.\n"
        "- CPU: Use standard classification (CV < 0.3 = steady, etc).\n"
        "- The Bitnami chart default of 250m CPU request is appropriate for "
        "low-to-medium traffic (up to ~10K ops/sec).\n\n"
        "**Bitnami chart specifics:**\n"
        "- The `nano` preset (default) sets request=limit for memory, creating "
        "Guaranteed QoS. This is actually correct for Redis (you want guaranteed "
        "memory), but the 256Mi value is too small.\n"
        "- Change the value, not the pattern: keep request close to limit for memory, "
        "but set both to an appropriate value for your dataset.",
        "classification", url,
    ))

    pairs.append(make_pair(
        "helm-redis-edge-case-005",
        "We deployed Bitnami Redis with 3 replicas using default resources. "
        "The master and replicas all use 256Mi memory limit. During a failover "
        "test the new master OOMKilled immediately.",
        "## Edge Case: Redis Failover OOMKill with Default Resources\n\n"
        "**Root cause: The replica promoted to master did not have enough memory "
        "to handle write operations and client buffer growth.**\n\n"
        "**What happens during Redis Sentinel failover:**\n"
        "1. Sentinel detects master is down\n"
        "2. A replica is promoted to master\n"
        "3. Clients reconnect and redirect writes to the new master\n"
        "4. The new master begins accepting writes AND replicating to remaining replicas\n\n"
        "**Why the OOMKill with 256Mi limit:**\n"
        "- As a replica, memory usage was: dataset + replication buffer (~230Mi)\n"
        "- As new master, memory usage adds: client output buffers for replicas "
        "(default 256MB per replica for backlog), write operation buffers, AOF buffer\n"
        "- Total: 230Mi + 256Mi replication buffer = exceeds 256Mi limit immediately\n\n"
        "**Fix: Master and replicas must have identical resource limits:**\n"
        "```yaml\n"
        "master:\n"
        "  resourcesPreset: \"none\"\n"
        "  resources:\n"
        "    requests: { cpu: 250m, memory: 1Gi }\n"
        "    limits: { memory: 2Gi }\n"
        "replica:\n"
        "  resourcesPreset: \"none\"\n"
        "  resources:\n"
        "    requests: { cpu: 250m, memory: 1Gi }\n"
        "    limits: { memory: 2Gi }  # Same as master\n"
        "```\n\n"
        "**Rule: Any Redis replica that can be promoted to master must have the same "
        "memory limit as the master.** In the Bitnami chart, this means both "
        "`master.resources` and `replica.resources` should have identical memory limits.\n\n"
        "**Sentinel resources are separate:** Sentinel itself uses minimal resources "
        "(default 256Mi is fine for Sentinel since it stores no data). Do not confuse "
        "Sentinel resource limits with data node limits.",
        "edge-case", url,
    ))

    pairs.append(make_pair(
        "helm-redis-runtime-specific-006",
        "What is the total resource overhead of deploying Bitnami Redis "
        "with 1 master, 3 replicas, Sentinel enabled, and metrics exporter?",
        "## Total Resource Footprint: Bitnami Redis Full Deployment\n\n"
        "**With default `nano` preset (1 master + 3 replicas + 3 sentinels + "
        "metrics exporters):**\n\n"
        "| Component | Pods | CPU req/pod | Mem req/pod | Total CPU | Total Mem |\n"
        "|-----------|------|-----------|-----------|----------|----------|\n"
        "| Master | 1 | 250m | 256Mi | 250m | 256Mi |\n"
        "| Replicas | 3 | 250m | 256Mi | 750m | 768Mi |\n"
        "| Sentinels | 3 | 250m | 256Mi | 750m | 768Mi |\n"
        "| Metrics (sidecars) | 4 | 100m | 256Mi | 400m | 1Gi |\n"
        "| **Total** | | | | **2150m** | **2.8Gi** |\n\n"
        "**Production-sized (2Gi dataset, same topology):**\n\n"
        "| Component | Pods | CPU req/pod | Mem req/pod | Total CPU | Total Mem |\n"
        "|-----------|------|-----------|-----------|----------|----------|\n"
        "| Master | 1 | 500m | 3Gi | 500m | 3Gi |\n"
        "| Replicas | 3 | 250m | 3Gi | 750m | 9Gi |\n"
        "| Sentinels | 3 | 50m | 64Mi | 150m | 192Mi |\n"
        "| Metrics (sidecars) | 4 | 50m | 64Mi | 200m | 256Mi |\n"
        "| **Total** | | | | **1600m** | **12.4Gi** |\n\n"
        "**Key insight:** The default config uses 2.8Gi of memory requests across "
        "the cluster but can only store ~200Mi of actual data. The production config "
        "uses 12.4Gi but can store 2Gi -- a much better ratio.\n\n"
        "**Sentinel overhead is negligible.** Sentinels store no data and use minimal "
        "CPU. The default 256Mi memory preset is wasteful for Sentinel specifically; "
        "64Mi is sufficient. Reducing Sentinel memory from 256Mi to 64Mi saves 576Mi "
        "across 3 sentinel pods.",
        "runtime-specific", url,
    ))

    return pairs


# ── postgresql (6 pairs) ─────────────────────────────────────────────


def postgresql_pairs() -> list[dict]:
    url = URLS["postgresql"]
    pairs = []

    pairs.append(make_pair(
        "helm-postgresql-right-sizing-001",
        "What are the default resource settings in the Bitnami PostgreSQL Helm chart?",
        "## Bitnami PostgreSQL Chart Default Resources\n\n"
        "The Bitnami PostgreSQL chart uses resource presets:\n\n"
        "**Primary container:**\n"
        "- requests: cpu=250m, memory=256Mi\n"
        "- limits: cpu=not set, memory=256Mi\n"
        "- Uses `resourcesPreset: nano` by default\n\n"
        "**Read replicas (if enabled):**\n"
        "- Same defaults as primary: 250m CPU request, 256Mi memory\n\n"
        "**Metrics exporter sidecar (if enabled):**\n"
        "- requests: cpu=100m, memory=256Mi\n"
        "- limits: cpu=not set, memory=256Mi\n\n"
        "**Volume permissions init container:**\n"
        "- Uses the same resource preset as the main container\n\n"
        "**Key concern:** PostgreSQL with 256Mi memory limit is severely constrained. "
        "PostgreSQL allocates shared_buffers (default 128MB) plus per-connection memory "
        "(sort_buffer, work_mem, temp_buffers). Even with default PostgreSQL config and "
        "10 connections, actual memory usage exceeds 256Mi.\n\n"
        "The Bitnami chart's `nano` preset is designed for development, not production. "
        "Any production PostgreSQL deployment requires custom resource values.",
        "right-sizing", url,
    ))

    pairs.append(make_pair(
        "helm-postgresql-right-sizing-002",
        "Are the Bitnami PostgreSQL defaults suitable for production? "
        "We have a 20Gi database with 100 connections.",
        "## Production Assessment: Bitnami PostgreSQL\n\n"
        "**No. The defaults (256Mi memory limit) are completely inadequate for "
        "a production database.**\n\n"
        "**Memory calculation for 20Gi database, 100 connections:**\n"
        "- shared_buffers: Should be 25% of available memory. For 20Gi dataset, "
        "ideally 4-5Gi to cache hot data.\n"
        "- Per-connection buffers: work_mem (4MB) + sort_buffer (256KB) + "
        "read_buffer (128KB) + temp_buffers (8MB) per connection\n"
        "- 100 connections worst case: ~1.2Gi\n"
        "- OS page cache: PostgreSQL relies on OS cache for buffer misses. 2-4Gi.\n"
        "- maintenance_work_mem: 256Mi for VACUUM operations\n"
        "- Total: ~8-10Gi\n\n"
        "**Recommended values.yaml:**\n"
        "```yaml\n"
        "primary:\n"
        "  resourcesPreset: \"none\"\n"
        "  resources:\n"
        "    requests:\n"
        "      cpu: 1000m\n"
        "      memory: 8Gi\n"
        "    limits:\n"
        "      cpu: 4000m\n"
        "      memory: 12Gi\n"
        "  persistence:\n"
        "    size: 50Gi     # 2.5x dataset for WAL + temp files\n"
        "  extendedConfiguration: |-\n"
        "    shared_buffers = 2GB\n"
        "    effective_cache_size = 6GB\n"
        "    work_mem = 4MB\n"
        "    max_connections = 100\n"
        "```\n\n"
        "**CPU sizing:**\n"
        "- OLTP (short queries, high TPS): 1000-2000m request\n"
        "- OLAP (complex analytical queries): 2000-4000m request\n"
        "- Mixed: 1000m request, 4000m limit\n"
        "- VACUUM/autovacuum: Background CPU. 3 workers default = up to 3000m spike.\n\n"
        "**The default 250m CPU request is only suitable for <50 TPS light OLTP.**",
        "right-sizing", url,
    ))

    pairs.append(make_pair(
        "helm-postgresql-right-sizing-003",
        "My Bitnami PostgreSQL pod with default 256Mi memory keeps getting OOMKilled "
        "after about 50 client connections.",
        "## PostgreSQL OOMKill at Default Memory Limits\n\n"
        "**Root cause: Each PostgreSQL connection allocates per-connection memory "
        "that quickly exceeds the 256Mi container limit.**\n\n"
        "**Per-connection memory breakdown (PostgreSQL defaults):**\n"
        "- work_mem: 4MB (per sort operation, per connection)\n"
        "- temp_buffers: 8MB (per connection for temp table access)\n"
        "- sort_buffer_size: 256KB\n"
        "- thread_stack: 256KB\n"
        "- Total per connection: ~12.5MB worst case\n\n"
        "**At 50 connections:**\n"
        "- Base PostgreSQL: ~80Mi (shared_buffers=128MB default)\n"
        "- 50 connections overhead: 50 * 12.5MB = 625Mi\n"
        "- Total: ~705Mi -- far exceeding the 256Mi limit\n\n"
        "**In practice, OOMKill happens earlier** because not every connection uses "
        "max memory simultaneously, but 50 connections is enough to trigger it.\n\n"
        "**Fix:**\n"
        "```yaml\n"
        "primary:\n"
        "  resourcesPreset: \"none\"\n"
        "  resources:\n"
        "    requests:\n"
        "      cpu: 500m\n"
        "      memory: 2Gi\n"
        "    limits:\n"
        "      memory: 4Gi\n"
        "```\n\n"
        "**Also tune PostgreSQL config to match container limits:**\n"
        "```yaml\n"
        "  extendedConfiguration: |-\n"
        "    shared_buffers = 512MB        # 25% of 2Gi request\n"
        "    effective_cache_size = 1536MB  # 75% of 2Gi request\n"
        "    work_mem = 4MB\n"
        "    max_connections = 50\n"
        "```\n\n"
        "**Alternatively, reduce connections:** Use PgBouncer (connection pooler) "
        "in front of PostgreSQL. PgBouncer with 200 client connections can multiplex "
        "to 20 PostgreSQL connections, reducing memory from 625Mi to 250Mi.",
        "right-sizing", url,
    ))

    pairs.append(make_pair(
        "helm-postgresql-classification-004",
        "How should I classify a Bitnami PostgreSQL primary for workload pattern analysis?",
        "## PostgreSQL Primary Workload Classification\n\n"
        "**PostgreSQL primaries are typically STEADY for memory and BURSTABLE for CPU.**\n\n"
        "**Memory -- Steady:**\n"
        "- shared_buffers is allocated at startup and remains constant\n"
        "- Connection memory scales with active connection count (usually stable)\n"
        "- OS page cache fills gradually and stays warm\n"
        "- CV for memory is typically < 0.2\n"
        "- Exception: Temporary tables in complex queries can cause memory spikes\n\n"
        "**CPU -- Burstable (OLTP) or Steady (OLAP):**\n"
        "- OLTP: P50 is low (simple SELECTs/INSERTs), P99 is high (complex JOINs, "
        "autovacuum, index builds)\n"
        "- OLAP: P50-P95 is consistently high during query execution\n"
        "- P99/P50 ratio > 5 for OLTP = burstable classification\n\n"
        "**Right-sizing approach:**\n"
        "- Memory: Calculate from PostgreSQL configuration, not from P95 metrics.\n"
        "  Formula: shared_buffers + (max_connections * 12MB) + 2Gi OS cache headroom\n"
        "- CPU request: P95 * 1.20 for OLTP, P50 * 1.20 for OLAP\n"
        "- CPU limit: Max * 1.25 (must accommodate VACUUM)\n\n"
        "**Bitnami chart specifics:**\n"
        "- The `nano` preset creates Guaranteed QoS for memory (request=limit=256Mi).\n"
        "- For production, use Burstable QoS: set memory request to steady-state "
        "need and memory limit with VACUUM/sort headroom.\n"
        "- Never set PostgreSQL memory limit equal to shared_buffers. The limit "
        "must accommodate shared_buffers + connection memory + OS overhead.",
        "classification", url,
    ))

    pairs.append(make_pair(
        "helm-postgresql-edge-case-005",
        "We run Bitnami PostgreSQL with primary and 2 read replicas, all at default "
        "256Mi. Replication lag keeps increasing and replicas restart frequently.",
        "## Edge Case: PostgreSQL Replication Lag with Default Resources\n\n"
        "**Root cause: Read replicas are OOMKilled during WAL replay, causing restarts "
        "and increasing replication lag.**\n\n"
        "**Why replicas are more memory-hungry than they appear:**\n"
        "- Replicas apply WAL records (write-ahead log) from the primary\n"
        "- During WAL replay, PostgreSQL needs memory for the replay operation\n"
        "- If the replica serves read queries simultaneously, those queries also "
        "need work_mem and temp_buffers\n"
        "- Heavy read queries on a replica can push it past the 256Mi limit\n\n"
        "**Replication lag feedback loop:**\n"
        "1. Replica OOMKilled during WAL replay or heavy read query\n"
        "2. Replica restarts and must replay accumulated WAL from primary\n"
        "3. WAL replay is memory-intensive, increasing OOMKill risk\n"
        "4. If lag exceeds wal_keep_size, primary removes needed WAL segments\n"
        "5. Replica cannot catch up and requires full base backup (hours of downtime)\n\n"
        "**Fix:**\n"
        "```yaml\n"
        "readReplicas:\n"
        "  resourcesPreset: \"none\"\n"
        "  resources:\n"
        "    requests:\n"
        "      cpu: 500m\n"
        "      memory: 2Gi    # Match primary resources\n"
        "    limits:\n"
        "      memory: 4Gi\n"
        "  replicaCount: 2\n"
        "```\n\n"
        "**Critical:** Read replicas should have the same or more memory than the "
        "primary. They apply the same writes (via WAL) AND serve read queries. "
        "Giving replicas less memory than the primary is a common mistake.\n\n"
        "**Bitnami chart note:** `readReplicas.resources` is separate from "
        "`primary.resources`. You must configure both explicitly.",
        "edge-case", url,
    ))

    pairs.append(make_pair(
        "helm-postgresql-runtime-specific-006",
        "What is the total resource footprint of Bitnami PostgreSQL "
        "with primary, 2 read replicas, and metrics exporter?",
        "## Total Resource Footprint: Bitnami PostgreSQL\n\n"
        "**With default `nano` preset (primary + 2 replicas + metrics exporters):**\n\n"
        "| Component | Pods | CPU req | Mem req | Total CPU | Total Mem |\n"
        "|-----------|------|---------|---------|----------|----------|\n"
        "| Primary | 1 | 250m | 256Mi | 250m | 256Mi |\n"
        "| Replicas | 2 | 250m | 256Mi | 500m | 512Mi |\n"
        "| Metrics (sidecars) | 3 | 100m | 256Mi | 300m | 768Mi |\n"
        "| **Total** | | | | **1050m** | **1.5Gi** |\n\n"
        "**Production-sized (20Gi database, 100 connections):**\n\n"
        "| Component | Pods | CPU req | Mem req | Total CPU | Total Mem |\n"
        "|-----------|------|---------|---------|----------|----------|\n"
        "| Primary | 1 | 1000m | 8Gi | 1000m | 8Gi |\n"
        "| Replicas | 2 | 500m | 8Gi | 1000m | 16Gi |\n"
        "| Metrics (sidecars) | 3 | 50m | 64Mi | 150m | 192Mi |\n"
        "| **Total** | | | | **2150m** | **24.2Gi** |\n\n"
        "**Observations:**\n"
        "- The metrics exporter sidecars use more memory (768Mi total) than the "
        "database containers themselves (768Mi total) at default settings. This is "
        "disproportionate.\n"
        "- In production, metrics exporters are negligible compared to the database.\n"
        "- Read replicas need the same memory as the primary but can use less CPU "
        "if they only serve read queries.\n"
        "- Persistent volume costs often exceed compute costs for database workloads. "
        "Factor in 3x PV cost (primary + 2 replicas, each with full dataset copy).",
        "runtime-specific", url,
    ))

    return pairs


# ── mongodb (5 pairs) ────────────────────────────────────────────────


def mongodb_pairs() -> list[dict]:
    url = URLS["mongodb"]
    pairs = []

    pairs.append(make_pair(
        "helm-mongodb-right-sizing-001",
        "What are the default resource settings in the Bitnami MongoDB Helm chart?",
        "## Bitnami MongoDB Chart Default Resources\n\n"
        "The Bitnami MongoDB chart uses the `nano` resource preset by default:\n\n"
        "**Standalone / ReplicaSet members:**\n"
        "- requests: cpu=250m, memory=256Mi\n"
        "- limits: cpu=not set, memory=256Mi\n\n"
        "**Arbiter (if replicaSet enabled):**\n"
        "- requests: cpu=250m, memory=256Mi\n"
        "- limits: cpu=not set, memory=256Mi\n\n"
        "**Hidden node (if enabled):**\n"
        "- Same as primary: 250m CPU, 256Mi memory\n\n"
        "**Metrics exporter sidecar:**\n"
        "- requests: cpu=100m, memory=256Mi\n"
        "- limits: cpu=not set, memory=256Mi\n\n"
        "**Key problem:** MongoDB uses WiredTiger as its storage engine, which "
        "allocates a cache sized at 50% of (RAM - 1Gi) by default. With a 256Mi "
        "container limit, WiredTiger tries to allocate 50% of (256Mi - 1Gi) which "
        "resolves to the minimum 256MB. This leaves virtually no memory for "
        "connections, indexes, or operations.\n\n"
        "The arbiter, which stores no data and only votes in elections, has the "
        "same 256Mi default -- this is actually reasonable for an arbiter since it "
        "needs minimal resources.",
        "right-sizing", url,
    ))

    pairs.append(make_pair(
        "helm-mongodb-right-sizing-002",
        "Are the Bitnami MongoDB defaults appropriate for production? We have "
        "a 10Gi database with a 3-member replica set.",
        "## Production Assessment: Bitnami MongoDB\n\n"
        "**No. The 256Mi memory limit will cause severe performance degradation "
        "or OOMKill with a 10Gi dataset.**\n\n"
        "**MongoDB memory model (WiredTiger):**\n"
        "- WiredTiger internal cache: Should hold hot data + indexes.\n"
        "  Rule of thumb: cache = 50% of (container_memory - 1Gi)\n"
        "- Connection overhead: ~10KB per connection\n"
        "- File handles, journal, diagnostics: ~500Mi\n"
        "- For 10Gi database with 80% hot data: cache should be at least 4Gi\n\n"
        "**Calculation:**\n"
        "- WiredTiger cache target: 4Gi\n"
        "- Reverse formula: container = (cache / 0.5) + 1Gi = 9Gi\n"
        "- Add connections (200 * 10KB = 2Mi) + overhead (500Mi)\n"
        "- Total: ~10Gi\n\n"
        "**Recommended values.yaml:**\n"
        "```yaml\n"
        "resourcesPreset: \"none\"\n"
        "resources:\n"
        "  requests:\n"
        "    cpu: 1000m\n"
        "    memory: 8Gi\n"
        "  limits:\n"
        "    cpu: 4000m\n"
        "    memory: 12Gi\n"
        "```\n\n"
        "**Set WiredTiger cache explicitly:**\n"
        "```yaml\n"
        "extraFlags:\n"
        "  - \"--wiredTigerCacheSizeGB=4\"\n"
        "```\n\n"
        "**Arbiter is fine at lower resources:**\n"
        "```yaml\n"
        "arbiter:\n"
        "  resourcesPreset: \"none\"\n"
        "  resources:\n"
        "    requests: { cpu: 100m, memory: 256Mi }\n"
        "    limits: { memory: 512Mi }\n"
        "```\n\n"
        "**Per-replica-set-member:** Each member holds a full copy of the data. "
        "All 3 members need the same memory resources (not just the primary).",
        "right-sizing", url,
    ))

    pairs.append(make_pair(
        "helm-mongodb-right-sizing-003",
        "My Bitnami MongoDB pod is extremely slow with default 256Mi memory. "
        "Queries that used to take 10ms now take 5 seconds.",
        "## MongoDB Performance Degradation at 256Mi\n\n"
        "**Root cause: WiredTiger cache is too small, causing every operation "
        "to read from disk instead of cache.**\n\n"
        "**What is happening:**\n"
        "With 256Mi container memory, WiredTiger gets approximately 0 bytes of "
        "effective cache (formula: 50% of (256Mi - 1Gi) = negative, so minimum "
        "256MB is used, but after MongoDB server overhead there is almost nothing "
        "left for actual data caching).\n\n"
        "**Every query hits disk:**\n"
        "- With adequate cache: document lookup = in-memory hash table access (~10ms)\n"
        "- Without cache: document lookup = disk seek + read (~5-50ms per doc)\n"
        "- Index scans without cached indexes: 10-100x slower\n"
        "- Multiple documents per query: latency multiplies\n\n"
        "**Diagnosis:**\n"
        "```javascript\n"
        "db.serverStatus().wiredTiger.cache\n"
        "// Check: 'bytes currently in the cache'\n"
        "// Check: 'pages read into cache' (high = cache misses)\n"
        "// Check: 'pages evicted' (high = cache pressure)\n"
        "```\n\n"
        "**Fix -- increase memory and set WiredTiger cache:**\n"
        "```yaml\n"
        "resourcesPreset: \"none\"\n"
        "resources:\n"
        "  requests:\n"
        "    cpu: 500m\n"
        "    memory: 4Gi\n"
        "  limits:\n"
        "    memory: 6Gi\n"
        "extraFlags:\n"
        "  - \"--wiredTigerCacheSizeGB=2\"\n"
        "```\n\n"
        "**After increasing memory, monitor:**\n"
        "- `wiredTiger.cache.bytes currently in the cache` should stabilize\n"
        "- `wiredTiger.cache.pages read into cache` rate should drop\n"
        "- Query latency should return to ~10ms for indexed lookups\n\n"
        "**Rule: WiredTiger cache size determines MongoDB performance more than "
        "any other configuration parameter.** Always size the container to allow "
        "sufficient WiredTiger cache.",
        "right-sizing", url,
    ))

    pairs.append(make_pair(
        "helm-mongodb-classification-004",
        "How should I classify a Bitnami MongoDB replica set for right-sizing?",
        "## MongoDB Replica Set Workload Classification\n\n"
        "**Each member has a different resource profile:**\n\n"
        "**Primary -- BURSTABLE:**\n"
        "- CPU: Handles all writes plus read preference=primary reads. CPU varies "
        "with write volume and query complexity.\n"
        "- Memory: Steady (WiredTiger cache is fixed size). Spikes during "
        "compaction and chunk migration (if sharded).\n"
        "- P99/P50 CPU ratio is typically 3-10x for OLTP workloads.\n\n"
        "**Secondary -- STEADY to BURSTABLE:**\n"
        "- CPU: Applies oplog entries (steady background work) + serves reads if "
        "readPreference=secondary.\n"
        "- Memory: Same as primary (full dataset copy + same WiredTiger cache).\n"
        "- If read-heavy with secondaryPreferred: burstable like primary.\n"
        "- If no reads routed to secondary: steady (oplog replay only).\n\n"
        "**Arbiter -- STEADY (near idle):**\n"
        "- CPU: Near zero. Only participates in elections.\n"
        "- Memory: ~50-100Mi. No data, no WiredTiger cache.\n"
        "- Classification: Steady. Safe to size at minimum.\n\n"
        "**Right-sizing approach for the Bitnami chart:**\n"
        "- All data-bearing members: Size identically. Any secondary can become primary "
        "during failover.\n"
        "- Memory: Based on WiredTiger cache formula, not P95 metrics.\n"
        "- CPU: Primary = P95 * 1.20 for request, secondary can use same or lower "
        "if no reads are routed to it.\n"
        "- Arbiter: 100m / 256Mi is sufficient.",
        "classification", url,
    ))

    pairs.append(make_pair(
        "helm-mongodb-edge-case-005",
        "We deployed Bitnami MongoDB with default resources and enabled the "
        "metrics exporter. The exporter sidecar uses more memory than MongoDB itself.",
        "## Edge Case: Metrics Exporter Memory Exceeding MongoDB\n\n"
        "**This is a known inefficiency in the Bitnami chart defaults.** Both MongoDB "
        "and the metrics exporter sidecar default to 256Mi memory. The exporter "
        "using comparable memory to the database is disproportionate.\n\n"
        "**Why the exporter uses significant memory:**\n"
        "- The percona/mongodb_exporter collects serverStatus, replSetGetStatus, "
        "dbStats, collStats, and topMetrics\n"
        "- On a database with many collections, the collector goroutines hold "
        "all stats in memory before exposing as Prometheus metrics\n"
        "- Default 256Mi is generous for the exporter -- it typically needs 64-128Mi\n\n"
        "**Right-sizing the exporter separately:**\n"
        "```yaml\n"
        "metrics:\n"
        "  enabled: true\n"
        "  resourcesPreset: \"none\"\n"
        "  resources:\n"
        "    requests:\n"
        "      cpu: 50m\n"
        "      memory: 64Mi\n"
        "    limits:\n"
        "      memory: 128Mi\n"
        "```\n\n"
        "**Impact on pod resources:**\n"
        "The metrics exporter runs as a sidecar in the same pod as MongoDB. The pod's "
        "total resource request is the sum of all containers. With defaults:\n"
        "- MongoDB container: 250m + 256Mi\n"
        "- Exporter sidecar: 100m + 256Mi\n"
        "- Pod total: 350m + 512Mi\n"
        "- 50% of the pod's memory request is for the exporter\n\n"
        "With right-sized exporter:\n"
        "- MongoDB container: 250m + 256Mi (still needs increasing for production)\n"
        "- Exporter sidecar: 50m + 64Mi\n"
        "- Pod total: 300m + 320Mi\n"
        "- Exporter is now 20% of pod memory (more proportionate)\n\n"
        "**This pattern applies across all Bitnami charts.** The metrics exporter "
        "sidecar always defaults to 256Mi regardless of the main application's actual "
        "needs. Always right-size the exporter separately.",
        "edge-case", url,
    ))

    return pairs


# ── Remaining chart functions will be defined below ──────────────────
# Placeholder for generate_all and __main__

def mysql_pairs() -> list[dict]:
    return []

def kafka_pairs() -> list[dict]:
    return []

def rabbitmq_pairs() -> list[dict]:
    return []

def elasticsearch_pairs() -> list[dict]:
    return []

def ingress_nginx_pairs() -> list[dict]:
    return []

def cert_manager_pairs() -> list[dict]:
    return []

def argo_cd_pairs() -> list[dict]:
    return []

def grafana_pairs() -> list[dict]:
    return []

def vault_pairs() -> list[dict]:
    return []

def istio_pairs() -> list[dict]:
    return []

def linkerd_pairs() -> list[dict]:
    return []

def traefik_pairs() -> list[dict]:
    return []

def jenkins_pairs() -> list[dict]:
    return []

def airflow_pairs() -> list[dict]:
    return []

def spark_operator_pairs() -> list[dict]:
    return []


def generate_all() -> list[dict]:
    """Collect all Helm chart grounded pairs."""
    all_pairs = []
    all_pairs.extend(kube_prometheus_stack_pairs())
    all_pairs.extend(prometheus_pairs())
    all_pairs.extend(redis_pairs())
    all_pairs.extend(postgresql_pairs())
    all_pairs.extend(mongodb_pairs())
    all_pairs.extend(mysql_pairs())
    all_pairs.extend(kafka_pairs())
    all_pairs.extend(rabbitmq_pairs())
    all_pairs.extend(elasticsearch_pairs())
    all_pairs.extend(ingress_nginx_pairs())
    all_pairs.extend(cert_manager_pairs())
    all_pairs.extend(argo_cd_pairs())
    all_pairs.extend(grafana_pairs())
    all_pairs.extend(vault_pairs())
    all_pairs.extend(istio_pairs())
    all_pairs.extend(linkerd_pairs())
    all_pairs.extend(traefik_pairs())
    all_pairs.extend(jenkins_pairs())
    all_pairs.extend(airflow_pairs())
    all_pairs.extend(spark_operator_pairs())

    # Validate
    ids = set()
    for pair in all_pairs:
        pid = pair["id"]
        assert pid not in ids, f"Duplicate ID: {pid}"
        ids.add(pid)
        assert len(pair["assistant"]) >= 50, f"{pid}: assistant too short"
        assert pair["metadata"]["category"] in (
            "right-sizing", "classification", "runtime-specific", "edge-case",
        ), f"{pid}: bad category"
        assert pair["metadata"].get("provenance"), f"{pid}: missing provenance"
        assert pair["source"] == "expert", f"{pid}: source must be 'expert'"

    # Write
    OUTPUT_PATH.parent.mkdir(parents=True, exist_ok=True)
    with open(OUTPUT_PATH, "w") as f:
        for pair in all_pairs:
            f.write(json.dumps(pair) + "\n")

    print(f"Wrote {len(all_pairs)} Helm grounded pairs -> {OUTPUT_PATH}")
    return all_pairs


if __name__ == "__main__":
    pairs = generate_all()
    cats: dict[str, int] = {}
    for p in pairs:
        c = p["metadata"]["category"]
        cats[c] = cats.get(c, 0) + 1
    print("Pairs by category:")
    for cat, count in sorted(cats.items()):
        print(f"  {cat}: {count}")
    charts: dict[str, int] = {}
    for p in pairs:
        chart = p["id"].split("-")[1]
        # Handle multi-word chart names
        parts = p["id"].replace("helm-", "").split("-")
        for i, part in enumerate(parts):
            if part in ("right", "classification", "runtime", "edge"):
                chart = "-".join(parts[:i])
                break
        charts[chart] = charts.get(chart, 0) + 1
    print("\nPairs by chart:")
    for chart, count in sorted(charts.items()):
        print(f"  {chart}: {count}")

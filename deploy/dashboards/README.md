# Observability Dashboard Templates

Pre-built dashboard templates for the gerty escalation pipeline metrics.

## Metrics Covered

| Metric | Type | Description |
|--------|------|-------------|
| `gerty_escalation_queue_depth` | Gauge | Current L3 escalation queue depth. Drives KEDA autoscaling. |
| `gerty_escalations_dispatched_total` | Counter | Total workloads escalated to L3. |
| `gerty_escalation_degradations_total` | Counter (labels: `from`, `to`) | Tier degradations when L3 fails or has insufficient headroom. |
| `gerty_cycle_skipped_total` | Counter | Analysis cycles that could not complete in time. |

All metrics are exposed at `/metrics` on the gerty-server pod (port 8080).

## Templates

### Grafana (recommended)

**File:** `grafana-gerty-escalation.json`

Import via Grafana UI (Dashboards > Import) or provision via ConfigMap. Requires a Prometheus datasource. 13 panels across 4 sections: queue depth, dispatch rate, degradations, cycle health.

The Helm chart can auto-provision this via the existing `grafana.dashboards.enabled` value if using the Grafana sidecar.

### Datadog

**File:** `datadog-gerty-escalation.json`

Import via Datadog API or UI (Dashboards > New Dashboard > Import). Requires the Datadog Prometheus/OpenMetrics integration check configured to scrape gerty-server.

Datadog agent config snippet:
```yaml
# datadog-agent values.yaml
datadog:
  prometheusScrape:
    enabled: true
    serviceEndpoints: true
```

### New Relic

**File:** `newrelic-gerty-escalation.json`

Import via NerdGraph API (`dashboardCreate` mutation) or the New Relic UI. Requires the Prometheus OpenMetrics integration or the OTel collector config below.

### OpenTelemetry Collector (catch-all)

**File:** `otel-collector.yaml`

Scrapes gerty Prometheus metrics and forwards to any OTLP-compatible backend. Edit the `exporters` section to point at your backend (Grafana Cloud, Datadog, New Relic, Honeycomb, etc). Deploy as a ConfigMap or mount into your collector container.

## Prometheus Scrape Setup

The gerty Helm chart adds standard Prometheus annotations to the server pod:

```yaml
prometheus.io/scrape: "true"
prometheus.io/port: "8080"
prometheus.io/path: "/metrics"
```

If using the Prometheus Operator, create a ServiceMonitor instead:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: gerty-server
spec:
  selector:
    matchLabels:
      app.kubernetes.io/component: server
  endpoints:
    - port: http
      path: /metrics
      interval: 15s
```

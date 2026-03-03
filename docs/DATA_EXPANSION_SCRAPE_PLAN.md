# Data Expansion Scrape Plan — Complete Source Inventory

Generated: 2026-03-03
All URLs probed with HTTP status codes. Local files verified.

---

## Complete Probe Results

### Fetchability Key
- **OK** = HTTP 200, content confirmed accessible
- **REDIR** = Redirects but content accessible at new URL
- **MANUAL** = HTTP 403, requires manual browser access (Medium paywall, Reddit block)
- **LOCAL** = File provided locally in docs/
- **DEAD** = 404 or DNS failure
- **GATED** = Landing page loads but full content behind email/download form
- **YT** = YouTube video, page loads (transcript extraction possible)
- **SLIDES** = SlideShare, page loads (limited extraction)

---

## Source Inventory by Category

### 1. Helm Chart values.yaml (20 files, ALL fetchable)

| # | Chart | Full Raw URL | Status | Size | resources: |
|---|-------|-------------|--------|------|-----------|
| 1 | kube-prometheus-stack | `https://raw.githubusercontent.com/prometheus-community/helm-charts/main/charts/kube-prometheus-stack/values.yaml` | OK | 187KB, 5473L | 14 |
| 2 | prometheus | `https://raw.githubusercontent.com/prometheus-community/helm-charts/main/charts/prometheus/values.yaml` | OK | 40KB, 1251L | 3 |
| 3 | redis | `https://raw.githubusercontent.com/bitnami/charts/main/bitnami/redis/values.yaml` | OK | 107KB, 2347L | 14 |
| 4 | postgresql | `https://raw.githubusercontent.com/bitnami/charts/main/bitnami/postgresql/values.yaml` | OK | 92KB, 1946L | 13 |
| 5 | mongodb | `https://raw.githubusercontent.com/bitnami/charts/main/bitnami/mongodb/values.yaml` | OK | 123KB, 2670L | 30 |
| 6 | mysql | `https://raw.githubusercontent.com/bitnami/charts/main/bitnami/mysql/values.yaml` | OK | 76KB, 1628L | 14 |
| 7 | kafka | `https://raw.githubusercontent.com/bitnami/charts/main/bitnami/kafka/values.yaml` | OK | 125KB, 2487L | 14 |
| 8 | rabbitmq | `https://raw.githubusercontent.com/bitnami/charts/main/bitnami/rabbitmq/values.yaml` | OK | 72KB, 1643L | 7 |
| 9 | elasticsearch | `https://raw.githubusercontent.com/bitnami/charts/main/bitnami/elasticsearch/values.yaml` | OK | 129KB, 2744L | 18 |
| 10 | ingress-nginx | `https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/charts/ingress-nginx/values.yaml` | OK | — | — |
| 11 | cert-manager | `https://raw.githubusercontent.com/cert-manager/cert-manager/master/deploy/charts/cert-manager/values.yaml` | OK | 64KB, 1710L | 4 |
| 12 | argo-cd | `https://raw.githubusercontent.com/argoproj/argo-helm/main/charts/argo-cd/values.yaml` | OK | 160KB, 4301L | 14 |
| 13 | grafana | `https://raw.githubusercontent.com/grafana/helm-charts/grafana-8.8.4/charts/grafana/values.yaml` | OK | — | — |
| 14 | vault | `https://raw.githubusercontent.com/hashicorp/vault-helm/main/values.yaml` | OK | 55KB, 1434L | 8 |
| 15 | istio | `https://raw.githubusercontent.com/istio/istio/master/manifests/charts/istio-control/istio-discovery/values.yaml` | OK | 24KB, 584L | 3 |
| 16 | linkerd | `https://raw.githubusercontent.com/linkerd/linkerd2/main/charts/linkerd-control-plane/values.yaml` | OK | 30KB, 744L | 2 |
| 17 | traefik | `https://raw.githubusercontent.com/traefik/traefik-helm-chart/master/traefik/values.yaml` | OK | 67KB, 1402L | 1 |
| 18 | jenkins | `https://raw.githubusercontent.com/jenkinsci/helm-charts/main/charts/jenkins/values.yaml` | OK | 58KB, 1418L | 5 |
| 19 | airflow | `https://raw.githubusercontent.com/apache/airflow/main/chart/values.yaml` | OK | 114KB, 3611L | 28 |
| 20 | spark-operator | `https://raw.githubusercontent.com/GoogleCloudPlatform/spark-on-k8s-operator/master/charts/spark-operator-chart/values.yaml` | OK | 18KB, 516L | 2 |

---

### 2. VPA / Tools / Autoscaler Source Code

| # | File | Full Raw URL | Status | Size |
|---|------|-------------|--------|------|
| 1 | recommender.go | `https://raw.githubusercontent.com/kubernetes/autoscaler/master/vertical-pod-autoscaler/pkg/recommender/logic/recommender.go` | OK | 10KB, 217L |
| 2 | vpa.go | `https://raw.githubusercontent.com/kubernetes/autoscaler/master/vertical-pod-autoscaler/pkg/recommender/model/vpa.go` | OK | 15KB, 395L |
| 3 | aggregate_container_state.go | `https://raw.githubusercontent.com/kubernetes/autoscaler/master/vertical-pod-autoscaler/pkg/recommender/model/aggregate_container_state.go` | OK | 18KB, 426L |
| 4 | estimator.go | `https://raw.githubusercontent.com/kubernetes/autoscaler/master/vertical-pod-autoscaler/pkg/recommender/logic/estimator.go` | OK | 10KB, 249L |
| 5 | cluster.go | `https://raw.githubusercontent.com/kubernetes/autoscaler/master/vertical-pod-autoscaler/pkg/recommender/model/cluster.go` | OK | 23KB, 564L |
| 6 | VPA README | `https://raw.githubusercontent.com/kubernetes/autoscaler/master/vertical-pod-autoscaler/README.md` | OK | 3KB, 64L |
| 7 | VPA FAQ | — | DEAD | 404 |
| 8 | Goldilocks README | `https://raw.githubusercontent.com/FairwindsOps/goldilocks/master/README.md` | OK | 3KB, 52L |
| 9 | OpenCost README | `https://raw.githubusercontent.com/opencost/opencost/develop/README.md` | OK | 11KB, 274L |
| 10 | OpenCost spec | `https://www.opencost.io/docs/specification` | OK | 39KB |
| 11 | Karpenter README | `https://raw.githubusercontent.com/kubernetes-sigs/karpenter/main/README.md` | OK | 6KB, 72L |
| 12 | automaxprocs README | `https://raw.githubusercontent.com/uber-go/automaxprocs/master/README.md` | OK | 3KB, 71L |
| 13 | kube-resource-report (recommender) | `https://codeberg.org/hjacobs/kube-resource-report/raw/branch/main/kube_resource_report/recommender.py` | OK | ~8KB |
| 14 | kube-resource-report (histogram) | `https://codeberg.org/hjacobs/kube-resource-report/raw/branch/main/kube_resource_report/histogram.py` | OK | ~5KB |
| 15 | kube-resource-report (pricing) | `https://codeberg.org/hjacobs/kube-resource-report/raw/branch/main/kube_resource_report/pricing.py` | OK | ~4KB |
| 16 | kube-resource-report (query) | `https://codeberg.org/hjacobs/kube-resource-report/raw/branch/main/kube_resource_report/query.py` | OK | ~12KB |
| 17 | kube-resource-report (metrics) | `https://codeberg.org/hjacobs/kube-resource-report/raw/branch/main/kube_resource_report/metrics.py` | OK | ~2KB |
| 18 | KEDA ScaledObject types | `https://raw.githubusercontent.com/kedacore/keda/main/apis/keda/v1alpha1/scaledobject_types.go` | OK | ~15KB |
| 19 | KEDA scale executor | `https://raw.githubusercontent.com/kedacore/keda/main/pkg/scaling/executor/scale_executor.go` | OK | ~8KB |
| 20 | KEDA scale_scaledobjects | `https://raw.githubusercontent.com/kedacore/keda/main/pkg/scaling/executor/scale_scaledobjects.go` | OK | ~12KB |
| 21 | KEDA fallback | `https://raw.githubusercontent.com/kedacore/keda/main/pkg/fallback/fallback.go` | OK | ~5KB |
| 22 | KEDA HPA controller | `https://raw.githubusercontent.com/kedacore/keda/main/controllers/keda/hpa.go` | OK | ~8KB |
| 23 | KEDA operator deployment | `https://raw.githubusercontent.com/kedacore/keda/main/config/manager/manager.yaml` | OK | ~3KB |
| 24 | prometheus-adapter default config | `https://raw.githubusercontent.com/kubernetes-sigs/prometheus-adapter/master/pkg/config/utils/default.go` | OK | ~5KB |
| 25 | prometheus-adapter adapter.go | `https://raw.githubusercontent.com/kubernetes-sigs/prometheus-adapter/master/cmd/adapter/adapter.go` | OK | ~8KB |
| 26 | prometheus-adapter sample config | `https://raw.githubusercontent.com/kubernetes-sigs/prometheus-adapter/master/docs/sample-config.yaml` | OK | ~3KB |

---

### 3. Postmortem Blog Posts (from kubernetes-failure-stories + extras)

#### Directly fetchable (OK)

| # | Source | URL | Status | Size | Topics |
|---|--------|-----|--------|------|--------|
| 1 | Blue Matador | `https://www.bluematador.com/blog/post-mortem-kubernetes-node-oom` | OK | — | Node OOM, fluentd, no resource limits |
| 2 | erickhun (Buffer) | `https://erickhun.com/posts/kubernetes-faster-services-no-cpu-limits/` | OK | 34KB | CPU limits, throttling, kops |
| 3 | deploy.live conntrack | `https://deploy.live/blog/kubernetes-networking-problems-due-to-the-conntrack/` | OK | 22KB | GKE, conntrack, HAProxy |
| 4 | deploy.live IP exhaustion | `https://deploy.live/blog/when-gke-ran-out-of-ip-addresses/` | OK | 19KB | GKE, cluster autoscaler, HPA, VPC |
| 5 | deploy.live GKE upgrade | `https://deploy.live/blog/the-shipwreck-of-gke-cluster-upgrade/` | OK | 30KB | GKE upgrade, pod availability, ingress |
| 6 | srvaroa latency | `https://srvaroa.github.io/kubernetes/migration/latency/dns/java/aws/microservices/2019/10/22/kubernetes-added-a-0-to-my-latency.html` | OK | 25KB | KIAM, DNS, AWS IAM, 10x latency |
| 7 | philpearl ingress | `https://philpearl.github.io/post/k8s_ingress/` | OK | 6KB | GKE, Ingress, 502 errors |
| 8 | keepingitclassless NRE | `https://keepingitclassless.net/2018/12/december-4-nre-labs-outage-post-mortem/` | OK | 42KB | GCP, kubeadm, etcd, livenessProbe |
| 9 | adammargherio DNS | `https://www.adammargherio.com/a-perfect-dns-storm/` | DEAD | — | Returns blog index, not article |
| 10 | itnext ELB | `https://itnext.io/kubernetes-and-the-menace-elb-the-tale-of-an-outage-c00bef678fc0` | DEAD | — | SSL certificate error |
| 11 | tibobeijen cluster fail | `https://www.tibobeijen.nl/2019/02/01/learning-from-kubernetes-cluster-failure/` | OK | 24KB | AWS, SystemOOM, no resource limits |
| 12 | jetstack webhook | `https://blog.jetstack.io/blog/gke-webhook-outage` | DEAD | — | Redirects to CyberArk marketing (Venafi acquired) |
| 13 | pieterlange dex | `https://pieterlange.github.io/failure-stories/2019-06.dex.html` | OK | 10KB | etcd, apiserver, dex, CRDs |
| 14 | dev.to Spark OOM | `https://dev.to/pranavbhasker/postmortem-eliminating-oom-failures-in-spark-on-kubernetes-azure-after-cloud-migration-5fia` | OK | 80KB | Spark, AKS, OOM |
| 15 | Robusta CPU limits | `https://home.robusta.dev/blog/stop-using-cpu-limits` | OK | 60KB | CPU limits analysis |
| 16 | Grafana pod priorities | `https://grafana.com/blog/2019/07/24/how-a-production-outage-was-caused-using-kubernetes-pod-priorities/` | OK | 495KB | Pod priorities, cascading eviction |
| 17 | Prezi crime story | `https://engineering.prezi.com/https-engineering-prezi-com-a-kubernetes-crime-story-2e8d75a77630` | DEAD | — | SSL certificate error |
| 18 | CNCF Fluentd migration | `https://www.cncf.io/blog/2025/10/01/fluentd-to-fluent-bit-a-migration-guide/` | OK | 170KB | Fluentd to Fluent Bit |
| 19 | yashmehrotra missing packet | `https://yashmehrotra.com/posts/the-case-of-the-missing-packet-an-eks-migration-tale/` | OK (redir) | — | EKS, AWS CNI |

#### Requires manual browser access (Medium 403)

| # | Source | URL | Topics |
|---|--------|-----|--------|
| 20 | Skyscanner chars | `https://medium.com/@SkyscannerEng/how-a-couple-of-characters-brought-down-our-site-356ccaf1fbc3` | GitOps, templating, namespace deletion |
| 21 | Preply DNS | `https://medium.com/preply-engineering/dns-postmortem-e169efd45afd` | conntrack, DNS, CoreDNS |
| 22 | Omio CPU throttling | `https://medium.com/omio-engineering/cpu-limits-and-aggressive-throttling-in-kubernetes-c5b20bd8a718` | GKE, CPU limits, CFS throttling |
| 23 | MindTickle delays | `https://medium.com/techmindtickle/intermittent-delays-in-kubernetes-e9de8239e2fa` | conntrack DNAT/SNAT, DNS |
| 24 | Istio shallow water | `https://medium.com/@jakubkulich/sailing-with-the-istio-through-the-shallow-water-8ae81668381e` | Istio, GKE, proxy injection |
| 25 | JW Player cryptominer | `https://medium.com/jw-player-engineering/how-a-cryptocurrency-miner-made-its-way-onto-our-internal-kubernetes-clusters-9b09c4704205` | Weave Scope, security, resource theft |
| 26 | Civis breaking K8s | `https://medium.com/civis-analytics/https-medium-com-civis-analytics-breaking-kubernetes-how-we-broke-and-fixed-our-k8s-cluster-adfa6fbade61` | AWS, kops, large clusters, CPU throttling |
| 27 | Skyscanner templating | `https://medium.com/@SkyscannerEng/misunderstanding-the-behaviour-of-one-templating-line-and-the-pain-it-caused-our-k8s-clusters-a420f30a99f1` | HAProxy-Ingress, Golang templating |
| 28 | Target cascading failure | `https://medium.com/@daniel.p.woods/on-infrastructure-at-scale-a-cascading-failure-of-distributed-systems-7cff2a3cd2df` | on-premise, Kafka, cascading failure |
| 29 | Reddit Pi Day | `https://www.reddit.com/r/RedditEng/comments/11xx5o0/you_broke_reddit_the_piday_outage/` | Calico CNI, upgrades |

#### Dead URLs

| # | Source | URL | Issue |
|---|--------|-----|-------|
| 30 | sethmccombs | `https://sethmccombs.github.io/work/2018/12/03/Outages.html` | 404 |
| 31 | devops-hof | `https://www.devops-hof.de/kubernetes-load-balancer-konfiguration-beware-when-draining-nodes/` | Timeout → recovered as `docs/medium/nodedrain.txt` (generic article, low value) |
| 32 | airmap | `https://www.airmap.com/incident-180719/` | Timeout |
| 33 | prometheuskube | `https://prometheuskube.com/why-we-switched-from-fluent-bit-to-fluentd-in-2-hours` | DNS dead |
| 34 | gravitational PG | `https://gravitational.com/blog/running-postgresql-on-kubernetes/` | 403 |
| 35 | saltside outage | `https://engineering.saltside.se/our-first-kubernetes-outage-c6b9249cfd3a` | 404 → not recovered (different article) |
| 36 | saltside migration | `https://engineering.saltside.se/our-failure-migrating-to-kubernetes-25c28e6dd604` | 404 → recovered as `docs/medium/saltside.txt` (ELB migration failure) |

#### GitHub Postmortems (raw incident reports, all fetchable)

| # | Source | Raw URL | Status |
|---|--------|---------|--------|
| 37 | Zalando kubelet QPS | `https://raw.githubusercontent.com/zalando-incubator/kubernetes-on-aws/dev/docs/postmortems/jun-2019-kubelet-qps.md` | OK |
| 38 | Zalando DNS outage | `https://raw.githubusercontent.com/zalando-incubator/kubernetes-on-aws/dev/docs/postmortems/jan-2019-dns-outage.md` | OK |
| 39 | FREE NOW workers | `https://raw.githubusercontent.com/freenowtech/postmortems/master/2019-09-19%20-%20New%20K8s%20workers%20unable%20to%20join%20cluster.pdf` | OK (PDF) |

#### Incident Status Pages (all fetchable)

| # | Source | URL | Status |
|---|--------|-----|--------|
| 40 | Monzo outage | `https://community.monzo.com/t/resolved-current-account-payments-may-fail-major-outage-27-10-2017/26296/95` | OK |
| 41 | SaleMove | `https://status.salemove.com/incidents/xf6cr710yrzn` | OK |
| 42 | Moonlight | `https://updates.moonlightwork.com/outage-post-mortem-87370` | OK |
| 43 | Universe | `http://status.universe.com/incidents/115n3vxqwzcf` | OK |

#### KubeCon / Conference Talks (YouTube, all loadable)

| # | Talk | URL | Topics |
|---|------|-----|--------|
| 44 | Airbnb "10 More Weird Ways" 2020 | `https://www.youtube.com/watch?v=4CT0cI62YHk` | MutatingWebhook, CPU Limits, OOMKill, HPA |
| 45 | Airbnb "10 Weird Ways" 2019 | `https://www.youtube.com/watch?v=FrQ8Lwm9_j8` | Sidecars, DaemonSet, JVM, HPA |
| 46 | Airbnb "p95s Worse" 2019 | `https://www.youtube.com/watch?v=QXApVwRBeys` | CPU Limit, Throttling, DNS |
| 47 | Datadog "10 Ways to Shoot" | `https://www.youtube.com/watch?v=QKI-JRs2RIE` | CoreDNS, IPVS, OOMKill, PodPriority |
| 48 | Spotify "Deleted All Clusters" | `https://www.youtube.com/watch?v=ix0Tw8uinWs` | GKE, Terraform, cluster deletion |
| 49 | Zalando "Failure Stories" | `https://www.youtube.com/watch?v=6sDTB4eV4F8` | OOMKill, CronJob, CPU throttling |
| 50 | Pusher "Config Changed" | `https://www.youtube.com/watch?v=8P7-C44Gjj8` | AWS, nginx, ConfigMap |
| 51 | Algolia "Black Friday" | `https://www.youtube.com/watch?v=Fjyg7cxRZQs` | GKE, Jobs, overload |
| 52 | Monzo "Production Outage" | `https://www.youtube.com/watch?v=OUYTNywPk-s` | etcd, Linkerd, gRPC |
| 53 | Google "Stories from Playbook" | `https://youtu.be/N2JUGnwinbQ` | GKE, etcd, Docker, dnsmasq |
| 54 | Yahoo "101 Ways Break/Recover" | `https://www.youtube.com/watch?v=likHm-KHGWQ` | namespace deletion, OOM, DNS, etcd |
| 55 | Nordstrom "101 Ways Crash" | `https://www.youtube.com/watch?v=xZO9nx6GBu0` | OOM, eviction, ELB, etcd split |
| 56 | ThredUP "Moving Entire Stack" | `https://www.youtube.com/watch?v=tA8Sr3Nsx1I` | HAProxy, DNS, livenessProbe |
| 57 | Google "How NOT to do K8s" | `https://www.youtube.com/watch?v=V0DVkrHf08k` | Container registry, ingress, replicas |
| 58 | Xing "Bad and Ugly" | `https://www.youtube.com/watch?v=MoIdU0J0f0E` | nginx, conntrack, PLEG, stuck controllers |
| 59 | Zalando "Crash Your Cluster" | `https://www.youtube.com/watch?v=LpFApeaGv7A` | OOMKill, CronJob, CoreDNS, CPU throttling |

#### SlideShare (all loadable, limited text extraction)

| # | Talk | URL | Status |
|---|------|-----|--------|
| 60 | Zalando Hamburg meetup | `https://www.slideshare.net/try_except_/lets-talk-about-failures-with-kubernetes-hamburg-meetup` | OK |
| 61 | Zalando DevOpsCon | `https://www.slideshare.net/try_except_/running-kubernetes-in-production-a-million-ways-to-crash-your-cluster-devopscon-munich-2018` | OK |
| 62 | Zalando AWS fallacies | `https://www.slideshare.net/RaffaeleDiFazio/fallacies-of-distributed-computing-with-kubernetes-on-aws` | OK |

#### danluu/post-mortems — K8s-relevant entries (from curated index)

Source: `https://github.com/danluu/post-mortems` (README.md). Broad curated postmortem collection. K8s/infrastructure-relevant entries below. Most do NOT overlap with kubernetes-failure-stories.

**HIGH relevance (directly K8s/container):**

| # | Company | URL | Status | Extracted |
|---|---------|-----|--------|-----------|
| 64 | Datadog | `https://www.datadoghq.com/blog/2023-03-08-multiregion-infrastructure-connectivity-issue/` | OK | YES — systemd v249 deleted Cilium routes, tens of thousands of nodes, 48h recovery, 450-750 responders |
| 65 | Allegro | `https://allegro.tech/2018/08/postmortem-why-allegro-went-down.html` | DEAD | Returns Next.js redirect shell only |
| 66 | PagerDuty | `https://status.pagerduty.com/incidents/vbp7ht2647l8` | DEAD | Returns config page, not incident report |

**MEDIUM relevance (infrastructure/cloud with K8s-applicable lessons):**

| # | Company | URL | Status | Extracted |
|---|---------|-----|--------|-----------|
| 67 | Amazon Kinesis | `https://aws.amazon.com/message/11201/` | OK | YES — OS thread limit exceeded, ~20h recovery, fleet size scaling |
| 68 | Amazon | `https://aws.amazon.com/message/12721/` | OK | YES — scaling overwhelmed networking devices, ~11h recovery, retry storm |
| 69 | Heroku | `https://status.heroku.com/incidents/2451` | DEAD | Returns empty page header |
| 70 | Discord | `https://discordstatus.com/incidents/dj3l6lw926kl` | OK | YES — CPU soft lockups → split brain → thundering herd → OOM, 5h16m |
| 71 | Buildkite | `https://buildkite.com/blog/outage-post-mortem-for-august-22nd` | OK | YES — m4.10xlarge→r3.2xlarge cost-cut, health check DB dependency, 6h spiral |
| 72 | Slack | `https://slack.engineering/slacks-outage-on-january-4th-2021/` | OK | YES — 1200 servers scaled, TGW saturated, autoscaler downscaled on low CPU, file descriptor limit |
| 73 | Slack | `https://slack.engineering/slacks-incident-on-2-22-22/` | OK | YES — Consul rollout → memcached churn → cache miss → DB timeout → metastable loop |
| 74 | Amazon EBS | `https://aws.amazon.com/message/680342/` | OK | YES — agent memory leak from DNS propagation failure, correlated server exhaustion |

Note: Reddit Pi Day entry already in our list (#29). All extracted data saved to `docs/medium/extracted_postmortems.txt`.

#### Local files (kubernetes-failure-stories index)

| # | File | Location | Content |
|---|------|----------|---------|
| 75 | Failure stories README | `docs/kubernetes-failure-stories-main.zip` → `README.md` | 204 lines, ~45 indexed stories with categories and links |

---

### 4. Industry Reports

#### Local PDFs (provided by user)

| # | Report | Location | Content |
|---|--------|----------|---------|
| 1 | Dynatrace "K8s in the Wild 2025" | `docs/bae16112-ebk-k8s-in-the-wild.pdf` | 810 lines extracted. Cluster stats, node sizes, pod hours, runtime languages, workload distribution |
| 2 | Sysdig 2025 Cloud-Native Report | `docs/Sysdig-Cloud-Native-Security-Report-2025.pdf` | 1406 lines extracted. Container lifespan (60% <5min), vulnerability stats, AI/ML growth (500%), permissions waste (98% unused) |

#### Fetchable web reports

| # | Source | URL | Status | Useful Data |
|---|--------|-----|--------|-------------|
| 3 | Cast.ai K8s Cost Benchmark | `https://cast.ai/kubernetes-cost-benchmark/` | OK | YES — real % waste figures |
| 4 | Sysdig "Millions Wasted" | `https://www.sysdig.com/blog/millions-wasted-kubernetes` | OK | YES — waste %, utilization |
| 5 | CNCF Annual Survey 2024 | `https://www.cncf.io/reports/cncf-annual-survey-2024/` | OK | YES — adoption stats |
| 6 | Datadog rightsize blog | `https://www.datadoghq.com/blog/rightsize-kubernetes-workloads/` | OK | YES — right-sizing guide |
| 7 | Datadog State of Containers | `https://www.datadoghq.com/state-of-containers-and-serverless/` | GATED | Landing page, report behind form |
| 8 | Datadog State of Cloud Costs | `https://www.datadoghq.com/state-of-cloud-costs/` | GATED | Landing page, report behind form |
| 9 | Sysdig 2025 report page | `https://sysdig.com/2025-cloud-native-security-and-usage-report/` | GATED | Landing page (but we have PDF locally) |
| 10 | learnkube.com archive | `https://learnkube.com/archive` | OK | K8s learning resource index |

---

### 5. Runtime Documentation (from web search, all confirmed free)

| Runtime | Source | URL | Status |
|---------|--------|-----|--------|
| **JVM** | Red Hat Java 17 container awareness | `https://developers.redhat.com/articles/2022/04/19/java-17-whats-new-openjdks-container-awareness` | Free |
| | merikan.com JVM-in-a-container | `https://www.merikan.com/2019/04/jvm-in-a-container/` | Free |
| | DZone Java memory args | `https://dzone.com/articles/best-practices-java-memory-arguments-for-container` | Free |
| **Go** | Ardan Labs GOMEMLIMIT + K8s | `https://www.ardanlabs.com/blog/2024/02/kubernetes-memory-limits-go.html` | Free |
| | howardjohn GOMAXPROCS/GOMEMLIMIT | `https://blog.howardjohn.info/posts/gomaxprocs/` | Free |
| | Go GC guide | `https://tip.golang.org/doc/gc-guide` | OK |
| | automaxprocs README | `https://raw.githubusercontent.com/uber-go/automaxprocs/master/README.md` | OK |
| | kupczynski container-aware Go | `https://kupczynski.info/posts/go-container-aware/` | Free |
| **Node.js** | Red Hat Node.js 20 memory | `https://developers.redhat.com/articles/2025/10/10/nodejs-20-memory-management-containers` | Free |
| | goldbergyoni best practices | `https://github.com/goldbergyoni/nodebestpractices/blob/master/sections/docker/memory-limit.md` | Free |
| | nodeshift container reference | `https://nodeshift.dev/nodejs-reference-architecture/development/building-good-containers/` | Free |
| **.NET** | MS DevBlogs GCHeapHardLimit Pt0 | `https://devblogs.microsoft.com/dotnet/running-with-server-gc-in-a-small-container-scenario-part-0/` | Free |
| | MS DevBlogs GCHeapHardLimit Pt1 | `https://devblogs.microsoft.com/dotnet/running-with-server-gc-in-a-small-container-scenario-part-1-hard-limit-for-the-gc-heap/` | Free |
| | MS Learn GC runtime config | `https://learn.microsoft.com/en-us/dotnet/core/runtime-config/garbage-collector` | OK |
| | dotnet/designs memory-limits | `https://github.com/dotnet/designs/blob/main/accepted/2019/support-for-memory-limits.md` | Free |
| | markvincze .NET memory K8s | `https://blog.markvincze.com/troubleshooting-high-memory-usage-with-asp-net-core-on-kubernetes/` | Free |
| | Thinktecture .NET 8 DATAS | `https://www.thinktecture.com/en/net/optimize-asp-net-core-memory-with-datas/` | Free |
| | MS Learn Workstation vs Server | `https://learn.microsoft.com/en-us/dotnet/standard/garbage-collection/workstation-server-gc` | Free |
| **Python** | ragoragino.dev memory debugging | `https://ragoragino.dev/tech/2024-12-01-python-memory/` | Free |
| | BetterUp jemalloc RSS fix | `https://build.betterup.com/chasing-a-memory-leak-in-our-async-fastapi-service-how-jemalloc-fixed-our-rss-creep/` | Free |
| | dev.to Celery K8s memory | `https://dev.to/redhap/python-celery-kubernetes-and-memory-2old` | Free |
| | Celery School prefork pool | `https://celery.school/the-prefork-worker-pool` | Free |
| | Official Celery workers docs | `https://docs.celeryq.dev/en/stable/userguide/workers.html` | Free |
| **Rust** | kerkour.com Rust jemalloc | `https://kerkour.com/rust-jemalloc` | Free |
| | dev.to allocator comparison | `https://dev.to/frosnerd/libmalloc-jemalloc-tcmalloc-mimalloc-exploring-different-memory-allocators-4lp3` | Free |
| | raniz.blog Rust MUSL malloc | `https://raniz.blog/2025-02-06_rust-musl-malloc/` | Free |

---

### 6. Infrastructure Documentation (all confirmed OK)

| Topic | Source | URL | Status |
|-------|--------|-----|--------|
| cgroups v2 memory QoS | K8s blog | `https://kubernetes.io/blog/2023/05/05/qos-memory-resources/` | OK |
| cgroups v2 overview | K8s docs | `https://kubernetes.io/docs/concepts/architecture/cgroups/` | OK |
| CFS bandwidth | Linux kernel docs | `https://docs.kernel.org/scheduler/sched-bwc.html` | OK |
| cgroups v2 hands-on | linuxera.org | `https://linuxera.org/cpu-memory-management-kubernetes-cgroupsv2/` | Free |
| Kubelet eviction | K8s docs | `https://kubernetes.io/docs/concepts/scheduling-eviction/node-pressure-eviction/` | OK |
| Karpenter disruption | karpenter.sh | `https://karpenter.sh/docs/concepts/disruption/` | OK |
| Karpenter migration | karpenter.sh | `https://karpenter.sh/docs/getting-started/migrating-from-cas/` | OK |
| Karpenter vs CA | Spacelift | `https://spacelift.io/blog/karpenter-vs-cluster-autoscaler` | Free |
| Karpenter EKS workshop | AWS | `https://www.eksworkshop.com/docs/autoscaling/compute/karpenter/consolidation` | Free |
| CRI-O vs containerd | GitHub issue | `https://github.com/cri-o/cri-o/issues/8282` | Free |

---

### 7. Cloud Provider Documentation (all confirmed OK)

| Provider | Source | URL | Status |
|----------|--------|-----|--------|
| **AWS** | EKS best practices | `https://aws.github.io/aws-eks-best-practices/cost_optimization/cost_opt_compute/` | OK |
| | Kubecost EKS add-on | `https://aws.amazon.com/blogs/containers/dynamic-kubernetes-request-right-sizing-with-kubecost/` | Free |
| | Right-sizing whitepaper | `https://docs.aws.amazon.com/whitepapers/latest/cost-optimization-right-sizing/tips-for-right-sizing-your-workloads.html` | Free |
| **GCP** | Autopilot resource requests | `https://cloud.google.com/kubernetes-engine/docs/concepts/autopilot-resource-requests` | OK |
| | Autopilot overview | `https://cloud.google.com/kubernetes-engine/docs/concepts/autopilot-overview` | OK |
| | Performance pods | `https://cloud.google.com/kubernetes-engine/docs/how-to/performance-pods` | OK |
| **Azure** | AKS DevSecOps workshop | `https://azure.github.io/AKS-DevSecOps-Workshop/modules/Module5/Lab01.html` | Free |
| | AKS checklist | `https://www.the-aks-checklist.com/docs/AKS-Checklist.pdf` | Free |

---

### 8. Competitor Analysis

#### K8_AI_Query_Agent (`github.com/johnwroge/K8_AI_Query_Agent`)

Python Flask + OpenAI GPT-4o-mini wrapper for K8s natural language queries and pod crash debugging. **NOT a training data source** — no domain-specific resource constants or optimization knowledge. Useful for competitive positioning.

**Architecture comparison:**

| Aspect | K8_AI_Query_Agent | k8s-sage |
|--------|-------------------|----------|
| Purpose | General K8s troubleshooting | Resource efficiency optimization |
| AI | OpenAI GPT-4o-mini (cloud API, per-call cost) | Fine-tuned SLM (in-cluster, CPU-only, free) |
| Language | Python + Flask | Go |
| Agent resources | 250m CPU / 256Mi req → 500m/512Mi lim | 50m CPU / 50MB (5x lighter) |
| Pattern detection | CrashLoopBackOff, OOM, ImagePull (keyword match) | Steady/burstable/batch/idle/anomalous (statistical) |
| Output | Debugging advice, kubectl commands | Right-sizing recs with confidence + safety floors |
| Data collection | On-demand (no persistence) | Continuous DaemonSet metrics |
| Safety | Fallback if GPT fails (generic advice) | L1 rules engine: 50m floor, 64Mi floor, reduction caps |
| Dependency | Requires OPENAI_API_KEY (external) | Self-contained |

**Key code observations:**
- `debug_assistant.py`: Deterministic pattern detection (CrashLoopBackOff, OOMKilled, ImagePull, exit codes, log keyword matching for database/permission/network/config) → sends to GPT-4o for analysis at temperature=0.3 with max_tokens=2000
- `k8s_analyzer.py`: Gets pod details + current/previous logs (100 lines) + events (30min window)
- `ai_service.py`: Dumps full cluster JSON into system prompt, caps at 50 resources per type
- `deployment.yaml`: 250m/256Mi requests, 500m/512Mi limits; RBAC: pods/nodes/services/deployments get+list

**Training data value**: LOW (0-3 pairs). Possible pairs:
- "Why is a K8s-specific SLM better than a GPT wrapper for resource optimization?" (k8s-sage differentiator)
- "What resources does a typical K8s AI debugging tool need?" (250m/256Mi as benchmark)
- "Compare reactive debugging (K8_AI_Query_Agent) vs proactive right-sizing (k8s-sage)"

**Competitive takeaway**: This project validates the market (people want AI-powered K8s tools) but demonstrates exactly why a general-purpose LLM wrapper is insufficient: 5x more resources, per-API-call cost, no domain training, no persistent metrics, no safety invariants. k8s-sage's SLM approach is fundamentally different.

---

## Batch Execution Plan

### Schema (applies to all batches)

```json
{
  "id": "{prefix}-{category}-{NNN}",
  "source": "synthetic",
  "system": "You are k8s-sage, a Kubernetes resource efficiency advisor...",
  "user": "question/scenario",
  "assistant": "analysis/recommendation",
  "metadata": {
    "category": "right-sizing|classification|runtime-specific|edge-case",
    "provenance": "description of data source",
    "grounded_source": "URL or training_knowledge"
  }
}
```

---

### Batch 1: Helm Chart Real Defaults
**Sessions**: 1 (4 sub-batches due to file sizes)
**Est. pairs**: 100-150
**Sources**: 20 values.yaml files (all OK)

| Sub-batch | Charts | Total Size |
|-----------|--------|-----------|
| 1a | kube-prometheus-stack, prometheus, redis, postgresql, mongodb | ~548KB |
| 1b | mysql, kafka, rabbitmq, elasticsearch, ingress-nginx | ~403KB |
| 1c | cert-manager, argo-cd, grafana, vault, istio | ~303KB |
| 1d | linkerd, traefik, jenkins, airflow, spark-operator | ~270KB |

Per chart: extract resource defaults → generate 5-8 pairs (defaults Q&A, production sizing, OOM scenarios, contention analysis).

---

### Batch 2: VPA + Tools + Autoscaler Source Code Deep Dive
**Sessions**: 2
**Est. pairs**: 160-210
**Sources**: 10 VPA Go files + 5 kube-resource-report Python files + 6 KEDA Go files + 3 prometheus-adapter files + READMEs/docs

Extract: percentile constants, confidence multipliers, decay rates, OOM bump logic, allocation model.

#### kube-resource-report (Zalando) — Verified Constants

Cloned from `https://codeberg.org/hjacobs/kube-resource-report` (Python, MIT license). Key extractable ground truth:

| File | Constant | Value | k8s-sage Comparison |
|------|----------|-------|---------------------|
| `recommender.py` | CPU_PERCENTILE | 0.9 (P90) | k8s-sage uses P95 |
| `recommender.py` | CPU_SAFETY_MARGIN_FACTOR | 1.15 | k8s-sage uses 1.20 |
| `recommender.py` | MEMORY_PERCENTILE | 1.0 (P100/Max) | k8s-sage steady=P95, burst/batch=P99/Max |
| `recommender.py` | MEMORY_SAFETY_MARGIN_FACTOR | 1.15 | k8s-sage uses 1.20 |
| `recommender.py` | DEFAULT_CPU_RECOMMENDATION | 1m | k8s-sage floor 50m |
| `recommender.py` | DEFAULT_MEMORY_RECOMMENDATION | 100Mi | k8s-sage floor 64Mi |
| `histogram.py` | BUCKET_RATIO | 1.05 | VPA-compatible exponential histogram |
| `histogram.py` | DECAY_HALF_LIFE | 86400s (24h) | Same as VPA default |
| `histogram.py` | Memory peak retention | ~53 days (in tests) | Derived from decay + histogram |
| `pricing.py` | Cost model | Dominant resource: `max(cpu_cost, memory_cost)` | Novel approach for training |
| `pricing.py` | DEFAULT_SPOT_DISCOUNT | 0.60 | Real-world spot pricing benchmark |
| `pricing.py` | GCP extended memory threshold | 8 GiB/vCPU | Cloud-specific constraint |
| `metrics.py` | EMA default alpha | 1.0 (no smoothing) | Latest-sample weighting |
| `query.py` | Cost attribution | 50/50 CPU/memory split | Simplistic vs dominant-resource |
| `query.py` | Slack formula | `request_cost - recommendation_cost` | Waste calculation |

**Training pair themes from this source (~20-25 pairs):**
- Comparing recommendation algorithms (P90/1.15x vs P95/1.20x — why Zalando chose less aggressive)
- Memory safety: why P100 + margin for memory (OOM is harder to recover from than CPU throttling)
- Histogram decay: how 24h half-life handles workload pattern changes
- Cost models: dominant resource vs proportional allocation
- Spot pricing: how 60% discount factors into right-sizing ROI
- Tool comparison: kube-resource-report vs VPA vs k8s-sage approach differences
- Slack/waste calculation methodology

#### KEDA (kedacore/keda) — Verified Constants

Cloned from `https://github.com/kedacore/keda` (Go, Apache 2.0). Key extractable ground truth:

| Constant | Value | File |
|----------|-------|------|
| defaultPollingInterval | 30s | apis/keda/v1alpha1/withtriggers_types.go |
| defaultCooldownPeriod | 300s (5min) | pkg/scaling/executor/scale_executor.go |
| defaultHPAMinReplicas | 1 | apis/keda/v1alpha1/scaledobject_types.go |
| defaultHPAMaxReplicas | 100 | apis/keda/v1alpha1/scaledobject_types.go |
| Operator CPU request/limit | 100m / 1000m | config/manager/manager.yaml |
| Operator memory request/limit | 100Mi / 1000Mi | config/manager/manager.yaml |
| Metrics server CPU request/limit | 100m / 1000m | config/metrics-server/deployment.yaml |
| Metrics server memory request/limit | 100Mi / 1000Mi | config/metrics-server/deployment.yaml |

Key algorithms: scale-to-zero logic, cooldown enforcement (`now > lastActiveTime + cooldownPeriod`), fallback mechanism (4 behaviors: static/currentReplicas/currentReplicasIfHigher/Lower), pause annotations for scale-in/scale-out blocking, formula-based scaling modifiers.

**Training pair themes (~25-30 pairs):**
- KEDA vs HPA vs VPA: when to use which autoscaler
- Event-driven scaling patterns (Kafka lag, queue depth, HTTP requests)
- Cooldown tuning: preventing flapping under intermittent load
- Scale-to-zero mechanics and activation thresholds
- KEDA operator resource overhead (100m/100Mi baseline)
- Fallback strategies when external metrics fail
- Combining KEDA with VPA for event-driven right-sizing

#### prometheus-adapter — Verified Constants

Cloned from `https://github.com/kubernetes-sigs/prometheus-adapter` (Go, Apache 2.0). Key extractable ground truth:

| Constant | Value | File |
|----------|-------|------|
| MetricsRelistInterval | 10min | cmd/adapter/adapter.go |
| CPU query window | 5m (irate) | config-map.yaml |
| Memory metric | container_memory_working_set_bytes (gauge) | pkg/config/utils/default.go |
| CPU metric | container_cpu_usage_seconds_total (rate) | pkg/config/utils/default.go |
| Adapter CPU request | 102m | deploy/ |
| Adapter memory request | 180Mi | deploy/ |
| Container filter | container!="POD" (excludes pause containers) | default.go |

Key algorithms: Prometheus series → K8s custom/external metrics API conversion, irate vs rate window selection, label-to-resource mapping (pod/namespace/node), metric discovery caching every 10min.

**Training pair themes (~15-20 pairs):**
- How custom metrics flow from Prometheus to HPA decisions
- Rate window tuning: 2m (responsive) vs 5m (smooth) vs 10m (stable)
- prometheus-adapter resource overhead vs benefit
- Container filter importance (excluding pause/POD containers from metrics)
- Discovery interval impact on HPA responsiveness (~10min lag for new metrics)
- Combining prometheus-adapter with KEDA for hybrid autoscaling

---

### Batch 3: Postmortems — Fetchable Blog Posts + danluu
**Sessions**: 2
**Est. pairs**: 95-130
**Sources**: 14 fetchable blog posts (5 now dead) + 2 GitHub postmortems + 8 danluu entries
**Extraction**: COMPLETE — all data saved to `docs/medium/extracted_postmortems.txt`

| Sub-batch | Sources | Focus | Est. Pairs |
|-----------|---------|-------|-----------|
| 3a | Blue Matador, erickhun, deploy.live x3, srvaroa, NRE Labs, tibobeijen | OOM, CPU throttling, conntrack, DNS, IP, liveness probes | ~45-55 |
| 3b | Grafana priorities, Datadog Cilium, Zalando x2, Spark, Discord, Slack x2, Buildkite, AWS x3, EBS | Cascading failures, scaling, cost-cutting | ~50-75 |

Dead/unavailable (7 URLs): adammargherio, itnext, jetstack, Prezi (SSL), Allegro (JS shell), PagerDuty (config page), Heroku (empty)

Per postmortem: extract trigger, resource values, cascade, fix → generate 3-5 pairs.

---

### Batch 4: Postmortems + Cost Guides — Medium + Manual Content (all populated)
**Sessions**: 1-2
**Est. pairs**: 70-100
**Status**: All 14 files populated with content in `docs/medium/`

#### HIGH value (5-8 pairs each)

| File | Size | Topic | Extractable Training Data |
|------|------|-------|--------------------------|
| `omio.txt` | 11KB | CFS CPU throttling (GKE) | CFS quota mechanics: cfs_period_us=100ms, cfs_quota_us; 10 threads on 16-core → quota exhausted in 20ms, throttled 80ms; 50% over-throttle rate; fix: remove CPU limits; before/after 5xx and p95 latency graphs |
| `civis.txt` | 13KB | Scaling K8s to 700 nodes (AWS) | Master sizing: m4.large→m4.4xlarge (2→16 CPU, 8→64 GiB); API server unbounded CPU chewed through node; DaemonSet API polling (Datadog metadata tags); rate limit 400→1200; etcd IOPS 100→300; kube2iam OOM from storing all pod metadata; DNS ClusterFirst→Default at scale |
| `reddit.txt` | 23KB | Pi Day outage (K8s 1.24 upgrade) | Calico route reflectors used `node-role.kubernetes.io/master` label, removed in 1.24; CRI-O upgrade stuck containers; OPA admission controller timeouts; etcd backup/restore with TLS cert mismatch; thundering herd ramp: 1%→5%→10%→20%→35%→55%→80%→100%; 314-min outage |
| `danveloper.txt` | 9KB | Cascading failure at Target | 2000 workloads on one cluster; Kafka outage→logging sidecar CPU spike→docker daemon overload→node unhealthy→K8s reschedule cascade; 41,000 new pods spawned; Consul gossip mesh poisoned; "smaller clusters, more of them" |
| `mindtickle.txt` | 11KB | DNS 5s/10s/15s delays | conntrack DNAT/SNAT race condition; A+AAAA on same port; single-request-reopen fix; gRPC: GRPC_DNS_RESOLVER=native; Alpine musl vs glibc; tcpdump evidence of port reuse |

#### MEDIUM value (3-5 pairs each)

| File | Size | Topic | Extractable Training Data |
|------|------|-------|--------------------------|
| `prepl.txt` | 6KB | Preply DNS postmortem | Formal postmortem: kube-proxy failed conntrack table delete; CoreDNS autoscaler 3→2 pods triggered outage; 15K events dropped; TTM 26min; fix: disable CoreDNS autoscaler, add DNS cache |
| `skyscanner.txt` | 9KB | Global outage (ArgoCD + template error) | Missing `{{ }}` in template → ArgoCD deleted 478 services globally; cells architecture: n+2, spot instances; 4.5h outage; recovery via GitOps reconciliation |
| `skyscanner2.txt` | 12KB | HAProxy hot-podding → OOMKill cascade | Go text/template sorts map keys deterministically; backends sorted by pod IP → uneven distribution → queue fill → OOMKill → cascade; 80 ingress controller instances as DaemonSet |
| `saltside.txt` | 9KB | K8s migration failure (ELB, Thrift) | ELB BackendConnectionErrors ~10% success rate; client→ELB→NodePort→kube-proxy→pod chain overhead; cross-AZ load balancing impact; Kops 1.6.0 / K8s 1.5.x |
| `istio.txt` | 10KB | Istio integration failures | Sidecar keeps Jobs running forever; StatefulSet + headless service discovery bug; gRPC client-side vs envoy LB; graceful shutdown: sidecar exits before app (preStop hook workaround) |

#### HIGH value — Cloud Cost Guides (8-12 pairs each)

| File | Size | Topic | Extractable Training Data |
|------|------|-------|--------------------------|
| `gke-cost-advice.txt` | 55KB | Google Cloud GKE cost optimization best practices | HPA target formula: `(1-buff)/(1+perc)` e.g. 0.69 for 30% growth + 10% buffer; VPA modes (Off/Initial/Auto) with best practices; CA scale-down behavior; pause pods for over-provisioning (3200m CPU on 4-CPU nodes); Spot VMs up to 91% cheaper; E2 VMs 31% savings vs N1; committed-use up to 70% discount; NodeLocal DNSCache; memory request=limit, CPU limit unset; terminationGracePeriod <10min (CA limit); Metrics Server nanny resize |
| `komodor-cost.txt` | 21KB | Komodor EKS cost optimization guide | Cluster Autoscaler defaults table (scale-down-delay 10min, utilization-threshold 0.5, scan-interval 10s, max-empty-bulk-delete 10) vs cost-optimized values; Karpenter consolidation; EKS control plane $0.10/hr ($73/mo); Spot 60-90% discount; Fargate 30-40% premium; r5 memory-optimized 15-30% savings; Graviton 20-40% better price-perf; gp2→gp3 20% savings; cross-AZ $0.01-0.02/GB |

#### LOW value (1-2 pairs each, if any)

| File | Size | Topic | Notes |
|------|------|-------|-------|
| `jwplayer.txt` | 15KB | Cryptominer on K8s (Weave Scope) | Security story — miner used 100% CPU/core but pod scheduling unaffected; --privileged flag + root filesystem mount; limited resource training value |
| `nodedrain.txt` | 5KB | Generic node drain best practices | Not a postmortem — generic PDB/graceful-termination advice, no real metrics or incident data |

---

### ~~Batch 5: YouTube Talks~~ — SKIPPED
YouTube IP-blocked transcript extraction. Script at `docs/medium/youtube/extract_all.py` supports `PROXY_URL` env var if revisited later. Dropping 50-80 pairs from estimate.

---

### Batch 6: Runtime Memory Models
**Sessions**: 2
**Est. pairs**: 80-100
**Sources**: ~28 confirmed free URLs across 6 runtimes

| Sub-batch | Runtimes | Sources |
|-----------|----------|---------|
| 6a | JVM, Go, Node.js | 10 URLs |
| 6b | .NET, Python, Rust | 18 URLs |

---

### Batch 7: Infrastructure + Cloud + Industry
**Sessions**: 2
**Est. pairs**: 100-140
**Sources**: 10 infra docs + 8 cloud docs + 2 local PDFs + 4 web reports

| Sub-batch | Focus | Sources |
|-----------|-------|---------|
| 7a | cgroups, CFS, eviction, Karpenter | 10 infra URLs |
| 7b | AWS/GCP/Azure + Dynatrace PDF + Sysdig PDF + Cast.ai + CNCF | 8 cloud URLs + 2 PDFs + 4 report URLs |

---

## Summary

| Batch | Phase | Est. Pairs | Sessions | Source Count | Extraction |
|-------|-------|-----------|----------|-------------|-----------|
| 1 | Helm chart defaults | 100-150 | 1 | 20 values.yaml | Pending |
| 2 | VPA + KEDA + prom-adapter + tools | 160-210 | 2 | 26 source files + docs | Pending |
| 3 | Postmortems (fetchable blogs + danluu) | 95-130 | 2 | 25 extracted (7 dead) | **EXTRACTED** |
| 4 | Postmortems + cost guides (local) | 70-100 | 1-2 | 14 local text files | **EXTRACTED** |
| ~~5~~ | ~~YouTube transcripts~~ | ~~50-80~~ | — | — | **SKIPPED** (IP blocked) |
| 6 | Runtime memory models | 80-100 | 2 | 28 docs/blogs | Pending |
| 7 | Infra + cloud + industry | 100-140 | 2 | 10 + 8 + 2 PDF + 4 reports | Pending |
| | **Total** | **605-830** | **10-12** | **~135 sources** | |

### Manual content status

1. **Medium articles** (10): DONE — all saved to `docs/medium/`
2. **Reddit Pi Day** (1): DONE — saved as `docs/medium/reddit.txt`
3. **Node drain article** (1): DONE — saved as `docs/medium/nodedrain.txt` (generic, low value)
4. **Saltside migration** (1): DONE — saved as `docs/medium/saltside.txt`
5. **SlideShare decks** (3): Still pending — download or screenshot key slides with resource numbers
6. **Datadog gated reports** (2): Still pending — if you have access, download PDFs to docs/

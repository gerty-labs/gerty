# k8s-sage Training Data Expansion — Claude Code Master Prompt

## Context

You are expanding the training dataset for k8s-sage, a small language model (Jamba 3B) fine-tuned on Kubernetes resource right-sizing knowledge. The current dataset has 6,982 pairs with 50.5% synthetic data. The goal is to add ~1,450+ real-world-grounded pairs, dropping synthetic to ~33%.

### Why grounded data matters

The existing AI-generated expert pairs are not bad data. They encode real operational knowledge — the same knowledge found in documentation, blog posts, and Stack Overflow answers that LLMs were trained on. That knowledge is valid.

But there are two problems with stopping there:

1. **Verifiability.** If a pair says "JVM default Xmx is 1/4 of container memory", that's probably correct — but you can't prove it without checking the source. If it's subtly wrong (e.g., the default changed in Java 21, or the fraction applies to physical RAM not the cgroup limit), you'd never catch it. When a pair is generated from a fetched source, you can diff the pair against the source.

2. **Differentiation.** A general-purpose LLM already has the same knowledge as ungrounded expert pairs. The pairs built from fetching actual `values.yaml` files, real postmortem numbers, and real algorithm constants from source code — that's data in a form no other model has. This is what justifies fine-tuning a specialist SLM rather than just prompting a bigger model.

**The existing pairs are the baseline. This grounded expansion is the differentiation layer.**

### Principle

For this expansion: every pair should be grounded in a specific source you have fetched and read in this session. Prioritise verifiable facts — real numbers, real defaults, real incident details, real algorithm constants. Where you are generating analysis or recommendations on top of fetched data, that's fine — the analysis is the model's value-add. But the facts underneath the analysis must trace to a real source.

Where a real source is unavailable (e.g., a report is behind a paywall), you may fall back on your training knowledge, but you must:
- Mark the pair with `"grounded_source": "training_knowledge"` in metadata
- Keep these to a minimum (under 10% of new pairs)
- Prioritise these pairs for human review before training

The project lives at the repo root. Existing generators are in `ml/dataset/` and output JSONL to `ml/dataset/raw/`. Follow the existing patterns in `generate_expert_pairs.py` and `generate_vpa_pairs.py` exactly — stdlib only, deterministic, JSONL output.

---

## Output Schema

Every pair must conform to the existing schema in `ml/dataset/schema.json`. Read it first before generating anything:

```bash
cat ml/dataset/schema.json
```

Each JSONL line has this structure:

```json
{
  "instruction": "The user's question or scenario description with real metrics",
  "response": "The model's analysis and recommendation in structured format",
  "source": "One of: expert | k8s-docs | stackoverflow | github | vpa-source",
  "category": "One of: right-sizing | classification | runtime | comparison | troubleshooting | cost-optimization",
  "metadata": {
    "grounded_source": "URL or file path this pair was derived from",
    "verified": true
  }
}
```

**Source labelling rules:**
- `expert` = pairs derived from real incident postmortems, production blog posts, industry reports, or expert operational knowledge
- `k8s-docs` = pairs derived from official Kubernetes, cloud provider, or tool documentation
- `vpa-source` = pairs derived from the VPA recommender source code
- Do NOT use `synthetic` — every pair in this expansion must be expert-level knowledge, not rules-engine reformulation
- Pairs where a real source was fetched should have `metadata.grounded_source` set to the URL
- Pairs where you fell back on training knowledge should have `metadata.grounded_source` set to `"training_knowledge"` — keep these under 10% of total new pairs

---

## Safety Invariants

These are hard constraints from the rules engine. Every recommendation in every pair MUST respect them. Verify before writing each pair.

| Invariant | Value |
|-----------|-------|
| CPU floor | 50m |
| Memory floor | 64Mi |
| Memory recommendation | >= P99 working set × 1.10 |
| CPU recommendation | >= P95 × headroom (1.20) |
| No zero recommendations | Always |
| Max single-cycle reduction (low confidence) | 30% |
| Max single-cycle reduction (medium confidence) | 50% |
| Max single-cycle reduction (high confidence) | 75% |

If a real-world source recommends values below these floors, the pair's response must note the floor was applied and explain why.

---

## Phase 1: Helm Chart Real Defaults (~250 pairs)

### Step 1: Fetch actual values.yaml files

Fetch ALL of these. Do not skip any. Read the full file, not just the top.

```
https://raw.githubusercontent.com/prometheus-community/helm-charts/main/charts/kube-prometheus-stack/values.yaml
https://raw.githubusercontent.com/prometheus-community/helm-charts/main/charts/prometheus/values.yaml
https://raw.githubusercontent.com/bitnami/charts/main/bitnami/redis/values.yaml
https://raw.githubusercontent.com/bitnami/charts/main/bitnami/postgresql/values.yaml
https://raw.githubusercontent.com/bitnami/charts/main/bitnami/mongodb/values.yaml
https://raw.githubusercontent.com/bitnami/charts/main/bitnami/mysql/values.yaml
https://raw.githubusercontent.com/bitnami/charts/main/bitnami/kafka/values.yaml
https://raw.githubusercontent.com/bitnami/charts/main/bitnami/rabbitmq/values.yaml
https://raw.githubusercontent.com/bitnami/charts/main/bitnami/elasticsearch/values.yaml
https://raw.githubusercontent.com/ingress-nginx/ingress-nginx/main/charts/ingress-nginx/values.yaml
https://raw.githubusercontent.com/cert-manager/cert-manager/master/deploy/charts/cert-manager/values.yaml
https://raw.githubusercontent.com/argoproj/argo-helm/main/charts/argo-cd/values.yaml
https://raw.githubusercontent.com/grafana/helm-charts/main/charts/grafana/values.yaml
https://raw.githubusercontent.com/hashicorp/vault-helm/main/values.yaml
https://raw.githubusercontent.com/istio/istio/master/manifests/charts/istio-control/istio-discovery/values.yaml
https://raw.githubusercontent.com/linkerd/linkerd2/main/charts/linkerd-control-plane/values.yaml
https://raw.githubusercontent.com/traefik/traefik-helm-chart/master/traefik/values.yaml
https://raw.githubusercontent.com/jenkinsci/helm-charts/main/charts/jenkins/values.yaml
https://raw.githubusercontent.com/apache/airflow/main/chart/values.yaml
https://raw.githubusercontent.com/GoogleCloudPlatform/spark-on-k8s-operator/master/charts/spark-operator-chart/values.yaml
```

### Step 2: For each chart, extract

- Exact default CPU requests and limits (or absence of them)
- Exact default memory requests and limits (or absence of them)
- Number of replicas defaulted
- Any resource-related comments in the values.yaml
- Whether resources are set, commented out, or left as `{}`

### Step 3: Generate pairs for each chart

For each of the 20 charts, generate 10-15 pairs covering:

1. **"What are the defaults?"** — State the exact defaults from the fetched values.yaml. If resources are `{}` or commented out, say so explicitly.
2. **"Are these defaults appropriate for production?"** — Analyse whether the defaults are over/under-provisioned for a typical production deployment. Reference the actual values.
3. **"Right-size this chart for a small/medium/large cluster"** — Provide concrete recommendations for 3 scale tiers, always starting from the real defaults.
4. **"This chart is using X CPU / Y memory, is that expected?"** — Present a scenario where observed usage differs from defaults, and analyse why.
5. **"I'm seeing OOM kills on this component"** — Use the real default memory value and explain why it might be insufficient under load.
6. **"How does this chart interact with other charts for resource contention?"** — e.g., Prometheus + Grafana + node-exporter on the same node.

**Verification**: After generating, spot-check 5 random pairs. Confirm the default values cited match what you fetched. If any don't match, regenerate.

Output: `ml/dataset/raw/helm_defaults_pairs.jsonl`

---

## Phase 2: Real Kubernetes Failure Postmortems (~200 pairs)

### Step 1: Fetch the kubernetes-failure-stories index

```
https://raw.githubusercontent.com/hjacobs/kubernetes-failure-stories/master/README.md
```

Also fetch the Codeberg mirror if GitHub is unavailable:
```
https://codeberg.org/hjacobs/kubernetes-failure-stories/raw/branch/master/README.md
```

### Step 2: From the index, fetch the actual blog posts

Prioritise posts that involve resource-related failures. Fetch at least 25-30 of these posts. Focus on posts tagged with or mentioning:
- OOMKill / OOM
- CPU throttling
- Node pressure / eviction
- Cluster autoscaler issues
- Resource quota exhaustion
- Memory leaks in K8s components
- Cascading failures triggered by resource exhaustion

Key posts to definitely fetch (from the index):
- Blue Matador: Post Mortem Kubernetes Node OOM
- Zalando: Running Kubernetes in Production (slides/post)
- Target: On Infrastructure at Scale — A Cascading Failure
- Datadog: 10 Ways to Shoot Yourself in the Foot with Kubernetes
- Any posts mentioning Skipper-Ingress OOMKill
- Any posts mentioning kubelet memory leak
- Any posts mentioning etcd resource issues
- JW Player cryptocurrency miner post (resource theft angle)
- Spotify accidentally deleted clusters (recovery/resource angle)
- Algolia Black Friday post

Also fetch these standalone postmortems not in the index:
```
https://dev.to/pranavbhasker/postmortem-eliminating-oom-failures-in-spark-on-kubernetes-azure-after-cloud-migration-5fia
https://medium.com/@reefland/tracking-down-invisible-oom-kills-in-kubernetes-192a3de33a60
```

### Step 3: For each postmortem, extract

- What actually happened (the trigger)
- What resource values were in play (actual numbers if provided)
- The cascade mechanism (how did it spread)
- The actual fix applied
- What monitoring/metrics revealed the issue

### Step 4: Generate pairs from each postmortem

For each postmortem, generate 5-8 pairs:

1. **Scenario recognition**: "I'm seeing [symptoms from the real incident]. What's happening?" → Response explains the failure pattern, referencing real numbers from the postmortem.
2. **Prevention**: "How do I prevent [this type of failure]?" → Response provides concrete resource configuration advice grounded in what actually fixed the real incident.
3. **Diagnosis**: Present the metrics/symptoms from the real incident and ask for root cause analysis.
4. **Right-sizing connection**: "Given this failure pattern, what should I set my resources to?" → Connect the incident to a k8s-sage recommendation.
5. **Classification**: Present the workload pattern from the incident and ask k8s-sage to classify it (steady/burstable/batch/idle/anomalous).

**Every pair must cite which postmortem it was derived from in the metadata.grounded_source field.**

Output: `ml/dataset/raw/postmortem_pairs.jsonl`

---

## Phase 3: Industry Reports — Real Aggregate Data (~150 pairs)

### Step 1: Fetch these reports

```
https://www.datadoghq.com/state-of-containers-and-serverless/
https://www.datadoghq.com/state-of-cloud-costs/
https://www.sysdig.com/blog/millions-wasted-kubernetes
https://sysdig.com/2025-cloud-native-security-and-usage-report/
https://cast.ai/kubernetes-cost-benchmark/
https://www.dynatrace.com/news/blog/kubernetes-in-the-wild-2023/
https://www.dynatrace.com/news/blog/kubernetes-in-the-wild-2025/
https://www.cncf.io/reports/cncf-annual-survey-2024/
https://wozz.io/blog/kubernetes-memory-overprovisioning-study-2026
```

Also search for and fetch:
- "Cast AI 2025 Kubernetes Cost Benchmark Report" (full report or detailed blog post)
- "Sysdig 2025 Cloud-Native Security and Usage Report"
- "CNCF Annual Survey 2024" results

### Step 2: Extract hard numbers only

From each report, extract only verifiable statistics. Examples of what to extract:
- "X% of CPU requested is unused" (with source)
- "Average CPU utilisation is Y%" (with source)
- "Z% of clusters are over-provisioned" (with source)
- Container lifetime distributions
- Autoscaler adoption rates
- Common resource request ranges by workload type
- Idle cost percentages

Do NOT extract opinions, marketing claims, or vague statements. Only hard data with numbers.

### Step 3: Generate pairs grounded in real stats

For each statistic, generate 2-3 pairs:

1. **"Is my cluster typical?"** — User describes their utilisation, response compares to industry benchmarks with real numbers from the reports.
2. **"How much am I likely wasting?"** — User describes cluster size and broad usage, response estimates waste using real industry percentages.
3. **"What's a realistic utilisation target?"** — Response provides targets grounded in what top-performing organisations actually achieve per the reports.
4. **Cloud-provider-specific**: "I'm on EKS/GKE/AKS, what does the data say about waste on my platform?" — Use real per-platform data where available.
5. **FinOps integration**: "How does right-sizing fit into our FinOps practice?" — Ground in real FinOps framework data.

Output: `ml/dataset/raw/industry_data_pairs.jsonl`

---

## Phase 4: VPA Recommender Deep Dive (~200 pairs)

### Step 1: Fetch the actual VPA recommender source code

```
https://raw.githubusercontent.com/kubernetes/autoscaler/master/vertical-pod-autoscaler/pkg/recommender/logic/recommender.go
https://raw.githubusercontent.com/kubernetes/autoscaler/master/vertical-pod-autoscaler/pkg/recommender/model/vpa.go
https://raw.githubusercontent.com/kubernetes/autoscaler/master/vertical-pod-autoscaler/pkg/recommender/model/aggregate_container_state.go
https://raw.githubusercontent.com/kubernetes/autoscaler/master/vertical-pod-autoscaler/pkg/recommender/logic/estimator.go
https://raw.githubusercontent.com/kubernetes/autoscaler/master/vertical-pod-autoscaler/pkg/recommender/model/cluster.go
```

Also fetch:
```
https://raw.githubusercontent.com/kubernetes/autoscaler/master/vertical-pod-autoscaler/README.md
https://raw.githubusercontent.com/kubernetes/autoscaler/master/vertical-pod-autoscaler/FAQ.md
```

And the Goldilocks source:
```
https://raw.githubusercontent.com/FairwindsOps/goldilocks/master/README.md
```

And the OpenCost allocation model:
```
https://raw.githubusercontent.com/opencost/opencost/develop/README.md
https://www.opencost.io/docs/specification
```

### Step 2: Extract the actual algorithm constants and logic

From the recommender source code, extract:
- The actual percentile used for CPU recommendations
- The actual percentile used for memory recommendations
- The confidence multiplier formula (how confidence grows with sample count)
- The decay half-life for historical data
- The safety margin multipliers
- The min/max resource boundaries
- How the recommender handles different UpdateModes (Off, Initial, Auto, Recreate)
- The actual OOM bump-up logic (what happens after an OOM event)

### Step 3: Generate pairs from real algorithm behaviour

1. **"How does VPA actually calculate recommendations?"** — Explain using the real constants and formulas from the source code, not general descriptions.
2. **"Why does VPA recommend X when my usage is Y?"** — Walk through the actual calculation with real multipliers.
3. **"VPA keeps recommending too much/too little memory"** — Explain using real confidence intervals and decay rates.
4. **"How does VPA handle OOM events?"** — Use the actual OOM bump-up logic from the source.
5. **"VPA vs k8s-sage: when to use which?"** — Compare the real VPA algorithm to k8s-sage's rules engine approach.
6. **"Can I run VPA and HPA together?"** — Use the actual documented constraints and workarounds.
7. **Goldilocks interpretation**: "Goldilocks shows X for my deployment, what does that mean?" — Explain using real VPA recommendation modes.
8. **OpenCost integration**: "OpenCost shows my efficiency is X%, how does that map to right-sizing?" — Use real OpenCost allocation model concepts.

Output: `ml/dataset/raw/vpa_deep_dive_pairs.jsonl`

---

## Phase 5: Runtime Memory Models from Official Documentation (~300 pairs)

### Step 1: Fetch official runtime documentation

**JVM:**
```
https://docs.oracle.com/en/java/javase/21/docs/specs/man/java.html (or search for "java 21 JVM options reference")
https://raw.githubusercontent.com/openjdk/jdk/master/src/hotspot/share/gc/g1/g1Arguments.cpp
```
Search for: "JVM container support UseContainerSupport MaxRAMPercentage"
Search for: "JVM G1GC default heap region size ergonomics"
Search for: "kubernetes JVM memory settings best practices" — fetch top 3 results

**Go:**
Search for: "GOMEMLIMIT GOGC container kubernetes" — fetch official Go blog post
Search for: "uber automaxprocs kubernetes" — fetch the GitHub README
```
https://raw.githubusercontent.com/uber-go/automaxprocs/master/README.md
```
Search for: "Go runtime memory ballast kubernetes" — fetch top 2 results

**Node.js:**
Search for: "node.js max-old-space-size container kubernetes memory"
Search for: "V8 heap memory limit kubernetes OOM" — fetch top 3 results
Search for: "libuv thread pool UV_THREADPOOL_SIZE kubernetes"

**.NET:**
Search for: "dotnet kubernetes container memory GCHeapHardLimit"
Search for: "dotnet 8 container support ServerGC WorkstationGC kubernetes"
Fetch: https://learn.microsoft.com/en-us/dotnet/core/runtime-config/garbage-collector (or search for it)

**Python:**
Search for: "python kubernetes memory pymalloc fragmentation"
Search for: "celery kubernetes memory worker prefork" — fetch top 2 results
Search for: "python multiprocessing fork spawn kubernetes container"

**Rust:**
Search for: "rust kubernetes memory jemalloc mimalloc container"
Search for: "rust RSS virtual memory kubernetes limits"

### Step 2: For each runtime, extract verified facts

Only extract facts that appear in official documentation or verified engineering blog posts from reputable sources. For each runtime, document:
- Default memory behaviour in containers (does it respect cgroup limits?)
- GC/allocator settings that affect container memory
- Known gotchas (e.g., JVM not respecting container limits pre-Java 10)
- Recommended container memory formula (e.g., `-Xmx` = 75% of container limit)
- CPU-related container behaviour (e.g., Go `GOMAXPROCS` defaulting to node CPUs not cgroup)

### Step 3: Generate pairs from verified runtime facts

For each runtime, generate 35-50 pairs across these patterns:

1. **"My JVM app is getting OOM killed even though heap is within limits"** — Explain off-heap, metaspace, thread stacks using real JVM memory model. Use actual default values from the docs you fetched.
2. **"How should I set memory limits for a Go service in K8s?"** — Use real GOMEMLIMIT behaviour from the Go docs.
3. **"My Python workers use way more memory than expected"** — Explain pymalloc fragmentation or multiprocessing fork behaviour using real documentation.
4. **"What's the right memory formula for a .NET service?"** — Use real GCHeapHardLimit and ServerGC behaviour from Microsoft docs.
5. **Cross-runtime comparison**: "I'm migrating from JVM to Go, how should my resource requests change?" — Use real characteristics from both runtimes' documentation.
6. **"My Node.js app CPU usage is 100% but only on one core"** — Explain event loop single-threading and UV_THREADPOOL_SIZE from real Node.js docs.

**Every pair must include the specific runtime setting/flag being discussed with its real default value and real behaviour as documented in the source you fetched.**

Output: `ml/dataset/raw/runtime_memory_pairs.jsonl`

---

## Phase 6: Infrastructure and cgroups from Real Documentation (~200 pairs)

### Step 1: Fetch real documentation

**cgroups v2:**
Search for: "kubernetes cgroups v2 memory.max memory.high"
Search for: "linux cgroup v2 cpu.max CFS period kubernetes"
Fetch kernel docs if available, or verified blog posts explaining real cgroup v2 behaviour.

**Karpenter:**
```
https://raw.githubusercontent.com/kubernetes-sigs/karpenter/main/README.md
```
Search for: "karpenter consolidation policy kubernetes"
Search for: "karpenter vs cluster-autoscaler comparison" — fetch top 3 results
Fetch Karpenter docs: search for "karpenter.sh docs concepts"

**Kubelet eviction:**
Search for: "kubernetes kubelet eviction thresholds memory.available"
Fetch: official K8s docs on node pressure eviction
Search for: "kubernetes node allocatable vs capacity" — fetch top 2 results

**Container runtimes:**
Search for: "containerd CRI-O overhead memory comparison kubernetes"
Search for: "kubernetes ephemeral storage container runtime overhead"

### Step 2: Extract real thresholds, defaults, and behaviours

From each source, extract:
- Real default eviction thresholds (e.g., `memory.available < 100Mi`)
- Real kubelet reserved defaults
- Real kube-reserved and system-reserved recommendations
- Actual cgroup v2 memory.max/memory.high interaction
- Actual CFS quota period defaults and behaviour
- Actual Karpenter consolidation logic and timing
- Actual cluster-autoscaler scale-up thresholds

### Step 3: Generate pairs

1. **"Why can't I schedule pods even though the node shows free memory?"** — Explain allocatable vs capacity using real default reserves.
2. **"My pods are getting evicted but node memory looks fine"** — Explain eviction thresholds using real defaults.
3. **"Should I use Karpenter or Cluster Autoscaler?"** — Compare using real feature sets from fetched docs.
4. **"How do cgroups v2 change my resource limits?"** — Explain using real cgroup v2 parameters from kernel docs.
5. **"What's the actual overhead of containerd/CRI-O?"** — Use real measurements from fetched sources.

Output: `ml/dataset/raw/infra_pairs.jsonl`

---

## Phase 7: Cloud Provider Optimisation from Real Docs (~250 pairs)

### Step 1: Fetch official cloud provider documentation

**AWS/EKS:**
Search for: "AWS EKS right-sizing best practices" — fetch from docs.aws.amazon.com
Search for: "AWS Compute Optimizer EKS recommendations"
Search for: "AWS Karpenter EKS consolidation configuration"
Search for: "AWS spot instance interruption kubernetes" — fetch top 2 results
Search for: "AWS Graviton ARM kubernetes performance comparison"

**GCP/GKE:**
Search for: "GKE Autopilot resource recommendations"
Search for: "GKE cost optimization best practices" — fetch from cloud.google.com
Search for: "GKE vertical pod autoscaling recommendations"
Search for: "GCP committed use discounts kubernetes"

**Azure/AKS:**
Search for: "AKS Advisor container insights right-sizing"
Search for: "Azure Kubernetes Service cost optimization" — fetch from learn.microsoft.com
Search for: "AKS spot node pools best practices"
Search for: "Azure Container Insights resource recommendations"

### Step 2: Extract real provider-specific features and numbers

For each provider, document:
- What right-sizing tools are built in and how they work
- Real pricing differences between instance families
- Real spot/preemptible interruption rates or behaviours
- Actual Graviton/Arm performance characteristics
- Provider-specific autoscaling behaviour differences

### Step 3: Generate pairs

1. **"I'm on EKS, what tools can help me right-size?"** — Reference real AWS tools from fetched docs.
2. **"How does GKE Autopilot handle resource requests differently?"** — Use real Autopilot behaviour from GCP docs.
3. **"Should I use spot nodes for my workload?"** — Use real spot interruption data and patterns.
4. **"How much can I save moving to Graviton/ARM?"** — Use real benchmark data.
5. **"How does k8s-sage complement [provider tool]?"** — Position k8s-sage alongside real cloud-native tools.

Output: `ml/dataset/raw/cloud_provider_pairs.jsonl`

---

## Phase 8: Integration and Verification

### Step 1: Validate all generated files

```bash
# Count all pairs
for f in ml/dataset/raw/helm_defaults_pairs.jsonl \
         ml/dataset/raw/postmortem_pairs.jsonl \
         ml/dataset/raw/industry_data_pairs.jsonl \
         ml/dataset/raw/vpa_deep_dive_pairs.jsonl \
         ml/dataset/raw/runtime_memory_pairs.jsonl \
         ml/dataset/raw/infra_pairs.jsonl \
         ml/dataset/raw/cloud_provider_pairs.jsonl; do
  echo "$(wc -l < "$f") $f"
done
```

Expected totals:
- helm_defaults_pairs.jsonl: ~250
- postmortem_pairs.jsonl: ~200
- industry_data_pairs.jsonl: ~150
- vpa_deep_dive_pairs.jsonl: ~200
- runtime_memory_pairs.jsonl: ~300
- infra_pairs.jsonl: ~200
- cloud_provider_pairs.jsonl: ~250
- **Total: ~1,550 pairs**

### Step 2: Validate JSON format

```bash
for f in ml/dataset/raw/*_pairs.jsonl; do
  python3 -c "
import json, sys
errors = 0
with open('$f') as fh:
    for i, line in enumerate(fh, 1):
        try:
            obj = json.loads(line)
            assert 'instruction' in obj, f'Missing instruction'
            assert 'response' in obj, f'Missing response'
            assert 'source' in obj, f'Missing source'
            assert 'category' in obj, f'Missing category'
            assert obj['source'] != 'synthetic', f'Source cannot be synthetic'
        except Exception as e:
            errors += 1
            print(f'Line {i}: {e}', file=sys.stderr)
if errors:
    print(f'$f: {errors} errors', file=sys.stderr)
    sys.exit(1)
else:
    print(f'$f: OK')
"
done
```

### Step 3: Safety floor validation

```bash
python3 -c "
import json, re, sys

floors_violated = 0
files = [
    'ml/dataset/raw/helm_defaults_pairs.jsonl',
    'ml/dataset/raw/postmortem_pairs.jsonl',
    'ml/dataset/raw/industry_data_pairs.jsonl',
    'ml/dataset/raw/vpa_deep_dive_pairs.jsonl',
    'ml/dataset/raw/runtime_memory_pairs.jsonl',
    'ml/dataset/raw/infra_pairs.jsonl',
    'ml/dataset/raw/cloud_provider_pairs.jsonl',
]

for fpath in files:
    with open(fpath) as fh:
        for i, line in enumerate(fh, 1):
            obj = json.loads(line)
            resp = obj['response'].lower()
            # Check for CPU recommendations below 50m
            cpu_matches = re.findall(r'recommend.*?(\d+)m\b.*?cpu', resp)
            for m in cpu_matches:
                if int(m) < 50 and int(m) > 0:
                    print(f'{fpath}:{i} — CPU recommendation {m}m below 50m floor')
                    floors_violated += 1
            # Check for memory recommendations below 64Mi
            mem_matches = re.findall(r'recommend.*?(\d+)mi\b.*?mem', resp)
            for m in mem_matches:
                if int(m) < 64 and int(m) > 0:
                    print(f'{fpath}:{i} — Memory recommendation {m}Mi below 64Mi floor')
                    floors_violated += 1

if floors_violated:
    print(f'WARNING: {floors_violated} safety floor violations found')
else:
    print('All pairs pass safety floor check')
"
```

### Step 4: Grounding ratio check

Verify that at least 90% of new pairs have real URL sources, not training_knowledge fallback:

```bash
python3 -c "
import json
files = [
    'ml/dataset/raw/helm_defaults_pairs.jsonl',
    'ml/dataset/raw/postmortem_pairs.jsonl',
    'ml/dataset/raw/industry_data_pairs.jsonl',
    'ml/dataset/raw/vpa_deep_dive_pairs.jsonl',
    'ml/dataset/raw/runtime_memory_pairs.jsonl',
    'ml/dataset/raw/infra_pairs.jsonl',
    'ml/dataset/raw/cloud_provider_pairs.jsonl',
]
total = 0
grounded = 0
ungrounded = 0
for fpath in files:
    with open(fpath) as fh:
        for line in fh:
            obj = json.loads(line)
            total += 1
            src = obj.get('metadata', {}).get('grounded_source', 'MISSING')
            if src in ('training_knowledge', 'MISSING', ''):
                ungrounded += 1
            else:
                grounded += 1
ratio = grounded / total * 100 if total else 0
print(f'Total pairs: {total}')
print(f'Grounded (real URL): {grounded} ({ratio:.1f}%)')
print(f'Training knowledge fallback: {ungrounded} ({100-ratio:.1f}%)')
if ratio < 90:
    print('WARNING: Below 90% grounding target. Consider fetching additional sources.')
else:
    print('PASS: Grounding ratio meets target.')
"
```

### Step 5: Source verification audit

Randomly sample 10 pairs from each file and verify that:
- The `metadata.grounded_source` URL/path is valid
- The facts cited in the pair actually appear in that source
- The numbers are accurate (not hallucinated or rounded incorrectly)

```bash
python3 -c "
import json, random
files = [
    'ml/dataset/raw/helm_defaults_pairs.jsonl',
    'ml/dataset/raw/postmortem_pairs.jsonl',
    'ml/dataset/raw/industry_data_pairs.jsonl',
    'ml/dataset/raw/vpa_deep_dive_pairs.jsonl',
    'ml/dataset/raw/runtime_memory_pairs.jsonl',
    'ml/dataset/raw/infra_pairs.jsonl',
    'ml/dataset/raw/cloud_provider_pairs.jsonl',
]
for fpath in files:
    with open(fpath) as fh:
        pairs = [json.loads(l) for l in fh]
    sample = random.sample(pairs, min(10, len(pairs)))
    print(f'\n=== {fpath} — {len(sample)} samples ===')
    for p in sample:
        src = p.get('metadata', {}).get('grounded_source', 'MISSING')
        print(f'  Source: {src}')
        print(f'  Instruction: {p[\"instruction\"][:100]}...')
        print()
"
```

Review the output manually. If any pair has `MISSING` as grounded_source, fix it — either add the real URL or mark it `training_knowledge`. Aim for at least 90% of pairs having a real URL. If a file has more than 10% `training_knowledge` pairs, consider whether those pairs could be grounded by fetching an additional source.

### Step 6: Update format pipeline

Edit `ml/dataset/format_instruct.py`:
- Add all 7 new files to `INPUT_FILES`
- Lower `MAX_SYNTHETIC_RATIO` from 0.60 to 0.35
- Run the full pipeline and verify output count

```bash
python3 ml/dataset/format_instruct.py
wc -l ml/dataset/data/training_data.jsonl
# Expected: ~8,000-8,500 total pairs
```

### Step 7: Update sources.md

Add entries for all new data sources to `ml/dataset/sources.md`, including URLs and access dates.

### Step 8: Update pyproject.toml

Add E501 ignores for all new `generate_*.py` files in pyproject.toml.

### Step 9: Run linting

```bash
ruff check ml/dataset/
```

Fix any issues before committing.

---

## Execution Order

Run phases in this order to manage context and fetch load:

1. **Phase 1 (Helm defaults)** — Highest signal, most verifiable, simplest fetches
2. **Phase 4 (VPA deep dive)** — Source code is deterministic, pairs are highly differentiated
3. **Phase 2 (Postmortems)** — Real incidents, high value, moderate fetch effort
4. **Phase 5 (Runtime memory)** — Needs many searches, but documentation is authoritative
5. **Phase 3 (Industry reports)** — Some reports may be behind paywalls, work with what's accessible
6. **Phase 6 (Infrastructure)** — Official K8s docs are reliable sources
7. **Phase 7 (Cloud providers)** — Official cloud docs are reliable but verbose
8. **Phase 8 (Integration)** — Validation and pipeline integration

Commit after each phase. Each phase should be independently verifiable and revertible.

---

## Quality Checklist (Apply to Every Single Pair)

Before writing any pair to the JSONL file, verify:

- [ ] The instruction contains a realistic scenario or question a K8s operator would actually ask
- [ ] The response contains specific, actionable advice (not vague guidance)
- [ ] Specific numbers (exact CPU/memory values, exact percentages, exact defaults) trace to a fetched source where one was available
- [ ] General recommendations and analysis are grounded in real operational knowledge (even if not tied to a single URL)
- [ ] The response format matches existing training pairs (read 5 existing pairs from `ml/dataset/raw/` first)
- [ ] Safety floors are respected in all recommendations
- [ ] The `metadata.grounded_source` field contains either the actual URL/file path, or `"training_knowledge"` if no source was fetched
- [ ] The pair teaches the model something a general-purpose LLM would NOT express well without domain expertise — runtime-specific tuning, failure chain reasoning, real-world defaults analysis, or production edge cases
- [ ] The instruction and response are self-contained (no references to "the above" or external context)
- [ ] The response length is appropriate (not too terse, not padded — match existing pair style)
- [ ] The category label is accurate

---

## What NOT To Do

- Do NOT generate pairs that are purely from general knowledge when a real source is available and fetchable. Always try to fetch first.
- Do NOT make up specific resource values (exact CPU/memory numbers, exact percentages). If you're citing a specific number, it must come from a fetched source. If you're providing a general recommendation range, that's fine as expert knowledge.
- Do NOT include marketing language from tool vendors. Extract only technical facts and data.
- Do NOT reproduce verbatim text from any source. Paraphrase and transform into instruction-response format.
- Do NOT generate pairs that just restate what the rules engine already does. L2 pairs must add insight that L1 cannot provide — runtime-specific knowledge, failure pattern recognition, real-world context.
- Do NOT skip the verification steps. Run every validation script after each phase.
- Do NOT generate pairs with `source: synthetic`. Every pair in this expansion encodes expert knowledge, whether grounded in a fetched source or in operational understanding that goes beyond the rules engine.
- Do NOT let unfetchable sources block progress. If a report is paywalled or a URL is dead, note it, fall back to training knowledge with `"grounded_source": "training_knowledge"`, and move on. Momentum matters — you have 13 days, not 13 months.

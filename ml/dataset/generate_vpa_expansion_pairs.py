#!/usr/bin/env python3
"""Generate expanded VPA/Goldilocks/OpenCost training pairs for k8s-sage.

Covers Goldilocks dashboard, OpenCost allocation, VPA modes, VPA+HPA
coexistence, VPA bounds/policies, and real-world tuning scenarios.

Output: ml/dataset/raw/vpa_expansion_pairs.jsonl
"""

import json
from pathlib import Path

SYSTEM_PROMPT = (
    "You are k8s-sage, a Kubernetes resource efficiency specialist. "
    "Analyse the provided workload metrics and give actionable right-sizing "
    "recommendations. Be specific about numbers, explain your reasoning, "
    "and flag risks."
)

OUTPUT_PATH = Path(__file__).parent / "raw" / "vpa_expansion_pairs.jsonl"


def make_pair(
    id_str: str, user: str, assistant: str, category: str,
    provenance: str, **extra_meta
) -> dict:
    """Create a validated training pair."""
    assert len(assistant) >= 50, f"{id_str}: assistant too short ({len(assistant)} chars)"
    assert category in ("right-sizing", "classification", "runtime-specific", "edge-case")
    meta = {"category": category, "provenance": provenance}
    meta.update(extra_meta)
    return {
        "id": id_str,
        "source": "synthetic",
        "system": SYSTEM_PROMPT,
        "user": user,
        "assistant": assistant,
        "metadata": meta,
    }


# ── Goldilocks ───────────────────────────────────────────────────────


def goldilocks_pairs() -> list[dict]:
    """Goldilocks dashboard interpretation, deployment, configuration."""
    p = "Synthetic: AI-generated knowledge based on Goldilocks project"
    pairs = []

    pairs.append(make_pair("vpa-exp-right-sizing-001",
        "What is Goldilocks and how does it help with Kubernetes right-sizing?",
        "## Goldilocks for K8s Right-Sizing\n\n**Goldilocks (by Fairwinds) creates a VPA for every deployment in a namespace and shows recommendations in a dashboard.** It makes VPA recommendations accessible without manually creating VPA objects.\n\n**How it works:**\n1. Label a namespace: `goldilocks.fairwinds.com/enabled: true`\n2. Goldilocks controller creates a VPA (mode=Off) for each Deployment in the namespace\n3. VPA recommender generates CPU/memory recommendations based on actual usage\n4. Goldilocks dashboard shows recommendations per container with current vs recommended\n\n**Dashboard output example:**\n```\nDeployment: api-server\nContainer: app\n  Current Request:    1000m CPU / 2Gi memory\n  Recommended Lower:  100m CPU / 256Mi memory\n  Recommended Target: 250m CPU / 512Mi memory\n  Recommended Upper:  500m CPU / 1Gi memory\n  Savings:            750m CPU / 1.5Gi memory per pod\n```\n\n**Lower/Target/Upper bound interpretation:**\n- **Lower bound**: Minimum resources the container needs. Below this = frequent throttling/OOM.\n- **Target**: The recommended right-sized value. Based on usage percentiles.\n- **Upper bound**: Maximum the container has been observed to need. Above this = over-provisioned.\n\n**Right-sizing with Goldilocks:**\n- Set CPU request = Target * 1.10 (slight headroom over VPA target)\n- Set memory request = Target * 1.15 (slightly more headroom for memory safety)\n- Set CPU limit = Upper * 1.25 (room for spikes above observed max)\n- Set memory limit = Upper * 1.25\n\n**Limitations:**\n- VPA recommendations are based on raw percentiles — no workload pattern awareness\n- No runtime-specific adjustments (doesn't know JVM heap from Go working set)\n- Recommends per-container, not considering pod-level interactions\n- Goldilocks is read-only (mode=Off). You must apply changes manually.\n\n**k8s-sage advantage**: k8s-sage adds pattern classification (steady/burstable/batch), runtime awareness, and safety floors on top of the same usage data. Goldilocks tells you WHAT the numbers are; k8s-sage tells you WHAT THEY MEAN.",
        "right-sizing", p))

    pairs.append(make_pair("vpa-exp-right-sizing-002",
        "Goldilocks shows my deployment needs 100m CPU (target) but I'm requesting 2000m. Should I just set it to 100m?",
        "## Goldilocks Target vs Safe Right-Sizing\n\n**Do NOT blindly set requests to Goldilocks target.** The target is a starting point, not the final answer.\n\n**Why Goldilocks target may be too low:**\n\n1. **Observation window may be too short**: If Goldilocks has only 24h of data during a quiet period, it misses weekly peaks.\n   - Check: How long has VPA been observing? Minimum 7 days for reliable recommendations.\n\n2. **No headroom for spikes**: Target is based on past usage. Future spikes above observed max will throttle or OOM.\n   - Fix: Add 20% headroom above target.\n\n3. **Startup not considered**: If pods have been running for days, the recommendation reflects steady state. Restart memory spikes are not captured.\n   - Fix: For JVM/.NET workloads, add 30-50% headroom for startup.\n\n4. **Pattern matters**: A burstable workload with P50=50m and P99=500m might get target=100m. Setting request to 100m means 50% of the time you're above request (eligible for eviction).\n   - Fix: Use P95 as the request baseline, not VPA target.\n\n**Safe approach from 2000m → right-sized:**\n\n| Step | CPU Request | Risk | Duration |\n|------|-----------|------|----------|\n| Current | 2000m | None (over-provisioned) | - |\n| Step 1 | 1000m (50% reduction) | Very low | 7 days |\n| Step 2 | 500m (50% of step 1) | Low | 7 days |\n| Step 3 | 250m (Goldilocks target * 1.20 + headroom) | Medium | Monitor |\n\n**Never reduce by more than 50% in one step.** Gradual reduction with 7-day observation catches workloads that have periodic spikes (weekly batch jobs, monthly reports).\n\n**Safety floors**: Even if Goldilocks recommends 10m CPU, never go below 50m CPU / 64Mi memory. These are the k8s-sage safety floors based on kubelet overhead and minimum scheduling guarantees.",
        "right-sizing", p))

    return pairs


# ── OpenCost ─────────────────────────────────────────────────────────


def opencost_pairs() -> list[dict]:
    """OpenCost allocation model, efficiency scoring."""
    p = "Synthetic: AI-generated knowledge based on OpenCost project"
    pairs = []

    pairs.append(make_pair("vpa-exp-right-sizing-003",
        "How does OpenCost calculate cost allocation and how does it help with right-sizing?",
        "## OpenCost for Right-Sizing\n\n**OpenCost calculates per-pod and per-namespace costs** by apportioning node costs based on resource requests and usage.\n\n**Cost allocation model:**\n```\npod_cost = (pod_cpu_request / node_cpu_allocatable) * node_cpu_cost\n         + (pod_memory_request / node_memory_allocatable) * node_memory_cost\n```\n\n**Efficiency metrics for right-sizing:**\n\n1. **CPU Efficiency**: `cpu_usage / cpu_request`\n   - 100% = perfectly right-sized\n   - 20% = requesting 5x what you use → 80% waste\n   - >100% = over request, bursting (which is fine with burstable QoS)\n\n2. **Memory Efficiency**: `memory_usage / memory_request`\n   - Same interpretation as CPU\n   - Memory < 50% efficiency = significant waste\n\n3. **Cost Efficiency**: `(used_cpu_cost + used_memory_cost) / (requested_cpu_cost + requested_memory_cost)`\n   - Dollar-denominated waste\n   - \"You're paying $500/mo but using $100/mo worth of resources\"\n\n**OpenCost right-sizing workflow:**\n\n1. Deploy OpenCost (Helm chart or kubectl apply)\n2. Access dashboard: `kubectl port-forward svc/opencost 9090:9090`\n3. Sort by: `Efficiency (ascending)` → see most wasteful workloads first\n4. For each low-efficiency workload:\n   - Check OpenCost's recommended CPU/memory (based on usage)\n   - Apply k8s-sage recommendations (adds pattern/runtime context)\n   - Update deployment resources\n5. Track total cluster cost trend week-over-week\n\n**OpenCost vs k8s-sage:**\n- OpenCost: Answers \"how much does this pod cost and how much is wasted?\"\n- k8s-sage: Answers \"what should the resources be set to?\"\n- Together: OpenCost identifies where to focus, k8s-sage provides the specific recommendations\n\n**Typical findings:**\n- Top 10% of wasteful workloads account for 50% of wasted spend\n- Focus right-sizing effort on the top 10 most expensive low-efficiency workloads\n- Quick wins: Any workload with <30% efficiency and >$100/mo cost",
        "right-sizing", p))

    pairs.append(make_pair("vpa-exp-right-sizing-004",
        "OpenCost shows our cluster efficiency is 25%. What's a realistic target after right-sizing?",
        "## Cluster Efficiency Targets\n\n**25% efficiency means you're using 25% of what you're requesting.** That's typical for clusters without active right-sizing.\n\n**Industry benchmarks:**\n\n| Maturity | CPU Efficiency | Memory Efficiency | Overall |\n|----------|---------------|-------------------|----------|\n| No right-sizing | 15-30% | 20-40% | 20-35% |\n| Basic right-sizing | 40-60% | 40-55% | 40-55% |\n| Active right-sizing | 55-75% | 50-70% | 55-70% |\n| Optimised | 70-85% | 65-80% | 65-80% |\n\n**Why 100% is not the target:**\n- Need headroom for traffic spikes (P95 to Max gap)\n- Need headroom for pod restarts and rescheduling\n- Need headroom for node-level system overhead\n- Burst capacity for unexpected events\n- Safety floors (50m CPU, 64Mi memory minimum)\n\n**Realistic target: 55-70% overall efficiency.**\n\n**Getting from 25% to 65%:**\n\n| Phase | Effort | Efficiency gain | Cost reduction |\n|-------|--------|----------------|----------------|\n| Quick wins (top 10 wasteful) | 1-2 days | 25% → 35% | 15-20% |\n| Systematic (all workloads) | 1-2 weeks | 35% → 50% | 30-40% |\n| Node optimization | 1 week | 50% → 55% | 5-10% |\n| Ongoing tuning | Continuous | 55% → 65% | 5-10% |\n| **Total** | | **25% → 65%** | **60-70%** |\n\n**Cost impact:**\n- At 25% efficiency on $10,000/mo cluster: ~$7,500/mo wasted\n- At 65% efficiency: ~$3,500/mo wasted (headroom + overhead)\n- Net savings: ~$4,000/mo = $48,000/year\n\n**Monitoring after right-sizing:**\n- Track weekly efficiency trends in OpenCost\n- Alert when efficiency drops below 50% (new deployments with default resources)\n- Monthly review: top 10 new wasteful workloads for right-sizing\n- Quarterly review: overall cluster efficiency trend\n\n**Don't chase 90%+ efficiency** — the engineering effort and risk (more OOMKills, throttling) outweigh the marginal savings. 65% is the sweet spot for most organisations.",
        "right-sizing", p))

    return pairs


# ── VPA Modes ────────────────────────────────────────────────────────


def vpa_modes_pairs() -> list[dict]:
    """VPA Off/Initial/Auto/Recreate deep dives."""
    p = "Synthetic: AI-generated knowledge based on VPA documentation"
    pairs = []

    pairs.append(make_pair("vpa-exp-right-sizing-005",
        "What are the VPA update modes and which should I use in production?",
        "## VPA Update Modes\n\n**VPA has 4 update modes**, each with different risk profiles.\n\n**1. Off (recommendation only):**\n```yaml\nupdatePolicy:\n  updateMode: \"Off\"\n```\n- VPA calculates recommendations but does NOT apply them\n- You read recommendations via `kubectl get vpa <name> -o yaml`\n- Safest mode. No pod disruption. Manual application.\n- Use for: Initial assessment, production services where you want human approval\n\n**2. Initial (new pods only):**\n```yaml\nupdatePolicy:\n  updateMode: \"Initial\"\n```\n- VPA sets resources on newly created pods (during scaling, restart)\n- Existing running pods are NOT modified\n- Moderate risk: new pods get new values, existing pods keep old values\n- Use for: Workloads with frequent pod churn (CronJobs, scaling services)\n\n**3. Auto (recommended for most):**\n```yaml\nupdatePolicy:\n  updateMode: \"Auto\"\n```\n- VPA evicts and recreates pods to apply new resource values\n- Currently equivalent to \"Recreate\" (in-place resize is alpha)\n- Respects PodDisruptionBudgets\n- Use for: Stateless services with multiple replicas and PDBs\n- Risk: Temporary availability reduction during pod recreation\n\n**4. Recreate (explicit eviction):**\n```yaml\nupdatePolicy:\n  updateMode: \"Recreate\"\n```\n- Same behaviour as Auto: evicts pods to apply recommendations\n- Explicit about the mechanism (vs Auto which may use in-place in future)\n\n**Production recommendation:**\n\n| Workload type | VPA mode | Why |\n|--------------|----------|-----|\n| Databases (StatefulSet) | Off | Manual review required, eviction = downtime |\n| Stateless APIs (3+ replicas) | Auto | PDB protects availability |\n| Batch jobs (CronJob) | Initial | New runs get right-sized resources |\n| Singleton services (1 replica) | Off | Eviction = 100% downtime |\n| Dev/staging | Auto | Disruption is acceptable |\n\n**Safety**: Always pair Auto/Recreate mode with PodDisruptionBudgets:\n```yaml\napiVersion: policy/v1\nkind: PodDisruptionBudget\nmetadata:\n  name: api-pdb\nspec:\n  minAvailable: 2   # At least 2 pods always running\n```",
        "right-sizing", p))

    pairs.append(make_pair("vpa-exp-right-sizing-006",
        "I set VPA to Auto mode and now my pods are being evicted every few hours. Is this normal?",
        "## VPA Frequent Eviction Problem\n\n**VPA evicts pods when the recommendation changes significantly.** Frequent eviction means the workload's resource profile is unstable.\n\n**Why VPA keeps evicting:**\n\n1. **Burstable workload**: Usage swings between 100m and 2000m. VPA adjusts recommendation after each spike, then readjusts after calm period.\n   - Spike → VPA recommends 2400m → evict + recreate\n   - Calm → VPA recommends 200m → evict + recreate\n   - Loop: every few hours\n\n2. **Short observation window**: VPA's decay factor weights recent data heavily (24h half-life). Short-lived spikes dominate the recommendation.\n\n3. **Recommendation close to min/maxAllowed bounds**: VPA oscillates between its calculated value and the configured bound.\n\n**Fixes:**\n\n1. **Increase eviction threshold** (minimum change before eviction):\n   ```yaml\n   resourcePolicy:\n     containerPolicies:\n     - containerName: \"*\"\n       minAllowed:\n         cpu: 100m\n         memory: 128Mi\n       maxAllowed:\n         cpu: 4000m\n         memory: 8Gi\n       controlledValues: RequestsAndLimits\n   ```\n   VPA only evicts when recommendation differs from current by >10% (default). You can't directly configure this threshold, but setting `minAllowed`/`maxAllowed` constrains the range.\n\n2. **Switch to Off or Initial mode** for burstable workloads:\n   - Let VPA recommend, but don't auto-apply\n   - Apply recommendations manually during maintenance windows\n\n3. **Use k8s-sage instead of VPA Auto**:\n   - k8s-sage classifies the workload pattern (burstable)\n   - Sizes request for P95 of the burstable pattern (not P50 during calm)\n   - One-time application, not continuous adjustment\n   - Result: stable resources that handle the burstable pattern without VPA churn\n\n**General rule**: VPA Auto works well for steady workloads. For burstable or batch workloads, use VPA Off + manual right-sizing based on pattern-aware analysis.\n\n**If keeping VPA Auto**: Set wider minAllowed/maxAllowed to reduce oscillation. A wider range means VPA recommendation changes are smaller relative to the bounds.",
        "edge-case", p))

    return pairs


# ── VPA + HPA ────────────────────────────────────────────────────────


def vpa_hpa_pairs() -> list[dict]:
    """VPA+HPA conflict, coexistence patterns, MPA/KEDA."""
    p = "Synthetic: AI-generated knowledge based on VPA+HPA interaction patterns"
    pairs = []

    pairs.append(make_pair("vpa-exp-right-sizing-007",
        "Can I use VPA and HPA together? What are the conflicts and how do I resolve them?",
        "## VPA + HPA Coexistence\n\n**VPA and HPA can conflict when both try to adjust the same resource.** The key is understanding what each controls.\n\n**Conflict scenario:**\n- HPA: \"CPU usage is 80% of request, scale from 3 to 6 replicas\" (horizontal)\n- VPA: \"CPU request should be 500m, not 1000m\" (vertical)\n- VPA reduces request → CPU utilization doubles → HPA adds more replicas\n- Loop: VPA keeps reducing, HPA keeps adding replicas\n\n**Resolution strategies:**\n\n**1. VPA on memory, HPA on CPU (recommended):**\n```yaml\n# VPA controls memory only\napiVersion: autoscaling.k8s.io/v1\nkind: VerticalPodAutoscaler\nspec:\n  targetRef:\n    kind: Deployment\n    name: api-server\n  resourcePolicy:\n    containerPolicies:\n    - containerName: \"*\"\n      controlledResources: [\"memory\"]  # Only memory\n  updatePolicy:\n    updateMode: \"Auto\"\n---\n# HPA controls replicas based on CPU\napiVersion: autoscaling/v2\nkind: HorizontalPodAutoscaler\nspec:\n  scaleTargetRef:\n    kind: Deployment\n    name: api-server\n  metrics:\n  - type: Resource\n    resource:\n      name: cpu\n      target:\n        type: Utilization\n        averageUtilization: 70\n```\nNo conflict: VPA adjusts memory vertically, HPA scales replicas based on CPU.\n\n**2. VPA in Off mode + HPA (manual VPA):**\n- VPA provides recommendations only\n- HPA does active scaling\n- Human applies VPA recommendations during maintenance windows\n- No automation conflict\n\n**3. Multidimensional Pod Autoscaler (MPA):**\n- Kubernetes-native solution for combined horizontal + vertical scaling\n- Still in development (not GA)\n- Resolves the conflict by coordinating both dimensions\n\n**4. KEDA + VPA Off:**\n- KEDA for event-driven horizontal scaling (queue depth, custom metrics)\n- VPA Off for vertical recommendations\n- Apply VPA recommendations periodically\n\n**Which to choose:**\n\n| Workload | Autoscaler | Why |\n|----------|-----------|-----|\n| Stateless API | HPA (CPU) + VPA (memory-only, Auto) | CPU drives scaling, memory is per-pod |\n| Queue worker | KEDA (queue depth) + VPA (Off) | Scale on queue, right-size manually |\n| Database | VPA (Off) only | No horizontal scaling, manual vertical |\n| Batch job | Neither | Right-size once, runs to completion |",
        "right-sizing", p))

    return pairs


# ── VPA Bounds ───────────────────────────────────────────────────────


def vpa_bounds_pairs() -> list[dict]:
    """VPA targetRef, minAllowed/maxAllowed, UpdatePolicy, ContainerPolicies."""
    p = "Synthetic: AI-generated knowledge based on VPA configuration"
    pairs = []

    pairs.append(make_pair("vpa-exp-right-sizing-008",
        "How do I configure VPA minAllowed and maxAllowed bounds for safe right-sizing?",
        "## VPA Bounds Configuration\n\n**minAllowed and maxAllowed constrain VPA's recommendations** to a safe range. Without bounds, VPA might recommend 1m CPU (too low) or 100 CPU (too high).\n\n**Configuration:**\n```yaml\napiVersion: autoscaling.k8s.io/v1\nkind: VerticalPodAutoscaler\nmetadata:\n  name: api-server-vpa\nspec:\n  targetRef:\n    apiVersion: apps/v1\n    kind: Deployment\n    name: api-server\n  resourcePolicy:\n    containerPolicies:\n    - containerName: app\n      minAllowed:\n        cpu: 100m        # Never recommend below 100m\n        memory: 128Mi    # Never recommend below 128Mi\n      maxAllowed:\n        cpu: 4000m       # Never recommend above 4000m\n        memory: 8Gi      # Never recommend above 8Gi\n      controlledResources: [\"cpu\", \"memory\"]\n      controlledValues: RequestsAndLimits  # or RequestsOnly\n    - containerName: sidecar\n      mode: \"Off\"        # Don't touch sidecar resources\n  updatePolicy:\n    updateMode: \"Auto\"\n```\n\n**Setting bounds correctly:**\n\n1. **minAllowed (safety floor):**\n   - CPU: At least 50m (k8s-sage safety floor). Below this, kubelet overhead > useful work.\n   - Memory: At least 64Mi (k8s-sage safety floor). Below this, many runtimes can't start.\n   - For JVM containers: min memory = Xmx * 0.5 (can't run with less than half the heap)\n   - For Go containers: min memory = 64Mi (Go is efficient at small sizes)\n\n2. **maxAllowed (cost ceiling):**\n   - CPU: Historical max * 2 (room for new traffic patterns)\n   - Memory: Historical max * 1.5 (memory should be more constrained)\n   - Match to the largest instance type in your cluster (no point recommending 32 CPU on a 4-CPU node)\n\n3. **controlledValues:**\n   - `RequestsOnly`: VPA sets requests, limits stay as you configured. Recommended.\n   - `RequestsAndLimits`: VPA sets both. Maintain the request:limit ratio.\n\n**Per-container policies:**\n- Use `mode: \"Off\"` for sidecars (Istio proxy, log shippers) — don't let VPA adjust these\n- Use specific bounds per container when main app and sidecar have different needs\n- Use `containerName: \"*\"` for a default policy, then override specific containers\n\n**Common mistake**: Setting maxAllowed too low. If VPA recommends above maxAllowed, it caps at maxAllowed. But if the workload actually needs more, it will OOMKill or throttle. Review VPA status for `providedRecommendation` vs `recommendation` — if they differ, bounds are limiting VPA.",
        "right-sizing", p))

    pairs.append(make_pair("vpa-exp-right-sizing-009",
        "How does controlledValues: RequestsOnly vs RequestsAndLimits affect right-sizing?",
        "## VPA controlledValues Impact\n\n**This setting determines whether VPA adjusts only requests (leaving limits manual) or both.**\n\n**RequestsOnly (recommended for most):**\n```yaml\ncontrolledValues: RequestsOnly\n```\n- VPA adjusts requests based on usage percentiles\n- Limits stay as you manually configured\n- Burstable QoS: request < limit (pod can burst)\n- You control the limit headroom independently\n\n**RequestsAndLimits:**\n```yaml\ncontrolledValues: RequestsAndLimits\n```\n- VPA adjusts both requests AND limits\n- Maintains the original request:limit ratio\n- If original: 1000m request / 2000m limit (1:2 ratio)\n- VPA recommends 500m → sets request=500m, limit=1000m (1:2 maintained)\n\n**Which to use:**\n\n| Scenario | Setting | Why |\n|----------|---------|-----|\n| Stateless services | RequestsOnly | Limits manually set for burst headroom |\n| Guaranteed QoS required | RequestsAndLimits | Must keep request=limit |\n| Cost-sensitive, low risk | RequestsAndLimits | Tight limits reduce overcommit |\n| Latency-sensitive, no CPU limit | RequestsOnly | Don't want VPA adding CPU limits |\n\n**Interaction with HPA:**\n- RequestsOnly + HPA on CPU: Safe. VPA changes request, HPA adjusts replicas based on utilization ratio.\n- RequestsAndLimits + HPA on CPU: Risky. VPA changes limit, which changes burst capacity, which affects HPA scaling decisions.\n\n**Example impact:**\n```\nBefore VPA:\n  Request: 1000m, Limit: 2000m\n  Usage P95: 800m → Utilization: 80% → HPA considers scaling\n\nAfter VPA (RequestsOnly):\n  Request: 960m, Limit: 2000m (unchanged)\n  Usage P95: 800m → Utilization: 83% → HPA behaviour similar\n\nAfter VPA (RequestsAndLimits):\n  Request: 960m, Limit: 1920m (scaled proportionally)\n  Usage P95: 800m → Utilization: 83% → same\n  BUT: burst capacity dropped from 2000m to 1920m\n```\n\n**Recommendation**: Start with RequestsOnly. Only use RequestsAndLimits if you need Guaranteed QoS or want VPA to manage your entire resource profile. With RequestsOnly, you maintain explicit control over limits and burst capacity.",
        "right-sizing", p))

    return pairs


# ── VPA Tuning ───────────────────────────────────────────────────────


def vpa_tuning_pairs() -> list[dict]:
    """Real-world VPA tuning scenarios with metrics, troubleshooting."""
    p = "Synthetic: AI-generated VPA tuning knowledge"
    pairs = []

    pairs.append(make_pair("vpa-exp-right-sizing-010",
        "VPA recommends 50m CPU for my service but I know it needs at least 500m during peak hours. Why is VPA wrong?",
        "## VPA Recommendation Too Low — Diagnosis\n\n**VPA isn't necessarily wrong — it may not have seen the peak.** Several factors cause low recommendations.\n\n**Possible causes:**\n\n1. **Insufficient observation window:**\n   - VPA uses a decay-weighted algorithm (24h half-life)\n   - If the peak was 5 days ago, it's weighted at ~3% of current recommendation\n   - Recent calm period dominates the recommendation\n   - Check: `kubectl get vpa <name> -o yaml` → `status.recommendation` timestamps\n\n2. **Peak happened before VPA was created:**\n   - VPA only sees data from when it was deployed\n   - If deployed during off-peak, it has no peak data\n   - Fix: Wait for a full business cycle (7 days minimum)\n\n3. **Peak is on a different schedule:**\n   - Weekly peak (Monday morning surge)\n   - Monthly peak (end-of-month processing)\n   - VPA's decay may smooth out infrequent peaks\n\n4. **Pod was restarted recently:**\n   - VPA tracks per-pod. If the pod restarted, VPA data is from the new pod only.\n   - Historical data from previous pod incarnations is partially retained via VPA checkpoint.\n\n**How to verify:**\n```bash\n# Check VPA recommendation details\nkubectl get vpa api-server-vpa -o json | jq '.status.recommendation'\n\n# Check actual peak usage\nkubectl top pod api-server-xxx --containers\n\n# Check Prometheus for historical peak\n# query: max_over_time(container_cpu_usage_seconds_total{pod=~\"api-server.*\"}[7d])\n```\n\n**Fixes:**\n1. Set `minAllowed: {cpu: 500m}` in VPA spec to enforce your known minimum\n2. Wait for 7+ days of observation including peak periods\n3. Use k8s-sage which uses 7-day P95 by default (captures weekly patterns)\n4. If VPA consistently underestimates, set the recommendation manually using VPA Off mode\n\n**Key lesson**: VPA recommendations are only as good as the data they've seen. Short observation windows, recent pod restarts, or decay weighting can all cause under-recommendations. Always validate against known traffic patterns.",
        "edge-case", p))

    pairs.append(make_pair("vpa-exp-right-sizing-011",
        "My VPA shows different recommendations for each replica of the same deployment. Which one should I use?",
        "## VPA Per-Replica Variation\n\n**VPA generates one recommendation per VPA object, but different replicas may show different usage patterns.** The recommendation accounts for all replicas.\n\n**Why replicas have different usage:**\n\n1. **Load imbalance**: Service mesh or load balancer routes unevenly. One replica gets 60% of traffic.\n2. **Cache warmth**: Replica A has warm cache (low CPU). Replica B just restarted (cold cache, high CPU).\n3. **Leader election**: In leader-follower patterns, the leader does more work.\n4. **Node differences**: Replica on a busy node gets less CPU (throttled), showing lower usage.\n\n**How VPA handles this:**\n- VPA aggregates data from ALL pods matching the targetRef\n- The recommendation is based on the AGGREGATE distribution\n- If 3 replicas use 100m, 200m, 500m → VPA sees the combined distribution\n- Recommendation targets a percentile of the combined data\n\n**What to do:**\n\n1. **Use VPA's aggregate recommendation** (it already accounts for all replicas)\n2. **Investigate if imbalance is a problem:**\n   - If one replica uses 5x more than others → fix load balancing\n   - If replicas are similar → VPA recommendation is reliable\n\n3. **For leader-follower patterns:**\n   - Create separate VPAs for leader and follower if possible\n   - Or set minAllowed to the leader's minimum need (higher)\n\n4. **For cache-dependent variation:**\n   - Use the max across replicas as your baseline\n   - VPA's aggregate will naturally capture this in the upper percentiles\n\n**Checking per-replica usage:**\n```bash\nkubectl top pods -l app=api-server --containers\n# Shows CPU/memory per pod — look for outliers\n\n# Prometheus:\nsum by (pod) (rate(container_cpu_usage_seconds_total{pod=~\"api-server.*\"}[5m]))\n# Shows per-pod CPU rate — identify imbalance\n```\n\n**Rule**: If the max replica is more than 3x the min replica, investigate the imbalance before trusting VPA's recommendation. The aggregate may under-size the busy replica and over-size the quiet ones.",
        "right-sizing", p))

    pairs.append(make_pair("vpa-exp-right-sizing-012",
        "VPA status shows 'RecommendationNotProvided' for one of my containers. Why?",
        "## VPA RecommendationNotProvided Troubleshooting\n\n**VPA requires minimum data before generating a recommendation.** Several conditions cause it to withhold recommendations.\n\n**Common causes:**\n\n1. **Not enough data points:**\n   - VPA needs at least 8 data points (samples) before recommending\n   - With default 1-minute metrics interval: minimum 8 minutes of data\n   - If the pod was just created, wait for data accumulation\n\n2. **Container restarts too frequently:**\n   - Each restart resets the per-pod tracking\n   - CrashLoopBackOff with <8 minutes uptime → never enough data\n   - Fix the crash first, then VPA can observe stable usage\n\n3. **VPA recommender not running:**\n   - Check: `kubectl get pods -n kube-system -l app=vpa-recommender`\n   - If recommender is down, no VPA can generate recommendations\n   - Common issue: recommender OOMKilled on large clusters (needs more memory)\n\n4. **Container excluded by policy:**\n   ```yaml\n   containerPolicies:\n   - containerName: sidecar\n     mode: \"Off\"  # This container won't get recommendations\n   ```\n\n5. **Metrics server not providing data:**\n   - VPA recommender reads from metrics-server\n   - If metrics-server is down: `kubectl top pods` fails AND VPA has no data\n   - Check: `kubectl get apiservice v1beta1.metrics.k8s.io -o yaml`\n\n6. **Container has no resource usage** (init container that completed):\n   - Init containers that run and exit show zero usage\n   - VPA can't recommend for zero-usage containers\n\n**Diagnosis steps:**\n```bash\n# 1. Check VPA status\nkubectl describe vpa <name>\n\n# 2. Check VPA recommender logs\nkubectl logs -n kube-system -l app=vpa-recommender --tail=50\n\n# 3. Check if metrics are available\nkubectl top pods <pod-name> --containers\n\n# 4. Check VPA checkpoint (stored recommendations)\nkubectl get vpacheckpoint -n kube-system\n```\n\n**If VPA never provides recommendations** after 24h with a stable pod:\n- Verify metrics-server is healthy\n- Verify VPA recommender has enough memory (increase to 1Gi for clusters >100 VPAs)\n- Check VPA recommender logs for errors about the specific deployment\n- As a workaround: use k8s-sage or manual right-sizing based on Prometheus metrics",
        "edge-case", p))

    return pairs


# ── Main ─────────────────────────────────────────────────────────────


def generate_all() -> list[dict]:
    """Collect all VPA expansion pairs."""
    all_pairs = []
    all_pairs.extend(goldilocks_pairs())
    all_pairs.extend(opencost_pairs())
    all_pairs.extend(vpa_modes_pairs())
    all_pairs.extend(vpa_hpa_pairs())
    all_pairs.extend(vpa_bounds_pairs())
    all_pairs.extend(vpa_tuning_pairs())

    # Validate
    ids = set()
    for pair in all_pairs:
        pid = pair["id"]
        assert pid not in ids, f"Duplicate ID: {pid}"
        ids.add(pid)
        assert len(pair["assistant"]) >= 50, f"{pid}: assistant too short"
        assert pair["metadata"]["category"] in (
            "right-sizing", "classification", "runtime-specific", "edge-case"
        ), f"{pid}: bad category"
        assert pair["metadata"].get("provenance"), f"{pid}: missing provenance"

    # Write
    OUTPUT_PATH.parent.mkdir(parents=True, exist_ok=True)
    with open(OUTPUT_PATH, "w") as f:
        for pair in all_pairs:
            f.write(json.dumps(pair) + "\n")

    print(f"Wrote {len(all_pairs)} VPA expansion pairs -> {OUTPUT_PATH}")
    return all_pairs


if __name__ == "__main__":
    pairs = generate_all()
    cats = {}
    for p in pairs:
        c = p["metadata"]["category"]
        cats[c] = cats.get(c, 0) + 1
    print("Pairs by category:")
    for cat, count in sorted(cats.items()):
        print(f"  {cat}: {count}")

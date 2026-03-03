#!/usr/bin/env python3
"""Generate postmortem and failure mode training pairs for k8s-sage.

Covers OOM cascades, CPU throttling, node pressure eviction, sidecar
contention, spot interruption, resource quota, and complex failure scenarios.

Output: ml/dataset/raw/postmortem_pairs.jsonl
"""

import json
from pathlib import Path

SYSTEM_PROMPT = (
    "You are k8s-sage, a Kubernetes resource efficiency specialist. "
    "Analyse the provided workload metrics and give actionable right-sizing "
    "recommendations. Be specific about numbers, explain your reasoning, "
    "and flag risks."
)

OUTPUT_PATH = Path(__file__).parent / "raw" / "postmortem_pairs.jsonl"


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


# ── OOM Cascades ─────────────────────────────────────────────────────


def oom_cascade_pairs() -> list[dict]:
    """OOM cascade patterns and prevention."""
    p = "Synthetic: AI-generated failure mode knowledge"
    pairs = []

    pairs.append(make_pair("postmortem-edge-case-001",
        "Our payment-service pod got OOMKilled, then 3 more pods crashed within 2 minutes. What happened and how do I prevent this cascade?",
        "## OOM Cascade Analysis\n\n**Trigger → Cascade mechanism:**\n1. payment-service OOMKilled (memory limit exceeded)\n2. Load shifts to remaining replicas (K8s routes traffic away from dead pod)\n3. Each surviving replica now handles 33% more traffic\n4. More traffic → more in-flight requests → more memory per replica\n5. Second replica hits memory limit → OOMKilled\n6. Surviving 2 replicas now handle 50% more traffic each → cascade\n7. Within 2 minutes, all replicas are dead\n\n**Root cause**: Memory limit was sized for normal traffic, not for the load increase when a replica dies.\n\n**Prevention strategies:**\n\n1. **Size for N-1 capacity**: If you have 3 replicas, each replica's memory limit must handle 50% more traffic (in case 1 dies).\n   - Current: 3 replicas * 1Gi limit = 3Gi total budget\n   - Fixed: 3 replicas * 1.5Gi limit = 4.5Gi total budget\n   - Each pod can handle 50% surge from a peer failure\n\n2. **PodDisruptionBudget**: Set `minAvailable: 2` so K8s doesn't voluntarily evict below 2 replicas.\n\n3. **HPA with memory metric**: Scale replicas before memory hits limit.\n   ```yaml\n   metrics:\n   - type: Resource\n     resource:\n       name: memory\n       target:\n         type: Utilization\n         averageUtilization: 70  # Scale up at 70% of request\n   ```\n\n4. **Circuit breaker**: If a service is overloaded, shed load (return 503) instead of accepting requests that consume memory.\n\n5. **Memory request vs limit gap**: Set request = P95 (for scheduling) but limit = Max * 1.50 (for burst/failover). The gap between request and limit is your failover buffer.\n\n**Right-sizing for cascade prevention:**\n- Memory request: P95 * 1.20 (normal operation)\n- Memory limit: P95 * 1.50 (handles +50% from peer failure)\n- Minimum replicas: 3 (so one failure = 50% increase, not 100%)\n\n**Safety floor**: Memory limit must never be below 64Mi regardless of usage pattern.",
        "edge-case", p))

    pairs.append(make_pair("postmortem-edge-case-002",
        "A Java service with -Xmx4g keeps getting OOMKilled at 4.2Gi but the JVM heap dump shows only 2.5Gi of live objects. What is using the other 1.7Gi?",
        "## JVM Off-Heap OOMKill Analysis\n\n**The 1.7Gi gap between live heap objects (2.5Gi) and container OOMKill (4.2Gi) is off-heap memory.**\n\n**Memory breakdown:**\n```\nJava Heap (in -Xmx budget):     2.5Gi live objects\nJava Heap (GC overhead):         0.5Gi  (G1GC region metadata, remembered sets)\nMetaspace:                       0.3Gi  (class metadata, typical Spring Boot)\nThread stacks:                   0.2Gi  (200 threads * 1MB each)\nDirect ByteBuffers:              0.4Gi  (Netty, NIO, gRPC)\nCode cache:                      0.1Gi  (JIT compiled code)\nNative memory (JNI, malloc):     0.1Gi  (native libraries)\nGC temporary:                    0.1Gi  (G1GC evacuation buffers)\n─────────────────────────────────────────\nTotal:                           4.2Gi  ← OOMKill threshold\n```\n\n**Why the JVM didn't throw OutOfMemoryError:**\n- OOMKill is from the Linux kernel (cgroup limit), not from the JVM\n- The JVM thinks it has 4Gi of heap available and only uses 2.5Gi\n- But the kernel counts ALL process memory (heap + off-heap) against the cgroup limit\n- When total reaches 4.2Gi (container limit), kernel kills the process\n\n**Fix:**\n\n1. **Increase container limit**: 4Gi is too tight for a -Xmx4g JVM.\n   - Formula: container_limit = Xmx * 1.5 + 512Mi\n   - For -Xmx4g: limit = 6Gi + 512Mi ≈ 6.5Gi\n\n2. **Or reduce Xmx**: If container limit must stay at 4.2Gi:\n   - Xmx = (container_limit - 1.5Gi) / 1.0 = 2.7Gi\n   - Set -Xmx2700m to leave room for off-heap\n\n3. **Cap off-heap explicitly**:\n   - `-XX:MaxDirectMemorySize=256m` (caps Netty buffers)\n   - `-XX:MaxMetaspaceSize=256m` (caps class loading)\n   - `-XX:ReservedCodeCacheSize=64m` (caps JIT cache)\n\n**Right-sizing**: Container limit = Xmx + MaxDirectMemorySize + MaxMetaspaceSize + (threads * 1Mi) + 512Mi safety margin. Never set container limit equal to Xmx.",
        "edge-case", p, runtime="jvm"))

    pairs.append(make_pair("postmortem-edge-case-003",
        "Three pods on the same node all OOMKilled simultaneously even though each was well within its memory limit. What happened?",
        "## Node-Level OOM (System OOM vs Container OOM)\n\n**When all pods on a node OOMKill simultaneously, it's a node-level OOM, not a container-level OOM.**\n\n**Cause**: The sum of all container memory limits exceeds node memory (overcommit), and all containers simultaneously approached their limits.\n\n**How it happens:**\n```\nNode memory: 16Gi\nPod A limit: 8Gi (using 6Gi)\nPod B limit: 8Gi (using 5Gi)\nPod C limit: 8Gi (using 4Gi)\nSystem overhead: 2Gi\nTotal limits: 26Gi (overcommitted by 10Gi)\nTotal actual usage: 17Gi > 16Gi node memory → node OOM\n```\n\n**Why individual containers weren't OOMKilled first:**\n- Each container is within its own cgroup limit\n- But the node's physical memory is exhausted\n- Kernel's system OOM killer activates and kills processes by oom_score\n- Multiple processes may be killed in rapid succession\n\n**Symptoms:**\n- All pods show `OOMKilled` in status\n- `kubectl describe node` shows `MemoryPressure: True`\n- Node events show `SystemOOM` or `NodeHasMemoryPressure`\n- Container-level metrics show none were at their individual limit\n\n**Prevention:**\n\n1. **Don't overcommit memory limits on a node:**\n   - Sum of all pod memory limits <= node allocatable memory\n   - This means Guaranteed QoS for critical workloads\n\n2. **Set memory request = limit for critical pods:**\n   - Guaranteed QoS pods are killed last by OOM scorer\n   - BestEffort and Burstable pods are killed first\n\n3. **kubelet eviction thresholds:**\n   ```\n   --eviction-hard=memory.available<500Mi\n   --eviction-soft=memory.available<1Gi\n   ```\n   Kubelet evicts pods BEFORE system OOM occurs.\n\n4. **Right-sizing to prevent overcommit:**\n   - Reduce memory requests (allows better scheduling)\n   - Reduce memory limits (prevents node-level overcommit)\n   - Memory request = P95 * 1.20, limit = Max * 1.25 (not 2x Max)\n\n**Key lesson**: Memory limits are per-container, but physical memory is per-node. Right-sizing limits prevents the overcommit that causes cascading node OOM.",
        "edge-case", p))

    return pairs


# ── CPU Throttling ───────────────────────────────────────────────────


def cpu_throttling_pairs() -> list[dict]:
    """CFS throttling, multi-threaded interaction, invisible performance issues."""
    p = "Synthetic: AI-generated failure mode knowledge"
    pairs = []

    pairs.append(make_pair("postmortem-edge-case-004",
        "Our API has P99 latency of 500ms but CPU usage is only 30%. We suspect CPU throttling. How do we diagnose and fix?",
        "## Invisible CPU Throttling Diagnosis\n\n**30% average CPU with high P99 latency is a classic CFS throttling signature.** The average hides burst throttling.\n\n**Diagnosis steps:**\n\n1. **Check throttle metrics:**\n```promql\n# Throttle percentage\nrate(container_cpu_cfs_throttled_periods_total{pod=\"api-xxx\"}[5m])\n/ rate(container_cpu_cfs_periods_total{pod=\"api-xxx\"}[5m])\n```\nIf > 5%, throttling is the cause.\n\n2. **Check throttle duration:**\n```promql\nrate(container_cpu_cfs_throttled_seconds_total{pod=\"api-xxx\"}[5m])\n```\nThis shows seconds of CPU time lost to throttling per second.\n\n**Why 30% average but still throttled:**\n- CFS period is 100ms\n- CPU limit: 500m = 50ms of CPU time per 100ms period\n- Average request: 3ms CPU (handles most requests quickly)\n- P99 request: 45ms CPU (complex queries, cold cache)\n- When a P99 request arrives mid-period after 10ms already used: only 40ms left\n- 45ms request gets 40ms, then throttled for 60ms waiting for next period\n- Total P99 latency: 45ms processing + 60ms throttle wait = 105ms added\n- Average CPU: (50 requests * 3ms + 1 request * 45ms) / 51 = 3.8ms/period = 38%\n\n**Fixes (choose one):**\n\n1. **Increase CPU limit** to 1000m (2x current):\n   - P99 request now has 100ms budget, enough for 45ms processing\n   - No throttling, P99 latency drops\n   - Cost: 2x CPU limit (but actual usage stays ~30%)\n\n2. **Remove CPU limit entirely** (keep only request):\n   - Pod can burst to any available CPU on the node\n   - Best option for latency-sensitive services\n   - Risk: Noisy neighbour if other pods on same node\n\n3. **Reduce CFS period** (kubelet flag):\n   - `--cpu-cfs-quota-period=10ms` (default 100ms)\n   - Finer-grained scheduling, less burst accumulation\n   - Risk: Higher scheduler overhead\n\n**Right-sizing rule**: For latency-sensitive services, set CPU request to P95 * 1.20 (for scheduling guarantee) and either remove the CPU limit or set it to 3-5x the request. The limit exists to prevent runaway processes, not to constrain normal operation.",
        "edge-case", p))

    pairs.append(make_pair("postmortem-edge-case-005",
        "Our multi-threaded Java application with 8 threads and 2000m CPU limit shows severe throttling even though average CPU is 60%. Why?",
        "## Multi-Thread CFS Throttling Amplification\n\n**8 threads sharing a 2000m (2 core) CFS budget creates a much worse throttling problem than single-threaded workloads.**\n\n**How CFS handles multi-threaded containers:**\n- Budget: 200ms per 100ms period (2 cores)\n- 8 threads can collectively consume 200ms of CPU time very quickly\n- If all 8 threads are active: 200ms consumed in 25ms wall time (8 threads * 25ms each)\n- Container throttled for 75ms until next period\n- During throttle: ALL 8 threads are paused, not just the one that exhausted the budget\n\n**The math:**\n```\nBudget per period: 200ms (2000m limit)\nThreads: 8\nWall time to exhaust: 200ms / 8 = 25ms\nThrottle duration: 100ms - 25ms = 75ms\nEffective CPU: 200ms / 100ms period = 2 cores (matches limit)\nBut wall time efficiency: 25ms active + 75ms throttled = 25%\n```\n\n**Average CPU shows 60%** because not all 8 threads are always active simultaneously. But when they are (GC, burst of parallel requests), throttling kicks in.\n\n**Symptoms:**\n- High `nr_throttled` count\n- P99 latency has 75ms steps (one throttle period)\n- GC pauses appear longer than expected (GC threads throttled too)\n- Thread dumps show all threads in RUNNABLE but making no progress\n\n**Fixes:**\n\n1. **Match limit to thread count**: 8 threads → 8000m limit (8 cores).\n   - Expensive but eliminates throttling completely.\n\n2. **Reduce thread count to match limit**: `JAVA_OPTS=-XX:ActiveProcessorCount=2`\n   - JVM creates thread pools based on processor count\n   - With 2 processors: 2 GC threads, smaller thread pools\n   - Actually uses 2 cores efficiently\n\n3. **Use automaxprocs or explicit GOMAXPROCS** (for Go): Matches goroutine scheduler to CPU limit.\n\n4. **Remove CPU limit**: Best for JVM workloads. Let threads burst freely.\n\n**Right-sizing multi-threaded workloads:**\n- CPU request: P95 * 1.20 (for scheduling)\n- CPU limit: either match to thread count, reduce thread count to match limit, or remove limit entirely\n- Never set CPU limit < active_thread_count * 500m (minimum for progress)",
        "edge-case", p, runtime="jvm"))

    return pairs


# ── Node Pressure ────────────────────────────────────────────────────


def node_pressure_pairs() -> list[dict]:
    """Eviction storms, QoS priority, eviction order."""
    p = "Synthetic: AI-generated failure mode knowledge"
    pairs = []

    pairs.append(make_pair("postmortem-edge-case-006",
        "Our cluster had a node pressure eviction storm — 20 pods evicted in 5 minutes. How do we prevent this?",
        "## Node Pressure Eviction Storm Analysis\n\n**An eviction storm occurs when kubelet evicts multiple pods in rapid succession due to resource pressure.**\n\n**How it happens:**\n1. Node memory approaches threshold (e.g., `memory.available < 100Mi`)\n2. Kubelet enters eviction mode, sorts pods by QoS class and usage\n3. Evicts BestEffort pods first, then Burstable pods exceeding their request\n4. Each evicted pod's memory is freed, but may be quickly consumed by remaining pods\n5. If freed memory is insufficient, kubelet evicts more pods\n6. Cascade: evicted pods are rescheduled, potentially on the same node, re-triggering pressure\n\n**Eviction order (kubelet priority):**\n1. BestEffort pods (no requests/limits)\n2. Burstable pods using > request (sorted by overage percentage)\n3. Burstable pods within request\n4. Guaranteed pods (only under extreme pressure)\n\n**Prevention:**\n\n1. **Set soft eviction thresholds with grace periods:**\n```\n--eviction-soft=memory.available<1Gi\n--eviction-soft-grace-period=memory.available=2m\n--eviction-hard=memory.available<500Mi\n```\nSoft eviction gives pods 2 minutes to gracefully terminate before hard eviction.\n\n2. **Eliminate BestEffort pods**: Every pod must have resource requests. BestEffort pods are evicted first and create unpredictable memory usage.\n\n3. **Right-size to prevent overcommit:**\n   - Sum of memory requests on a node should be < node allocatable\n   - Sum of memory limits should be < node memory * 1.5 (moderate overcommit)\n   - Tight overcommit = more efficient but higher eviction risk\n\n4. **Use PriorityClasses:**\n```yaml\napiVersion: scheduling.k8s.io/v1\nkind: PriorityClass\nmetadata:\n  name: critical-service\nvalue: 1000000\n```\nHigher priority pods evict lower priority pods, not the other way around.\n\n5. **Pod topology spread**: Distribute replicas across nodes so one node's eviction doesn't take down the entire service.\n\n**Right-sizing connection:** Over-provisioned requests prevent scheduling (waste), but under-provisioned requests cause eviction storms (instability). The sweet spot: request = P95 * 1.20 (enough headroom to stay within request under normal conditions).",
        "edge-case", p))

    return pairs


# ── Sidecar Contention ───────────────────────────────────────────────


def sidecar_pairs() -> list[dict]:
    """Sidecar proxy and log shipper contention patterns."""
    p = "Synthetic: AI-generated failure mode knowledge"
    pairs = []

    pairs.append(make_pair("postmortem-edge-case-007",
        "Our application container's CPU usage spiked after adding an Istio sidecar, even though the sidecar itself shows low CPU. What's happening?",
        "## Sidecar-Induced Resource Contention\n\n**Adding a sidecar changes the resource dynamics of the entire pod.** The application container's CPU increase isn't caused by the sidecar using CPU — it's caused by the sidecar adding latency.\n\n**Mechanism:**\n1. Before sidecar: Application makes direct network calls (1ms per call)\n2. After sidecar: All traffic proxied through Envoy (+1-3ms per call)\n3. Application makes 100 calls per request: +100-300ms added latency\n4. Request takes longer → in-flight request count increases\n5. More in-flight requests → more threads active → more CPU used\n6. Application CPU increases even though sidecar CPU is low\n\n**The math:**\n```\nBefore sidecar:\n  Request time: 50ms (10ms app + 40ms network calls)\n  Throughput: 1000 rps with 50 concurrent connections\n  CPU: 500m\n\nAfter sidecar:\n  Request time: 80ms (10ms app + 70ms network via proxy)\n  Same throughput needs: 80 concurrent connections (60% more)\n  CPU: 800m (60% more due to more concurrent request handling)\n```\n\n**Diagnosis:**\n- Application CPU increased ~50-100% after sidecar injection\n- Sidecar CPU is low (50-100m)\n- Request latency P95 increased by 30-50ms\n- In-flight request count increased proportionally\n\n**Fixes:**\n\n1. **Right-size for the sidecar overhead:**\n   - Application CPU request: increase by 50-100% (to handle higher concurrency)\n   - Sidecar CPU request: 50-100m (its own usage)\n   - Total pod CPU request: original * 1.5 + sidecar overhead\n\n2. **Exclude health checks from sidecar:**\n   - Kubelet health probes through Envoy add unnecessary proxy overhead\n   - Use `traffic.sidecar.istio.io/excludeInboundPorts: \"8080\"` for health port\n\n3. **Use protocol detection to avoid double-proxying:**\n   - If application already does TLS, Envoy re-terminates and re-encrypts\n   - Use `ISTIO_MUTUAL` to avoid double TLS\n\n4. **Consider sidecar-less mode** (Istio ambient mesh):\n   - L4 proxy at node level instead of per-pod sidecar\n   - Eliminates per-pod CPU/memory overhead\n   - Saves 50-100m CPU + 128Mi memory per pod\n\n**Right-sizing rule**: When adding a sidecar, increase application container resources by 30-50% AND add sidecar resources. The total increase is more than just the sidecar's own usage.",
        "edge-case", p))

    pairs.append(make_pair("postmortem-edge-case-008",
        "Our Fluentd log shipper sidecar is consuming 500m CPU and 1Gi memory on high-traffic pods. This is more than the application itself. How do I right-size it?",
        "## Log Shipper Sidecar Right-Sizing\n\n**Fluentd/Fluent Bit resource usage scales with log volume, not application load.** A chatty application can have a log sidecar that costs more than the application.\n\n**What drives log shipper resources:**\n\n| Factor | CPU impact | Memory impact |\n|--------|-----------|---------------|\n| Log volume (lines/sec) | ~50m per 1000 lines/s | ~100Mi per 1000 lines/s buffer |\n| JSON parsing | +30% CPU vs plain text | +50% memory vs plain text |\n| Multi-line parsing | +20% CPU (regex matching) | +50Mi for multi-line buffer |\n| Filtering/transformation | +10-50% CPU per filter | +10Mi per filter |\n| Output buffering | Minimal | 256Mi-1Gi for retry buffer |\n\n**Your case: 500m CPU, 1Gi memory suggests:**\n- Log volume: ~5000-10,000 lines/second\n- JSON parsing with multi-line support\n- Multiple filters (Kubernetes metadata enrichment, field extraction)\n- Output buffering for reliability\n\n**Right-sizing strategies:**\n\n1. **Reduce log volume (biggest impact):**\n   - Reduce application log level: DEBUG→INFO can drop volume 90%\n   - 10K lines/s → 1K lines/s: CPU drops from 500m to 100m, memory from 1Gi to 256Mi\n\n2. **Switch Fluentd → Fluent Bit:**\n   - Fluent Bit uses ~1/10 the memory of Fluentd for equivalent throughput\n   - Same log volume: Fluentd 1Gi → Fluent Bit 100Mi\n   - Fluent Bit is C-based, Fluentd is Ruby-based (higher overhead)\n\n3. **DaemonSet instead of sidecar:**\n   - One Fluent Bit per node instead of per pod\n   - Reads from node's `/var/log/containers/` directory\n   - Eliminates sidecar overhead entirely\n   - Total cluster savings: 200 pods * (100m + 128Mi) = 20,000m + 25.6Gi\n\n4. **Tune buffer sizes:**\n```yaml\n[OUTPUT]\n    Name  forward\n    Match *\n    # Reduce retry buffer (trades reliability for memory)\n    Retry_Limit 3\n    storage.total_limit_size 128Mi\n```\n\n**Right-sizing the sidecar (if keeping it):**\n- CPU: 100m request, 500m limit (burst during log rotation)\n- Memory: 256Mi request, 512Mi limit (buffer headroom)\n- Add rate limiting: `[FILTER] Name throttle Limit 5000` (cap at 5K lines/s)\n\n**Safety floor**: Log shipper sidecar should never have less than 50m CPU / 64Mi memory.",
        "right-sizing", p))

    return pairs


# ── Spot Interruption ────────────────────────────────────────────────


def spot_interruption_pairs() -> list[dict]:
    """Spot/preemptible interruption during peak, graceful migration."""
    p = "Synthetic: AI-generated failure mode knowledge"
    pairs = []

    pairs.append(make_pair("postmortem-edge-case-009",
        "A spot node was reclaimed during peak traffic, and the pods that were rescheduled to on-demand nodes OOMKilled on startup. Why?",
        "## Spot Interruption + Cold Start OOM\n\n**Pods that run fine on spot nodes can OOMKill on restart because startup memory exceeds steady-state memory.**\n\n**The scenario:**\n1. Pod runs on spot node for 3 days. Memory settles to 800Mi (steady state).\n2. Memory limit set to 1Gi (800Mi * 1.25 headroom). Reasonable.\n3. Spot node reclaimed. Pod rescheduled to on-demand node.\n4. Pod starts up: loads caches, warms connections, JIT compiles.\n5. Startup memory: 1.2Gi (50% higher than steady state).\n6. 1.2Gi > 1Gi limit → OOMKilled before reaching steady state.\n7. Retry → same startup pattern → OOMKill loop.\n\n**Why startup memory exceeds steady state:**\n\n| Runtime | Startup memory vs steady | Cause |\n|---------|------------------------|-------|\n| JVM | 1.3-1.5x | Class loading, JIT compilation, buffer pool warmup |\n| Node.js | 1.1-1.2x | Module loading, V8 optimization |\n| Python | 1.2-1.4x | Module imports, ML model loading |\n| Go | 1.0-1.1x | Minimal (no class loading or JIT) |\n| .NET | 1.2-1.3x | Assembly loading, JIT, DI container |\n\n**Fixes:**\n\n1. **Size memory limit for startup, not steady state:**\n   - Limit = startup_peak * 1.25 (not steady_state * 1.25)\n   - For JVM: limit = steady_state * 1.5 * 1.25\n\n2. **Use startup probes with longer timeout:**\n   - Prevent K8s from killing the pod during slow startup\n   ```yaml\n   startupProbe:\n     httpGet:\n       path: /healthz\n     failureThreshold: 30\n     periodSeconds: 10  # 5 minutes to start\n   ```\n\n3. **Separate startup resources** (K8s 1.27+ resize feature):\n   - Request more memory during startup, reduce after\n   - Alpha feature: `InPlacePodVerticalScaling`\n\n4. **Pre-warm on-demand nodes:**\n   - Use DaemonSet to pre-pull images on on-demand nodes\n   - Reduces startup time and memory spike (image decompression uses memory)\n\n**Right-sizing for spot workloads:**\n- Memory request: P95 steady state * 1.20 (for scheduling)\n- Memory limit: startup peak * 1.30 (for cold start headroom)\n- The limit-to-request ratio may be 1.5-2x for JVM workloads on spot — this is correct, not over-provisioning.",
        "edge-case", p))

    return pairs


# ── Resource Quota ───────────────────────────────────────────────────


def resource_quota_pairs() -> list[dict]:
    """Quota exhaustion, LimitRange conflicts."""
    p = "Synthetic: AI-generated failure mode knowledge"
    pairs = []

    pairs.append(make_pair("postmortem-edge-case-010",
        "Our deployment can't scale because the namespace ResourceQuota CPU limit is reached, but actual CPU usage is only 20%. How do I fix this?",
        "## ResourceQuota Exhaustion with Low Usage\n\n**ResourceQuota counts requests and limits, not actual usage.** Over-provisioned pods exhaust quota without using it.\n\n**The scenario:**\n```yaml\napiVersion: v1\nkind: ResourceQuota\nmetadata:\n  name: team-backend\nspec:\n  hard:\n    requests.cpu: \"20\"       # 20 CPU cores budget\n    limits.cpu: \"40\"         # 40 CPU cores max\n    requests.memory: \"40Gi\"\n    limits.memory: \"80Gi\"\n```\n\n**Current state:**\n- 10 deployments, average 4 replicas each = 40 pods\n- Average CPU request: 500m per pod\n- Total CPU requests: 40 * 500m = 20,000m = 20 CPU ← quota exhausted\n- Actual CPU usage: 40 * 100m average = 4,000m = 4 CPU (20% of quota)\n\n**The fix is right-sizing, not quota increase:**\n\n1. **Right-size pods first:**\n   - Reduce CPU requests from 500m to 150m (P95 * 1.20 where P95=125m)\n   - New total: 40 * 150m = 6,000m = 6 CPU\n   - Quota headroom: 14 CPU available for scaling\n\n2. **Then evaluate if quota needs adjustment:**\n   - If right-sized pods still hit quota during scale events → increase quota\n   - If right-sized pods fit comfortably → quota was correct, pods were wasteful\n\n**LimitRange interaction:**\n```yaml\napiVersion: v1\nkind: LimitRange\nmetadata:\n  name: default-limits\nspec:\n  limits:\n  - default:                 # Applied if pod doesn't specify\n      cpu: 500m\n      memory: 512Mi\n    defaultRequest:\n      cpu: 500m\n      memory: 512Mi\n    type: Container\n```\n\n**Problem**: LimitRange sets default 500m CPU for pods without explicit requests. New deployments get 500m even if they only need 50m.\n\n**Fix LimitRange defaults:**\n```yaml\ndefault:\n  cpu: 100m               # Smaller default\n  memory: 128Mi\ndefaultRequest:\n  cpu: 100m\n  memory: 128Mi\nmax:\n  cpu: 4000m              # Cap individual pod requests\n  memory: 8Gi\n```\n\n**Workflow:**\n1. Right-size existing pods (k8s-sage)\n2. Update LimitRange defaults to match typical right-sized values\n3. Review ResourceQuota — may not need changing after right-sizing\n4. Set up alerting: `quota_used / quota_hard > 0.8` → time to right-size or adjust quota",
        "edge-case", p))

    return pairs


# ── Complex Scenarios ────────────────────────────────────────────────


def complex_scenario_pairs() -> list[dict]:
    """Multi-failure cascades, postmortem templates."""
    p = "Synthetic: AI-generated failure mode knowledge"
    pairs = []

    pairs.append(make_pair("postmortem-edge-case-011",
        "We had a cascading failure: a memory leak caused OOMKills, which triggered HPA scale-up, which exhausted node resources, which caused eviction storms. How do we prevent this in the future?",
        "## Multi-Failure Cascade Postmortem\n\n**Timeline reconstruction:**\n```\nT+0:    Memory leak in service-A causes RSS to grow 100Mi/hour\nT+4h:   Pod reaches memory limit → OOMKilled\nT+4h:   HPA sees fewer healthy replicas → scales up to maintain target\nT+4h:   New pods scheduled, including leaked ones that will also OOM\nT+5h:   Multiple pods cycling through OOM → HPA keeps scaling up\nT+5h:   Node resources exhausted by rapid pod churn\nT+5h:   kubelet enters memory pressure → evicts other pods\nT+5.5h: Evicted pods (unrelated services) rescheduled elsewhere\nT+5.5h: Other nodes now also under pressure → cluster-wide eviction\nT+6h:   Alert fires. On-call investigates.\n```\n\n**Root causes (multiple):**\n1. Memory leak in service-A (primary)\n2. No memory-based HPA scale-down (HPA only scales by CPU or custom metrics)\n3. No pod memory limit that would isolate the leak (or limit too generous)\n4. No ResourceQuota to cap namespace expansion\n5. No PodDisruptionBudget on other services\n\n**Prevention (defense in depth):**\n\n**Layer 1 — Detect the leak early:**\n- Alert on memory growth rate: `delta(container_memory_rss[1h]) > 100Mi`\n- Alert on OOMKill count: `kube_pod_container_status_restarts_total` increase > 2/hour\n\n**Layer 2 — Contain the blast radius:**\n- ResourceQuota per namespace: cap max pods and total CPU/memory\n- LimitRange: prevent any single pod from requesting too much\n- PDB on all production services: `minAvailable: 50%`\n\n**Layer 3 — HPA guard rails:**\n- Set `maxReplicas` on HPA (prevents unbounded scaling)\n- Use `behavior.scaleUp.stabilizationWindowSeconds: 300` (5 min cooldown)\n- Add memory-based scaling to catch memory pressure before OOM\n\n**Layer 4 — Right-sizing as prevention:**\n- Right-sized memory limits (Max * 1.25) catch leaks sooner\n- With a 1Gi limit, a 100Mi/hour leak is caught in 10 hours\n- With a 4Gi limit (over-provisioned), same leak takes 40 hours\n- Tighter limits = faster failure = faster detection = less cascade damage\n\n**Key insight**: Over-provisioning doesn't prevent failures — it DELAYS them. When the failure eventually happens, it's bigger because more resources were consumed before detection. Right-sizing makes failures fast and small.",
        "edge-case", p))

    pairs.append(make_pair("postmortem-edge-case-012",
        "How should I structure a postmortem for a resource-related K8s incident and what right-sizing changes should come out of it?",
        "## Resource Incident Postmortem Template\n\n**Section 1: Incident Summary**\n- Service(s) affected and duration\n- User impact (error rate, latency increase, downtime)\n- Resource type (CPU, memory, disk, or combination)\n- Detection method (alert, customer report, or discovered in review)\n\n**Section 2: Timeline**\n- When did resource metrics start deviating from baseline?\n- When was the threshold/limit breached?\n- When was the first visible impact?\n- When was the incident detected?\n- When was it mitigated?\n- Gap between deviation and detection = monitoring improvement opportunity\n\n**Section 3: Resource Analysis**\n```\nResource:     [CPU / Memory / Disk]\nRequest:      [current request value]\nLimit:        [current limit value]\nActual usage: P50=[x] P95=[y] P99=[z] Max=[w]\nAt incident:  [peak value that caused the issue]\nHeadroom:     [Max / Limit = x%]\n```\n\n**Section 4: Root Cause Categories**\n\n| Category | Description | Right-sizing action |\n|----------|------------|--------------------|\n| Under-provisioned limit | Max exceeded limit | Increase limit to Max * 1.25 |\n| Under-provisioned request | Pod evicted (using > request under pressure) | Increase request to P95 * 1.20 |\n| Over-provisioned request | Quota exhaustion blocked scaling | Reduce request to P95 * 1.20 |\n| Memory leak | RSS grows until OOMKill | Fix leak + tighter limit for faster detection |\n| CPU throttling | Latency from CFS enforcement | Increase limit or remove limit |\n| Node pressure | Aggregate pods exceeded node capacity | Right-size + add headroom |\n\n**Section 5: Action Items**\n1. Immediate: Adjust resource requests/limits based on analysis\n2. Short-term: Add monitoring for the specific metric that would have caught this\n3. Long-term: Implement automated right-sizing (k8s-sage) for continuous adjustment\n\n**Section 6: Right-Sizing Changes**\n```yaml\n# Before (incident configuration)\nresources:\n  requests: { cpu: \"X\", memory: \"Y\" }\n  limits:   { cpu: \"X\", memory: \"Y\" }\n\n# After (right-sized based on incident data)\nresources:\n  requests: { cpu: \"X_new\", memory: \"Y_new\" }  # P95 * 1.20\n  limits:   { cpu: \"X_new\", memory: \"Y_new\" }  # Max * 1.25\n\n# Justification: [metric data supporting the change]\n```\n\n**Key principle**: Every resource incident should produce at least one right-sizing change. If the incident was caused by under-provisioning, increase. If it was caused by over-provisioning masking a bug, tighten. Right-sizing is not a one-time activity — it's updated after every incident.",
        "edge-case", p))

    return pairs


# ── Main ─────────────────────────────────────────────────────────────


def generate_all() -> list[dict]:
    """Collect all postmortem pairs."""
    all_pairs = []
    all_pairs.extend(oom_cascade_pairs())
    all_pairs.extend(cpu_throttling_pairs())
    all_pairs.extend(node_pressure_pairs())
    all_pairs.extend(sidecar_pairs())
    all_pairs.extend(spot_interruption_pairs())
    all_pairs.extend(resource_quota_pairs())
    all_pairs.extend(complex_scenario_pairs())

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

    print(f"Wrote {len(all_pairs)} postmortem pairs -> {OUTPUT_PATH}")
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

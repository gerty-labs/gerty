#!/usr/bin/env python3
"""Generate synthetic training pairs from the rules engine logic.

Produces metric→recommendation pairs covering scenario combinations that
real data doesn't reach. Every pair passes metric plausibility checks and
safety invariant validation.

Usage:
    python3 ml/dataset/generate_synthetic.py --count 4500 --seed 42
    python3 ml/dataset/generate_synthetic.py --output ml/dataset/raw/synthetic_pairs.jsonl
"""

import argparse
import json
import logging
import math
import random
from dataclasses import dataclass
from pathlib import Path

logger = logging.getLogger(__name__)

SYSTEM_PROMPT = (
    "You are k8s-sage, a Kubernetes resource efficiency specialist. "
    "Analyse the provided workload metrics and give actionable right-sizing "
    "recommendations. Be specific about numbers, explain your reasoning, and flag risks."
)

# --- Constants mirroring internal/rules ---

HEADROOM_STEADY = 1.20
HEADROOM_BURSTABLE_REQ = 1.20
HEADROOM_BURSTABLE_LIMIT = 1.20
HEADROOM_BATCH_LIMIT = 1.20
MIN_CPU_MILLIS = 50
MIN_MEM_BYTES = 64 * 1024 * 1024  # 64Mi
WASTE_THRESHOLD_PCT = 10.0

# Classification thresholds
CV_STEADY_THRESHOLD = 0.3
BATCH_SPIKE_RATIO = 5.0
BATCH_IDLE_RATIO = 10.0
LOW_USAGE_P50_FLOOR = 25.0
LOW_USAGE_SPIKE_FLOOR = 100.0
IDLE_UTILISATION_THRESHOLD = 0.05
IDLE_MIN_DATA_WINDOW = 48 * 60  # minutes

MiB = 1024 * 1024
GiB = 1024 * MiB

# --- Workload names ---

DEPLOYMENT_NAMES = [
    "payment-service", "api-gateway", "user-service", "notification-service",
    "order-processor", "inventory-manager", "auth-service", "search-indexer",
    "analytics-worker", "billing-engine", "image-resizer", "pdf-generator",
    "email-sender", "webhook-relay", "cache-warmer", "session-manager",
    "rate-limiter", "config-server", "audit-logger", "health-checker",
    "data-pipeline", "ml-inference", "feature-store", "model-serving",
    "recommendation-engine", "fraud-detector", "etl-runner", "log-aggregator",
    "metrics-collector", "event-processor", "scheduler-service", "cron-manager",
    "file-uploader", "video-transcoder", "chat-service", "presence-service",
    "feed-generator", "content-delivery", "geo-service", "pricing-engine",
]

NAMESPACES = ["production", "staging", "default", "backend", "platform", "data", "infra"]

CONTAINER_IMAGES = {
    "jvm": [
        "eclipse-temurin:17-jre", "eclipse-temurin:21-jre", "amazoncorretto:17",
        "openjdk:17-slim", "azul/zulu-openjdk:21",
    ],
    "go": [
        "golang:1.22-alpine", "gcr.io/distroless/static-debian12",
        "scratch", "alpine:3.19",
    ],
    "python": [
        "python:3.12-slim", "python:3.11-slim", "tiangolo/uvicorn-gunicorn:python3.11",
    ],
    "node": [
        "node:20-slim", "node:22-alpine", "node:18-slim",
    ],
    "dotnet": [
        "mcr.microsoft.com/dotnet/aspnet:8.0", "mcr.microsoft.com/dotnet/runtime:8.0",
    ],
    "ruby": [
        "ruby:3.3-slim", "ruby:3.2-alpine",
    ],
    "generic": [
        "nginx:1.25-alpine", "redis:7-alpine", "memcached:1.6-alpine",
        "envoyproxy/envoy:v1.29", "haproxy:2.9-alpine",
    ],
}

JVM_FLAGS = [
    "-Xmx512m -Xms256m -XX:+UseG1GC",
    "-Xmx1g -Xms512m -XX:+UseG1GC -XX:MaxGCPauseMillis=200",
    "-Xmx2g -Xms1g -XX:+UseG1GC",
    "-Xmx3g -Xms1g -XX:+UseG1GC -XX:+UseStringDeduplication",
    "-Xmx4g -Xms2g -XX:+UseZGC",
    "-Xmx6g -Xms2g -XX:+UseG1GC -XX:MaxRAMPercentage=75.0",
    "-Xmx256m -Xms128m -XX:+UseSerialGC",
]

# Parse -Xmx from JVM flags for memory calculations
def parse_xmx_bytes(flags: str) -> int:
    for part in flags.split():
        if part.startswith("-Xmx"):
            val = part[4:]
            if val.endswith("g"):
                return int(val[:-1]) * GiB
            if val.endswith("m"):
                return int(val[:-1]) * MiB
    return 512 * MiB


# --- Pattern profiles ---

@dataclass
class PatternProfile:
    """Defines metric relationships for a workload pattern."""
    cv_range: tuple[float, float]
    p95_p50_ratio: tuple[float, float]
    p99_p95_ratio: tuple[float, float]
    max_p99_ratio: tuple[float, float]


PATTERN_PROFILES = {
    "steady": PatternProfile(
        cv_range=(0.05, 0.25),
        p95_p50_ratio=(1.05, 1.40),
        p99_p95_ratio=(1.05, 1.30),
        max_p99_ratio=(1.05, 1.40),
    ),
    "burstable": PatternProfile(
        cv_range=(0.35, 0.90),
        p95_p50_ratio=(1.50, 4.50),
        p99_p95_ratio=(1.20, 2.50),
        max_p99_ratio=(1.10, 1.80),
    ),
    "batch": PatternProfile(
        cv_range=(0.60, 2.0),
        p95_p50_ratio=(3.0, 8.0),
        p99_p95_ratio=(1.20, 2.50),
        max_p99_ratio=(1.30, 2.50),
    ),
    "idle": PatternProfile(
        cv_range=(0.0, 0.10),
        p95_p50_ratio=(1.0, 1.15),
        p99_p95_ratio=(1.0, 1.10),
        max_p99_ratio=(1.0, 1.20),
    ),
}

# --- Data window profiles ---

DATA_WINDOWS = {
    "7d": 7 * 24 * 60,
    "5d": 5 * 24 * 60,
    "3d": 3 * 24 * 60,
    "1d": 24 * 60,
    "12h": 12 * 60,
    "6h": 6 * 60,
}


# --- Scenario generators ---

@dataclass
class Metrics:
    """Generated workload metrics."""
    cpu_p50: float
    cpu_p95: float
    cpu_p99: float
    cpu_max: float
    mem_p50: float  # bytes
    mem_p95: float
    mem_p99: float
    mem_max: float
    cpu_request: int  # millicores
    cpu_limit: int
    mem_request: int  # bytes
    mem_limit: int
    pattern: str
    data_window: str
    data_window_minutes: float
    replicas: int
    restarts: int
    runtime: str
    container_image: str
    workload_name: str
    namespace: str
    workload_kind: str = "Deployment"
    jvm_flags: str = ""
    qos: str = "Burstable"
    sidecar: str = ""


def rand_range(rng: random.Random, low: float, high: float) -> float:
    return rng.uniform(low, high)


def generate_cpu_metrics(rng: random.Random, profile: PatternProfile, base_p50: float) -> tuple[float, float, float, float]:
    """Generate plausible CPU percentile chain from a P50 baseline."""
    p50 = base_p50
    p95 = p50 * rand_range(rng, *profile.p95_p50_ratio)
    p99 = p95 * rand_range(rng, *profile.p99_p95_ratio)
    max_val = p99 * rand_range(rng, *profile.max_p99_ratio)
    return p50, p95, p99, max_val


def generate_mem_metrics(rng: random.Random, profile: PatternProfile, base_p50: float) -> tuple[float, float, float, float]:
    """Generate plausible memory percentile chain."""
    # Memory is typically less spiky than CPU, so compress the ratios slightly
    p50 = base_p50
    mem_p95_ratio = (
        1.0 + (profile.p95_p50_ratio[0] - 1.0) * 0.6,
        1.0 + (profile.p95_p50_ratio[1] - 1.0) * 0.6,
    )
    mem_p99_ratio = (
        1.0 + (profile.p99_p95_ratio[0] - 1.0) * 0.7,
        1.0 + (profile.p99_p95_ratio[1] - 1.0) * 0.7,
    )
    mem_max_ratio = (
        1.0 + (profile.max_p99_ratio[0] - 1.0) * 0.5,
        1.0 + (profile.max_p99_ratio[1] - 1.0) * 0.5,
    )
    p95 = p50 * rand_range(rng, *mem_p95_ratio)
    p99 = p95 * rand_range(rng, *mem_p99_ratio)
    max_val = p99 * rand_range(rng, *mem_max_ratio)
    return p50, p95, p99, max_val


def pick_workload(rng: random.Random, runtime: str) -> tuple[str, str, str]:
    """Pick a workload name, namespace, and container image."""
    name = rng.choice(DEPLOYMENT_NAMES)
    ns = rng.choice(NAMESPACES)
    image = rng.choice(CONTAINER_IMAGES.get(runtime, CONTAINER_IMAGES["generic"]))
    return name, ns, image


def generate_overprovisioned(rng: random.Random, pattern: str, runtime: str) -> Metrics:
    """Request >> P95, varying waste 20%-95%."""
    profile = PATTERN_PROFILES[pattern]
    waste_factor = rand_range(rng, 1.5, 10.0)  # request is 1.5x to 10x P95

    # CPU
    cpu_p50 = rand_range(rng, 50, 2000)
    cpu_p50, cpu_p95, cpu_p99, cpu_max = generate_cpu_metrics(rng, profile, cpu_p50)
    cpu_request = int(cpu_p95 * waste_factor)
    cpu_limit = int(cpu_request * rand_range(rng, 1.0, 2.0))

    # Memory
    mem_base = rand_range(rng, 64 * MiB, 4 * GiB)
    mem_p50, mem_p95, mem_p99, mem_max = generate_mem_metrics(rng, profile, mem_base)
    mem_waste = rand_range(rng, 1.3, 8.0)
    mem_request = int(mem_p99 * mem_waste)
    mem_limit = int(mem_request * rand_range(rng, 1.0, 2.0))

    name, ns, image = pick_workload(rng, runtime)
    dw_name, dw_minutes = rng.choice(list(DATA_WINDOWS.items()))

    jvm_flags = ""
    if runtime == "jvm":
        jvm_flags = rng.choice(JVM_FLAGS)
        xmx = parse_xmx_bytes(jvm_flags)
        # JVM memory: heap + overhead
        mem_p50 = xmx * rand_range(rng, 0.60, 0.93)
        mem_p95 = xmx * rand_range(rng, 0.85, 0.96)
        mem_p99 = xmx * rand_range(rng, 0.90, 0.98)
        mem_max = xmx * rand_range(rng, 0.95, 1.0)
        # JVM memory request should include heap + metaspace + thread stacks
        overhead = rand_range(rng, 0.20, 0.40)  # 20-40% overhead above -Xmx
        mem_request = int(xmx * (1 + overhead) * mem_waste * 0.5)  # still overprovisioned
        mem_limit = max(mem_request, int(xmx * (1 + overhead)))

    return Metrics(
        cpu_p50=round(cpu_p50, 1), cpu_p95=round(cpu_p95, 1),
        cpu_p99=round(cpu_p99, 1), cpu_max=round(cpu_max, 1),
        mem_p50=mem_p50, mem_p95=mem_p95, mem_p99=mem_p99, mem_max=mem_max,
        cpu_request=cpu_request, cpu_limit=cpu_limit,
        mem_request=mem_request, mem_limit=mem_limit,
        pattern=pattern, data_window=dw_name, data_window_minutes=dw_minutes,
        replicas=rng.choice([1, 2, 3, 3, 5, 8]),
        restarts=0, runtime=runtime, container_image=image,
        workload_name=name, namespace=ns, jvm_flags=jvm_flags,
    )


def generate_underprovisioned(rng: random.Random, pattern: str, runtime: str) -> Metrics:
    """Request < P95, risk of throttling/OOM."""
    profile = PATTERN_PROFILES[pattern]

    cpu_p50 = rand_range(rng, 100, 3000)
    cpu_p50, cpu_p95, cpu_p99, cpu_max = generate_cpu_metrics(rng, profile, cpu_p50)
    # Request is BELOW P95
    cpu_request = int(cpu_p95 * rand_range(rng, 0.3, 0.85))
    cpu_limit = int(cpu_p99 * rand_range(rng, 0.8, 1.2))

    mem_base = rand_range(rng, 128 * MiB, 4 * GiB)
    mem_p50, mem_p95, mem_p99, mem_max = generate_mem_metrics(rng, profile, mem_base)
    mem_request = int(mem_p99 * rand_range(rng, 0.4, 0.85))
    mem_limit = int(mem_max * rand_range(rng, 0.9, 1.3))

    name, ns, image = pick_workload(rng, runtime)
    dw_name, dw_minutes = rng.choice(list(DATA_WINDOWS.items()))

    return Metrics(
        cpu_p50=round(cpu_p50, 1), cpu_p95=round(cpu_p95, 1),
        cpu_p99=round(cpu_p99, 1), cpu_max=round(cpu_max, 1),
        mem_p50=mem_p50, mem_p95=mem_p95, mem_p99=mem_p99, mem_max=mem_max,
        cpu_request=cpu_request, cpu_limit=cpu_limit,
        mem_request=mem_request, mem_limit=mem_limit,
        pattern=pattern, data_window=dw_name, data_window_minutes=dw_minutes,
        replicas=rng.choice([1, 2, 3, 5]),
        restarts=rng.choice([0, 1, 2, 3, 5, 8, 12]),
        runtime=runtime, container_image=image,
        workload_name=name, namespace=ns,
    )


def generate_well_sized(rng: random.Random, pattern: str, runtime: str) -> Metrics:
    """Request is within 10% of recommended — no changes needed."""
    profile = PATTERN_PROFILES[pattern]

    cpu_p50 = rand_range(rng, 50, 2000)
    cpu_p50, cpu_p95, cpu_p99, cpu_max = generate_cpu_metrics(rng, profile, cpu_p50)
    # Request is close to where the rules engine would put it
    if pattern == "steady":
        cpu_request = int(cpu_p95 * HEADROOM_STEADY * rand_range(rng, 0.95, 1.08))
    elif pattern == "burstable":
        cpu_request = int(cpu_p50 * HEADROOM_BURSTABLE_REQ * rand_range(rng, 0.95, 1.08))
    else:
        cpu_request = int(cpu_p50 * HEADROOM_BURSTABLE_REQ * rand_range(rng, 0.95, 1.08))
    cpu_limit = int(cpu_p99 * HEADROOM_BURSTABLE_LIMIT * rand_range(rng, 1.0, 1.15))

    mem_base = rand_range(rng, 64 * MiB, 4 * GiB)
    mem_p50, mem_p95, mem_p99, mem_max = generate_mem_metrics(rng, profile, mem_base)
    mem_request = int(mem_p99 * HEADROOM_STEADY * rand_range(rng, 0.95, 1.08))
    mem_limit = int(mem_max * HEADROOM_BURSTABLE_LIMIT * rand_range(rng, 1.0, 1.15))

    name, ns, image = pick_workload(rng, runtime)
    dw_name, dw_minutes = rng.choice(list(DATA_WINDOWS.items()))

    return Metrics(
        cpu_p50=round(cpu_p50, 1), cpu_p95=round(cpu_p95, 1),
        cpu_p99=round(cpu_p99, 1), cpu_max=round(cpu_max, 1),
        mem_p50=mem_p50, mem_p95=mem_p95, mem_p99=mem_p99, mem_max=mem_max,
        cpu_request=cpu_request, cpu_limit=cpu_limit,
        mem_request=mem_request, mem_limit=mem_limit,
        pattern=pattern, data_window=dw_name, data_window_minutes=dw_minutes,
        replicas=rng.choice([1, 2, 3, 5]),
        restarts=0, runtime=runtime, container_image=image,
        workload_name=name, namespace=ns,
    )


def generate_edge_case(rng: random.Random, runtime: str) -> Metrics:
    """Near-zero usage, extreme spikes, memory leaks, short windows."""
    variant = rng.choice(["near_zero", "extreme_spike", "memory_leak", "short_window", "tiny_workload"])

    name, ns, image = pick_workload(rng, runtime)

    if variant == "near_zero":
        # Almost no usage but resources allocated
        cpu_p50 = rand_range(rng, 0.5, 5)
        cpu_p95 = cpu_p50 * rand_range(rng, 1.0, 2.0)
        cpu_p99 = cpu_p95 * rand_range(rng, 1.0, 1.5)
        cpu_max = cpu_p99 * rand_range(rng, 1.0, 3.0)
        cpu_request = rng.choice([100, 250, 500, 1000, 2000])
        cpu_limit = cpu_request * 2

        mem_p50 = rand_range(rng, 5 * MiB, 20 * MiB)
        mem_p95 = mem_p50 * rand_range(rng, 1.0, 1.2)
        mem_p99 = mem_p95 * rand_range(rng, 1.0, 1.1)
        mem_max = mem_p99 * rand_range(rng, 1.0, 1.2)
        mem_request = rng.choice([256 * MiB, 512 * MiB, 1 * GiB, 2 * GiB])
        mem_limit = mem_request * 2
        dw_name, dw_minutes = rng.choice([("7d", 7*24*60), ("3d", 3*24*60)])
        pattern = "idle" if dw_minutes >= IDLE_MIN_DATA_WINDOW else "steady"

    elif variant == "extreme_spike":
        # Normal baseline with extreme max spike
        cpu_p50 = rand_range(rng, 100, 500)
        cpu_p95 = cpu_p50 * rand_range(rng, 1.5, 3.0)
        cpu_p99 = cpu_p95 * rand_range(rng, 2.0, 4.0)
        cpu_max = cpu_p99 * rand_range(rng, 2.0, 5.0)
        cpu_request = int(cpu_p95 * 1.5)
        cpu_limit = int(cpu_max * 1.2)

        mem_base = rand_range(rng, 200 * MiB, 1 * GiB)
        mem_p50 = mem_base
        mem_p95 = mem_p50 * rand_range(rng, 1.3, 2.0)
        mem_p99 = mem_p95 * rand_range(rng, 1.5, 3.0)
        mem_max = mem_p99 * rand_range(rng, 1.2, 2.0)
        mem_request = int(mem_p95 * 1.5)
        mem_limit = int(mem_max * 1.2)
        dw_name, dw_minutes = rng.choice(list(DATA_WINDOWS.items()))
        pattern = "burstable"

    elif variant == "memory_leak":
        # P50 low, P99 high, Max ≈ P99 (monotonic growth)
        cpu_p50 = rand_range(rng, 100, 800)
        profile = PATTERN_PROFILES["steady"]
        _, cpu_p95, cpu_p99, cpu_max = generate_cpu_metrics(rng, profile, cpu_p50)
        cpu_request = int(cpu_p95 * 2.0)
        cpu_limit = cpu_request * 2

        mem_p50 = rand_range(rng, 200 * MiB, 1 * GiB)
        mem_p99 = mem_p50 * rand_range(rng, 2.5, 5.0)  # growth ratio >= 2.0
        mem_p95 = mem_p50 * rand_range(rng, 1.8, mem_p99 / mem_p50 * 0.9)
        mem_max = mem_p99 * rand_range(rng, 1.0, 1.12)  # max_proximity >= 0.85
        mem_request = int(mem_max * rand_range(rng, 1.1, 2.0))
        mem_limit = int(mem_request * rand_range(rng, 1.0, 1.5))
        dw_name, dw_minutes = rng.choice([("7d", 7*24*60), ("3d", 3*24*60)])
        pattern = "anomalous"

    elif variant == "short_window":
        # Very little data — low confidence
        profile = PATTERN_PROFILES[rng.choice(["steady", "burstable"])]
        cpu_p50 = rand_range(rng, 50, 1500)
        cpu_p50, cpu_p95, cpu_p99, cpu_max = generate_cpu_metrics(rng, profile, cpu_p50)
        cpu_request = int(cpu_p95 * rand_range(rng, 2.0, 5.0))
        cpu_limit = cpu_request * 2

        mem_base = rand_range(rng, 128 * MiB, 2 * GiB)
        mem_p50, mem_p95, mem_p99, mem_max = generate_mem_metrics(rng, profile, mem_base)
        mem_request = int(mem_p99 * rand_range(rng, 2.0, 5.0))
        mem_limit = mem_request * 2
        dw_name, dw_minutes = rng.choice([("6h", 6*60), ("12h", 12*60), ("1d", 24*60)])
        pattern = "steady" if profile == PATTERN_PROFILES["steady"] else "burstable"

    else:  # tiny_workload
        # Everything is tiny — near the floor
        cpu_p50 = rand_range(rng, 1, 15)
        cpu_p95 = cpu_p50 * rand_range(rng, 1.1, 2.0)
        cpu_p99 = cpu_p95 * rand_range(rng, 1.0, 1.5)
        cpu_max = cpu_p99 * rand_range(rng, 1.0, 2.0)
        cpu_request = rng.choice([50, 100, 250])
        cpu_limit = cpu_request * 2

        mem_p50 = rand_range(rng, 10 * MiB, 40 * MiB)
        mem_p95 = mem_p50 * rand_range(rng, 1.0, 1.2)
        mem_p99 = mem_p95 * rand_range(rng, 1.0, 1.1)
        mem_max = mem_p99 * rand_range(rng, 1.0, 1.2)
        mem_request = rng.choice([128 * MiB, 256 * MiB, 512 * MiB])
        mem_limit = mem_request
        dw_name, dw_minutes = rng.choice(list(DATA_WINDOWS.items()))
        pattern = "steady"

    return Metrics(
        cpu_p50=round(cpu_p50, 1), cpu_p95=round(cpu_p95, 1),
        cpu_p99=round(cpu_p99, 1), cpu_max=round(cpu_max, 1),
        mem_p50=mem_p50, mem_p95=mem_p95, mem_p99=mem_p99, mem_max=mem_max,
        cpu_request=max(cpu_request, 1), cpu_limit=max(cpu_limit, cpu_request),
        mem_request=max(mem_request, 1), mem_limit=max(mem_limit, mem_request),
        pattern=pattern, data_window=dw_name, data_window_minutes=dw_minutes,
        replicas=rng.choice([1, 2, 3]),
        restarts=rng.choice([0, 0, 0, 3, 8, 15]) if variant == "memory_leak" else 0,
        runtime=runtime, container_image=image,
        workload_name=name, namespace=ns,
    )


def generate_classification(rng: random.Random, runtime: str) -> Metrics:
    """Metrics at pattern boundary conditions."""
    boundary = rng.choice([
        "steady_burstable", "burstable_batch", "near_idle", "low_p50_spike",
    ])
    name, ns, image = pick_workload(rng, runtime)
    dw_name, dw_minutes = rng.choice(list(DATA_WINDOWS.items()))

    if boundary == "steady_burstable":
        # CV right at the 0.3 boundary
        cpu_p50 = rand_range(rng, 100, 1000)
        # CV ≈ (P95-P50)/P50; target CV ≈ 0.25-0.35
        target_cv = rand_range(rng, 0.25, 0.38)
        cpu_p95 = cpu_p50 * (1 + target_cv)
        cpu_p99 = cpu_p95 * rand_range(rng, 1.1, 1.4)
        cpu_max = cpu_p99 * rand_range(rng, 1.1, 1.5)
        pattern = "steady" if target_cv < CV_STEADY_THRESHOLD else "burstable"

    elif boundary == "burstable_batch":
        # P99/P50 near the batch threshold (5.0)
        cpu_p50 = rand_range(rng, 50, 500)
        target_ratio = rand_range(rng, 4.0, 6.5)
        cpu_p99 = cpu_p50 * target_ratio
        cpu_p95 = cpu_p50 * rand_range(rng, 2.0, target_ratio * 0.8)
        max_ratio = rand_range(rng, 8.0, 12.0)
        cpu_max = cpu_p50 * max_ratio
        is_batch = target_ratio >= BATCH_SPIKE_RATIO and max_ratio >= BATCH_IDLE_RATIO
        pattern = "batch" if is_batch else "burstable"

    elif boundary == "near_idle":
        # Usage just above/below the idle threshold
        cpu_request = rng.choice([500, 1000, 2000])
        target_util = rand_range(rng, 0.02, 0.08)
        cpu_p50 = cpu_request * target_util
        cpu_p95 = cpu_p50 * rand_range(rng, 1.1, 1.5)
        cpu_p99 = cpu_p95 * rand_range(rng, 1.0, 1.3)
        cpu_max = cpu_p99 * rand_range(rng, 1.0, 1.5)
        is_idle = target_util < IDLE_UTILISATION_THRESHOLD and dw_minutes >= IDLE_MIN_DATA_WINDOW
        pattern = "idle" if is_idle else "steady"

        mem_base = rand_range(rng, 30 * MiB, 200 * MiB)
        mem_p50, mem_p95, mem_p99, mem_max = generate_mem_metrics(rng, PATTERN_PROFILES["steady"], mem_base)
        return Metrics(
            cpu_p50=round(cpu_p50, 1), cpu_p95=round(cpu_p95, 1),
            cpu_p99=round(cpu_p99, 1), cpu_max=round(cpu_max, 1),
            mem_p50=mem_p50, mem_p95=mem_p95, mem_p99=mem_p99, mem_max=mem_max,
            cpu_request=cpu_request, cpu_limit=cpu_request * 2,
            mem_request=int(mem_p99 * 3), mem_limit=int(mem_p99 * 4),
            pattern=pattern, data_window=dw_name, data_window_minutes=dw_minutes,
            replicas=rng.choice([1, 2, 3]),
            restarts=0, runtime=runtime, container_image=image,
            workload_name=name, namespace=ns,
        )

    else:  # low_p50_spike
        # P50 below the low-usage floor with real spikes
        cpu_p50 = rand_range(rng, 2, LOW_USAGE_P50_FLOOR - 1)
        has_spike = rng.choice([True, False])
        if has_spike:
            cpu_max = rand_range(rng, LOW_USAGE_SPIKE_FLOOR, 1000)
            cpu_p99 = cpu_max * rand_range(rng, 0.5, 0.9)
            cpu_p95 = cpu_p99 * rand_range(rng, 0.3, 0.8)
            pattern = "burstable"
        else:
            cpu_p95 = cpu_p50 * rand_range(rng, 1.1, 3.0)
            cpu_p99 = cpu_p95 * rand_range(rng, 1.0, 1.5)
            cpu_max = cpu_p99 * rand_range(rng, 1.0, 2.0)
            if cpu_max < LOW_USAGE_SPIKE_FLOOR:
                pattern = "steady"
            else:
                pattern = "burstable"

    # Default: overprovisioned on these boundary workloads
    cpu_request = int(cpu_p95 * rand_range(rng, 2.0, 5.0))
    cpu_limit = cpu_request * 2
    mem_base = rand_range(rng, 128 * MiB, 2 * GiB)
    mem_p50, mem_p95, mem_p99, mem_max = generate_mem_metrics(
        rng, PATTERN_PROFILES.get(pattern, PATTERN_PROFILES["steady"]), mem_base
    )
    mem_request = int(mem_p99 * rand_range(rng, 2.0, 4.0))
    mem_limit = mem_request * 2

    return Metrics(
        cpu_p50=round(cpu_p50, 1), cpu_p95=round(cpu_p95, 1),
        cpu_p99=round(cpu_p99, 1), cpu_max=round(cpu_max, 1),
        mem_p50=mem_p50, mem_p95=mem_p95, mem_p99=mem_p99, mem_max=mem_max,
        cpu_request=max(cpu_request, 1), cpu_limit=max(cpu_limit, cpu_request),
        mem_request=max(mem_request, 1), mem_limit=max(mem_limit, mem_request),
        pattern=pattern, data_window=dw_name, data_window_minutes=dw_minutes,
        replicas=rng.choice([1, 2, 3]),
        restarts=0, runtime=runtime, container_image=image,
        workload_name=name, namespace=ns,
    )


def generate_multicontainer(rng: random.Random, runtime: str) -> Metrics:
    """Sidecars and init containers."""
    m = generate_overprovisioned(rng, rng.choice(["steady", "burstable"]), runtime)
    sidecar = rng.choice([
        "istio-proxy", "envoy-sidecar", "fluentd", "fluentbit",
        "datadog-agent", "linkerd-proxy", "vault-agent",
    ])
    m.sidecar = sidecar
    return m


# --- Recommendation calculator (mirrors Go rules engine) ---

def compute_cpu_recommendation(m: Metrics) -> dict:
    """Calculate CPU recommendation mirroring the Go rules engine."""
    if m.pattern == "anomalous":
        return {
            "action": "investigate",
            "current_req": f"{m.cpu_request}m",
            "current_limit": f"{m.cpu_limit}m",
            "recommended_req": f"{m.cpu_request}m",
            "recommended_limit": f"{m.cpu_limit}m",
            "risk": "HIGH",
        }

    if m.pattern == "steady":
        rec_req = m.cpu_p95 * HEADROOM_STEADY
        rec_limit = m.cpu_p99 * HEADROOM_BURSTABLE_LIMIT
    elif m.pattern == "burstable":
        rec_req = m.cpu_p50 * HEADROOM_BURSTABLE_REQ
        rec_limit = m.cpu_p99 * HEADROOM_BURSTABLE_LIMIT
    elif m.pattern == "batch":
        rec_req = m.cpu_p50 * HEADROOM_BURSTABLE_REQ
        rec_limit = m.cpu_max * HEADROOM_BATCH_LIMIT
    elif m.pattern == "idle":
        rec_req = m.cpu_p95 * HEADROOM_STEADY
        rec_limit = m.cpu_p99 * HEADROOM_BURSTABLE_LIMIT
    else:
        rec_req = m.cpu_p95 * HEADROOM_STEADY
        rec_limit = m.cpu_p99 * HEADROOM_BURSTABLE_LIMIT

    # Confidence
    confidence = compute_confidence(m)

    # Reduction cap
    rec_req, cap_applied = cap_reduction(m.cpu_request, rec_req, confidence)
    rec_limit = max(rec_limit, rec_req)

    # Floor
    rec_req = max(rec_req, MIN_CPU_MILLIS)
    rec_limit = max(rec_limit, rec_req)

    savings = m.cpu_request - rec_req
    waste_pct = (savings / m.cpu_request * 100) if m.cpu_request > 0 else 0

    if savings <= 0:
        action = "upscale"
        risk = "HIGH"
    elif waste_pct < WASTE_THRESHOLD_PCT:
        action = "no_change"
        risk = "LOW"
    else:
        action = "downscale"
        risk = "LOW" if rec_req / m.cpu_p99 >= 1.10 else ("MEDIUM" if rec_req / m.cpu_p99 >= 1.0 else "HIGH")

    return {
        "action": action,
        "current_req": f"{m.cpu_request}m",
        "current_limit": f"{m.cpu_limit}m",
        "recommended_req": f"{math.ceil(rec_req)}m",
        "recommended_limit": f"{math.ceil(rec_limit)}m",
        "savings_millis": max(0, int(savings)),
        "waste_pct": round(waste_pct, 1),
        "risk": risk,
        "confidence": round(confidence, 2),
        "cap_applied": cap_applied,
    }


def compute_mem_recommendation(m: Metrics) -> dict:
    """Calculate memory recommendation mirroring the Go rules engine."""
    if m.pattern == "anomalous":
        return {
            "action": "investigate",
            "current_req": format_bytes(m.mem_request),
            "current_limit": format_bytes(m.mem_limit),
            "recommended_req": format_bytes(m.mem_request),
            "recommended_limit": format_bytes(m.mem_limit),
            "risk": "HIGH",
        }

    if m.pattern in ("steady", "idle"):
        rec_req = m.mem_p99 * HEADROOM_STEADY
        rec_limit = m.mem_max * HEADROOM_BURSTABLE_LIMIT
    else:  # burstable, batch
        rec_req = m.mem_p99 * HEADROOM_BURSTABLE_LIMIT
        rec_limit = m.mem_max * HEADROOM_BURSTABLE_LIMIT

    confidence = compute_confidence(m) * 0.95

    rec_req, cap_applied = cap_reduction(m.mem_request, rec_req, confidence)
    rec_limit = max(rec_limit, rec_req)

    rec_req = max(rec_req, MIN_MEM_BYTES)
    rec_limit = max(rec_limit, rec_req)

    savings = m.mem_request - rec_req
    waste_pct = (savings / m.mem_request * 100) if m.mem_request > 0 else 0

    if savings <= 0:
        action = "upscale"
        risk = "HIGH"
    elif waste_pct < WASTE_THRESHOLD_PCT:
        action = "no_change"
        risk = "LOW"
    else:
        action = "downscale"
        risk = "LOW" if rec_req / m.mem_max >= 1.10 else ("MEDIUM" if rec_req / m.mem_max >= 1.0 else "HIGH")

    return {
        "action": action,
        "current_req": format_bytes(m.mem_request),
        "current_limit": format_bytes(m.mem_limit),
        "recommended_req": format_bytes(math.ceil(rec_req)),
        "recommended_limit": format_bytes(math.ceil(rec_limit)),
        "savings_bytes": max(0, int(savings)),
        "waste_pct": round(waste_pct, 1),
        "risk": risk,
        "confidence": round(confidence, 2),
        "cap_applied": cap_applied,
    }


def compute_confidence(m: Metrics) -> float:
    """Compute confidence mirroring the Go rules engine."""
    dw = m.data_window_minutes
    dw_3d = 3 * 24 * 60
    dw_7d = 7 * 24 * 60

    if dw >= dw_7d:
        base = 0.95
    elif dw >= dw_3d:
        ratio = (dw - dw_3d) / (dw_7d - dw_3d)
        base = 0.85 + ratio * (0.95 - 0.85)
    else:
        ratio = dw / dw_3d
        base = 0.20 + ratio * (0.50 - 0.20)

    if m.pattern == "burstable":
        base = min(base, 0.80)
    elif m.pattern == "batch":
        base = min(base, 0.70)
    elif m.pattern == "anomalous":
        base = 0.0

    return round(base, 2)


def cap_reduction(current: int, recommended: float, confidence: float) -> tuple[float, bool]:
    """Cap reduction based on confidence, mirroring the Go rules engine."""
    if confidence > 0.8:
        max_pct = 0.75
    elif confidence > 0.5:
        max_pct = 0.50
    else:
        max_pct = 0.30
    floor = current * (1.0 - max_pct)
    if recommended < floor:
        return floor, True
    return recommended, False


def format_bytes(b: float) -> str:
    """Format bytes to human-readable K8s units."""
    b = int(b)
    if b >= GiB and b % GiB == 0:
        return f"{b // GiB}Gi"
    if b >= MiB:
        return f"{b // MiB}Mi"
    if b >= 1024:
        return f"{b // 1024}Ki"
    return f"{b}"


def format_bytes_float(b: float) -> str:
    """Format bytes to human-readable with decimal precision."""
    if b >= GiB:
        return f"{b / GiB:.1f}Gi"
    if b >= MiB:
        return f"{b / MiB:.1f}Mi"
    return f"{b / 1024:.1f}Ki"


# --- Explanation templates ---

CPU_OVERPROV_TEMPLATES = [
    "Requesting {req}m but P95 is {p95}m — {waste_pct:.0f}% waste at the request level. The {pattern} pattern (CV={cv:.2f}) suggests this is predictable. Recommend {rec_req}m (P95 + 20% headroom).",
    "CPU request of {req}m is {waste_factor:.1f}x the P95 usage of {p95:.0f}m. With {dw} of stable {pattern} data, confident this can be reduced to {rec_req} safely.",
    "P95 CPU at {p95}m against a {req}m request leaves {waste_pct:.0f}% idle capacity. Reducing to {rec_req}m (P95 * 1.20) reclaims {savings}m per replica.",
    "Over-provisioned by {waste_factor:.1f}x: requesting {req}m, using {p95}m at P95. {pattern_cap} pattern allows {rec_req}m with 20% headroom above P95.",
    "CPU utilisation is only {util_pct:.0f}% of request at P95. The {dw} observation window shows {pattern} behaviour. Safe to reduce request from {req}m to {rec_req}m.",
]

CPU_UNDERPROV_TEMPLATES = [
    "Under-provisioned: current request {req}m is below P95 of {p95}m. The workload is being throttled. Recommend increasing to {rec_req}m.",
    "CPU request ({req}m) is {deficit_pct:.0f}% below P95 usage ({p95}m). This causes CFS throttling and increased latency. Increase to {rec_req}m.",
    "Active throttling risk: P95 CPU ({p95}m) exceeds request ({req}m) by {over}m. Recommend {rec_req}m with 20% headroom.",
]

CPU_WELLSIZED_TEMPLATES = [
    "Well-sized: request of {req}m is within {waste_pct:.0f}% of the recommended {rec_req}m based on P95 usage of {p95}m. No changes needed.",
    "CPU allocation is appropriate. Current {req}m request provides {headroom_pct:.0f}% headroom above P95 ({p95}m). Leave as-is.",
]

CPU_IDLE_TEMPLATES = [
    "Workload is idle: P50 CPU is {p50}m ({util_pct:.1f}% of {req}m request) sustained over {dw}. Consider scaling to zero or removing.",
    "Near-zero utilisation over {dw}: P50={p50}m against {req}m request. If this workload is no longer needed, decommission it to reclaim {req}m.",
]

MEM_OVERPROV_TEMPLATES = [
    "Memory request {req} is {waste_factor:.1f}x P99 working set of {p99}. Recommend {rec_req} (P99 + 20% headroom). OOMKill risk is {risk} — P99 is well below the new request.",
    "P99 memory at {p99} against {req} request leaves {waste_pct:.0f}% unused. Safe to reduce to {rec_req} based on {dw} of data.",
    "Over-allocated memory: using {p99} at P99, requesting {req}. The {pattern} pattern is stable; {rec_req} provides adequate headroom.",
    "Memory waste of {waste_pct:.0f}%: {req} requested, {p99} used at P99. Reducing to {rec_req} saves {savings} per replica with {risk} OOMKill risk.",
]

MEM_UNDERPROV_TEMPLATES = [
    "Under-provisioned memory: request {req} is below P99 of {p99}. OOMKill risk is HIGH. Increase to {rec_req} immediately.",
    "Memory request ({req}) is {deficit_pct:.0f}% below P99 working set ({p99}). This is an active OOMKill risk. Recommend {rec_req}.",
]

MEM_WELLSIZED_TEMPLATES = [
    "Memory is well-sized: {req} request provides {headroom_pct:.0f}% headroom above P99 ({p99}). No changes recommended.",
    "Current memory allocation of {req} is appropriate for P99 usage of {p99}. Leave as-is.",
]

MEM_LEAK_TEMPLATES = [
    "Possible memory leak detected: P50={p50}, P99={p99}, Max={max}. Memory shows monotonic growth (P99/P50 ratio of {ratio:.1f}x). Do NOT reduce resources — investigate the leak first.",
    "Anomalous memory pattern: usage grew from {p50} (P50) to {p99} (P99) with Max at {max}. This suggests a memory leak or unbounded cache. Investigate before making resource changes.",
    "Memory growth detected: P99 is {ratio:.1f}x P50, and Max is within {prox_pct:.0f}% of P99. This is characteristic of a leak. Hold resources at current levels and investigate.",
]

JVM_TEMPLATES = [
    "\n\n**JVM Note**: Configured with {flags}. The -Xmx{xmx} reserves heap up to that limit. High memory usage tracking the heap ceiling is normal GC behaviour, not waste. {jvm_advice}",
    "\n\n**JVM Runtime**: {flags}. G1GC will use heap up to -Xmx{xmx} in a sawtooth pattern. Memory at 90%+ of -Xmx is healthy. {jvm_advice}",
    "\n\n**Runtime-specific**: This JVM ({flags}) needs -Xmx{xmx} for heap plus ~{overhead} for metaspace, JIT code, thread stacks, and native memory. {jvm_advice}",
]

JVM_ADVICE_OVERPROV = [
    "Reduce CPU request but keep memory above -Xmx + overhead to prevent OOMKill during full GC.",
    "CPU can be safely reduced. Memory must cover heap + off-heap. Current allocation provides adequate headroom.",
]

JVM_ADVICE_WELLSIZED = [
    "Both CPU and memory allocation are appropriate for this JVM configuration.",
    "Memory correctly accounts for -Xmx plus runtime overhead. No changes needed on memory side.",
]

GO_TEMPLATES = [
    "\n\n**Go Runtime**: Go's garbage collector targets a memory overhead ratio (GOGC, default 100%). Actual heap usage will fluctuate around 2x live heap. {advice}",
    "\n\n**Go Runtime Note**: Memory usage may appear high due to Go's conservative GC. The runtime retains freed memory for reuse rather than returning it to the OS immediately. {advice}",
]

PYTHON_TEMPLATES = [
    "\n\n**Python Runtime**: CPython's GIL means this workload is effectively single-threaded for CPU-bound work. CPU limit above 1 core provides no benefit for pure Python. {advice}",
    "\n\n**Python Note**: Python memory may appear higher than expected due to GC fragmentation. The reference-counting GC doesn't compact memory, so long-running processes accumulate fragmentation. {advice}",
]

NODE_TEMPLATES = [
    "\n\n**Node.js Runtime**: The event loop is single-threaded; CPU limit above 1 core helps only with worker threads or native addons. V8 heap default is ~1.5Gi. {advice}",
    "\n\n**Node.js Note**: V8's heap is limited by --max-old-space-size (default ~1.5Gi). Memory usage near this ceiling triggers more frequent GC. {advice}",
]

DOTNET_TEMPLATES = [
    "\n\n**.NET Runtime**: The CLR GC workstation mode uses less memory but more CPU. Server GC mode (default in containers) pre-allocates heap segments. High memory is expected. {advice}",
]

RUBY_TEMPLATES = [
    "\n\n**Ruby Runtime**: CRuby's GIL limits CPU parallelism to one core for Ruby code. Memory fragmentation from malloc is common in long-running Rails apps. {advice}",
]

SIDECAR_TEMPLATES = [
    "\n\n**Note**: This pod includes a {sidecar} sidecar. The metrics above are for the primary container only. The sidecar's resource usage is separate and should be sized independently. Ensure total pod resources account for both containers.",
    "\n\n**Multi-container**: A {sidecar} sidecar is co-located in this pod. Right-size each container independently — the sidecar typically needs 100-200m CPU and 128-256Mi memory depending on traffic volume.",
]

SHORT_WINDOW_NOTE = [
    "\n\n**Low Confidence**: Only {dw} of data available. Recommendations are conservative (reduction capped at {cap_pct}%). Re-evaluate after 7 days of data collection.",
    "\n\n**Caution**: {dw} data window is short. Confidence is {conf:.2f}. Apply conservatively and monitor for 48 hours after changes.",
]


def build_explanation(m: Metrics, cpu_rec: dict, mem_rec: dict, rng: random.Random) -> str:
    """Build a natural language explanation from metrics and recommendations."""
    parts = ["## Analysis\n"]

    # CPU section
    parts.append("**CPU**:")
    cv = (m.cpu_p95 - m.cpu_p50) / m.cpu_p50 if m.cpu_p50 > 0 else 0

    if m.pattern == "anomalous":
        parts.append(" Anomalous pattern detected. Do not modify CPU resources until the memory anomaly is investigated.\n")
    elif cpu_rec["action"] == "downscale":
        tmpl = rng.choice(CPU_OVERPROV_TEMPLATES)
        parts.append(" " + tmpl.format(
            req=m.cpu_request, p95=m.cpu_p95, p50=m.cpu_p50,
            waste_pct=cpu_rec["waste_pct"],
            waste_factor=m.cpu_request / m.cpu_p95 if m.cpu_p95 > 0 else 0,
            pattern=m.pattern, pattern_cap=m.pattern.capitalize(),
            cv=cv, dw=m.data_window,
            rec_req=cpu_rec["recommended_req"],
            savings=cpu_rec.get("savings_millis", 0),
            util_pct=(m.cpu_p95 / m.cpu_request * 100) if m.cpu_request > 0 else 0,
        ) + "\n")
    elif cpu_rec["action"] == "upscale":
        tmpl = rng.choice(CPU_UNDERPROV_TEMPLATES)
        parts.append(" " + tmpl.format(
            req=m.cpu_request, p95=m.cpu_p95,
            rec_req=cpu_rec["recommended_req"],
            deficit_pct=((m.cpu_p95 - m.cpu_request) / m.cpu_request * 100) if m.cpu_request > 0 else 0,
            over=int(m.cpu_p95 - m.cpu_request),
        ) + "\n")
    elif cpu_rec["action"] == "no_change":
        tmpl = rng.choice(CPU_WELLSIZED_TEMPLATES)
        parts.append(" " + tmpl.format(
            req=m.cpu_request, p95=m.cpu_p95,
            rec_req=cpu_rec["recommended_req"],
            waste_pct=cpu_rec["waste_pct"],
            headroom_pct=((m.cpu_request - m.cpu_p95) / m.cpu_p95 * 100) if m.cpu_p95 > 0 else 0,
        ) + "\n")

    if m.pattern == "idle":
        tmpl = rng.choice(CPU_IDLE_TEMPLATES)
        parts.append(" " + tmpl.format(
            p50=m.cpu_p50, req=m.cpu_request, dw=m.data_window,
            util_pct=(m.cpu_p50 / m.cpu_request * 100) if m.cpu_request > 0 else 0,
        ) + "\n")

    # Memory section
    parts.append("\n**Memory**:")

    if m.pattern == "anomalous":
        tmpl = rng.choice(MEM_LEAK_TEMPLATES)
        ratio = m.mem_p99 / m.mem_p50 if m.mem_p50 > 0 else 0
        prox = m.mem_p99 / m.mem_max if m.mem_max > 0 else 0
        parts.append(" " + tmpl.format(
            p50=format_bytes_float(m.mem_p50),
            p99=format_bytes_float(m.mem_p99),
            max=format_bytes_float(m.mem_max),
            ratio=ratio, prox_pct=prox * 100,
        ) + "\n")
    elif mem_rec["action"] == "downscale":
        tmpl = rng.choice(MEM_OVERPROV_TEMPLATES)
        parts.append(" " + tmpl.format(
            req=format_bytes_float(m.mem_request),
            p99=format_bytes_float(m.mem_p99),
            rec_req=mem_rec["recommended_req"],
            waste_pct=mem_rec["waste_pct"],
            waste_factor=m.mem_request / m.mem_p99 if m.mem_p99 > 0 else 0,
            pattern=m.pattern, risk=mem_rec["risk"],
            savings=format_bytes_float(mem_rec.get("savings_bytes", 0)),
            dw=m.data_window,
        ) + "\n")
    elif mem_rec["action"] == "upscale":
        tmpl = rng.choice(MEM_UNDERPROV_TEMPLATES)
        parts.append(" " + tmpl.format(
            req=format_bytes_float(m.mem_request),
            p99=format_bytes_float(m.mem_p99),
            rec_req=mem_rec["recommended_req"],
            deficit_pct=((m.mem_p99 - m.mem_request) / m.mem_request * 100) if m.mem_request > 0 else 0,
        ) + "\n")
    elif mem_rec["action"] == "no_change":
        tmpl = rng.choice(MEM_WELLSIZED_TEMPLATES)
        parts.append(" " + tmpl.format(
            req=format_bytes_float(m.mem_request),
            p99=format_bytes_float(m.mem_p99),
            headroom_pct=((m.mem_request - m.mem_p99) / m.mem_p99 * 100) if m.mem_p99 > 0 else 0,
        ) + "\n")

    # Recommendations section
    parts.append("\n## Recommendations\n")
    parts.append(f"**CPU**: {'Keep' if cpu_rec['action'] == 'no_change' else 'Change'} request "
                 f"from {cpu_rec['current_req']} to {cpu_rec['recommended_req']}, "
                 f"limit from {cpu_rec['current_limit']} to {cpu_rec['recommended_limit']}.\n")
    parts.append(f"- Confidence: {cpu_rec.get('confidence', 0):.2f}\n")
    parts.append(f"- Risk: {cpu_rec['risk']}\n")

    parts.append(f"\n**Memory**: {'Keep' if mem_rec['action'] == 'no_change' else 'Change'} request "
                 f"from {mem_rec['current_req']} to {mem_rec['recommended_req']}, "
                 f"limit from {mem_rec['current_limit']} to {mem_rec['recommended_limit']}.\n")
    parts.append(f"- Confidence: {mem_rec.get('confidence', 0):.2f}\n")
    parts.append(f"- Risk: {mem_rec['risk']}\n")

    # Runtime-specific notes
    if m.runtime == "jvm" and m.jvm_flags:
        xmx_bytes = parse_xmx_bytes(m.jvm_flags)
        xmx_str = format_bytes_float(xmx_bytes)
        overhead_str = format_bytes_float(int(xmx_bytes * 0.3))
        advice_list = JVM_ADVICE_OVERPROV if cpu_rec["action"] == "downscale" else JVM_ADVICE_WELLSIZED
        parts.append(rng.choice(JVM_TEMPLATES).format(
            flags=m.jvm_flags, xmx=xmx_str, overhead=overhead_str,
            jvm_advice=rng.choice(advice_list),
        ))
    elif m.runtime == "go":
        parts.append(rng.choice(GO_TEMPLATES).format(
            advice="Memory overhead from the GC is normal and doesn't indicate a leak."
        ))
    elif m.runtime == "python":
        parts.append(rng.choice(PYTHON_TEMPLATES).format(
            advice="Consider using multiprocessing or async I/O for CPU-bound workloads."
        ))
    elif m.runtime == "node":
        parts.append(rng.choice(NODE_TEMPLATES).format(
            advice="Check --max-old-space-size if memory usage approaches the V8 limit."
        ))
    elif m.runtime == "dotnet":
        parts.append(rng.choice(DOTNET_TEMPLATES).format(
            advice="Check GC mode (workstation vs server) in the container entrypoint."
        ))
    elif m.runtime == "ruby":
        parts.append(rng.choice(RUBY_TEMPLATES).format(
            advice="Consider jemalloc to reduce memory fragmentation in long-running processes."
        ))

    # Sidecar note
    if m.sidecar:
        parts.append(rng.choice(SIDECAR_TEMPLATES).format(sidecar=m.sidecar))

    # Short window warning
    confidence = compute_confidence(m)
    if m.data_window_minutes < 3 * 24 * 60:
        cap_pct = "30" if confidence <= 0.5 else "50"
        parts.append(rng.choice(SHORT_WINDOW_NOTE).format(
            dw=m.data_window, cap_pct=cap_pct, conf=confidence,
        ))

    return "".join(parts)


def build_user_message(m: Metrics) -> str:
    """Build the user message with workload metrics."""
    lines = [
        f"Workload: {m.workload_kind.lower()}/{m.workload_name} in namespace {m.namespace}",
        f"Replicas: {m.replicas}",
        f"Container: app ({m.container_image})",
        f"CPU Request: {m.cpu_request}m, Limit: {m.cpu_limit}m",
        f"Memory Request: {format_bytes(m.mem_request)}, Limit: {format_bytes(m.mem_limit)}",
        f"CPU Usage ({m.data_window}): P50={m.cpu_p50:.0f}m, P95={m.cpu_p95:.0f}m, P99={m.cpu_p99:.0f}m, Max={m.cpu_max:.0f}m",
        f"Memory Usage ({m.data_window}): P50={format_bytes_float(m.mem_p50)}, P95={format_bytes_float(m.mem_p95)}, P99={format_bytes_float(m.mem_p99)}, Max={format_bytes_float(m.mem_max)}",
        f"Memory Working Set ({m.data_window}): P50={format_bytes_float(m.mem_p50 * 0.95)}, P95={format_bytes_float(m.mem_p95 * 0.97)}, P99={format_bytes_float(m.mem_p99 * 0.98)}, Max={format_bytes_float(m.mem_max * 0.99)}",
        f"Pattern: {m.pattern.capitalize()}",
        f"Restarts ({m.data_window}): {m.restarts}",
        f"QoS: {m.qos}",
    ]
    if m.jvm_flags:
        lines.append(f"JVM Flags: {m.jvm_flags}")
    if m.sidecar:
        lines.append(f"Sidecar: {m.sidecar}")
    return "\n".join(lines)


def determine_category(m: Metrics, _cpu_rec: dict, _mem_rec: dict) -> str:
    """Determine the training pair category."""
    if m.pattern == "anomalous":
        return "edge-case"
    if m.sidecar:
        return "edge-case"
    # Only tag as runtime-specific if the explanation has meaningful runtime content
    if m.runtime in ("jvm", "go", "python", "node", "dotnet", "ruby"):
        return "runtime-specific"
    return "right-sizing"


def make_pair(m: Metrics, seq: int, rng: random.Random) -> dict:
    """Create a complete training pair from metrics."""
    cpu_rec = compute_cpu_recommendation(m)
    mem_rec = compute_mem_recommendation(m)

    category = determine_category(m, cpu_rec, mem_rec)
    explanation = build_explanation(m, cpu_rec, mem_rec, rng)
    user_msg = build_user_message(m)

    # Map runtime to schema-allowed values
    schema_runtime = m.runtime if m.runtime in ("jvm", "go", "python", "node", "generic") else "generic"
    schema_pattern = m.pattern if m.pattern in ("steady", "burstable", "batch", "idle") else None

    pair = {
        "id": f"synthetic-{category}-{seq:04d}",
        "source": "synthetic",
        "system": SYSTEM_PROMPT,
        "user": user_msg,
        "assistant": explanation,
        "metadata": {
            "category": category,
            "provenance": f"Synthetic generation from rules engine logic (seed scenario, pattern={m.pattern}, runtime={m.runtime})",
        },
    }

    if schema_runtime != "generic":
        pair["metadata"]["runtime"] = schema_runtime
    if schema_pattern:
        pair["metadata"]["pattern"] = schema_pattern

    return pair


def validate_pair(pair: dict) -> list[str]:
    """Basic validation of a generated pair."""
    errors = []
    if len(pair.get("assistant", "")) < 50:
        errors.append(f"Assistant too short: {len(pair.get('assistant', ''))} chars")
    if pair["metadata"]["category"] not in ("right-sizing", "classification", "runtime-specific", "edge-case"):
        errors.append(f"Invalid category: {pair['metadata']['category']}")
    if "runtime" in pair["metadata"] and pair["metadata"]["runtime"] not in ("jvm", "go", "python", "node", "generic"):
        errors.append(f"Invalid runtime: {pair['metadata']['runtime']}")
    if "pattern" in pair["metadata"] and pair["metadata"]["pattern"] not in ("steady", "burstable", "batch", "idle"):
        errors.append(f"Invalid pattern: {pair['metadata']['pattern']}")

    # Check metric plausibility in user text
    user = pair.get("user", "")
    # Quick check: P50 <= P95 <= P99 <= Max for CPU
    import re
    cpu_match = re.search(r"P50=([0-9.]+)m[^P]{0,80}P95=([0-9.]+)m[^P]{0,80}P99=([0-9.]+)m[^M]{0,80}Max=([0-9.]+)m", user)
    if cpu_match:
        p50, p95, p99, mx = (float(x) for x in cpu_match.groups())
        if p50 > p95:
            errors.append(f"CPU P50 ({p50}) > P95 ({p95})")
        if p95 > p99:
            errors.append(f"CPU P95 ({p95}) > P99 ({p99})")
        if p99 > mx:
            errors.append(f"CPU P99 ({p99}) > Max ({mx})")

    return errors


# --- Main generation loop ---

RUNTIMES = ["jvm", "go", "python", "node", "generic", "dotnet", "ruby"]
PATTERNS = ["steady", "burstable", "batch", "idle"]


def generate_all(count: int, seed: int) -> list[dict]:
    """Generate all synthetic training pairs."""
    rng = random.Random(seed)
    pairs: list[dict] = []
    seq = 0

    # Distribution targets (proportional to count)
    targets = {
        "overprovisioned": int(count * 0.33),   # ~1500
        "underprovisioned": int(count * 0.11),   # ~500
        "runtime_specific": int(count * 0.18),   # ~800
        "edge_case": int(count * 0.15),           # ~700
        "classification": int(count * 0.11),      # ~500
        "well_sized": int(count * 0.07),           # ~300
        "multi_container": int(count * 0.05),      # ~200
    }

    logger.info("Generating %d synthetic pairs (seed=%d)", count, seed)
    logger.info("Distribution: %s", targets)

    # Overprovisioned — bias toward generic runtime for right-sizing category
    for _ in range(targets["overprovisioned"]):
        pattern = rng.choice(PATTERNS)
        runtime = rng.choice(["generic", "generic", "generic", "jvm", "go", "python", "node"])
        m = generate_overprovisioned(rng, pattern, runtime)
        pair = make_pair(m, seq, rng)
        pairs.append(pair)
        seq += 1

    # Underprovisioned — same generic bias
    for _ in range(targets["underprovisioned"]):
        pattern = rng.choice(["steady", "burstable", "batch"])
        runtime = rng.choice(["generic", "generic", "generic", "jvm", "go", "python", "node"])
        m = generate_underprovisioned(rng, pattern, runtime)
        pair = make_pair(m, seq, rng)
        pairs.append(pair)
        seq += 1

    # Runtime-specific (force non-generic runtimes)
    runtime_pool = ["jvm", "go", "python", "node", "dotnet", "ruby"]
    for _ in range(targets["runtime_specific"]):
        pattern = rng.choice(PATTERNS)
        runtime = rng.choice(runtime_pool)
        m = generate_overprovisioned(rng, pattern, runtime)
        pair = make_pair(m, seq, rng)
        # Force category to runtime-specific
        pair["metadata"]["category"] = "runtime-specific"
        pair["id"] = f"synthetic-runtime-specific-{seq:04d}"
        pairs.append(pair)
        seq += 1

    # Edge cases
    for _ in range(targets["edge_case"]):
        runtime = rng.choice(RUNTIMES)
        m = generate_edge_case(rng, runtime)
        pair = make_pair(m, seq, rng)
        pair["metadata"]["category"] = "edge-case"
        pair["id"] = f"synthetic-edge-case-{seq:04d}"
        pairs.append(pair)
        seq += 1

    # Classification boundary scenarios
    for _ in range(targets["classification"]):
        runtime = rng.choice(RUNTIMES)
        m = generate_classification(rng, runtime)
        pair = make_pair(m, seq, rng)
        pair["metadata"]["category"] = "classification"
        pair["id"] = f"synthetic-classification-{seq:04d}"
        pairs.append(pair)
        seq += 1

    # Well-sized
    for _ in range(targets["well_sized"]):
        pattern = rng.choice(PATTERNS)
        runtime = rng.choice(RUNTIMES)
        m = generate_well_sized(rng, pattern, runtime)
        pair = make_pair(m, seq, rng)
        pairs.append(pair)
        seq += 1

    # Multi-container
    for _ in range(targets["multi_container"]):
        runtime = rng.choice(RUNTIMES)
        m = generate_multicontainer(rng, runtime)
        pair = make_pair(m, seq, rng)
        pair["metadata"]["category"] = "edge-case"
        pair["id"] = f"synthetic-edge-case-{seq:04d}"
        pairs.append(pair)
        seq += 1

    # Validate all
    valid = []
    invalid = 0
    for pair in pairs:
        errors = validate_pair(pair)
        if errors:
            logger.warning("Invalid pair %s: %s", pair["id"], errors)
            invalid += 1
        else:
            valid.append(pair)

    logger.info("Generated %d pairs, %d valid, %d invalid", len(pairs), len(valid), invalid)
    return valid


def main() -> None:
    parser = argparse.ArgumentParser(description="Generate synthetic training pairs")
    parser.add_argument("--count", type=int, default=4500, help="Number of pairs to generate")
    parser.add_argument("--seed", type=int, default=42, help="Random seed for reproducibility")
    parser.add_argument(
        "--output", type=Path,
        default=Path("ml/dataset/raw/synthetic_pairs.jsonl"),
        help="Output file path",
    )
    parser.add_argument("--verbose", action="store_true")
    args = parser.parse_args()

    logging.basicConfig(
        level=logging.DEBUG if args.verbose else logging.INFO,
        format="%(asctime)s %(levelname)s %(message)s",
    )

    pairs = generate_all(args.count, args.seed)

    args.output.parent.mkdir(parents=True, exist_ok=True)
    with open(args.output, "w") as f:
        for pair in pairs:
            f.write(json.dumps(pair, ensure_ascii=False) + "\n")

    logger.info("Wrote %d pairs to %s", len(pairs), args.output)

    # Category breakdown
    cats: dict[str, int] = {}
    for p in pairs:
        c = p["metadata"]["category"]
        cats[c] = cats.get(c, 0) + 1
    logger.info("Category distribution:")
    for c, n in sorted(cats.items()):
        logger.info("  %s: %d", c, n)

    # Runtime breakdown
    rts: dict[str, int] = {}
    for p in pairs:
        r = p["metadata"].get("runtime", "generic")
        rts[r] = rts.get(r, 0) + 1
    logger.info("Runtime distribution:")
    for r, n in sorted(rts.items()):
        logger.info("  %s: %d", r, n)

    # Pattern breakdown
    pats: dict[str, int] = {}
    for p in pairs:
        pt = p["metadata"].get("pattern", "none")
        pats[pt] = pats.get(pt, 0) + 1
    logger.info("Pattern distribution:")
    for pt, n in sorted(pats.items()):
        logger.info("  %s: %d", pt, n)


if __name__ == "__main__":
    main()

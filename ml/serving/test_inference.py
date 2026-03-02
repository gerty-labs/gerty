#!/usr/bin/env python3
"""Smoke test for k8s-sage model inference via llama.cpp server.

Sends representative prompts and validates:
- JSON structure of responses
- Presence of required fields
- Safety invariant compliance
- Latency per request

Usage:
    python test_inference.py                          # default localhost:8080
    python test_inference.py --url http://host:8080   # custom URL
"""

import argparse
import json
import sys
import time
import urllib.parse
import urllib.request

_ALLOWED_SCHEMES = {"http", "https"}


def _safe_urlopen(req, **kwargs):
    """Wrapper around urlopen that rejects non-HTTP schemes (e.g. file://)."""
    url = req.full_url if isinstance(req, urllib.request.Request) else req
    scheme = urllib.parse.urlparse(url).scheme.lower()
    if scheme not in _ALLOWED_SCHEMES:
        raise ValueError(f"URL scheme {scheme!r} not allowed, must be http or https")
    return urllib.request.urlopen(req, **kwargs)


REQUIRED_FIELDS = [
    "cpu_request",
    "cpu_limit",
    "memory_request",
    "memory_limit",
    "pattern",
    "confidence",
    "explanation",
]

VALID_PATTERNS = {"steady", "burstable", "batch", "idle", "anomalous"}

SYSTEM_PROMPT = (
    "You are k8s-sage, a Kubernetes resource efficiency specialist. "
    "Analyse the provided workload metrics and respond with a JSON object containing: "
    "cpu_request, cpu_limit, memory_request, memory_limit, pattern, confidence, explanation, risk."
)

TEST_PROMPTS = [
    {
        "name": "steady_web_server",
        "user": (
            "Workload: deployment/nginx-web in namespace production\n"
            "Container: nginx\n"
            "CPU: Request=1000m, Limit=2000m\n"
            "  Usage: P50=120m, P95=180m, P99=250m, Max=400m\n"
            "Memory: Request=512Mi, Limit=1Gi\n"
            "  Usage: P50=180Mi, P95=200Mi, P99=220Mi, Max=250Mi\n"
            "Data window: 7 days, Pattern hints: low variance"
        ),
    },
    {
        "name": "burstable_api",
        "user": (
            "Workload: deployment/api-gateway in namespace default\n"
            "Container: api\n"
            "CPU: Request=2000m, Limit=4000m\n"
            "  Usage: P50=300m, P95=1500m, P99=2800m, Max=3500m\n"
            "Memory: Request=1Gi, Limit=2Gi\n"
            "  Usage: P50=400Mi, P95=800Mi, P99=1100Mi, Max=1400Mi\n"
            "Data window: 5 days, Pattern hints: high variance, periodic spikes"
        ),
    },
    {
        "name": "batch_cronjob",
        "user": (
            "Workload: cronjob/data-pipeline in namespace batch\n"
            "Container: etl\n"
            "CPU: Request=4000m, Limit=8000m\n"
            "  Usage: P50=100m, P95=3000m, P99=6000m, Max=7500m\n"
            "Memory: Request=4Gi, Limit=8Gi\n"
            "  Usage: P50=500Mi, P95=3Gi, P99=5Gi, Max=6Gi\n"
            "Data window: 14 days, Pattern hints: idle between runs, extreme spikes"
        ),
    },
    {
        "name": "idle_workload",
        "user": (
            "Workload: deployment/legacy-service in namespace staging\n"
            "Container: app\n"
            "CPU: Request=500m, Limit=1000m\n"
            "  Usage: P50=5m, P95=10m, P99=15m, Max=20m\n"
            "Memory: Request=256Mi, Limit=512Mi\n"
            "  Usage: P50=30Mi, P95=35Mi, P99=38Mi, Max=40Mi\n"
            "Data window: 30 days, Pattern hints: consistently near-zero usage"
        ),
    },
    {
        "name": "jvm_application",
        "user": (
            "Workload: deployment/order-service in namespace production\n"
            "Container: java-app\n"
            "CPU: Request=2000m, Limit=4000m\n"
            "  Usage: P50=800m, P95=1200m, P99=1500m, Max=2000m\n"
            "Memory: Request=2Gi, Limit=4Gi\n"
            "  Usage: P50=1.5Gi, P95=1.8Gi, P99=2.0Gi, Max=2.2Gi\n"
            "Data window: 10 days, Runtime: JVM (heap=1536m), Pattern hints: steady with GC spikes"
        ),
    },
]


def send_prompt(url: str, system: str, user: str, max_tokens: int = 512) -> tuple[str, float]:
    """Send prompt to llama.cpp server. Returns (response_text, latency_ms)."""
    prompt = f"<|system|>\n{system}\n<|user|>\n{user}\n<|assistant|>\n"

    payload = json.dumps({
        "prompt": prompt,
        "n_predict": max_tokens,
        "temperature": 0.1,
        "stop": ["<|end|>", "</s>", "<|im_end|>"],
    }).encode()

    req = urllib.request.Request(
        f"{url}/completion",
        data=payload,
        headers={"Content-Type": "application/json"},
    )

    start = time.monotonic()
    with _safe_urlopen(req, timeout=30) as resp:
        result = json.loads(resp.read())
    latency = (time.monotonic() - start) * 1000

    return result.get("content", ""), latency


def validate_response(name: str, text: str) -> list[str]:
    """Validate model response structure. Returns list of errors."""
    errors = []

    # Parse JSON
    try:
        data = json.loads(text.strip())
    except json.JSONDecodeError:
        # Try extracting JSON from text
        import re

        match = re.search(r"\{.*\}", text, re.DOTALL)
        if not match:
            return ["No valid JSON found in response"]
        try:
            data = json.loads(match.group(0))
        except json.JSONDecodeError:
            return ["Failed to parse JSON from response"]

    # Check required fields
    for field in REQUIRED_FIELDS:
        if field not in data:
            errors.append(f"Missing required field: {field}")

    # Validate pattern
    pattern = data.get("pattern", "")
    if pattern and pattern.lower() not in VALID_PATTERNS:
        errors.append(f"Invalid pattern: {pattern}")

    # Validate confidence
    confidence = data.get("confidence")
    if confidence is not None:
        try:
            conf_val = float(confidence)
            if conf_val < 0 or conf_val > 1:
                errors.append(f"Confidence out of range [0,1]: {conf_val}")
        except (ValueError, TypeError):
            errors.append(f"Invalid confidence value: {confidence}")

    # Safety: no zero CPU/memory requests
    for field in ["cpu_request", "memory_request"]:
        val = data.get(field)
        if val is not None:
            val_str = str(val).strip()
            if val_str in ("0", "0m", "0Mi", "0Gi"):
                errors.append(f"Safety violation: {field} is zero")

    return errors


def check_health(url: str) -> bool:
    """Check if llama.cpp server is reachable."""
    try:
        req = urllib.request.Request(f"{url}/health")
        with _safe_urlopen(req, timeout=5) as resp:
            return resp.status == 200
    except Exception:
        return False


def main() -> None:
    parser = argparse.ArgumentParser(description="Smoke test k8s-sage inference")
    parser.add_argument("--url", default="http://localhost:8080", help="llama.cpp server URL")
    args = parser.parse_args()

    print(f"Testing k8s-sage inference at {args.url}")
    print("=" * 60)

    # Health check
    if not check_health(args.url):
        print(f"FAIL: Server not reachable at {args.url}")
        print("Start the server with: ./ml/serving/run_llama_cpp.sh")
        sys.exit(1)
    print("Health check: OK\n")

    passed = 0
    failed = 0
    latencies = []

    for test in TEST_PROMPTS:
        name = test["name"]
        print(f"Test: {name}")

        try:
            response, latency = send_prompt(args.url, SYSTEM_PROMPT, test["user"])
            latencies.append(latency)

            errors = validate_response(name, response)
            if errors:
                print(f"  FAIL ({latency:.0f}ms)")
                for err in errors:
                    print(f"    - {err}")
                failed += 1
            else:
                print(f"  PASS ({latency:.0f}ms)")
                passed += 1

        except Exception as e:
            print(f"  ERROR: {e}")
            failed += 1

    # Summary
    print("\n" + "=" * 60)
    print(f"Results: {passed} passed, {failed} failed, {len(TEST_PROMPTS)} total")
    if latencies:
        avg_lat = sum(latencies) / len(latencies)
        max_lat = max(latencies)
        print(f"Latency: avg={avg_lat:.0f}ms, max={max_lat:.0f}ms")

    sys.exit(0 if failed == 0 else 1)


if __name__ == "__main__":
    main()

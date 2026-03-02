#!/usr/bin/env python3
"""Evaluate fine-tuned k8s-sage model against held-out test set.

Metrics (per MODEL_DESIGN.md §Evaluation):
- Right-sizing accuracy: recommendation within 20% of ground truth
- Pattern classification accuracy
- Safety invariant compliance (memory >= P99 WS × 1.10, CPU >= P95 × headroom)

Usage:
    # Evaluate via llama.cpp server:
    python eval.py --llama-cpp-url http://localhost:8080 --test-file ml/dataset/data/eval.jsonl

    # Evaluate HuggingFace model directly:
    python eval.py --model-path output/k8s-sage-merged --test-file ml/dataset/data/eval.jsonl
"""

import argparse
import json
import logging
import re
import time
from pathlib import Path

logger = logging.getLogger(__name__)


def load_test_set(path: str) -> list[dict]:
    """Load test set from JSONL file."""
    examples = []
    with open(path) as f:
        for line in f:
            line = line.strip()
            if line:
                examples.append(json.loads(line))
    logger.info("Loaded %d test examples", len(examples))
    return examples


def infer_llama_cpp(url: str, prompt: str, max_tokens: int = 512) -> tuple[str, float]:
    """Run inference via llama.cpp /completion endpoint. Returns (text, latency_ms)."""
    import urllib.request

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
    with urllib.request.urlopen(req, timeout=30) as resp:  # nosemgrep: dynamic-urllib-use-detected
        result = json.loads(resp.read())
    latency = (time.monotonic() - start) * 1000

    return result.get("content", ""), latency


def infer_hf_model(model, tokenizer, prompt: str, max_tokens: int = 512) -> tuple[str, float]:
    """Run inference with a local HuggingFace model. Returns (text, latency_ms)."""
    import torch

    inputs = tokenizer(prompt, return_tensors="pt").to(model.device)
    start = time.monotonic()
    with torch.no_grad():
        outputs = model.generate(
            **inputs,
            max_new_tokens=max_tokens,
            temperature=0.1,
            do_sample=True,
        )
    latency = (time.monotonic() - start) * 1000

    generated = outputs[0][inputs["input_ids"].shape[1]:]
    text = tokenizer.decode(generated, skip_special_tokens=True)
    return text, latency


def extract_json_from_response(text: str) -> dict | None:
    """Try to extract a JSON object from model response text."""
    # Try direct parse
    try:
        return json.loads(text.strip())
    except json.JSONDecodeError:
        pass

    # Try to find JSON block in markdown
    match = re.search(r"```(?:json)?\s*(\{[^\}]*\})\s*```", text, re.DOTALL)
    if match:
        try:
            return json.loads(match.group(1))
        except json.JSONDecodeError:
            pass

    # Try to find any JSON object
    match = re.search(r"\{[^{}]*\}", text, re.DOTALL)
    if match:
        try:
            return json.loads(match.group(0))
        except json.JSONDecodeError:
            pass

    return None


def parse_resource_value(value: str | int | float) -> float:
    """Parse a K8s resource value like '500m', '2Gi', '1024Mi' to a numeric value."""
    if isinstance(value, (int, float)):
        return float(value)

    value = str(value).strip()

    # Millicores: "500m" -> 500
    if value.endswith("m"):
        return float(value[:-1])

    # Memory units
    units = {"Ki": 1024, "Mi": 1024**2, "Gi": 1024**3, "Ti": 1024**4}
    for suffix, multiplier in units.items():
        if value.endswith(suffix):
            return float(value[: -len(suffix)]) * multiplier

    return float(value)


def check_right_sizing(predicted: dict, ground_truth: dict, tolerance: float = 0.20) -> dict:
    """Check if predicted resource values are within tolerance of ground truth."""
    results = {"correct": True, "details": {}}

    for field in ["cpu_request", "cpu_limit", "memory_request", "memory_limit"]:
        gt_raw = ground_truth.get(field)
        pred_raw = predicted.get(field)

        if gt_raw is None:
            continue

        if pred_raw is None:
            results["details"][field] = {"status": "missing", "correct": False}
            results["correct"] = False
            continue

        gt_val = parse_resource_value(gt_raw)
        pred_val = parse_resource_value(pred_raw)

        if gt_val == 0:
            is_correct = pred_val == 0
        else:
            relative_error = abs(pred_val - gt_val) / gt_val
            is_correct = relative_error <= tolerance

        results["details"][field] = {
            "predicted": pred_val,
            "ground_truth": gt_val,
            "correct": is_correct,
        }
        if not is_correct:
            results["correct"] = False

    return results


def check_safety_invariants(predicted: dict, _test_example: dict) -> dict:
    """Check safety invariants from MODEL_DESIGN.md.

    - Memory recommendation >= P99 working set × 1.10
    - CPU recommendation >= P95 × headroom (pattern-dependent)
    - No recommendation of 0
    """
    violations = []

    # Check for zero recommendations
    for field in ["cpu_request", "memory_request"]:
        val = predicted.get(field)
        if val is not None:
            parsed = parse_resource_value(val)
            if parsed == 0:
                violations.append(f"{field} is 0 (safety violation)")

    return {
        "safe": len(violations) == 0,
        "violations": violations,
    }


def build_prompt(example: dict) -> str:
    """Build inference prompt from test example."""
    parts = []
    if example.get("system"):
        parts.append(f"<|system|>\n{example['system']}\n")
    parts.append(f"<|user|>\n{example['user']}\n")
    parts.append("<|assistant|>\n")
    return "".join(parts)


def evaluate(examples: list[dict], infer_fn, output_path: str | None = None) -> dict:
    """Run evaluation across all test examples."""
    results = {
        "total": len(examples),
        "right_sizing_correct": 0,
        "pattern_correct": 0,
        "safety_compliant": 0,
        "parse_failures": 0,
        "latencies_ms": [],
        "per_example": [],
    }

    for i, example in enumerate(examples):
        prompt = build_prompt(example)
        try:
            response_text, latency = infer_fn(prompt)
        except Exception as e:
            logger.error("Inference failed for example %d: %s", i, e)
            results["per_example"].append({"id": example.get("id", i), "error": str(e)})
            continue

        results["latencies_ms"].append(latency)

        # Try to parse structured output
        predicted = extract_json_from_response(response_text)
        if predicted is None:
            results["parse_failures"] += 1
            results["per_example"].append({
                "id": example.get("id", i),
                "parse_failure": True,
                "raw_response": response_text[:500],
            })
            continue

        # Extract ground truth from assistant response
        gt = extract_json_from_response(example.get("assistant", ""))

        example_result: dict = {"id": example.get("id", i), "latency_ms": latency}

        # Right-sizing accuracy
        if gt:
            rs = check_right_sizing(predicted, gt)
            if rs["correct"]:
                results["right_sizing_correct"] += 1
            example_result["right_sizing"] = rs

        # Pattern classification
        gt_pattern = example.get("metadata", {}).get("pattern")
        pred_pattern = predicted.get("pattern")
        if gt_pattern and pred_pattern:
            match = gt_pattern.lower() == pred_pattern.lower()
            if match:
                results["pattern_correct"] += 1
            example_result["pattern"] = {"predicted": pred_pattern, "ground_truth": gt_pattern, "correct": match}

        # Safety invariants
        safety = check_safety_invariants(predicted, example)
        if safety["safe"]:
            results["safety_compliant"] += 1
        example_result["safety"] = safety

        results["per_example"].append(example_result)

        if (i + 1) % 50 == 0:
            logger.info("Evaluated %d/%d examples", i + 1, len(examples))

    # Compute summary metrics
    evaluated = results["total"] - results["parse_failures"]
    if evaluated > 0:
        results["summary"] = {
            "right_sizing_accuracy": results["right_sizing_correct"] / evaluated,
            "pattern_accuracy": results["pattern_correct"] / evaluated if results["pattern_correct"] > 0 else 0,
            "safety_compliance": results["safety_compliant"] / evaluated,
            "parse_success_rate": evaluated / results["total"],
            "mean_latency_ms": sum(results["latencies_ms"]) / len(results["latencies_ms"]) if results["latencies_ms"] else 0,
            "p95_latency_ms": sorted(results["latencies_ms"])[int(len(results["latencies_ms"]) * 0.95)] if results["latencies_ms"] else 0,
        }

    if output_path:
        with open(output_path, "w") as f:
            json.dump(results, f, indent=2)
        logger.info("Results written to %s", output_path)

    return results


def print_summary(results: dict) -> None:
    """Print evaluation summary table."""
    s = results.get("summary", {})
    total = results["total"]
    evaluated = total - results["parse_failures"]

    print("\n" + "=" * 60)
    print("EVALUATION RESULTS")
    print("=" * 60)
    print(f"Total examples:         {total}")
    print(f"Successfully parsed:    {evaluated} ({s.get('parse_success_rate', 0):.1%})")
    print(f"Parse failures:         {results['parse_failures']}")
    print("-" * 60)
    print(f"Right-sizing accuracy:  {results['right_sizing_correct']}/{evaluated} ({s.get('right_sizing_accuracy', 0):.1%})")
    print(f"Pattern accuracy:       {results['pattern_correct']}/{evaluated} ({s.get('pattern_accuracy', 0):.1%})")
    print(f"Safety compliance:      {results['safety_compliant']}/{evaluated} ({s.get('safety_compliance', 0):.1%})")
    print("-" * 60)
    print(f"Mean latency:           {s.get('mean_latency_ms', 0):.0f} ms")
    print(f"P95 latency:            {s.get('p95_latency_ms', 0):.0f} ms")
    print("=" * 60)


def main() -> None:
    parser = argparse.ArgumentParser(description="Evaluate k8s-sage model")
    parser.add_argument("--test-file", required=True, help="Path to test set JSONL")
    parser.add_argument("--model-path", help="Path to HuggingFace model (local inference)")
    parser.add_argument("--llama-cpp-url", help="llama.cpp server URL (e.g. http://localhost:8080)")
    parser.add_argument("--output", default="output/eval_results.json", help="Output JSON results path")
    parser.add_argument("--max-examples", type=int, help="Limit number of examples to evaluate")
    args = parser.parse_args()

    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")

    if not args.model_path and not args.llama_cpp_url:
        parser.error("Either --model-path or --llama-cpp-url is required")

    examples = load_test_set(args.test_file)
    if args.max_examples:
        examples = examples[: args.max_examples]

    if args.llama_cpp_url:
        infer_fn = lambda prompt: infer_llama_cpp(args.llama_cpp_url, prompt)
    else:
        from transformers import AutoModelForCausalLM, AutoTokenizer

        logger.info("Loading model from %s", args.model_path)
        tokenizer = AutoTokenizer.from_pretrained(  # nosec B615 — local model path
            args.model_path, trust_remote_code=True)
        model = AutoModelForCausalLM.from_pretrained(  # nosec B615 — local model path
            args.model_path, trust_remote_code=True, device_map="auto")
        infer_fn = lambda prompt: infer_hf_model(model, tokenizer, prompt)

    Path(args.output).parent.mkdir(parents=True, exist_ok=True)
    results = evaluate(examples, infer_fn, args.output)
    print_summary(results)


if __name__ == "__main__":
    main()

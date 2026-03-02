#!/usr/bin/env python3
"""Validate, deduplicate, and merge all data sources into the final training dataset.

Reads JSONL files from all collection scripts and expert examples,
validates each pair against the schema, runs deduplication, and
outputs the final instruction-tuning dataset.

Usage:
    python format_instruct.py --output ml/dataset/data/training_data.jsonl
"""

import argparse
import json
import logging
import re
from pathlib import Path

logger = logging.getLogger(__name__)

ALLOWED_SOURCES = {"k8s-docs", "github", "stackoverflow", "vpa-source", "expert", "synthetic"}
ALLOWED_CATEGORIES = {"right-sizing", "classification", "runtime-specific", "edge-case"}
ALLOWED_RUNTIMES = {"jvm", "go", "python", "node", "generic"}
ALLOWED_PATTERNS = {"steady", "burstable", "batch", "idle"}

# Maximum percentage of synthetic pairs in the final dataset.
# Originally 15%, raised to 60% after volume assessment confirmed acceptable
# composition: real data provides language diversity, synthetic provides
# scenario coverage with programmatic safety validation.
MAX_SYNTHETIC_RATIO = 0.60

# Input files to merge (relative to project root).
INPUT_FILES = [
    "ml/dataset/examples/expert_pairs.jsonl",
    "ml/dataset/raw/k8s_docs_pairs.jsonl",
    "ml/dataset/raw/github_issues_pairs.jsonl",
    "ml/dataset/raw/stackoverflow_filtered.jsonl",
    "ml/dataset/raw/vpa_source_pairs.jsonl",
    "ml/dataset/raw/synthetic_pairs.jsonl",
]


class ValidationError(Exception):
    """Raised when a training pair fails validation."""

    pass


def validate_pair(pair: dict, _pair_index: int) -> list[str]:
    """Validate a single training pair against schema and safety invariants.

    Returns a list of error messages. Empty list means valid.
    """
    errors = []

    # Required fields.
    for field in ["id", "source", "system", "user", "assistant", "metadata"]:
        if field not in pair:
            errors.append(f"Missing required field: {field}")

    if errors:
        return errors  # Can't validate further without required fields

    # Source validation.
    if pair["source"] not in ALLOWED_SOURCES:
        errors.append(
            f"Invalid source '{pair['source']}'. "
            f"Allowed: {', '.join(sorted(ALLOWED_SOURCES))}"
        )

    # System prompt validation.
    if not pair.get("system") or len(pair["system"]) < 20:
        errors.append("System prompt is missing or too short (< 20 chars)")

    # Assistant response length.
    if len(pair.get("assistant", "")) < 50:
        errors.append(
            f"Assistant response too short ({len(pair.get('assistant', ''))} chars, "
            f"minimum 50)"
        )

    # Metadata validation.
    metadata = pair.get("metadata", {})
    if "category" not in metadata:
        errors.append("Missing metadata.category")
    elif metadata["category"] not in ALLOWED_CATEGORIES:
        errors.append(f"Invalid category '{metadata['category']}'")

    if "provenance" not in metadata or len(metadata.get("provenance", "")) < 5:
        errors.append("Missing or too-short metadata.provenance")

    if "runtime" in metadata and metadata["runtime"] not in ALLOWED_RUNTIMES:
        errors.append(f"Invalid runtime '{metadata['runtime']}'")

    if "pattern" in metadata and metadata["pattern"] not in ALLOWED_PATTERNS:
        errors.append(f"Invalid pattern '{metadata['pattern']}'")

    # Metric plausibility checks (if metrics are present in user or assistant).
    metric_errors = validate_metric_plausibility(pair)
    errors.extend(metric_errors)

    return errors


def validate_metric_plausibility(pair: dict) -> list[str]:
    """Check that any metrics mentioned in the pair are physically plausible.

    Validates:
    - P50 <= P95 <= P99 <= Max (wherever these appear together)
    - Recommended request > 0
    - Recommended limit >= recommended request
    """
    errors = []
    text = pair.get("user", "") + "\n" + pair.get("assistant", "")

    # Extract percentile groups (e.g., "P50=120m, P95=340m, P99=890m, Max=1200m").
    # Pattern matches groups like "P50=X, P95=Y, P99=Z, Max=W" with various units.
    percentile_pattern = (
        r"P50[=:]?\s*([0-9.]+)\s*([mMGiKB]*)"
        r".*?P95[=:]?\s*([0-9.]+)\s*([mMGiKB]*)"
        r".*?P99[=:]?\s*([0-9.]+)\s*([mMGiKB]*)"
        r".*?Max[=:]?\s*([0-9.]+)\s*([mMGiKB]*)"
    )

    for match in re.finditer(percentile_pattern, text, re.IGNORECASE):
        try:
            p50 = float(match.group(1))
            p95 = float(match.group(3))
            p99 = float(match.group(5))
            max_val = float(match.group(7))

            # Only compare raw numbers (units may differ, but within a single
            # metric line they should be consistent).
            if p50 > p95:
                errors.append(f"Metric implausibility: P50 ({p50}) > P95 ({p95})")
            if p95 > p99:
                errors.append(f"Metric implausibility: P95 ({p95}) > P99 ({p99})")
            if p99 > max_val:
                errors.append(f"Metric implausibility: P99 ({p99}) > Max ({max_val})")
        except (ValueError, IndexError):
            pass  # Regex matched but values couldn't be parsed, skip

    # Check for zero recommendations in assistant text.
    assistant = pair.get("assistant", "")
    if re.search(r"[Rr]ecommend(?:ed|ation)?.*?(?:request|req).*?0\s*m\b", assistant):
        # Check if this is in a "do not reduce" context
        if "do not" not in assistant.lower() and "keep" not in assistant.lower():
            errors.append("Recommendation of 0m CPU detected (safety violation)")

    return errors


def deduplicate_pairs(pairs: list[dict]) -> list[dict]:
    """Remove duplicate and near-duplicate pairs.

    Strategy:
    1. Exact dedup: Remove pairs with identical user+assistant text.
    2. ID dedup: Remove pairs with duplicate IDs (keep first seen).

    TODO: Add semantic dedup using sentence-transformers embeddings.
    Cluster pairs by cosine similarity and keep the highest-quality
    pair from each cluster (longest assistant, most concrete numbers).
    """
    seen_ids: set[str] = set()
    seen_content: set[str] = set()
    unique_pairs = []
    duplicates = 0

    for pair in pairs:
        pair_id = pair.get("id", "")

        # ID dedup.
        if pair_id in seen_ids:
            duplicates += 1
            continue
        seen_ids.add(pair_id)

        # Content dedup (hash of user + assistant).
        content_key = pair.get("user", "") + "|||" + pair.get("assistant", "")
        content_hash = hash(content_key)
        if content_hash in seen_content:
            duplicates += 1
            continue
        seen_content.add(content_hash)

        unique_pairs.append(pair)

    logger.info("Deduplication removed %d pairs (%d remaining)", duplicates, len(unique_pairs))
    return unique_pairs


def enforce_synthetic_cap(pairs: list[dict]) -> list[dict]:
    """Ensure synthetic pairs don't exceed MAX_SYNTHETIC_RATIO of the dataset.

    If synthetic pairs exceed the cap, randomly sample down to the limit.
    """
    synthetic = [p for p in pairs if p.get("source") == "synthetic"]
    non_synthetic = [p for p in pairs if p.get("source") != "synthetic"]

    max_synthetic = int(len(non_synthetic) * MAX_SYNTHETIC_RATIO / (1 - MAX_SYNTHETIC_RATIO))

    if len(synthetic) > max_synthetic:
        logger.warning(
            "Synthetic pairs (%d) exceed %.0f%% cap. Sampling down to %d.",
            len(synthetic),
            MAX_SYNTHETIC_RATIO * 100,
            max_synthetic,
        )
        # TODO: Use random.sample with a fixed seed for reproducibility.
        # import random
        # random.seed(42)
        # synthetic = random.sample(synthetic, max_synthetic)
        synthetic = synthetic[:max_synthetic]

    return non_synthetic + synthetic


def format_all(output_path: Path, project_root: Path) -> None:
    """Main formatting pipeline."""
    all_pairs: list[dict] = []

    for input_file in INPUT_FILES:
        path = project_root / input_file
        if not path.exists():
            logger.info("Skipping %s (not found)", input_file)
            continue

        logger.info("Reading %s", input_file)
        count = 0
        with open(path) as f:
            for line_num, line in enumerate(f, 1):
                line = line.strip()
                if not line:
                    continue
                try:
                    pair = json.loads(line)
                    all_pairs.append(pair)
                    count += 1
                except json.JSONDecodeError as e:
                    logger.error("Invalid JSON at %s:%d: %s", input_file, line_num, e)

        logger.info("  Loaded %d pairs", count)

    logger.info("Total pairs loaded: %d", len(all_pairs))

    # Validate.
    valid_pairs = []
    invalid_count = 0
    for i, pair in enumerate(all_pairs):
        errors = validate_pair(pair, i)
        if errors:
            logger.warning("Pair %s failed validation:", pair.get("id", f"index-{i}"))
            for error in errors:
                logger.warning("  - %s", error)
            invalid_count += 1
        else:
            valid_pairs.append(pair)

    logger.info("Validation: %d valid, %d invalid", len(valid_pairs), invalid_count)

    # Deduplicate.
    unique_pairs = deduplicate_pairs(valid_pairs)

    # Enforce synthetic cap.
    final_pairs = enforce_synthetic_cap(unique_pairs)

    # Source breakdown.
    source_counts: dict[str, int] = {}
    for pair in final_pairs:
        src = pair.get("source", "unknown")
        source_counts[src] = source_counts.get(src, 0) + 1

    logger.info("Final dataset: %d pairs", len(final_pairs))
    for src, count in sorted(source_counts.items()):
        logger.info("  %s: %d pairs", src, count)

    # Write output.
    output_path.parent.mkdir(parents=True, exist_ok=True)
    with open(output_path, "w") as f:
        for pair in final_pairs:
            f.write(json.dumps(pair, ensure_ascii=False) + "\n")

    logger.info("Wrote %d pairs to %s", len(final_pairs), output_path)

    # Write manifest.
    manifest_path = output_path.with_suffix(".manifest.json")
    manifest = {
        "total_pairs": len(final_pairs),
        "sources": source_counts,
        "invalid_rejected": invalid_count,
        "duplicates_removed": len(valid_pairs) - len(unique_pairs),
    }
    with open(manifest_path, "w") as f:
        json.dump(manifest, f, indent=2)

    logger.info("Wrote manifest to %s", manifest_path)


def main() -> None:
    parser = argparse.ArgumentParser(description="Format and validate training dataset")
    parser.add_argument(
        "--output",
        type=Path,
        default=Path("ml/dataset/data/training_data.jsonl"),
    )
    parser.add_argument(
        "--project-root",
        type=Path,
        default=Path("."),
        help="Project root directory",
    )
    parser.add_argument("--verbose", action="store_true")
    args = parser.parse_args()

    logging.basicConfig(
        level=logging.DEBUG if args.verbose else logging.INFO,
        format="%(asctime)s %(levelname)s %(message)s",
    )

    format_all(args.output, args.project_root)


if __name__ == "__main__":
    main()

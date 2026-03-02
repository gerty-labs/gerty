#!/usr/bin/env python3
"""Collect training pairs from Kubernetes official documentation.

Scrapes K8s docs sections related to resource management and transforms
them into instruction-tuning pairs for the k8s-sage SLM.

Target sections:
- Resource Management for Pods and Containers
- Quality of Service for Pods
- Vertical Pod Autoscaler
- Horizontal Pod Autoscaler
- LimitRange and ResourceQuota
- Node-pressure eviction
- Pod overhead and init containers

Also targets provider-specific docs:
- GKE: Autopilot resource recommendations, cost optimisation
- EKS: Right Sizing Recommendations, Compute Optimizer
- AKS: Resource recommendations, cluster advisor

Licensing: K8s docs are Apache 2.0. Provider docs are proprietary
but transformation into instruction pairs for model training is
considered fair use. Always cite the source URL in provenance.

Expected output: ~3,500 instruction pairs in JSONL format.

Usage:
    python collect_k8s_docs.py --output ml/dataset/data/k8s_docs.jsonl
"""

import argparse
import json
import logging
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Optional

# NOTE: Do not hardcode API keys or tokens. Use environment variables.
# import os
# GITHUB_TOKEN = os.environ.get("GITHUB_TOKEN")  # for raw content fetching

logger = logging.getLogger(__name__)

SYSTEM_PROMPT = (
    "You are k8s-sage, a Kubernetes resource efficiency specialist. "
    "Analyse the provided workload metrics and give actionable right-sizing "
    "recommendations. Be specific about numbers, explain your reasoning, "
    "and flag risks."
)

# K8s documentation URLs to scrape.
# These are the raw markdown sources from the kubernetes/website repo.
K8S_DOC_SECTIONS = [
    {
        "url": "https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/",
        "topic": "resource-management",
        "description": "Resource Management for Pods and Containers",
    },
    {
        "url": "https://kubernetes.io/docs/tasks/configure-pod-container/quality-service-pod/",
        "topic": "qos-classes",
        "description": "Quality of Service for Pods",
    },
    {
        "url": "https://kubernetes.io/docs/concepts/workloads/autoscaling/",
        "topic": "autoscaling",
        "description": "Autoscaling workloads (VPA, HPA)",
    },
    {
        "url": "https://kubernetes.io/docs/concepts/policy/limit-range/",
        "topic": "limit-range",
        "description": "LimitRange",
    },
    {
        "url": "https://kubernetes.io/docs/concepts/policy/resource-quotas/",
        "topic": "resource-quotas",
        "description": "ResourceQuota",
    },
    {
        "url": "https://kubernetes.io/docs/concepts/scheduling-eviction/node-pressure-eviction/",
        "topic": "eviction",
        "description": "Node-pressure eviction",
    },
]


@dataclass
class TrainingPair:
    """A single instruction-tuning pair."""

    id: str
    source: str = "k8s-docs"
    system: str = SYSTEM_PROMPT
    user: str = ""
    assistant: str = ""
    metadata: dict = field(default_factory=dict)

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "source": self.source,
            "system": self.system,
            "user": self.user,
            "assistant": self.assistant,
            "metadata": self.metadata,
        }

    def is_valid(self) -> bool:
        """Basic validation before writing."""
        if len(self.assistant) < 50:
            logger.warning("Pair %s: assistant response too short (%d chars)", self.id, len(self.assistant))
            return False
        if not self.metadata.get("provenance"):
            logger.warning("Pair %s: missing provenance", self.id)
            return False
        return True


def fetch_page(url: str, max_retries: int = 3, backoff_base: float = 2.0) -> Optional[str]:
    """Fetch a web page with retry and exponential backoff.

    Args:
        url: The URL to fetch.
        max_retries: Maximum number of retry attempts.
        backoff_base: Base for exponential backoff (seconds).

    Returns:
        Page content as string, or None on failure.
    """
    # TODO: Implement with requests or httpx.
    # Rate limiting: respect robots.txt and add delays between requests.
    #
    # import requests
    # for attempt in range(max_retries):
    #     try:
    #         resp = requests.get(url, timeout=30, headers={"User-Agent": "k8s-sage-data-collector/0.1"})
    #         resp.raise_for_status()
    #         return resp.text
    #     except requests.RequestException as e:
    #         wait = backoff_base ** attempt
    #         logger.warning("Fetch %s failed (attempt %d/%d): %s. Retrying in %.1fs",
    #                        url, attempt + 1, max_retries, e, wait)
    #         time.sleep(wait)
    # return None
    raise NotImplementedError("Scraping not yet implemented — scaffold only")


def extract_sections(html: str) -> list[dict]:
    """Parse HTML and extract relevant sections.

    Each section should contain:
    - title: Section heading
    - content: Section text (markdown preferred)
    - code_examples: Any YAML/JSON code blocks

    TODO: Use BeautifulSoup or markdownify to convert HTML to structured sections.
    """
    raise NotImplementedError("Section extraction not yet implemented — scaffold only")


def section_to_pairs(section: dict, topic: str, source_url: str) -> list[TrainingPair]:
    """Transform a documentation section into instruction-tuning pairs.

    Each section generates 3-5 pairs covering:
    1. Conceptual: "What does this feature do?"
    2. Applied: "Given these metrics, how would this feature apply?"
    3. Operational: "What are the gotchas/pitfalls?"

    Args:
        section: Parsed section with title, content, code_examples.
        topic: Topic category (e.g., "resource-management").
        source_url: URL for provenance tracking.

    Returns:
        List of TrainingPair objects.
    """
    pairs = []

    # TODO: Implement transformation logic.
    # For each section:
    #   1. Generate a conceptual question about the section content
    #   2. Create a realistic metrics scenario where this knowledge applies
    #   3. Write an operational gotchas question
    #
    # Mark pairs with low confidence as needs_review=True.
    #
    # Example:
    # pair = TrainingPair(
    #     id=f"k8s-docs-{topic}-{seq:03d}",
    #     user="What is the difference between CPU requests and limits in Kubernetes?",
    #     assistant="CPU requests and limits serve different purposes...",
    #     metadata={
    #         "category": "right-sizing",
    #         "provenance": source_url,
    #         "needs_review": True,  # Auto-generated, needs human review
    #     },
    # )
    # pairs.append(pair)

    raise NotImplementedError("Pair generation not yet implemented — scaffold only")
    return pairs


def collect_all(output_path: Path) -> None:
    """Main collection pipeline.

    1. Fetch each documentation page
    2. Extract sections
    3. Transform to instruction pairs
    4. Validate and write to JSONL
    """
    all_pairs: list[TrainingPair] = []

    for doc in K8S_DOC_SECTIONS:
        logger.info("Processing: %s", doc["description"])

        html = fetch_page(doc["url"])
        if html is None:
            logger.error("Failed to fetch %s, skipping", doc["url"])
            continue

        sections = extract_sections(html)
        for section in sections:
            pairs = section_to_pairs(section, doc["topic"], doc["url"])
            all_pairs.extend(pairs)

        # Rate limit: 2 seconds between pages.
        time.sleep(2.0)

    # Validate and write.
    valid_pairs = [p for p in all_pairs if p.is_valid()]
    logger.info("Generated %d valid pairs from %d total", len(valid_pairs), len(all_pairs))

    output_path.parent.mkdir(parents=True, exist_ok=True)
    with open(output_path, "w") as f:
        for pair in valid_pairs:
            f.write(json.dumps(pair.to_dict()) + "\n")

    logger.info("Wrote %d pairs to %s", len(valid_pairs), output_path)


def main() -> None:
    parser = argparse.ArgumentParser(description="Collect K8s docs training pairs")
    parser.add_argument(
        "--output",
        type=Path,
        default=Path("ml/dataset/data/k8s_docs.jsonl"),
        help="Output JSONL file path",
    )
    parser.add_argument("--verbose", action="store_true")
    args = parser.parse_args()

    logging.basicConfig(
        level=logging.DEBUG if args.verbose else logging.INFO,
        format="%(asctime)s %(levelname)s %(message)s",
    )

    collect_all(args.output)


if __name__ == "__main__":
    main()

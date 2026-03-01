#!/usr/bin/env python3
"""Collect training pairs from Stack Overflow K8s resource questions.

Uses the Stack Exchange API to find high-quality Q&A pairs about
Kubernetes resource management, OOMKill, CPU throttling, and right-sizing.

Licensing: Stack Overflow content is licensed under CC BY-SA 4.0.
Attribution is required. Each training pair must include the question
URL in its provenance field.

API rate limits: Stack Exchange API allows 300 requests per day without
a key, 10,000 per day with a registered app key. Always use a key.

Expected output: ~2,000 instruction pairs in JSONL format.

Usage:
    export SO_API_KEY=...
    python collect_so.py --output ml/dataset/data/so_pairs.jsonl
"""

import argparse
import json
import logging
import os
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Optional

logger = logging.getLogger(__name__)

# NOTE: Never hardcode API keys. Use environment variables.
# Register an app at https://stackapps.com/ to get an API key.

SYSTEM_PROMPT = (
    "You are k8s-sage, a Kubernetes resource efficiency specialist. "
    "Analyse the provided workload metrics and give actionable right-sizing "
    "recommendations. Be specific about numbers, explain your reasoning, "
    "and flag risks."
)

# Stack Exchange API configuration.
SE_API_BASE = "https://api.stackexchange.com/2.3"

# Tag combinations to search.
# Primary tag must be "kubernetes", combined with resource-related tags.
TAG_COMBINATIONS = [
    "kubernetes;memory",
    "kubernetes;cpu",
    "kubernetes;resources",
    "kubernetes;oom",
    "kubernetes;limits",
    "kubernetes;requests",
    "kubernetes;vertical-pod-autoscaler",
    "kubernetes;resource-management",
]

# Minimum quality thresholds.
MIN_QUESTION_SCORE = 5
MIN_ANSWER_SCORE = 3
REQUIRE_ACCEPTED_ANSWER = True


@dataclass
class TrainingPair:
    """A single instruction-tuning pair."""

    id: str
    source: str = "stackoverflow"
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
        if len(self.assistant) < 50:
            return False
        if not self.metadata.get("provenance"):
            return False
        return True


class StackExchangeClient:
    """Stack Exchange API client with rate limiting.

    Args:
        api_key: Stack Exchange API key for higher rate limits.
    """

    def __init__(self, api_key: Optional[str] = None):
        self.api_key = api_key
        self._request_count = 0
        self._daily_quota_remaining = 10000 if api_key else 300

    def search_questions(
        self,
        tags: str,
        min_score: int = 5,
        has_accepted: bool = True,
        page_size: int = 100,
        max_pages: int = 5,
    ) -> list[dict]:
        """Search for questions with specific tags and quality thresholds.

        Args:
            tags: Semicolon-separated tag string (e.g., "kubernetes;memory").
            min_score: Minimum question score.
            has_accepted: If True, only return questions with accepted answers.
            page_size: Results per page (max 100).
            max_pages: Maximum number of pages to fetch.

        Returns:
            List of question dicts from the Stack Exchange API.

        TODO: Implement with requests library.
        """
        # url = f"{SE_API_BASE}/questions"
        # params = {
        #     "order": "desc",
        #     "sort": "votes",
        #     "tagged": tags,
        #     "site": "stackoverflow",
        #     "filter": "withbody",  # Include question body
        #     "min": min_score,
        #     "pagesize": page_size,
        #     "page": 1,
        # }
        # if has_accepted:
        #     params["accepted"] = "True"
        # if self.api_key:
        #     params["key"] = self.api_key
        #
        # all_items = []
        # for page in range(1, max_pages + 1):
        #     params["page"] = page
        #     resp = requests.get(url, params=params, timeout=30)
        #     data = resp.json()
        #
        #     # Check quota.
        #     self._daily_quota_remaining = data.get("quota_remaining", 0)
        #     if self._daily_quota_remaining < 10:
        #         logger.warning("Stack Exchange quota nearly exhausted: %d remaining",
        #                        self._daily_quota_remaining)
        #         break
        #
        #     # Check backoff directive.
        #     if "backoff" in data:
        #         wait = data["backoff"]
        #         logger.info("Stack Exchange requested backoff: %d seconds", wait)
        #         time.sleep(wait)
        #
        #     all_items.extend(data.get("items", []))
        #
        #     if not data.get("has_more", False):
        #         break
        #
        #     time.sleep(1.0)  # Rate limiting between pages
        #
        # return all_items
        raise NotImplementedError("Stack Exchange API client not yet implemented — scaffold only")

    def get_answers(self, question_id: int) -> list[dict]:
        """Fetch answers for a specific question, sorted by score.

        Args:
            question_id: Stack Overflow question ID.

        Returns:
            List of answer dicts, sorted by score descending.

        TODO: Implement with requests library.
        """
        # url = f"{SE_API_BASE}/questions/{question_id}/answers"
        # params = {
        #     "order": "desc",
        #     "sort": "votes",
        #     "site": "stackoverflow",
        #     "filter": "withbody",
        #     "pagesize": 5,  # Top 5 answers only
        # }
        # if self.api_key:
        #     params["key"] = self.api_key
        #
        # resp = requests.get(url, params=params, timeout=30)
        # return resp.json().get("items", [])
        raise NotImplementedError("Stack Exchange API client not yet implemented — scaffold only")


def qa_to_pair(question: dict, answer: dict) -> Optional[TrainingPair]:
    """Transform a Stack Overflow Q&A into an instruction-tuning pair.

    The user message is derived from the question (reformatted for clarity).
    The assistant message is derived from the accepted/top answer.

    Args:
        question: Question dict with 'title', 'body', 'tags', 'score'.
        answer: Answer dict with 'body', 'score', 'is_accepted'.

    Returns:
        TrainingPair if quality thresholds are met, None otherwise.

    Quality filters:
    - Answer score >= MIN_ANSWER_SCORE
    - Answer body is substantive (>100 chars after HTML stripping)
    - Question is about resource management (not just general K8s)
    - Skip answers that recommend deprecated approaches
    - Skip answers about K8s versions < 1.20 (API changes)
    """
    # TODO: Implement transformation logic.
    #
    # 1. Strip HTML from question body and answer body.
    #    - Use BeautifulSoup: BeautifulSoup(body, "html.parser").get_text()
    #    - Preserve code blocks (they often contain kubectl output, YAML)
    #
    # 2. Reformat question as a clear problem statement.
    #    - Include relevant context (error messages, resource values)
    #    - Remove noise (greetings, "I'm a beginner", etc.)
    #
    # 3. Reformat answer as actionable advice.
    #    - Focus on the solution, not the preamble
    #    - Include concrete commands or YAML snippets where present
    #
    # 4. Classify:
    #    - Detect category from tags and content
    #    - Detect runtime from question content (JVM, Go, Python mentions)
    #
    # 5. Flag for review if answer quality is uncertain:
    #    metadata["needs_review"] = answer["score"] < 10
    #
    # question_url = f"https://stackoverflow.com/questions/{question['question_id']}"
    # pair = TrainingPair(
    #     id=f"stackoverflow-{question['question_id']}",
    #     user=formatted_question,
    #     assistant=formatted_answer,
    #     metadata={
    #         "category": detected_category,
    #         "provenance": question_url,
    #         "needs_review": answer["score"] < 10,
    #     },
    # )
    # return pair
    raise NotImplementedError("QA transformation not yet implemented — scaffold only")


def collect_all(output_path: Path) -> None:
    """Main collection pipeline."""
    api_key = os.environ.get("SO_API_KEY")
    if not api_key:
        logger.warning(
            "SO_API_KEY not set. Using unauthenticated requests "
            "(300 req/day limit). Register at stackapps.com for higher limits."
        )

    client = StackExchangeClient(api_key=api_key)
    all_pairs: list[TrainingPair] = []
    seen_question_ids: set[int] = set()

    for tags in TAG_COMBINATIONS:
        logger.info("Searching tags: %s", tags)

        try:
            questions = client.search_questions(
                tags=tags,
                min_score=MIN_QUESTION_SCORE,
                has_accepted=REQUIRE_ACCEPTED_ANSWER,
            )
        except NotImplementedError:
            logger.info("Scaffold only — skipping actual API calls")
            break

        for question in questions:
            qid = question["question_id"]
            if qid in seen_question_ids:
                continue
            seen_question_ids.add(qid)

            answers = client.get_answers(qid)
            if not answers:
                continue

            # Use accepted answer if available, otherwise highest-scored.
            best_answer = next(
                (a for a in answers if a.get("is_accepted")),
                answers[0],
            )

            if best_answer.get("score", 0) < MIN_ANSWER_SCORE:
                continue

            pair = qa_to_pair(question, best_answer)
            if pair is not None:
                all_pairs.append(pair)

            # Rate limit: 0.5 seconds between questions.
            time.sleep(0.5)

        # Rate limit: 2 seconds between tag searches.
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
    parser = argparse.ArgumentParser(description="Collect Stack Overflow training pairs")
    parser.add_argument(
        "--output",
        type=Path,
        default=Path("ml/dataset/data/so_pairs.jsonl"),
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

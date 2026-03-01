#!/usr/bin/env python3
"""Collect training pairs from Kubernetes-related GitHub issues.

Uses the GitHub API to find issues related to resource management,
OOMKill, CPU throttling, and right-sizing from key repositories.

Target repositories:
- kubernetes/kubernetes (core resource issues)
- kubernetes/autoscaler (VPA/HPA issues)
- Popular operator repos with resource-related issues

Licensing: GitHub issues are public, but check each repo's licence
before including content. Issues themselves are user-contributed and
may not have explicit licensing. Transformation into instruction pairs
for model training is considered fair use for public discussions.

API rate limits: GitHub API allows 5,000 requests/hour with auth,
60/hour without. Always use authenticated requests and implement
backoff.

Expected output: ~3,000 instruction pairs in JSONL format.

Usage:
    export GITHUB_TOKEN=ghp_...
    python collect_gh_issues.py --output ml/dataset/data/gh_issues.jsonl
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

# NOTE: Never hardcode tokens. Always use environment variables.
# The GITHUB_TOKEN env var should be a personal access token with
# public_repo scope (read-only access to public repositories).

SYSTEM_PROMPT = (
    "You are k8s-sage, a Kubernetes resource efficiency specialist. "
    "Analyse the provided workload metrics and give actionable right-sizing "
    "recommendations. Be specific about numbers, explain your reasoning, "
    "and flag risks."
)

# Repositories and search queries.
TARGET_REPOS = [
    "kubernetes/kubernetes",
    "kubernetes/autoscaler",
    "FairwindsOps/goldilocks",
    "kubecost/cost-analyzer-helm-chart",
]

# Labels that indicate resource-related issues.
RESOURCE_LABELS = [
    "kind/bug",
    "sig/node",
    "area/resource-management",
]

# Search keywords for issue content.
SEARCH_KEYWORDS = [
    "OOMKill",
    "OOMKilled",
    "cpu throttl",
    "resource request",
    "resource limit",
    "right-siz",
    "over-provision",
    "under-provision",
    "memory leak kubernetes",
    "VPA recommendation",
]


@dataclass
class TrainingPair:
    """A single instruction-tuning pair."""

    id: str
    source: str = "github"
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


class GitHubClient:
    """GitHub API client with rate limiting and backoff.

    Args:
        token: GitHub personal access token. If None, uses unauthenticated
               requests (60 req/hour limit).
    """

    BASE_URL = "https://api.github.com"

    def __init__(self, token: Optional[str] = None):
        self.token = token
        self.session = None  # TODO: Use requests.Session for connection pooling
        self._request_count = 0
        self._rate_limit_remaining = 5000

    def _headers(self) -> dict:
        headers = {
            "Accept": "application/vnd.github.v3+json",
            "User-Agent": "k8s-sage-data-collector/0.1",
        }
        if self.token:
            headers["Authorization"] = f"Bearer {self.token}"
        return headers

    def search_issues(
        self,
        repo: str,
        query: str,
        max_results: int = 100,
    ) -> list[dict]:
        """Search for issues matching a query in a repository.

        Args:
            repo: Repository in "owner/name" format.
            query: Search query string.
            max_results: Maximum number of results to return.

        Returns:
            List of issue dicts from the GitHub API.

        TODO: Implement with requests library.
        """
        # url = f"{self.BASE_URL}/search/issues"
        # params = {
        #     "q": f"{query} repo:{repo} is:issue is:closed",
        #     "sort": "reactions",
        #     "order": "desc",
        #     "per_page": min(max_results, 100),
        # }
        #
        # Pagination: follow Link headers for multi-page results.
        # Rate limiting: check X-RateLimit-Remaining header.
        # Backoff: if rate limited, wait until X-RateLimit-Reset timestamp.
        #
        # resp = self.session.get(url, params=params, headers=self._headers())
        # self._rate_limit_remaining = int(resp.headers.get("X-RateLimit-Remaining", 0))
        # if self._rate_limit_remaining < 10:
        #     reset_time = int(resp.headers.get("X-RateLimit-Reset", 0))
        #     wait = max(reset_time - time.time(), 0) + 1
        #     logger.warning("Rate limit near, sleeping %.0f seconds", wait)
        #     time.sleep(wait)
        #
        # return resp.json().get("items", [])
        raise NotImplementedError("GitHub API client not yet implemented — scaffold only")

    def get_issue_with_comments(self, repo: str, issue_number: int) -> Optional[dict]:
        """Fetch a single issue with its top comments.

        Args:
            repo: Repository in "owner/name" format.
            issue_number: Issue number.

        Returns:
            Dict with issue body and top comments, or None on failure.

        TODO: Implement with requests library.
        """
        # Fetch issue: GET /repos/{repo}/issues/{number}
        # Fetch comments: GET /repos/{repo}/issues/{number}/comments?per_page=10&sort=reactions
        # Filter: only comments with reactions > 0 or from maintainers
        raise NotImplementedError("GitHub API client not yet implemented — scaffold only")


def issue_to_pair(issue: dict, comments: list[dict], repo: str) -> Optional[TrainingPair]:
    """Transform a GitHub issue + comments into an instruction-tuning pair.

    The user message describes the problem (from the issue body).
    The assistant message provides the solution (from the top comment/resolution).

    Args:
        issue: GitHub issue dict (title, body, labels).
        comments: List of comment dicts (body, reactions).
        repo: Repository name for provenance.

    Returns:
        TrainingPair if successfully transformed, None if the issue
        doesn't contain enough information for a useful pair.

    Quality filters:
    - Issue must have a clear problem description
    - Resolution must contain actionable advice (not just "fixed in v1.X")
    - Skip issues that are purely about API changes or test failures
    """
    # TODO: Implement transformation logic.
    #
    # 1. Extract the problem description from issue body.
    #    - Look for error messages, kubectl output, metric values.
    #    - Ignore issues that are feature requests without solutions.
    #
    # 2. Extract the solution from the best comment or issue resolution.
    #    - Prefer comments with highest reaction count.
    #    - Prefer comments from maintainers (check author_association).
    #
    # 3. Reformat into instruction pair:
    #    - user: "I'm seeing [problem]. My workload has [context]."
    #    - assistant: "The issue is [root cause]. To fix: [steps]."
    #
    # 4. Classify the pair:
    #    - category: right-sizing, classification, runtime-specific, edge-case
    #    - pattern: steady, burstable, batch, idle (if applicable)
    #
    # 5. Flag for review if extraction confidence is low:
    #    metadata["needs_review"] = True
    #
    # issue_url = f"https://github.com/{repo}/issues/{issue['number']}"
    # pair = TrainingPair(
    #     id=f"github-{repo.replace('/', '-')}-{issue['number']}",
    #     user=formatted_problem,
    #     assistant=formatted_solution,
    #     metadata={
    #         "category": detected_category,
    #         "provenance": issue_url,
    #         "needs_review": True,
    #     },
    # )
    # return pair
    raise NotImplementedError("Issue transformation not yet implemented — scaffold only")


def collect_all(output_path: Path) -> None:
    """Main collection pipeline."""
    token = os.environ.get("GITHUB_TOKEN")
    if not token:
        logger.warning(
            "GITHUB_TOKEN not set. Using unauthenticated requests "
            "(60 req/hour limit). Set GITHUB_TOKEN for 5000 req/hour."
        )

    client = GitHubClient(token=token)
    all_pairs: list[TrainingPair] = []

    for repo in TARGET_REPOS:
        logger.info("Searching %s", repo)

        for keyword in SEARCH_KEYWORDS:
            logger.debug("  Query: %s", keyword)

            try:
                issues = client.search_issues(repo, keyword, max_results=50)
            except NotImplementedError:
                logger.info("Scaffold only — skipping actual API calls")
                break

            for issue in issues:
                issue_detail = client.get_issue_with_comments(repo, issue["number"])
                if issue_detail is None:
                    continue

                pair = issue_to_pair(
                    issue_detail["issue"],
                    issue_detail["comments"],
                    repo,
                )
                if pair is not None:
                    all_pairs.append(pair)

            # Rate limit: 1 second between searches.
            time.sleep(1.0)

    # Validate and write.
    valid_pairs = [p for p in all_pairs if p.is_valid()]
    logger.info("Generated %d valid pairs from %d total", len(valid_pairs), len(all_pairs))

    output_path.parent.mkdir(parents=True, exist_ok=True)
    with open(output_path, "w") as f:
        for pair in valid_pairs:
            f.write(json.dumps(pair.to_dict()) + "\n")

    logger.info("Wrote %d pairs to %s", len(valid_pairs), output_path)


def main() -> None:
    parser = argparse.ArgumentParser(description="Collect GitHub issues training pairs")
    parser.add_argument(
        "--output",
        type=Path,
        default=Path("ml/dataset/data/gh_issues.jsonl"),
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

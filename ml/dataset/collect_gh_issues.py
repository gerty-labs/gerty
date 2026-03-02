#!/usr/bin/env python3
"""Collect training pairs from Kubernetes-related GitHub issues.

Uses the GitHub API (via urllib — no external dependencies) to find
issues related to resource management, OOMKill, CPU throttling, and
right-sizing from key Kubernetes repositories.

Licensing: GitHub issues are public user-contributed content.
Transformation into instruction pairs is considered fair use.
Each pair includes the issue URL as provenance.

API rate limits: 5,000 requests/hour with auth, 60/hour without.
This script uses ~300-600 API calls per run.

Usage:
    export GITHUB_TOKEN=ghp_...
    python collect_gh_issues.py [--output ml/dataset/raw/github_issues_pairs.jsonl] [--verbose]
"""

import argparse
import hashlib
import json
import logging
import os
import re
import ssl
import time
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass, field
from pathlib import Path
from typing import Optional

logger = logging.getLogger(__name__)

SYSTEM_PROMPT = (
    "You are k8s-sage, a Kubernetes resource efficiency specialist. "
    "Analyse the provided workload metrics and give actionable right-sizing "
    "recommendations. Be specific about numbers, explain your reasoning, "
    "and flag risks."
)

# Output goes to raw/ since it needs human review.
DEFAULT_OUTPUT = Path("ml/dataset/raw/github_issues_pairs.jsonl")

# Repositories to search.
TARGET_REPOS = [
    "kubernetes/kubernetes",
    "kubernetes/autoscaler",
    "FairwindsOps/goldilocks",
    "kubernetes-sigs/descheduler",
]

# Search queries — each combined with repo and is:issue is:closed.
SEARCH_QUERIES = [
    "OOMKill",
    "OOMKilled",
    "cpu throttling",
    "resource requests limits",
    "right-sizing resources",
    "over-provisioned",
    "under-provisioned",
    "memory leak pod",
    "VPA recommendation",
    "vertical pod autoscaler",
    "CPU limit throttle",
    "resource quota exceeded",
    "LimitRange",
    "container memory",
    "HPA scaling",
]

# Keywords that must appear in the resolution for quality.
RESOURCE_KEYWORDS = {
    "memory", "cpu", "request", "limit", "oom", "throttle", "throttling",
    "vpa", "hpa", "resource", "millicores", "mib", "gib", "mi", "gi",
    "container", "pod", "node", "autoscal", "right-siz", "provision",
    "evict", "qos", "burstable", "guaranteed", "besteffort",
}

# Words that indicate the issue is NOT useful for training.
SKIP_PATTERNS = [
    r"^(test|e2e|ci|flake)\b",
    r"\btest failure\b",
    r"\brelease note\b",
    r"\bapi deprecat",
    r"\bvendor\b.*\bupdate\b",
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
        if len(self.assistant) < 100:
            return False
        if len(self.user) < 30:
            return False
        if not self.metadata.get("provenance"):
            return False
        return True


class GitHubClient:
    """GitHub API client with rate limiting and backoff.

    Uses urllib.request (stdlib) to avoid external dependencies.
    """

    BASE_URL = "https://api.github.com"

    def __init__(self, token: Optional[str] = None):
        self.token = token
        self._request_count = 0
        self._rate_limit_remaining = 5000 if token else 60
        self._search_rate_remaining = 30 if token else 10
        # Create SSL context for HTTPS
        self._ssl_ctx = ssl.create_default_context()

    def _headers(self) -> dict:
        headers = {
            "Accept": "application/vnd.github.v3+json",
            "User-Agent": "k8s-sage-data-collector/0.1",
        }
        if self.token:
            # Support both classic (ghp_) and fine-grained (github_pat_) tokens.
            headers["Authorization"] = f"Bearer {self.token}"
        return headers

    def _request(
        self, url: str, params: Optional[dict] = None, _retry: int = 0
    ) -> Optional[dict]:
        """Make an authenticated GET request with rate limiting."""
        if params:
            url = f"{url}?{urllib.parse.urlencode(params)}"

        req = urllib.request.Request(url, headers=self._headers())
        self._request_count += 1

        is_search = "/search/" in url

        try:
            with urllib.request.urlopen(req, context=self._ssl_ctx, timeout=30) as resp:  # nosemgrep: dynamic-urllib-use-detected — URL from GitHub API, not user input
                # Update rate limit tracking (search and core APIs have separate limits).
                remaining = int(
                    resp.headers.get("X-RateLimit-Remaining", 999)
                )
                if is_search:
                    self._search_rate_remaining = remaining
                    threshold = 5  # Search API: 30/min, sleep only when very low
                else:
                    self._rate_limit_remaining = remaining
                    threshold = 100  # Core API: 5000/hr, sleep earlier

                if remaining < threshold:
                    reset_time = int(resp.headers.get("X-RateLimit-Reset", 0))
                    wait = min(max(reset_time - time.time(), 0) + 2, 120)
                    logger.warning(
                        "Rate limit low (%d remaining, %s), sleeping %.0f seconds",
                        remaining,
                        "search" if is_search else "core",
                        wait,
                    )
                    time.sleep(wait)

                return json.loads(resp.read().decode("utf-8"))

        except urllib.error.HTTPError as e:
            if e.code == 403 and _retry < 2:
                # Rate limited — wait and retry (max 2 retries).
                reset_time = int(e.headers.get("X-RateLimit-Reset", 0))
                wait = min(max(reset_time - time.time(), 0) + 5, 120)
                logger.warning(
                    "Rate limited (403, retry %d). Sleeping %.0f seconds",
                    _retry + 1, wait,
                )
                time.sleep(wait)
                return self._request(url, _retry=_retry + 1)
            elif e.code == 422:
                logger.debug("Unprocessable query (422) for %s", url)
                return None
            else:
                logger.error("HTTP %d for %s: %s", e.code, url, e.read().decode())
                return None
        except (urllib.error.URLError, TimeoutError) as e:
            logger.error("Request failed for %s: %s", url, e)
            return None

    def search_issues(
        self,
        repo: str,
        query: str,
        max_results: int = 30,
    ) -> list[dict]:
        """Search for closed issues matching a query in a repository."""
        url = f"{self.BASE_URL}/search/issues"
        params = {
            "q": f"{query} repo:{repo} is:issue is:closed",
            "sort": "reactions",
            "order": "desc",
            "per_page": min(max_results, 100),
        }

        data = self._request(url, params)
        if data is None:
            return []

        items = data.get("items", [])
        logger.debug(
            "  %s '%s': %d results (total_count=%d)",
            repo, query, len(items), data.get("total_count", 0),
        )
        return items

    def get_issue_with_comments(
        self, repo: str, issue_number: int
    ) -> Optional[dict]:
        """Fetch a single issue with its comments."""
        # Fetch issue.
        issue_url = f"{self.BASE_URL}/repos/{repo}/issues/{issue_number}"
        issue_data = self._request(issue_url)
        if issue_data is None:
            return None

        # Fetch comments (up to 20, sorted by created).
        comments_url = f"{self.BASE_URL}/repos/{repo}/issues/{issue_number}/comments"
        comments_data = self._request(comments_url, {"per_page": 20})
        if comments_data is None:
            comments_data = []

        return {
            "issue": issue_data,
            "comments": comments_data,
        }


def _classify_category(text: str) -> str:
    """Classify issue text into a training category."""
    text_lower = text.lower()

    # Check for runtime-specific patterns.
    runtime_keywords = [
        "jvm", "java", "golang", " go ", "python", "node.js", "nodejs",
        ".net", "dotnet", "ruby", "php", "rust", "erlang", "scala",
    ]
    if any(kw in text_lower for kw in runtime_keywords):
        return "runtime-specific"

    # Check for edge cases.
    edge_keywords = [
        "oomkill", "oomkilled", "crash", "crashloop", "evict",
        "pending", "stuck", "not working", "debug", "troubleshoot",
        "unexpected", "weird", "strange", "bug", "regression",
    ]
    if any(kw in text_lower for kw in edge_keywords):
        return "edge-case"

    # Check for classification topics.
    class_keywords = [
        "classify", "pattern", "steady", "burstable", "batch", "idle",
        "qos", "workload type", "best effort", "guaranteed",
    ]
    if any(kw in text_lower for kw in class_keywords):
        return "classification"

    # Default to right-sizing.
    return "right-sizing"


def _has_resource_content(text: str) -> bool:
    """Check if text contains resource-management keywords."""
    text_lower = text.lower()
    matches = sum(1 for kw in RESOURCE_KEYWORDS if kw in text_lower)
    return matches >= 2  # Need at least 2 resource keywords.


def _should_skip(title: str) -> bool:
    """Check if issue title indicates non-useful content."""
    title_lower = title.lower()
    return any(re.search(pat, title_lower) for pat in SKIP_PATTERNS)


def _clean_body(body: str, max_len: int = 2000) -> str:
    """Clean and truncate issue/comment body."""
    if not body:
        return ""
    # Remove HTML comments.
    body = re.sub(r"<!--.*?-->", "", body, flags=re.DOTALL)
    # Remove image links.
    body = re.sub(r"!\[.*?\]\(.*?\)", "[image]", body)
    # Remove very long code blocks (>500 chars).
    body = re.sub(r"```[\s\S]{500,}?```", "[long code block omitted]", body)
    # Collapse multiple newlines.
    body = re.sub(r"\n{3,}", "\n\n", body)
    # Truncate.
    if len(body) > max_len:
        body = body[:max_len] + "\n[truncated]"
    return body.strip()


def _score_comment(comment: dict) -> float:
    """Score a comment by quality signals."""
    score = 0.0
    reactions = comment.get("reactions", {})

    # Positive reactions.
    score += reactions.get("+1", 0) * 2
    score += reactions.get("heart", 0) * 2
    score += reactions.get("hooray", 0) * 1
    score += reactions.get("laugh", 0) * 0.5

    # Maintainer/member bonus.
    association = comment.get("author_association", "NONE")
    if association in ("MEMBER", "OWNER", "COLLABORATOR"):
        score += 10
    elif association == "CONTRIBUTOR":
        score += 5

    # Length bonus (longer = more detailed).
    body_len = len(comment.get("body", ""))
    if body_len > 200:
        score += 3
    if body_len > 500:
        score += 3

    # Resource keyword bonus.
    if _has_resource_content(comment.get("body", "")):
        score += 5

    return score


def issue_to_pair(
    issue: dict, comments: list[dict], repo: str
) -> Optional[TrainingPair]:
    """Transform a GitHub issue + comments into an instruction-tuning pair."""
    title = issue.get("title", "")
    body = issue.get("body", "")
    number = issue.get("number", 0)
    issue_url = f"https://github.com/{repo}/issues/{number}"

    # Skip non-useful issues.
    if _should_skip(title):
        logger.debug("Skipping (title filter): %s", title)
        return None

    if not body or len(body) < 50:
        logger.debug("Skipping (short body): %s", title)
        return None

    # Need at least 1 comment with substance.
    if len(comments) < 1:
        logger.debug("Skipping (no comments): %s", title)
        return None

    # Find best comment(s) as the "answer".
    scored = [(c, _score_comment(c)) for c in comments]
    scored.sort(key=lambda x: x[1], reverse=True)

    # Take top comment(s) with score > 0 and body > 100 chars.
    best_comments = [
        c for c, s in scored
        if s > 0 and len(c.get("body", "")) > 100
    ]

    if not best_comments:
        # Fallback: take the comment with most text that has resource keywords.
        for c, _ in scored:
            if len(c.get("body", "")) > 100 and _has_resource_content(c.get("body", "")):
                best_comments = [c]
                break

    if not best_comments:
        logger.debug("Skipping (no quality comments): %s", title)
        return None

    # Build user message from issue body.
    clean_body = _clean_body(body, max_len=1500)

    # Check resource relevance.
    combined_text = title + " " + clean_body
    if not _has_resource_content(combined_text):
        logger.debug("Skipping (not resource-related): %s", title)
        return None

    user_msg = f"## {title}\n\n{clean_body}"

    # Build assistant message from best comment(s).
    answer_parts = []
    for comment in best_comments[:3]:  # Max 3 comments.
        clean_comment = _clean_body(comment.get("body", ""), max_len=1200)
        if clean_comment:
            answer_parts.append(clean_comment)

    assistant_msg = "\n\n---\n\n".join(answer_parts)

    if len(assistant_msg) < 100:
        logger.debug("Skipping (short answer): %s", title)
        return None

    # Classify.
    category = _classify_category(combined_text + " " + assistant_msg)

    # Build labels from issue labels.
    labels = [
        label.get("name", "") for label in issue.get("labels", [])
    ]

    pair_id = f"github-{repo.replace('/', '-')}-{number}"

    return TrainingPair(
        id=pair_id,
        user=user_msg,
        assistant=assistant_msg,
        metadata={
            "category": category,
            "provenance": issue_url,
            "needs_review": True,
            "labels": labels,
            "comment_count": len(comments),
        },
    )


def collect_all(output_path: Path, verbose: bool = False) -> None:
    """Main collection pipeline."""
    token = os.environ.get("GITHUB_TOKEN")
    if not token:
        logger.warning(
            "GITHUB_TOKEN not set. Using unauthenticated requests "
            "(60 req/hour limit). Set GITHUB_TOKEN for 5000 req/hour."
        )

    client = GitHubClient(token=token)
    all_pairs: list[TrainingPair] = []
    seen_issue_ids: set[str] = set()  # Deduplicate across queries.

    for repo_idx, repo in enumerate(TARGET_REPOS):
        print(f"\n=== [{repo_idx+1}/{len(TARGET_REPOS)}] Searching {repo} ===", flush=True)
        repo_pairs = 0

        for q_idx, query in enumerate(SEARCH_QUERIES):
            print(f"  [{q_idx+1}/{len(SEARCH_QUERIES)}] Query: '{query}'", end="", flush=True)

            issues = client.search_issues(repo, query, max_results=30)
            new_issues = 0

            for issue in issues:
                number = issue.get("number", 0)
                pair_id = f"github-{repo.replace('/', '-')}-{number}"

                # Deduplicate.
                if pair_id in seen_issue_ids:
                    continue
                seen_issue_ids.add(pair_id)
                new_issues += 1

                # Fetch full issue + comments.
                detail = client.get_issue_with_comments(repo, number)
                if detail is None:
                    continue

                pair = issue_to_pair(detail["issue"], detail["comments"], repo)
                if pair is not None and pair.is_valid():
                    all_pairs.append(pair)
                    repo_pairs += 1

                # Small delay between issue fetches.
                time.sleep(0.3)

            print(f" -> {len(issues)} results, {new_issues} new, {repo_pairs} pairs so far", flush=True)

            # Rate limit between search queries.
            time.sleep(1.0)

        print(f"  {repo}: {repo_pairs} pairs collected (total: {len(all_pairs)})", flush=True)

    # Final dedup by content hash (different issues, same content).
    unique_pairs = []
    content_hashes: set[str] = set()
    for pair in all_pairs:
        h = hashlib.md5(  # noqa: S324 — used for dedup, not security
            (pair.user + pair.assistant).encode(),
            usedforsecurity=False,
        ).hexdigest()
        if h not in content_hashes:
            content_hashes.add(h)
            unique_pairs.append(pair)
        else:
            logger.debug("Removed duplicate content: %s", pair.id)

    logger.info(
        "Total: %d pairs (%d before dedup). API calls: %d",
        len(unique_pairs), len(all_pairs), client._request_count,
    )

    # Write output.
    output_path.parent.mkdir(parents=True, exist_ok=True)
    with open(output_path, "w") as f:
        for pair in unique_pairs:
            f.write(json.dumps(pair.to_dict()) + "\n")

    logger.info("Wrote %d pairs to %s", len(unique_pairs), output_path)

    # Print category breakdown.
    cats: dict[str, int] = {}
    for pair in unique_pairs:
        c = pair.metadata.get("category", "unknown")
        cats[c] = cats.get(c, 0) + 1
    print(f"\nCollected {len(unique_pairs)} pairs from GitHub issues")
    print(f"API calls made: {client._request_count}")
    print(f"Rate limit remaining: {client._rate_limit_remaining}")
    print("Category breakdown:")
    for cat, count in sorted(cats.items()):
        print(f"  {cat}: {count}")


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Collect GitHub issues training pairs for k8s-sage"
    )
    parser.add_argument(
        "--output",
        type=Path,
        default=DEFAULT_OUTPUT,
        help="Output JSONL file path",
    )
    parser.add_argument(
        "--verbose", "-v",
        action="store_true",
        help="Enable debug logging",
    )
    args = parser.parse_args()

    logging.basicConfig(
        level=logging.DEBUG if args.verbose else logging.INFO,
        format="%(asctime)s %(levelname)s %(message)s",
    )

    collect_all(args.output, verbose=args.verbose)


if __name__ == "__main__":
    main()

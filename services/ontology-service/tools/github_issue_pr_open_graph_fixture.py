#!/usr/bin/env python3
"""Capture a GitHub issue/PR pair as an OpenGraph fixture."""

from __future__ import annotations

import argparse
import json
import re
import subprocess
from pathlib import Path
from typing import Any


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Build an OpenGraph fixture from a GitHub issue and pull request.")
    parser.add_argument("--repo", required=True, help="GitHub repository, for example cli/cli.")
    parser.add_argument("--issue", type=int, required=True)
    parser.add_argument("--pr", type=int, required=True)
    parser.add_argument("--fixture-json", type=Path, required=True)
    parser.add_argument("--source-authority-json", type=Path, required=True)
    parser.add_argument("--golden-json", type=Path, required=True)
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> None:
    args = parse_args(argv)
    issue = gh_api(f"repos/{args.repo}/issues/{args.issue}")
    pull = gh_api(f"repos/{args.repo}/pulls/{args.pr}")
    fixture = build_fixture(args.repo, issue, pull)
    source_authority = build_source_authority()
    golden = build_golden(args.repo, issue, pull, fixture)
    write_json(args.fixture_json, fixture)
    write_json(args.source_authority_json, source_authority)
    write_json(args.golden_json, golden)


def gh_api(path: str) -> dict[str, Any]:
    completed = subprocess.run(
        ["gh", "api", path],
        check=True,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    payload = json.loads(completed.stdout)
    if not isinstance(payload, dict):
        raise ValueError(f"GitHub API {path} did not return an object")
    return payload


def build_fixture(repo: str, issue: dict[str, Any], pull: dict[str, Any]) -> dict[str, Any]:
    issue_number = int(issue["number"])
    pr_number = int(pull["number"])
    source_instance = f"github.com/{repo}"
    observed_at = latest_timestamp(issue, pull)
    issue_user = user_login(issue)
    pr_user = user_login(pull)
    users = sorted({value for value in [issue_user, pr_user] if value})
    objects: list[dict[str, Any]] = [
        {
            "objectType": "github_issue",
            "key": issue_key(repo, issue_number),
            "title": clean_text(issue.get("title")) or f"{repo} issue #{issue_number}",
            "sourceSystem": "github",
            "sourceInstance": source_instance,
            "externalKind": "issue",
            "externalID": str(issue_number),
            "sourceURL": str(issue.get("html_url") or ""),
            "rankScore": 90,
            "observedAt": observed_at,
        },
        {
            "objectType": "github_pull_request",
            "key": pr_key(repo, pr_number),
            "title": clean_text(pull.get("title")) or f"{repo} pull request #{pr_number}",
            "sourceSystem": "github",
            "sourceInstance": source_instance,
            "externalKind": "pull_request",
            "externalID": str(pr_number),
            "sourceURL": str(pull.get("html_url") or ""),
            "rankScore": 80,
            "observedAt": observed_at,
        },
    ]
    for index, login in enumerate(users):
        objects.append(
            {
                "objectType": "github_user",
                "key": user_key(login),
                "title": login,
                "sourceSystem": "github",
                "sourceInstance": "github.com",
                "externalKind": "user",
                "externalID": login,
                "sourceURL": f"https://github.com/{login}",
                "rankScore": 70 - index,
                "observedAt": observed_at,
            }
        )

    associations: list[dict[str, Any]] = [
        {
            "from": {"objectType": "github_issue", "key": issue_key(repo, issue_number)},
            "to": {"objectType": "github_pull_request", "key": pr_key(repo, pr_number)},
            "associationType": "closed_by",
            "sourceSystem": "github",
            "sourceInstance": source_instance,
            "externalKind": "closing_pull_request",
            "externalID": f"{issue_number}->{pr_number}",
            "sourceURL": str(pull.get("html_url") or ""),
            "locatorKind": "closing_pull_request",
            "locator": closing_locator(repo, issue_number, pr_number, pull),
            "rankScore": 20,
            "observedAt": observed_at,
        }
    ]
    if issue_user:
        associations.append(
            {
                "from": {"objectType": "github_issue", "key": issue_key(repo, issue_number)},
                "to": {"objectType": "github_user", "key": user_key(issue_user)},
                "associationType": "opened_by",
                "sourceSystem": "github",
                "sourceInstance": source_instance,
                "externalKind": "issue_author",
                "externalID": f"issue:{issue_number}:author:{issue_user}",
                "sourceURL": str(issue.get("html_url") or ""),
                "locatorKind": "issue_author",
                "locator": issue_user,
                "rankScore": 10,
                "observedAt": observed_at,
            }
        )
    if pr_user:
        associations.append(
            {
                "from": {"objectType": "github_pull_request", "key": pr_key(repo, pr_number)},
                "to": {"objectType": "github_user", "key": user_key(pr_user)},
                "associationType": "authored_by",
                "sourceSystem": "github",
                "sourceInstance": source_instance,
                "externalKind": "pull_request_author",
                "externalID": f"pr:{pr_number}:author:{pr_user}",
                "sourceURL": str(pull.get("html_url") or ""),
                "locatorKind": "pull_request_author",
                "locator": pr_user,
                "rankScore": 9,
                "observedAt": observed_at,
            }
        )

    return {
        "sourceInstance": source_instance,
        "observedAt": observed_at,
        "objects": objects,
        "associations": associations,
    }


def build_source_authority() -> dict[str, Any]:
    return {
        "relationship_authority": {
            "closed_by": {
                "presence_sources": ["github"],
                "presence_locator_kinds": {"github": ["closing_pull_request"]},
            },
            "opened_by": {
                "presence_sources": ["github"],
                "presence_locator_kinds": {"github": ["issue_author"]},
            },
            "authored_by": {
                "presence_sources": ["github"],
                "presence_locator_kinds": {"github": ["pull_request_author"]},
            },
        }
    }


def build_golden(repo: str, issue: dict[str, Any], pull: dict[str, Any], fixture: dict[str, Any]) -> dict[str, Any]:
    issue_number = int(issue["number"])
    pr_number = int(pull["number"])
    issue_ref = issue_key(repo, issue_number)
    pr_ref = pr_key(repo, pr_number)
    object_count = len(fixture["objects"])
    association_count = len(fixture["associations"])
    forbidden_common = [
        "WorkProgram",
        "TPM",
        "Flink",
        "Jira",
        "TicketLensResult",
        "PullRequestLensResult",
        "[analytics:",
        "implemented_by",
    ]
    return {
        "name": f"github-open-graph-{repo.replace('/', '-')}-{issue_number}-{pr_number}",
        "required_categories": [
            "generic-no-product-vocabulary",
            "github-author",
            "issue-pr",
            "open-ent-proof",
            "sparse-coverage",
            "traversal-shape",
        ],
        "required_source_coverage_states": ["sparse"],
        "questions": [
            {
                "key": "github:traversal-shape",
                "category": "traversal-shape",
                "source_coverage_state": "sparse",
                "question": "Does the answer describe the bounded GitHub open graph traversal shape?",
                "expected_facts": [
                    {
                        "text": f"{object_count} object(s) and {association_count} association(s)",
                        "citation_prefix": "[context:",
                    }
                ],
                "forbidden_phrases": forbidden_common,
            },
            {
                "key": "github:issue-pr",
                "category": "issue-pr",
                "source_coverage_state": "sparse",
                "question": "Does the answer cite the GitHub issue to pull request relationship?",
                "expected_facts": [
                    {"text": f"{issue_ref}` -> `{pr_ref}", "citation_prefix": "[graph_associations:"},
                    {"text": "closed_by", "citation_prefix": "[graph_associations:"},
                ],
                "forbidden_phrases": [*forbidden_common, "no linked pull request"],
            },
            {
                "key": "github:author",
                "category": "github-author",
                "source_coverage_state": "sparse",
                "question": "Does the answer cite at least one GitHub author relationship?",
                "expected_facts": [
                    {"text": "authored_by", "citation_prefix": "[graph_associations:"},
                ],
                "forbidden_phrases": forbidden_common,
            },
            {
                "key": "github:ent-proof",
                "category": "open-ent-proof",
                "source_coverage_state": "sparse",
                "question": "Does the answer stay on the generic open graph instead of typed product ontology rows?",
                "expected_facts": [
                    {"text": issue_ref, "citation_prefix": "[graph_objects:"},
                    {"text": pr_ref, "citation_prefix": "[graph_objects:"},
                ],
                "forbidden_phrases": forbidden_common,
            },
            {
                "key": "github:sparse-coverage",
                "category": "sparse-coverage",
                "source_coverage_state": "sparse",
                "question": "Does sparse source coverage keep missing neighbors unknown?",
                "expected_facts": [
                    {"text": "missing neighbors are unknown, not absent", "citation_prefix": "[guardrail:"},
                ],
                "forbidden_phrases": [
                    "no other pull requests",
                    "no other issues",
                    "absence claims are allowed",
                ],
            },
            {
                "key": "github:no-product-vocabulary",
                "category": "generic-no-product-vocabulary",
                "source_coverage_state": "sparse",
                "question": "Does the GitHub open graph answer avoid Flink, Jira, TPM, and analytics vocabulary?",
                "expected_facts": [
                    {"text": "missing neighbors are unknown, not absent", "citation_prefix": "[guardrail:"},
                ],
                "forbidden_phrases": forbidden_common,
            },
        ],
    }


def issue_key(repo: str, number: int) -> str:
    return f"github:issue:{repo}#{number}"


def pr_key(repo: str, number: int) -> str:
    return f"github:pr:{repo}#{number}"


def user_key(login: str) -> str:
    return f"github:user:{login}"


def user_login(row: dict[str, Any]) -> str:
    user = row.get("user")
    if not isinstance(user, dict):
        return ""
    return clean_text(user.get("login"))


def latest_timestamp(*rows: dict[str, Any]) -> str:
    values = sorted(
        {
            clean_text(row.get(key))
            for row in rows
            for key in ["merged_at", "closed_at", "updated_at", "created_at"]
            if clean_text(row.get(key))
        }
    )
    return values[-1] if values else "1970-01-01T00:00:00Z"


def closing_locator(repo: str, issue_number: int, pr_number: int, pull: dict[str, Any]) -> str:
    body = clean_text(pull.get("body"))
    if body:
        for pattern in [
            rf"(?:fixes|closes|resolves)\s+https://github\.com/{re.escape(repo)}/issues/{issue_number}",
            rf"(?:fixes|closes|resolves)\s+#{issue_number}",
        ]:
            match = re.search(pattern, body, re.IGNORECASE)
            if match:
                return match.group(0)
    return f"{repo}#{pr_number} closes issue #{issue_number}"


def clean_text(value: Any) -> str:
    return str(value or "").strip()


def write_json(path: Path, payload: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(payload, indent=2, sort_keys=True) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()

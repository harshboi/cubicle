#!/usr/bin/env python3

from __future__ import annotations

import pathlib
import sys
import unittest


TOOLS_DIR = pathlib.Path(__file__).parent
if str(TOOLS_DIR) not in sys.path:
    sys.path.insert(0, str(TOOLS_DIR))

import github_issue_pr_open_graph_fixture as capture  # noqa: E402


class GitHubIssuePROpenGraphFixtureTest(unittest.TestCase):
    def test_build_fixture_creates_issue_pr_and_user_relationships(self) -> None:
        issue = {
            "number": 12,
            "title": "Bug report",
            "html_url": "https://github.com/acme/app/issues/12",
            "updated_at": "2026-01-02T00:00:00Z",
            "user": {"login": "reporter"},
        }
        pull = {
            "number": 34,
            "title": "Fix bug",
            "html_url": "https://github.com/acme/app/pull/34",
            "updated_at": "2026-01-03T00:00:00Z",
            "body": "Fixes #12",
            "user": {"login": "author"},
        }

        fixture = capture.build_fixture("acme/app", issue, pull)
        golden = capture.build_golden("acme/app", issue, pull, fixture)

        self.assertEqual(fixture["sourceInstance"], "github.com/acme/app")
        self.assertEqual(len(fixture["objects"]), 4)
        self.assertEqual(len(fixture["associations"]), 3)
        associations = {row["associationType"]: row for row in fixture["associations"]}
        self.assertEqual(associations["closed_by"]["locator"], "Fixes #12")
        self.assertEqual(associations["closed_by"]["sourceSystem"], "github")
        self.assertIn("github:issue:acme/app#12", str(golden))
        self.assertIn("closed_by", str(golden))

    def test_closing_locator_handles_full_issue_url(self) -> None:
        locator = capture.closing_locator(
            "cli/cli",
            13262,
            13523,
            {"body": "Fixes https://github.com/cli/cli/issues/13262"},
        )

        self.assertEqual(locator, "Fixes https://github.com/cli/cli/issues/13262")


if __name__ == "__main__":
    unittest.main()

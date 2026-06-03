---
name: cubicle-repo-workflow
description: Work safely in the Cubicle repository. Use when Codex is asked to modify Cubicle code, create or split Cubicle PRs, update PR descriptions, choose where changes belong in the stacked branch flow, review Cubicle diffs, or explain implementation risk for Cubicle repo work.
---

# Cubicle Repo Workflow

Use this skill for code changes and PR work in `harshboi/cubicle`.

## Start Here

Run these before editing:

```bash
pwd
git status --short --branch
git log --oneline --decorate --graph --max-count=12
gh pr list --state open --json number,title,headRefName,baseRefName,url
```

Do not rely on old PR numbers or branch names without re-checking GitHub.

## Change Placement

Keep PRs single-purpose.

```text
change request
 |
 +-- data layer / DB setup
 |     -> DAO / KnowledgeStore PR
 |
 +-- connector abstractions
 |     -> signal connector substrate PR
 |
 +-- production refresh wiring
 |     -> connector wiring PR
 |
 +-- app launch / runtime root
 |     -> runtime fallback PR
 |
 +-- iMessage display correctness
       -> iMessage focus-state PR
```

If a bug appears during verification but is not the PR's purpose, split it onto a new branch stacked on the nearest appropriate base.

## PR Hygiene

Use `--force-with-lease`, never plain force push.

```text
before push
 |
 +-- git status --short --branch
 +-- targeted tests for touched behavior
 +-- git diff --check
 +-- gh pr diff <number> --name-only
```

When writing PR bodies, use:

```text
## What
## Why
## How
## Testing
```

Prefer ASCII DAGs with starred important files and one-line inline summaries. Keep prose short.

## Verification

Use focused tests first, then broaden when shared behavior changes:

```bash
swift test --filter SignalConnectorTests
swift test --filter QuestionEngineCoreIntegrationTests/testPersonFocus
swift test
bash scripts/build-app.sh
```

For GitHub state, prefer `gh pr view`, `gh pr diff --name-only`, and `gh pr list`.

## References

Read `references/pr-stack.md` when the task involves branch/PR placement, stacked PRs, or splitting mixed work.

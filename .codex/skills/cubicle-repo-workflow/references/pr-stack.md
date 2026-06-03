# Cubicle PR Stack

Verify this with GitHub before relying on it.

```text
main
 |
 +-- PR #2 chore/knowledge-dao-refactor        OPEN
 |     -> data access / DB setup abstractions
 |
 +-- PR #3 feat/signal-connectors              OPEN
 |     -> connector substrate and generic signal models
 |
 +-- PR #10 feat/wire-signal-connectors        OPEN
 |     -> production refresh wiring
 |
 +-- PR #11 fix/runtime-root-fallback          MERGED into #10 head
 |     -> Finder app runtime root fallback
 |
 +-- PR #12 fix/imessage-unavailable-focus-state MERGED into #10 head
       -> iMessage unavailable focus display
```

Use separate PRs for separate concerns. If a verification bug is found while working on another PR, create a stacked branch on the nearest suitable base.

Stack caveat:

```text
review split
 |
 +-- #10 connector wiring
 +-- #11 runtime fallback
 +-- #12 iMessage unavailable
 |
 v
current #10 diff may include #11/#12 after they merge into its head branch
```

Trust `gh pr diff <number> --name-only` over PR body claims.

## Common Commands

```bash
gh pr view <number> --json number,title,baseRefName,headRefName,url
gh pr diff <number> --name-only
gh pr edit <number> --body-file -
git push --force-with-lease origin <branch>
```

---
name: cubicle-macos-runtime
description: Build, run, and debug the Cubicle macOS Swift app. Use when Codex is asked about Cubicle app launch, runtime roots, env vars, cache files, SQLite access, Webex/iMessage runtime refresh, /Applications/Cubicle.app, Swift tests, or macOS permission issues.
---

# Cubicle macOS Runtime

Use this skill for the local Swift app under `apps/cubicle-macos`.

## Runtime Shape

```text
/Applications/Cubicle.app
 |
 +-- RuntimeConfiguration.current
 |     -> resolves runtime root + Webex tuning + Codex binary
 |
 +-- NativeRuntimeStore
 |     -> loads/writes focus cache JSON
 |
 +-- KnowledgeStore
 |     -> opens knowledge/knowledge.db
 |
 +-- NativeRefreshCoordinator
       -> runs refresh scopes
```

## Build And Run

```bash
cd apps/cubicle-macos
swift test
```

```bash
cd <repo-root>
bash scripts/build-app.sh
open /Applications/Cubicle.app
```

CLI refresh entry points:

```bash
env -u GETWEBEXSPACE_RUNTIME_ROOT /Applications/Cubicle.app/Contents/MacOS/Cubicle --refresh-person-focus-cache
env -u GETWEBEXSPACE_RUNTIME_ROOT /Applications/Cubicle.app/Contents/MacOS/Cubicle --refresh-space-focus-cache
env -u GETWEBEXSPACE_RUNTIME_ROOT /Applications/Cubicle.app/Contents/MacOS/Cubicle --sync-webex-now
```

## Debugging Path

```text
runtime bug
 |
 +-- check process
 |     -> pgrep -fl Cubicle
 |
 +-- check runtime root
 |     -> GETWEBEXSPACE_RUNTIME_ROOT or fallback path
 |
 +-- check cache files
 |     -> knowledge/native/live_*_focus_cache_*.json
 |     -> knowledge/native/*_focus_cache_*.native.json
 |
 +-- check DB
       -> knowledge/knowledge.db
```

If the installed app was rebuilt while Cubicle is running, tell the user to quit/reopen it.

## macOS Permissions

iMessage reads `~/Library/Messages/chat.db`. If the app sees `authorization denied`, this is usually Full Disk Access/TCC, not a SQL ordering bug.

```text
iMessage unavailable
 |
 +-- System Settings
       |
       +-- Privacy & Security
             |
             +-- Full Disk Access
                   -> enable /Applications/Cubicle.app
```

## References

Read `references/env-vars.md` when the user asks about env vars or runtime configuration.

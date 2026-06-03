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

## Knowledge Permission Error

If the app says it cannot save `knowledge` inside `getwebexspace-data`, check runtime root before changing file permissions.

```text
misleading permission error
 |
 +-- app lacks GETWEBEXSPACE_RUNTIME_ROOT
 |     -> may default to /Volumes/Webex/getwebexspace-data
 |
 +-- real data root is usually ~/Desktop/getwebexspace-data
 |
 v
set GUI launch env, then relaunch
```

Use:

```bash
launchctl getenv GETWEBEXSPACE_RUNTIME_ROOT
launchctl setenv GETWEBEXSPACE_RUNTIME_ROOT "$HOME/Desktop/getwebexspace-data"
osascript -e 'quit app "Cubicle"'
open /Applications/Cubicle.app
```

Verify the running process:

```bash
pid=$(pgrep -n -f '/Applications/Cubicle.app/Contents/MacOS/Cubicle' || true)
ps eww -p "$pid" | tr ' ' '\n' | rg 'GETWEBEXSPACE_RUNTIME_ROOT|HOME|PWD'
```

## Codex Status 1

If the app reports `Codex failed after 2 attempts: Codex exited with status 1`, inspect the Codex job log before changing code.

```text
Codex status 1
 |
 +-- read latest knowledge/codex/jobs/**/run.log
 |
 +-- check selected Settings -> Codex -> GPT model
 |
 +-- reproduce with codex exec from a readable working directory
```

Known local failure:

```text
unsupported model
 |
 +-- gpt-5 rejected for ChatGPT Codex account
 |
 +-- gpt-5.5 works
 +-- gpt-5.4-mini works
```

Use:

```bash
tmpout=/tmp/cubicle-codex-test-output.txt
printf 'Say OK only.\n' | codex exec \
  --ignore-user-config \
  --skip-git-repo-check \
  --ephemeral \
  --model gpt-5.5 \
  --config 'model_reasoning_effort="low"' \
  --output-last-message "$tmpout" -
cat "$tmpout"
```

If Terminal cannot read `~/Desktop/getwebexspace-data`, grant Terminal Full Disk Access or inspect the run log in Finder. Do not infer the cause from the app's retry summary alone.

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

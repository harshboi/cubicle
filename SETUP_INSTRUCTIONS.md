# Setup Instructions

## Quick Path

```text
repo checkout
  |
  +-- install/build app
  |     -> bash scripts/build-app.sh
  |
  +-- set GUI runtime env
  |     -> GETWEBEXSPACE_RUNTIME_ROOT
  |     -> optional CUBICLE_JSON_CONFIG_ENABLED
  |
  +-- grant macOS access
  |     -> Full Disk Access for /Applications/Cubicle.app
  |
  +-- open app
        -> open /Applications/Cubicle.app
```

## Prereqs

- macOS with Xcode command line tools.
- SwiftPM available through `swift`.
- GitHub CLI only if you are working with PRs.
- Local runtime data root, usually:

```bash
/Users/prabhat/Desktop/getwebexspace-data
```

## Build

```bash
cd /Users/prabhat/workspace/cubicle
bash scripts/build-app.sh
```

This writes and installs:

```text
/Applications/Cubicle.app
```

## Launch Env

GUI apps do not inherit shell env automatically. Set runtime env with `launchctl`:

```bash
launchctl setenv GETWEBEXSPACE_RUNTIME_ROOT "$HOME/Desktop/getwebexspace-data"
```

Enable JSON config defaults/overlay support:

```bash
launchctl setenv CUBICLE_JSON_CONFIG_ENABLED 1
```

Optional operator config:

```bash
launchctl setenv CUBICLE_JSON_CONFIG_DIR "$HOME/Desktop/getwebexspace-data/config"
launchctl setenv CUBICLE_CONFIG_FILE "$HOME/Desktop/getwebexspace-data/config/cubicle.json"
```

If `CUBICLE_CONFIG_FILE` is unset, the app uses bundled `base.json` defaults and overlays the default runtime config path only if present.

## Start App

```bash
osascript -e 'quit app "Cubicle"' || true
open /Applications/Cubicle.app
```

Verify the running process inherited env:

```bash
pid=$(pgrep -n -f '/Applications/Cubicle.app/Contents/MacOS/Cubicle')
ps eww -p "$pid" | tr ' ' '\n' | rg 'GETWEBEXSPACE_RUNTIME_ROOT|CUBICLE_JSON_CONFIG'
```

Expected:

```text
GETWEBEXSPACE_RUNTIME_ROOT=/Users/prabhat/Desktop/getwebexspace-data
CUBICLE_JSON_CONFIG_ENABLED=1
```

## macOS Permissions

```text
Cubicle.app
  |
  +-- Webex
  |     -> OAuth/API access through app settings
  |
  +-- iMessage
        -> reads ~/Library/Messages/chat.db
        -> needs Full Disk Access
```

Grant access:

```text
System Settings
  -> Privacy & Security
    -> Full Disk Access
      -> enable /Applications/Cubicle.app
```

## JSON Config

```text
bundled base.json
  |
  +-- optional cubicle.json overlay
  |     -> objects deep-merge
  |     -> arrays replace
  |     -> null falls back to bundled default
  |
  +-- RuntimeConfiguration
        -> services read one resolved snapshot
```

Do not put secrets in JSON config. Secret-bearing keys are rejected.

Useful env vars:

```text
CUBICLE_JSON_CONFIG_ENABLED -> turns JSON config on
CUBICLE_JSON_CONFIG_DIR     -> base directory for config-relative paths
CUBICLE_CONFIG_FILE         -> explicit cubicle.json path
GETWEBEXSPACE_RUNTIME_ROOT  -> local runtime data root
```

## Tests

Run all Swift tests:

```bash
cd /Users/prabhat/workspace/cubicle
swift test
```

Focused JSON config tests:

```bash
swift test --filter MacAppJSONConfigurationDocumentsTests
```

Focused Webex sync tests:

```bash
swift test --filter WebexSyncEngineTests
```

## Runtime Data

Runtime/generated files should stay outside git:

```text
GETWEBEXSPACE_RUNTIME_ROOT
  |
  +-- knowledge/
  |     -> knowledge.db, cache files, Codex job artifacts
  |
  +-- config/
        -> optional cubicle.json overlays
```

Do not commit:

```text
knowledge/
.build/
.venv/
.env*
logs/
*.tfstate*
*.tfvars*
```


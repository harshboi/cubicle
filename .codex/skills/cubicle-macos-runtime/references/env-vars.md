# Cubicle Environment Variables

## macOS App

```text
runtime
 |
 +-- GETWEBEXSPACE_RUNTIME_ROOT
 +-- CODEX_BIN
 +-- PATH
 +-- SHELL
 +-- LANG
 +-- LC_ALL
```

```text
Webex runtime
 |
 +-- WEBEX_API_BASE_URL
 +-- WEBEX_API_PAGE_SIZE
 +-- WEBEX_API_RETRIES
 +-- WEBEX_API_TIMEOUT_SECONDS
 +-- WEBEX_SYNC_CONCURRENCY_LIMIT
 +-- WEBEX_PUBLIC_WEBHOOK_URL
 |
 +-- WEBEX_ADAPTIVE_ACTIVE_INTERVAL_SECONDS
 +-- WEBEX_ADAPTIVE_RECENT_INTERVAL_SECONDS
 +-- WEBEX_ADAPTIVE_BACKGROUND_INTERVAL_SECONDS
 +-- WEBEX_ADAPTIVE_JITTER_PERCENT
```

```text
Webex OAuth
 |
 +-- WEBEX_OAUTH_CLIENT_ID
 +-- WEBEX_OAUTH_CLIENT_SECRET
 +-- WEBEX_OAUTH_REDIRECT_URI
 +-- WEBEX_OAUTH_SCOPE
 +-- WEBEX_OAUTH_TOKEN_FILE
 +-- WEBEX_OAUTH_REFRESH_SKEW_SECONDS
 +-- WEBEX_OAUTH_REFRESH_TOKEN_SKEW_SECONDS
```

```text
Outlook / Graph OAuth
 |
 +-- OUTLOOK_OAUTH_CLIENT_ID
 +-- OUTLOOK_OAUTH_CLIENT_SECRET
 +-- OUTLOOK_OAUTH_REDIRECT_URI
 +-- OUTLOOK_OAUTH_SCOPE
 +-- OUTLOOK_OAUTH_TENANT
 +-- OUTLOOK_OAUTH_TOKEN_FILE
 |
 +-- MS_GRAPH_OAUTH_CLIENT_ID
 +-- MS_GRAPH_OAUTH_CLIENT_SECRET
 +-- MS_GRAPH_OAUTH_REDIRECT_URI
 +-- MS_GRAPH_OAUTH_SCOPE
 +-- MS_GRAPH_OAUTH_TENANT
 +-- MS_GRAPH_OAUTH_TOKEN_FILE
 |
 +-- MICROSOFT_OAUTH_CLIENT_ID
 +-- MICROSOFT_OAUTH_CLIENT_SECRET
 |
 +-- OAUTH_CALLBACK_TIMEOUT_SECONDS
```

The most common local setup is:

```bash
export GETWEBEXSPACE_RUNTIME_ROOT=<path-to-getwebexspace-data>
export CODEX_BIN=/opt/homebrew/bin/codex
```

For Finder/Dock-launched apps, shell exports do not apply. Use `launchctl`:

```bash
launchctl setenv GETWEBEXSPACE_RUNTIME_ROOT "$HOME/Desktop/getwebexspace-data"
osascript -e 'quit app "Cubicle"'
open /Applications/Cubicle.app
```

Failure shape:

```text
"permission to save knowledge"
 |
 +-- first check GETWEBEXSPACE_RUNTIME_ROOT in the app process
 |
 +-- then check Desktop/Full Disk Access if the root is correct
```

Codex model/account check:

```text
Codex exited status 1
 |
 +-- run.log says unsupported model
 |
 +-- change Settings -> Codex -> GPT model
       -> GPT-5.5 or GPT-5.4 Mini
```

Observed on 2026-06-03: `gpt-5` was rejected by the local ChatGPT Codex account; `gpt-5.5` and `gpt-5.4-mini` succeeded.

## Other Repo Surfaces

These are not needed for normal macOS app work unless the user explicitly asks about those systems.

```text
transcription backend
 |
 +-- TRANSCRIPTION_*
 +-- TEXT_INTELLIGENCE_*
 +-- MISTRAL_API_KEY
 +-- PYANNOTE_AUTH_TOKEN
 +-- HF_TOKEN / HUGGINGFACE_TOKEN
 +-- VLLM_*
 +-- VOXTRAL_*
```

```text
VoiceNotes / infra
 |
 +-- VOICENOTES_*
 +-- AWS_*
 +-- HF_TOKEN_FILE / HUGGINGFACE_TOKEN_FILE
```

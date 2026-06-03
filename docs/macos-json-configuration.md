# macOS JSON Configuration

Cubicle's macOS app keeps its existing defaults, persisted UI settings, text target files, and environment variables. These JSON documents define the optional operator-facing configuration layer that can be enabled separately by runtime wiring.

## Files

When JSON configuration loading is enabled, the default directory is:

```text
$GETWEBEXSPACE_RUNTIME_ROOT/config
```

The document names are:

```text
runtime.json
targets.json
codex.json
question-generation.json
```

Runtime wiring can also point to another directory or individual files with env vars:

```text
CUBICLE_JSON_CONFIG_ENABLED=false
CUBICLE_JSON_CONFIG_DIR=/path/to/config
CUBICLE_RUNTIME_CONFIG_JSON=/path/to/runtime.json
CUBICLE_TARGETS_CONFIG_JSON=/path/to/targets.json
CUBICLE_CODEX_CONFIG_JSON=/path/to/codex.json
CUBICLE_QUESTION_CONFIG_JSON=/path/to/question-generation.json
```

`CUBICLE_JSON_CONFIG_ENABLED` defaults to `false`. Accepted truthy values are `1`, `true`, `yes`, and `on`.

## runtime.json

```json
{
  "version": 1,
  "codex": {
    "executable": "/opt/homebrew/bin/codex"
  },
  "webex": {
    "api_base_url": "https://webexapis.com/v1",
    "page_size": 100,
    "retry_count": 5,
    "timeout_seconds": 20,
    "oauth_token_file": ".webex_oauth_tokens.json",
    "oauth_refresh_skew_seconds": 300,
    "oauth_refresh_token_skew_seconds": 86400,
    "public_webhook_url": null,
    "sync": {
      "concurrency_limit": 3,
      "adaptive_active_interval_seconds": 20,
      "adaptive_recent_interval_seconds": 60,
      "adaptive_background_interval_seconds": 180,
      "adaptive_jitter_percent": 20
    }
  },
  "imessage": {
    "chat_database_path": "~/Library/Messages/chat.db",
    "busy_timeout_milliseconds": 2000
  }
}
```

## targets.json

```json
{
  "version": 1,
  "groups": {
    "important": [
      {
        "kind": "person",
        "label": "Pat Lee",
        "room_id": "Y2lzY29zcGFyazovL3VzL1JPT00v...",
        "room_type": "direct",
        "email": "pat@example.com",
        "auto_reply": false,
        "imessage_handles": ["pat@example.com"]
      },
      {
        "kind": "space",
        "label": "Launch Room",
        "room_id": "Y2lzY29zcGFyazovL3VzL1JPT00v...",
        "room_type": "group"
      }
    ],
    "executives": [
      {
        "kind": "person",
        "label": "Alex Exec",
        "room_id": "Y2lzY29zcGFyazovL3VzL1JPT00v...",
        "email": "alex@example.com"
      }
    ],
    "beliefs": [
      {
        "kind": "space",
        "label": "Architecture",
        "room_id": "Y2lzY29zcGFyazovL3VzL1JPT00v..."
      }
    ]
  }
}
```

## codex.json

```json
{
  "version": 1,
  "run_policy": {
    "timeout_seconds": 120,
    "max_attempts": 2,
    "retry_delay_seconds": 1.5
  },
  "cache_policy": {
    "summary_max_age_seconds": 900,
    "exec_questions_max_age_seconds": 900
  },
  "beliefs": {
    "stale_hours": 24,
    "evidence_chunk_size": 25,
    "max_incremental_window_days": 90
  },
  "question_synthesis": {
    "seed_candidate_limit": 40,
    "query_history_limit": 40,
    "prompt_history_limit": 24,
    "candidate_evidence_limit": 4,
    "output_limit": 7
  }
}
```

## question-generation.json

```json
{
  "version": 1,
  "core": {
    "privacy": {
      "anonymize_users": false,
      "redact_urls": true,
      "redact_emails": true
    },
    "topics": {
      "enabled": true,
      "number_of_topics": 8,
      "minimum_topic_size": 1
    },
    "questions": {
      "top_n": 12,
      "enabled_categories": ["behavioral", "diagnostic", "efficiency", "network"]
    }
  },
  "cubicle": {
    "fallback_draft_limit": 4,
    "generated_question_limit": 12,
    "publishable_question_limit": 4,
    "evidence_limit": 4,
    "core_evidence_limit": 4
  }
}
```

## Non-Goals

This layer does not move OAuth tokens, OAuth client secrets, prompt-version contracts, SQLite schema/migrations, generated caches, or transcription protocol constants into operator JSON.

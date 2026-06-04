# macOS JSON Configuration

Cubicle's macOS app keeps existing defaults, UI settings, DB-backed state, generated caches, and secret stores. The optional JSON configuration layer is a control plane for non-secret runtime tuning, connector selection, Codex/question behavior, and test-app fixture locations.

## Entry Point

Runtime wiring loads one entrypoint:

```text
$GETWEBEXSPACE_RUNTIME_ROOT/config/cubicle.json
```

Operators can override the entrypoint with:

```text
CUBICLE_JSON_CONFIG_ENABLED=true
CUBICLE_CONFIG_FILE=/path/to/cubicle.json
CUBICLE_JSON_CONFIG_DIR=/path/to/config
```

`CUBICLE_JSON_CONFIG_ENABLED` defaults to `false`. Accepted truthy values are `1`, `true`, `yes`, and `on`. When disabled, the app should not read these files or start file watchers.

When enabled, the app always loads the bundled `base.json` defaults first. If no operator `cubicle.json` exists, those defaults are the complete JSON config. If `CUBICLE_CONFIG_FILE` points at a missing file, startup fails visibly instead of silently falling back.

`environment.runtime_root` can set the runtime root after the JSON document is loaded. `GETWEBEXSPACE_RUNTIME_ROOT` still wins when both are present, and the JSON config directory remains the directory that loaded `cubicle.json`.

## Composition

`cubicle.json` can include section files. Each key is the destination top-level section; each value is a relative file path. Included files contain the body of that section.

```json
{
  "version": 1,
  "include": {
    "environment": "environment.json",
    "connectors": "connectors.json",
    "codex": "codex.json",
    "question_generation": "question-generation.json",
    "test_mode": "test-mode.json"
  }
}
```

```text
cubicle.json
 |
 +-- include.environment
 |     -> environment.json
 |
 +-- include.connectors
 |     -> connectors.json
 |
 +-- include.codex
 |     -> codex.json
 |
 +-- include.question_generation
 |     -> question-generation.json
 |
 +-- include.test_mode
       -> test-mode.json
```

Include paths are relative to the file containing `include`. Absolute paths, `~`, and `..` are rejected.

`cubicle.json` can extend one or more JSON files. Paths are relative to the file containing the `extends` entry.

```json
{
  "version": 1,
  "extends": [
    "base.json",
    "connectors/webex.json",
    "connectors/imessage.json",
    "modes/test.json"
  ]
}
```

Merge rules are intentionally small and deterministic:

- Earlier extended files load first; later files override earlier files.
- The current file overrides all extended files.
- Objects deep-merge.
- Scalars replace.
- `null` keeps the bundled/default value when a default exists.
- Arrays replace.
- Extend cycles fail visibly.
- Missing or malformed files fail visibly when JSON config is enabled.

## Reusable Policy Blocks

Each parent section can define internal common policy blocks. Child sections reuse them with `use` and override only the differences.

```json
{
  "codex": {
    "common": {
      "run_policy": {
        "timeout_seconds": 120,
        "max_attempts": 2,
        "retry_delay_seconds": 1.5
      }
    },
    "run_policy": {
      "use": "codex.common.run_policy"
    },
    "question_synthesis": {
      "run_policy": {
        "use": "codex.common.run_policy",
        "timeout_seconds": 180
      }
    }
  },
  "connectors": {
    "common": {
      "network_policy": {
        "timeout_seconds": 20,
        "retry_count": 5,
        "page_size": 100
      }
    },
    "webex": {
      "network_policy": {
        "use": "connectors.common.network_policy",
        "retry_count": 3
      }
    }
  }
}
```

Resolution order:

1. Load the entrypoint.
2. Apply `extends` in order.
3. Load `include` section files.
4. Deep-merge the resolved objects; the current file wins.
5. Resolve `use` references against the merged root object.
6. Decode into typed Cubicle configuration.

Invalid `use` references fail visibly.

## Example `base.json`

```json
{
  "version": 1,
  "environment": {
    "webex": {
      "api_base_url": "https://webexapis.com/v1",
      "oauth_token_file": ".webex_oauth_tokens.json",
      "oauth_refresh_skew_seconds": 300,
      "oauth_refresh_token_skew_seconds": 86400,
      "network_policy": {
        "timeout_seconds": 20,
        "retry_count": 5,
        "page_size": 100
      },
      "sync_policy": {
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
  },
  "connectors": {
    "enabled": ["webex", "imessage"],
    "webex": {},
    "imessage": {}
  },
  "codex": {
    "common": {
      "run_policy": {
        "timeout_seconds": 120,
        "max_attempts": 2,
        "retry_delay_seconds": 1.5
      }
    },
    "run_policy": {
      "use": "codex.common.run_policy"
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
      "run_policy": {
        "use": "codex.common.run_policy"
      },
      "seed_candidate_limit": 40,
      "query_history_limit": 40,
      "prompt_history_limit": 24,
      "candidate_evidence_limit": 4,
      "output_limit": 7
    }
  },
  "question_generation": {
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
}
```

## Example `cubicle.json`

```json
{
  "version": 1,
  "extends": ["base.json"],
  "include": {
    "environment": "environment.json",
    "codex": "codex.json"
  },
  "connectors": {
    "enabled": ["webex", "imessage"],
    "webex": {
      "network_policy": {
        "timeout_seconds": 30
      }
    }
  }
}
```

Connector-local Webex policies override `environment.webex` policies field-by-field. Environment variables still override both JSON layers.

`connectors.enabled` is the authoritative connector selection list. If the list is omitted, connector-local `enabled` booleans are accepted as a fallback. Connector-local objects otherwise hold connector-specific tuning such as policies and fixture paths.

## Test Mode

Test-app mode points to stable input files. These are source data and must not be deleted by runtime cleanup. When `test_mode.enabled` is true:

- `settings` replaces the normal persisted settings file.
- `target_data` replaces important people/spaces, executives, and belief targets for read paths.
- `connector_fixtures.webex` or `connectors.webex.fixture_path` selects a file-backed Webex client.
- `connector_fixtures.imessage` or `connectors.imessage.fixture_path` selects a fixture `chat.db`.
- `protect_paths` prevents configured files/directories from cleanup paths such as OAuth token deletion and refresh-checkpoint clearing.

`target_data`, `settings`, and `protect_paths` resolve relative to the JSON config directory. Connector fixture paths resolve relative to `fixture_root` when it is set.

```json
{
  "test_mode": {
    "enabled": true,
    "profile": "integration",
    "fixture_root": "test-data/fixtures",
    "target_data": "test-data/targets.json",
    "settings": "test-data/settings.json",
    "protect_paths": ["test-data"],
    "connector_fixtures": {
      "webex": "connectors/webex.json",
      "imessage": "connectors/imessage-chat.db"
    }
  }
}
```

`target_data` may be either a root object with `important`, `executives`, and `beliefs`, or a `{ "groups": ... }` wrapper. Map keys can provide the stable person email or Webex room ID when the target object does not repeat it; mixed `important` maps infer person/space kind from those keys when possible.

## What Belongs Here

- Env-like non-secret runtime settings.
- Connector enablement and non-secret connector settings.
- Shared policy objects such as `run_policy`, `network_policy`, `cache_policy`, and `sync_policy`.
- Codex/question-generation tuning.
- Test-app fixture, target-data, settings, and protected-path locations.

## Non-Goals

Do not put these in operator JSON config:

- Important spaces, important people, targets, learned beliefs, questions, relationships, or mutable per-person/per-space preferences. These belong in SQLite/DAO-backed state.
- OAuth access tokens, refresh tokens, client secrets, auth tokens, or other secret material. These belong in Keychain or environment-backed secret paths.
- Generated focus caches, Codex job artifacts, SQLite schema/migrations, prompt-version contracts, or transcription protocol constants.

import XCTest
@testable import GetWebexSpaceMacApp

final class MacAppJSONConfigurationDocumentsTests: XCTestCase {
    private let decoder = JSONDecoder()

    func testTopLevelDocumentDecodesEnvironmentConnectorsCodexQuestionGenerationAndTestMode() throws {
        let json = """
        {
          "version": 1,
          "environment": {
            "codex_executable": "/opt/homebrew/bin/codex",
            "webex": {
              "api_base_url": "https://webexapis.com/v1",
              "oauth_token_file": ".webex_oauth_tokens.json",
              "oauth_refresh_skew_seconds": 600,
              "oauth_refresh_token_skew_seconds": 172800,
              "public_webhook_url": "https://example.com/webex",
              "network_policy": {
                "page_size": 200,
                "retry_count": 4,
                "timeout_seconds": 30
              },
              "sync_policy": {
                "concurrency_limit": 5,
                "adaptive_active_interval_seconds": 15,
                "adaptive_recent_interval_seconds": 45,
                "adaptive_background_interval_seconds": 240,
                "adaptive_jitter_percent": 10
              }
            },
            "imessage": {
              "chat_database_path": "/Users/test/Library/Messages/chat.db",
              "busy_timeout_milliseconds": 1500
            }
          },
          "connectors": {
            "enabled": ["webex", "imessage"],
            "webex": {
              "enabled": true,
              "fixture_path": "fixtures/webex/messages.json"
            },
            "imessage": {
              "enabled": true,
              "fixture_path": "fixtures/imessage/chat.db"
            }
          },
          "codex": {
            "run_policy": {
              "timeout_seconds": 180,
              "max_attempts": 3,
              "retry_delay_seconds": 2.5
            },
            "cache_policy": {
              "summary_max_age_seconds": 600,
              "exec_questions_max_age_seconds": 900
            },
            "beliefs": {
              "stale_hours": 12,
              "evidence_chunk_size": 40,
              "max_incremental_window_days": 60
            },
            "question_synthesis": {
              "seed_candidate_limit": 50,
              "query_history_limit": 80,
              "prompt_history_limit": 20,
              "candidate_evidence_limit": 5,
              "output_limit": 9
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
                "number_of_topics": 10,
                "minimum_topic_size": 2
              },
              "questions": {
                "top_n": 16,
                "enabled_categories": ["behavioral", "diagnostic", "network"]
              }
            },
            "cubicle": {
              "fallback_draft_limit": 5,
              "generated_question_limit": 16,
              "publishable_question_limit": 6,
              "evidence_limit": 5,
              "core_evidence_limit": 6
            }
          },
          "test_mode": {
            "enabled": true,
            "profile": "integration",
            "fixture_root": "test-data/fixtures",
            "target_data": "test-data/targets.json",
            "settings": "test-data/settings.json",
            "protect_paths": ["test-data"],
            "connector_fixtures": {
              "webex": "test-data/connectors/webex.json"
            }
          }
        }
        """

        let document = try decode(MacAppJSONConfigurationDocument.self, from: json)

        XCTAssertEqual(document.version, 1)
        XCTAssertEqual(document.environment?.codexExecutable, "/opt/homebrew/bin/codex")
        XCTAssertEqual(document.environment?.webex?.apiBaseURL, "https://webexapis.com/v1")
        XCTAssertEqual(document.environment?.webex?.oauthTokenFile, ".webex_oauth_tokens.json")
        XCTAssertEqual(document.environment?.webex?.oauthRefreshSkewSeconds, 600)
        XCTAssertEqual(document.environment?.webex?.oauthRefreshTokenSkewSeconds, 172_800)
        XCTAssertEqual(document.environment?.webex?.publicWebhookURL, "https://example.com/webex")
        XCTAssertEqual(document.environment?.webex?.networkPolicy?.pageSize, 200)
        XCTAssertEqual(document.environment?.webex?.networkPolicy?.retryCount, 4)
        XCTAssertEqual(document.environment?.webex?.networkPolicy?.timeoutSeconds, 30)
        XCTAssertEqual(document.environment?.webex?.syncPolicy?.concurrencyLimit, 5)
        XCTAssertEqual(document.environment?.webex?.syncPolicy?.adaptiveActiveIntervalSeconds, 15)
        XCTAssertEqual(document.environment?.webex?.syncPolicy?.adaptiveRecentIntervalSeconds, 45)
        XCTAssertEqual(document.environment?.webex?.syncPolicy?.adaptiveBackgroundIntervalSeconds, 240)
        XCTAssertEqual(document.environment?.webex?.syncPolicy?.adaptiveJitterPercent, 10)
        XCTAssertEqual(document.environment?.imessage?.chatDatabasePath, "/Users/test/Library/Messages/chat.db")
        XCTAssertEqual(document.environment?.imessage?.busyTimeoutMilliseconds, 1500)
        XCTAssertEqual(document.connectors?.enabled, ["webex", "imessage"])
        XCTAssertEqual(document.connectors?.webex?.enabled, true)
        XCTAssertEqual(document.connectors?.webex?.fixturePath, "fixtures/webex/messages.json")
        XCTAssertEqual(document.connectors?.imessage?.fixturePath, "fixtures/imessage/chat.db")
        XCTAssertEqual(document.codex?.runPolicy?.timeoutSeconds, 180)
        XCTAssertEqual(document.codex?.runPolicy?.maxAttempts, 3)
        XCTAssertEqual(document.codex?.runPolicy?.retryDelaySeconds, 2.5)
        XCTAssertEqual(document.codex?.cachePolicy?.summaryMaxAgeSeconds, 600)
        XCTAssertEqual(document.codex?.cachePolicy?.execQuestionsMaxAgeSeconds, 900)
        XCTAssertEqual(document.codex?.beliefs?.staleHours, 12)
        XCTAssertEqual(document.codex?.beliefs?.evidenceChunkSize, 40)
        XCTAssertEqual(document.codex?.beliefs?.maxIncrementalWindowDays, 60)
        XCTAssertEqual(document.codex?.questionSynthesis?.seedCandidateLimit, 50)
        XCTAssertEqual(document.codex?.questionSynthesis?.queryHistoryLimit, 80)
        XCTAssertEqual(document.codex?.questionSynthesis?.promptHistoryLimit, 20)
        XCTAssertEqual(document.codex?.questionSynthesis?.candidateEvidenceLimit, 5)
        XCTAssertEqual(document.codex?.questionSynthesis?.outputLimit, 9)
        XCTAssertEqual(document.questionGeneration?.core?.privacy?.anonymizeUsers, false)
        XCTAssertEqual(document.questionGeneration?.core?.privacy?.redactURLs, true)
        XCTAssertEqual(document.questionGeneration?.core?.privacy?.redactEmails, true)
        XCTAssertEqual(document.questionGeneration?.core?.topics?.enabled, true)
        XCTAssertEqual(document.questionGeneration?.core?.topics?.numberOfTopics, 10)
        XCTAssertEqual(document.questionGeneration?.core?.topics?.minimumTopicSize, 2)
        XCTAssertEqual(document.questionGeneration?.core?.questions?.topN, 16)
        XCTAssertEqual(document.questionGeneration?.core?.questions?.enabledCategories, ["behavioral", "diagnostic", "network"])
        XCTAssertEqual(document.questionGeneration?.cubicle?.fallbackDraftLimit, 5)
        XCTAssertEqual(document.questionGeneration?.cubicle?.generatedQuestionLimit, 16)
        XCTAssertEqual(document.questionGeneration?.cubicle?.publishableQuestionLimit, 6)
        XCTAssertEqual(document.questionGeneration?.cubicle?.evidenceLimit, 5)
        XCTAssertEqual(document.questionGeneration?.cubicle?.coreEvidenceLimit, 6)
        XCTAssertEqual(document.testMode?.enabled, true)
        XCTAssertEqual(document.testMode?.profile, "integration")
        XCTAssertEqual(document.testMode?.fixtureRoot, "test-data/fixtures")
        XCTAssertEqual(document.testMode?.targetData, "test-data/targets.json")
        XCTAssertEqual(document.testMode?.settings, "test-data/settings.json")
        XCTAssertEqual(document.testMode?.protectPaths, ["test-data"])
        XCTAssertEqual(document.testMode?.connectorFixtures?["webex"], "test-data/connectors/webex.json")
    }

    func testBundledBaseDefaultsDecodeAndContainNoUserData() throws {
        let url = try XCTUnwrap(MacAppJSONConfigurationDefaults.bundledBaseURL)

        let document = try MacAppJSONConfigurationComposer()
            .loadDocument(entrypointURL: url)

        XCTAssertEqual(document.environment?.webex?.apiBaseURL, "https://webexapis.com/v1")
        XCTAssertEqual(document.environment?.webex?.networkPolicy?.pageSize, 100)
        XCTAssertEqual(document.environment?.webex?.networkPolicy?.retryCount, 5)
        XCTAssertEqual(document.environment?.webex?.networkPolicy?.timeoutSeconds, 20)
        XCTAssertEqual(document.environment?.webex?.syncPolicy?.concurrencyLimit, 3)
        XCTAssertEqual(document.environment?.imessage?.chatDatabasePath, "~/Library/Messages/chat.db")
        XCTAssertEqual(document.connectors?.connectorEnabled("webex"), true)
        XCTAssertEqual(document.connectors?.connectorEnabled("imessage"), true)
        XCTAssertNil(document.connectors?.webex?.enabled)
        XCTAssertNil(document.connectors?.imessage?.enabled)
        XCTAssertEqual(document.codex?.runPolicy?.timeoutSeconds, 120)
        XCTAssertEqual(document.codex?.questionSynthesis?.runPolicy?.timeoutSeconds, 120)
        XCTAssertEqual(document.codex?.beliefs?.maxIncrementalWindowDays, 90)
        XCTAssertEqual(document.questionGeneration?.cubicle?.publishableQuestionLimit, 4)
        XCTAssertEqual(document.testMode?.enabled, false)
        XCTAssertTrue(document.testMode?.protectPaths?.isEmpty ?? false)
        XCTAssertNil(document.testMode?.targetData)
        XCTAssertNil(document.testMode?.settings)
        XCTAssertNil(document.testMode?.connectorFixtures)
    }

    func testComposerLoadsBaseExtendsDeepMergesObjectsAndReplacesArrays() throws {
        let root = temporaryRuntimeRoot(label: "compose")
        defer { try? FileManager.default.removeItem(at: root) }
        try writeConfigFile(
            root: root,
            filename: MacAppJSONConfigurationFiles.base,
            contents: """
            {
              "version": 1,
              "connectors": {
                "enabled": ["webex"],
                "webex": {
                  "enabled": true,
                  "network_policy": {
                    "retry_count": 5,
                    "timeout_seconds": 20
                  }
                }
              },
              "test_mode": {
                "protect_paths": ["test-data"]
              }
            }
            """
        )
        try writeConfigFile(
            root: root,
            filename: MacAppJSONConfigurationFiles.entrypoint,
            contents: """
            {
              "extends": ["base.json"],
              "connectors": {
                "enabled": ["webex", "imessage"],
                "webex": {
                  "network_policy": {
                    "timeout_seconds": 45
                  }
                }
              },
              "test_mode": {
                "protect_paths": ["test-data", "fixtures"]
              }
            }
            """
        )

        let document = try MacAppJSONConfigurationComposer()
            .loadDocument(entrypointURL: root.appendingPathComponent(MacAppJSONConfigurationFiles.entrypoint))

        XCTAssertEqual(document.connectors?.enabled, ["webex", "imessage"])
        XCTAssertEqual(document.connectors?.webex?.enabled, true)
        XCTAssertEqual(document.connectors?.webex?.networkPolicy?.retryCount, 5)
        XCTAssertEqual(document.connectors?.webex?.networkPolicy?.timeoutSeconds, 45)
        XCTAssertEqual(document.testMode?.protectPaths, ["test-data", "fixtures"])
    }

    func testComposerLoadsIncludedSectionFilesAndLetsEntrypointOverrideThem() throws {
        let root = temporaryRuntimeRoot(label: "include")
        defer { try? FileManager.default.removeItem(at: root) }
        try writeConfigFile(
            root: root,
            filename: "environment.json",
            contents: """
            {
              "webex": {
                "api_base_url": "https://webexapis.com/v1",
                "network_policy": {
                  "retry_count": 5,
                  "timeout_seconds": 20
                }
              },
              "imessage": {
                "busy_timeout_milliseconds": 1500
              }
            }
            """
        )
        try writeConfigFile(
            root: root,
            filename: "codex.json",
            contents: """
            {
              "run_policy": {
                "max_attempts": 3,
                "timeout_seconds": 120
              }
            }
            """
        )
        try writeConfigFile(
            root: root,
            filename: MacAppJSONConfigurationFiles.entrypoint,
            contents: """
            {
              "version": 1,
              "include": {
                "environment": "environment.json",
                "codex": "codex.json"
              },
              "environment": {
                "webex": {
                  "network_policy": {
                    "timeout_seconds": 45
                  }
                }
              }
            }
            """
        )

        let document = try MacAppJSONConfigurationComposer()
            .loadDocument(entrypointURL: root.appendingPathComponent(MacAppJSONConfigurationFiles.entrypoint))

        XCTAssertEqual(document.environment?.webex?.apiBaseURL, "https://webexapis.com/v1")
        XCTAssertEqual(document.environment?.webex?.networkPolicy?.retryCount, 5)
        XCTAssertEqual(document.environment?.webex?.networkPolicy?.timeoutSeconds, 45)
        XCTAssertEqual(document.environment?.imessage?.busyTimeoutMilliseconds, 1500)
        XCTAssertEqual(document.codex?.runPolicy?.maxAttempts, 3)
        XCTAssertEqual(document.codex?.runPolicy?.timeoutSeconds, 120)
    }

    func testComposerResolvesParentScopedCommonPolicyUseReferences() throws {
        let root = temporaryRuntimeRoot(label: "use")
        defer { try? FileManager.default.removeItem(at: root) }
        try writeConfigFile(
            root: root,
            filename: MacAppJSONConfigurationFiles.entrypoint,
            contents: """
            {
              "version": 1,
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
            """
        )

        let document = try MacAppJSONConfigurationComposer()
            .loadDocument(entrypointURL: root.appendingPathComponent(MacAppJSONConfigurationFiles.entrypoint))

        XCTAssertEqual(document.codex?.runPolicy?.timeoutSeconds, 120)
        XCTAssertEqual(document.codex?.runPolicy?.maxAttempts, 2)
        XCTAssertEqual(document.codex?.runPolicy?.retryDelaySeconds, 1.5)
        XCTAssertEqual(document.codex?.questionSynthesis?.runPolicy?.timeoutSeconds, 180)
        XCTAssertEqual(document.codex?.questionSynthesis?.runPolicy?.maxAttempts, 2)
        XCTAssertEqual(document.codex?.questionSynthesis?.runPolicy?.retryDelaySeconds, 1.5)
        XCTAssertEqual(document.connectors?.webex?.networkPolicy?.timeoutSeconds, 20)
        XCTAssertEqual(document.connectors?.webex?.networkPolicy?.retryCount, 3)
        XCTAssertEqual(document.connectors?.webex?.networkPolicy?.pageSize, 100)
    }

    func testComposerRejectsExtendsCycles() throws {
        let root = temporaryRuntimeRoot(label: "cycle")
        defer { try? FileManager.default.removeItem(at: root) }
        try writeConfigFile(root: root, filename: "a.json", contents: #"{ "extends": ["b.json"] }"#)
        try writeConfigFile(root: root, filename: "b.json", contents: #"{ "extends": ["a.json"] }"#)

        XCTAssertThrowsError(
            try MacAppJSONConfigurationComposer()
                .loadDocument(entrypointURL: root.appendingPathComponent("a.json"))
        ) { error in
            guard case MacAppJSONConfigurationError.extendCycle = error else {
                return XCTFail("Expected extend cycle, got \(error)")
            }
        }
    }

    func testComposerRejectsInvalidUseReferences() throws {
        let root = temporaryRuntimeRoot(label: "bad-use")
        defer { try? FileManager.default.removeItem(at: root) }
        try writeConfigFile(
            root: root,
            filename: MacAppJSONConfigurationFiles.entrypoint,
            contents: #"{ "codex": { "run_policy": { "use": "codex.common.missing" } } }"#
        )

        XCTAssertThrowsError(
            try MacAppJSONConfigurationComposer()
                .loadDocument(entrypointURL: root.appendingPathComponent(MacAppJSONConfigurationFiles.entrypoint))
        ) { error in
            XCTAssertEqual(error as? MacAppJSONConfigurationError, .invalidUseReference("codex.common.missing"))
        }
    }

    func testComposerRejectsInvalidIncludeShape() throws {
        let root = temporaryRuntimeRoot(label: "bad-include-shape")
        defer { try? FileManager.default.removeItem(at: root) }
        try writeConfigFile(
            root: root,
            filename: MacAppJSONConfigurationFiles.entrypoint,
            contents: #"{ "include": ["environment.json"] }"#
        )

        XCTAssertThrowsError(
            try MacAppJSONConfigurationComposer()
                .loadDocument(entrypointURL: root.appendingPathComponent(MacAppJSONConfigurationFiles.entrypoint))
        ) { error in
            XCTAssertEqual(
                error as? MacAppJSONConfigurationError,
                .invalidInclude(root.appendingPathComponent(MacAppJSONConfigurationFiles.entrypoint).standardizedFileURL)
            )
        }
    }

    func testComposerRejectsUnsafeIncludePaths() throws {
        let paths = [
            "../environment.json",
            "/tmp/environment.json",
            "~/environment.json"
        ]

        for path in paths {
            let root = temporaryRuntimeRoot(label: "bad-include-path")
            defer { try? FileManager.default.removeItem(at: root) }
            try writeConfigFile(
                root: root,
                filename: MacAppJSONConfigurationFiles.entrypoint,
                contents: """
                {
                  "include": {
                    "environment": "\(path)"
                  }
                }
                """
            )

            XCTAssertThrowsError(
                try MacAppJSONConfigurationComposer()
                    .loadDocument(entrypointURL: root.appendingPathComponent(MacAppJSONConfigurationFiles.entrypoint))
            ) { error in
                XCTAssertEqual(
                    error as? MacAppJSONConfigurationError,
                    .invalidIncludePath(
                        root.appendingPathComponent(MacAppJSONConfigurationFiles.entrypoint).standardizedFileURL,
                        path
                    )
                )
            }
        }
    }

    func testComposerRejectsMissingIncludedFile() throws {
        let root = temporaryRuntimeRoot(label: "missing-include")
        defer { try? FileManager.default.removeItem(at: root) }
        try writeConfigFile(
            root: root,
            filename: MacAppJSONConfigurationFiles.entrypoint,
            contents: #"{ "include": { "environment": "missing-environment.json" } }"#
        )

        XCTAssertThrowsError(
            try MacAppJSONConfigurationComposer()
                .loadDocument(entrypointURL: root.appendingPathComponent(MacAppJSONConfigurationFiles.entrypoint))
        ) { error in
            XCTAssertEqual(
                error as? MacAppJSONConfigurationError,
                .missingFile(root.appendingPathComponent("missing-environment.json").standardizedFileURL)
            )
        }
    }

    func testComposerRejectsSecretBearingKeys() throws {
        let root = temporaryRuntimeRoot(label: "secret")
        defer { try? FileManager.default.removeItem(at: root) }
        try writeConfigFile(
            root: root,
            filename: MacAppJSONConfigurationFiles.entrypoint,
            contents: #"{ "environment": { "webex": { "client_secret": "do-not-store" } } }"#
        )

        XCTAssertThrowsError(
            try MacAppJSONConfigurationComposer()
                .loadDocument(entrypointURL: root.appendingPathComponent(MacAppJSONConfigurationFiles.entrypoint))
        ) { error in
            XCTAssertEqual(
                error as? MacAppJSONConfigurationError,
                .secretKeyNotAllowed("environment.webex.client_secret")
            )
        }
    }

    func testComposerRejectsGenericAPIKeySecretBearingKeys() throws {
        let root = temporaryRuntimeRoot(label: "api-key-secret")
        defer { try? FileManager.default.removeItem(at: root) }
        try writeConfigFile(
            root: root,
            filename: MacAppJSONConfigurationFiles.entrypoint,
            contents: #"{ "connectors": { "webex": { "api_key": "do-not-store" } } }"#
        )

        XCTAssertThrowsError(
            try MacAppJSONConfigurationComposer()
                .loadDocument(entrypointURL: root.appendingPathComponent(MacAppJSONConfigurationFiles.entrypoint))
        ) { error in
            XCTAssertEqual(
                error as? MacAppJSONConfigurationError,
                .secretKeyNotAllowed("connectors.webex.api_key")
            )
        }
    }

    func testRuntimeConfigurationIgnoresComposedJSONWhenFeatureFlagIsOff() throws {
        let runtimeRoot = temporaryRuntimeRoot(label: "json-off")
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        try writeRuntimeConfigFile(
            root: runtimeRoot,
            contents: """
            {
              "environment": {
                "codex_executable": "/tmp/json-codex",
                "webex": {
                  "network_policy": {
                    "page_size": 25,
                    "retry_count": 1
                  }
                }
              }
            }
            """
        )

        let configuration = RuntimeConfiguration.resolved(environment: [
            "GETWEBEXSPACE_RUNTIME_ROOT": runtimeRoot.path
        ])

        XCTAssertEqual(configuration.webexPageSize, 100)
        XCTAssertEqual(configuration.webexRetryCount, 5)
        XCTAssertNotEqual(configuration.codexExecutable, "/tmp/json-codex")
    }

    func testRuntimeConfigurationAppliesComposedJSONWhenEnabled() throws {
        let runtimeRoot = temporaryRuntimeRoot(label: "json-on")
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        try writeRuntimeConfigFile(
            root: runtimeRoot,
            contents: """
            {
              "extends": ["base.json"],
              "environment": {
                "webex": {
                  "network_policy": {
                    "timeout_seconds": 9
                  }
                }
              }
            }
            """
        )
        try writeConfigFile(
            root: runtimeRoot.appendingPathComponent("config", isDirectory: true),
            filename: MacAppJSONConfigurationFiles.base,
            contents: """
            {
              "environment": {
                "codex_executable": "/tmp/json-codex",
                "webex": {
                  "api_base_url": "https://example.com/webex",
                  "oauth_token_file": "tokens/webex.json",
                  "oauth_refresh_skew_seconds": 60,
                  "oauth_refresh_token_skew_seconds": 120,
                  "network_policy": {
                    "page_size": 25,
                    "retry_count": 1,
                    "timeout_seconds": 20
                  },
                  "sync_policy": {
                    "concurrency_limit": 4,
                    "adaptive_active_interval_seconds": 10,
                    "adaptive_recent_interval_seconds": 30,
                    "adaptive_background_interval_seconds": 90,
                    "adaptive_jitter_percent": 5
                  }
                }
              }
            }
            """
        )

        let configuration = RuntimeConfiguration.resolved(environment: [
            "GETWEBEXSPACE_RUNTIME_ROOT": runtimeRoot.path,
            MacAppJSONConfigurationEnvironment.enabled: "true"
        ])

        XCTAssertEqual(configuration.webexBaseURL, URL(string: "https://example.com/webex")!)
        XCTAssertEqual(configuration.webexPageSize, 25)
        XCTAssertEqual(configuration.webexRetryCount, 1)
        XCTAssertEqual(configuration.webexTimeoutSeconds, 9)
        XCTAssertEqual(configuration.webexOAuthTokenPathOverride, "tokens/webex.json")
        XCTAssertEqual(configuration.webexOAuthRefreshSkewSeconds, 60)
        XCTAssertEqual(configuration.webexOAuthRefreshTokenSkewSeconds, 120)
        XCTAssertEqual(configuration.webexSyncConcurrencyLimit, 4)
        XCTAssertEqual(configuration.webexAdaptiveActiveIntervalSeconds, 10)
        XCTAssertEqual(configuration.webexAdaptiveRecentIntervalSeconds, 30)
        XCTAssertEqual(configuration.webexAdaptiveBackgroundIntervalSeconds, 90)
        XCTAssertEqual(configuration.webexAdaptiveJitterRatio, 0.05)
        XCTAssertEqual(configuration.codexExecutable, "/tmp/json-codex")
    }

    func testRuntimeConfigurationEnvOverridesComposedJSONWhenEnabled() throws {
        let runtimeRoot = temporaryRuntimeRoot(label: "json-env-wins")
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        try writeRuntimeConfigFile(
            root: runtimeRoot,
            contents: """
            {
              "environment": {
                "codex_executable": "/tmp/json-codex",
                "webex": {
                  "network_policy": {
                    "page_size": 25,
                    "retry_count": 1
                  }
                }
              }
            }
            """
        )

        let configuration = RuntimeConfiguration.resolved(environment: [
            "GETWEBEXSPACE_RUNTIME_ROOT": runtimeRoot.path,
            MacAppJSONConfigurationEnvironment.enabled: "on",
            "CODEX_BIN": "/tmp/env-codex",
            "WEBEX_API_PAGE_SIZE": "77"
        ])

        XCTAssertEqual(configuration.codexExecutable, "/tmp/env-codex")
        XCTAssertEqual(configuration.webexPageSize, 77)
        XCTAssertEqual(configuration.webexRetryCount, 1)
    }

    func testRuntimeConfigurationAppliesJSONRuntimeRootWithEnvPrecedence() throws {
        let configRoot = temporaryRuntimeRoot(label: "json-runtime-root-config")
        let jsonRuntimeRoot = temporaryRuntimeRoot(label: "json-runtime-root-json")
        let envRuntimeRoot = temporaryRuntimeRoot(label: "json-runtime-root-env")
        defer {
            try? FileManager.default.removeItem(at: configRoot)
            try? FileManager.default.removeItem(at: jsonRuntimeRoot)
            try? FileManager.default.removeItem(at: envRuntimeRoot)
        }
        let configDirectory = configRoot.appendingPathComponent("operator-config", isDirectory: true)
        try writeConfigFile(
            root: configDirectory,
            filename: MacAppJSONConfigurationFiles.entrypoint,
            contents: """
            {
              "environment": {
                "runtime_root": "\(jsonRuntimeRoot.path)"
              }
            }
            """
        )

        let jsonConfiguration = RuntimeConfiguration.resolved(environment: [
            MacAppJSONConfigurationEnvironment.enabled: "true",
            MacAppJSONConfigurationEnvironment.directory: configDirectory.path
        ])
        XCTAssertEqual(jsonConfiguration.runtimeRoot.standardizedFileURL, jsonRuntimeRoot.standardizedFileURL)
        XCTAssertEqual(jsonConfiguration.jsonConfigurationDirectory?.standardizedFileURL, configDirectory.standardizedFileURL)

        let envConfiguration = RuntimeConfiguration.resolved(environment: [
            "GETWEBEXSPACE_RUNTIME_ROOT": envRuntimeRoot.path,
            MacAppJSONConfigurationEnvironment.enabled: "true",
            MacAppJSONConfigurationEnvironment.directory: configDirectory.path
        ])
        XCTAssertEqual(envConfiguration.runtimeRoot.standardizedFileURL, envRuntimeRoot.standardizedFileURL)
        XCTAssertEqual(envConfiguration.jsonConfigurationDirectory?.standardizedFileURL, configDirectory.standardizedFileURL)
    }

    func testRuntimeConfigurationUsesBundledDefaultsWhenEnabledWithoutOperatorConfig() throws {
        let runtimeRoot = temporaryRuntimeRoot(label: "json-defaults-only")
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }

        let configuration = RuntimeConfiguration.resolved(environment: [
            "GETWEBEXSPACE_RUNTIME_ROOT": runtimeRoot.path,
            MacAppJSONConfigurationEnvironment.enabled: "true"
        ])

        XCTAssertEqual(configuration.webexBaseURL, URL(string: "https://webexapis.com/v1")!)
        XCTAssertEqual(configuration.webexPageSize, 100)
        XCTAssertEqual(configuration.webexRetryCount, 5)
        XCTAssertEqual(configuration.webexTimeoutSeconds, 20)
        XCTAssertEqual(configuration.webexSyncConcurrencyLimit, 3)
        XCTAssertEqual(configuration.jsonConfiguration?.testMode?.enabled, false)
        XCTAssertEqual(configuration.jsonConfiguration?.codex?.runPolicy?.timeoutSeconds, 120)
    }

    func testRuntimeConfigurationTreatsNullOperatorValuesAsDefaultFallbacks() throws {
        let runtimeRoot = temporaryRuntimeRoot(label: "json-null-defaults")
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        try writeRuntimeConfigFile(
            root: runtimeRoot,
            contents: """
            {
              "environment": {
                "codex_executable": null,
                "webex": {
                  "network_policy": {
                    "retry_count": null,
                    "timeout_seconds": 11
                  }
                }
              },
              "codex": {
                "run_policy": {
                  "max_attempts": null
                }
              }
            }
            """
        )

        let configuration = RuntimeConfiguration.resolved(environment: [
            "GETWEBEXSPACE_RUNTIME_ROOT": runtimeRoot.path,
            MacAppJSONConfigurationEnvironment.enabled: "true"
        ])

        XCTAssertEqual(configuration.webexRetryCount, 5)
        XCTAssertEqual(configuration.webexTimeoutSeconds, 11)
        XCTAssertEqual(configuration.jsonConfiguration?.codex?.runPolicy?.maxAttempts, 2)
    }

    func testRuntimeConfigurationAppliesConnectorWebexPolicyOverEnvironmentPolicy() throws {
        let runtimeRoot = temporaryRuntimeRoot(label: "json-connector-policy")
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        try writeRuntimeConfigFile(
            root: runtimeRoot,
            contents: """
            {
              "environment": {
                "webex": {
                  "network_policy": {
                    "page_size": 50,
                    "retry_count": 1,
                    "timeout_seconds": 30
                  },
                  "sync_policy": {
                    "concurrency_limit": 2,
                    "adaptive_jitter_percent": 5
                  }
                }
              },
              "connectors": {
                "webex": {
                  "network_policy": {
                    "retry_count": 4
                  },
                  "sync_policy": {
                    "concurrency_limit": 6
                  }
                }
              }
            }
            """
        )

        let configuration = RuntimeConfiguration.resolved(environment: [
            "GETWEBEXSPACE_RUNTIME_ROOT": runtimeRoot.path,
            MacAppJSONConfigurationEnvironment.enabled: "true"
        ])

        XCTAssertEqual(configuration.webexPageSize, 50)
        XCTAssertEqual(configuration.webexRetryCount, 4)
        XCTAssertEqual(configuration.webexTimeoutSeconds, 30)
        XCTAssertEqual(configuration.webexSyncConcurrencyLimit, 6)
        XCTAssertEqual(configuration.webexAdaptiveJitterRatio, 0.05)
    }

    func testRuntimeConfigurationConnectorEnabledListIsAuthoritative() throws {
        let runtimeRoot = temporaryRuntimeRoot(label: "json-connector-enabled")
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        try writeRuntimeConfigFile(
            root: runtimeRoot,
            contents: """
            {
              "connectors": {
                "enabled": ["imessage"],
                "webex": {
                  "enabled": true
                },
                "imessage": {
                  "enabled": false
                }
              }
            }
            """
        )

        let configuration = RuntimeConfiguration.resolved(environment: [
            "GETWEBEXSPACE_RUNTIME_ROOT": runtimeRoot.path,
            MacAppJSONConfigurationEnvironment.enabled: "true"
        ])

        XCTAssertEqual(configuration.jsonConfiguration?.connectors?.connectorEnabled("webex"), false)
        XCTAssertEqual(configuration.jsonConfiguration?.connectors?.connectorEnabled("imessage"), true)
    }

    private func decode<T: Decodable>(_ type: T.Type, from json: String) throws -> T {
        try decoder.decode(T.self, from: XCTUnwrap(json.data(using: .utf8)))
    }

    private func temporaryRuntimeRoot(label: String) -> URL {
        FileManager.default.temporaryDirectory
            .appendingPathComponent("Cubicle-\(label)-\(UUID().uuidString)", isDirectory: true)
    }

    private func writeRuntimeConfigFile(root: URL, contents: String) throws {
        try writeConfigFile(
            root: root.appendingPathComponent("config", isDirectory: true),
            filename: MacAppJSONConfigurationFiles.entrypoint,
            contents: contents
        )
    }

    private func writeConfigFile(root: URL, filename: String, contents: String) throws {
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        try contents.write(
            to: root.appendingPathComponent(filename),
            atomically: true,
            encoding: .utf8
        )
    }
}

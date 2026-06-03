import XCTest
@testable import GetWebexSpaceMacApp

final class MacAppJSONConfigurationDocumentsTests: XCTestCase {
    private let decoder = JSONDecoder()

    func testRuntimeDocumentDecodesWebexCodexAndIMessageSettings() throws {
        let json = """
        {
          "version": 1,
          "codex": {
            "executable": "/opt/homebrew/bin/codex"
          },
          "webex": {
            "api_base_url": "https://webexapis.com/v1",
            "page_size": 200,
            "retry_count": 4,
            "timeout_seconds": 30,
            "oauth_token_file": ".webex_oauth_tokens.json",
            "oauth_refresh_skew_seconds": 600,
            "oauth_refresh_token_skew_seconds": 172800,
            "public_webhook_url": "https://example.com/webex",
            "sync": {
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
        }
        """

        let document = try decode(MacAppJSONRuntimeDocument.self, from: json)

        XCTAssertEqual(document.version, 1)
        XCTAssertEqual(document.codex?.executable, "/opt/homebrew/bin/codex")
        XCTAssertEqual(document.webex?.apiBaseURL, "https://webexapis.com/v1")
        XCTAssertEqual(document.webex?.pageSize, 200)
        XCTAssertEqual(document.webex?.retryCount, 4)
        XCTAssertEqual(document.webex?.timeoutSeconds, 30)
        XCTAssertEqual(document.webex?.oauthTokenFile, ".webex_oauth_tokens.json")
        XCTAssertEqual(document.webex?.oauthRefreshSkewSeconds, 600)
        XCTAssertEqual(document.webex?.oauthRefreshTokenSkewSeconds, 172_800)
        XCTAssertEqual(document.webex?.publicWebhookURL, "https://example.com/webex")
        XCTAssertEqual(document.webex?.sync?.concurrencyLimit, 5)
        XCTAssertEqual(document.webex?.sync?.adaptiveActiveIntervalSeconds, 15)
        XCTAssertEqual(document.webex?.sync?.adaptiveRecentIntervalSeconds, 45)
        XCTAssertEqual(document.webex?.sync?.adaptiveBackgroundIntervalSeconds, 240)
        XCTAssertEqual(document.webex?.sync?.adaptiveJitterPercent, 10)
        XCTAssertEqual(document.imessage?.chatDatabasePath, "/Users/test/Library/Messages/chat.db")
        XCTAssertEqual(document.imessage?.busyTimeoutMilliseconds, 1500)
    }

    func testTargetsDocumentDecodesConfiguredGroups() throws {
        let json = """
        {
          "version": 1,
          "groups": {
            "important": [
              {
                "kind": "person",
                "label": "Pat Lee",
                "room_id": "Y2lzY29zcGFyazovL3VzL1JPT00vperson",
                "room_type": "direct",
                "email": "pat@example.com",
                "auto_reply": true,
                "imessage_handles": ["+14085550123", "pat@example.com"]
              },
              {
                "kind": "space",
                "label": "Launch Room",
                "room_id": "Y2lzY29zcGFyazovL3VzL1JPT00vc3BhY2U",
                "room_type": "group"
              }
            ],
            "executives": [
              {
                "kind": "person",
                "label": "Alex Exec",
                "room_id": "Y2lzY29zcGFyazovL3VzL1JPT00vexec",
                "email": "alex@example.com"
              }
            ],
            "beliefs": [
              {
                "kind": "space",
                "label": "Architecture",
                "room_id": "Y2lzY29zcGFyazovL3VzL1JPT00vYmVsaWVm"
              }
            ]
          }
        }
        """

        let document = try decode(MacAppJSONTargetsDocument.self, from: json)

        XCTAssertEqual(document.version, 1)
        XCTAssertEqual(document.groups?.important?.count, 2)
        XCTAssertEqual(document.groups?.important?.first?.kind, "person")
        XCTAssertEqual(document.groups?.important?.first?.autoReply, true)
        XCTAssertEqual(document.groups?.important?.first?.iMessageHandles, ["+14085550123", "pat@example.com"])
        XCTAssertEqual(document.groups?.executives?.first?.email, "alex@example.com")
        XCTAssertEqual(document.groups?.beliefs?.first?.label, "Architecture")
    }

    func testCodexDocumentDecodesRunCacheBeliefAndSynthesisPolicies() throws {
        let json = """
        {
          "version": 1,
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
        }
        """

        let document = try decode(MacAppJSONCodexDocument.self, from: json)

        XCTAssertEqual(document.version, 1)
        XCTAssertEqual(document.runPolicy?.timeoutSeconds, 180)
        XCTAssertEqual(document.runPolicy?.maxAttempts, 3)
        XCTAssertEqual(document.runPolicy?.retryDelaySeconds, 2.5)
        XCTAssertEqual(document.cachePolicy?.summaryMaxAgeSeconds, 600)
        XCTAssertEqual(document.cachePolicy?.execQuestionsMaxAgeSeconds, 900)
        XCTAssertEqual(document.beliefs?.staleHours, 12)
        XCTAssertEqual(document.beliefs?.evidenceChunkSize, 40)
        XCTAssertEqual(document.beliefs?.maxIncrementalWindowDays, 60)
        XCTAssertEqual(document.questionSynthesis?.seedCandidateLimit, 50)
        XCTAssertEqual(document.questionSynthesis?.queryHistoryLimit, 80)
        XCTAssertEqual(document.questionSynthesis?.promptHistoryLimit, 20)
        XCTAssertEqual(document.questionSynthesis?.candidateEvidenceLimit, 5)
        XCTAssertEqual(document.questionSynthesis?.outputLimit, 9)
    }

    func testQuestionGenerationDocumentDecodesCoreAndCubicleSettings() throws {
        let json = """
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
        }
        """

        let document = try decode(MacAppJSONQuestionGenerationDocument.self, from: json)

        XCTAssertEqual(document.version, 1)
        XCTAssertEqual(document.core?.privacy?.anonymizeUsers, false)
        XCTAssertEqual(document.core?.privacy?.redactURLs, true)
        XCTAssertEqual(document.core?.privacy?.redactEmails, true)
        XCTAssertEqual(document.core?.topics?.enabled, true)
        XCTAssertEqual(document.core?.topics?.numberOfTopics, 10)
        XCTAssertEqual(document.core?.topics?.minimumTopicSize, 2)
        XCTAssertEqual(document.core?.questions?.topN, 16)
        XCTAssertEqual(document.core?.questions?.enabledCategories, ["behavioral", "diagnostic", "network"])
        XCTAssertEqual(document.cubicle?.fallbackDraftLimit, 5)
        XCTAssertEqual(document.cubicle?.generatedQuestionLimit, 16)
        XCTAssertEqual(document.cubicle?.publishableQuestionLimit, 6)
        XCTAssertEqual(document.cubicle?.evidenceLimit, 5)
        XCTAssertEqual(document.cubicle?.coreEvidenceLimit, 6)
    }

    func testDocumentsCanBePartialAndIgnoreUnknownFutureFields() throws {
        let json = """
        {
          "version": 2,
          "unknown_root": true,
          "webex": {
            "page_size": 25,
            "unknown_webex": "ignored"
          }
        }
        """

        let document = try decode(MacAppJSONRuntimeDocument.self, from: json)

        XCTAssertEqual(document.version, 2)
        XCTAssertEqual(document.webex?.pageSize, 25)
        XCTAssertNil(document.codex)
        XCTAssertNil(document.webex?.apiBaseURL)
    }

    private func decode<T: Decodable>(_ type: T.Type, from json: String) throws -> T {
        try decoder.decode(T.self, from: XCTUnwrap(json.data(using: .utf8)))
    }
}

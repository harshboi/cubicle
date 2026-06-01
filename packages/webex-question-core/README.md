# WebexQuestionGeneratorCore

`WebexQuestionGeneratorCore` is a pure Swift package for importing Webex chat exports, analyzing local conversation patterns, and generating ranked analytical questions. It is designed to be consumed by a macOS SwiftUI app, but it contains no app UI, no SwiftUI views, and no persistence-specific code.

The package is local-first by default. Chat content is processed in memory with Foundation-only logic and is not sent to external services. Optional LLM support is exposed only as a protocol so an app can plug in its own provider if explicitly desired.

## Add To A SwiftUI macOS App

Add this package to your app in Xcode:

1. Open the app project.
2. Select **File > Add Package Dependencies**.
3. Add the local package path or repository URL for `WebexQuestionGeneratorCore`.
4. Link the `WebexQuestionGeneratorCore` library target to the app target.

Or add it to another Swift package:

```swift
.package(path: "../WebexQuestionGeneratorCore")
```

Then add it as a dependency:

```swift
.product(name: "WebexQuestionGeneratorCore", package: "WebexQuestionGeneratorCore")
```

## Basic Usage

```swift
import WebexQuestionGeneratorCore

let generator = WebexQuestionGenerator()

let result = try await generator.run(
    inputURL: webexExportURL,
    topN: 50
)

for question in result.questions {
    print(question.text)
    print(question.rationale)
}
```

You can also run the pipeline in stages:

```swift
let messages = try await generator.importMessages(from: webexExportURL)
let analysis = try await generator.analyze(messages: messages)
let questions = try await generator.generateQuestions(from: analysis, topN: 25)
```

## Supported Input Formats

- CSV
- JSON arrays, JSON objects with common message containers, and JSON Lines
- Plain text fallback

CSV imports support flexible automatic column mapping for common variants:

- Sender: `sender`, `from`, `author`, `person`
- Timestamp: `timestamp`, `time`, `created`, `date`
- Message text: `message`, `text`, `body`, `content`
- Space: `room`, `space`, `channel`
- Thread: `thread`, `parent`, `conversation_id`

Use `ColumnMapper.suggestions(for:)` when a UI needs to show automatic or ambiguous mappings before import.

## Privacy-First Design

The default configuration enables:

- User anonymization
- URL redaction
- Email redaction

No network calls are made by the deterministic pipeline. The optional LLM extension point is only this protocol:

```swift
public protocol LLMQuestionGenerating {
    func generateQuestions(from analysis: AnalysisResult) async throws -> [GeneratedQuestion]
}
```

No provider implementation is included.

## Generated Data Structures

The public API returns Codable, Sendable models suitable for app state, export, or later persistence by the host app:

- `Message`: normalized message content and extracted message-level features.
- `ConversationThread`: reconstructed thread metrics and participant/topic summaries.
- `AnalysisResult`: dataset summary, ranked topic/user/space metrics, outliers, contrasts, correlations, activity buckets, and network summary.
- `GeneratedQuestion`: ranked analytical question with rationale, supporting metrics, suggested next analysis, scores, and related dimensions.

## Analysis Pipeline

The package performs these local processing steps:

1. Import CSV, JSON, or text data.
2. Normalize whitespace and timestamps.
3. Apply privacy controls.
4. Extract mentions and question flags.
5. Compute message length, hour, day, sentiment, and topic labels.
6. Reconstruct threads using explicit thread IDs first, then reply relationships and time-window fallback.
7. Compute thread metrics, topic metrics, user metrics, space metrics, activity buckets, anomalies, contrasts, correlations, and network summaries.

Thread reconstruction prefers explicit `threadID`. If missing, it groups by reply relationships, same space, and the configured fallback window.

## Question Generation

Question generation is deterministic and template-based. It generates questions from:

- Metric and dimension combinations
- Topic findings
- Anomaly findings
- Contrast findings
- Correlation findings
- Network findings
- Configured objectives

Generated questions are categorized as:

- Descriptive
- Diagnostic
- Comparative
- Behavioral
- Network
- Efficiency
- Predictive

Ranking uses the configured scoring weights:

```swift
finalScore =
    interestingnessWeight * interestingnessScore +
    actionabilityWeight * actionabilityScore +
    confidenceWeight * confidenceScore
```

Scores are normalized by total configured weight. Defaults are:

- Interestingness: `0.45`
- Actionability: `0.35`
- Confidence: `0.20`

## Exports

Use `JSONExportService` for JSON output:

```swift
let analysisData = try JSONExportService().exportAnalysis(result.analysis)
let questionsData = try JSONExportService().exportQuestions(result.questions)
```

Use `MarkdownReportGenerator` for a readable report:

```swift
let markdown = MarkdownReportGenerator().generate(
    analysis: result.analysis,
    questions: result.questions
)
```

The Markdown report includes:

- Executive summary
- Dataset overview
- Top patterns
- Top generated questions
- Rationale and suggested next analysis
- Privacy note

## Configuration

```swift
var configuration = WebexQGConfiguration.default
configuration.privacy.anonymizeUsers = true
configuration.threading.fallbackWindowMinutes = 30
configuration.questions.topN = 50
configuration.objectives = [
    "improve team productivity",
    "identify communication bottlenecks",
    "find unresolved questions",
    "understand topic-level friction"
]

let generator = WebexQuestionGenerator(configuration: configuration)
```

## Limitations And TODOs

- Topic labels are lightweight term-frequency labels, not full probabilistic topic models.
- Sentiment is lexicon-based and intentionally simple for local-first behavior.
- Thread reconstruction is heuristic when exports do not include thread IDs or reply IDs.
- Network metrics are interaction summaries, not full graph-centrality algorithms.
- Predictive questions are generated as analysis prompts; the package does not train predictive models.
- Optional LLM generation is protocol-only and must be implemented by the host app if needed.

## Development

```bash
swift build
swift test
```

To print generated questions for a local export:

```bash
swift run WebexQGSmoke /path/to/webex-export.csv --top 10
```

Use `--no-anonymize` when testing with synthetic data and you want sender names preserved in printed output:

```bash
swift run WebexQGSmoke /tmp/webex-qg-sample.csv --top 5 --no-anonymize
```

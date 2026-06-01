#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

assert_pattern() {
  local pattern="$1"
  local file="$2"
  local description="$3"

  if rg -n --fixed-strings -- "$pattern" "$ROOT_DIR/$file" >/dev/null; then
    echo "PASS: $description"
  else
    echo "FAIL: $description" >&2
    echo "  missing pattern: $pattern" >&2
    echo "  file: $file" >&2
    exit 1
  fi
}

echo "RV-003 parity sweep: trigger frequency + update conditions"

# Belief maintenance trigger cadence and stale gate semantics.
assert_pattern "RefreshPlan(scope: .beliefMaintenance, cadenceSeconds: 300" "apps/cubicle-macos/Sources/Services/NativeRefreshCoordinator.swift" "belief maintenance cadence is 300s"
assert_pattern "staleHours: Int = 24" "apps/cubicle-macos/Sources/Services/CodexPromptOrchestration.swift" "belief stale gate defaults to 24h"
assert_pattern "reason: .evidenceChanged" "apps/cubicle-macos/Sources/Services/CodexPromptOrchestration.swift" "belief update triggers on evidence hash changes"
assert_pattern "if staleAt <= now" "apps/cubicle-macos/Sources/Services/CodexPromptOrchestration.swift" "belief update triggers when stale threshold is crossed"

# Clustering refresh cadence and cache-reuse/update conditions.
assert_pattern "RefreshPlan(scope: .personFocus, cadenceSeconds: settings.personFocusRefreshMinutes * 60" "apps/cubicle-macos/Sources/Services/NativeRefreshCoordinator.swift" "person-focus cadence follows settings"
assert_pattern "RefreshPlan(scope: .spaceFocus, cadenceSeconds: settings.spaceFocusRefreshMinutes * 60" "apps/cubicle-macos/Sources/Services/NativeRefreshCoordinator.swift" "space-focus cadence follows settings"
assert_pattern "canReuseByFingerprint(" "apps/cubicle-macos/Sources/Services/NativeRuntimeStore.swift" "cluster refresh checks fingerprint reuse gate"
assert_pattern "canReuseBySignature(" "apps/cubicle-macos/Sources/Services/NativeRuntimeStore.swift" "cluster refresh checks signature reuse gate"

# Exec questions generation conditions.
assert_pattern "let promptVersion = CodexPromptVersionRegistry.spaceFocusExecQuestions" "apps/cubicle-macos/Sources/Services/CodexPromptOrchestration.swift" "exec questions use canonical prompt version"
assert_pattern "let participantEmails = Set(participantNameByEmail.keys)" "apps/cubicle-macos/Sources/Services/CodexPromptOrchestration.swift" "exec questions derive room participant set"
assert_pattern "return participantEmails.contains(normalized) ? exec : nil" "apps/cubicle-macos/Sources/Services/CodexPromptOrchestration.swift" "exec questions require important-exec intersection"
assert_pattern "guard !matchedExecutives.isEmpty else" "apps/cubicle-macos/Sources/Services/CodexPromptOrchestration.swift" "exec question section is skipped when no matched execs"

# Space summary and So-what behavior.
assert_pattern "let promptVersion = CodexPromptVersionRegistry.spaceFocusSummary" "apps/cubicle-macos/Sources/Services/CodexPromptOrchestration.swift" "space summary uses canonical prompt version"
assert_pattern "topics: Array(parsed.topics.prefix(5))" "apps/cubicle-macos/Sources/Services/CodexPromptOrchestration.swift" "space summary enforces top-5 topic semantics"
assert_pattern "case soWhat = \"so_what\"" "apps/cubicle-macos/Sources/Services/CodexPromptOrchestration.swift" "summary decoder reads snake_case so_what"
assert_pattern "case soWhatCamel = \"soWhat\"" "apps/cubicle-macos/Sources/Services/CodexPromptOrchestration.swift" "summary decoder reads camelCase soWhat"
assert_pattern "QuestionCandidateService(" "apps/cubicle-macos/Sources/Services/NativeRefreshCoordinator.swift" "question engine is part of native refresh orchestration"
assert_pattern "ScopedQuestionsCard(questions: scopedQuestions)" "apps/cubicle-macos/Sources/Views/DetailView.swift" "focus detail replaces top-level meaningful topics with scoped questions"
assert_pattern "Based on what changed, these are the questions worth asking now." "apps/cubicle-macos/Sources/Views/QuestionsView.swift" "native Questions surface explains the question engine purpose"

echo "RV-003 parity sweep: PASS"

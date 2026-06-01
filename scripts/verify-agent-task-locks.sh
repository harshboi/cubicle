#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TASKS_FILE="${AGENT_TASKS_FILE:-$ROOT_DIR/AGENT_TASKS.md}"
PROGRESS_FILE="${IMPLEMENTATION_PROGRESS_FILE:-$ROOT_DIR/IMPLEMENTATION_PROGRESS.md}"

fail() {
  echo "verify-agent-task-locks: $*" >&2
  exit 1
}

require_file() {
  local file_path="$1"
  [[ -f "$file_path" ]] || fail "required file not found: $file_path"
}

count_heading_occurrences() {
  local heading="$1"
  awk -v heading="$heading" 'BEGIN { count = 0 } $0 == heading { count++ } END { print count }' "$TASKS_FILE"
}

extract_section_rows() {
  local section_heading="$1"
  awk -v section_heading="$section_heading" '
    $0 == section_heading { in_section = 1; next }
    in_section && /^## / { exit }
    in_section && /^\|/ {
      line = $0
      if (line ~ /^\|[[:space:]]*---/) next
      if (line ~ /^\|[[:space:]]*(Owner|Priority|Role|Completed At)[[:space:]]*\|/) next
      print line
    }
  ' "$TASKS_FILE"
}

count_section_rows() {
  local section_heading="$1"
  extract_section_rows "$section_heading" | sed '/^[[:space:]]*$/d' | wc -l | tr -d ' '
}

validate_section_statuses() {
  local section_heading="$1"
  local expected_status="$2"

  extract_section_rows "$section_heading" | awk -v section_heading="$section_heading" -v expected_status="$expected_status" '
    function trim(value) {
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
      return value
    }

    {
      columns = split($0, parts, "|")
      if (columns < 3) {
        printf("verify-agent-task-locks: malformed table row in %s: %s\n", section_heading, $0) > "/dev/stderr"
        errors = 1
        next
      }

      status = tolower(trim(parts[columns - 1]))
      if (status != tolower(expected_status)) {
        printf("verify-agent-task-locks: unexpected status in %s: %s (expected %s)\n", section_heading, status, expected_status) > "/dev/stderr"
        errors = 1
      }
    }

    END {
      if (errors) exit 1
    }
  '
}

parse_progress_counts() {
  local snapshot_line
  snapshot_line="$(grep -E "^- Board counts \(live snapshot\):" "$PROGRESS_FILE" | tail -n 1 || true)"
  [[ -n "$snapshot_line" ]] || fail "missing board counts snapshot line in $PROGRESS_FILE"

  if [[ "$snapshot_line" =~ Active[[:space:]]Locks[[:space:]]([0-9]+),[[:space:]]Ready[[:space:]]([0-9]+),[[:space:]]Blocked[[:space:]]([0-9]+),[[:space:]]Done[[:space:]]([0-9]+) ]]; then
    printf "%s %s %s %s\n" "${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}" "${BASH_REMATCH[3]}" "${BASH_REMATCH[4]}"
    return 0
  fi

  fail "unable to parse board counts from snapshot line: $snapshot_line"
}

main() {
  require_file "$TASKS_FILE"
  require_file "$PROGRESS_FILE"

  local headings=(
    "## Active Locks"
    "## Ready Tasks"
    "## Blocked / External Dependency Tasks"
    "## Done Tasks"
  )

  for heading in "${headings[@]}"; do
    local count
    count="$(count_heading_occurrences "$heading")"
    [[ "$count" == "1" ]] || fail "expected heading '$heading' exactly once in $TASKS_FILE; found $count"
  done

  validate_section_statuses "## Active Locks" "in_progress"
  validate_section_statuses "## Ready Tasks" "ready"
  validate_section_statuses "## Blocked / External Dependency Tasks" "blocked"

  local active_count ready_count blocked_count done_count
  active_count="$(count_section_rows "## Active Locks")"
  ready_count="$(count_section_rows "## Ready Tasks")"
  blocked_count="$(count_section_rows "## Blocked / External Dependency Tasks")"
  done_count="$(count_section_rows "## Done Tasks")"

  local progress_active progress_ready progress_blocked progress_done
  read -r progress_active progress_ready progress_blocked progress_done < <(parse_progress_counts)

  [[ "$active_count" == "$progress_active" ]] || fail "Active Locks count mismatch: AGENT_TASKS=$active_count IMPLEMENTATION_PROGRESS=$progress_active"
  [[ "$ready_count" == "$progress_ready" ]] || fail "Ready count mismatch: AGENT_TASKS=$ready_count IMPLEMENTATION_PROGRESS=$progress_ready"
  [[ "$blocked_count" == "$progress_blocked" ]] || fail "Blocked count mismatch: AGENT_TASKS=$blocked_count IMPLEMENTATION_PROGRESS=$progress_blocked"
  [[ "$done_count" == "$progress_done" ]] || fail "Done count mismatch: AGENT_TASKS=$done_count IMPLEMENTATION_PROGRESS=$progress_done"

  echo "verify-agent-task-locks: ok (Active=$active_count Ready=$ready_count Blocked=$blocked_count Done=$done_count)"
}

main "$@"

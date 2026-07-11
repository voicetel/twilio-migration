#!/usr/bin/env bash
set -euo pipefail

MODE="--enforce"
CHANGED_ONLY="0"
REPORT_ONLY="0"
for arg in "$@"; do
  case "$arg" in
    --enforce) MODE="--enforce" ;;
    --changed-only) CHANGED_ONLY="1" ;;
    --report) REPORT_ONLY="1" ;;
  esac
done

ROOT="${CLAUDE_PROJECT_DIR:-$(pwd)}"
LOCAL_MCP="$ROOT/.claude/local/mcp"
STATUS="$LOCAL_MCP/status.md"
TS="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
mkdir -p "$LOCAL_MCP"
cd "$ROOT"

# --- Temporary coverage ratchet (installed 2026-07-10) ---
# The required coverage floor ramps linearly from RATCHET_START_PCT to
# RATCHET_TARGET_PCT over RATCHET_DAYS days, then holds at the target.
# Override any of these via environment to retune without editing the script.
RATCHET_START_DATE="${GO_COV_RATCHET_START_DATE:-2026-07-10}"
RATCHET_START_PCT="${GO_COV_RATCHET_START_PCT:-67.9}"
RATCHET_TARGET_PCT="${GO_COV_RATCHET_TARGET_PCT:-100}"
RATCHET_DAYS="${GO_COV_RATCHET_DAYS:-7}"

report_failure() {
  message="$1"
  {
    printf '\n## Quality gate failed\n\n'
    printf -- '- Timestamp: %s\n' "$TS"
    printf -- '- Reason: %s\n' "$message"
  } >> "$STATUS"
  printf 'Go quality gate failed: %s\n' "$message" >&2
  if [ "$REPORT_ONLY" = "1" ]; then
    exit 0
  fi
  exit 2
}

required_floor() {
  # Prints the required coverage floor for today given the ratchet config.
  start_epoch="$(date -u -d "$RATCHET_START_DATE" +%s 2>/dev/null || printf '')"
  today_epoch="$(date -u -d "$(date -u +%Y-%m-%d)" +%s 2>/dev/null || printf '')"
  if [ -z "$start_epoch" ] || [ -z "$today_epoch" ]; then
    # Cannot compute dates; fall back to the target floor (safest).
    printf '%s' "$RATCHET_TARGET_PCT"
    return 0
  fi
  days_elapsed=$(( (today_epoch - start_epoch) / 86400 ))
  awk -v s="$RATCHET_START_PCT" -v t="$RATCHET_TARGET_PCT" -v d="$RATCHET_DAYS" -v el="$days_elapsed" 'BEGIN {
    if (el < 0) el = 0
    if (d <= 0) { printf "%.4f", t; exit }
    if (el > d) el = d
    r = s + (t - s) * el / d
    if (r > t) r = t
    if (r < s) r = s
    printf "%.4f", r
  }'
}

ratchet_day() {
  start_epoch="$(date -u -d "$RATCHET_START_DATE" +%s 2>/dev/null || printf '')"
  today_epoch="$(date -u -d "$(date -u +%Y-%m-%d)" +%s 2>/dev/null || printf '')"
  if [ -z "$start_epoch" ] || [ -z "$today_epoch" ]; then
    printf 'unknown'
    return 0
  fi
  d=$(( (today_epoch - start_epoch) / 86400 ))
  [ "$d" -lt 0 ] && d=0
  [ "$d" -gt "$RATCHET_DAYS" ] && d="$RATCHET_DAYS"
  printf '%s' "$d"
}

if [ ! -f go.mod ]; then
  report_failure "go.mod not found at repository root"
fi

if [ "$CHANGED_ONLY" = "1" ]; then
  changed_relevant_files="$(git status --short -- '*.go' 'go.mod' 'go.sum' 'go.work' 'Makefile' 'Taskfile*' '.github/workflows/*' 'README*' 'CONTRIBUTING*' 'ARCHITECTURE*' 'SECURITY*' 'DESIGN*' 'ADR*' 'CHANGELOG*' 'docs/**' 'doc/**' 2>/dev/null || true)"
  if [ -z "$changed_relevant_files" ]; then
    printf 'Go quality gate skipped: no Go-relevant changes detected.\n'
    exit 0
  fi
fi

printf 'Running Go quality gate...\n' >&2

if ! go fmt ./...; then
  report_failure "go fmt ./... failed"
fi

if ! go vet ./...; then
  report_failure "go vet ./... failed"
fi

if [ -n "${GO_LINT_COMMAND:-}" ]; then
  if ! sh -c "$GO_LINT_COMMAND"; then
    report_failure "custom GO_LINT_COMMAND failed: $GO_LINT_COMMAND"
  fi
elif [ -f Makefile ] && grep -q '^lint:' Makefile; then
  if ! make lint; then
    report_failure "make lint failed"
  fi
elif command -v golangci-lint >/dev/null 2>&1; then
  if ! golangci-lint run ./...; then
    report_failure "golangci-lint run ./... failed"
  fi
elif command -v staticcheck >/dev/null 2>&1; then
  if ! staticcheck ./...; then
    report_failure "staticcheck ./... failed"
  fi
else
  report_failure "no lint command found; install golangci-lint, install staticcheck, add Makefile lint target, or set GO_LINT_COMMAND"
fi

if ! go test -race -covermode=atomic -coverprofile=coverage.out ./...; then
  report_failure "go test -race -covermode=atomic -coverprofile=coverage.out ./... failed"
fi

coverage="$(go tool cover -func=coverage.out | awk '/^total:/ {gsub(/%/, "", $3); print $3}')"
floor="$(required_floor)"
day="$(ratchet_day)"
printf 'Total coverage: %s%% (ratchet floor today: %s%%, day %s/%s)\n' "$coverage" "$floor" "$day" "$RATCHET_DAYS" >&2

if ! awk -v coverage="$coverage" -v floor="$floor" 'BEGIN { exit !(coverage + 1e-9 >= floor) }'; then
  report_failure "coverage ${coverage}% is below ratchet floor ${floor}% (day ${day}/${RATCHET_DAYS}, ramping ${RATCHET_START_PCT}%->${RATCHET_TARGET_PCT}%)"
fi

{
  printf '\n## Quality gate passed\n\n'
  printf -- '- Timestamp: %s\n' "$TS"
  printf -- '- go fmt ./...: pass\n'
  printf -- '- lint: pass\n'
  printf -- '- go vet ./...: pass\n'
  printf -- '- go test -race -covermode=atomic -coverprofile=coverage.out ./...: pass\n'
  printf -- '- coverage: %s%% (ratchet floor %s%%, day %s/%s)\n' "$coverage" "$floor" "$day" "$RATCHET_DAYS"
} >> "$STATUS"

printf 'Go quality gate passed with %s%% coverage (floor %s%%).\n' "$coverage" "$floor" >&2

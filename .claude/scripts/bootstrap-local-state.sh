#!/usr/bin/env bash
set -euo pipefail

MODE="--bootstrap"
STRICT="0"
for arg in "$@"; do
  case "$arg" in
    --bootstrap|--verify|--prompt|--task-created|--shutdown) MODE="$arg" ;;
    --strict) STRICT="1" ;;
  esac
done

ROOT="${CLAUDE_PROJECT_DIR:-$(pwd)}"
LOCAL_DIR="$ROOT/.claude/local"
LOCAL_MCP="$LOCAL_DIR/mcp"
MIGRATED="$LOCAL_MCP/migrated"
EXCLUDE="$ROOT/.git/info/exclude"
HEALTH="$LOCAL_MCP/bootstrap-health.json"
LOG="$LOCAL_MCP/bootstrap.log"
TS="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

mkdir -p "$LOCAL_MCP" "$MIGRATED" "$ROOT/.claude/scripts" "$ROOT/.claude/commands" "$ROOT/.claude/rules"
touch "$LOG"

log() {
  printf '%s %s\n' "$TS" "$*" >> "$LOG"
}

write_health() {
  status="$1"
  message="$2"
  cat > "$HEALTH" <<EOF
{
  "status": "$status",
  "timestamp": "$TS",
  "mode": "$MODE",
  "message": "$message"
}
EOF
}

block_or_warn() {
  message="$1"
  write_health "blocked" "$message"
  log "blocked: $message"
  printf 'Local Go MCP bootstrap blocked: %s\n' "$message" >&2
  if [ "$STRICT" = "1" ]; then
    exit 2
  fi
  exit 0
}

repair_note() {
  message="$1"
  log "repair: $message"
  printf 'Repair: %s\n' "$message" >&2
}

ensure_file() {
  file="$1"
  title="$2"
  if [ ! -f "$file" ]; then
    mkdir -p "$(dirname "$file")"
    printf '# %s\n\n' "$title" > "$file"
    repair_note "created ${file#$ROOT/}"
  fi
}

ensure_exclude() {
  if [ ! -d "$ROOT/.git" ]; then
    block_or_warn "repository root has no .git directory; cannot maintain .git/info/exclude"
  fi
  mkdir -p "$(dirname "$EXCLUDE")"
  touch "$EXCLUDE"
  for pattern in \
    "CLAUDE.local.md" \
    ".claude/settings.local.json" \
    ".claude/local/" \
    ".mcp.local.json" \
    "coverage.out" \
    "coverage.html"; do
    if ! grep -qxF "$pattern" "$EXCLUDE"; then
      printf '%s\n' "$pattern" >> "$EXCLUDE"
      repair_note "added $pattern to .git/info/exclude"
    fi
  done
}

append_migrated_file() {
  source_file="$1"
  target_file="$2"
  source_root="$3"
  rel="${source_file#$source_root/}"
  {
    printf '\n\n### %s\n\n' "$rel"
    if [ ! -s "$source_file" ]; then
      printf '_Empty file._\n'
    elif LC_ALL=C grep -Iq . "$source_file"; then
      cat "$source_file"
      printf '\n'
    else
      printf '_Binary or non-text file preserved in migration archive._\n'
    fi
  } >> "$target_file"
}

migrate_dir() {
  source_dir="$1"
  target_file="$2"
  label="$3"

  if [ ! -d "$source_dir" ]; then
    return 0
  fi

  case "$source_dir" in
    "$LOCAL_MCP") return 0 ;;
    "$LOCAL_MCP/"*) ;;
  esac

  safe_label="$(printf '%s' "$label" | tr '/ .' '___')"
  archive="$MIGRATED/${TS}-${safe_label}.tar.gz"

  if ! tar -czf "$archive" -C "$(dirname "$source_dir")" "$(basename "$source_dir")"; then
    block_or_warn "failed to archive legacy directory ${source_dir#$ROOT/}"
  fi

  {
    printf '\n\n## Migrated from %s\n\n' "${source_dir#$ROOT/}"
    printf -- '- Migrated at: %s\n' "$TS"
    printf -- '- Archive: %s\n\n' "${archive#$ROOT/}"
  } >> "$target_file"

  while IFS= read -r file; do
    append_migrated_file "$file" "$target_file" "$source_dir"
  done < <(find "$source_dir" -type f | sort)

  if ! rm -rf "$source_dir"; then
    block_or_warn "failed to remove migrated legacy directory ${source_dir#$ROOT/}"
  fi

  repair_note "migrated and removed legacy directory ${source_dir#$ROOT/}"
}

find_docs() {
  find "$ROOT" \
    -path "$ROOT/.git" -prune -o \
    -path "$ROOT/.claude/local" -prune -o \
    -path "$ROOT/vendor" -prune -o \
    \( \
      -iname 'README*' -o \
      -iname 'CONTRIBUTING*' -o \
      -iname 'ARCHITECTURE*' -o \
      -iname 'SECURITY*' -o \
      -iname 'DESIGN*' -o \
      -iname 'ADR*' -o \
      -iname 'CHANGELOG*' -o \
      -path '*/docs/*' -o \
      -path '*/doc/*' -o \
      -name 'Makefile' -o \
      -name 'Taskfile*' -o \
      -name 'go.mod' -o \
      -name 'go.work' -o \
      -path '*/.github/workflows/*' \
    \) -type f -print | sort
}

rebuild_docs() {
  docs_index="$LOCAL_MCP/docs-index.md"
  docs_full="$LOCAL_MCP/repository-docs.md"
  tmp_docs="$LOCAL_MCP/.docs-files.tmp"

  find_docs > "$tmp_docs"

  {
    printf '# Repository Documentation Index\n\n'
    printf 'Generated: %s\n\n' "$TS"
    while IFS= read -r file; do
      printf -- '- %s\n' "${file#$ROOT/}"
    done < "$tmp_docs"
  } > "$docs_index"

  {
    printf '# Repository Documentation Snapshot\n\n'
    printf 'Generated: %s\n\n' "$TS"
    printf 'This file is generated from repository documentation files for local Claude/MCP context.\n\n'
    while IFS= read -r file; do
      rel="${file#$ROOT/}"
      printf '\n\n---\n\n## %s\n\n' "$rel"
      if [ ! -s "$file" ]; then
        printf '_Empty file._\n'
      elif LC_ALL=C grep -Iq . "$file"; then
        cat "$file"
        printf '\n'
      else
        printf '_Binary or non-text documentation file omitted from text snapshot._\n'
      fi
    done < "$tmp_docs"
  } > "$docs_full"

  rm -f "$tmp_docs"
}

write_default_task_policy() {
  task_policy="$LOCAL_MCP/task-policy.md"
  if [ ! -f "$task_policy" ]; then
    cat > "$task_policy" <<'EOF'
# Task Policy

- Use TaskCreate, TaskUpdate, and TaskList for every non-trivial task.
- Use TodoWrite only if Task tools are unavailable.
- Keep task status in the task facility as the source of truth.
- Mirror durable task state and blockers into .claude/local/mcp/status.md.
- Do not mark a task complete until bootstrap and quality gates pass.
EOF
    repair_note "created .claude/local/mcp/task-policy.md"
  fi
}

write_default_directives() {
  directives="$LOCAL_MCP/directives.md"
  if [ ! -s "$directives" ]; then
    cat > "$directives" <<'EOF'
# Local Directives

- Always run the full Go quality process after changes.
- Always enforce the coverage ratchet unless a user-approved exception is documented.
- Never bypass fmt, lint, vet, tests, race detection, or coverage.
- Never assume missing requirements; research first, then ask interactively.
- Load work into TaskCreate/TaskUpdate/TaskList when available.
EOF
    repair_note "initialized local directives"
  fi
}

write_default_status() {
  status="$LOCAL_MCP/status.md"
  if [ ! -s "$status" ]; then
    cat > "$status" <<'EOF'
# Local Status

## Current task

- Task: None
- State: idle
- Blockers: none

## Last quality run

- Timestamp: not run
- go fmt: unknown
- lint: unknown
- go vet: unknown
- go test: unknown
- coverage: unknown
- exception: none
EOF
    repair_note "initialized local status"
  fi
}

write_default_memory() {
  memory="$LOCAL_MCP/memory.md"
  if [ ! -s "$memory" ]; then
    cat > "$memory" <<EOF
# Local Memory

Durable local facts Claude should remember for this repository.

## Project facts

- Module: github.com/voicetel/twilio-migration

## Decisions

- 2026-07-10: Temporary coverage ratchet installed. Floor ramps from 70.5% to 100% over 7 days, then holds at 100%.

## Testing notes

- Coverage floor is enforced by .claude/scripts/quality-gate.sh (ratchet-aware).
EOF
    repair_note "initialized local memory"
  fi
}

ensure_core_state() {
  # Populate the files that have dedicated default writers FIRST. Each writer
  # guards on the file being empty/absent, so this is idempotent and will not
  # clobber user edits. (Calling ensure_file first would create a non-empty
  # heading stub and cause the writers to skip, leaving empty state files.)
  write_default_directives
  write_default_status
  write_default_memory
  write_default_task_policy
  # Files without dedicated writers get a heading stub as a fallback. docs-index
  # and repository-docs are regenerated wholesale by rebuild_docs afterwards.
  ensure_file "$LOCAL_MCP/docs-index.md" "Repository Documentation Index"
  ensure_file "$LOCAL_MCP/repository-docs.md" "Repository Documentation Snapshot"
  ensure_file "$LOCAL_MCP/session-log.md" "Session Log"
}

migrate_legacy_state() {
  migrate_dir "$ROOT/directives" "$LOCAL_MCP/directives.md" "directives-root"
  migrate_dir "$ROOT/.directives" "$LOCAL_MCP/directives.md" "directives-dotroot"
  migrate_dir "$ROOT/.claude/directives" "$LOCAL_MCP/directives.md" "directives-claude"
  migrate_dir "$ROOT/.mcp/directives" "$LOCAL_MCP/directives.md" "directives-mcp"
  migrate_dir "$LOCAL_MCP/directives" "$LOCAL_MCP/directives.md" "directives-local-mcp"

  migrate_dir "$ROOT/memory" "$LOCAL_MCP/memory.md" "memory-root"
  migrate_dir "$ROOT/memories" "$LOCAL_MCP/memory.md" "memories-root"
  migrate_dir "$ROOT/.memory" "$LOCAL_MCP/memory.md" "memory-dotroot"
  migrate_dir "$ROOT/.memories" "$LOCAL_MCP/memory.md" "memories-dotroot"
  migrate_dir "$ROOT/.claude/memory" "$LOCAL_MCP/memory.md" "memory-claude"
  migrate_dir "$ROOT/.claude/memories" "$LOCAL_MCP/memory.md" "memories-claude"
  migrate_dir "$ROOT/.mcp/memory" "$LOCAL_MCP/memory.md" "memory-mcp"
  migrate_dir "$ROOT/.mcp/memories" "$LOCAL_MCP/memory.md" "memories-mcp"
  migrate_dir "$LOCAL_MCP/memory" "$LOCAL_MCP/memory.md" "memory-local-mcp"
  migrate_dir "$LOCAL_MCP/memories" "$LOCAL_MCP/memory.md" "memories-local-mcp"

  migrate_dir "$ROOT/status" "$LOCAL_MCP/status.md" "status-root"
  migrate_dir "$ROOT/.status" "$LOCAL_MCP/status.md" "status-dotroot"
  migrate_dir "$ROOT/.claude/status" "$LOCAL_MCP/status.md" "status-claude"
  migrate_dir "$ROOT/.mcp/status" "$LOCAL_MCP/status.md" "status-mcp"
  migrate_dir "$LOCAL_MCP/status" "$LOCAL_MCP/status.md" "status-local-mcp"
}

verify_project() {
  if [ ! -d "$ROOT/.git" ]; then
    block_or_warn "repository root has no .git directory; cannot maintain .git/info/exclude"
  fi

  if [ ! -f "$ROOT/go.mod" ]; then
    block_or_warn "go.mod is missing at the repository root; this Go template requires a root Go module"
  fi

  if ! command -v go >/dev/null 2>&1; then
    block_or_warn "Go toolchain is not available on PATH"
  fi

  for script in bootstrap-local-state.sh startup-context.sh prompt-context.sh shutdown-persist.sh quality-gate.sh; do
    if [ -f "$ROOT/.claude/scripts/$script" ]; then
      chmod +x "$ROOT/.claude/scripts/$script" 2>/dev/null || true
    fi
  done
}

ensure_core_state
ensure_exclude
migrate_legacy_state
rebuild_docs
verify_project
write_health "functional" "Local Go MCP bootstrap is fully functional"
log "functional: bootstrap completed in mode $MODE"

printf 'Local Go MCP bootstrap status: functional\n'
printf 'Local MCP state: .claude/local/mcp\n'
printf 'Documentation index: .claude/local/mcp/docs-index.md\n'
printf 'Repository docs snapshot: .claude/local/mcp/repository-docs.md\n'

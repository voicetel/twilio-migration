#!/usr/bin/env bash
set -euo pipefail

ROOT="${CLAUDE_PROJECT_DIR:-$(pwd)}"
LOCAL_MCP="$ROOT/.claude/local/mcp"
EXCLUDE="$ROOT/.git/info/exclude"
TS="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

bash "$ROOT/.claude/scripts/bootstrap-local-state.sh" --shutdown || true

mkdir -p "$LOCAL_MCP" "$(dirname "$EXCLUDE")"
touch "$LOCAL_MCP/session-log.md" "$EXCLUDE"

for pattern in \
  "CLAUDE.local.md" \
  ".claude/settings.local.json" \
  ".claude/local/" \
  ".mcp.local.json" \
  "coverage.out" \
  "coverage.html"; do
  grep -qxF "$pattern" "$EXCLUDE" || printf '%s\n' "$pattern" >> "$EXCLUDE"
done

{
  printf '\n## Session end\n'
  printf -- '- Timestamp: %s\n' "$TS"
  printf -- '- Git branch: %s\n' "$(git -C "$ROOT" rev-parse --abbrev-ref HEAD 2>/dev/null || printf unknown)"
  printf -- '- Git status: %s changed paths\n' "$(git -C "$ROOT" status --short 2>/dev/null | wc -l | tr -d ' ')"
  printf -- '- Bootstrap health: %s\n' "$(sed -n 's/.*"status": "\([^"]*\)".*/\1/p' "$LOCAL_MCP/bootstrap-health.json" 2>/dev/null | head -n 1)"
} >> "$LOCAL_MCP/session-log.md"

printf 'Local Go MCP state persisted.\n'

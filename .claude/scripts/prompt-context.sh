#!/usr/bin/env bash
set -euo pipefail

ROOT="${CLAUDE_PROJECT_DIR:-$(pwd)}"
LOCAL_MCP="$ROOT/.claude/local/mcp"

if ! bash "$ROOT/.claude/scripts/bootstrap-local-state.sh" --prompt --strict; then
  printf 'Local Go MCP bootstrap failed. Repair the bootstrap issue before processing this prompt.\n' >&2
  exit 2
fi

printf '## Prompt Context: Local Go MCP\n'
printf 'Bootstrap status: functional. Confirm this in the response when the task is complete.\n'
printf 'For all non-trivial prompts, create or update tasks with TaskCreate/TaskUpdate/TaskList. Use TodoWrite only as fallback.\n'
printf 'All prompts are interactive: research first, then ask focused questions if code/docs/tests do not answer required details.\n'
printf 'Do not claim completion until bootstrap health is functional and Go quality gates pass.\n\n'

printf '## Current Local Status\n'
sed -n '1,160p' "$LOCAL_MCP/status.md"

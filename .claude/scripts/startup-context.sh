#!/usr/bin/env bash
set -euo pipefail

ROOT="${CLAUDE_PROJECT_DIR:-$(pwd)}"
LOCAL_MCP="$ROOT/.claude/local/mcp"

bash "$ROOT/.claude/scripts/bootstrap-local-state.sh" --bootstrap

printf '\n## Bootstrap Health\n'
sed -n '1,120p' "$LOCAL_MCP/bootstrap-health.json"

printf '\n\n## Local MCP Directives\n'
sed -n '1,240p' "$LOCAL_MCP/directives.md"

printf '\n\n## Local MCP Status\n'
sed -n '1,240p' "$LOCAL_MCP/status.md"

printf '\n\n## Local MCP Memory\n'
sed -n '1,240p' "$LOCAL_MCP/memory.md"

printf '\n\n## Task Policy\n'
sed -n '1,200p' "$LOCAL_MCP/task-policy.md"

printf '\n\n## Documentation Index\n'
sed -n '1,300p' "$LOCAL_MCP/docs-index.md"

printf '\n\n## Startup Directive\n'
printf 'Acknowledge bootstrap as fully functional only after checking bootstrap-health.json. For non-trivial work, create/update TaskCreate/TaskUpdate/TaskList tasks before editing. Read relevant repository documentation before implementation.\n'

# Go Bootstrap Health

You are verifying the local Go MCP bootstrap.

Interactive requirements:

1. Run `bash .claude/scripts/bootstrap-local-state.sh --verify --strict`.
2. Read `.claude/local/mcp/bootstrap-health.json`.
3. Confirm `.git/info/exclude` contains all local Claude/MCP private paths.
4. Confirm legacy directives/memory/memories/status directories were migrated and removed.
5. Confirm docs index and repository documentation snapshot were regenerated.
6. Confirm task policy exists.
7. If a repair is possible, perform it and rerun verification.
8. Ask an interactive question only for non-repairable issues.

Output:

- Fully functional, repaired, or blocked.
- Files repaired.
- Legacy migrations performed.
- Remaining interactive questions, if any.

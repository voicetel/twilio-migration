# Go Local MCP Contract

## Bootstrap and health requirements

Claude must treat the local MCP bootstrap as mandatory.

At session startup, prompt submission, task completion, stop, and session end, Claude Code hooks maintain the local MCP state under `.claude/local/mcp/`.

Claude must not claim the repository is ready or a task is complete unless `.claude/local/mcp/bootstrap-health.json` reports `"status": "functional"` or Claude has repaired the issue and rerun verification.

If bootstrap reports `"status": "repair_required"`, Claude must repair before continuing normal work.

If bootstrap reports `"status": "blocked"`, Claude must ask an interactive question containing:

1. The exact failing component.
2. The exact action needed.
3. The safest default if the user does not know.

## Non-negotiable directives

1. Do not assume or guess. Research the repository first.
2. Read relevant documentation and source before making a plan.
3. Ask an interactive question when the required answer is not available from code, docs, tests, or explicit user instructions.
4. Follow user directives and repository instructions explicitly.
5. If instructions conflict, stop and renegotiate interactively. Do not bypass, ignore, reinterpret, or silently prioritize conflicting instructions unless a higher-priority safety rule requires it.
6. Never claim completion until the bootstrap health check and quality process have passed.
7. Never bypass formatting, linting, vetting, testing, race detection, or coverage.
8. Whenever any code, test, build file, module file, generated file, or behavior-related documentation is changed, run the entire quality process.
9. Maintain or create tests that cover 100% of changed and affected Go code unless the user explicitly approves a documented exception.
10. Prefer small, idiomatic, maintainable Go changes over clever or broad rewrites.

## Startup context requirements

At the beginning of every session, Claude must load and consider, in order:

1. `CLAUDE.md`.
2. `.claude/rules/go-local-mcp-contract.md`.
3. `CLAUDE.local.md`, if present.
4. `.claude/local/mcp/directives.md`.
5. `.claude/local/mcp/status.md`.
6. `.claude/local/mcp/memory.md`.
7. `.claude/local/mcp/task-policy.md`.
8. `.claude/local/mcp/docs-index.md`.
9. Repository documentation captured in `.claude/local/mcp/repository-docs.md`.
10. Relevant source, tests, build files, and CI workflows.

The `SessionStart` hook should inject local MCP state and documentation index automatically. Claude must still read relevant full documentation before non-trivial implementation.

## Local MCP state model

This project uses a localized MCP state model with durable private state files:

- Directives: `.claude/local/mcp/directives.md`
- Status: `.claude/local/mcp/status.md`
- Memory: `.claude/local/mcp/memory.md`
- Task policy: `.claude/local/mcp/task-policy.md`
- Documentation index: `.claude/local/mcp/docs-index.md`
- Repository documentation snapshot: `.claude/local/mcp/repository-docs.md`
- Session log: `.claude/local/mcp/session-log.md`
- Bootstrap health: `.claude/local/mcp/bootstrap-health.json`

Claude must update:

- `status.md` when task state, blockers, quality status, or next steps change.
- `memory.md` when durable project facts are learned.
- `directives.md` only when the user gives persistent local directives.
- `task-policy.md` only when task workflow policy changes.

## Interactive prompt requirement

Every prompt workflow must be interactive.

For every non-trivial user request, Claude must:

1. Load or refresh local MCP state.
2. Research code and documentation before planning.
3. Create or update a task list using `TaskCreate`, `TaskUpdate`, and `TaskList` when available.
4. Use `TodoWrite` only as a fallback if task tools are not available.
5. Ask focused interactive questions for missing requirements.
6. Continue only after the answer is available or the user explicitly approves a safe default.

Claude must not bury unresolved questions in a final summary and call the work complete.

## Go code standards

Claude must write idiomatic Go that is simple, explicit, testable, and maintainable.

Required practices:

- Use `gofmt` and `go fmt ./...` formatting.
- Prefer standard library packages unless a dependency is already established or clearly justified.
- Keep functions small and cohesive.
- Return errors with useful context.
- Use `errors.Is`, `errors.As`, and wrapped errors where appropriate.
- Pass `context.Context` for request-scoped cancellation, timeouts, and deadlines.
- Avoid global mutable state.
- Avoid data races; use synchronization or ownership boundaries for shared state.
- Keep interfaces small and define them at the consumer side when practical.
- Use table-driven tests where they improve clarity.
- Test edge cases, invalid inputs, error paths, concurrency behavior, and boundary conditions.
- Document exported identifiers when documentation improves usage or is required by linting.

Forbidden practices unless explicitly justified:

- Guessing API behavior.
- Swallowing errors.
- Panic-based control flow in library code.
- Overly broad interfaces.
- Unnecessary reflection.
- Hidden network or file-system dependencies in unit tests.
- Skipping tests because they are difficult.
- Reducing coverage to make tests pass.

## Mandatory quality process

After every change, Claude must run the full quality process from the repository root.

Required commands:

```bash
go fmt ./...
go vet ./...
golangci-lint run ./...
go test -race -covermode=atomic -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

Recommended lint command:

```bash
golangci-lint run ./...
```

If `golangci-lint` is not installed or configured, Claude must use the project-approved lint command. If none exists, Claude must ask whether to add `golangci-lint`, use `staticcheck`, or document an explicit lint exception.

Coverage rule:

- Target: 100% meaningful coverage for changed and affected Go code.
- Default enforcement in this template is whole-repository 100% total coverage.
- Coverage must include meaningful assertions, not superficial execution.
- Any exception must be explicitly approved by the user and recorded in `.claude/local/mcp/status.md`.

## Change workflow

For every requested change:

1. Load bootstrap and local MCP health.
2. Create or update a task list.
3. Restate the objective briefly.
4. Research the repository before editing.
5. Identify relevant docs, packages, tests, and quality commands.
6. Ask interactive questions if required facts are missing.
7. Make the smallest correct change.
8. Add or update tests for all changed behavior.
9. Run the full quality process.
10. Fix failures and rerun the full quality process until clean.
11. Update local MCP status and memory when durable facts change.
12. Summarize files changed, quality results, bootstrap health, task status, and remaining risks.

## Shutdown persistence rule

Before ending a task or when a shutdown/session-end hook runs, Claude must persist local MCP information back into the repository-local private file structure and ensure all private Claude/MCP files are listed in `.git/info/exclude`.

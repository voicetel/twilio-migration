# Go Quality Review

You are verifying whether a Go change is complete.

Interactive requirements:

1. Load local MCP state and bootstrap health.
2. Load current task list with TaskList when available.
3. Inspect changed files and affected packages.
4. Run or verify:
   - `go fmt ./...`
   - lint command
   - `go vet ./...`
   - `go test -race -covermode=atomic -coverprofile=coverage.out ./...`
   - `go tool cover -func=coverage.out`
5. Confirm coverage meets today's ratchet floor for changed and affected code.
6. If any gate fails, update tasks/status and repair. Do not claim completion.
7. Ask an interactive question only when a user-approved exception is needed.

Output:

- Bootstrap health.
- Task status.
- Files reviewed.
- Quality command results.
- Coverage result (and today's ratchet floor).
- Repairs made or blockers.

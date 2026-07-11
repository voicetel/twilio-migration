# Claude Project Instructions for twilio-migration

@.claude/rules/go-local-mcp-contract.md

## Project identity

- Language: Go
- Module: `github.com/voicetel/twilio-migration`
- Primary package/binary: `cmd/twilio-migration`
- Owner/team: `voicetel`

## Repository-specific notes

Add stable project facts here. Do not add secrets, credentials, private hostnames, or developer-specific preferences.

- Quality gate enforces a **temporary coverage ratchet** (owner decision, 2026-07-10; baseline corrected same day). The required floor ramps linearly from the true install-time coverage of **67.9%** (fresh `go test -race -covermode=atomic ./...` run, not the earlier stale-`cov.out`-derived 70.5% figure) to **100%** over **7 days** (≈+4.59%/day), then stays at 100% permanently. The `Stop` and `TaskCompleted` hooks block completion when total coverage is below that day's floor. Constants are env-overridable: `GO_COV_RATCHET_START_DATE` (2026-07-10), `GO_COV_RATCHET_START_PCT` (67.9), `GO_COV_RATCHET_TARGET_PCT` (100), `GO_COV_RATCHET_DAYS` (7). Any exception must be recorded in `.claude/local/mcp/status.md`.
- Coverage profile written by the gate is `coverage.out`; the existing `make cover` target uses `cov.out`. Both are git-ignored.
- Security scanning is handled by `.github/workflows/security-scan.yml` (gosec advisory + govulncheck fatal) plus `codeql.yml` and `dependency-submission.yml`.

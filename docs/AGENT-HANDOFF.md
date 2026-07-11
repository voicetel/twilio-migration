# Agent handoff — twilio-migration

Status report for an agent picking up the remaining migration work. Read this
top to bottom, then work the remaining-work task list (TaskList; G1-G7,
formerly tracked in `docs/PARITY-GAPS.md`, now in the Claude Code task
system).

Last updated by the previous session after shipping the coverage gate.

---

## What this tool is

A CLI that copies **account configuration** from a Twilio account to a VoiceML
account. It **reads** from Twilio with the official
[`twilio-go`](https://github.com/twilio/twilio-go) SDK and **writes** to VoiceML
with the official [`voiceml-go-sdk`](https://github.com/voicetel/voiceml-go-sdk)
(`github.com/voicetel/voiceml-go-sdk`). VoiceML's REST API is Twilio-compatible,
so field shapes line up.

It does **not** migrate historical usage records (call/message logs) — those are
Twilio Bulk Export's domain and are not importable configuration.

- Module: `github.com/voicetel/twilio-migration`
- Repo: `github.com/voicetel/twilio-migration` (PUBLIC)
- Go: 1.25+
- Version policy: the tool version (`internal/version.Version`) tracks the
  VoiceML OpenAPI/SDK version; a test fails the build if it drifts from the
  linked `voiceml-go-sdk` `Version`. Currently **0.9.2**.

## Architecture

```
Twilio ──(twilio-go: read)──▶ migrate.Migrator ──(voiceml-go-sdk: write)──▶ VoiceML
```

- `internal/config` — resolve credentials from env, else prompt (injectable
  `Prompter`; secrets read no-echo).
- `internal/migrate` — the `Migrator` interface, the runner, one file per
  resource, and the **coverage gate** (`coverage.go` + `coverage_test.go`).
- `cmd/twilio-migration` — CLI (`--dry-run`, `--only`, `--coverage`,
  `--version`, `--yes`, `--voiceml-base-url`).

`internal/migrate/clients.go` holds `Clients{ Twilio *twapi.ApiService,
TwilioMessaging *twmsg.ApiService, TwilioVoice *twvoice.ApiService, VoiceML
*voiceml.Client }`. Add new source sub-clients here if a resource lives in
yet another twilio-go package.

## Current status (implemented ✅)

| Migrator name | File | Notes |
|---|---|---|
| `phone-numbers` | `phonenumbers.go` | idempotent by number |
| `applications` | `applications.go` | idempotent by friendly name |
| `sip-trunking` | `sip.go` | domains, cred lists (+ credentials), IP ACLs (+ IPs), domain↔list/ACL mappings; mappings re-pointed by friendly name |
| `messaging` | `messaging.go` | Messaging Services (messaging/v1 sub-client) |
| `queues` | `queues.go` | idempotent by friendly name |
| `ip-records` | `iprecords.go` | Voice v1, idempotent by IP address; produces SIDs `source-ip-mappings` (G4) will need |
| `connection-policies` | `connectionpolicies.go` | Voice v1, policies idempotent by friendly name, nested Targets idempotent by SIP URI; produces SIDs `byoc-trunks` (G3) will need |
| `byoc-trunks` | `byoctrunks.go` | Voice v1, idempotent by friendly name; re-points `ConnectionPolicySid`/`FromDomainSid` at already-migrated VoiceML SIDs (bridged via friendly name / domain name); runs after `connection-policies` and `sip-trunking` |
| `source-ip-mappings` | `sourceipmappings.go` | Voice v1, idempotent by the resolved (IP record, SIP domain) pair; re-points `IpRecordSid`/`SipDomainSid` (bridged via IP address / domain name); runs after `ip-records` and `sip-trunking` |
| coverage gate | `coverage.go`, `coverage_test.go` | `Inventory()` = authoritative status list; test fails on drift |

- All four packages (`cmd/twilio-migration`, `internal/config`,
  `internal/migrate`, `internal/version`) are at literal 100% statement
  coverage (`go test -race -covermode=atomic`), including the `Migrate()` /
  `NewClients` SDK-wiring adapters — via local httptest/fake-transport doubles
  (`wiring_test.go`, `clients_test.go`) and two test-only seams in
  `clients.go` (`SetTestTwilioTransport`, `SetTestVoiceMLClientFactory`). No
  coverage exceptions remain; the quality gate's ratchet target is 100%.
- CI green: gosec, govulncheck, CodeQL, dependency-submission; Dependabot on.

Run `twilio-migration --coverage` to see the live status of every resource.

## Remaining work

The full, prioritized gap list — with build order, cross-reference notes, and
**verified** twilio-go + voiceml-go-sdk signatures for each resource (G1-G7) —
is tracked in the Claude Code task system (TaskList). Work it top to bottom
(G3 is blocked on G2, G4 is blocked on G1); it is mirrored by
`internal/migrate.Inventory()` (the coverage test fails the build if they
drift).

---

## The pattern — how to add a migrator

Each resource is one file with (1) a thin `Migrator` type, (2) narrow
reader/writer interfaces so the core logic is unit-tested against fakes, and
(3) a `migrateX(ctx, src, dst, opts)` function. Copy `queues.go` as the
template. Skeleton:

```go
type xSource interface {
    ListX(params *twapi.ListXParams) ([]twapi.ApiV2010X, error) // twilio-go
}
type xDest interface {
    List(ctx context.Context, params voiceml.ListPageParams) (*voiceml.XList, error)
    Create(ctx context.Context, params voiceml.CreateXParams) (*voiceml.X, error)
}

type X struct{}
func (X) Name() string { return "x-name" }
func (X) Migrate(ctx context.Context, c *Clients, opts Options) (Result, error) {
    return migrateX(ctx, c.Twilio /*or c.VoiceV1 etc*/, c.VoiceML.SomeService, opts)
}

func migrateX(ctx context.Context, src xSource, dst xDest, opts Options) (Result, error) {
    res := Result{Resource: "x-name"}
    items, err := src.ListX(&twapi.ListXParams{})
    if err != nil { return res, fmt.Errorf("list Twilio x: %w", err) }
    existing, err := dst.List(ctx, voiceml.ListPageParams{})
    if err != nil { return res, fmt.Errorf("list VoiceML x: %w", err) }
    have := map[string]bool{}
    for _, e := range existing.Items { have[e.Key] = true }
    for _, it := range items {
        key := deref(it.Key)               // twilio-go fields are *string — use deref()
        r := ItemResult{ID: key}
        switch {
        case key == "":        r.Status, r.Detail = StatusFailed, "no key"
        case have[key]:        r.Status, r.Detail = StatusSkipped, "already present on VoiceML"
        case opts.DryRun:      r.Status = StatusPlanned
        default:
            if _, e := dst.Create(ctx, voiceml.CreateXParams{...}); e != nil {
                r.Status, r.Detail = StatusFailed, e.Error()
            } else { r.Status = StatusCreated }
        }
        res.Items = append(res.Items, r)
    }
    return res, nil
}
```

Then:
1. Register `X{}` in `migrate.Default()` (`migrate.go`) **in dependency order**.
2. Flip the resource's row in `Inventory()` (`coverage.go`) from `CovRoadmap`
   to `CovMigrated` and drop its `Reason`. The coverage test enforces this.
   Mark the matching G-task completed in the Claude Code task system.
3. Add table-driven tests against fakes (`x_test.go`): create / skip-existing /
   dry-run / create-error / list-errors. See `queues_test.go` or
   `iprecords_test.go`.
4. Wire `Migrate()` into `internal/migrate/wiring_test.go`'s
   `newWiringTestClients` if it needs a new `*Clients` field (see
   `TwilioVoice`) — the quality gate's ratchet target is literal 100%
   coverage, no exceptions, so a missing wire-up there will panic on a nil
   sub-client rather than just under-cover.

### Non-negotiable rules

- **Never copy or store secrets.** Twilio does not expose SIP credential
  passwords (or Assistant BYO-LLM keys). Generate a new one with
  `generatePassword()` (`password.go`) and REPORT it (set `ItemResult.Detail`,
  status `StatusCreated`; the CLI surfaces created-item details). Never write a
  password to disk.
- **twilio-go list-item SIDs and most fields are `*string`** — always `deref()`.
- **Idempotency**: list the destination first, skip by a stable natural key
  (number / friendly name / IP / domain name), never by SID.
- **Cross-migrator SID remap**: a resource that references another must re-point
  the reference at the **new** VoiceML SID. The Twilio side gives the OLD SID;
  resolve it to a natural key on the Twilio side, then look up the NEW VoiceML
  SID by that key via the destination `List`. `sip.go`
  (`credListByName` / `aclByName` / `domainBySID`) is the worked example.
- **Dry-run must not write.** Every create path checks `opts.DryRun`.
- Run migrators that produce SIDs others need EARLIER in `Default()`.

---

## Quality gate & repo conventions (mandatory before every push)

```sh
gofmt -w . && test -z "$(gofmt -l .)"
go vet ./...
go build ./...
go test -race ./...          # coverage_test.go MUST stay green
twilio-migration --coverage  # sanity-check the matrix
```

- Add/keep tests for changed behavior; the migrate package should stay high.
- **Commit hygiene (this is a PUBLIC repo — strict):** NO AI/agent attribution
  anywhere — no `Co-authored-by`, no "Generated with", no tool names in commit
  messages / PR text / comments. NO GPG signing (`--no-gpg-sign`). Verify before
  every push:
  ```sh
  git log @{u}..HEAD --format='%B' | \
    grep -iE 'co-authored-by:|generated with (claude|cursor|copilot|gpt|ai)|🤖|anthropic' \
    && echo FAIL || echo OK
  ```
- **When you add a migrator, flip its `Inventory()` row** — the coverage test
  will fail otherwise, which is the point (no silent gaps).
- Versioning: keep `internal/version.Version` == the linked `voiceml-go-sdk`
  `Version`. Bumping the SDK dep requires bumping the const + re-tagging.
  Caveat: the Go module proxy immutably pinned `v0.9.2` to the initial commit;
  cut a new tag (e.g. `v0.9.3`) for a fresh installable release rather than
  moving `v0.9.2`.

## Key files

- `internal/migrate/migrate.go` — `Migrator`, `Default()`, `Run`, `Result`/`ItemStatus`.
- `internal/migrate/clients.go` — source/dest client wiring (add sub-clients here).
- `internal/migrate/coverage.go` — `Inventory()` (authoritative status list).
- `internal/migrate/coverage_test.go` — the gate.
- `internal/migrate/{phonenumbers,applications,sip,messaging,queues}.go` — worked examples (`sip.go` for cross-ref remap).
- `internal/migrate/password.go` — `generatePassword()` (never store secrets).
- `cmd/twilio-migration/main.go` — CLI, report printing, coverage/version flags.
- Remaining-work checklist (G1-G7): Claude Code task system (TaskList).
- `README.md` — user-facing docs + the ⚠️ "credentials are never copied" section.

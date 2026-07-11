# twilio-migration

Migrate account **configuration** from Twilio to [VoiceML](https://voiceml.voicetel.com).

`twilio-migration` reads your configuration from a Twilio account with the
official [`twilio-go`](https://github.com/twilio/twilio-go) SDK and recreates it
in a VoiceML account with the official
[`voiceml-go-sdk`](https://github.com/voicetel/voiceml-go-sdk). VoiceML's REST
API is Twilio-compatible, so resources map across with the same field shapes.

## ⚠️ Credentials are never copied or stored

This tool **does not copy, read, or store secrets it cannot get back from
Twilio.** Twilio's API doesn't return them, so there is nothing to copy:

- **SIP credential passwords** — the tool creates each username on VoiceML
  with a **brand-new, randomly generated password** and prints it once so you
  can redistribute it to the affected devices. **Registered devices will not
  re-authenticate until they receive the new password.**
- **Conversations push-notification Credentials** (APN certificate/key, GCM/FCM
  keys) and **Assistants' SegmentCredential** (analytics API key/write key) —
  these can't be regenerated the way SIP passwords can (they're issued by
  Apple/Google/Segment, not this tool), so those sub-resources are simply
  **not migrated**. Everything else on the parent resource still is.

No password or secret — original or generated — is ever written to disk by
this tool.

## What it migrates (and what it doesn't)

This tool migrates **configuration** — the resources you set up in the console:

| Resource                        | Migrator name         | Status         |
|----------------------------------|------------------------|----------------|
| Phone numbers                     | `phone-numbers`        | ✅ implemented |
| TwiML applications                 | `applications`         | ✅ implemented |
| SIP trunking                       | `sip-trunking`         | ✅ implemented — domains, credential lists (+ credentials¹), IP ACLs (+ IP addresses), and domain↔list / domain↔ACL mappings |
| Messaging services                  | `messaging`            | ✅ implemented |
| Queues                              | `queues`               | ✅ implemented |
| IP Records                          | `ip-records`           | ✅ implemented |
| Connection Policies (+ targets)     | `connection-policies`  | ✅ implemented |
| BYOC trunks                         | `byoc-trunks`          | ✅ implemented — re-points Connection Policy / SIP Domain references at their migrated VoiceML equivalents |
| Source IP Mappings                  | `source-ip-mappings`   | ✅ implemented — re-points IP Record / SIP Domain references at their migrated VoiceML equivalents |
| Conversations (config only)²        | `conversations`        | ✅ implemented — Services, default-scope Roles/Users/Conversations (+ Participants, Messages, Webhooks), Config Addresses, account Configuration |
| Assistants (config only)³           | `assistants`           | ✅ implemented — Assistants, standalone Tools/Knowledge + their attachments |
| Outgoing Caller IDs                 | —                       | ❌ unmigratable — created only via phone validation (`CreateValidationRequest`); no direct create exists on Twilio or VoiceML, so each number must be re-verified interactively |
| SIP Inbound Region                  | —                       | ❌ not exposed by the VoiceML Go SDK |
| Dialing Permissions                  | —                       | ❌ unmigratable — VoiceML exposes only one inheritance toggle, not the per-country/prefix allow/deny list Twilio actually models; copying just the toggle would misrepresent the resource as migrated |

¹ Credentials get freshly generated passwords — see the section above.
² Excludes push-notification Credentials (see above), each named Service's own
nested Roles/Users/Conversations (default-scope only), and `MessagingServiceSid`
cross-refs (left unset).
³ Excludes SegmentCredential (see above), Policies (VoiceML has no write
endpoint for them), and Sessions/Messages/Feedback (the only write endpoint is
a live LLM-execution call, not a data-import one — technically impossible to
migrate, not a scope choice).

**Coverage is gated, not documented-by-hand.** `internal/migrate.Inventory()` is
the authoritative list of every resource and its status; a build-failing test
(`coverage_test.go`) asserts it stays consistent with the registered migrators,
so a resource can never be silently dropped. Print the live matrix any time:

```sh
twilio-migration --coverage
```

It does **not** migrate historical usage records (call/message logs). Twilio's
[Bulk Export](https://www.twilio.com/docs/usage/bulkexport) covers those —
Messages, Calls, Conferences, Participants — but they are activity *logs*, not
importable configuration, so they are out of scope here.

> **Note on caveats surfaced during migration.** Some Twilio fields have no
> VoiceML equivalent on create (e.g. a phone number's SMS URL, friendly name and
> status-callback are not part of VoiceML's create body today). Those are mapped
> where possible and reported; see each migrator's doc comment.

## Install

```sh
go install github.com/voicetel/twilio-migration/cmd/twilio-migration@latest
# or, from a clone:
make build      # produces ./twilio-migration
```

Requires Go 1.25+.

## Usage

Credentials are read from the environment, or prompted for interactively
(the auth tokens are read without echo when stdin is a TTY):

```sh
export TWILIO_ACCOUNT_SID=ACxxxxxxxx
export TWILIO_AUTH_TOKEN=xxxxxxxx
export VOICEML_ACCOUNT_SID=ACyyyyyyyy
export VOICEML_AUTH_TOKEN=yyyyyyyy
```

**Always preview first with `--dry-run`:**

```sh
twilio-migration --dry-run
```

Then run for real (you'll be asked to confirm):

```sh
twilio-migration
```

### Flags

| Flag                 | Meaning                                                        |
|----------------------|----------------------------------------------------------------|
| `--dry-run`          | Report what would be migrated without writing to VoiceML.      |
| `--only a,b`         | Restrict to specific migrators (default: all).                 |
| `--voiceml-base-url` | Override the VoiceML API host (default `voiceml.voicetel.com`).|
| `--yes`              | Skip the confirmation prompt before writing.                   |

```sh
twilio-migration --only phone-numbers --dry-run
```

## Safety

- **Idempotent.** A resource already present on VoiceML is skipped (phone
  numbers by number, applications by friendly name), so re-running is safe.
- **Dry-run first.** `--dry-run` performs no writes.
- **No credential storage.** Credentials come from env/prompt and are never
  written to disk. `.env` files are git-ignored.

## Architecture

```
Twilio  ──(twilio-go: read)──▶  migrate.Migrator  ──(voiceml-go-sdk: write)──▶  VoiceML
```

- `internal/config` — resolve credentials from env, else prompt (injectable
  `Prompter` for testing).
- `internal/migrate` — the `Migrator` interface, the runner, and one file per
  resource. Each resource's core logic takes small reader/writer interfaces so
  it is unit-tested against fakes (no network).

### Adding a resource

1. Add `internal/migrate/<resource>.go` with a `type X struct{}` implementing
   `Migrator` (a thin wrapper) plus a `migrateX(ctx, src, dst, opts)` function
   over narrow interfaces.
2. Add `X{}` to `migrate.Default()` and flip its row in
   `internal/migrate.Inventory()` (`coverage.go`) to `CovMigrated`.
3. Add table-driven tests against fakes, and wire the new source sub-client
   into `wiring_test.go`'s `newWiringTestClients` if it's a new one. See
   `docs/AGENT-HANDOFF.md` for the full pattern and non-negotiable rules.

## Testing

```sh
make cover      # go test -race + coverage
```

The whole repo — `cmd/twilio-migration`, `internal/config`, `internal/migrate`,
`internal/version` — is held to **literal 100% statement coverage**
(`go test -race -covermode=atomic`), including `NewClients` and every one-line
`Migrate` wrapper: those are exercised against local `httptest`/fake-transport
doubles (`wiring_test.go`, `clients_test.go`), never live endpoints, so the
suite has no network dependency.

## Versioning

`twilio-migration` is versioned to **match the VoiceML OpenAPI version** — the
same version the [`voiceml-go-sdk`](https://github.com/voicetel/voiceml-go-sdk)
is generated from and tagged with (currently **0.9.2**). Because the migration
mappings target a specific VoiceML API surface, tying the tool's tag to that
surface makes upgrades mechanical:

1. Bump the `voiceml-go-sdk` dependency (`go get github.com/voicetel/voiceml-go-sdk@vX.Y.Z`).
2. Set `internal/version.Version` to the same `X.Y.Z`.
3. Review/extend the migrators for any new or changed resources.
4. `git tag vX.Y.Z`.

A unit test (`internal/version`) **fails the build if the tool version drifts
from the linked SDK's `Version`**, so a dependency bump can't silently ship
against a mismatched API surface. Check what a build targets with:

```sh
twilio-migration --version
# twilio-migration 0.9.2 (targets VoiceML OpenAPI 0.9.2; linked voiceml-go-sdk 0.9.2)
```

## License

MIT with Commons Clause Restriction — see [LICENSE](LICENSE).

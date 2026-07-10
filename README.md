# twilio-migration

Migrate account **configuration** from Twilio to [VoiceML](https://voiceml.voicetel.com).

`twilio-migration` reads your configuration from a Twilio account with the
official [`twilio-go`](https://github.com/twilio/twilio-go) SDK and recreates it
in a VoiceML account with the official
[`voiceml-go-sdk`](https://github.com/voicetel/voiceml-go-sdk). VoiceML's REST
API is Twilio-compatible, so resources map across with the same field shapes.

## ⚠️ Credentials are never copied or stored

This tool **does not copy, read, or store SIP credential passwords.** Twilio
does not expose a credential's password over its API, so there is nothing to
copy. When migrating SIP credentials, the tool creates each username on VoiceML
with a **brand-new, randomly generated password** and prints it once so you can
redistribute it to the affected devices. **Registered devices will not
re-authenticate until they receive the new password.** No password — original or
generated — is ever written to disk by this tool.

## What it migrates (and what it doesn't)

This tool migrates **configuration** — the resources you set up in the console:

| Resource                  | Migrator name   | Status         |
|---------------------------|-----------------|----------------|
| Phone numbers             | `phone-numbers` | ✅ implemented |
| TwiML applications        | `applications`  | ✅ implemented |
| SIP trunking              | `sip-trunking`  | ✅ implemented — domains, credential lists (+ credentials¹), IP ACLs (+ IP addresses), and domain↔list / domain↔ACL mappings |
| Messaging services        | `messaging`     | ✅ implemented |
| Queues                    | `queues`        | ✅ implemented |
| BYOC trunks, Connection Policies (+targets), IP Records, Source IP Mappings | — | 🟡 roadmap (SDK-supported) |
| Dialing Permissions, Conversations, Assistants | — | 🟡 roadmap |
| Outgoing Caller IDs       | —               | ❌ unmigratable — created only via phone validation (`CreateValidationRequest`); no direct create exists on Twilio or VoiceML, so each number must be re-verified interactively |
| SIP Inbound Region        | —               | ❌ not exposed by the VoiceML Go SDK |

¹ Credentials get freshly generated passwords — see the section above.

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
2. Add `X{}` to `migrate.Default()`.
3. Add table-driven tests against fakes.

## Testing

```sh
make cover      # go test -race + coverage
```

Pure logic (config, the runner, and every `migrateX` function) is unit-tested
against fakes. `NewClients` and the one-line `Migrate` wrappers are thin
SDK-wiring adapters that only talk to live endpoints; they're exercised by real
runs (and, later, an optional `httptest` integration test) rather than mocked.

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

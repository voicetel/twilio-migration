# Twilio parity gaps — task checklist

Every VoiceML config resource this tool does **not** yet migrate. These are the
Twilio→VoiceML parity gaps detected while building the tool. Work them top to
bottom (the order respects cross-resource dependencies). Read
[`AGENT-HANDOFF.md`](AGENT-HANDOFF.md) first for the migrator pattern and rules.

This file is the authoritative task list; `internal/migrate.Inventory()` is the
machine-checked mirror (the coverage test fails the build if they drift). When
you finish a gap: implement the migrator, flip its `Inventory()` row to
`CovMigrated`, and check its box here.

Legend: **roadmap** = migratable, not built yet · **unmigratable** = cannot be
copied by any tool (documented reason).

---

## Build order (roadmap — do these in sequence)

Cross-references mean some resources must be created before others (a reference
must be re-pointed at the *new* VoiceML SID). Order:

- [ ] **G1 · `ip-records`** (roadmap) — standalone. Produces SIDs G4 needs.
  - Read `src.VoiceV1.ListIpRecord`; write `dst.VoiceML.VoiceV1.CreateIpRecord`.
  - `CreateVoiceV1IpRecordParams{IpAddress string, FriendlyName *string, CidrPrefixLength *int}`.
  - Idempotent by IP address.

- [ ] **G2 · `connection-policies`** (+ targets) (roadmap) — produces SIDs G3 needs.
  - Two-level. Policy: `ListConnectionPolicy` → `CreateConnectionPolicy{FriendlyName *string}`, idempotent by friendly name, track old→new SID.
  - Targets: `ListConnectionPolicyTarget(policySid)` → `CreateConnectionPolicyTarget(newPolicySid, {Target string, FriendlyName *string, Priority,Weight *int, Enabled *bool})`.

- [ ] **G3 · `byoc-trunks`** (roadmap) — references a connection policy + a SIP domain.
  - `ListByocTrunk` → `CreateByocTrunk{FriendlyName,VoiceURL,...,ConnectionPolicySid,FromDomainSid *string}`.
  - Remap `ConnectionPolicySid` (via G2's name→SID map) and `FromDomainSid` (via VoiceML SIP domain list, by domain name). Idempotent by friendly name.

- [ ] **G4 · `source-ip-mappings`** (roadmap) — references an IP record + a SIP domain.
  - `ListSourceIpMapping` → `CreateSourceIpMapping{IpRecordSid string, SipDomainSid string}`.
  - The Twilio mapping gives you only the OLD `IpRecordSid`/`SipDomainSid`. Resolve each to its natural key (IP record → its IP address via a Fetch; SIP domain → its domain name) then match the NEW VoiceML resource by that key. Must run after G1 + the existing `sip-trunking` migrator.

- [ ] **G5 · `dialing-permissions`** (roadmap — ASSESS FIRST)
  - Twilio models this as a settings/bulk surface (`ListDialingPermissionsCountry`, `ListDialingPermissionsHrsPrefixes(IsoCode)`), NOT a simple create. Confirm what `voiceml-go-sdk` exposes on the write side. If there is no write side, mark the `Inventory()` row `CovUnmigratable` with the reason instead of forcing a migrator.

- [ ] **G6 · `conversations`** (roadmap — large)
  - Conversations v1 product (`dst.VoiceML.ConversationsV1`): services, users, roles, conversations, participants, webhooks. Large stateful surface with cross-refs — scope sub-resources, idempotency keys, and ordering carefully. Likely its own multi-file effort.

- [ ] **G7 · `assistants`** (roadmap — large)
  - Assistants v1 product (`dst.VoiceML.AssistantsV1`): assistants, tools, knowledge, policy. **BYO-LLM keys are secrets** — they cannot be read from the source; surface that like SIP passwords (never copy/store; require re-entry).

---

## Unmigratable (documented — not a straight copy)

- [x] **`outgoing-caller-ids`** (unmigratable) — created ONLY via phone
  validation. Twilio (and VoiceML) expose `CreateValidationRequest`, not a
  direct create; there is no `CreateOutgoingCallerId` anywhere. Each number must
  be re-verified interactively on VoiceML.
  - *Optional convenience (not a copy):* a `--validate-caller-ids` mode that
    reads the Twilio caller IDs and *initiates* a VoiceML validation request per
    number, reporting the code/call_sid. Only if the VoiceML SDK exposes a
    validation-request create (it has no OutgoingCallerId service today). Keep
    the `Inventory()` row `CovUnmigratable`.

- [x] **`sip-inbound-region`** (unmigratable — BLOCKED on SDK) — callBroadcast
  serves it (`registerSIPInboundRegionRoutes`) but `voiceml-go-sdk` exposes no
  service for it, so the tool cannot write it. The SDK (a separate repo, the
  SDK-agent's) must add an InboundRegion service first; then add a
  `sip-inbound-region` migrator and flip the `Inventory()` row. **Do NOT modify
  `~/Sync/voiceml-sdk` from this repo** — raise it with the SDK owner.

---

## Wiring notes (apply once, benefits G1–G4)

- Add the voice/v1 source sub-client to `internal/migrate/clients.go`:
  `TwilioVoice *twvoice.ApiService` (`github.com/twilio/twilio-go/rest/voice/v1`),
  set from `src.VoiceV1`. Destination is `c.VoiceML.VoiceV1`
  (`*voiceml.VoiceV1Service`).
- All VoiceV1 `List*` write-side methods take `voiceml.V1PageParams{}`.
- twilio-go source fields are `*string` (SIDs included) — always `deref()`.

## Done / for reference (already migrated ✅)

`phone-numbers`, `applications`, `sip-trunking` (domains, cred lists +
credentials, IP ACLs + IPs, mappings), `messaging`, `queues`. See the matching
`internal/migrate/*.go` files; `sip.go` is the worked example for cross-resource
SID remapping.

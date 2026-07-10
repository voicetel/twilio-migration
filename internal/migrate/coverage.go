package migrate

// Coverage classifies a Twilio→VoiceML config resource's migration status.
type Coverage string

const (
	// CovMigrated: a registered Migrator handles this resource.
	CovMigrated Coverage = "migrated"
	// CovUnmigratable: the resource cannot be migrated by any tool; Reason says why.
	CovUnmigratable Coverage = "unmigratable"
	// CovRoadmap: migratable and planned, but not yet implemented; Reason tracks it.
	CovRoadmap Coverage = "roadmap"
)

// ResourceCoverage is one row of the migration coverage matrix.
type ResourceCoverage struct {
	// Resource matches a Migrator.Name() when Status is CovMigrated.
	Resource string
	Status   Coverage
	// Reason is required for CovUnmigratable and CovRoadmap.
	Reason string
}

// Inventory is the AUTHORITATIVE list of every VoiceML config resource the
// migration tool knows about, with its status. It is the tool's own
// coverage/"parity" surface: coverage_test.go asserts this stays consistent
// with the registered migrators, so a resource can never be silently dropped —
// every one is either migrated, or carries a documented reason it is not.
//
// When the VoiceML SDK gains a new create-capable config resource, add a row
// here (as CovRoadmap until a migrator lands); the test then enforces it.
func Inventory() []ResourceCoverage {
	return []ResourceCoverage{
		// Implemented.
		{Resource: "phone-numbers", Status: CovMigrated},
		{Resource: "applications", Status: CovMigrated},
		{Resource: "sip-trunking", Status: CovMigrated},
		{Resource: "messaging", Status: CovMigrated},
		{Resource: "queues", Status: CovMigrated},

		// Migratable via the SDK's VoiceV1 service; migrators pending (task #44).
		{Resource: "byoc-trunks", Status: CovRoadmap, Reason: "voiceml-go-sdk VoiceV1.CreateByocTrunk supported; migrator pending (#44)"},
		{Resource: "connection-policies", Status: CovRoadmap, Reason: "voiceml-go-sdk VoiceV1.CreateConnectionPolicy(+Target) supported; migrator pending (#44)"},
		{Resource: "ip-records", Status: CovRoadmap, Reason: "voiceml-go-sdk VoiceV1.CreateIpRecord supported; migrator pending (#44)"},
		{Resource: "source-ip-mappings", Status: CovRoadmap, Reason: "voiceml-go-sdk VoiceV1.CreateSourceIpMapping supported; migrator pending (#44)"},

		// Larger surfaces / non-trivial shapes; migrators pending (task #45).
		{Resource: "dialing-permissions", Status: CovRoadmap, Reason: "Twilio models this as a settings/bulk-update surface (DialingPermissionsCountry/HrsPrefixes), not a simple create; needs assessment (#45)"},
		{Resource: "conversations", Status: CovRoadmap, Reason: "Conversations v1 is a large stateful product (services, users, roles, conversations, participants, webhooks); pending (#45)"},
		{Resource: "assistants", Status: CovRoadmap, Reason: "Assistants v1 is a large stateful product (assistants, tools, knowledge, policy); pending (#45)"},

		// Cannot be migrated by any tool.
		{Resource: "outgoing-caller-ids", Status: CovUnmigratable, Reason: "created ONLY via phone validation — Twilio (and VoiceML) expose CreateValidationRequest, not a direct create; each number must be re-verified interactively on VoiceML"},
		{Resource: "sip-inbound-region", Status: CovUnmigratable, Reason: "not exposed by voiceml-go-sdk"},
	}
}

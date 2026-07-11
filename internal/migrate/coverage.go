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
		{Resource: "ip-records", Status: CovMigrated},
		{Resource: "connection-policies", Status: CovMigrated},
		{Resource: "byoc-trunks", Status: CovMigrated},
		{Resource: "source-ip-mappings", Status: CovMigrated},
		{Resource: "conversations", Status: CovMigrated},
		{Resource: "assistants", Status: CovMigrated},

		// Cannot be migrated by any tool.
		{Resource: "outgoing-caller-ids", Status: CovUnmigratable, Reason: "created ONLY via phone validation — Twilio (and VoiceML) expose CreateValidationRequest, not a direct create; each number must be re-verified interactively on VoiceML"},
		{Resource: "sip-inbound-region", Status: CovUnmigratable, Reason: "not exposed by voiceml-go-sdk"},
		{Resource: "dialing-permissions", Status: CovUnmigratable, Reason: "assessed (#45): voiceml-go-sdk's VoiceV1Service exposes only a Fetch/UpdateSettings singleton for one boolean (DialingPermissionsInheritance); it has no write endpoint for what this resource actually represents on Twilio — the per-country and high-risk-prefix allow/deny list (DialingPermissionsCountry, bulk country updates, HrsPrefixes). Migrating only the inheritance flag would misrepresent the resource as migrated"},
	}
}

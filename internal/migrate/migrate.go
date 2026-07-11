// Package migrate moves account CONFIGURATION from a Twilio account to a
// VoiceML account. It reads resources from Twilio with the official twilio-go
// SDK and writes them to VoiceML with the official voiceml-go-sdk — VoiceML's
// REST API is Twilio-compatible, so the shapes line up.
//
// Note: this migrates configuration (phone numbers, TwiML apps, SIP trunking,
// messaging config). It does NOT migrate historical usage records; Twilio's
// Bulk Export covers those (Messages/Calls/Conferences/Participants) and they
// are activity logs, not importable configuration.
package migrate

import "context"

// Options controls a migration run.
type Options struct {
	// DryRun reports what would change without writing to VoiceML.
	DryRun bool
}

// ItemStatus is the outcome for a single migrated resource instance.
type ItemStatus string

const (
	// StatusCreated: the resource was created on VoiceML.
	StatusCreated ItemStatus = "created"
	// StatusSkipped: an equivalent resource already exists on VoiceML.
	StatusSkipped ItemStatus = "skipped"
	// StatusPlanned: dry-run; the resource would be created.
	StatusPlanned ItemStatus = "planned"
	// StatusFailed: creating the resource on VoiceML failed.
	StatusFailed ItemStatus = "failed"
)

// ItemResult is the outcome for one source resource.
type ItemResult struct {
	// ID identifies the source resource to a human (phone number, friendly
	// name, SID, ...).
	ID     string
	Status ItemStatus
	// Detail carries an error message or an explanatory note.
	Detail string
}

// Result aggregates the per-item outcomes for one resource type.
type Result struct {
	Resource string
	Items    []ItemResult
}

// Count returns the number of items with the given status.
func (r Result) Count(s ItemStatus) int {
	n := 0
	for _, it := range r.Items {
		if it.Status == s {
			n++
		}
	}
	return n
}

// HasFailures reports whether any item failed.
func (r Result) HasFailures() bool { return r.Count(StatusFailed) > 0 }

// Migrator migrates one resource type from Twilio to VoiceML.
type Migrator interface {
	// Name is the stable, lower-case identifier used by --only and in output.
	Name() string
	// Migrate reads the resource from Twilio and writes it to VoiceML. A
	// returned error means the resource could not be enumerated at all;
	// per-item write failures are recorded in the Result, not returned.
	Migrate(ctx context.Context, c *Clients, opts Options) (Result, error)
}

// Default returns the built-in migrators in a sensible run order.
func Default() []Migrator {
	return []Migrator{
		PhoneNumbers{},
		Applications{},
		SIP{},
		Messaging{},
		Queues{},
		// IPRecords produces SIDs a future source-ip-mappings migrator will
		// need to resolve by IP address; run it early.
		IPRecords{},
		// ConnectionPolicies produces SIDs a future byoc-trunks migrator will
		// need to resolve by friendly name; run it before that.
		ConnectionPolicies{},
	}
}

// Select returns the migrators whose names appear in only. When only is empty
// it returns all of them. Unknown names are returned so the caller can warn.
func Select(all []Migrator, only []string) (selected []Migrator, unknown []string) {
	if len(only) == 0 {
		return all, nil
	}

	byName := make(map[string]Migrator, len(all))
	for _, m := range all {
		byName[m.Name()] = m
	}

	for _, name := range only {
		if m, ok := byName[name]; ok {
			selected = append(selected, m)
		} else {
			unknown = append(unknown, name)
		}
	}

	return selected, unknown
}

// Run executes each migrator in turn. A migrator that fails to enumerate its
// resource is recorded as a single failed item and does not stop the run.
func Run(ctx context.Context, c *Clients, migrators []Migrator, opts Options) []Result {
	results := make([]Result, 0, len(migrators))
	for _, m := range migrators {
		res, err := m.Migrate(ctx, c, opts)
		if res.Resource == "" {
			res.Resource = m.Name()
		}
		if err != nil {
			res.Items = append(res.Items, ItemResult{
				ID:     m.Name(),
				Status: StatusFailed,
				Detail: err.Error(),
			})
		}
		results = append(results, res)
	}
	return results
}

package migrate

import "testing"

// TestCoverageInventoryConsistent is the tool's own coverage gate: it fails the
// build if the resource inventory and the registered migrators drift apart, so
// a resource can never be silently unhandled.
func TestCoverageInventoryConsistent(t *testing.T) {
	inv := Inventory()

	byName := make(map[string]ResourceCoverage, len(inv))
	for _, e := range inv {
		if e.Resource == "" {
			t.Fatalf("inventory entry with empty resource name: %+v", e)
		}
		if _, dup := byName[e.Resource]; dup {
			t.Fatalf("duplicate inventory entry: %q", e.Resource)
		}
		byName[e.Resource] = e

		switch e.Status {
		case CovMigrated:
			if e.Reason != "" {
				t.Errorf("%q is migrated but carries a reason (%q) — reasons are for unmigratable/roadmap", e.Resource, e.Reason)
			}
		case CovUnmigratable, CovRoadmap:
			if e.Reason == "" {
				t.Errorf("%q (%s) must document a reason", e.Resource, e.Status)
			}
		default:
			t.Errorf("%q has invalid status %q", e.Resource, e.Status)
		}
	}

	registered := make(map[string]bool)
	for _, m := range Default() {
		registered[m.Name()] = true

		// Every registered migrator MUST be inventoried as migrated.
		e, ok := byName[m.Name()]
		if !ok {
			t.Errorf("migrator %q is registered but missing from Inventory()", m.Name())
			continue
		}
		if e.Status != CovMigrated {
			t.Errorf("migrator %q is registered but Inventory() marks it %q", m.Name(), e.Status)
		}
	}

	// Every migrated inventory entry MUST have a registered migrator.
	for _, e := range inv {
		if e.Status == CovMigrated && !registered[e.Resource] {
			t.Errorf("Inventory() marks %q migrated but no migrator is registered for it", e.Resource)
		}
	}
}

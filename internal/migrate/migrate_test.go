package migrate

import (
	"context"
	"errors"
	"testing"
)

type fakeMigrator struct {
	name string
	res  Result
	err  error
}

func (f fakeMigrator) Name() string { return f.name }
func (f fakeMigrator) Migrate(context.Context, *Clients, Options) (Result, error) {
	return f.res, f.err
}

func TestDefaultMigrators(t *testing.T) {
	names := make(map[string]bool)
	for _, m := range Default() {
		names[m.Name()] = true
	}
	for _, want := range []string{"phone-numbers", "applications"} {
		if !names[want] {
			t.Errorf("Default() missing %q", want)
		}
	}
}

func TestSelect(t *testing.T) {
	all := []Migrator{fakeMigrator{name: "a"}, fakeMigrator{name: "b"}}

	sel, unknown := Select(all, nil)
	if len(sel) != 2 || len(unknown) != 0 {
		t.Errorf("empty only should select all: %d %v", len(sel), unknown)
	}

	sel, unknown = Select(all, []string{"b", "zzz"})
	if len(sel) != 1 || sel[0].Name() != "b" {
		t.Errorf("select b failed: %+v", sel)
	}
	if len(unknown) != 1 || unknown[0] != "zzz" {
		t.Errorf("unknown not reported: %v", unknown)
	}
}

func TestRunRecordsEnumerationError(t *testing.T) {
	ms := []Migrator{
		fakeMigrator{name: "ok", res: Result{Resource: "ok", Items: []ItemResult{{ID: "1", Status: StatusCreated}}}},
		fakeMigrator{name: "bad", err: errors.New("enumerate failed")},
	}

	results := Run(context.Background(), nil, ms, Options{})
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
	// The erroring migrator gets its name filled in and a failed item.
	bad := results[1]
	if bad.Resource != "bad" || !bad.HasFailures() {
		t.Errorf("enumeration error not recorded: %+v", bad)
	}
}

func TestResultCount(t *testing.T) {
	r := Result{Items: []ItemResult{
		{Status: StatusCreated}, {Status: StatusCreated}, {Status: StatusSkipped}, {Status: StatusFailed},
	}}
	if r.Count(StatusCreated) != 2 || r.Count(StatusSkipped) != 1 || r.Count(StatusFailed) != 1 {
		t.Errorf("counts wrong: %+v", r)
	}
	if !r.HasFailures() {
		t.Error("HasFailures should be true")
	}
	if (Result{}).HasFailures() {
		t.Error("empty result has no failures")
	}
}

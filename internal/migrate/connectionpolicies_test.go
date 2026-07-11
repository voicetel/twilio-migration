package migrate

import (
	"context"
	"errors"
	"testing"

	twvoice "github.com/twilio/twilio-go/rest/voice/v1"
	voiceml "github.com/voicetel/voiceml-go-sdk"
)

type fakeConnPolicySource struct {
	policies  []twvoice.VoiceV1ConnectionPolicy
	targets   map[string][]twvoice.VoiceV1ConnectionPolicyTarget // keyed by policy sid
	policyErr error
	targetErr error
}

func (f fakeConnPolicySource) ListConnectionPolicy(*twvoice.ListConnectionPolicyParams) ([]twvoice.VoiceV1ConnectionPolicy, error) {
	if f.policyErr != nil {
		return nil, f.policyErr
	}
	return f.policies, nil
}

func (f fakeConnPolicySource) ListConnectionPolicyTarget(sid string, _ *twvoice.ListConnectionPolicyTargetParams) ([]twvoice.VoiceV1ConnectionPolicyTarget, error) {
	if f.targetErr != nil {
		return nil, f.targetErr
	}
	return f.targets[sid], nil
}

type fakeConnPolicyDest struct {
	existing        []voiceml.VoiceV1ConnectionPolicy
	existingTargets []voiceml.VoiceV1ConnectionPolicyTarget
	created         []string
	createdTargets  []voiceml.CreateVoiceV1ConnectionPolicyTargetParams

	listErr, createErr, listTargetErr, createTargetErr error
}

func (f *fakeConnPolicyDest) ListConnectionPolicies(context.Context, voiceml.V1PageParams) (*voiceml.VoiceV1ConnectionPolicyList, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return &voiceml.VoiceV1ConnectionPolicyList{ConnectionPolicies: f.existing}, nil
}

func (f *fakeConnPolicyDest) CreateConnectionPolicy(_ context.Context, p voiceml.CreateVoiceV1ConnectionPolicyParams) (*voiceml.VoiceV1ConnectionPolicy, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = append(f.created, deref(p.FriendlyName))
	return &voiceml.VoiceV1ConnectionPolicy{Sid: strp("NYnew"), FriendlyName: p.FriendlyName}, nil
}

func (f *fakeConnPolicyDest) ListConnectionPolicyTargets(context.Context, string, voiceml.V1PageParams) (*voiceml.VoiceV1ConnectionPolicyTargetList, error) {
	if f.listTargetErr != nil {
		return nil, f.listTargetErr
	}
	return &voiceml.VoiceV1ConnectionPolicyTargetList{Targets: f.existingTargets}, nil
}

func (f *fakeConnPolicyDest) CreateConnectionPolicyTarget(_ context.Context, _ string, p voiceml.CreateVoiceV1ConnectionPolicyTargetParams) (*voiceml.VoiceV1ConnectionPolicyTarget, error) {
	if f.createTargetErr != nil {
		return nil, f.createTargetErr
	}
	f.createdTargets = append(f.createdTargets, p)
	return &voiceml.VoiceV1ConnectionPolicyTarget{Target: &p.Target}, nil
}

func TestMigrateConnectionPolicies_FullGraph(t *testing.T) {
	src := fakeConnPolicySource{
		policies: []twvoice.VoiceV1ConnectionPolicy{{Sid: strp("NYold"), FriendlyName: strp("Primary")}},
		targets: map[string][]twvoice.VoiceV1ConnectionPolicyTarget{
			"NYold": {{Target: strp("sip:a@example.com"), FriendlyName: strp("a"), Priority: 5, Weight: 10, Enabled: boolp(true)}},
		},
	}
	dst := &fakeConnPolicyDest{}

	res, err := migrateConnectionPolicies(context.Background(), src, dst, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(dst.created) != 1 || dst.created[0] != "Primary" {
		t.Errorf("policy not created: %+v", dst.created)
	}
	if len(dst.createdTargets) != 1 || dst.createdTargets[0].Target != "sip:a@example.com" {
		t.Errorf("target not created: %+v", dst.createdTargets)
	}
	if dst.createdTargets[0].Priority == nil || *dst.createdTargets[0].Priority != 5 {
		t.Errorf("priority not mapped: %+v", dst.createdTargets[0])
	}
	if dst.createdTargets[0].Weight == nil || *dst.createdTargets[0].Weight != 10 {
		t.Errorf("weight not mapped: %+v", dst.createdTargets[0])
	}
	if res.Count(StatusCreated) != 2 {
		t.Errorf("expected 2 created items (policy + target), got %+v", res)
	}
}

func TestMigrateConnectionPolicies_DryRun(t *testing.T) {
	src := fakeConnPolicySource{
		policies: []twvoice.VoiceV1ConnectionPolicy{{Sid: strp("NYold"), FriendlyName: strp("Primary")}},
	}
	dst := &fakeConnPolicyDest{}

	res, err := migrateConnectionPolicies(context.Background(), src, dst, Options{DryRun: true})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(dst.created) != 0 {
		t.Errorf("dry run must not create: %+v", dst.created)
	}
	if res.Count(StatusPlanned) != 1 {
		t.Errorf("expected 1 planned item, got %+v", res)
	}
}

func TestMigrateConnectionPolicies_SkipsExistingPolicyCreatesTarget(t *testing.T) {
	src := fakeConnPolicySource{
		policies: []twvoice.VoiceV1ConnectionPolicy{{Sid: strp("NYold"), FriendlyName: strp("Primary")}},
		targets: map[string][]twvoice.VoiceV1ConnectionPolicyTarget{
			"NYold": {{Target: strp("sip:a@example.com")}},
		},
	}
	dst := &fakeConnPolicyDest{
		existing: []voiceml.VoiceV1ConnectionPolicy{{Sid: strp("NYx"), FriendlyName: strp("Primary")}},
	}

	res, err := migrateConnectionPolicies(context.Background(), src, dst, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusSkipped) != 1 {
		t.Errorf("expected policy skipped, got %+v", res)
	}
	if len(dst.createdTargets) != 1 {
		t.Errorf("expected target created against the existing policy, got %+v", dst.createdTargets)
	}
}

func TestMigrateConnectionPolicies_EmptyName(t *testing.T) {
	src := fakeConnPolicySource{policies: []twvoice.VoiceV1ConnectionPolicy{{Sid: strp("NY1")}}}
	res, err := migrateConnectionPolicies(context.Background(), src, &fakeConnPolicyDest{}, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusFailed) != 1 {
		t.Errorf("expected 1 failed item, got %+v", res)
	}
}

func TestMigrateConnectionPolicies_CreateError(t *testing.T) {
	src := fakeConnPolicySource{policies: []twvoice.VoiceV1ConnectionPolicy{{Sid: strp("NY1"), FriendlyName: strp("Primary")}}}
	dst := &fakeConnPolicyDest{createErr: errors.New("boom")}
	res, err := migrateConnectionPolicies(context.Background(), src, dst, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusFailed) != 1 {
		t.Errorf("expected 1 failed item, got %+v", res)
	}
}

func TestMigrateConnectionPolicies_ListErrors(t *testing.T) {
	src := fakeConnPolicySource{policyErr: errors.New("boom")}
	if _, err := migrateConnectionPolicies(context.Background(), src, &fakeConnPolicyDest{}, Options{}); err == nil {
		t.Error("want Twilio list error")
	}
	if _, err := migrateConnectionPolicies(context.Background(), fakeConnPolicySource{}, &fakeConnPolicyDest{listErr: errors.New("boom")}, Options{}); err == nil {
		t.Error("want VoiceML list error")
	}
}

func TestMigrateConnectionPolicies_PropagatesTargetError(t *testing.T) {
	src := fakeConnPolicySource{
		policies:  []twvoice.VoiceV1ConnectionPolicy{{Sid: strp("NY1"), FriendlyName: strp("Primary")}},
		targetErr: errors.New("boom"),
	}
	if _, err := migrateConnectionPolicies(context.Background(), src, &fakeConnPolicyDest{}, Options{}); err == nil {
		t.Error("want target list error to propagate")
	}
}

// --- migrateConnectionPolicyTargets branch coverage ---

func TestMigrateConnectionPolicyTargets_TwilioListError(t *testing.T) {
	src := fakeConnPolicySource{targetErr: errors.New("boom")}
	res := &Result{}
	err := migrateConnectionPolicyTargets(context.Background(), src, &fakeConnPolicyDest{}, "NY1", "Primary", "NYnew", Options{}, res)
	if err == nil {
		t.Fatal("want error")
	}
}

func TestMigrateConnectionPolicyTargets_VoiceMLListError(t *testing.T) {
	src := fakeConnPolicySource{}
	dst := &fakeConnPolicyDest{listTargetErr: errors.New("boom")}
	res := &Result{}
	err := migrateConnectionPolicyTargets(context.Background(), src, dst, "NY1", "Primary", "NYnew", Options{}, res)
	if err == nil {
		t.Fatal("want error")
	}
}

func TestMigrateConnectionPolicyTargets_AlreadyMapped(t *testing.T) {
	src := fakeConnPolicySource{targets: map[string][]twvoice.VoiceV1ConnectionPolicyTarget{"NY1": {{Target: strp("sip:a@example.com")}}}}
	dst := &fakeConnPolicyDest{existingTargets: []voiceml.VoiceV1ConnectionPolicyTarget{{Target: strp("sip:a@example.com")}}}
	res := &Result{}
	if err := migrateConnectionPolicyTargets(context.Background(), src, dst, "NY1", "Primary", "NYnew", Options{}, res); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusSkipped) != 1 {
		t.Errorf("expected 1 skipped item, got %+v", res)
	}
}

func TestMigrateConnectionPolicyTargets_DryRunPlanned(t *testing.T) {
	src := fakeConnPolicySource{targets: map[string][]twvoice.VoiceV1ConnectionPolicyTarget{"NY1": {{Target: strp("sip:a@example.com")}}}}
	res := &Result{}
	if err := migrateConnectionPolicyTargets(context.Background(), src, &fakeConnPolicyDest{}, "NY1", "Primary", "NYnew", Options{DryRun: true}, res); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusPlanned) != 1 {
		t.Errorf("expected 1 planned item, got %+v", res)
	}
}

func TestMigrateConnectionPolicyTargets_EmptyURI(t *testing.T) {
	src := fakeConnPolicySource{targets: map[string][]twvoice.VoiceV1ConnectionPolicyTarget{"NY1": {{}}}}
	res := &Result{}
	if err := migrateConnectionPolicyTargets(context.Background(), src, &fakeConnPolicyDest{}, "NY1", "Primary", "NYnew", Options{}, res); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusFailed) != 1 {
		t.Errorf("expected 1 failed item, got %+v", res)
	}
}

func TestMigrateConnectionPolicyTargets_CreateError(t *testing.T) {
	src := fakeConnPolicySource{targets: map[string][]twvoice.VoiceV1ConnectionPolicyTarget{"NY1": {{Target: strp("sip:a@example.com")}}}}
	dst := &fakeConnPolicyDest{createTargetErr: errors.New("boom")}
	res := &Result{}
	if err := migrateConnectionPolicyTargets(context.Background(), src, dst, "NY1", "Primary", "NYnew", Options{}, res); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusFailed) != 1 {
		t.Errorf("expected 1 failed item, got %+v", res)
	}
}

func TestMigrateConnectionPolicyTargets_NoPriorityOrWeight(t *testing.T) {
	src := fakeConnPolicySource{targets: map[string][]twvoice.VoiceV1ConnectionPolicyTarget{"NY1": {{Target: strp("sip:a@example.com")}}}}
	dst := &fakeConnPolicyDest{}
	res := &Result{}
	if err := migrateConnectionPolicyTargets(context.Background(), src, dst, "NY1", "Primary", "NYnew", Options{}, res); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(dst.createdTargets) != 1 || dst.createdTargets[0].Priority != nil || dst.createdTargets[0].Weight != nil {
		t.Errorf("expected no priority/weight set, got %+v", dst.createdTargets)
	}
}

func TestConnectionPoliciesName(t *testing.T) {
	if (ConnectionPolicies{}).Name() != "connection-policies" {
		t.Errorf("name=%q", (ConnectionPolicies{}).Name())
	}
}

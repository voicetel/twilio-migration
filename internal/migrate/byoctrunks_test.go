package migrate

import (
	"context"
	"errors"
	"testing"

	twapi "github.com/twilio/twilio-go/rest/api/v2010"
	twvoice "github.com/twilio/twilio-go/rest/voice/v1"
	voiceml "github.com/voicetel/voiceml-go-sdk"
)

type fakeByocTrunkSource struct {
	trunks    []twvoice.VoiceV1ByocTrunk
	policies  []twvoice.VoiceV1ConnectionPolicy
	trunkErr  error
	policyErr error
}

func (f fakeByocTrunkSource) ListByocTrunk(*twvoice.ListByocTrunkParams) ([]twvoice.VoiceV1ByocTrunk, error) {
	if f.trunkErr != nil {
		return nil, f.trunkErr
	}
	return f.trunks, nil
}

func (f fakeByocTrunkSource) ListConnectionPolicy(*twvoice.ListConnectionPolicyParams) ([]twvoice.VoiceV1ConnectionPolicy, error) {
	if f.policyErr != nil {
		return nil, f.policyErr
	}
	return f.policies, nil
}

type fakeByocDomainSource struct {
	domains []twapi.ApiV2010SipDomain
	err     error
}

func (f fakeByocDomainSource) ListSipDomain(*twapi.ListSipDomainParams) ([]twapi.ApiV2010SipDomain, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.domains, nil
}

type fakeByocTrunkDest struct {
	existingTrunks   []voiceml.VoiceV1ByocTrunk
	existingPolicies []voiceml.VoiceV1ConnectionPolicy
	created          []voiceml.CreateVoiceV1ByocTrunkParams

	listTrunkErr, createErr, listPolicyErr error
}

func (f *fakeByocTrunkDest) ListByocTrunks(context.Context, voiceml.V1PageParams) (*voiceml.VoiceV1ByocTrunkList, error) {
	if f.listTrunkErr != nil {
		return nil, f.listTrunkErr
	}
	return &voiceml.VoiceV1ByocTrunkList{ByocTrunks: f.existingTrunks}, nil
}

func (f *fakeByocTrunkDest) CreateByocTrunk(_ context.Context, p voiceml.CreateVoiceV1ByocTrunkParams) (*voiceml.VoiceV1ByocTrunk, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = append(f.created, p)
	return &voiceml.VoiceV1ByocTrunk{FriendlyName: p.FriendlyName}, nil
}

func (f *fakeByocTrunkDest) ListConnectionPolicies(context.Context, voiceml.V1PageParams) (*voiceml.VoiceV1ConnectionPolicyList, error) {
	if f.listPolicyErr != nil {
		return nil, f.listPolicyErr
	}
	return &voiceml.VoiceV1ConnectionPolicyList{ConnectionPolicies: f.existingPolicies}, nil
}

type fakeByocDomainDest struct {
	existing []voiceml.SIPDomain
	err      error
}

func (f *fakeByocDomainDest) List(context.Context, voiceml.ListPageParams) (*voiceml.SIPDomainList, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &voiceml.SIPDomainList{Domains: f.existing}, nil
}

func TestMigrateByocTrunks_FullRemap(t *testing.T) {
	src := fakeByocTrunkSource{
		trunks: []twvoice.VoiceV1ByocTrunk{{
			FriendlyName:        strp("Carrier"),
			VoiceUrl:            strp("https://example.com/voice"),
			ConnectionPolicySid: strp("NYold"),
			FromDomainSid:       strp("SDold"),
		}},
		policies: []twvoice.VoiceV1ConnectionPolicy{{Sid: strp("NYold"), FriendlyName: strp("Primary")}},
	}
	domainSrc := fakeByocDomainSource{domains: []twapi.ApiV2010SipDomain{{Sid: strp("SDold"), DomainName: strp("acme.vml.voice.tel")}}}
	dst := &fakeByocTrunkDest{existingPolicies: []voiceml.VoiceV1ConnectionPolicy{{Sid: strp("NYnew"), FriendlyName: strp("Primary")}}}
	domainDst := &fakeByocDomainDest{existing: []voiceml.SIPDomain{{Sid: "SDnew", DomainName: "acme.vml.voice.tel"}}}

	res, err := migrateByocTrunks(context.Background(), src, domainSrc, dst, domainDst, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(dst.created) != 1 {
		t.Fatalf("expected 1 created trunk, got %+v", dst.created)
	}
	c := dst.created[0]
	if c.ConnectionPolicySid == nil || *c.ConnectionPolicySid != "NYnew" {
		t.Errorf("connection policy SID not remapped: %+v", c)
	}
	if c.FromDomainSid == nil || *c.FromDomainSid != "SDnew" {
		t.Errorf("from-domain SID not remapped: %+v", c)
	}
	if res.Count(StatusCreated) != 1 {
		t.Errorf("counts: %+v", res)
	}
}

func TestMigrateByocTrunks_NoOptionalRefs(t *testing.T) {
	src := fakeByocTrunkSource{trunks: []twvoice.VoiceV1ByocTrunk{{FriendlyName: strp("Carrier")}}}
	dst := &fakeByocTrunkDest{}

	res, err := migrateByocTrunks(context.Background(), src, fakeByocDomainSource{}, dst, &fakeByocDomainDest{}, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(dst.created) != 1 || dst.created[0].ConnectionPolicySid != nil || dst.created[0].FromDomainSid != nil {
		t.Errorf("expected no refs set, got %+v", dst.created)
	}
	if res.Count(StatusCreated) != 1 {
		t.Errorf("counts: %+v", res)
	}
}

func TestMigrateByocTrunks_UnresolvableConnectionPolicy(t *testing.T) {
	src := fakeByocTrunkSource{
		trunks: []twvoice.VoiceV1ByocTrunk{{FriendlyName: strp("Carrier"), ConnectionPolicySid: strp("NYold")}},
		// No matching policy in Twilio's list at all, so the bridge map is empty.
	}
	res, err := migrateByocTrunks(context.Background(), src, fakeByocDomainSource{}, &fakeByocTrunkDest{}, &fakeByocDomainDest{}, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusFailed) != 1 {
		t.Errorf("expected 1 failed item, got %+v", res)
	}
}

func TestMigrateByocTrunks_UnresolvableFromDomain(t *testing.T) {
	src := fakeByocTrunkSource{
		trunks: []twvoice.VoiceV1ByocTrunk{{FriendlyName: strp("Carrier"), FromDomainSid: strp("SDold")}},
	}
	res, err := migrateByocTrunks(context.Background(), src, fakeByocDomainSource{}, &fakeByocTrunkDest{}, &fakeByocDomainDest{}, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusFailed) != 1 {
		t.Errorf("expected 1 failed item, got %+v", res)
	}
}

func TestMigrateByocTrunks_EmptyName(t *testing.T) {
	src := fakeByocTrunkSource{trunks: []twvoice.VoiceV1ByocTrunk{{}}}
	res, err := migrateByocTrunks(context.Background(), src, fakeByocDomainSource{}, &fakeByocTrunkDest{}, &fakeByocDomainDest{}, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusFailed) != 1 {
		t.Errorf("expected 1 failed item, got %+v", res)
	}
}

func TestMigrateByocTrunks_SkipsExisting(t *testing.T) {
	src := fakeByocTrunkSource{trunks: []twvoice.VoiceV1ByocTrunk{{FriendlyName: strp("Carrier")}}}
	dst := &fakeByocTrunkDest{existingTrunks: []voiceml.VoiceV1ByocTrunk{{FriendlyName: strp("Carrier")}}}
	res, err := migrateByocTrunks(context.Background(), src, fakeByocDomainSource{}, dst, &fakeByocDomainDest{}, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusSkipped) != 1 || len(dst.created) != 0 {
		t.Errorf("expected skip, got %+v / created=%+v", res, dst.created)
	}
}

func TestMigrateByocTrunks_DryRun(t *testing.T) {
	src := fakeByocTrunkSource{trunks: []twvoice.VoiceV1ByocTrunk{{FriendlyName: strp("Carrier")}}}
	dst := &fakeByocTrunkDest{}
	res, err := migrateByocTrunks(context.Background(), src, fakeByocDomainSource{}, dst, &fakeByocDomainDest{}, Options{DryRun: true})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusPlanned) != 1 || len(dst.created) != 0 {
		t.Errorf("expected planned, got %+v / created=%+v", res, dst.created)
	}
}

func TestMigrateByocTrunks_CreateError(t *testing.T) {
	src := fakeByocTrunkSource{trunks: []twvoice.VoiceV1ByocTrunk{{FriendlyName: strp("Carrier")}}}
	dst := &fakeByocTrunkDest{createErr: errors.New("boom")}
	res, err := migrateByocTrunks(context.Background(), src, fakeByocDomainSource{}, dst, &fakeByocDomainDest{}, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusFailed) != 1 {
		t.Errorf("expected 1 failed item, got %+v", res)
	}
}

func TestMigrateByocTrunks_ListErrors(t *testing.T) {
	cases := []struct {
		name      string
		src       fakeByocTrunkSource
		domainSrc fakeByocDomainSource
		dst       *fakeByocTrunkDest
		domainDst *fakeByocDomainDest
	}{
		{"twilio trunks", fakeByocTrunkSource{trunkErr: errors.New("x")}, fakeByocDomainSource{}, &fakeByocTrunkDest{}, &fakeByocDomainDest{}},
		{"voiceml trunks", fakeByocTrunkSource{}, fakeByocDomainSource{}, &fakeByocTrunkDest{listTrunkErr: errors.New("x")}, &fakeByocDomainDest{}},
		{"twilio policies", fakeByocTrunkSource{policyErr: errors.New("x")}, fakeByocDomainSource{}, &fakeByocTrunkDest{}, &fakeByocDomainDest{}},
		{"voiceml policies", fakeByocTrunkSource{}, fakeByocDomainSource{}, &fakeByocTrunkDest{listPolicyErr: errors.New("x")}, &fakeByocDomainDest{}},
		{"twilio domains", fakeByocTrunkSource{}, fakeByocDomainSource{err: errors.New("x")}, &fakeByocTrunkDest{}, &fakeByocDomainDest{}},
		{"voiceml domains", fakeByocTrunkSource{}, fakeByocDomainSource{}, &fakeByocTrunkDest{}, &fakeByocDomainDest{err: errors.New("x")}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := migrateByocTrunks(context.Background(), c.src, c.domainSrc, c.dst, c.domainDst, Options{}); err == nil {
				t.Errorf("%s: want error", c.name)
			}
		})
	}
}

func TestByocTrunksName(t *testing.T) {
	if (ByocTrunks{}).Name() != "byoc-trunks" {
		t.Errorf("name=%q", (ByocTrunks{}).Name())
	}
}

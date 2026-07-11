package migrate

import (
	"context"
	"errors"
	"testing"

	twapi "github.com/twilio/twilio-go/rest/api/v2010"
	twvoice "github.com/twilio/twilio-go/rest/voice/v1"
	voiceml "github.com/voicetel/voiceml-go-sdk"
)

type fakeSourceIPMappingSource struct {
	mappings  []twvoice.VoiceV1SourceIpMapping
	records   []twvoice.VoiceV1IpRecord
	mapErr    error
	recordErr error
}

func (f fakeSourceIPMappingSource) ListSourceIpMapping(*twvoice.ListSourceIpMappingParams) ([]twvoice.VoiceV1SourceIpMapping, error) {
	if f.mapErr != nil {
		return nil, f.mapErr
	}
	return f.mappings, nil
}

func (f fakeSourceIPMappingSource) ListIpRecord(*twvoice.ListIpRecordParams) ([]twvoice.VoiceV1IpRecord, error) {
	if f.recordErr != nil {
		return nil, f.recordErr
	}
	return f.records, nil
}

type fakeSourceIPMappingDest struct {
	existingMappings []voiceml.VoiceV1SourceIpMapping
	existingRecords  []voiceml.VoiceV1IpRecord
	created          []voiceml.CreateVoiceV1SourceIpMappingParams

	listMapErr, createErr, listRecordErr error
}

func (f *fakeSourceIPMappingDest) ListSourceIpMappings(context.Context, voiceml.V1PageParams) (*voiceml.VoiceV1SourceIpMappingList, error) {
	if f.listMapErr != nil {
		return nil, f.listMapErr
	}
	return &voiceml.VoiceV1SourceIpMappingList{SourceIpMappings: f.existingMappings}, nil
}

func (f *fakeSourceIPMappingDest) CreateSourceIpMapping(_ context.Context, p voiceml.CreateVoiceV1SourceIpMappingParams) (*voiceml.VoiceV1SourceIpMapping, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = append(f.created, p)
	return &voiceml.VoiceV1SourceIpMapping{IpRecordSid: &p.IpRecordSid, SipDomainSid: &p.SipDomainSid}, nil
}

func (f *fakeSourceIPMappingDest) ListIpRecords(context.Context, voiceml.V1PageParams) (*voiceml.VoiceV1IpRecordList, error) {
	if f.listRecordErr != nil {
		return nil, f.listRecordErr
	}
	return &voiceml.VoiceV1IpRecordList{IpRecords: f.existingRecords}, nil
}

func sampleMappingSource() fakeSourceIPMappingSource {
	return fakeSourceIPMappingSource{
		mappings: []twvoice.VoiceV1SourceIpMapping{{IpRecordSid: strp("ILold"), SipDomainSid: strp("SDold")}},
		records:  []twvoice.VoiceV1IpRecord{{Sid: strp("ILold"), IpAddress: strp("203.0.113.4")}},
	}
}

func sampleDomainSource() fakeByocDomainSource {
	return fakeByocDomainSource{domains: []twapi.ApiV2010SipDomain{{Sid: strp("SDold"), DomainName: strp("acme.vml.voice.tel")}}}
}

func TestMigrateSourceIPMappings_FullRemap(t *testing.T) {
	dst := &fakeSourceIPMappingDest{existingRecords: []voiceml.VoiceV1IpRecord{{Sid: strp("ILnew"), IpAddress: strp("203.0.113.4")}}}
	domainDst := &fakeByocDomainDest{existing: []voiceml.SIPDomain{{Sid: "SDnew", DomainName: "acme.vml.voice.tel"}}}

	res, err := migrateSourceIPMappings(context.Background(), sampleMappingSource(), sampleDomainSource(), dst, domainDst, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(dst.created) != 1 || dst.created[0].IpRecordSid != "ILnew" || dst.created[0].SipDomainSid != "SDnew" {
		t.Errorf("mapping not remapped: %+v", dst.created)
	}
	if res.Count(StatusCreated) != 1 {
		t.Errorf("counts: %+v", res)
	}
	if res.Items[0].ID != "ip-record 203.0.113.4 -> domain acme.vml.voice.tel" {
		t.Errorf("unexpected label: %q", res.Items[0].ID)
	}
}

func TestMigrateSourceIPMappings_UnresolvableIPRecord(t *testing.T) {
	src := sampleMappingSource()
	src.records = nil // Twilio no longer has this IP record at all
	res, err := migrateSourceIPMappings(context.Background(), src, sampleDomainSource(), &fakeSourceIPMappingDest{}, &fakeByocDomainDest{}, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusFailed) != 1 {
		t.Errorf("expected 1 failed item, got %+v", res)
	}
}

func TestMigrateSourceIPMappings_UnresolvableDomain(t *testing.T) {
	// The IP record resolves fine; only the domain is unresolvable, so this
	// exercises the !domainOK branch specifically (not !ipOK).
	dst := &fakeSourceIPMappingDest{existingRecords: []voiceml.VoiceV1IpRecord{{Sid: strp("ILnew"), IpAddress: strp("203.0.113.4")}}}
	res, err := migrateSourceIPMappings(context.Background(), sampleMappingSource(), fakeByocDomainSource{}, dst, &fakeByocDomainDest{}, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusFailed) != 1 || res.Items[0].Detail != "no migrated SIP domain for source SID SDold" {
		t.Errorf("expected domain-unresolved failure, got %+v", res)
	}
}

func TestMigrateSourceIPMappings_SkipsExisting(t *testing.T) {
	dst := &fakeSourceIPMappingDest{
		existingRecords:  []voiceml.VoiceV1IpRecord{{Sid: strp("ILnew"), IpAddress: strp("203.0.113.4")}},
		existingMappings: []voiceml.VoiceV1SourceIpMapping{{IpRecordSid: strp("ILnew"), SipDomainSid: strp("SDnew")}},
	}
	domainDst := &fakeByocDomainDest{existing: []voiceml.SIPDomain{{Sid: "SDnew", DomainName: "acme.vml.voice.tel"}}}

	res, err := migrateSourceIPMappings(context.Background(), sampleMappingSource(), sampleDomainSource(), dst, domainDst, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusSkipped) != 1 || len(dst.created) != 0 {
		t.Errorf("expected skip, got %+v / created=%+v", res, dst.created)
	}
}

func TestMigrateSourceIPMappings_DryRun(t *testing.T) {
	dst := &fakeSourceIPMappingDest{existingRecords: []voiceml.VoiceV1IpRecord{{Sid: strp("ILnew"), IpAddress: strp("203.0.113.4")}}}
	domainDst := &fakeByocDomainDest{existing: []voiceml.SIPDomain{{Sid: "SDnew", DomainName: "acme.vml.voice.tel"}}}

	res, err := migrateSourceIPMappings(context.Background(), sampleMappingSource(), sampleDomainSource(), dst, domainDst, Options{DryRun: true})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusPlanned) != 1 || len(dst.created) != 0 {
		t.Errorf("expected planned, got %+v / created=%+v", res, dst.created)
	}
}

func TestMigrateSourceIPMappings_CreateError(t *testing.T) {
	dst := &fakeSourceIPMappingDest{
		existingRecords: []voiceml.VoiceV1IpRecord{{Sid: strp("ILnew"), IpAddress: strp("203.0.113.4")}},
		createErr:       errors.New("boom"),
	}
	domainDst := &fakeByocDomainDest{existing: []voiceml.SIPDomain{{Sid: "SDnew", DomainName: "acme.vml.voice.tel"}}}

	res, err := migrateSourceIPMappings(context.Background(), sampleMappingSource(), sampleDomainSource(), dst, domainDst, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusFailed) != 1 {
		t.Errorf("expected 1 failed item, got %+v", res)
	}
}

func TestMigrateSourceIPMappings_ListErrors(t *testing.T) {
	cases := []struct {
		name      string
		src       fakeSourceIPMappingSource
		domainSrc fakeByocDomainSource
		dst       *fakeSourceIPMappingDest
		domainDst *fakeByocDomainDest
	}{
		{"twilio mappings", fakeSourceIPMappingSource{mapErr: errors.New("x")}, fakeByocDomainSource{}, &fakeSourceIPMappingDest{}, &fakeByocDomainDest{}},
		{"twilio records", fakeSourceIPMappingSource{recordErr: errors.New("x")}, fakeByocDomainSource{}, &fakeSourceIPMappingDest{}, &fakeByocDomainDest{}},
		{"voiceml records", fakeSourceIPMappingSource{}, fakeByocDomainSource{}, &fakeSourceIPMappingDest{listRecordErr: errors.New("x")}, &fakeByocDomainDest{}},
		{"twilio domains", fakeSourceIPMappingSource{}, fakeByocDomainSource{err: errors.New("x")}, &fakeSourceIPMappingDest{}, &fakeByocDomainDest{}},
		{"voiceml domains", fakeSourceIPMappingSource{}, fakeByocDomainSource{}, &fakeSourceIPMappingDest{}, &fakeByocDomainDest{err: errors.New("x")}},
		{"voiceml mappings", sampleMappingSource(), sampleDomainSource(), &fakeSourceIPMappingDest{listMapErr: errors.New("x")}, &fakeByocDomainDest{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dst := c.dst
			if c.name == "voiceml mappings" {
				dst.existingRecords = []voiceml.VoiceV1IpRecord{{Sid: strp("ILnew"), IpAddress: strp("203.0.113.4")}}
				c.domainDst.existing = []voiceml.SIPDomain{{Sid: "SDnew", DomainName: "acme.vml.voice.tel"}}
			}
			if _, err := migrateSourceIPMappings(context.Background(), c.src, c.domainSrc, dst, c.domainDst, Options{}); err == nil {
				t.Errorf("%s: want error", c.name)
			}
		})
	}
}

func TestLabelOr(t *testing.T) {
	labels := map[string]string{"SID1": "known"}
	if got := labelOr(labels, "SID1"); got != "known" {
		t.Errorf("labelOr known = %q", got)
	}
	if got := labelOr(labels, "SID2"); got != "SID2" {
		t.Errorf("labelOr unknown = %q", got)
	}
	if got := labelOr(map[string]string{"SID3": ""}, "SID3"); got != "SID3" {
		t.Errorf("labelOr empty label = %q, want fallback to sid", got)
	}
}

func TestSourceIPMappingsName(t *testing.T) {
	if (SourceIPMappings{}).Name() != "source-ip-mappings" {
		t.Errorf("name=%q", (SourceIPMappings{}).Name())
	}
}

package migrate

import (
	"context"
	"errors"
	"testing"

	twvoice "github.com/twilio/twilio-go/rest/voice/v1"
	voiceml "github.com/voicetel/voiceml-go-sdk"
)

type fakeIPRecordSource struct {
	records []twvoice.VoiceV1IpRecord
	err     error
}

func (f fakeIPRecordSource) ListIpRecord(*twvoice.ListIpRecordParams) ([]twvoice.VoiceV1IpRecord, error) {
	return f.records, f.err
}

type fakeIPRecordDest struct {
	existing  []voiceml.VoiceV1IpRecord
	listErr   error
	createErr error
	created   []voiceml.CreateVoiceV1IpRecordParams
}

func (f *fakeIPRecordDest) ListIpRecords(context.Context, voiceml.V1PageParams) (*voiceml.VoiceV1IpRecordList, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return &voiceml.VoiceV1IpRecordList{IpRecords: f.existing}, nil
}

func (f *fakeIPRecordDest) CreateIpRecord(_ context.Context, p voiceml.CreateVoiceV1IpRecordParams) (*voiceml.VoiceV1IpRecord, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = append(f.created, p)
	return &voiceml.VoiceV1IpRecord{IpAddress: strp(p.IpAddress)}, nil
}

func TestMigrateIPRecords(t *testing.T) {
	src := fakeIPRecordSource{records: []twvoice.VoiceV1IpRecord{
		{IpAddress: strp("203.0.113.4"), FriendlyName: strp("hq"), CidrPrefixLength: 32},
		{IpAddress: strp("203.0.113.5")}, // exists → skip
		{IpAddress: nil},                 // no address → fail
	}}
	dst := &fakeIPRecordDest{existing: []voiceml.VoiceV1IpRecord{{IpAddress: strp("203.0.113.5")}}}

	res, err := migrateIPRecords(context.Background(), src, dst, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusCreated) != 1 || res.Count(StatusSkipped) != 1 || res.Count(StatusFailed) != 1 {
		t.Errorf("counts: %+v", res)
	}
	if len(dst.created) != 1 || dst.created[0].CidrPrefixLength == nil || *dst.created[0].CidrPrefixLength != 32 {
		t.Errorf("CIDR not mapped: %+v", dst.created)
	}
	if dst.created[0].FriendlyName == nil || *dst.created[0].FriendlyName != "hq" {
		t.Errorf("friendly name not mapped: %+v", dst.created)
	}
}

func TestMigrateIPRecords_NoCidr(t *testing.T) {
	src := fakeIPRecordSource{records: []twvoice.VoiceV1IpRecord{{IpAddress: strp("203.0.113.6"), CidrPrefixLength: 0}}}
	dst := &fakeIPRecordDest{}

	if _, err := migrateIPRecords(context.Background(), src, dst, Options{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(dst.created) != 1 || dst.created[0].CidrPrefixLength != nil {
		t.Errorf("expected no CIDR set, got %+v", dst.created)
	}
}

func TestMigrateIPRecordsDryRunAndErrors(t *testing.T) {
	src := fakeIPRecordSource{records: []twvoice.VoiceV1IpRecord{{IpAddress: strp("203.0.113.7")}}}

	res, err := migrateIPRecords(context.Background(), src, &fakeIPRecordDest{}, Options{DryRun: true})
	if err != nil || res.Count(StatusPlanned) != 1 {
		t.Fatalf("dry run: err=%v res=%+v", err, res)
	}

	res, _ = migrateIPRecords(context.Background(), src, &fakeIPRecordDest{createErr: errors.New("x")}, Options{})
	if res.Count(StatusFailed) != 1 {
		t.Errorf("create error: %+v", res)
	}

	if _, err := migrateIPRecords(context.Background(), fakeIPRecordSource{err: errors.New("d")}, &fakeIPRecordDest{}, Options{}); err == nil {
		t.Error("want source error")
	}
	if _, err := migrateIPRecords(context.Background(), src, &fakeIPRecordDest{listErr: errors.New("d")}, Options{}); err == nil {
		t.Error("want dest list error")
	}
}

func TestIPRecordsName(t *testing.T) {
	if (IPRecords{}).Name() != "ip-records" {
		t.Errorf("name=%q", (IPRecords{}).Name())
	}
}

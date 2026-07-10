package migrate

import (
	"context"
	"errors"
	"testing"

	twapi "github.com/twilio/twilio-go/rest/api/v2010"
	voiceml "github.com/voicetel/voiceml-go-sdk"
)

func strp(s string) *string { return &s }

type fakePhoneSource struct {
	nums []twapi.ApiV2010IncomingPhoneNumber
	err  error
}

func (f fakePhoneSource) ListIncomingPhoneNumber(*twapi.ListIncomingPhoneNumberParams) ([]twapi.ApiV2010IncomingPhoneNumber, error) {
	return f.nums, f.err
}

type fakePhoneDest struct {
	existing  []voiceml.IncomingPhoneNumber
	listErr   error
	createErr error
	created   []voiceml.CreateIncomingPhoneNumberParams
}

func (f *fakePhoneDest) List(context.Context, *voiceml.ListIncomingPhoneNumbersParams) (*voiceml.IncomingPhoneNumbersList, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return &voiceml.IncomingPhoneNumbersList{IncomingPhoneNumbers: f.existing}, nil
}

func (f *fakePhoneDest) Create(_ context.Context, p voiceml.CreateIncomingPhoneNumberParams) (*voiceml.IncomingPhoneNumber, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = append(f.created, p)
	return &voiceml.IncomingPhoneNumber{PhoneNumber: p.PhoneNumber}, nil
}

func TestMigratePhoneNumbers_CreatesAndSkips(t *testing.T) {
	src := fakePhoneSource{nums: []twapi.ApiV2010IncomingPhoneNumber{
		{PhoneNumber: strp("+12025550100"), VoiceUrl: strp("https://a.example/voice"), VoiceMethod: strp("POST")},
		{PhoneNumber: strp("+12025550101")}, // already on VoiceML → skipped
		{PhoneNumber: nil},                  // no number → failed
	}}
	dst := &fakePhoneDest{existing: []voiceml.IncomingPhoneNumber{{PhoneNumber: "+12025550101"}}}

	res, err := migratePhoneNumbers(context.Background(), src, dst, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if got := res.Count(StatusCreated); got != 1 {
		t.Errorf("created=%d want 1", got)
	}
	if got := res.Count(StatusSkipped); got != 1 {
		t.Errorf("skipped=%d want 1", got)
	}
	if got := res.Count(StatusFailed); got != 1 {
		t.Errorf("failed=%d want 1", got)
	}
	if len(dst.created) != 1 || dst.created[0].PhoneNumber != "+12025550100" {
		t.Errorf("unexpected create calls: %+v", dst.created)
	}
	if dst.created[0].VoiceURL == nil || *dst.created[0].VoiceURL != "https://a.example/voice" {
		t.Errorf("voice url not mapped: %+v", dst.created[0])
	}
}

func TestMigratePhoneNumbers_DryRun(t *testing.T) {
	src := fakePhoneSource{nums: []twapi.ApiV2010IncomingPhoneNumber{{PhoneNumber: strp("+12025550100")}}}
	dst := &fakePhoneDest{}

	res, err := migratePhoneNumbers(context.Background(), src, dst, Options{DryRun: true})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if got := res.Count(StatusPlanned); got != 1 {
		t.Errorf("planned=%d want 1", got)
	}
	if len(dst.created) != 0 {
		t.Errorf("dry run must not create: %+v", dst.created)
	}
}

func TestMigratePhoneNumbers_CreateError(t *testing.T) {
	src := fakePhoneSource{nums: []twapi.ApiV2010IncomingPhoneNumber{{PhoneNumber: strp("+12025550100")}}}
	dst := &fakePhoneDest{createErr: errors.New("boom")}

	res, err := migratePhoneNumbers(context.Background(), src, dst, Options{})
	if err != nil {
		t.Fatalf("migrate should not hard-fail on per-item create error: %v", err)
	}
	if got := res.Count(StatusFailed); got != 1 {
		t.Errorf("failed=%d want 1", got)
	}
}

func TestMigratePhoneNumbers_ListErrors(t *testing.T) {
	if _, err := migratePhoneNumbers(context.Background(),
		fakePhoneSource{err: errors.New("twilio down")}, &fakePhoneDest{}, Options{}); err == nil {
		t.Error("expected source list error")
	}
	if _, err := migratePhoneNumbers(context.Background(),
		fakePhoneSource{}, &fakePhoneDest{listErr: errors.New("voiceml down")}, Options{}); err == nil {
		t.Error("expected dest list error")
	}
}

func TestPhoneNumbersName(t *testing.T) {
	if (PhoneNumbers{}).Name() != "phone-numbers" {
		t.Errorf("name=%q", (PhoneNumbers{}).Name())
	}
}

func TestDeref(t *testing.T) {
	if deref(nil) != "" {
		t.Error("nil deref should be empty")
	}
	if deref(strp("x")) != "x" {
		t.Error("deref failed")
	}
}

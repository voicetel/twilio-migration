package migrate

import (
	"context"
	"errors"
	"strings"
	"testing"

	twapi "github.com/twilio/twilio-go/rest/api/v2010"
	voiceml "github.com/voicetel/voiceml-go-sdk"
)

func strp(s string) *string { return &s }
func boolp(b bool) *bool    { return &b }

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

	res, err := migratePhoneNumbers(context.Background(), src, dst, fakeAppSource{}, &fakeAppDest{}, Options{})
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

	res, err := migratePhoneNumbers(context.Background(), src, dst, fakeAppSource{}, &fakeAppDest{}, Options{DryRun: true})
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

	res, err := migratePhoneNumbers(context.Background(), src, dst, fakeAppSource{}, &fakeAppDest{}, Options{})
	if err != nil {
		t.Fatalf("migrate should not hard-fail on per-item create error: %v", err)
	}
	if got := res.Count(StatusFailed); got != 1 {
		t.Errorf("failed=%d want 1", got)
	}
}

func TestMigratePhoneNumbers_ListErrors(t *testing.T) {
	if _, err := migratePhoneNumbers(context.Background(), fakePhoneSource{err: errors.New("twilio down")}, &fakePhoneDest{}, fakeAppSource{}, &fakeAppDest{}, Options{}); err == nil {
		t.Error("expected source list error")
	}
	if _, err := migratePhoneNumbers(context.Background(), fakePhoneSource{}, &fakePhoneDest{listErr: errors.New("voiceml down")}, fakeAppSource{}, &fakeAppDest{}, Options{}); err == nil {
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

// recordingAppDest is a fakeAppDest that records the page size it was asked for.
type recordingAppDest struct {
	fakeAppDest
	pageSize *int
	calls    int
}

func (f *recordingAppDest) List(ctx context.Context, p voiceml.ListApplicationsParams) (*voiceml.ApplicationList, error) {
	f.pageSize = p.PageSize
	f.calls++

	return f.fakeAppDest.List(ctx, p)
}

func TestMigratePhoneNumbers_CarriesSMSAndStatusCallback(t *testing.T) {
	src := fakePhoneSource{nums: []twapi.ApiV2010IncomingPhoneNumber{{
		PhoneNumber:          strp("+12025550100"),
		SmsUrl:               strp("https://a.example/sms"),
		SmsMethod:            strp("GET"),
		SmsFallbackUrl:       strp("https://a.example/sms-fb"),
		SmsFallbackMethod:    strp("POST"),
		StatusCallback:       strp("https://a.example/status"),
		StatusCallbackMethod: strp("POST"),
	}}}
	dst := &fakePhoneDest{}
	apps := &recordingAppDest{}

	if _, err := migratePhoneNumbers(context.Background(), src, dst, fakeAppSource{}, apps, Options{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if apps.calls != 0 {
		t.Errorf("application lookups=%d want 0 for an unbound number", apps.calls)
	}

	if len(dst.created) != 1 {
		t.Fatalf("created=%+v", dst.created)
	}

	got := dst.created[0]
	for name, pair := range map[string][2]*string{
		"SmsURL":               {got.SmsURL, strp("https://a.example/sms")},
		"SmsMethod":            {got.SmsMethod, strp("GET")},
		"SmsFallbackURL":       {got.SmsFallbackURL, strp("https://a.example/sms-fb")},
		"SmsFallbackMethod":    {got.SmsFallbackMethod, strp("POST")},
		"StatusCallback":       {got.StatusCallback, strp("https://a.example/status")},
		"StatusCallbackMethod": {got.StatusCallbackMethod, strp("POST")},
	} {
		if deref(pair[0]) != deref(pair[1]) {
			t.Errorf("%s=%q want %q", name, deref(pair[0]), deref(pair[1]))
		}
	}

	if got.VoiceApplicationSid != nil || got.SmsApplicationSid != nil {
		t.Errorf("unbound number got bindings: %+v", got)
	}
}

func TestMigratePhoneNumbers_LeavesEmptyValuesOut(t *testing.T) {
	src := fakePhoneSource{nums: []twapi.ApiV2010IncomingPhoneNumber{{
		PhoneNumber:         strp("+12025550100"),
		SmsUrl:              strp(""),
		StatusCallback:      strp(""),
		VoiceApplicationSid: strp(""),
		SmsApplicationSid:   strp(""),
	}}}
	dst := &fakePhoneDest{}

	if _, err := migratePhoneNumbers(context.Background(), src, dst, fakeAppSource{}, &fakeAppDest{}, Options{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	got := dst.created[0]
	if got.SmsURL != nil || got.StatusCallback != nil || got.VoiceApplicationSid != nil || got.SmsApplicationSid != nil {
		t.Errorf("empty values were sent: %+v", got)
	}
}

func TestMigratePhoneNumbers_RemapsApplicationBindings(t *testing.T) {
	src := fakePhoneSource{nums: []twapi.ApiV2010IncomingPhoneNumber{{
		PhoneNumber:         strp("+12025550100"),
		VoiceApplicationSid: strp("APtwiliovoice"),
		SmsApplicationSid:   strp("APtwiliosms"),
	}}}
	appSrc := fakeAppSource{apps: []twapi.ApiV2010Application{
		{Sid: strp("APtwiliovoice"), FriendlyName: strp("Voice App")},
		{Sid: strp("APtwiliosms"), FriendlyName: strp("SMS App")},
		{Sid: strp("APnotmigrated"), FriendlyName: strp("Elsewhere")},
	}}
	appDst := &recordingAppDest{fakeAppDest: fakeAppDest{existing: []voiceml.Application{
		{Sid: "APvoicemlvoice", FriendlyName: "Voice App"},
		{Sid: "APvoicemlsms", FriendlyName: "SMS App"},
	}}}
	dst := &fakePhoneDest{}

	res, err := migratePhoneNumbers(context.Background(), src, dst, appSrc, appDst, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if res.Count(StatusCreated) != 1 {
		t.Fatalf("result=%+v", res)
	}

	got := dst.created[0]
	if deref(got.VoiceApplicationSid) != "APvoicemlvoice" || deref(got.SmsApplicationSid) != "APvoicemlsms" {
		t.Errorf("bindings voice=%q sms=%q", deref(got.VoiceApplicationSid), deref(got.SmsApplicationSid))
	}

	if appDst.pageSize == nil || *appDst.pageSize != appLookupPageSize {
		t.Errorf("page size=%v want %d", appDst.pageSize, appLookupPageSize)
	}
}

func TestMigratePhoneNumbers_UnresolvedBindingFailsTheNumber(t *testing.T) {
	cases := []struct {
		name string
		num  twapi.ApiV2010IncomingPhoneNumber
		want string
	}{
		{"voice", twapi.ApiV2010IncomingPhoneNumber{PhoneNumber: strp("+12025550100"), VoiceApplicationSid: strp("APgone")}, "voice application APgone"},
		{"sms", twapi.ApiV2010IncomingPhoneNumber{PhoneNumber: strp("+12025550100"), SmsApplicationSid: strp("APgone")}, "SMS application APgone"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dst := &fakePhoneDest{}
			src := fakePhoneSource{nums: []twapi.ApiV2010IncomingPhoneNumber{tc.num}}

			res, err := migratePhoneNumbers(context.Background(), src, dst, fakeAppSource{}, &fakeAppDest{}, Options{})
			if err != nil {
				t.Fatalf("migrate: %v", err)
			}

			if res.Count(StatusFailed) != 1 || len(dst.created) != 0 {
				t.Fatalf("result=%+v created=%+v", res, dst.created)
			}

			if !strings.Contains(res.Items[0].Detail, tc.want) {
				t.Errorf("detail=%q want %q", res.Items[0].Detail, tc.want)
			}
		})
	}
}

func TestMigratePhoneNumbers_ApplicationLookupErrors(t *testing.T) {
	src := fakePhoneSource{nums: []twapi.ApiV2010IncomingPhoneNumber{{PhoneNumber: strp("+12025550100"), VoiceApplicationSid: strp("AP1")}}}
	boom := errors.New("boom")

	if _, err := migratePhoneNumbers(context.Background(), src, &fakePhoneDest{}, fakeAppSource{err: boom}, &fakeAppDest{}, Options{}); !errors.Is(err, boom) {
		t.Errorf("source error=%v", err)
	}

	if _, err := migratePhoneNumbers(context.Background(), src, &fakePhoneDest{}, fakeAppSource{}, &fakeAppDest{listErr: boom}, Options{}); !errors.Is(err, boom) {
		t.Errorf("dest error=%v", err)
	}
}

func TestMigratePhoneNumbers_NoApplicationLookupWhenNotNeeded(t *testing.T) {
	bound := twapi.ApiV2010IncomingPhoneNumber{PhoneNumber: strp("+12025550100"), VoiceApplicationSid: strp("AP1")}

	t.Run("dry run", func(t *testing.T) {
		apps := &recordingAppDest{}
		src := fakePhoneSource{nums: []twapi.ApiV2010IncomingPhoneNumber{bound}}

		res, err := migratePhoneNumbers(context.Background(), src, &fakePhoneDest{}, fakeAppSource{}, apps, Options{DryRun: true})
		if err != nil || res.Count(StatusPlanned) != 1 || apps.calls != 0 {
			t.Errorf("err=%v result=%+v lookups=%d", err, res, apps.calls)
		}
	})

	t.Run("already on VoiceML", func(t *testing.T) {
		apps := &recordingAppDest{}
		src := fakePhoneSource{nums: []twapi.ApiV2010IncomingPhoneNumber{bound}}
		dst := &fakePhoneDest{existing: []voiceml.IncomingPhoneNumber{{PhoneNumber: "+12025550100"}}}

		res, err := migratePhoneNumbers(context.Background(), src, dst, fakeAppSource{}, apps, Options{})
		if err != nil || res.Count(StatusSkipped) != 1 || apps.calls != 0 {
			t.Errorf("err=%v result=%+v lookups=%d", err, res, apps.calls)
		}
	})
}

func TestDefaultRunsApplicationsBeforePhoneNumbers(t *testing.T) {
	apps, nums := -1, -1

	for i, m := range Default() {
		switch m.Name() {
		case "applications":
			apps = i
		case "phone-numbers":
			nums = i
		}
	}

	if apps < 0 || nums < 0 || apps > nums {
		t.Errorf("applications at %d, phone-numbers at %d", apps, nums)
	}
}

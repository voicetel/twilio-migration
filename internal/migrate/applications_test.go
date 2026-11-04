package migrate

import (
	"context"
	"errors"
	"testing"

	twapi "github.com/twilio/twilio-go/rest/api/v2010"
	voiceml "github.com/voicetel/voiceml-go-sdk"
)

type fakeAppSource struct {
	apps []twapi.ApiV2010Application
	err  error
}

func (f fakeAppSource) ListApplication(*twapi.ListApplicationParams) ([]twapi.ApiV2010Application, error) {
	return f.apps, f.err
}

type fakeAppDest struct {
	existing  []voiceml.Application
	listErr   error
	createErr error
	created   []voiceml.ApplicationParams
}

func (f *fakeAppDest) List(context.Context, voiceml.ListApplicationsParams) (*voiceml.ApplicationList, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return &voiceml.ApplicationList{Applications: f.existing}, nil
}

func (f *fakeAppDest) Create(_ context.Context, p voiceml.ApplicationParams) (*voiceml.Application, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = append(f.created, p)
	return &voiceml.Application{FriendlyName: deref(p.FriendlyName)}, nil
}

func TestMigrateApplications_CreatesSkipsFails(t *testing.T) {
	src := fakeAppSource{apps: []twapi.ApiV2010Application{
		{FriendlyName: strp("Voice App"), VoiceUrl: strp("https://a.example/voice"), VoiceMethod: strp("POST")},
		{FriendlyName: strp("Existing")}, // already present → skip
		{FriendlyName: nil},              // no name → failed
	}}
	dst := &fakeAppDest{existing: []voiceml.Application{{FriendlyName: "Existing"}}}

	res, err := migrateApplications(context.Background(), src, dst, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusCreated) != 1 || res.Count(StatusSkipped) != 1 || res.Count(StatusFailed) != 1 {
		t.Errorf("unexpected counts: %+v", res)
	}
	if len(dst.created) != 1 || dst.created[0].FriendlyName == nil || *dst.created[0].FriendlyName != "Voice App" {
		t.Errorf("unexpected create: %+v", dst.created)
	}
}

func TestMigrateApplications_DryRunAndErrors(t *testing.T) {
	src := fakeAppSource{apps: []twapi.ApiV2010Application{{FriendlyName: strp("A")}}}
	res, err := migrateApplications(context.Background(), src, &fakeAppDest{}, Options{DryRun: true})
	if err != nil || res.Count(StatusPlanned) != 1 {
		t.Fatalf("dry run: err=%v res=%+v", err, res)
	}

	res, _ = migrateApplications(context.Background(), src, &fakeAppDest{createErr: errors.New("x")}, Options{})
	if res.Count(StatusFailed) != 1 {
		t.Errorf("create error not recorded: %+v", res)
	}

	if _, err := migrateApplications(context.Background(), fakeAppSource{err: errors.New("down")}, &fakeAppDest{}, Options{}); err == nil {
		t.Error("expected source error")
	}
	if _, err := migrateApplications(context.Background(), src, &fakeAppDest{listErr: errors.New("down")}, Options{}); err == nil {
		t.Error("expected dest list error")
	}
}

func TestApplicationsName(t *testing.T) {
	if (Applications{}).Name() != "applications" {
		t.Errorf("name=%q", Applications{}.Name())
	}
}

func TestMigrateApplications_CarriesEveryMappedField(t *testing.T) {
	src := fakeAppSource{apps: []twapi.ApiV2010Application{{
		FriendlyName:                    strp("Full"),
		VoiceFallbackUrl:                strp("https://a.example/voice-fb"),
		VoiceFallbackMethod:             strp("GET"),
		VoiceCallerIdLookup:             boolp(true),
		MessageStatusCallback:           strp("https://a.example/dlr"),
		SmsStatusCallback:               strp("https://a.example/sms-status"),
		SmsUrl:                          strp("https://a.example/sms"),
		SmsMethod:                       strp("GET"),
		SmsFallbackUrl:                  strp("https://a.example/sms-fb"),
		SmsFallbackMethod:               strp("POST"),
		PublicApplicationConnectEnabled: boolp(true),
	}, {
		FriendlyName: strp("Empty"),
		SmsUrl:       strp(""),
	}}}
	dst := &fakeAppDest{}

	if _, err := migrateApplications(context.Background(), src, dst, Options{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if len(dst.created) != 2 {
		t.Fatalf("created=%+v", dst.created)
	}

	got := dst.created[0]
	for name, pair := range map[string][2]string{
		"VoiceFallbackURL":      {deref(got.VoiceFallbackURL), "https://a.example/voice-fb"},
		"VoiceFallbackMethod":   {deref(got.VoiceFallbackMethod), "GET"},
		"MessageStatusCallback": {deref(got.MessageStatusCallback), "https://a.example/dlr"},
		"SmsStatusCallback":     {deref(got.SmsStatusCallback), "https://a.example/sms-status"},
		"SmsURL":                {deref(got.SmsURL), "https://a.example/sms"},
		"SmsMethod":             {deref(got.SmsMethod), "GET"},
		"SmsFallbackURL":        {deref(got.SmsFallbackURL), "https://a.example/sms-fb"},
		"SmsFallbackMethod":     {deref(got.SmsFallbackMethod), "POST"},
	} {
		if pair[0] != pair[1] {
			t.Errorf("%s=%q want %q", name, pair[0], pair[1])
		}
	}

	if got.VoiceCallerIDLookup == nil || !*got.VoiceCallerIDLookup {
		t.Errorf("VoiceCallerIDLookup=%v", got.VoiceCallerIDLookup)
	}

	if got.PublicApplicationConnectEnabled == nil || !*got.PublicApplicationConnectEnabled {
		t.Errorf("PublicApplicationConnectEnabled=%v", got.PublicApplicationConnectEnabled)
	}

	if dst.created[1].SmsURL != nil {
		t.Errorf("an empty SmsUrl was sent: %q", deref(dst.created[1].SmsURL))
	}
}

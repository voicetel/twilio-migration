package migrate

import (
	"context"
	"errors"
	"testing"

	twmsg "github.com/twilio/twilio-go/rest/messaging/v1"
	voiceml "github.com/voicetel/voiceml-go-sdk"
)

type fakeMsgSource struct {
	services []twmsg.MessagingV1Service
	err      error
}

func (f fakeMsgSource) ListService(*twmsg.ListServiceParams) ([]twmsg.MessagingV1Service, error) {
	return f.services, f.err
}

type fakeMsgDest struct {
	existing  []voiceml.MessagingService
	listErr   error
	createErr error
	created   []voiceml.CreateMessagingServiceParams
}

func (f *fakeMsgDest) List(context.Context, voiceml.V1PageParams) (*voiceml.MessagingServiceList, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return &voiceml.MessagingServiceList{Services: f.existing}, nil
}

func (f *fakeMsgDest) Create(_ context.Context, p voiceml.CreateMessagingServiceParams) (*voiceml.MessagingService, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = append(f.created, p)
	return &voiceml.MessagingService{FriendlyName: strp(p.FriendlyName)}, nil
}

func TestMigrateMessaging(t *testing.T) {
	src := fakeMsgSource{services: []twmsg.MessagingV1Service{
		{FriendlyName: strp("Alerts"), InboundRequestUrl: strp("https://a.example/sms")},
		{FriendlyName: strp("Existing")}, // skip
		{FriendlyName: nil},              // fail
	}}
	dst := &fakeMsgDest{existing: []voiceml.MessagingService{{FriendlyName: strp("Existing")}}}

	res, err := migrateMessaging(context.Background(), src, dst, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusCreated) != 1 || res.Count(StatusSkipped) != 1 || res.Count(StatusFailed) != 1 {
		t.Errorf("counts: %+v", res)
	}
	if len(dst.created) != 1 || dst.created[0].InboundRequestURL == nil || *dst.created[0].InboundRequestURL != "https://a.example/sms" {
		t.Errorf("inbound url not mapped: %+v", dst.created)
	}
}

func TestMigrateMessagingDryRunAndErrors(t *testing.T) {
	src := fakeMsgSource{services: []twmsg.MessagingV1Service{{FriendlyName: strp("M")}}}
	res, err := migrateMessaging(context.Background(), src, &fakeMsgDest{}, Options{DryRun: true})
	if err != nil || res.Count(StatusPlanned) != 1 {
		t.Fatalf("dry run: err=%v res=%+v", err, res)
	}
	res, _ = migrateMessaging(context.Background(), src, &fakeMsgDest{createErr: errors.New("x")}, Options{})
	if res.Count(StatusFailed) != 1 {
		t.Errorf("create error: %+v", res)
	}
	if _, err := migrateMessaging(context.Background(), fakeMsgSource{err: errors.New("d")}, &fakeMsgDest{}, Options{}); err == nil {
		t.Error("want source error")
	}
	if _, err := migrateMessaging(context.Background(), src, &fakeMsgDest{listErr: errors.New("d")}, Options{}); err == nil {
		t.Error("want dest list error")
	}
}

func TestMessagingName(t *testing.T) {
	if (Messaging{}).Name() != "messaging" {
		t.Errorf("name=%q", (Messaging{}).Name())
	}
}

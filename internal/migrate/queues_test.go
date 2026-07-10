package migrate

import (
	"context"
	"errors"
	"testing"

	twapi "github.com/twilio/twilio-go/rest/api/v2010"
	voiceml "github.com/voicetel/voiceml-go-sdk"
)

type fakeQueueSource struct {
	queues []twapi.ApiV2010Queue
	err    error
}

func (f fakeQueueSource) ListQueue(*twapi.ListQueueParams) ([]twapi.ApiV2010Queue, error) {
	return f.queues, f.err
}

type fakeQueueDest struct {
	existing  []voiceml.Queue
	listErr   error
	createErr error
	created   []voiceml.CreateQueueParams
}

func (f *fakeQueueDest) List(context.Context, voiceml.ListPageParams) (*voiceml.QueueList, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return &voiceml.QueueList{Queues: f.existing}, nil
}

func (f *fakeQueueDest) Create(_ context.Context, p voiceml.CreateQueueParams) (*voiceml.Queue, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = append(f.created, p)
	return &voiceml.Queue{FriendlyName: p.FriendlyName}, nil
}

func TestMigrateQueues(t *testing.T) {
	src := fakeQueueSource{queues: []twapi.ApiV2010Queue{
		{FriendlyName: strp("Support"), MaxSize: 50},
		{FriendlyName: strp("Sales")}, // exists → skip
		{FriendlyName: nil},           // no name → fail
	}}
	dst := &fakeQueueDest{existing: []voiceml.Queue{{FriendlyName: "Sales"}}}

	res, err := migrateQueues(context.Background(), src, dst, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusCreated) != 1 || res.Count(StatusSkipped) != 1 || res.Count(StatusFailed) != 1 {
		t.Errorf("counts: %+v", res)
	}
	if len(dst.created) != 1 || dst.created[0].MaxSize == nil || *dst.created[0].MaxSize != 50 {
		t.Errorf("max size not mapped: %+v", dst.created)
	}
}

func TestMigrateQueuesDryRunAndErrors(t *testing.T) {
	src := fakeQueueSource{queues: []twapi.ApiV2010Queue{{FriendlyName: strp("Q")}}}
	res, err := migrateQueues(context.Background(), src, &fakeQueueDest{}, Options{DryRun: true})
	if err != nil || res.Count(StatusPlanned) != 1 {
		t.Fatalf("dry run: err=%v res=%+v", err, res)
	}
	res, _ = migrateQueues(context.Background(), src, &fakeQueueDest{createErr: errors.New("x")}, Options{})
	if res.Count(StatusFailed) != 1 {
		t.Errorf("create error: %+v", res)
	}
	if _, err := migrateQueues(context.Background(), fakeQueueSource{err: errors.New("d")}, &fakeQueueDest{}, Options{}); err == nil {
		t.Error("want source error")
	}
	if _, err := migrateQueues(context.Background(), src, &fakeQueueDest{listErr: errors.New("d")}, Options{}); err == nil {
		t.Error("want dest list error")
	}
}

func TestQueuesName(t *testing.T) {
	if (Queues{}).Name() != "queues" {
		t.Errorf("name=%q", (Queues{}).Name())
	}
}

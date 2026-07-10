package migrate

import (
	"context"
	"fmt"

	twapi "github.com/twilio/twilio-go/rest/api/v2010"
	voiceml "github.com/voicetel/voiceml-go-sdk"
)

type queueSource interface {
	ListQueue(params *twapi.ListQueueParams) ([]twapi.ApiV2010Queue, error)
}

type queueDest interface {
	List(ctx context.Context, params voiceml.ListPageParams) (*voiceml.QueueList, error)
	Create(ctx context.Context, params voiceml.CreateQueueParams) (*voiceml.Queue, error)
}

// Queues migrates call Queues (friendly name + max size). Idempotent by
// friendly name.
type Queues struct{}

// Name implements Migrator.
func (Queues) Name() string { return "queues" }

// Migrate implements Migrator.
func (Queues) Migrate(ctx context.Context, c *Clients, opts Options) (Result, error) {
	return migrateQueues(ctx, c.Twilio, c.VoiceML.Queues, opts)
}

func migrateQueues(ctx context.Context, src queueSource, dst queueDest, opts Options) (Result, error) {
	res := Result{Resource: "queues"}

	queues, err := src.ListQueue(&twapi.ListQueueParams{})
	if err != nil {
		return res, fmt.Errorf("list Twilio queues: %w", err)
	}

	existing, err := dst.List(ctx, voiceml.ListPageParams{})
	if err != nil {
		return res, fmt.Errorf("list VoiceML queues: %w", err)
	}

	have := make(map[string]bool, len(existing.Queues))
	for _, q := range existing.Queues {
		have[q.FriendlyName] = true
	}

	for _, q := range queues {
		name := deref(q.FriendlyName)
		item := ItemResult{ID: name}

		switch {
		case name == "":
			item.Status = StatusFailed
			item.Detail = "source queue has no friendly_name"
		case have[name]:
			item.Status = StatusSkipped
			item.Detail = "already present on VoiceML"
		case opts.DryRun:
			item.Status = StatusPlanned
		default:
			params := voiceml.CreateQueueParams{FriendlyName: name}
			if q.MaxSize > 0 {
				ms := q.MaxSize
				params.MaxSize = &ms
			}
			if _, cErr := dst.Create(ctx, params); cErr != nil {
				item.Status = StatusFailed
				item.Detail = cErr.Error()
			} else {
				item.Status = StatusCreated
			}
		}

		res.Items = append(res.Items, item)
	}

	return res, nil
}

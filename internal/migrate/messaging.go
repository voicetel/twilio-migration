package migrate

import (
	"context"
	"fmt"

	twmsg "github.com/twilio/twilio-go/rest/messaging/v1"
	voiceml "github.com/voicetel/voiceml-go-sdk"
)

type messagingSource interface {
	ListService(params *twmsg.ListServiceParams) ([]twmsg.MessagingV1Service, error)
}

type messagingDest interface {
	List(ctx context.Context, params voiceml.V1PageParams) (*voiceml.MessagingServiceList, error)
	Create(ctx context.Context, params voiceml.CreateMessagingServiceParams) (*voiceml.MessagingService, error)
}

// Messaging migrates Messaging Services (friendly name + inbound/fallback
// webhooks). Idempotent by friendly name.
type Messaging struct{}

// Name implements Migrator.
func (Messaging) Name() string { return "messaging" }

// Migrate implements Migrator.
func (Messaging) Migrate(ctx context.Context, c *Clients, opts Options) (Result, error) {
	return migrateMessaging(ctx, c.TwilioMessaging, c.VoiceML.MessagingV1.Services, opts)
}

func migrateMessaging(ctx context.Context, src messagingSource, dst messagingDest, opts Options) (Result, error) {
	res := Result{Resource: "messaging"}

	services, err := src.ListService(&twmsg.ListServiceParams{})
	if err != nil {
		return res, fmt.Errorf("list Twilio messaging services: %w", err)
	}

	existing, err := dst.List(ctx, voiceml.V1PageParams{})
	if err != nil {
		return res, fmt.Errorf("list VoiceML messaging services: %w", err)
	}

	have := make(map[string]bool, len(existing.Services))
	for _, s := range existing.Services {
		have[deref(s.FriendlyName)] = true
	}

	for _, s := range services {
		name := deref(s.FriendlyName)
		item := ItemResult{ID: name}

		switch {
		case name == "":
			item.Status = StatusFailed
			item.Detail = "source messaging service has no friendly_name"
		case have[name]:
			item.Status = StatusSkipped
			item.Detail = "already present on VoiceML"
		case opts.DryRun:
			item.Status = StatusPlanned
		default:
			if _, cErr := dst.Create(ctx, voiceml.CreateMessagingServiceParams{
				FriendlyName:      name,
				InboundRequestURL: s.InboundRequestUrl,
				InboundMethod:     s.InboundMethod,
				FallbackURL:       s.FallbackUrl,
				FallbackMethod:    s.FallbackMethod,
				StatusCallback:    s.StatusCallback,
			}); cErr != nil {
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

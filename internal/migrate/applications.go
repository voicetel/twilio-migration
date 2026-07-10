package migrate

import (
	"context"
	"fmt"

	twapi "github.com/twilio/twilio-go/rest/api/v2010"
	voiceml "github.com/voicetel/voiceml-go-sdk"
)

// appSource is the slice of twilio-go used to read TwiML applications.
type appSource interface {
	ListApplication(params *twapi.ListApplicationParams) ([]twapi.ApiV2010Application, error)
}

// appDest is the slice of voiceml-go-sdk used to read/write applications.
type appDest interface {
	List(ctx context.Context, params voiceml.ListApplicationsParams) (*voiceml.ApplicationList, error)
	Create(ctx context.Context, params voiceml.ApplicationParams) (*voiceml.Application, error)
}

// Applications migrates TwiML Applications (voice/SMS request URLs + friendly
// name). Idempotent by friendly name: an application whose friendly name
// already exists on VoiceML is skipped.
type Applications struct{}

// Name implements Migrator.
func (Applications) Name() string { return "applications" }

// Migrate implements Migrator.
func (Applications) Migrate(ctx context.Context, c *Clients, opts Options) (Result, error) {
	return migrateApplications(ctx, c.Twilio, c.VoiceML.Applications, opts)
}

func migrateApplications(ctx context.Context, src appSource, dst appDest, opts Options) (Result, error) {
	res := Result{Resource: "applications"}

	apps, err := src.ListApplication(&twapi.ListApplicationParams{})
	if err != nil {
		return res, fmt.Errorf("list Twilio applications: %w", err)
	}

	existing, err := dst.List(ctx, voiceml.ListApplicationsParams{})
	if err != nil {
		return res, fmt.Errorf("list VoiceML applications: %w", err)
	}

	have := make(map[string]bool, len(existing.Applications))
	for _, a := range existing.Applications {
		have[a.FriendlyName] = true
	}

	for _, a := range apps {
		name := deref(a.FriendlyName)
		item := ItemResult{ID: name}

		switch {
		case name == "":
			item.Status = StatusFailed
			item.Detail = "source application has no friendly_name"
		case have[name]:
			item.Status = StatusSkipped
			item.Detail = "already present on VoiceML"
		case opts.DryRun:
			item.Status = StatusPlanned
		default:
			if _, cErr := dst.Create(ctx, voiceml.ApplicationParams{
				FriendlyName:         a.FriendlyName,
				VoiceURL:             a.VoiceUrl,
				VoiceMethod:          a.VoiceMethod,
				StatusCallback:       a.StatusCallback,
				StatusCallbackMethod: a.StatusCallbackMethod,
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

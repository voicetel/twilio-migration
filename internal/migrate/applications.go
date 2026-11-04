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

// Applications migrates TwiML Applications: friendly name, voice and SMS request
// URLs with their fallbacks, status callbacks, caller-ID lookup and the public
// Connect flag. Idempotent by friendly name: an application whose friendly name
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
			if _, cErr := dst.Create(ctx, applicationParams(a)); cErr != nil {
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

// applicationParams maps a Twilio application onto VoiceML's create body. Twilio
// reports an unset URL as an empty string, which is left out rather than sent.
func applicationParams(a twapi.ApiV2010Application) voiceml.ApplicationParams {
	return voiceml.ApplicationParams{
		FriendlyName:                    a.FriendlyName,
		VoiceURL:                        a.VoiceUrl,
		VoiceMethod:                     a.VoiceMethod,
		VoiceFallbackURL:                nonEmpty(a.VoiceFallbackUrl),
		VoiceFallbackMethod:             nonEmpty(a.VoiceFallbackMethod),
		VoiceCallerIDLookup:             a.VoiceCallerIdLookup,
		StatusCallback:                  a.StatusCallback,
		StatusCallbackMethod:            a.StatusCallbackMethod,
		MessageStatusCallback:           nonEmpty(a.MessageStatusCallback),
		SmsStatusCallback:               nonEmpty(a.SmsStatusCallback),
		SmsURL:                          nonEmpty(a.SmsUrl),
		SmsMethod:                       nonEmpty(a.SmsMethod),
		SmsFallbackURL:                  nonEmpty(a.SmsFallbackUrl),
		SmsFallbackMethod:               nonEmpty(a.SmsFallbackMethod),
		PublicApplicationConnectEnabled: a.PublicApplicationConnectEnabled,
	}
}

// appLookupPageSize is the largest page the API serves. The listing behind
// applicationSIDMap is one page, so an account would need more applications
// than this before a binding could be reported unresolved by mistake.
const appLookupPageSize = 1000

// applicationSIDMap maps each Twilio application SID to the SID of the VoiceML
// application with the same friendly name, the key the Applications migrator
// creates them under. An application with no match on VoiceML has no entry.
func applicationSIDMap(ctx context.Context, src appSource, dst appDest) (map[string]string, error) {
	apps, err := src.ListApplication(&twapi.ListApplicationParams{})
	if err != nil {
		return nil, fmt.Errorf("list Twilio applications: %w", err)
	}

	pageSize := appLookupPageSize

	existing, err := dst.List(ctx, voiceml.ListApplicationsParams{PageSize: &pageSize})
	if err != nil {
		return nil, fmt.Errorf("list VoiceML applications: %w", err)
	}

	byName := make(map[string]string, len(existing.Applications))
	for _, a := range existing.Applications {
		byName[a.FriendlyName] = a.Sid
	}

	out := make(map[string]string, len(apps))
	for _, a := range apps {
		if sid, ok := byName[deref(a.FriendlyName)]; ok {
			out[deref(a.Sid)] = sid
		}
	}

	return out, nil
}

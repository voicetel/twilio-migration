package migrate

import (
	"context"
	"fmt"

	twapi "github.com/twilio/twilio-go/rest/api/v2010"
	voiceml "github.com/voicetel/voiceml-go-sdk"
)

// phoneSource is the slice of twilio-go used to read phone numbers.
type phoneSource interface {
	ListIncomingPhoneNumber(params *twapi.ListIncomingPhoneNumberParams) ([]twapi.ApiV2010IncomingPhoneNumber, error)
}

// phoneDest is the slice of voiceml-go-sdk used to read/write phone numbers.
type phoneDest interface {
	List(ctx context.Context, params *voiceml.ListIncomingPhoneNumbersParams) (*voiceml.IncomingPhoneNumbersList, error)
	Create(ctx context.Context, params voiceml.CreateIncomingPhoneNumberParams) (*voiceml.IncomingPhoneNumber, error)
}

// PhoneNumbers migrates IncomingPhoneNumbers (DIDs) and their voice request
// URLs. Idempotent: a number already present on VoiceML is skipped.
//
// Note: VoiceML's create accepts the voice request URLs; SMS URLs, friendly
// name and status-callback are not part of the create body and are left for a
// follow-up Update pass (tracked in the README).
type PhoneNumbers struct{}

// Name implements Migrator.
func (PhoneNumbers) Name() string { return "phone-numbers" }

// Migrate implements Migrator.
func (PhoneNumbers) Migrate(ctx context.Context, c *Clients, opts Options) (Result, error) {
	return migratePhoneNumbers(ctx, c.Twilio, c.VoiceML.IncomingPhoneNumbers, opts)
}

func migratePhoneNumbers(ctx context.Context, src phoneSource, dst phoneDest, opts Options) (Result, error) {
	res := Result{Resource: "phone-numbers"}

	nums, err := src.ListIncomingPhoneNumber(&twapi.ListIncomingPhoneNumberParams{})
	if err != nil {
		return res, fmt.Errorf("list Twilio phone numbers: %w", err)
	}

	existing, err := dst.List(ctx, &voiceml.ListIncomingPhoneNumbersParams{})
	if err != nil {
		return res, fmt.Errorf("list VoiceML phone numbers: %w", err)
	}

	have := make(map[string]bool, len(existing.IncomingPhoneNumbers))
	for _, n := range existing.IncomingPhoneNumbers {
		have[n.PhoneNumber] = true
	}

	for _, n := range nums {
		pn := deref(n.PhoneNumber)
		item := ItemResult{ID: pn}

		switch {
		case pn == "":
			item.Status = StatusFailed
			item.Detail = "source phone number has no phone_number value"
		case have[pn]:
			item.Status = StatusSkipped
			item.Detail = "already present on VoiceML"
		case opts.DryRun:
			item.Status = StatusPlanned
		default:
			if _, cErr := dst.Create(ctx, voiceml.CreateIncomingPhoneNumberParams{
				PhoneNumber:         pn,
				VoiceURL:            n.VoiceUrl,
				VoiceMethod:         n.VoiceMethod,
				VoiceFallbackURL:    n.VoiceFallbackUrl,
				VoiceFallbackMethod: n.VoiceFallbackMethod,
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

// deref returns the pointed-to string, or "" for a nil pointer.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

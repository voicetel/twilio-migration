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

// PhoneNumbers migrates IncomingPhoneNumbers (DIDs): voice and SMS request URLs
// with their fallbacks, the status callback, and any binding to a TwiML
// application. Idempotent: a number already present on VoiceML is skipped.
//
// A number bound to an application routes through that application, so its SID is
// re-pointed at the VoiceML application with the same friendly name, which needs
// Applications to have run first. A binding with no match fails that number rather
// than creating it: a re-run skips a number that exists, so one created unbound
// would stay without its routing for good. The friendly name is not copied, since
// VoiceML derives it from the number.
type PhoneNumbers struct{}

// Name implements Migrator.
func (PhoneNumbers) Name() string { return "phone-numbers" }

// Migrate implements Migrator.
func (PhoneNumbers) Migrate(ctx context.Context, c *Clients, opts Options) (Result, error) {
	return migratePhoneNumbers(ctx, c.Twilio, c.VoiceML.IncomingPhoneNumbers, c.Twilio, c.VoiceML.Applications, opts)
}

func migratePhoneNumbers(ctx context.Context, src phoneSource, dst phoneDest, appSrc appSource, appDst appDest, opts Options) (Result, error) {
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

	var appSIDs map[string]string
	if !opts.DryRun && bindsApplication(nums, have) {
		if appSIDs, err = applicationSIDMap(ctx, appSrc, appDst); err != nil {
			return res, err
		}
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
			params, cErr := phoneNumberParams(n, pn, appSIDs)
			if cErr == nil {
				_, cErr = dst.Create(ctx, params)
			}

			if cErr != nil {
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

// bindsApplication reports whether a number still to be created is bound to a
// TwiML application, the only case that needs the application lookup.
func bindsApplication(nums []twapi.ApiV2010IncomingPhoneNumber, have map[string]bool) bool {
	for _, n := range nums {
		if have[deref(n.PhoneNumber)] {
			continue
		}

		if deref(n.VoiceApplicationSid) != "" || deref(n.SmsApplicationSid) != "" {
			return true
		}
	}

	return false
}

// phoneNumberParams maps a Twilio number onto VoiceML's create body, re-pointing
// its application bindings. Twilio reports an unset value as an empty string,
// which is left out rather than sent.
func phoneNumberParams(n twapi.ApiV2010IncomingPhoneNumber, pn string, appSIDs map[string]string) (voiceml.CreateIncomingPhoneNumberParams, error) {
	voiceApp, err := remapApplication("voice", n.VoiceApplicationSid, appSIDs)
	if err != nil {
		return voiceml.CreateIncomingPhoneNumberParams{}, err
	}

	smsApp, err := remapApplication("SMS", n.SmsApplicationSid, appSIDs)
	if err != nil {
		return voiceml.CreateIncomingPhoneNumberParams{}, err
	}

	return voiceml.CreateIncomingPhoneNumberParams{
		PhoneNumber:          pn,
		VoiceURL:             n.VoiceUrl,
		VoiceMethod:          n.VoiceMethod,
		VoiceFallbackURL:     n.VoiceFallbackUrl,
		VoiceFallbackMethod:  n.VoiceFallbackMethod,
		VoiceApplicationSid:  voiceApp,
		SmsURL:               nonEmpty(n.SmsUrl),
		SmsMethod:            nonEmpty(n.SmsMethod),
		SmsFallbackURL:       nonEmpty(n.SmsFallbackUrl),
		SmsFallbackMethod:    nonEmpty(n.SmsFallbackMethod),
		SmsApplicationSid:    smsApp,
		StatusCallback:       nonEmpty(n.StatusCallback),
		StatusCallbackMethod: nonEmpty(n.StatusCallbackMethod),
	}, nil
}

// remapApplication resolves a number's application binding to its VoiceML SID.
// No binding yields nil; a binding with no VoiceML match is an error.
func remapApplication(kind string, sid *string, appSIDs map[string]string) (*string, error) {
	if deref(sid) == "" {
		return nil, nil
	}

	mapped, ok := appSIDs[*sid]
	if !ok {
		return nil, fmt.Errorf("%s application %s has no match on VoiceML: migrate applications first", kind, *sid)
	}

	return &mapped, nil
}

// nonEmpty returns s, or nil when it is nil or points at an empty string.
func nonEmpty(s *string) *string {
	if deref(s) == "" {
		return nil
	}

	return s
}

// deref returns the pointed-to string, or "" for a nil pointer.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

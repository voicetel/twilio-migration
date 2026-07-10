package migrate

import (
	twilio "github.com/twilio/twilio-go"
	twapi "github.com/twilio/twilio-go/rest/api/v2010"
	voiceml "github.com/voicetel/voiceml-go-sdk"

	"github.com/voicetel/twilio-migration/internal/config"
)

// Clients bundles the source (Twilio) and destination (VoiceML) API clients.
// Reads go through Twilio; writes go through VoiceML.
type Clients struct {
	// Twilio is the source, via the official twilio-go SDK.
	Twilio *twapi.ApiService
	// VoiceML is the destination, via the official voiceml-go-sdk.
	VoiceML *voiceml.Client
}

// NewClients builds the source and destination clients from cfg.
func NewClients(cfg config.Config) (*Clients, error) {
	src := twilio.NewRestClientWithParams(twilio.ClientParams{
		Username: cfg.TwilioAccountSid,
		Password: cfg.TwilioAuthToken,
	})

	dst, err := voiceml.NewClient(voiceml.ClientOptions{
		AccountSid: cfg.VoiceMLAccountSid,
		AuthToken:  cfg.VoiceMLAuthToken,
		BaseURL:    cfg.VoiceMLBaseURL,
	})
	if err != nil {
		return nil, err
	}

	return &Clients{Twilio: src.Api, VoiceML: dst}, nil
}

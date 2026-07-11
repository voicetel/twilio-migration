package migrate

import (
	"net/http"

	twilio "github.com/twilio/twilio-go"
	twclient "github.com/twilio/twilio-go/client"
	twapi "github.com/twilio/twilio-go/rest/api/v2010"
	twasst "github.com/twilio/twilio-go/rest/assistants/v1"
	twconv "github.com/twilio/twilio-go/rest/conversations/v1"
	twmsg "github.com/twilio/twilio-go/rest/messaging/v1"
	twvoice "github.com/twilio/twilio-go/rest/voice/v1"
	voiceml "github.com/voicetel/voiceml-go-sdk"

	"github.com/voicetel/twilio-migration/internal/config"
)

// Clients bundles the source (Twilio) and destination (VoiceML) API clients.
// Reads go through Twilio; writes go through VoiceML.
type Clients struct {
	// Twilio is the source /2010-04-01 API, via the official twilio-go SDK.
	Twilio *twapi.ApiService
	// TwilioMessaging is the source messaging.twilio.com/v1 API (Messaging
	// Services live here, not under /2010-04-01).
	TwilioMessaging *twmsg.ApiService
	// TwilioVoice is the source voice.twilio.com/v1 API (ByocTrunks,
	// ConnectionPolicies (+Targets), DialingPermissions, SourceIpMappings,
	// IpRecords live here, not under /2010-04-01).
	TwilioVoice *twvoice.ApiService
	// TwilioConversations is the source conversations.twilio.com/v1 API
	// (Services, Roles, Users, Conversations, Config, ...).
	TwilioConversations *twconv.ApiService
	// TwilioAssistants is the source assistants.twilio.com/v1 API
	// (Assistants, Tools, Knowledge, Policies, ...).
	TwilioAssistants *twasst.ApiService
	// VoiceML is the destination, via the official voiceml-go-sdk.
	VoiceML *voiceml.Client
}

// testTwilioTransport, when non-nil, replaces the HTTP transport used for
// the Twilio (source) side of NewClients. It exists solely so this module's
// own tests can redirect outbound Twilio requests to a local double —
// twilio-go's ApiService.baseURL is unexported with no public override,
// unlike VoiceML's ClientOptions.BaseURL (already reachable for tests via
// config.Config.VoiceMLBaseURL / --voiceml-base-url). Not part of the public
// API surface: internal/ packages are only importable within this module.
var testTwilioTransport http.RoundTripper

// SetTestTwilioTransport overrides the Twilio-side HTTP transport used by
// NewClients and returns a restore func. Test-only.
func SetTestTwilioTransport(rt http.RoundTripper) (restore func()) {
	prev := testTwilioTransport
	testTwilioTransport = rt
	return func() { testTwilioTransport = prev }
}

// testVoiceMLClientFactory, when non-nil, replaces the voiceml.NewClient
// call used for the VoiceML (destination) side of NewClients. Test-only, for
// exercising NewClients' error-return branch: by the time NewClients runs
// from the CLI, cfg has already passed config.Config.Validate(), which rules
// out every condition the real voiceml.NewClient can fail on (missing
// AccountSid; absent/conflicting API key+token; negative MaxRetries — this
// package never sets the last two). A fault-injection seam is the only way
// to reach that branch without weakening Validate()'s guarantee.
var testVoiceMLClientFactory func(voiceml.ClientOptions) (*voiceml.Client, error)

// SetTestVoiceMLClientFactory overrides the VoiceML client constructor used
// by NewClients and returns a restore func. Test-only.
func SetTestVoiceMLClientFactory(f func(voiceml.ClientOptions) (*voiceml.Client, error)) (restore func()) {
	prev := testVoiceMLClientFactory
	testVoiceMLClientFactory = f
	return func() { testVoiceMLClientFactory = prev }
}

// NewClients builds the source and destination clients from cfg.
func NewClients(cfg config.Config) (*Clients, error) {
	params := twilio.ClientParams{
		Username: cfg.TwilioAccountSid,
		Password: cfg.TwilioAuthToken,
	}
	if testTwilioTransport != nil {
		params.Client = &twclient.Client{
			Credentials: twclient.NewCredentials(cfg.TwilioAccountSid, cfg.TwilioAuthToken),
			HTTPClient:  &http.Client{Transport: testTwilioTransport},
		}
	}
	src := twilio.NewRestClientWithParams(params)

	newVoiceMLClient := voiceml.NewClient
	if testVoiceMLClientFactory != nil {
		newVoiceMLClient = testVoiceMLClientFactory
	}
	dst, err := newVoiceMLClient(voiceml.ClientOptions{
		AccountSid: cfg.VoiceMLAccountSid,
		AuthToken:  cfg.VoiceMLAuthToken,
		BaseURL:    cfg.VoiceMLBaseURL,
	})
	if err != nil {
		return nil, err
	}

	return &Clients{
		Twilio:              src.Api,
		TwilioMessaging:     src.MessagingV1,
		TwilioVoice:         src.VoiceV1,
		TwilioConversations: src.ConversationsV1,
		TwilioAssistants:    src.AssistantsV1,
		VoiceML:             dst,
	}, nil
}

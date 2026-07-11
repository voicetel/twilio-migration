package migrate

import (
	"errors"
	"testing"

	voiceml "github.com/voicetel/voiceml-go-sdk"

	"github.com/voicetel/twilio-migration/internal/config"
)

func TestNewClients(t *testing.T) {
	cfg := config.Config{
		TwilioAccountSid:  "AC00000000000000000000000000abcd",
		TwilioAuthToken:   "twiliotoken0000000000000000000",
		VoiceMLAccountSid: "AC00000000000000000000000000efgh",
		VoiceMLAuthToken:  "voicemltoken000000000000000000",
	}

	c, err := NewClients(cfg)
	if err != nil {
		t.Fatalf("NewClients: %v", err)
	}
	if c.Twilio == nil || c.TwilioMessaging == nil || c.VoiceML == nil {
		t.Errorf("expected all client fields populated, got %+v", c)
	}
}

// TestNewClients_VoiceMLError exercises NewClients' error-return branch: a
// missing VoiceML AccountSid makes voiceml.NewClient return a
// *ConfigurationError, which NewClients must propagate.
func TestNewClients_VoiceMLError(t *testing.T) {
	cfg := config.Config{
		TwilioAccountSid: "AC00000000000000000000000000abcd",
		TwilioAuthToken:  "twiliotoken0000000000000000000",
		VoiceMLAuthToken: "voicemltoken000000000000000000",
		// VoiceMLAccountSid intentionally left empty.
	}

	if _, err := NewClients(cfg); err == nil {
		t.Fatal("expected an error when VoiceML AccountSid is missing")
	}
}

// TestSetTestTwilioTransport exercises the test-only override seam itself:
// setting it must flow into the *twclient.Client built for the Twilio side,
// and the returned restore func must put the previous value back.
func TestSetTestTwilioTransport(t *testing.T) {
	restore := SetTestTwilioTransport(emptyJSONTransport{})
	defer restore()

	cfg := config.Config{
		TwilioAccountSid:  "AC00000000000000000000000000abcd",
		TwilioAuthToken:   "twiliotoken0000000000000000000",
		VoiceMLAccountSid: "AC00000000000000000000000000efgh",
		VoiceMLAuthToken:  "voicemltoken000000000000000000",
	}
	c, err := NewClients(cfg)
	if err != nil {
		t.Fatalf("NewClients: %v", err)
	}
	if c.Twilio == nil {
		t.Fatal("expected Twilio client to be populated")
	}

	restore()
	if testTwilioTransport != nil {
		t.Error("restore() did not clear testTwilioTransport")
	}
}

// TestSetTestVoiceMLClientFactory exercises the fault-injection seam itself,
// including that its restore func puts the previous value back.
func TestSetTestVoiceMLClientFactory(t *testing.T) {
	injected := errors.New("boom")
	restore := SetTestVoiceMLClientFactory(func(voiceml.ClientOptions) (*voiceml.Client, error) {
		return nil, injected
	})
	defer restore()

	cfg := config.Config{
		TwilioAccountSid:  "AC00000000000000000000000000abcd",
		TwilioAuthToken:   "twiliotoken0000000000000000000",
		VoiceMLAccountSid: "AC00000000000000000000000000efgh",
		VoiceMLAuthToken:  "voicemltoken000000000000000000",
	}
	if _, err := NewClients(cfg); !errors.Is(err, injected) {
		t.Fatalf("NewClients() = %v, want %v", err, injected)
	}

	restore()
	if testVoiceMLClientFactory != nil {
		t.Error("restore() did not clear testVoiceMLClientFactory")
	}
}

package migrate

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	twilio "github.com/twilio/twilio-go"
	twclient "github.com/twilio/twilio-go/client"
	voiceml "github.com/voicetel/voiceml-go-sdk"
)

// emptyJSONTransport answers every request with 200 {}. That is enough for
// every twilio-go List/Page call used by the Migrate() wrappers when the
// source account has no resources: pagination-envelope fields are all
// optional (pointer/omitempty), so an empty object decodes to a nil result
// slice and a nil next-page cursor. No real network I/O ever occurs —
// http.RoundTripper is called in-process before any socket is opened.
type emptyJSONTransport struct{}

func (emptyJSONTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader("{}")),
		Request:    req,
	}, nil
}

// newWiringTestClients builds a *Clients backed entirely by local doubles:
// the Twilio (source) side never leaves the process via emptyJSONTransport,
// and the VoiceML (destination) side is a local httptest.Server (BaseURL is
// natively overridable for tests, per voiceml-go-sdk's ClientOptions docs).
func newWiringTestClients(t *testing.T) (*Clients, func()) {
	t.Helper()

	vmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))

	rc := twilio.NewRestClientWithParams(twilio.ClientParams{
		Client: &twclient.Client{
			Credentials: twclient.NewCredentials("AC00000000000000000000000000abcd", "twiliotoken0000000000000000000"),
			HTTPClient:  &http.Client{Transport: emptyJSONTransport{}},
		},
	})

	vc, err := voiceml.NewClient(voiceml.ClientOptions{
		AccountSid: "AC00000000000000000000000000abcd",
		AuthToken:  "voicemltoken000000000000000000",
		BaseURL:    vmServer.URL,
	})
	if err != nil {
		t.Fatalf("voiceml.NewClient: %v", err)
	}

	return &Clients{Twilio: rc.Api, TwilioMessaging: rc.MessagingV1, TwilioVoice: rc.VoiceV1, VoiceML: vc}, vmServer.Close
}

// TestMigratorsMigrate_EmptySource exercises every Migrator.Migrate() wrapper
// (the thin SDK-wiring adapters) against an account with no resources on
// either side, closing the coverage gap those one-line methods otherwise
// leave, without any real network dependency.
func TestMigratorsMigrate_EmptySource(t *testing.T) {
	clients, cleanup := newWiringTestClients(t)
	defer cleanup()

	for _, m := range Default() {
		t.Run(m.Name(), func(t *testing.T) {
			res, err := m.Migrate(context.Background(), clients, Options{})
			if err != nil {
				t.Fatalf("%s.Migrate: %v", m.Name(), err)
			}
			if len(res.Items) != 0 {
				t.Errorf("%s: expected no items against an empty source, got %+v", m.Name(), res.Items)
			}
		})
	}
}

package version

import (
	"testing"

	voiceml "github.com/voicetel/voiceml-go-sdk"
)

// TestVersionMatchesSDK keeps twilio-migration pinned to the VoiceML OpenAPI
// version it targets: this fails the moment the voiceml-go-sdk dependency is
// bumped without re-tagging the tool, which is exactly when the migration
// mappings need review.
func TestVersionMatchesSDK(t *testing.T) {
	if Version != voiceml.Version {
		t.Fatalf("twilio-migration version %q != voiceml-go-sdk version %q — "+
			"bump internal/version.Version and re-tag to match the SDK", Version, voiceml.Version)
	}
}

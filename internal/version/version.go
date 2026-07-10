// Package version records the twilio-migration release.
//
// The version is kept in lockstep with the VoiceML OpenAPI version — the same
// version the voiceml-go-sdk is generated from and tagged with. Migration
// mappings target a specific VoiceML API surface, so tying the tool's version
// to that surface makes "which API does this build target?" unambiguous and
// keeps upgrades mechanical: bump the voiceml-go-sdk dependency and this
// constant together, then git-tag v<Version>.
//
// version_test.go enforces that this constant equals the linked
// voiceml-go-sdk's Version, so a dependency bump that forgets to re-tag the
// tool fails the build.
package version

// Version is the twilio-migration release, equal to the VoiceML OpenAPI /
// voiceml-go-sdk version it targets.
const Version = "0.9.2"

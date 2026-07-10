// Package config loads the Twilio (source) and VoiceML (destination)
// credentials and run options for the migration tool. Values come from
// environment variables when present, and are otherwise prompted for
// interactively via an injected Prompter (which keeps this package testable
// without a real terminal).
package config

import (
	"fmt"
	"strings"
)

// Environment variables checked before prompting.
const (
	EnvTwilioAccountSid  = "TWILIO_ACCOUNT_SID"
	EnvTwilioAuthToken   = "TWILIO_AUTH_TOKEN"
	EnvVoiceMLAccountSid = "VOICEML_ACCOUNT_SID"
	EnvVoiceMLAuthToken  = "VOICEML_AUTH_TOKEN"
	EnvVoiceMLBaseURL    = "VOICEML_BASE_URL"
)

// Config holds everything a migration run needs.
type Config struct {
	TwilioAccountSid string
	TwilioAuthToken  string

	VoiceMLAccountSid string
	VoiceMLAuthToken  string
	// VoiceMLBaseURL overrides the VoiceML API host. Empty means the SDK
	// default (https://voiceml.voicetel.com).
	VoiceMLBaseURL string

	// DryRun reports what would be migrated without writing to VoiceML.
	DryRun bool
	// Only, when non-empty, restricts the run to these migrator names.
	Only []string
}

// Prompter reads interactive input. Line echoes the typed value; Secret does
// not (used for auth tokens).
type Prompter interface {
	Line(label string) (string, error)
	Secret(label string) (string, error)
}

// Getenv matches os.Getenv; injected so Load is testable.
type Getenv func(string) string

// Load resolves credentials from the environment first, then prompts for any
// that are still missing. base carries the non-credential options (DryRun,
// Only, VoiceMLBaseURL) already parsed from flags; credentials on base are
// ignored in favour of env/prompt.
func Load(base Config, env Getenv, p Prompter) (Config, error) {
	cfg := base

	if cfg.VoiceMLBaseURL == "" {
		cfg.VoiceMLBaseURL = strings.TrimSpace(env(EnvVoiceMLBaseURL))
	}

	var err error
	if cfg.TwilioAccountSid, err = resolve(env(EnvTwilioAccountSid), "Twilio Account SID", false, p); err != nil {
		return cfg, err
	}
	if cfg.TwilioAuthToken, err = resolve(env(EnvTwilioAuthToken), "Twilio Auth Token", true, p); err != nil {
		return cfg, err
	}
	if cfg.VoiceMLAccountSid, err = resolve(env(EnvVoiceMLAccountSid), "VoiceML Account SID", false, p); err != nil {
		return cfg, err
	}
	if cfg.VoiceMLAuthToken, err = resolve(env(EnvVoiceMLAuthToken), "VoiceML Auth Token", true, p); err != nil {
		return cfg, err
	}

	return cfg, cfg.Validate()
}

// resolve returns the trimmed env value when set, otherwise prompts for it.
func resolve(envVal, label string, secret bool, p Prompter) (string, error) {
	if v := strings.TrimSpace(envVal); v != "" {
		return v, nil
	}

	if p == nil {
		return "", fmt.Errorf("%s not set and no prompter available", label)
	}

	var (
		v   string
		err error
	)
	if secret {
		v, err = p.Secret(label)
	} else {
		v, err = p.Line(label)
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", label, err)
	}

	return strings.TrimSpace(v), nil
}

// Validate checks that all four credentials are present.
func (c Config) Validate() error {
	missing := make([]string, 0, 4)
	if c.TwilioAccountSid == "" {
		missing = append(missing, "Twilio Account SID")
	}
	if c.TwilioAuthToken == "" {
		missing = append(missing, "Twilio Auth Token")
	}
	if c.VoiceMLAccountSid == "" {
		missing = append(missing, "VoiceML Account SID")
	}
	if c.VoiceMLAuthToken == "" {
		missing = append(missing, "VoiceML Auth Token")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required credentials: %s", strings.Join(missing, ", "))
	}

	return nil
}

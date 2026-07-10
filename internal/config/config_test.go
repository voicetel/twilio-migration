package config

import (
	"errors"
	"strings"
	"testing"
)

// mapEnv returns a Getenv backed by a map.
func mapEnv(m map[string]string) Getenv {
	return func(k string) string { return m[k] }
}

// scriptPrompter returns queued answers; Secret and Line draw from the same
// queue in call order. A nil err with an empty queue returns io.EOF-like error.
type scriptPrompter struct {
	lines   []string
	secrets []string
	err     error
}

func (p *scriptPrompter) Line(string) (string, error) {
	if p.err != nil {
		return "", p.err
	}
	if len(p.lines) == 0 {
		return "", errors.New("no more line answers")
	}
	v := p.lines[0]
	p.lines = p.lines[1:]
	return v, nil
}

func (p *scriptPrompter) Secret(string) (string, error) {
	if p.err != nil {
		return "", p.err
	}
	if len(p.secrets) == 0 {
		return "", errors.New("no more secret answers")
	}
	v := p.secrets[0]
	p.secrets = p.secrets[1:]
	return v, nil
}

func TestLoadFromEnv(t *testing.T) {
	env := mapEnv(map[string]string{
		EnvTwilioAccountSid:  "ACtwilio",
		EnvTwilioAuthToken:   "twtoken",
		EnvVoiceMLAccountSid: "ACvoiceml",
		EnvVoiceMLAuthToken:  "vmtoken",
		EnvVoiceMLBaseURL:    "https://example.test",
	})

	cfg, err := Load(Config{DryRun: true}, env, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TwilioAccountSid != "ACtwilio" || cfg.TwilioAuthToken != "twtoken" {
		t.Errorf("twilio creds wrong: %+v", cfg)
	}
	if cfg.VoiceMLAccountSid != "ACvoiceml" || cfg.VoiceMLAuthToken != "vmtoken" {
		t.Errorf("voiceml creds wrong: %+v", cfg)
	}
	if cfg.VoiceMLBaseURL != "https://example.test" {
		t.Errorf("base url = %q", cfg.VoiceMLBaseURL)
	}
	if !cfg.DryRun {
		t.Error("DryRun should carry through from base")
	}
}

func TestLoadPromptsForMissing(t *testing.T) {
	env := mapEnv(map[string]string{
		EnvTwilioAccountSid: "ACtwilio", // only this from env
	})
	p := &scriptPrompter{
		lines:   []string{"ACvoiceml"}, // VoiceML SID (Twilio SID came from env)
		secrets: []string{"twtoken", "vmtoken"},
	}

	cfg, err := Load(Config{}, env, p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TwilioAuthToken != "twtoken" || cfg.VoiceMLAccountSid != "ACvoiceml" || cfg.VoiceMLAuthToken != "vmtoken" {
		t.Errorf("prompted creds wrong: %+v", cfg)
	}
}

func TestLoadTrimsWhitespace(t *testing.T) {
	env := mapEnv(map[string]string{
		EnvTwilioAccountSid:  "  ACtwilio  ",
		EnvTwilioAuthToken:   "twtoken",
		EnvVoiceMLAccountSid: "ACvoiceml",
		EnvVoiceMLAuthToken:  "vmtoken",
	})
	cfg, err := Load(Config{}, env, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TwilioAccountSid != "ACtwilio" {
		t.Errorf("whitespace not trimmed: %q", cfg.TwilioAccountSid)
	}
}

func TestLoadMissingNoPrompter(t *testing.T) {
	_, err := Load(Config{}, mapEnv(nil), nil)
	if err == nil {
		t.Fatal("expected error when creds missing and no prompter")
	}
	if !strings.Contains(err.Error(), "no prompter") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadPrompterError(t *testing.T) {
	p := &scriptPrompter{err: errors.New("boom")}
	_, err := Load(Config{}, mapEnv(nil), p)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected prompter error to propagate, got %v", err)
	}
}

func TestLoadKeepsPresetBaseURL(t *testing.T) {
	env := mapEnv(map[string]string{
		EnvTwilioAccountSid: "a", EnvTwilioAuthToken: "b",
		EnvVoiceMLAccountSid: "c", EnvVoiceMLAuthToken: "d",
		EnvVoiceMLBaseURL: "https://from-env.test", // must be ignored when base already set
	})
	cfg, err := Load(Config{VoiceMLBaseURL: "https://from-flag.test"}, env, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.VoiceMLBaseURL != "https://from-flag.test" {
		t.Errorf("preset base url overwritten: %q", cfg.VoiceMLBaseURL)
	}
}

// countingErrPrompter succeeds for the first ok answers then errors, so the
// error-forwarding branch of a later credential in Load is exercised.
type countingErrPrompter struct {
	ok int
	n  int
}

func (p *countingErrPrompter) Line(string) (string, error)   { return p.next() }
func (p *countingErrPrompter) Secret(string) (string, error) { return p.next() }
func (p *countingErrPrompter) next() (string, error) {
	p.n++
	if p.n > p.ok {
		return "", errors.New("prompt failed")
	}
	return "val", nil
}

func TestLoadLaterCredentialError(t *testing.T) {
	// Only the Twilio SID comes from env; the next prompted credential errors.
	env := mapEnv(map[string]string{EnvTwilioAccountSid: "ACtwilio"})
	_, err := Load(Config{}, env, &countingErrPrompter{ok: 0})
	if err == nil || !strings.Contains(err.Error(), "prompt failed") {
		t.Fatalf("expected later-credential error, got %v", err)
	}
}

func TestValidate(t *testing.T) {
	full := Config{
		TwilioAccountSid: "a", TwilioAuthToken: "b",
		VoiceMLAccountSid: "c", VoiceMLAuthToken: "d",
	}
	if err := full.Validate(); err != nil {
		t.Errorf("full config should validate: %v", err)
	}

	if err := (Config{}).Validate(); err == nil {
		t.Error("empty config should not validate")
	} else if !strings.Contains(err.Error(), "Twilio Account SID") {
		t.Errorf("error should list missing fields: %v", err)
	}
}

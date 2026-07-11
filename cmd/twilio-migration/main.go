// Command twilio-migration copies account configuration from a Twilio account
// to a VoiceML account: phone numbers, TwiML applications, and (planned) SIP
// trunking and messaging config. It reads from Twilio with the official
// twilio-go SDK and writes to VoiceML with the official voiceml-go-sdk.
//
// Credentials are read from the environment (TWILIO_ACCOUNT_SID,
// TWILIO_AUTH_TOKEN, VOICEML_ACCOUNT_SID, VOICEML_AUTH_TOKEN) or prompted for
// interactively. Run with --dry-run first to preview.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	voiceml "github.com/voicetel/voiceml-go-sdk"

	"github.com/voicetel/twilio-migration/internal/config"
	"github.com/voicetel/twilio-migration/internal/migrate"
	"github.com/voicetel/twilio-migration/internal/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		dryRun       = flag.Bool("dry-run", false, "preview what would be migrated without writing to VoiceML")
		baseURL      = flag.String("voiceml-base-url", "", "override the VoiceML API base URL (default https://voiceml.voicetel.com)")
		only         = flag.String("only", "", "comma-separated migrator names to run (default: all). Available: "+strings.Join(migratorNames(), ", "))
		assumeYes    = flag.Bool("yes", false, "skip the confirmation prompt before writing")
		showVersion  = flag.Bool("version", false, "print the version (matches the VoiceML OpenAPI/SDK version) and exit")
		showCoverage = flag.Bool("coverage", false, "print the resource coverage matrix (what is migrated / unmigratable / planned) and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("twilio-migration %s (targets VoiceML OpenAPI %s; linked voiceml-go-sdk %s)\n",
			version.Version, version.Version, voiceml.Version)
		return nil
	}

	if *showCoverage {
		printCoverage(os.Stdout)
		return nil
	}

	base := config.Config{
		DryRun:         *dryRun,
		VoiceMLBaseURL: *baseURL,
		Only:           splitCSV(*only),
	}

	cfg, err := config.Load(base, os.Getenv, newTerminalPrompter())
	if err != nil {
		return err
	}

	all := migrate.Default()
	selected, unknown := migrate.Select(all, cfg.Only)
	for _, name := range unknown {
		fmt.Fprintf(os.Stderr, "warning: unknown migrator %q (ignored)\n", name)
	}
	if len(selected) == 0 {
		return fmt.Errorf("no migrators selected")
	}

	fmt.Printf("Source (Twilio):      %s\n", cfg.TwilioAccountSid)
	fmt.Printf("Destination (VoiceML): %s @ %s\n", cfg.VoiceMLAccountSid, baseURLOrDefault(cfg.VoiceMLBaseURL))
	fmt.Printf("Resources:            %s\n", strings.Join(names(selected), ", "))
	if cfg.DryRun {
		fmt.Println("Mode:                 DRY RUN (no writes)")
	} else {
		fmt.Println("Mode:                 LIVE (will create resources on VoiceML)")
		if !*assumeYes && !confirm("Proceed?") {
			return fmt.Errorf("aborted")
		}
	}

	clients, err := migrate.NewClients(cfg)
	if err != nil {
		return err
	}

	results := migrate.Run(context.Background(), clients, selected, migrate.Options{DryRun: cfg.DryRun})
	failures := report(os.Stdout, results)
	if failures > 0 {
		return fmt.Errorf("%d item(s) failed", failures)
	}

	return nil
}

// report prints a per-resource summary and returns the total failure count.
func report(w *os.File, results []migrate.Result) int {
	total := 0
	_, _ = fmt.Fprintln(w, "\nResults:")
	for _, r := range results {
		_, _ = fmt.Fprintf(w, "  %-16s created=%d skipped=%d planned=%d failed=%d\n",
			r.Resource,
			r.Count(migrate.StatusCreated),
			r.Count(migrate.StatusSkipped),
			r.Count(migrate.StatusPlanned),
			r.Count(migrate.StatusFailed),
		)
		for _, it := range r.Items {
			switch {
			case it.Status == migrate.StatusFailed:
				_, _ = fmt.Fprintf(w, "      FAILED %s: %s\n", it.ID, it.Detail)
			case it.Status == migrate.StatusCreated && it.Detail != "":
				// Surfaces generated SIP credential passwords for redistribution.
				_, _ = fmt.Fprintf(w, "      %s: %s\n", it.ID, it.Detail)
			}
		}
		total += r.Count(migrate.StatusFailed)
	}
	return total
}

// printCoverage prints the resource coverage matrix grouped by status.
func printCoverage(w *os.File) {
	_, _ = fmt.Fprintln(w, "Resource coverage (Twilio → VoiceML):")
	for _, status := range []migrate.Coverage{migrate.CovMigrated, migrate.CovRoadmap, migrate.CovUnmigratable} {
		_, _ = fmt.Fprintf(w, "\n[%s]\n", status)
		for _, e := range migrate.Inventory() {
			if e.Status != status {
				continue
			}
			if e.Reason != "" {
				_, _ = fmt.Fprintf(w, "  %-22s %s\n", e.Resource, e.Reason)
			} else {
				_, _ = fmt.Fprintf(w, "  %s\n", e.Resource)
			}
		}
	}
}

func migratorNames() []string { return names(migrate.Default()) }

func names(ms []migrate.Migrator) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.Name())
	}
	return out
}

func baseURLOrDefault(u string) string {
	if strings.TrimSpace(u) == "" {
		return "https://voiceml.voicetel.com"
	}
	return u
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func confirm(prompt string) bool {
	fmt.Printf("%s [y/N] ", prompt)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// terminalPrompter reads interactive input from the real terminal. Secrets are
// read without echo when stdin is a TTY.
type terminalPrompter struct {
	r *bufio.Reader
}

func newTerminalPrompter() *terminalPrompter {
	return &terminalPrompter{r: bufio.NewReader(os.Stdin)}
}

func (p *terminalPrompter) Line(label string) (string, error) {
	fmt.Printf("%s: ", label)
	s, err := p.r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(s, "\r\n"), nil
}

func (p *terminalPrompter) Secret(label string) (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		// Not a TTY (piped input): fall back to a plain line read.
		return p.Line(label)
	}
	fmt.Printf("%s: ", label)
	b, err := term.ReadPassword(fd)
	fmt.Println()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

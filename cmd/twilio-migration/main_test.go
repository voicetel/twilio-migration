package main

import (
	"errors"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	voiceml "github.com/voicetel/voiceml-go-sdk"

	"github.com/voicetel/twilio-migration/internal/config"
	"github.com/voicetel/twilio-migration/internal/migrate"
)

// testBinPath is the real test binary path, captured before any test can
// mutate the package-level os.Args (resetFlags does, for run() tests).
var testBinPath = os.Args[0]

// --- pure helpers ---

func TestMigratorNames(t *testing.T) {
	got := migratorNames()
	want := names(migrate.Default())
	if len(got) != len(want) {
		t.Fatalf("migratorNames() = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("migratorNames()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestNames(t *testing.T) {
	if got := names(nil); len(got) != 0 {
		t.Errorf("names(nil) = %v, want empty", got)
	}
	got := names(migrate.Default())
	if len(got) != len(migrate.Default()) {
		t.Errorf("names() length = %d, want %d", len(got), len(migrate.Default()))
	}
}

func TestBaseURLOrDefault(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "https://voiceml.voicetel.com"},
		{"   ", "https://voiceml.voicetel.com"},
		{"https://example.test", "https://example.test"},
	}
	for _, c := range cases {
		if got := baseURLOrDefault(c.in); got != c.want {
			t.Errorf("baseURLOrDefault(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSplitCSV(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a,b", []string{"a", "b"}},
		{" a , , b ", []string{"a", "b"}},
	}
	for _, c := range cases {
		got := splitCSV(c.in)
		if len(got) != len(c.want) {
			t.Fatalf("splitCSV(%q) = %v, want %v", c.in, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("splitCSV(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

// --- report / printCoverage (take *os.File) ---

func TestReport(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "report")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = f.Close() }()

	results := []migrate.Result{
		{Resource: "applications", Items: []migrate.ItemResult{
			{ID: "a", Status: migrate.StatusCreated},
			{ID: "b", Status: migrate.StatusFailed, Detail: "boom"},
			{ID: "c", Status: migrate.StatusCreated, Detail: "generated secret"},
		}},
	}

	failures := report(f, results)
	if failures != 1 {
		t.Errorf("failures = %d, want 1", failures)
	}

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	out := string(data)
	for _, want := range []string{"Results:", "applications", "FAILED b: boom", "c: generated secret"} {
		if !strings.Contains(out, want) {
			t.Errorf("report output missing %q, got:\n%s", want, out)
		}
	}
}

func TestPrintCoverage(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "coverage")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = f.Close() }()

	printCoverage(f)

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !strings.Contains(string(data), "Resource coverage") {
		t.Errorf("printCoverage output missing header, got:\n%s", data)
	}
}

// --- stdin-dependent helpers (confirm, terminalPrompter) ---

// withStdin redirects os.Stdin to a pipe fed with input for the duration of
// fn, then restores it.
func withStdin(t *testing.T, input string, fn func()) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdin
	os.Stdin = r
	defer func() {
		os.Stdin = orig
		_ = r.Close()
	}()

	done := make(chan struct{})
	go func() {
		_, _ = w.WriteString(input)
		_ = w.Close()
		close(done)
	}()

	fn()
	<-done
}

func TestConfirm(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"y\n", true},
		{"yes\n", true},
		{"Y\n", true},
		{"n\n", false},
		{"no\n", false},
		{"\n", false},
	}
	for _, c := range cases {
		withStdin(t, c.in, func() {
			if got := confirm("Proceed?"); got != c.want {
				t.Errorf("confirm() with input %q = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestConfirm_ReadError(t *testing.T) {
	// Immediately-closed stdin: ReadString hits EOF before any delimiter.
	withStdin(t, "", func() {
		if got := confirm("Proceed?"); got != false {
			t.Errorf("confirm() on EOF = %v, want false", got)
		}
	})
}

func TestTerminalPrompter_Line(t *testing.T) {
	withStdin(t, "hello world\n", func() {
		p := newTerminalPrompter()
		v, err := p.Line("Label")
		if err != nil {
			t.Fatalf("Line: %v", err)
		}
		if v != "hello world" {
			t.Errorf("Line() = %q, want %q", v, "hello world")
		}
	})
}

func TestTerminalPrompter_LineError(t *testing.T) {
	withStdin(t, "", func() {
		p := newTerminalPrompter()
		if _, err := p.Line("Label"); err == nil {
			t.Fatal("expected an error reading from a closed stdin")
		}
	})
}

// TestTerminalPrompter_SecretNonTTYFallback covers Secret()'s non-TTY branch
// (falls back to a plain Line read).
func TestTerminalPrompter_SecretNonTTYFallback(t *testing.T) {
	withStdin(t, "shh\n", func() {
		p := newTerminalPrompter()
		v, err := p.Secret("Label")
		if err != nil {
			t.Fatalf("Secret: %v", err)
		}
		if v != "shh" {
			t.Errorf("Secret() = %q, want %q", v, "shh")
		}
	})
}

// TestTerminalPrompter_SecretRealTTY covers Secret()'s real-terminal branch
// (term.ReadPassword, no-echo). term.IsTerminal resolves via a real
// ioctl(TCGETS) syscall (golang.org/x/term term_unix.go) that a pipe cannot
// satisfy, so this uses a real pseudo-terminal pair via github.com/creack/pty.
func TestTerminalPrompter_SecretRealTTY(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open: %v", err)
	}
	defer func() { _ = ptmx.Close() }()
	defer func() { _ = tty.Close() }()

	orig := os.Stdin
	os.Stdin = tty
	defer func() { os.Stdin = orig }()

	go func() {
		_, _ = ptmx.Write([]byte("supersecret\n"))
	}()

	p := newTerminalPrompter()
	v, err := p.Secret("Password")
	if err != nil {
		t.Fatalf("Secret: %v", err)
	}
	if v != "supersecret" {
		t.Errorf("Secret() = %q, want %q", v, "supersecret")
	}
}

// TestTerminalPrompter_SecretReadError covers term.ReadPassword's own
// error-return branch inside Secret(). The master must still be open when
// Secret() checks term.IsTerminal (closing it first makes the slave stop
// reporting as a terminal, which takes the non-TTY fallback branch instead)
// — so the close is deferred to a goroutine, landing while ReadPassword's
// blocked read is in flight. Reading zero bytes before EOF is exactly the
// case readPasswordLine (golang.org/x/term/terminal.go) treats as a real
// error, not success (its "EOF is success" case only applies once some data
// has already been read).
func TestTerminalPrompter_SecretReadError(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open: %v", err)
	}
	defer func() { _ = tty.Close() }()

	orig := os.Stdin
	os.Stdin = tty
	defer func() { os.Stdin = orig }()

	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = ptmx.Close() // no data written before the master closes
	}()

	p := newTerminalPrompter()
	if _, err := p.Secret("Password"); err == nil {
		t.Fatal("expected an error when the pty master closes mid-read with no data")
	}
}

// --- run() ---

func resetFlags(args []string) {
	os.Args = args
	flag.CommandLine = flag.NewFlagSet(args[0], flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
}

func setCredsEnv(t *testing.T) {
	t.Helper()
	t.Setenv(config.EnvTwilioAccountSid, "AC00000000000000000000000000abcd")
	t.Setenv(config.EnvTwilioAuthToken, "twiliotoken0000000000000000000")
	t.Setenv(config.EnvVoiceMLAccountSid, "AC00000000000000000000000000efgh")
	t.Setenv(config.EnvVoiceMLAuthToken, "voicemltoken000000000000000000")
}

func clearCredsEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		config.EnvTwilioAccountSid, config.EnvTwilioAuthToken,
		config.EnvVoiceMLAccountSid, config.EnvVoiceMLAuthToken,
	} {
		orig, had := os.LookupEnv(k)
		if err := os.Unsetenv(k); err != nil {
			t.Fatalf("Unsetenv(%s): %v", k, err)
		}
		t.Cleanup(func() {
			if had {
				_ = os.Setenv(k, orig)
			}
		})
	}
}

// withStdout redirects os.Stdout to a discarded pipe for the duration of fn,
// so tests that legitimately print (e.g. --coverage, --version) don't clutter
// test output.
func withStdout(t *testing.T, fn func()) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, r)
		close(done)
	}()
	defer func() {
		os.Stdout = orig
		_ = w.Close()
		<-done
	}()
	fn()
}

func TestRun_Version(t *testing.T) {
	resetFlags([]string{"twilio-migration", "--version"})
	withStdout(t, func() {
		if err := run(); err != nil {
			t.Fatalf("run(): %v", err)
		}
	})
}

func TestRun_Coverage(t *testing.T) {
	resetFlags([]string{"twilio-migration", "--coverage"})
	withStdout(t, func() {
		if err := run(); err != nil {
			t.Fatalf("run(): %v", err)
		}
	})
}

func TestRun_LoadError(t *testing.T) {
	clearCredsEnv(t)
	resetFlags([]string{"twilio-migration"})
	withStdout(t, func() {
		withStdin(t, "", func() {
			if err := run(); err == nil {
				t.Fatal("expected an error when credentials are missing and stdin is closed")
			}
		})
	})
}

func TestRun_UnknownMigratorNoneSelected(t *testing.T) {
	setCredsEnv(t)
	resetFlags([]string{"twilio-migration", "--only=bogus"})
	withStdout(t, func() {
		err := run()
		if err == nil || !strings.Contains(err.Error(), "no migrators selected") {
			t.Fatalf("run() = %v, want \"no migrators selected\"", err)
		}
	})
}

func TestRun_AbortedByConfirm(t *testing.T) {
	setCredsEnv(t)
	resetFlags([]string{"twilio-migration", "--only=queues"})
	withStdout(t, func() {
		withStdin(t, "n\n", func() {
			err := run()
			if err == nil || !strings.Contains(err.Error(), "aborted") {
				t.Fatalf("run() = %v, want \"aborted\"", err)
			}
		})
	})
}

// emptyTwilioTransport answers every Twilio request with 200 {} — no real
// network I/O occurs; RoundTrip is called in-process. See
// internal/migrate/wiring_test.go for the pagination-envelope rationale
// (duplicated here since _test.go files are not importable across packages).
type emptyTwilioTransport struct{}

func (emptyTwilioTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK, Status: "200 OK", Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
		Header:  http.Header{"Content-Type": []string{"application/json"}},
		Body:    io.NopCloser(strings.NewReader("{}")),
		Request: req,
	}, nil
}

// oneUnnamedQueueTransport answers a Queues.json list with a single queue
// that has no friendly_name — migrateQueues records that deterministically
// as a Failed item with no Create call needed — and {} for everything else.
// Exercises run()'s failures>0 branch.
type oneUnnamedQueueTransport struct{}

func (oneUnnamedQueueTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body := "{}"
	if strings.Contains(req.URL.Path, "Queues.json") {
		body = `{"queues":[{"sid":"QU00000000000000000000000000abcd"}]}`
	}
	return &http.Response{
		StatusCode: http.StatusOK, Status: "200 OK", Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
		Header:  http.Header{"Content-Type": []string{"application/json"}},
		Body:    io.NopCloser(strings.NewReader(body)),
		Request: req,
	}, nil
}

// newEmptyVoiceMLServer starts a local httptest server answering every
// request with 200 {} and returns its URL for VOICEML_BASE_URL. VoiceML's
// BaseURL is natively test-overridable (unlike Twilio's), so no production
// seam is needed on that side.
func newEmptyVoiceMLServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// TestRun_LiveSuccess drives run() all the way to its final `return nil`,
// via migrate.SetTestTwilioTransport (Twilio side) and VOICEML_BASE_URL
// (VoiceML side) — no real network dependency.
func TestRun_LiveSuccess(t *testing.T) {
	setCredsEnv(t)
	t.Setenv(config.EnvVoiceMLBaseURL, newEmptyVoiceMLServer(t))
	restore := migrate.SetTestTwilioTransport(emptyTwilioTransport{})
	defer restore()

	resetFlags([]string{"twilio-migration", "--only=queues", "--yes"})
	withStdout(t, func() {
		if err := run(); err != nil {
			t.Fatalf("run(): %v", err)
		}
	})
}

// TestRun_DryRunSuccess covers the DryRun-true print branch and confirms
// dry-run still reaches the same success tail.
func TestRun_DryRunSuccess(t *testing.T) {
	setCredsEnv(t)
	t.Setenv(config.EnvVoiceMLBaseURL, newEmptyVoiceMLServer(t))
	restore := migrate.SetTestTwilioTransport(emptyTwilioTransport{})
	defer restore()

	resetFlags([]string{"twilio-migration", "--only=queues", "--dry-run"})
	withStdout(t, func() {
		if err := run(); err != nil {
			t.Fatalf("run(): %v", err)
		}
	})
}

// TestRun_ReportsFailures covers run()'s failures>0 branch.
func TestRun_ReportsFailures(t *testing.T) {
	setCredsEnv(t)
	t.Setenv(config.EnvVoiceMLBaseURL, newEmptyVoiceMLServer(t))
	restore := migrate.SetTestTwilioTransport(oneUnnamedQueueTransport{})
	defer restore()

	resetFlags([]string{"twilio-migration", "--only=queues", "--yes"})
	withStdout(t, func() {
		err := run()
		if err == nil || !strings.Contains(err.Error(), "item(s) failed") {
			t.Fatalf("run() = %v, want \"item(s) failed\"", err)
		}
	})
}

// TestRun_NewClientsError covers run()'s migrate.NewClients error-return
// branch via migrate.SetTestVoiceMLClientFactory — see that seam's doc
// comment for why fault injection is the only way to reach it (the real
// voiceml.NewClient cannot fail once cfg has passed config.Config.Validate(),
// which run() already guarantees by this point).
func TestRun_NewClientsError(t *testing.T) {
	setCredsEnv(t)
	restore := migrate.SetTestVoiceMLClientFactory(func(voiceml.ClientOptions) (*voiceml.Client, error) {
		return nil, errors.New("boom")
	})
	defer restore()

	resetFlags([]string{"twilio-migration", "--only=queues", "--yes"})
	withStdout(t, func() {
		err := run()
		if err == nil || !strings.Contains(err.Error(), "boom") {
			t.Fatalf("run() = %v, want \"boom\"", err)
		}
	})
}

// --- main() (os.Exit paths, via subprocess) ---

func TestMain_ExitsCleanly(t *testing.T) {
	if os.Getenv("TWILIO_MIGRATION_TEST_MAIN") == "clean" {
		os.Args = []string{"twilio-migration", "--version"}
		main()
		return
	}

	cmd := exec.Command(testBinPath, "-test.run=TestMain_ExitsCleanly")
	cmd.Env = append(os.Environ(), "TWILIO_MIGRATION_TEST_MAIN=clean")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("main() exited with error: %v, output: %s", err, out)
	}
}

func TestMain_ExitsWithError(t *testing.T) {
	if os.Getenv("TWILIO_MIGRATION_TEST_MAIN") == "error" {
		os.Args = []string{"twilio-migration", "--only=bogus"}
		main()
		return
	}

	cmd := exec.Command(testBinPath, "-test.run=TestMain_ExitsWithError")
	cmd.Env = append(os.Environ(),
		"TWILIO_MIGRATION_TEST_MAIN=error",
		"TWILIO_ACCOUNT_SID=AC00000000000000000000000000abcd",
		"TWILIO_AUTH_TOKEN=twiliotoken0000000000000000000",
		"VOICEML_ACCOUNT_SID=AC00000000000000000000000000efgh",
		"VOICEML_AUTH_TOKEN=voicemltoken000000000000000000",
	)
	out, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("expected exit code 1, got err=%v output=%s", err, out)
	}
}

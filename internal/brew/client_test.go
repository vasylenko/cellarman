package brew

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// fakeRunner routes brew subcommands to recorded fixtures, so client logic is
// tested without a real brew. doctorFile lets a test pick the warnings vs
// all-clear output; outErr forces the failure path.
type fakeRunner struct {
	t             *testing.T
	doctorFile    string
	doctorExitErr error // simulate brew doctor's non-zero exit when warnings exist
	outErr        error
	outStderr     string
	lastArgs      []string // captured for flag assertions
	streamLines   []string
	streamErr     error
}

func (f *fakeRunner) Output(_ context.Context, args ...string) ([]byte, []byte, error) {
	f.lastArgs = args
	if f.outErr != nil {
		return nil, []byte(f.outStderr), f.outErr
	}
	has := func(want string) bool {
		for _, a := range args {
			if a == want {
				return true
			}
		}
		return false
	}
	switch args[0] {
	case "info":
		switch {
		case has("--installed"):
			return readFixture(f.t, "installed.json"), nil, nil
		case has("--cask"):
			return readFixture(f.t, "info_cask.json"), nil, nil
		default:
			return readFixture(f.t, "info_formula.json"), nil, nil
		}
	case "tap-info":
		return readFixture(f.t, "taps.json"), nil, nil
	case "outdated":
		return readFixture(f.t, "outdated.json"), nil, nil
	case "search":
		return readFixture(f.t, "search_formula.txt"), nil, nil
	case "doctor":
		return readFixture(f.t, f.doctorFile), nil, f.doctorExitErr
	}
	f.t.Fatalf("fakeRunner: unexpected args %v", args)
	return nil, nil, nil
}

func (f *fakeRunner) Stream(ctx context.Context, args ...string) (<-chan Event, error) {
	f.lastArgs = args
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	ch := make(chan Event)
	go func() {
		defer close(ch)
		for _, line := range f.streamLines {
			select {
			case ch <- Event{Line: line}:
			case <-ctx.Done():
				return
			}
		}
		ch <- Event{Done: true}
	}()
	return ch, nil
}

func TestInstalled(t *testing.T) {
	c := NewWithRunner(&fakeRunner{t: t})
	formulae, casks, err := c.Installed(context.Background())
	if err != nil {
		t.Fatalf("Installed: %v", err)
	}
	if len(formulae) != 2 {
		t.Fatalf("formulae = %d, want 2", len(formulae))
	}
	if len(casks) != 1 {
		t.Fatalf("casks = %d, want 1", len(casks))
	}

	gnutls := formulae[1]
	if got := gnutls.InstalledVersion(); got != "3.8.13_1" {
		t.Errorf("InstalledVersion = %q, want 3.8.13_1", got)
	}
	if !gnutls.IsInstalled() {
		t.Error("gnutls should be installed")
	}
	if !gnutls.Outdated {
		t.Error("gnutls should be outdated")
	}
	if len(gnutls.Dependencies) != 6 {
		t.Errorf("gnutls deps = %d, want 6", len(gnutls.Dependencies))
	}

	cask := casks[0]
	if !cask.IsInstalled() {
		t.Error("qlmarkdown should be installed")
	}
	if got := cask.DisplayName(); got != "sbarex QLMarkdown" {
		t.Errorf("DisplayName = %q, want 'sbarex QLMarkdown'", got)
	}
}

func TestInfoFormula(t *testing.T) {
	c := NewWithRunner(&fakeRunner{t: t})
	f, cask, err := c.Info(context.Background(), "jq", KindFormula)
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if cask != nil {
		t.Fatal("expected nil cask for formula query")
	}
	if f.Name != "jq" || f.Versions.Stable != "1.8.1" {
		t.Errorf("got %q %q, want jq 1.8.1", f.Name, f.Versions.Stable)
	}
	if f.IsInstalled() {
		t.Error("jq fixture should not be installed")
	}
}

func TestInfoCask(t *testing.T) {
	c := NewWithRunner(&fakeRunner{t: t})
	f, cask, err := c.Info(context.Background(), "google-chrome", KindCask)
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if f != nil {
		t.Fatal("expected nil formula for cask query")
	}
	if cask.Token != "google-chrome" || cask.DisplayName() != "Google Chrome" {
		t.Errorf("got %q %q", cask.Token, cask.DisplayName())
	}
	if cask.IsInstalled() {
		t.Error("chrome fixture (installed:null) should not be installed")
	}
}

func TestTaps(t *testing.T) {
	c := NewWithRunner(&fakeRunner{t: t})
	taps, err := c.Taps(context.Background())
	if err != nil {
		t.Fatalf("Taps: %v", err)
	}
	if len(taps) != 2 {
		t.Fatalf("taps = %d, want 2", len(taps))
	}
	if !taps[0].Official {
		t.Error("homebrew/core should be official")
	}
	if got := taps[1].FormulaCount(); got != 2 {
		t.Errorf("hashicorp/tap formula count = %d, want 2", got)
	}
}

func TestOutdated(t *testing.T) {
	c := NewWithRunner(&fakeRunner{t: t})
	report, err := c.Outdated(context.Background())
	if err != nil {
		t.Fatalf("Outdated: %v", err)
	}
	if report.Total() != 3 {
		t.Fatalf("total = %d, want 3", report.Total())
	}
	tf := report.Formulae[1]
	if tf.Name != "hashicorp/tap/terraform" {
		t.Errorf("tapped formula name = %q", tf.Name)
	}
	if got := tf.InstalledVersion(); got != "1.15.3" {
		t.Errorf("installed version = %q, want 1.15.3", got)
	}
}

func TestSearch(t *testing.T) {
	c := NewWithRunner(&fakeRunner{t: t})
	names, err := c.Search(context.Background(), "wget", KindFormula, false)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	want := []string{"wget", "wget2", "wgetpaste"}
	if len(names) != len(want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, names[i], want[i])
		}
	}
}

func TestDoctorWarnings(t *testing.T) {
	c := NewWithRunner(&fakeRunner{t: t, doctorFile: "doctor_warnings.txt"})
	report, err := c.Doctor(context.Background())
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}
	if report.OK {
		t.Error("report should not be OK with warnings")
	}
	if len(report.Warnings) != 2 {
		t.Fatalf("warnings = %d, want 2", len(report.Warnings))
	}
	if report.Warnings[0].Title != "Some installed kegs have no formulae!" {
		t.Errorf("warning title = %q", report.Warnings[0].Title)
	}
	if len(report.Warnings[0].Details) == 0 {
		t.Error("first warning should have details")
	}
	// The leading "Please note…" preamble must be excluded, not folded into a warning.
	for _, w := range report.Warnings {
		for _, d := range w.Details {
			if strings.Contains(d, "Please note") {
				t.Errorf("preamble leaked into warning details: %q", d)
			}
		}
	}
	// The affected item brew indented must be preserved WITH its indentation so
	// the UI can highlight it as the actual problem.
	var foundIndentedItem bool
	for _, w := range report.Warnings {
		for _, d := range w.Details {
			if strings.TrimSpace(d) == "tflint" && d != strings.TrimSpace(d) {
				foundIndentedItem = true
			}
		}
	}
	if !foundIndentedItem {
		t.Error("expected the indented affected item 'tflint' to be preserved")
	}
}

func TestDoctorOK(t *testing.T) {
	c := NewWithRunner(&fakeRunner{t: t, doctorFile: "doctor_ok.txt"})
	report, err := c.Doctor(context.Background())
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}
	if !report.OK || len(report.Warnings) != 0 {
		t.Errorf("expected OK with no warnings, got OK=%v n=%d", report.OK, len(report.Warnings))
	}
}

func TestOutputErrorIncludesStderr(t *testing.T) {
	c := NewWithRunner(&fakeRunner{t: t, outErr: errors.New("exit 1"), outStderr: "Error: No such formula"})
	_, _, err := c.Installed(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); !strings.Contains(got, "No such formula") {
		t.Errorf("error %q should include stderr", got)
	}
}

func TestParseDescriptions(t *testing.T) {
	// Formula descriptions are kept verbatim, including a legitimate leading "(".
	f := parseDescriptions([]byte("wget: Internet file retriever\nbyacc: (Arguably) the best yacc variant\n"), KindFormula)
	if f["wget"] != "Internet file retriever" {
		t.Errorf("wget = %q", f["wget"])
	}
	if f["byacc"] != "(Arguably) the best yacc variant" {
		t.Errorf("formula leading parenthetical must be kept, got %q", f["byacc"])
	}
	// Cask lines carry a redundant "(Human Name)" that is stripped.
	c := parseDescriptions([]byte("firefox: (Mozilla Firefox) Web browser\n"), KindCask)
	if c["firefox"] != "Web browser" {
		t.Errorf("cask (Name) prefix should be stripped, got %q", c["firefox"])
	}
}

func TestParseJSONToleratesLeadingNotice(t *testing.T) {
	// brew occasionally prefixes a notice line before JSON on stdout.
	noisy := []byte("Warning: something\n{\"formulae\":[],\"casks\":[]}")
	var resp infoResponse
	if err := parseJSON(noisy, &resp); err != nil {
		t.Fatalf("parseJSON should tolerate leading notice: %v", err)
	}
}

// TestExecRunnerStream exercises the real os/exec streaming path (OS pipe,
// line scanning, terminal Done event) against a controlled command.
func TestExecRunnerStream(t *testing.T) {
	r := &execRunner{bin: "sh"}
	ch, err := r.Stream(context.Background(), "-c", "printf 'line1\nline2\n'")
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var lines []string
	var done Event
	for ev := range ch {
		if ev.Done {
			done = ev
			continue
		}
		lines = append(lines, ev.Line)
	}
	if len(lines) != 2 || lines[0] != "line1" || lines[1] != "line2" {
		t.Errorf("lines = %v, want [line1 line2]", lines)
	}
	if done.Err != nil {
		t.Errorf("Done.Err = %v, want nil", done.Err)
	}
}

func TestExecRunnerStreamPropagatesExitError(t *testing.T) {
	r := &execRunner{bin: "sh"}
	ch, err := r.Stream(context.Background(), "-c", "exit 3")
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var done Event
	for ev := range ch {
		if ev.Done {
			done = ev
		}
	}
	if done.Err == nil {
		t.Error("expected non-nil Done.Err for exit 3")
	}
}

// brew doctor exits non-zero when warnings exist; with output present that exit
// code is data, not failure, and must still parse into warnings.
func TestDoctorIgnoresExitCodeWhenOutputPresent(t *testing.T) {
	c := NewWithRunner(&fakeRunner{t: t, doctorFile: "doctor_warnings.txt", doctorExitErr: errors.New("exit status 1")})
	report, err := c.Doctor(context.Background())
	if err != nil {
		t.Fatalf("non-zero exit with output should not be an error: %v", err)
	}
	if report.OK || len(report.Warnings) != 2 {
		t.Errorf("expected 2 warnings, got OK=%v n=%d", report.OK, len(report.Warnings))
	}
}

// A genuine failure to run brew (error, no output) must surface — not be reported
// as a healthy system.
func TestDoctorRunFailureSurfaces(t *testing.T) {
	c := NewWithRunner(&fakeRunner{t: t, outErr: errors.New(`exec: "brew": not found`)})
	report, err := c.Doctor(context.Background())
	if err == nil {
		t.Fatal("a failure to run brew must surface as an error, not a healthy report")
	}
	if report != nil {
		t.Errorf("expected nil report on run failure, got %+v", report)
	}
}

// The "--" end-of-options guard must sit immediately before any user-controlled
// operand so a value like "--macports" or "-rf" can never be read as a brew flag.
func TestEndOfOptionsGuard(t *testing.T) {
	fr := &fakeRunner{t: t}
	c := NewWithRunner(fr)

	if _, err := c.Search(context.Background(), "--macports", KindFormula, false); err != nil {
		t.Fatalf("Search: %v", err)
	}
	assertGuarded(t, "Search", fr.lastArgs, "--macports")

	if _, _, err := c.Info(context.Background(), "-rf", KindFormula); err != nil {
		t.Fatalf("Info: %v", err)
	}
	assertGuarded(t, "Info", fr.lastArgs, "-rf")

	if _, err := c.Upgrade(context.Background(), "pkg"); err != nil {
		t.Fatalf("Upgrade: %v", err)
	}
	if len(fr.lastArgs) < 2 || fr.lastArgs[0] != "upgrade" || fr.lastArgs[1] != "--" {
		t.Errorf("Upgrade args %v must insert '--' right after the subcommand", fr.lastArgs)
	}

	if _, err := c.Install(context.Background(), KindFormula, "-rf"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	assertGuarded(t, "Install", fr.lastArgs, "-rf")
}

func TestInstallPassesKindFlag(t *testing.T) {
	fr := &fakeRunner{t: t}
	c := NewWithRunner(fr)
	if _, err := c.Install(context.Background(), KindCask, "firefox"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	joined := strings.Join(fr.lastArgs, " ")
	if !strings.HasPrefix(joined, "install --cask") {
		t.Errorf("cask install args %q should start with 'install --cask'", joined)
	}
}

// assertGuarded checks args end with the sequence ["--", operand].
func assertGuarded(t *testing.T, what string, args []string, operand string) {
	t.Helper()
	n := len(args)
	if n < 2 || args[n-2] != "--" || args[n-1] != operand {
		t.Errorf(`%s args %v must end with ["--", %q]`, what, args, operand)
	}
}

func TestSearchPassesEvalAllFlag(t *testing.T) {
	fr := &fakeRunner{t: t}
	c := NewWithRunner(fr)
	if _, err := c.Search(context.Background(), "x", KindFormula, true); err != nil {
		t.Fatalf("Search: %v", err)
	}
	joined := strings.Join(fr.lastArgs, " ")
	if !strings.Contains(joined, "--eval-all") || !strings.Contains(joined, "--formula") {
		t.Errorf("args %q should include --eval-all and --formula", joined)
	}

	if _, err := c.Search(context.Background(), "x", KindCask, false); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if strings.Contains(strings.Join(fr.lastArgs, " "), "--eval-all") {
		t.Errorf("args %q should NOT include --eval-all when evalAll=false", fr.lastArgs)
	}
}

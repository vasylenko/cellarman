//go:build integration

// These tests run the real `brew` binary to prove the client parses actual
// Homebrew output, not just hand-written fixtures. They are environment- and
// network-dependent, so they are gated behind the `integration` build tag:
//
//	go test -tags integration ./internal/brew/
package brew

import (
	"context"
	"testing"
	"time"
)

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestIntegrationInstalled(t *testing.T) {
	formulae, casks, err := New().Installed(testCtx(t))
	if err != nil {
		t.Fatalf("Installed: %v", err)
	}
	t.Logf("installed: %d formulae, %d casks", len(formulae), len(casks))
	for _, f := range formulae {
		if f.Name == "" {
			t.Fatalf("formula with empty name: %+v", f)
		}
		if !f.IsInstalled() {
			t.Errorf("%q from --installed should report installed", f.Name)
		}
	}
}

func TestIntegrationInfo(t *testing.T) {
	// go is installed in this environment; assert the real schema maps through.
	f, cask, err := New().Info(testCtx(t), "go", KindFormula)
	if err != nil {
		t.Fatalf("Info(go): %v", err)
	}
	if cask != nil || f == nil {
		t.Fatalf("expected a formula, got formula=%v cask=%v", f, cask)
	}
	if f.Name != "go" || f.Versions.Stable == "" || f.Homepage == "" {
		t.Errorf("unexpected formula fields: %+v", f)
	}
}

func TestIntegrationOutdated(t *testing.T) {
	report, err := New().Outdated(testCtx(t))
	if err != nil {
		t.Fatalf("Outdated: %v", err)
	}
	t.Logf("outdated: %d total", report.Total())
	for _, p := range report.Formulae {
		if p.Name == "" || p.CurrentVersion == "" {
			t.Errorf("malformed outdated entry: %+v", p)
		}
	}
}

func TestIntegrationTaps(t *testing.T) {
	taps, err := New().Taps(testCtx(t))
	if err != nil {
		t.Fatalf("Taps: %v", err)
	}
	t.Logf("taps: %d", len(taps))
	if len(taps) == 0 {
		t.Skip("no taps installed")
	}
	for _, tp := range taps {
		if tp.Name == "" {
			t.Errorf("tap with empty name: %+v", tp)
		}
	}
}

func TestIntegrationSearch(t *testing.T) {
	names, err := New().Search(testCtx(t), "wget", KindFormula, false)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	t.Logf("search 'wget': %v", names)
	found := false
	for _, n := range names {
		if n == "wget" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'wget' in results, got %v", names)
	}
}

func TestIntegrationDoctor(t *testing.T) {
	report, err := New().Doctor(testCtx(t))
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}
	// Either healthy or warnings — both are valid; just prove it parsed.
	t.Logf("doctor: ok=%v warnings=%d", report.OK, len(report.Warnings))
	if !report.OK && len(report.Warnings) == 0 && report.Raw == "" {
		t.Error("not OK but no warnings and no raw output — parse likely failed")
	}
}

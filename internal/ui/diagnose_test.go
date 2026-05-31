package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/vasylenko/cellarman/internal/brew"
)

// loadedDiagnose returns a diagnose view sized and populated with one warning.
func loadedDiagnose(t *testing.T, f *fakeBrew) child {
	t.Helper()
	m := newDiagnoseView(f)
	dm := m.(diagnoseModel)
	m, _ = m.Update(contentSizeMsg{width: 100, height: 20})
	m, _ = m.Update(dm.load()()) // run the load Cmd and feed its result back
	if m.(diagnoseModel).state != stateLoaded {
		t.Fatalf("expected loaded, got state %d", m.(diagnoseModel).state)
	}
	return m
}

func warnedBrew() *fakeBrew {
	return &fakeBrew{doctor: &brew.DoctorReport{
		OK: false,
		Warnings: []brew.Warning{
			{Title: "Some installed kegs have no formulae!", Details: []string{"tflint"}},
		},
	}}
}

func TestDiagnoseLoadShowsWarning(t *testing.T) {
	m := loadedDiagnose(t, warnedBrew())
	dm := m.(diagnoseModel)
	if !strings.Contains(dm.View(), "Some installed kegs have no formulae!") {
		t.Errorf("view should show the warning title:\n%s", dm.View())
	}
	if !strings.Contains(dm.View(), "1 warning(s)") {
		t.Errorf("view should show the warning count:\n%s", dm.View())
	}
}

func TestDiagnoseHighlightsAffectedItems(t *testing.T) {
	m := loadedDiagnose(t, &fakeBrew{doctor: &brew.DoctorReport{
		OK: false,
		Warnings: []brew.Warning{{
			Title:   "Some installed kegs have no formulae!",
			Details: []string{"You should find replacements for the following formulae:", "  tflint"},
		}},
	}})
	v := m.(diagnoseModel).View()
	if !strings.Contains(v, "tflint") {
		t.Errorf("the affected item must be shown:\n%s", v)
	}
	if !strings.Contains(v, "•") {
		t.Errorf("indented affected items should render as bullets:\n%s", v)
	}
}

func TestDiagnoseHealthy(t *testing.T) {
	m := loadedDiagnose(t, &fakeBrew{doctor: &brew.DoctorReport{OK: true}})
	dm := m.(diagnoseModel)
	if !strings.Contains(dm.View(), "healthy") {
		t.Errorf("healthy report should show the healthy banner:\n%s", dm.View())
	}
}

func TestDiagnoseCleanupStreamsThenRerunsDoctor(t *testing.T) {
	f := &fakeBrew{
		doctor: &brew.DoctorReport{OK: true},
		events: []brew.Event{{Line: "Removing: ~/Library/Caches"}, {Line: "done"}},
	}
	m := loadedDiagnose(t, f)

	// 'c' starts cleanup; the view leaves loaded for the running fix.
	m, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if !m.(diagnoseModel).running {
		t.Fatal("c should start the cleanup fix")
	}
	if cmd == nil {
		t.Fatal("starting cleanup should issue a command")
	}

	// Drive the stream from the fake's channel through the waitForEvent loop.
	ch, _ := f.Cleanup(context.Background())
	m, cmd = m.Update(diagnoseStartedMsg{ch})
	for {
		msg := cmd()
		m, cmd = m.Update(msg)
		if _, done := msg.(diagnoseDoneMsg); done {
			break
		}
		dm := m.(diagnoseModel)
		if s, ok := msg.(diagnoseLineMsg); ok && !strings.Contains(dm.report.View(), string(s)) {
			t.Errorf("streamed line %q should appear in the viewport:\n%s", string(s), dm.report.View())
		}
	}

	// Done with no error re-runs doctor: back to loading with a reload command.
	if m.(diagnoseModel).state != stateLoading {
		t.Fatalf("done should re-run doctor (state loading), got %d", m.(diagnoseModel).state)
	}
	if cmd == nil {
		t.Fatal("done should issue a doctor reload command")
	}
}

func TestDiagnoseLoadError(t *testing.T) {
	m := newDiagnoseView(&fakeBrew{err: errors.New("doctor exploded")})
	dm := m.(diagnoseModel)
	m, _ = m.Update(dm.load()())
	dm = m.(diagnoseModel)
	if dm.state != stateError {
		t.Fatalf("state = %d, want error", dm.state)
	}
	if !strings.Contains(dm.View(), "doctor exploded") {
		t.Errorf("error view should surface the message:\n%s", dm.View())
	}
}

func TestDiagnoseRetryRerunsDoctor(t *testing.T) {
	m := newDiagnoseView(&fakeBrew{err: errors.New("boom")})
	dm := m.(diagnoseModel)
	m, _ = m.Update(dm.load()()) // -> error
	m, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if m.(diagnoseModel).state != stateLoading {
		t.Fatal("retry should reset to loading")
	}
	if cmd == nil {
		t.Fatal("retry should issue a reload command")
	}
}

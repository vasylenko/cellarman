package ui

import (
	"bytes"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/serhii-vasylenko/brew-tui/internal/brew"
)

// fullBrew is a fake with data for every view, so an end-to-end run renders real
// content in each screen.
func fullBrew() *fakeBrew {
	b := sampleBrew()
	b.outdated = &brew.OutdatedReport{
		Formulae: []brew.OutdatedPackage{
			{Name: "gnutls", InstalledVersions: []string{"3.8.13_1"}, CurrentVersion: "3.8.13_2"},
		},
	}
	b.doctor = &brew.DoctorReport{
		OK:       false,
		Warnings: []brew.Warning{{Title: "Some installed kegs have no formulae!", Details: []string{"tflint"}}},
	}
	b.searchHits = []string{"wget", "wget2"}
	b.descs = map[string]string{"wget": "retrieve files over HTTP", "wget2": "successor to wget"}
	b.infoFormula = &brew.Formula{Name: "wget", Versions: brew.Versions{Stable: "1.21"}, Desc: "internet file retriever"}
	return b
}

func waitForText(t *testing.T, tm *teatest.TestModel, want string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte(want))
	}, teatest.WithDuration(3*time.Second), teatest.WithCheckInterval(20*time.Millisecond))
}

// TestE2ETabThroughAllViews runs the real program and tab-cycles through every
// view, asserting each one renders its own content. Assertions target strings
// unique to each view (teatest output is cumulative) so a match proves that
// specific view actually rendered, with its async load resolved.
func TestE2ETabThroughAllViews(t *testing.T) {
	tm := teatest.NewTestModel(t, NewRoot(fullBrew()), teatest.WithInitialTermSize(120, 40))

	// Browse loads on startup: section header with the formula count.
	waitForText(t, tm, "Formulae (2)")

	tm.Send(tea.KeyPressMsg{Code: tea.KeyTab}) // -> Search
	waitForText(t, tm, "type a query")

	tm.Send(tea.KeyPressMsg{Code: tea.KeyTab}) // -> Upgrade (loads outdated)
	waitForText(t, tm, "enter upgrade")

	tm.Send(tea.KeyPressMsg{Code: tea.KeyTab}) // -> Diagnose (runs doctor)
	waitForText(t, tm, "Some installed kegs have no formulae!")

	// q quits from a non-input view.
	tm.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))

	if _, ok := tm.FinalModel(t).(Root); !ok {
		t.Fatalf("final model is not Root")
	}
}

// TestE2ESearchFlow drives the search view end to end: type a query, submit, see
// results, open a result's detail — all in a running program.
func TestE2ESearchFlow(t *testing.T) {
	tm := teatest.NewTestModel(t, NewRoot(fullBrew()), teatest.WithInitialTermSize(120, 40))
	waitForText(t, tm, "Formulae (2)") // browse ready

	tm.Send(tea.KeyPressMsg{Code: tea.KeyTab}) // -> Search (input focused)
	waitForText(t, tm, "type a query")

	for _, r := range "wget" {
		tm.Send(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})   // run search
	waitForText(t, tm, "retrieve files over HTTP") // results + description column rendered in one frame

	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})  // open detail of highlighted result
	waitForText(t, tm, "internet file retriever") // hydrated Info rendered

	tm.Send(tea.KeyPressMsg{Code: tea.KeyEsc})
	tm.Send(tea.KeyPressMsg{Code: tea.KeyTab}) // leave search so q isn't typed
	tm.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

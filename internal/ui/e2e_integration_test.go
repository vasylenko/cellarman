//go:build integration

// End-to-end test wiring the REAL brew client through the running program. It
// proves the full chain — brew binary -> JSON/text parsing -> UI render — works,
// not just against fakes. Gated behind the `integration` tag:
//
//	go test -tags integration ./internal/ui/
package ui

import (
	"bytes"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/serhii-vasylenko/brew-tui/internal/brew"
)

func TestE2ERealBrew(t *testing.T) {
	tm := teatest.NewTestModel(t, NewRoot(brew.New()), teatest.WithInitialTermSize(120, 40))

	wait := func(want string) {
		t.Helper()
		teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
			return bytes.Contains(b, []byte(want))
		}, teatest.WithDuration(20*time.Second), teatest.WithCheckInterval(50*time.Millisecond))
	}

	// Browse renders real installed formulae (the count varies; the header prefix
	// is always present once the real Installed() call resolves).
	wait("Formulae (")

	tm.Send(tea.KeyPressMsg{Code: tea.KeyTab}) // Search
	wait("type a query")

	tm.Send(tea.KeyPressMsg{Code: tea.KeyTab}) // Upgrade — real Outdated()
	wait("refresh")                            // hint appears whether or not anything is outdated

	tm.Send(tea.KeyPressMsg{Code: tea.KeyTab}) // Diagnose — real brew doctor
	wait("cleanup")                            // diagnose hint appears once the report loads

	tm.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
}

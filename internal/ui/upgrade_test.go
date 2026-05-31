package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/vasylenko/cellarman/internal/brew"
)

// outdatedBrew is a fake with one outdated formula and a canned upgrade stream.
func outdatedBrew() *fakeBrew {
	return &fakeBrew{
		outdated: &brew.OutdatedReport{
			Formulae: []brew.OutdatedPackage{
				{Name: "gnutls", InstalledVersions: []string{"3.8.13_1"}, CurrentVersion: "3.8.13_2"},
			},
		},
		events: []brew.Event{{Line: "==> Upgrading gnutls"}, {Line: "done"}},
	}
}

// loadedUpgrade returns an upgrade view sized and populated from the given brew.
func loadedUpgrade(t *testing.T, b Brew) child {
	t.Helper()
	m := newUpgradeView(b)
	um := m.(upgradeModel)
	m, _ = m.Update(contentSizeMsg{width: 100, height: 20})
	m, _ = m.Update(um.load()()) // run the load Cmd and feed its result back
	if m.(upgradeModel).state != stateLoaded {
		t.Fatalf("expected loaded, got state %d", m.(upgradeModel).state)
	}
	return m
}

func TestUpgradeLoadPopulatesTable(t *testing.T) {
	m := loadedUpgrade(t, outdatedBrew())
	um := m.(upgradeModel)
	if got := len(um.table.Rows()); got != 1 {
		t.Fatalf("outdated rows = %d, want 1", got)
	}
	if !strings.Contains(um.View(), "3.8.13_1 → 3.8.13_2") {
		t.Errorf("table should show the version change:\n%s", um.View())
	}
}

func TestUpgradeEmptyIsUpToDate(t *testing.T) {
	m := loadedUpgrade(t, &fakeBrew{outdated: &brew.OutdatedReport{}})
	if !strings.Contains(m.(upgradeModel).View(), "Everything is up to date") {
		t.Errorf("empty outdated should show up-to-date message:\n%s", m.(upgradeModel).View())
	}
}

func TestUpgradeSelectionToggle(t *testing.T) {
	m := loadedUpgrade(t, outdatedBrew())
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace}) // mark highlighted row
	um := m.(upgradeModel)
	if !um.rows[0].selected {
		t.Fatal("space should select the highlighted row")
	}
	if names := um.selectedNames(); len(names) != 1 || names[0] != "gnutls" {
		t.Fatalf("selected names = %v, want [gnutls]", names)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"}) // toggle off
	if m.(upgradeModel).rows[0].selected {
		t.Fatal("x should deselect the row")
	}
}

func TestUpgradeStreamsToCompletion(t *testing.T) {
	m := loadedUpgrade(t, outdatedBrew())
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})    // select the row
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // upgrade selected
	if !m.(upgradeModel).upgrading {
		t.Fatal("enter should upgrade the selected package")
	}
	if cmd == nil {
		t.Fatal("starting an upgrade should issue a stream command")
	}

	// Drive the stream: run each Cmd, feed its msg back, until done. The fake
	// channel is buffered and closed, so cmd() never blocks.
	for range 100 {
		msg := cmd()
		m, cmd = m.Update(msg)
		// Guard the freeze-after-line-one bug: a line must always hand back a
		// follow-up Cmd to pull the next event.
		if _, ok := msg.(upgradeLineMsg); ok && cmd == nil {
			t.Fatal("handling a line must re-issue waitForEvent (got nil cmd)")
		}
		if _, ok := msg.(upgradeDoneMsg); ok {
			break
		}
	}

	if cmd == nil {
		t.Error("a clean upgrade should broadcast packagesChanged so views refresh")
	}
	um := m.(upgradeModel)
	if um.upgrading || !um.done {
		t.Fatalf("after the stream: upgrading=%v done=%v, want false/true", um.upgrading, um.done)
	}
	if !strings.Contains(um.logBuf, "==> Upgrading gnutls") || !strings.Contains(um.logBuf, "done") {
		t.Errorf("log should contain the streamed lines:\n%s", um.logBuf)
	}
}

func TestUpgradeEnterWithoutSelectionDoesNothing(t *testing.T) {
	m := loadedUpgrade(t, outdatedBrew())
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // nothing selected
	um := m.(upgradeModel)
	if um.upgrading {
		t.Fatal("enter with no selection must NOT start an upgrade")
	}
	if cmd != nil {
		t.Fatal("enter with no selection should not issue a command")
	}
	if !strings.Contains(um.View(), "Nothing selected") {
		t.Errorf("expected guidance notice, got:\n%s", um.View())
	}
}

func TestUpgradeAllKeyStartsUpgrade(t *testing.T) {
	m := loadedUpgrade(t, outdatedBrew())
	m, cmd := m.Update(tea.KeyPressMsg{Code: 'U', Text: "U"}) // upgrade all
	if !m.(upgradeModel).upgrading {
		t.Fatal("U should start an upgrade without requiring a selection")
	}
	if cmd == nil {
		t.Fatal("U should issue a stream command")
	}
}

func TestUpgradePackagesChangedReloads(t *testing.T) {
	m := loadedUpgrade(t, outdatedBrew())
	_, cmd := m.Update(packagesChangedMsg{})
	if cmd == nil {
		t.Fatal("packagesChangedMsg should reload the outdated list")
	}
}

// A stream that fails to start must leave the view recoverable: upgrading is
// reset so the error-state retry key is reachable (regression guard).
func TestUpgradeRecoversFromStreamStartFailure(t *testing.T) {
	b := outdatedBrew()
	b.streamErr = errors.New("cannot start brew upgrade")
	m := loadedUpgrade(t, b)

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})    // select a row
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // attempt upgrade
	if cmd == nil {
		t.Fatal("starting an upgrade should issue a stream command")
	}
	m, _ = m.Update(cmd()) // startStream's onErr resolves to upgradeErrMsg

	um := m.(upgradeModel)
	if um.state != stateError {
		t.Fatalf("a stream-start failure should land in error state, got %d", um.state)
	}
	if um.upgrading {
		t.Fatal("upgrading must reset so the retry key is reachable")
	}

	m, cmd = m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if m.(upgradeModel).state != stateLoading {
		t.Fatal("r should retry from the error state (would be swallowed if still upgrading)")
	}
	if cmd == nil {
		t.Fatal("retry should issue a reload command")
	}
}

// esc during an upgrade cancels the context bound to the brew command.
func TestUpgradeAbortCancelsContext(t *testing.T) {
	b := outdatedBrew()
	m := loadedUpgrade(t, b)

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})    // select a row
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // start upgrade
	m, _ = m.Update(cmd())                                  // startStream calls b.Upgrade(ctx) -> upgradeStartedMsg
	if b.upgradeCtx == nil {
		t.Fatal("Upgrade should have been called with a context")
	}
	if b.upgradeCtx.Err() != nil {
		t.Fatal("context should be live before abort")
	}

	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc}) // abort
	if b.upgradeCtx.Err() == nil {
		t.Fatal("esc should cancel the in-flight upgrade context")
	}
}

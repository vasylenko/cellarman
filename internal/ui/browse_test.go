package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// loadedBrowse returns a browse view sized and fully populated with sampleBrew
// data, feeding both concurrent load results back as Init would.
func loadedBrowse(t *testing.T) child {
	t.Helper()
	m := newBrowseView(sampleBrew())
	bm := m.(browseModel)
	m, _ = m.Update(contentSizeMsg{width: 100, height: 20})
	m, _ = m.Update(bm.loadInstalled()())
	m, _ = m.Update(bm.loadTaps()())
	if m.(browseModel).state != stateLoaded {
		t.Fatalf("expected loaded, got state %d", m.(browseModel).state)
	}
	return m
}

func TestBrowseLoadPopulatesTable(t *testing.T) {
	m := loadedBrowse(t)
	bm := m.(browseModel)
	if got := len(bm.table.Rows()); got != 2 {
		t.Fatalf("formulae rows = %d, want 2", got)
	}
	if !strings.Contains(bm.View(), "Formulae (2)") {
		t.Errorf("section header missing formula count:\n%s", bm.View())
	}
}

func TestBrowseSectionSwitch(t *testing.T) {
	m := loadedBrowse(t)
	m, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"}) // next section -> casks
	bm := m.(browseModel)
	if bm.section != sectionCasks {
		t.Fatalf("section = %d, want casks", bm.section)
	}
	if got := len(bm.table.Rows()); got != 1 {
		t.Fatalf("cask rows = %d, want 1", got)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"}) // prev -> back to formulae
	if m.(browseModel).section != sectionFormulae {
		t.Fatalf("expected back to formulae")
	}
}

func TestBrowseOpenAndCloseDetail(t *testing.T) {
	m := loadedBrowse(t)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	bm := m.(browseModel)
	if !bm.showing {
		t.Fatal("enter should open the detail panel")
	}
	if !strings.Contains(bm.detail.View(), "1.26.3") {
		t.Errorf("detail should show the selected formula version:\n%s", bm.detail.View())
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.(browseModel).showing {
		t.Fatal("esc should close the detail panel")
	}
}

func TestBrowseLoadError(t *testing.T) {
	m := newBrowseView(&fakeBrew{err: errors.New("brew exploded")})
	bm := m.(browseModel)
	m, _ = m.Update(bm.loadInstalled()())
	bm = m.(browseModel)
	if bm.state != stateError {
		t.Fatalf("state = %d, want error", bm.state)
	}
	if !strings.Contains(bm.View(), "brew exploded") {
		t.Errorf("error view should surface the message:\n%s", bm.View())
	}
}

func TestBrowseManualRefresh(t *testing.T) {
	m := loadedBrowse(t)
	m, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if m.(browseModel).state != stateLoading {
		t.Fatal("r should refresh the installed list (loading)")
	}
	if cmd == nil {
		t.Fatal("r should issue a reload command")
	}
}

func TestBrowseRefreshesOnPackagesChanged(t *testing.T) {
	m := loadedBrowse(t)
	_, cmd := m.Update(packagesChangedMsg{})
	if cmd == nil {
		t.Fatal("packagesChangedMsg should trigger a background reload when loaded")
	}
}

func TestBrowseRetryReloads(t *testing.T) {
	m := newBrowseView(&fakeBrew{err: errors.New("boom")})
	bm := m.(browseModel)
	m, _ = m.Update(bm.loadInstalled()()) // -> error
	m, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if m.(browseModel).state != stateLoading {
		t.Fatal("retry should reset to loading")
	}
	if cmd == nil {
		t.Fatal("retry should issue a reload command")
	}
}

// The point of the split: the primary content paints before the slow taps load,
// and taps fill in afterward without blocking it.
func TestBrowseTapsLoadAfterInstalled(t *testing.T) {
	m := newBrowseView(sampleBrew())
	bm := m.(browseModel)
	m, _ = m.Update(contentSizeMsg{width: 100, height: 20})

	m, _ = m.Update(bm.loadInstalled()()) // primary lands first
	bm = m.(browseModel)
	if bm.state != stateLoaded {
		t.Fatalf("primary content should be loaded, got state %d", bm.state)
	}
	if bm.tapsState != stateLoading {
		t.Fatalf("taps should still be loading, got %d", bm.tapsState)
	}
	if !strings.Contains(bm.View(), "Taps (…)") {
		t.Errorf("header should mark taps still loading:\n%s", bm.View())
	}

	m, _ = m.Update(bm.loadTaps()()) // taps land in the background
	bm = m.(browseModel)
	if bm.tapsState != stateLoaded {
		t.Fatalf("taps should be loaded, got %d", bm.tapsState)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"}) // -> taps (wrap left)
	bm = m.(browseModel)
	if bm.section != sectionTaps {
		t.Fatalf("expected taps section, got %d", bm.section)
	}
	if got := len(bm.table.Rows()); got != 2 {
		t.Fatalf("tap rows = %d, want 2", got)
	}
}

func TestBrowseTapsSectionShowsLoading(t *testing.T) {
	m := newBrowseView(sampleBrew())
	bm := m.(browseModel)
	m, _ = m.Update(contentSizeMsg{width: 100, height: 20})
	m, _ = m.Update(bm.loadInstalled()()) // primary loaded, taps still pending

	m, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"}) // -> taps section
	bm = m.(browseModel)
	if bm.section != sectionTaps {
		t.Fatalf("expected taps section, got %d", bm.section)
	}
	if !strings.Contains(bm.View(), "Loading taps…") {
		t.Errorf("taps section should show its loading state:\n%s", bm.View())
	}
}

// A taps failure must not take down the primary view: formulae stay usable and
// the error is confined to the Taps section.
func TestBrowseTapsErrorKeepsPrimaryView(t *testing.T) {
	m := newBrowseView(sampleBrew())
	bm := m.(browseModel)
	m, _ = m.Update(contentSizeMsg{width: 100, height: 20})
	m, _ = m.Update(bm.loadInstalled()())                     // primary OK
	m, _ = m.Update(browseTapsErrMsg{errors.New("tap boom")}) // taps fail
	bm = m.(browseModel)
	if bm.state != stateLoaded {
		t.Fatalf("primary content must stay loaded despite taps error, got %d", bm.state)
	}
	if got := len(bm.table.Rows()); got != 2 {
		t.Fatalf("formulae rows = %d, want 2", got)
	}

	m, _ = bm.Update(tea.KeyPressMsg{Code: 'h', Text: "h"}) // -> taps section
	if !strings.Contains(m.(browseModel).View(), "tap boom") {
		t.Errorf("taps section should surface its error:\n%s", m.(browseModel).View())
	}
}

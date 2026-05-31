package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// loadedBrowse returns a browse view sized and populated with sampleBrew data.
func loadedBrowse(t *testing.T) child {
	t.Helper()
	m := newBrowseView(sampleBrew())
	bm := m.(browseModel)
	m, _ = m.Update(contentSizeMsg{width: 100, height: 20})
	m, _ = m.Update(bm.load()()) // run the load Cmd and feed its result back
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
	m, _ = m.Update(bm.load()())
	bm = m.(browseModel)
	if bm.state != stateError {
		t.Fatalf("state = %d, want error", bm.state)
	}
	if !strings.Contains(bm.View(), "brew exploded") {
		t.Errorf("error view should surface the message:\n%s", bm.View())
	}
}

func TestBrowseRetryReloads(t *testing.T) {
	m := newBrowseView(&fakeBrew{err: errors.New("boom")})
	bm := m.(browseModel)
	m, _ = m.Update(bm.load()()) // -> error
	m, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if m.(browseModel).state != stateLoading {
		t.Fatal("retry should reset to loading")
	}
	if cmd == nil {
		t.Fatal("retry should issue a reload command")
	}
}

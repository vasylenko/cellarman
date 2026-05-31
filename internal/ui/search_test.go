package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/serhii-vasylenko/brew-tui/internal/brew"
)

// typeQuery feeds each rune of term into the focused query input.
func typeQuery(m child, term string) child {
	for _, r := range term {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// runSearch types term, presses Enter to submit, then resolves the search Cmd
// synchronously and feeds the result back — mirroring browse_test's load pattern.
func runSearch(t *testing.T, m child, term string) child {
	t.Helper()
	m = typeQuery(m, term)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	sm := m.(searchModel)
	if sm.state != stateLoading {
		t.Fatalf("enter should start loading, got state %d", sm.state)
	}
	m, _ = m.Update(sm.search(sm.term, sm.section)()) // run the search Cmd, feed result back
	return m
}

func newSizedSearch(b Brew) child {
	m := newSearchView(b)
	m, _ = m.Update(contentSizeMsg{width: 100, height: 20})
	return m
}

func TestSearchTypeAndEnterPopulatesTable(t *testing.T) {
	m := newSizedSearch(&fakeBrew{searchHits: []string{"wget", "wget2"}})
	if !m.(searchModel).capturingInput() {
		t.Fatal("input should be focused before a search runs")
	}
	m = runSearch(t, m, "wget")
	sm := m.(searchModel)
	if sm.state != stateLoaded {
		t.Fatalf("state = %d, want loaded", sm.state)
	}
	if got := len(sm.table.Rows()); got != 2 {
		t.Fatalf("result rows = %d, want 2", got)
	}
	if sm.capturingInput() {
		t.Error("input should blur after results arrive")
	}
}

func TestSearchSwitchToCasks(t *testing.T) {
	m := newSizedSearch(&fakeBrew{searchHits: []string{"firefox"}})
	m = runSearch(t, m, "fire") // results blur the input
	m, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	sm := m.(searchModel)
	if sm.section != searchCasks {
		t.Fatalf("section = %d, want casks", sm.section)
	}
	if sm.state != stateLoading {
		t.Fatal("switching kind with a query should re-run the search")
	}
}

func TestSearchOpenDetailHydrates(t *testing.T) {
	m := newSizedSearch(&fakeBrew{
		searchHits:  []string{"wget", "wget2"},
		infoFormula: &brew.Formula{Name: "wget", Versions: brew.Versions{Stable: "1.21"}, Desc: "retriever"},
	})
	m = runSearch(t, m, "wget")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // open the highlighted result
	sm := m.(searchModel)
	if !sm.showing {
		t.Fatal("enter on a result should open the detail panel")
	}
	if !strings.Contains(sm.detail.View(), "retriever") {
		t.Errorf("detail should contain hydrated info:\n%s", sm.detail.View())
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.(searchModel).showing {
		t.Fatal("esc should close the detail panel")
	}
}

func TestSearchNoMatches(t *testing.T) {
	m := newSizedSearch(&fakeBrew{searchHits: nil})
	m = runSearch(t, m, "nopackage")
	if !strings.Contains(m.(searchModel).View(), "no matches") {
		t.Errorf("empty results should render 'no matches':\n%s", m.(searchModel).View())
	}
}

func TestSearchErrorAndRetry(t *testing.T) {
	m := newSizedSearch(&fakeBrew{err: errors.New("brew exploded")})
	m = typeQuery(m, "wget")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	sm := m.(searchModel)
	m, _ = m.Update(sm.search(sm.term, sm.section)()) // -> error
	sm = m.(searchModel)
	if sm.state != stateError {
		t.Fatalf("state = %d, want error", sm.state)
	}
	if !strings.Contains(sm.View(), "brew exploded") {
		t.Errorf("error view should surface the message:\n%s", sm.View())
	}
	m, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if m.(searchModel).state != stateLoading {
		t.Fatal("retry should reset to loading")
	}
	if cmd == nil {
		t.Fatal("retry should issue a reload command")
	}
}

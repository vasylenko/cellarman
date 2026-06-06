package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/vasylenko/cellarman/internal/brew"
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
	m, cmd := m.Update(sm.search(sm.term, sm.section)()) // -> searchResultsMsg (+ descs cmd)
	if cmd != nil {
		m, _ = m.Update(cmd()) // -> searchDescsMsg, descriptions fill in
	}
	return m
}

func newSizedSearch(b Brew) child {
	m := newSearchView(b)
	m, _ = m.Update(contentSizeMsg{width: 100, height: 20})
	return m
}

// openedSearchDetail runs a search and opens the first result's detail, leaving
// the model in its detail-showing state — the surface install runs from.
func openedSearchDetail(t *testing.T, f *fakeBrew) child {
	t.Helper()
	m := newSizedSearch(f)
	m = runSearch(t, m, "wget")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // start hydrating the result
	sm := m.(searchModel)
	m, _ = m.Update(sm.detailCmd(sm.detailName, sm.detailKind)()) // resolve Info -> showing
	if !m.(searchModel).showing {
		t.Fatal("detail panel should be open after Info resolves")
	}
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
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // start hydrating the result
	sm := m.(searchModel)
	if sm.state != stateLoading {
		t.Fatal("opening a result should fetch its detail asynchronously (loading state)")
	}
	m, _ = m.Update(sm.detailCmd("wget", sm.section.kind())()) // resolve Info, feed detail back
	sm = m.(searchModel)
	if !sm.showing {
		t.Fatal("detail panel should open once Info resolves")
	}
	if !strings.Contains(sm.detail.View(), "retriever") {
		t.Errorf("detail should contain hydrated info:\n%s", sm.detail.View())
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.(searchModel).showing {
		t.Fatal("esc should close the detail panel")
	}
}

func TestSearchShowsDescriptions(t *testing.T) {
	m := newSizedSearch(&fakeBrew{
		searchHits: []string{"wget", "wget2"},
		descs:      map[string]string{"wget": "Internet file retriever"},
	})
	m = runSearch(t, m, "wget")
	sm := m.(searchModel)
	if sm.descFor("wget") != "Internet file retriever" {
		t.Errorf("descFor(wget) = %q", sm.descFor("wget"))
	}
	if sm.descFor("homebrew/core/wget") != "Internet file retriever" {
		t.Error("descFor should fall back to short name for tap-qualified results")
	}
	if !strings.Contains(sm.View(), "Internet file retriever") {
		t.Errorf("results should show the description:\n%s", sm.View())
	}
}

func TestSearchNoMatches(t *testing.T) {
	m := newSizedSearch(&fakeBrew{searchHits: nil})
	m = runSearch(t, m, "nopackage")
	if !strings.Contains(m.(searchModel).View(), "no matches") {
		t.Errorf("empty results should render 'no matches':\n%s", m.(searchModel).View())
	}
}

func TestSearchInstallStreamsToCompletion(t *testing.T) {
	f := &fakeBrew{
		searchHits:  []string{"wget"},
		infoFormula: &brew.Formula{Name: "wget", Versions: brew.Versions{Stable: "1.21"}, Desc: "retriever"},
		events:      []brew.Event{{Line: "==> Installing wget"}, {Line: "done"}},
	}
	m := openedSearchDetail(t, f)
	if m.(searchModel).detailInstalled {
		t.Fatal("fixture formula should not be installed")
	}

	m, cmd := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	if !m.(searchModel).installing {
		t.Fatal("i should start the install")
	}
	if cmd == nil {
		t.Fatal("starting an install should issue a stream command")
	}

	// Drive the stream to completion the same way the upgrade test does: the fake
	// channel is buffered and closed, so cmd() never blocks.
	for range 100 {
		msg := cmd()
		m, cmd = m.Update(msg)
		// Guard the freeze-after-line-one bug: a line must always hand back a
		// follow-up Cmd to pull the next event.
		if _, ok := msg.(searchInstallLineMsg); ok && cmd == nil {
			t.Fatal("handling a line must re-issue nextInstall (got nil cmd)")
		}
		if _, ok := msg.(searchInstallDoneMsg); ok {
			break
		}
	}
	if cmd == nil {
		t.Error("a clean install should broadcast packagesChanged so views refresh")
	}

	sm := m.(searchModel)
	if sm.installing || !sm.installDone {
		t.Fatalf("after the stream: installing=%v done=%v, want false/true", sm.installing, sm.installDone)
	}
	if !sm.detailInstalled {
		t.Error("package should be marked installed after a successful install")
	}
	if !strings.Contains(sm.installLog, "==> Installing wget") {
		t.Errorf("log should contain the streamed lines:\n%s", sm.installLog)
	}

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.(searchModel).showing {
		t.Fatal("esc should close the panel after install completes")
	}
}

// esc during an install cancels the context bound to the brew command.
func TestSearchInstallAbortCancelsContext(t *testing.T) {
	f := &fakeBrew{
		searchHits:  []string{"wget"},
		infoFormula: &brew.Formula{Name: "wget", Versions: brew.Versions{Stable: "1.21"}},
	}
	m := openedSearchDetail(t, f)

	m, cmd := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m, _ = m.Update(cmd()) // startStream calls Install(ctx) -> searchInstallStartedMsg
	if f.installCtx == nil {
		t.Fatal("Install should have been called with a context")
	}
	if f.installCtx.Err() != nil {
		t.Fatal("context should be live before abort")
	}

	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc}) // abort
	if f.installCtx.Err() == nil {
		t.Fatal("esc should cancel the in-flight install context")
	}
}

// An already-installed package offers no install: the hint is hidden and i is a
// no-op, so the user can't kick off a redundant brew install.
func TestSearchInstallHiddenWhenInstalled(t *testing.T) {
	f := &fakeBrew{
		searchHits: []string{"wget"},
		infoFormula: &brew.Formula{
			Name: "wget", Versions: brew.Versions{Stable: "1.21"},
			Installed: []brew.InstalledKeg{{Version: "1.21"}},
		},
	}
	m := openedSearchDetail(t, f)
	sm := m.(searchModel)
	if !sm.detailInstalled {
		t.Fatal("installed fixture should set detailInstalled")
	}
	if strings.Contains(sm.View(), "i install") {
		t.Errorf("install hint must be hidden for an installed package:\n%s", sm.View())
	}

	m, cmd := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	if m.(searchModel).installing || cmd != nil {
		t.Fatal("i must do nothing for an already-installed package")
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

// A failed search must not wall off the query line: the input stays visible and
// editable so the user can fix the term and search again.
func TestSearchErrorKeepsInputUsable(t *testing.T) {
	m := newSizedSearch(&fakeBrew{err: errors.New("No formulae or casks found")})
	m = typeQuery(m, "file manager")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	sm := m.(searchModel)
	m, _ = m.Update(sm.search(sm.term, sm.section)()) // -> error
	sm = m.(searchModel)
	if sm.state != stateError {
		t.Fatalf("state = %d, want error", sm.state)
	}

	view := sm.View()
	if !strings.Contains(view, "file manager") {
		t.Errorf("error view must keep the query input visible:\n%s", view)
	}
	if !strings.Contains(view, "No formulae or casks found") {
		t.Errorf("error view should show the error below the input:\n%s", view)
	}

	// '/' re-focuses the input; editing + enter re-runs the search.
	m, _ = m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if !m.(searchModel).capturingInput() {
		t.Fatal("/ should re-focus the query input in the error state")
	}
	m = typeQuery(m, "x")
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.(searchModel).state != stateLoading {
		t.Fatal("editing the query and pressing enter should re-run the search")
	}
	if cmd == nil {
		t.Fatal("resubmitting should issue a search command")
	}
}

// From the error state, switching kind re-runs the same term against the other
// kind — the natural recovery when a formula search should have been a cask one.
func TestSearchErrorKindSwitchRetries(t *testing.T) {
	m := newSizedSearch(&fakeBrew{err: errors.New("not found")})
	m = typeQuery(m, "firefox")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	sm := m.(searchModel)
	m, _ = m.Update(sm.search(sm.term, sm.section)()) // -> error
	if m.(searchModel).state != stateError {
		t.Fatal("expected error state")
	}

	m, cmd := m.Update(tea.KeyPressMsg{Code: ']', Text: "]"}) // next kind
	sm = m.(searchModel)
	if sm.section != searchCasks {
		t.Fatalf("] should switch to casks, got section %d", sm.section)
	}
	if sm.state != stateLoading || cmd == nil {
		t.Fatal("switching kind from the error state should re-run the search")
	}
}

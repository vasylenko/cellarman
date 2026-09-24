package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/vasylenko/cellarman/internal/brew"
)

// sizedBrowse returns a browse view sized w×h with its primary load (formulae +
// casks) resolved; taps are left loading.
func sizedBrowse(b Brew, w, h int) child {
	m := newBrowseView(b)
	m, _ = m.Update(contentSizeMsg{width: w, height: h})
	m, _ = m.Update(m.(browseModel).loadInstalled()())
	return m
}

// loadedBrowse returns a browse view sized and fully populated with sampleBrew
// data, feeding both concurrent load results back as Init would.
func loadedBrowse(t *testing.T) child {
	t.Helper()
	m := sizedBrowse(sampleBrew(), 100, 20)
	m, _ = m.Update(m.(browseModel).loadTaps()())
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
	m := sizedBrowse(sampleBrew(), 100, 20) // primary lands first
	bm := m.(browseModel)
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
	m := sizedBrowse(sampleBrew(), 100, 20) // primary loaded, taps still pending

	m, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"}) // -> taps section
	bm := m.(browseModel)
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
	m := sizedBrowse(sampleBrew(), 100, 20)                   // primary OK
	m, _ = m.Update(browseTapsErrMsg{errors.New("tap boom")}) // taps fail
	bm := m.(browseModel)
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

func TestBrowseShowsDescriptions(t *testing.T) {
	m := loadedBrowse(t)
	if v := m.View(); !strings.Contains(v, "Go programming language") {
		t.Errorf("formulae should show their description:\n%s", v)
	}
	// The sample cask ships no desc, so its human-readable name stands in.
	m, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"}) // -> casks
	if v := m.View(); !strings.Contains(v, "sbarex QLMarkdown") {
		t.Errorf("a cask without a desc should fall back to its name:\n%s", v)
	}
}

func TestCaskDesc(t *testing.T) {
	tests := []struct {
		name string
		cask brew.Cask
		want string
	}{
		{"desc wins", brew.Cask{Token: "iterm2", Names: []string{"iTerm2"}, Desc: "Terminal emulator"}, "Terminal emulator"},
		{"falls back to the human name", brew.Cask{Token: "iterm2", Names: []string{"iTerm2"}}, "iTerm2"},
		{"never repeats the token", brew.Cask{Token: "iterm2"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := caskDesc(tt.cask); got != tt.want {
				t.Errorf("caskDesc = %q, want %q", got, tt.want)
			}
		})
	}
}

// Regression: column widths once ignored the table's per-cell padding, so rows
// overran the pane and the viewport clipped the rightmost column (the outdated
// flag was invisible at 80 columns). Every section must fit the pane.
func TestBrowseTableFitsWidth(t *testing.T) {
	for _, w := range []int{minTableWidth, 80, 120, 200} {
		t.Run(fmt.Sprint(w), func(t *testing.T) {
			m := sizedBrowse(sampleBrew(), w, 20)
			m, _ = m.Update(m.(browseModel).loadTaps()())
			if view := m.(browseModel).table.View(); !strings.Contains(view, "↑") {
				t.Errorf("outdated flag should be visible:\n%s", view)
			}
			for range browseSections {
				bm := m.(browseModel)
				if got := lipgloss.Width(bm.table.View()); got > w {
					t.Errorf("%s table is %d cells wide, pane is %d:\n%s", bm.section.label(), got, w, bm.table.View())
				}
				m, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"}) // next section
			}
		})
	}
}

// Description fills the pane, so a resize must re-fit the columns.
func TestBrowseResizeRefitsColumns(t *testing.T) {
	m := loadedBrowse(t) // 100 wide
	before := m.(browseModel).table.Columns()[3].Width
	m, _ = m.Update(contentSizeMsg{width: 200, height: 20})
	if after := m.(browseModel).table.Columns()[3].Width; after != before+100 {
		t.Errorf("description width = %d after widening by 100, want %d", after, before+100)
	}
}

func TestBrowseResizeKeepsSelectionVisible(t *testing.T) {
	b := &fakeBrew{}
	for _, name := range seqNames("pkg", 50) {
		b.formulae = append(b.formulae, brew.Formula{Name: name})
	}
	assertResizeKeepsSelectionVisible(t, sizedBrowse(b, 100, 20))
}

// A section switch starts the new list at its first row, even when the previous
// section was scrolled far down.
func TestBrowseSectionSwitchStartsAtTop(t *testing.T) {
	b := &fakeBrew{}
	for _, name := range seqNames("pkg", 50) {
		b.formulae = append(b.formulae, brew.Formula{Name: name})
	}
	for _, token := range seqNames("cask", 50) {
		b.casks = append(b.casks, brew.Cask{Token: token})
	}
	m := sizedBrowse(b, 100, 20)
	for range 30 {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"}) // -> casks
	if got := m.(browseModel).table.Cursor(); got != 0 {
		t.Errorf("cursor = %d on a newly shown section, want 0", got)
	}
	if !strings.Contains(m.View(), "cask00") {
		t.Errorf("first row should be visible on a newly shown section:\n%s", m.View())
	}
}

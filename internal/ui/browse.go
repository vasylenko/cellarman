package ui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/vasylenko/cellarman/internal/brew"
)

// browseSection is the kind of installed thing currently listed.
type browseSection int

const (
	sectionFormulae browseSection = iota
	sectionCasks
	sectionTaps
)

var browseSections = []browseSection{sectionFormulae, sectionCasks, sectionTaps}

func (s browseSection) label() string {
	switch s {
	case sectionFormulae:
		return "Formulae"
	case sectionCasks:
		return "Casks"
	case sectionTaps:
		return "Taps"
	}
	return ""
}

// browseKeys are this view's local bindings.
var browseKeys = struct {
	NextSection, PrevSection, Open, Back, Retry key.Binding
}{
	NextSection: key.NewBinding(key.WithKeys("right", "l")),
	PrevSection: key.NewBinding(key.WithKeys("left", "h")),
	Open:        key.NewBinding(key.WithKeys("enter")),
	Back:        key.NewBinding(key.WithKeys("esc")),
	Retry:       key.NewBinding(key.WithKeys("r")),
}

type browseModel struct {
	brew    Brew
	state   loadState
	err     error
	spinner spinner.Model
	table   table.Model
	detail  viewport.Model
	section browseSection
	showing bool // detail panel open over the list
	width   int
	height  int

	formulae []brew.Formula
	casks    []brew.Cask
	taps     []brew.Tap
}

// browseDataMsg / browseErrMsg carry the result of the one-shot load.
type browseDataMsg struct {
	formulae []brew.Formula
	casks    []brew.Cask
	taps     []brew.Tap
}
type browseErrMsg struct{ err error }

func newBrowseView(b Brew) child {
	return browseModel{
		brew:    b,
		state:   stateLoading,
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot)),
		table:   table.New(table.WithFocused(true)),
		detail:  viewport.New(),
	}
}

func (m browseModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.load())
}

// load fetches installed packages and taps in the background; brew reads local
// metadata so a single combined call is cheap.
func (m browseModel) load() tea.Cmd {
	b := m.brew
	return func() tea.Msg {
		formulae, casks, err := b.Installed(context.Background())
		if err != nil {
			return browseErrMsg{err}
		}
		taps, err := b.Taps(context.Background())
		if err != nil {
			return browseErrMsg{err}
		}
		return browseDataMsg{formulae, casks, taps}
	}
}

func (m browseModel) Update(msg tea.Msg) (child, tea.Cmd) {
	switch msg := msg.(type) {
	case contentSizeMsg:
		m.width, m.height = msg.width, msg.height
		m.layout()
		return m, nil

	case browseDataMsg:
		m.formulae, m.casks, m.taps = msg.formulae, msg.casks, msg.taps
		m.state = stateLoaded
		m.refreshTable()
		return m, nil

	case browseErrMsg:
		m.err, m.state = msg.err, stateError
		return m, nil

	case packagesChangedMsg:
		// Installed set changed elsewhere (e.g. an upgrade). Refresh in the
		// background, keeping current data on screen until the new data lands.
		if m.state == stateLoaded {
			return m, m.load()
		}
		return m, nil

	case spinner.TickMsg:
		if m.state != stateLoading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m browseModel) handleKey(msg tea.KeyPressMsg) (child, tea.Cmd) {
	switch m.state {
	case stateError:
		if key.Matches(msg, browseKeys.Retry) {
			m.state = stateLoading
			return m, tea.Batch(m.spinner.Tick, m.load())
		}
		return m, nil
	case stateLoading:
		return m, nil
	}

	if m.showing {
		if key.Matches(msg, browseKeys.Back) {
			m.showing = false
			return m, nil
		}
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		return m, cmd
	}

	switch {
	case key.Matches(msg, browseKeys.Retry):
		m.state = stateLoading
		return m, tea.Batch(m.spinner.Tick, m.load())
	case key.Matches(msg, browseKeys.NextSection):
		m.section = browseSection((int(m.section) + 1) % len(browseSections))
		m.refreshTable()
		return m, nil
	case key.Matches(msg, browseKeys.PrevSection):
		m.section = browseSection((int(m.section) + len(browseSections) - 1) % len(browseSections))
		m.refreshTable()
		return m, nil
	case key.Matches(msg, browseKeys.Open):
		m.openDetail()
		return m, nil
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m browseModel) View() string {
	switch m.state {
	case stateLoading:
		return fmt.Sprintf("%s Loading installed packages…", m.spinner.View())
	case stateError:
		return errorStyle.Render("Error: "+m.err.Error()) + "\n\n" + hintStyle.Render("press r to retry")
	}

	if m.showing {
		return m.detail.View() + "\n" + hintStyle.Render("esc back · ↑/↓ scroll")
	}

	hint := hintStyle.Render("←/→ section · ↑/↓ navigate · enter details · r refresh")
	return m.sectionHeader() + "\n" + m.table.View() + "\n" + hint
}

// sectionHeader renders the Formulae | Casks | Taps selector with counts.
func (m browseModel) sectionHeader() string {
	counts := map[browseSection]int{
		sectionFormulae: len(m.formulae),
		sectionCasks:    len(m.casks),
		sectionTaps:     len(m.taps),
	}
	parts := make([]string, 0, len(browseSections))
	for _, s := range browseSections {
		style := segmentStyle
		if s == m.section {
			style = activeSegmentStyle
		}
		parts = append(parts, style.Render(fmt.Sprintf("%s (%d)", s.label(), counts[s])))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// layout sizes the table and detail viewport to the content area, reserving two
// lines for the section header and the hint line.
func (m *browseModel) layout() {
	bodyHeight := m.height - 2
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	m.table.SetWidth(m.width)
	m.table.SetHeight(bodyHeight)
	m.detail.SetWidth(m.width)
	m.detail.SetHeight(bodyHeight)
}

// refreshTable rebuilds columns and rows for the current section. Column widths
// scale with the available width so the table fills the pane without overflow.
func (m *browseModel) refreshTable() {
	w := m.width
	if w < 40 {
		w = 40
	}
	switch m.section {
	case sectionFormulae:
		name, ver, tap := frac(w, 0.34), frac(w, 0.28), frac(w, 0.30)
		m.table.SetColumns([]table.Column{
			{Title: "Formula", Width: name},
			{Title: "Version", Width: ver},
			{Title: "Tap", Width: tap},
			{Title: "", Width: 3},
		})
		rows := make([]table.Row, 0, len(m.formulae))
		for _, f := range m.formulae {
			rows = append(rows, table.Row{f.Name, f.InstalledVersion(), f.Tap, outdatedFlag(f.Outdated)})
		}
		m.table.SetRows(rows)
	case sectionCasks:
		tok, ver, name := frac(w, 0.30), frac(w, 0.24), frac(w, 0.38)
		m.table.SetColumns([]table.Column{
			{Title: "Cask", Width: tok},
			{Title: "Version", Width: ver},
			{Title: "Name", Width: name},
			{Title: "", Width: 3},
		})
		rows := make([]table.Row, 0, len(m.casks))
		for _, c := range m.casks {
			rows = append(rows, table.Row{c.Token, c.Installed, c.DisplayName(), outdatedFlag(c.Outdated)})
		}
		m.table.SetRows(rows)
	case sectionTaps:
		name, f, c := frac(w, 0.46), frac(w, 0.18), frac(w, 0.18)
		m.table.SetColumns([]table.Column{
			{Title: "Tap", Width: name},
			{Title: "Formulae", Width: f},
			{Title: "Casks", Width: c},
			{Title: "Official", Width: 9},
		})
		rows := make([]table.Row, 0, len(m.taps))
		for _, t := range m.taps {
			rows = append(rows, table.Row{t.Name, fmt.Sprint(t.FormulaCount()), fmt.Sprint(t.CaskCount()), yesNo(t.Official)})
		}
		m.table.SetRows(rows)
	}
	m.table.SetCursor(0)
	m.layout()
}

// openDetail renders the selected item's detail (already in memory from the
// initial load) into the viewport; no extra brew call is needed.
func (m *browseModel) openDetail() {
	row := m.table.SelectedRow()
	if len(row) == 0 {
		return
	}
	key := row[0]
	var content string
	switch m.section {
	case sectionFormulae:
		if f := findFormula(m.formulae, key); f != nil {
			content = renderFormula(*f)
		}
	case sectionCasks:
		if c := findCask(m.casks, key); c != nil {
			content = renderCask(*c)
		}
	case sectionTaps:
		if t := findTap(m.taps, key); t != nil {
			content = renderTap(*t)
		}
	}
	if content == "" {
		return
	}
	m.detail.SetContent(content)
	m.detail.GotoTop()
	m.showing = true
}

// --- detail rendering ---

func renderFormula(f brew.Formula) string {
	var b strings.Builder
	title := f.Name
	if f.Deprecated {
		title += "  " + warnStyle.Render("(deprecated)")
	}
	fmt.Fprintln(&b, headingStyle.Render(title))
	fmt.Fprintln(&b, f.Desc)
	fmt.Fprintln(&b)
	field(&b, "Installed", f.InstalledVersion())
	field(&b, "Stable", f.Versions.Stable)
	if f.Outdated {
		field(&b, "Status", outdatedStyle.Render("outdated"))
	} else if f.IsInstalled() {
		field(&b, "Status", okStyle.Render("up to date"))
	}
	field(&b, "Tap", f.Tap)
	field(&b, "License", f.License)
	field(&b, "Homepage", f.Homepage)
	if f.Pinned {
		field(&b, "Pinned", "yes")
	}
	if len(f.Dependencies) > 0 {
		field(&b, "Dependencies", strings.Join(f.Dependencies, ", "))
	}
	if f.Caveats != "" {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, labelStyle.Render("Caveats"))
		fmt.Fprintln(&b, f.Caveats)
	}
	return b.String()
}

func renderCask(c brew.Cask) string {
	var b strings.Builder
	title := c.DisplayName()
	if c.Deprecated {
		title += "  " + warnStyle.Render("(deprecated)")
	}
	fmt.Fprintln(&b, headingStyle.Render(title))
	fmt.Fprintln(&b, c.Desc)
	fmt.Fprintln(&b)
	field(&b, "Token", c.Token)
	field(&b, "Installed", c.Installed)
	field(&b, "Latest", c.Version)
	if c.Outdated {
		field(&b, "Status", outdatedStyle.Render("outdated"))
	} else if c.IsInstalled() {
		field(&b, "Status", okStyle.Render("up to date"))
	}
	field(&b, "Tap", c.Tap)
	field(&b, "Homepage", c.Homepage)
	field(&b, "Auto-updates", yesNo(c.AutoUpdates))
	if c.Caveats != "" {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, labelStyle.Render("Caveats"))
		fmt.Fprintln(&b, c.Caveats)
	}
	return b.String()
}

func renderTap(t brew.Tap) string {
	var b strings.Builder
	fmt.Fprintln(&b, headingStyle.Render(t.Name))
	fmt.Fprintln(&b)
	field(&b, "Official", yesNo(t.Official))
	field(&b, "Formulae", fmt.Sprint(t.FormulaCount()))
	field(&b, "Casks", fmt.Sprint(t.CaskCount()))
	field(&b, "Remote", t.Remote)
	return b.String()
}

func field(b *strings.Builder, label, value string) {
	if value == "" {
		return
	}
	fmt.Fprintf(b, "%s %s\n", labelStyle.Render(label+":"), value)
}

// --- lookups & small helpers ---

func findFormula(fs []brew.Formula, name string) *brew.Formula {
	for i := range fs {
		if fs[i].Name == name {
			return &fs[i]
		}
	}
	return nil
}

func findCask(cs []brew.Cask, token string) *brew.Cask {
	for i := range cs {
		if cs[i].Token == token {
			return &cs[i]
		}
	}
	return nil
}

func findTap(ts []brew.Tap, name string) *brew.Tap {
	for i := range ts {
		if ts[i].Name == name {
			return &ts[i]
		}
	}
	return nil
}

func outdatedFlag(outdated bool) string {
	if outdated {
		return outdatedStyle.Render("↑")
	}
	return ""
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// frac returns a fraction of total as an int column width (min 4).
func frac(total int, f float64) int {
	w := int(float64(total) * f)
	if w < 4 {
		return 4
	}
	return w
}

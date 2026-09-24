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
	state   loadState // formulae + casks: the primary content
	err     error
	spinner spinner.Model
	table   table.Model
	detail  viewport.Model
	section browseSection
	showing bool // detail panel open over the list
	width   int
	height  int

	// taps load on a separate, slower track (brew tap-info is ~3x the cost of
	// info --installed), so the Taps section has its own lifecycle.
	tapsState loadState
	tapsErr   error

	formulae []brew.Formula
	casks    []brew.Cask
	taps     []brew.Tap
}

// The load splits into two messages so the slow taps fetch never blocks the
// first paint. browseInstalledMsg is the primary content (formulae + casks);
// browseTapsMsg fills the Taps section in the background once it lands.
type browseInstalledMsg struct {
	formulae []brew.Formula
	casks    []brew.Cask
}
type browseTapsMsg struct{ taps []brew.Tap }
type browseErrMsg struct{ err error }
type browseTapsErrMsg struct{ err error }

func newBrowseView(b Brew) child {
	return browseModel{
		brew:      b,
		state:     stateLoading,
		tapsState: stateLoading,
		spinner:   spinner.New(spinner.WithSpinner(spinner.Dot)),
		table:     table.New(table.WithFocused(true)),
		detail:    viewport.New(),
	}
}

func (m browseModel) Init() tea.Cmd {
	// Batch runs the two loads concurrently; each lands as its own message, so
	// Formulae paints the moment Installed returns without waiting on tap-info.
	return tea.Batch(m.spinner.Tick, m.loadInstalled(), m.loadTaps())
}

// loadInstalled fetches installed formulae and casks — the primary content.
// brew reads local metadata, so the single call is the fast path to first paint.
func (m browseModel) loadInstalled() tea.Cmd {
	b := m.brew
	return func() tea.Msg {
		formulae, casks, err := b.Installed(context.Background())
		if err != nil {
			return browseErrMsg{err}
		}
		return browseInstalledMsg{formulae, casks}
	}
}

// loadTaps fetches installed taps. `brew tap-info` is far slower than the rest
// (per-tap git scanning), so it runs on its own track and fills the Taps
// section after the primary content is already interactive.
func (m browseModel) loadTaps() tea.Cmd {
	b := m.brew
	return func() tea.Msg {
		taps, err := b.Taps(context.Background())
		if err != nil {
			return browseTapsErrMsg{err}
		}
		return browseTapsMsg{taps}
	}
}

// reload re-runs both tracks from scratch, resetting their lifecycles. Used by
// the manual refresh and the error retry, where the user wants everything —
// including taps, which an external `brew tap` may have changed — re-fetched.
func (m *browseModel) reload() tea.Cmd {
	m.state, m.tapsState = stateLoading, stateLoading
	return tea.Batch(m.spinner.Tick, m.loadInstalled(), m.loadTaps())
}

func (m browseModel) Update(msg tea.Msg) (child, tea.Cmd) {
	switch msg := msg.(type) {
	case contentSizeMsg:
		m.width, m.height = msg.width, msg.height
		m.refitColumns() // Description fills the width, so columns re-fit too
		return m, nil

	case browseInstalledMsg:
		m.formulae, m.casks = msg.formulae, msg.casks
		m.state = stateLoaded
		m.refreshTable()
		return m, nil

	case browseTapsMsg:
		m.taps, m.tapsState, m.tapsErr = msg.taps, stateLoaded, nil
		// Only the Taps section reads taps; refresh just that live view. Other
		// sections pick the data up when the user switches to Taps.
		if m.section == sectionTaps {
			m.refreshTable()
		}
		return m, nil

	case browseErrMsg:
		m.err, m.state = msg.err, stateError
		return m, nil

	case browseTapsErrMsg:
		m.tapsErr, m.tapsState = msg.err, stateError
		return m, nil

	case packagesChangedMsg:
		// Installed set changed elsewhere (e.g. an upgrade). Package operations
		// never change the tap set, so refresh only the installed list — not the
		// expensive tap-info call. Keep current data on screen until it lands.
		if m.state == stateLoaded {
			return m, m.loadInstalled()
		}
		return m, nil

	case spinner.TickMsg:
		// Keep the spinner alive while either track is still loading — the taps
		// spinner spins on the Taps section after the primary content paints.
		if m.state != stateLoading && m.tapsState != stateLoading {
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
			cmd := m.reload()
			return m, cmd
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
		cmd := m.reload()
		return m, cmd
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
	body := m.table.View()
	// Taps load behind the primary content; reflect their own lifecycle when the
	// user is on that section before tap-info has returned.
	if m.section == sectionTaps {
		switch m.tapsState {
		case stateLoading:
			body = fmt.Sprintf("%s Loading taps…", m.spinner.View())
		case stateError:
			body = errorStyle.Render("Error loading taps: "+m.tapsErr.Error()) + "\n\n" + hintStyle.Render("press r to retry")
		}
	}
	return m.sectionHeader() + "\n" + body + "\n" + hint
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
		label := fmt.Sprintf("%s (%d)", s.label(), counts[s])
		if s == sectionTaps && m.tapsState == stateLoading {
			label = s.label() + " (…)" // count unknown until tap-info returns
		}
		parts = append(parts, style.Render(label))
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

// refreshTable rebuilds the current section's rows and columns and puts the
// cursor back on the first row; it runs on every load and section switch.
func (m *browseModel) refreshTable() {
	rows := m.tableRows()
	m.table.SetColumns(m.tableColumns(rows))
	m.table.SetRows(rows)
	m.table.SetCursor(0)
	m.layout()
}

// refitColumns re-sizes the columns to a new pane width. Unlike refreshTable it
// keeps the rows, so the cursor and scroll position survive a resize.
func (m *browseModel) refitColumns() {
	m.table.SetColumns(m.tableColumns(m.table.Rows()))
	m.layout()
}

// tableRows builds the current section's rows from the loaded data.
func (m browseModel) tableRows() []table.Row {
	var rows []table.Row
	switch m.section {
	case sectionFormulae:
		rows = make([]table.Row, 0, len(m.formulae))
		for _, f := range m.formulae {
			rows = append(rows, table.Row{f.Name, f.InstalledVersion(), outdatedFlag(f.Outdated), f.Desc})
		}
	case sectionCasks:
		rows = make([]table.Row, 0, len(m.casks))
		for _, c := range m.casks {
			rows = append(rows, table.Row{c.Token, c.Installed, outdatedFlag(c.Outdated), caskDesc(c)})
		}
	case sectionTaps:
		rows = make([]table.Row, 0, len(m.taps))
		for _, t := range m.taps {
			rows = append(rows, table.Row{t.Name, fmt.Sprint(t.FormulaCount()), fmt.Sprint(t.CaskCount()), yesNo(t.Official)})
		}
	}
	return rows
}

// tableColumns sizes the current section's columns to the pane width.
func (m browseModel) tableColumns(rows []table.Row) []table.Column {
	w := tableWidth(m.width)
	switch m.section {
	case sectionFormulae:
		return packageColumns("Formula", rows, w)
	case sectionCasks:
		return packageColumns("Cask", rows, w)
	case sectionTaps:
		f, c, official := frac(w, 0.18), frac(w, 0.18), 9
		return []table.Column{
			{Title: "Tap", Width: fillCol(w, f, c, official)},
			{Title: "Formulae", Width: f},
			{Title: "Casks", Width: c},
			{Title: "Official", Width: official},
		}
	}
	return nil
}

// packageColumns lays out the name | version | flag | description rows shared
// by the Formulae and Casks sections: name and version fit their content and
// Description takes the rest, so wide terminals show descriptions in full. The
// flag sits beside the version it qualifies, not at the pane's far edge.
func packageColumns(nameTitle string, rows []table.Row, w int) []table.Column {
	name := fitCol(nameTitle, rows, 0, frac(w, 0.30))
	ver := fitCol("Version", rows, 1, frac(w, 0.20))
	return []table.Column{
		{Title: nameTitle, Width: name},
		{Title: "Version", Width: ver},
		{Title: "", Width: flagColWidth},
		{Title: "Description", Width: fillCol(w, name, ver, flagColWidth)},
	}
}

// caskDesc is a cask's Description cell. Not every cask ships a desc; its
// human-readable name is then the best hint at what the token is.
func caskDesc(c brew.Cask) string {
	if c.Desc != "" {
		return c.Desc
	}
	if name := c.DisplayName(); name != c.Token {
		return name
	}
	return ""
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

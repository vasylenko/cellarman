package ui

import (
	"context"
	"fmt"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/serhii-vasylenko/brew-tui/internal/brew"
)

// searchSection is the package kind searched against; results from one kind are
// never mixed with the other so detail hydration knows which Info to call.
type searchSection int

const (
	searchFormulae searchSection = iota
	searchCasks
)

var searchSections = []searchSection{searchFormulae, searchCasks}

func (s searchSection) label() string {
	switch s {
	case searchFormulae:
		return "Formulae"
	case searchCasks:
		return "Casks"
	}
	return ""
}

// kind maps the segment to the brew.Kind passed to Search/Info.
func (s searchSection) kind() brew.Kind {
	if s == searchCasks {
		return brew.KindCask
	}
	return brew.KindFormula
}

// searchKeys are this view's local bindings. Tab is intentionally absent — it is
// a global view switch owned by Root.
var searchKeys = struct {
	NextSection, PrevSection, Edit, Open, Back, Retry key.Binding
}{
	NextSection: key.NewBinding(key.WithKeys("right", "]")),
	PrevSection: key.NewBinding(key.WithKeys("left", "[")),
	Edit:        key.NewBinding(key.WithKeys("/")),
	Open:        key.NewBinding(key.WithKeys("enter")),
	Back:        key.NewBinding(key.WithKeys("esc")),
	Retry:       key.NewBinding(key.WithKeys("r")),
}

type searchModel struct {
	brew    Brew
	state   loadState
	err     error
	spinner spinner.Model
	input   textinput.Model
	table   table.Model
	detail  viewport.Model
	section searchSection
	showing bool // detail panel open over the results
	queried bool // a search has run at least once
	term    string
	width   int
	height  int

	results []string
}

// searchResultsMsg / searchErrMsg carry the result of a one-shot Search; both are
// namespaced so Root's broadcast never crosses with other views' data messages.
type searchResultsMsg struct{ names []string }
type searchErrMsg struct{ err error }

// searchDetailMsg carries hydrated Info for the selected result, fetched off the
// event loop so a slow (possibly network-bound) `brew info` never freezes the UI.
type searchDetailMsg struct {
	formula *brew.Formula
	cask    *brew.Cask
}

func newSearchView(b Brew) child {
	ti := textinput.New()
	ti.Placeholder = "search packages"
	ti.Focus() // start focused so the user types immediately on first visit
	return searchModel{
		brew:    b,
		state:   stateLoaded, // idle until the first query runs
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot)),
		input:   ti,
		table:   table.New(table.WithFocused(true)),
		detail:  viewport.New(),
	}
}

func (m searchModel) Init() tea.Cmd {
	return m.spinner.Tick
}

// capturingInput tells Root to yield single-letter shortcuts while the query
// field has focus, so q/? are typed rather than triggering global commands.
func (m searchModel) capturingInput() bool {
	return m.input.Focused()
}

// search runs a Search for term against the current kind. evalAll is forced true
// so results include third-party (non-official) installed taps.
func (m searchModel) search(term string, section searchSection) tea.Cmd {
	b := m.brew
	return func() tea.Msg {
		names, err := b.Search(context.Background(), term, section.kind(), true)
		if err != nil {
			return searchErrMsg{err}
		}
		return searchResultsMsg{names}
	}
}

func (m searchModel) Update(msg tea.Msg) (child, tea.Cmd) {
	switch msg := msg.(type) {
	case contentSizeMsg:
		m.width, m.height = msg.width, msg.height
		m.layout()
		return m, nil

	case searchResultsMsg:
		m.results = msg.names
		m.state, m.queried = stateLoaded, true
		m.refreshTable()
		return m, nil

	case searchErrMsg:
		m.err, m.state = msg.err, stateError
		return m, nil

	case searchDetailMsg:
		m.state = stateLoaded
		var content string
		switch {
		case msg.formula != nil:
			content = renderFormula(*msg.formula)
		case msg.cask != nil:
			content = renderCask(*msg.cask)
		}
		if content != "" {
			m.detail.SetContent(content)
			m.detail.GotoTop()
			m.showing = true
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

func (m searchModel) handleKey(msg tea.KeyPressMsg) (child, tea.Cmd) {
	// Typing in the query field: Enter submits, everything else edits.
	if m.input.Focused() {
		if key.Matches(msg, searchKeys.Open) {
			term := m.input.Value()
			if term == "" {
				return m, nil
			}
			m.term, m.state = term, stateLoading
			m.input.Blur() // hand the keyboard to results/retry while the search runs
			return m, tea.Batch(m.spinner.Tick, m.search(term, m.section))
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	if m.state == stateError {
		if key.Matches(msg, searchKeys.Retry) {
			m.state = stateLoading
			return m, tea.Batch(m.spinner.Tick, m.search(m.term, m.section))
		}
		return m, nil
	}
	if m.state == stateLoading {
		return m, nil
	}

	if m.showing {
		if key.Matches(msg, searchKeys.Back) {
			m.showing = false
			return m, nil
		}
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		return m, cmd
	}

	switch {
	case key.Matches(msg, searchKeys.Edit), key.Matches(msg, searchKeys.Back):
		m.input.Focus()
		return m, nil
	case key.Matches(msg, searchKeys.NextSection):
		return m.switchSection(1)
	case key.Matches(msg, searchKeys.PrevSection):
		return m.switchSection(-1)
	case key.Matches(msg, searchKeys.Open):
		return m.openDetail()
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// switchSection flips the Formulae/Casks segment and re-runs the search when a
// query exists, so the same term is evaluated against the other kind.
func (m searchModel) switchSection(delta int) (child, tea.Cmd) {
	m.section = searchSection((int(m.section) + delta + len(searchSections)) % len(searchSections))
	if m.term == "" {
		return m, nil
	}
	m.state = stateLoading
	return m, tea.Batch(m.spinner.Tick, m.search(m.term, m.section))
}

// openDetail hydrates the highlighted result via Info (search returns names only)
// in the background, then renders it into the viewport. The fetch runs in a
// tea.Cmd — never inline — because `brew info` for a non-installed package can hit
// the Homebrew API and would otherwise freeze the whole UI.
func (m searchModel) openDetail() (child, tea.Cmd) {
	row := m.table.SelectedRow()
	if len(row) == 0 {
		return m, nil
	}
	m.state = stateLoading
	return m, tea.Batch(m.spinner.Tick, m.detailCmd(row[0], m.section.kind()))
}

// detailCmd fetches full info for a result off the event loop.
func (m searchModel) detailCmd(name string, kind brew.Kind) tea.Cmd {
	b := m.brew
	return func() tea.Msg {
		f, c, err := b.Info(context.Background(), name, kind)
		if err != nil {
			return searchErrMsg{err}
		}
		return searchDetailMsg{formula: f, cask: c}
	}
}

func (m searchModel) View() string {
	switch m.state {
	case stateLoading:
		return fmt.Sprintf("%s Searching…", m.spinner.View())
	case stateError:
		return errorStyle.Render("Error: "+m.err.Error()) + "\n\n" + hintStyle.Render("press r to retry")
	}

	if m.showing {
		return m.detail.View() + "\n" + hintStyle.Render("esc back · ↑/↓ scroll")
	}

	hint := hintStyle.Render("/ edit · ←/→ kind · ↑/↓ navigate · enter details")
	return lipgloss.JoinVertical(lipgloss.Left,
		m.input.View(),
		m.sectionHeader(),
		m.resultsBody(),
		hint,
	)
}

// resultsBody renders the results area: an idle hint before the first search, a
// muted "no matches" for an empty result set, otherwise the table.
func (m searchModel) resultsBody() string {
	if !m.queried {
		return hintStyle.Render("type a query, press enter")
	}
	if len(m.results) == 0 {
		return mutedStyle.Render("no matches")
	}
	return m.table.View()
}

// sectionHeader renders the Formulae | Casks segment selector.
func (m searchModel) sectionHeader() string {
	parts := make([]string, 0, len(searchSections))
	for _, s := range searchSections {
		style := segmentStyle
		if s == m.section {
			style = activeSegmentStyle
		}
		parts = append(parts, style.Render(s.label()))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// layout sizes the results table and detail viewport, reserving lines for the
// query input, the segment header, and the hint line.
func (m *searchModel) layout() {
	bodyHeight := m.height - 3
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	m.table.SetWidth(m.width)
	m.table.SetHeight(bodyHeight)
	m.detail.SetWidth(m.width)
	detailHeight := m.height - 1
	if detailHeight < 1 {
		detailHeight = 1
	}
	m.detail.SetHeight(detailHeight)
}

// refreshTable rebuilds the single Name column from the latest results.
func (m *searchModel) refreshTable() {
	w := m.width
	if w < 40 {
		w = 40
	}
	m.table.SetColumns([]table.Column{
		{Title: m.section.label(), Width: w},
	})
	rows := make([]table.Row, 0, len(m.results))
	for _, name := range m.results {
		rows = append(rows, table.Row{name})
	}
	m.table.SetRows(rows)
	m.table.SetCursor(0)
	m.layout()
}

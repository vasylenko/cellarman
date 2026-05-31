package ui

import (
	"context"
	"fmt"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/vasylenko/cellarman/internal/brew"
)

// upgradeKeys are this view's local bindings.
var upgradeKeys = struct {
	Toggle, Run, All, Refresh, Abort key.Binding
}{
	Toggle:  key.NewBinding(key.WithKeys("space", "x")),
	Run:     key.NewBinding(key.WithKeys("enter")),
	All:     key.NewBinding(key.WithKeys("U")),
	Refresh: key.NewBinding(key.WithKeys("r")),
	Abort:   key.NewBinding(key.WithKeys("esc")),
}

// outdatedRow pairs an outdated package with its kind and selection state so the
// table can mark and upgrade rows without re-deriving which list each came from.
type outdatedRow struct {
	pkg      brew.OutdatedPackage
	kind     brew.Kind
	selected bool
}

type upgradeModel struct {
	brew    Brew
	state   loadState
	err     error
	spinner spinner.Model
	table   table.Model
	log     viewport.Model
	width   int
	height  int

	rows      []outdatedRow
	upgrading bool
	done      bool
	notice    string // transient guidance, e.g. enter pressed with nothing selected
	logBuf    string
	ch        <-chan brew.Event
	cancel    context.CancelFunc
}

// upgradeDataMsg / upgradeErrMsg carry the result of the one-shot Outdated load.
type upgradeDataMsg struct{ report *brew.OutdatedReport }
type upgradeErrMsg struct{ err error }

// Streaming msgs are namespaced to this view so Root's broadcast can't bleed one
// view's stream into another's buffer.
type upgradeStartedMsg struct{ ch <-chan brew.Event }
type upgradeLineMsg string
type upgradeDoneMsg struct{ err error }

func newUpgradeView(b Brew) child {
	return upgradeModel{
		brew:    b,
		state:   stateLoading,
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot)),
		table:   table.New(table.WithFocused(true)),
		log:     viewport.New(),
	}
}

func (m upgradeModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.load())
}

// load fetches the outdated report in the background; brew reads local metadata
// so the call is cheap.
func (m upgradeModel) load() tea.Cmd {
	b := m.brew
	return func() tea.Msg {
		report, err := b.Outdated(context.Background())
		if err != nil {
			return upgradeErrMsg{err}
		}
		return upgradeDataMsg{report}
	}
}

func (m upgradeModel) Update(msg tea.Msg) (child, tea.Cmd) {
	switch msg := msg.(type) {
	case contentSizeMsg:
		m.width, m.height = msg.width, msg.height
		m.layout()
		return m, nil

	case upgradeDataMsg:
		m.setRows(msg.report)
		m.state = stateLoaded
		m.done = false // fresh outdated list supersedes any completed-upgrade log
		m.refreshTable()
		return m, nil

	case upgradeErrMsg:
		// Reset the in-flight flag (and any cancel) so the error view's retry is
		// reachable — handleKey's `if m.upgrading` guard would otherwise swallow it.
		if m.cancel != nil {
			m.cancel()
			m.cancel = nil
		}
		m.upgrading = false
		m.err, m.state = msg.err, stateError
		return m, nil

	case packagesChangedMsg:
		// A mutation finished (here or in Diagnose); refresh the outdated list,
		// but only when idle on it — never mid-upgrade, before the first visit
		// (preserving lazy-load), or over an error screen. finishUpgrade leaves
		// state==stateLoaded with upgrading=false, so its own refresh still fires.
		if m.state == stateLoaded && !m.upgrading {
			return m, m.load()
		}
		return m, nil

	case upgradeStartedMsg:
		m.ch = msg.ch
		return m, m.next()

	case upgradeLineMsg:
		m.appendLog(string(msg))
		// Re-issue: a Cmd yields one msg, so the stream freezes after line one
		// unless we ask for the next event right here.
		return m, m.next()

	case upgradeDoneMsg:
		return m.finishUpgrade(msg.err)

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

// next pulls the upgrade stream's next event into a view-specific message.
func (m upgradeModel) next() tea.Cmd {
	return waitForEvent(m.ch,
		func(s string) tea.Msg { return upgradeLineMsg(s) },
		func(e error) tea.Msg { return upgradeDoneMsg{e} })
}

func (m upgradeModel) handleKey(msg tea.KeyPressMsg) (child, tea.Cmd) {
	if m.upgrading {
		if key.Matches(msg, upgradeKeys.Abort) && m.cancel != nil {
			m.cancel()
		}
		return m, nil
	}

	switch m.state {
	case stateError:
		if key.Matches(msg, upgradeKeys.Refresh) {
			m.state = stateLoading
			return m, tea.Batch(m.spinner.Tick, m.load())
		}
		return m, nil
	case stateLoading:
		return m, nil
	}

	m.notice = "" // any key clears a stale notice
	switch {
	case key.Matches(msg, upgradeKeys.Toggle):
		m.toggleSelection()
		return m, nil
	case key.Matches(msg, upgradeKeys.Refresh):
		m.state = stateLoading
		m.done = false
		return m, tea.Batch(m.spinner.Tick, m.load())
	case key.Matches(msg, upgradeKeys.All):
		return m.startUpgrade(nil) // explicit: upgrade everything outdated
	case key.Matches(msg, upgradeKeys.Run):
		names := m.selectedNames()
		if len(names) == 0 {
			// Never silently upgrade everything on an empty selection.
			m.notice = "Nothing selected — space to select, or U to upgrade all"
			return m, nil
		}
		return m.startUpgrade(names)
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// startUpgrade streams `brew upgrade` for the given names (empty = everything
// outdated). The context is cancellable so esc can abort mid-stream.
func (m upgradeModel) startUpgrade(names []string) (child, tea.Cmd) {
	if len(m.rows) == 0 {
		return m, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.upgrading = true
	m.done = false
	m.notice = ""
	m.logBuf = ""
	m.log.SetContent("")
	m.log.GotoTop()

	b := m.brew
	cmd := startStream(ctx,
		func(c context.Context) (<-chan brew.Event, error) { return b.Upgrade(c, names...) },
		func(ch <-chan brew.Event) tea.Msg { return upgradeStartedMsg{ch} },
		func(e error) tea.Msg { return upgradeErrMsg{e} })
	return m, cmd
}

// finishUpgrade tears down the stream, records the outcome, and reloads the
// outdated list so the table reflects what actually upgraded.
func (m upgradeModel) finishUpgrade(err error) (child, tea.Cmd) {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.upgrading = false
	m.done = true
	m.ch = nil
	if err != nil {
		m.appendLog(errorStyle.Render("✗ " + err.Error()))
		return m, nil // leave the failure on screen; r refreshes
	}
	m.appendLog(okStyle.Render("✓ done"))
	// Broadcast so this and other package views (Browse) refresh after the upgrade.
	return m, packagesChanged
}

// selectedNames returns the marked package names, or nil to upgrade everything.
func (m upgradeModel) selectedNames() []string {
	var names []string
	for _, r := range m.rows {
		if r.selected {
			names = append(names, r.pkg.Name)
		}
	}
	return names
}

// toggleSelection flips the marker on the highlighted row.
func (m *upgradeModel) toggleSelection() {
	i := m.table.Cursor()
	if i < 0 || i >= len(m.rows) {
		return
	}
	m.rows[i].selected = !m.rows[i].selected
	m.refreshTable()
}

// setRows flattens the report into one selectable list, formulae before casks.
func (m *upgradeModel) setRows(report *brew.OutdatedReport) {
	m.rows = nil
	if report == nil {
		return
	}
	for _, p := range report.Formulae {
		m.rows = append(m.rows, outdatedRow{pkg: p, kind: brew.KindFormula})
	}
	for _, p := range report.Casks {
		m.rows = append(m.rows, outdatedRow{pkg: p, kind: brew.KindCask})
	}
}

// appendLog grows the streaming buffer and pins the viewport to the latest line.
func (m *upgradeModel) appendLog(line string) {
	if m.logBuf == "" {
		m.logBuf = line
	} else {
		m.logBuf += "\n" + line
	}
	m.log.SetContent(m.logBuf)
	m.log.GotoBottom()
}

func (m upgradeModel) View() string {
	switch m.state {
	case stateLoading:
		return fmt.Sprintf("%s Checking for updates…", m.spinner.View())
	case stateError:
		return errorStyle.Render("Error: "+m.err.Error()) + "\n\n" + hintStyle.Render("press r to retry")
	}

	if m.upgrading {
		return m.log.View() + "\n" + hintStyle.Render("esc abort")
	}
	if m.done {
		return m.log.View() + "\n" + hintStyle.Render("r refresh")
	}

	if len(m.rows) == 0 {
		return okStyle.Render("Everything is up to date") + "\n\n" + hintStyle.Render("r refresh")
	}

	keys := "space select · enter upgrade selected · U upgrade all · r refresh"
	if m.notice != "" {
		keys = m.notice
	}
	line := hintStyle.Render(keys)
	if n := len(m.selectedNames()); n > 0 {
		line = labelStyle.Render(fmt.Sprintf("%d selected", n)) + "  " + line
	}
	return m.table.View() + "\n" + line
}

// layout sizes the table and log to the content area, reserving one line for the
// hint.
func (m *upgradeModel) layout() {
	bodyHeight := m.height - 1
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	m.table.SetWidth(m.width)
	m.table.SetHeight(bodyHeight)
	m.log.SetWidth(m.width)
	m.log.SetHeight(bodyHeight)
}

// refreshTable rebuilds rows from the current selection state, restoring the
// cursor so toggling a marker doesn't jump the highlight.
func (m *upgradeModel) refreshTable() {
	w := m.width
	if w < 40 {
		w = 40
	}
	name, change, kind := frac(w, 0.30), frac(w, 0.44), frac(w, 0.14)
	m.table.SetColumns([]table.Column{
		{Title: "", Width: 3},
		{Title: "Package", Width: name},
		{Title: "Change", Width: change},
		{Title: "Kind", Width: kind},
	})
	rows := make([]table.Row, 0, len(m.rows))
	for _, r := range m.rows {
		change := fmt.Sprintf("%s → %s", r.pkg.InstalledVersion(), r.pkg.CurrentVersion)
		rows = append(rows, table.Row{selectionMark(r.selected), r.pkg.Name, change, r.kind.String()})
	}
	cursor := m.table.Cursor()
	m.table.SetRows(rows)
	if cursor >= 0 && cursor < len(rows) {
		m.table.SetCursor(cursor)
	}
	m.layout()
}

func selectionMark(selected bool) string {
	if selected {
		return labelStyle.Render("✓")
	}
	return " "
}

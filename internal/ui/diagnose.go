package ui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/vasylenko/cellarman/internal/brew"
)

// diagnoseKeys are this view's local bindings. The two fixes (cleanup,
// autoremove) are the only ones offered — both safe and non-destructive.
var diagnoseKeys = struct {
	Recheck, Cleanup, Autoremove key.Binding
}{
	Recheck:    key.NewBinding(key.WithKeys("r")),
	Cleanup:    key.NewBinding(key.WithKeys("c")),
	Autoremove: key.NewBinding(key.WithKeys("a")),
}

type diagnoseModel struct {
	brew    Brew
	state   loadState
	err     error
	spinner spinner.Model
	report  viewport.Model
	width   int
	height  int

	running bool              // a fix is streaming; doctor re-runs once it finishes
	fixName string            // label of the fix in flight, for the spinner line
	ch      <-chan brew.Event // live fix stream, kept so each line can re-issue waitForEvent
	log     string            // accumulated streaming output of the running fix
}

// diagnoseReportMsg / diagnoseErrMsg carry the result of the one-shot doctor run.
type diagnoseReportMsg struct{ report *brew.DoctorReport }
type diagnoseErrMsg struct{ err error }

// Streaming-fix messages, namespaced so Root's broadcast never bleeds another
// view's stream into this viewport.
type diagnoseStartedMsg struct{ ch <-chan brew.Event }
type diagnoseLineMsg string
type diagnoseDoneMsg struct{ err error }

func newDiagnoseView(b Brew) child {
	return diagnoseModel{
		brew:    b,
		state:   stateLoading,
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot)),
		report:  viewport.New(),
	}
}

func (m diagnoseModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.load())
}

// load runs `brew doctor` in the background and feeds the parsed report back.
func (m diagnoseModel) load() tea.Cmd {
	b := m.brew
	return func() tea.Msg {
		report, err := b.Doctor(context.Background())
		if err != nil {
			return diagnoseErrMsg{err}
		}
		return diagnoseReportMsg{report}
	}
}

func (m diagnoseModel) Update(msg tea.Msg) (child, tea.Cmd) {
	switch msg := msg.(type) {
	case contentSizeMsg:
		m.width, m.height = msg.width, msg.height
		m.layout()
		return m, nil

	case diagnoseReportMsg:
		m.state = stateLoaded
		m.running = false
		m.log = "" // fresh report supersedes any prior fix log
		m.report.SetContent(renderReport(msg.report))
		m.report.GotoTop()
		return m, nil

	case diagnoseErrMsg:
		m.err, m.state = msg.err, stateError
		m.running = false
		return m, nil

	case diagnoseStartedMsg:
		// Keep the channel so each subsequent line can re-issue waitForEvent.
		m.ch = msg.ch
		return m, waitForEvent(m.ch, diagnoseOnLine, diagnoseOnDone)

	case diagnoseLineMsg:
		m.appendLog(string(msg))
		// Re-issue: a Cmd yields one event, so the stream stalls without this.
		return m, waitForEvent(m.ch, diagnoseOnLine, diagnoseOnDone)

	case diagnoseDoneMsg:
		m.ch = nil
		if msg.err != nil {
			m.err, m.state = msg.err, stateError
			m.running = false
			return m, nil
		}
		// Fix finished cleanly: re-run doctor so the report reflects the change,
		// and broadcast so views showing package data (Browse) refresh too.
		m.state = stateLoading
		return m, tea.Batch(m.spinner.Tick, m.load(), packagesChanged)

	case spinner.TickMsg:
		if m.state != stateLoading && !m.running {
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

func (m diagnoseModel) handleKey(msg tea.KeyPressMsg) (child, tea.Cmd) {
	// While loading or a fix streams, only let the viewport scroll its content.
	if m.state == stateLoading || m.running {
		var cmd tea.Cmd
		m.report, cmd = m.report.Update(msg)
		return m, cmd
	}

	// Recheck re-runs doctor and doubles as the error-state recovery key.
	if key.Matches(msg, diagnoseKeys.Recheck) {
		m.state = stateLoading
		return m, tea.Batch(m.spinner.Tick, m.load())
	}

	// In the error state only Recheck is honored — a fix must not run against a
	// doctor that just failed (mirrors browse/upgrade/search error handling).
	if m.state == stateError {
		return m, nil
	}

	switch {
	case key.Matches(msg, diagnoseKeys.Cleanup):
		return m.startFix("Cleaning up", m.brew.Cleanup)
	case key.Matches(msg, diagnoseKeys.Autoremove):
		return m.startFix("Removing unused dependencies", m.brew.Autoremove)
	}

	var cmd tea.Cmd
	m.report, cmd = m.report.Update(msg)
	return m, cmd
}

// startFix kicks off a safe maintenance command and shows its live output in the
// viewport; the report is re-run once the stream completes.
func (m diagnoseModel) startFix(label string, starter streamStarter) (child, tea.Cmd) {
	m.running = true
	m.fixName = label
	m.log = ""
	m.report.SetContent("")
	return m, tea.Batch(
		m.spinner.Tick,
		startStream(context.Background(), starter, diagnoseOnStart, diagnoseOnErr),
	)
}

func (m diagnoseModel) View() string {
	switch m.state {
	case stateLoading:
		return fmt.Sprintf("%s Running brew doctor…", m.spinner.View())
	case stateError:
		return errorStyle.Render("Error: "+m.err.Error()) + "\n\n" + hintStyle.Render("press r to retry")
	}

	if m.running {
		header := fmt.Sprintf("%s %s…", m.spinner.View(), m.fixName)
		return header + "\n" + m.report.View() + "\n" + hintStyle.Render("↑/↓ scroll")
	}

	hint := hintStyle.Render("c cleanup · a autoremove · r re-check · ↑/↓ scroll")
	return m.report.View() + "\n" + hint
}

// layout sizes the report viewport, reserving two lines for the spinner/header
// and the hint line (matching browse.layout's reservation).
func (m *diagnoseModel) layout() {
	bodyHeight := m.height - 2
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	m.report.SetWidth(m.width)
	m.report.SetHeight(bodyHeight)
}

// appendLog adds a streamed line and keeps the viewport pinned to the latest
// output so the user follows the fix as it runs.
func (m *diagnoseModel) appendLog(line string) {
	m.log += line + "\n"
	m.report.SetContent(m.log)
	m.report.GotoBottom()
}

// renderReport builds the viewport body from a doctor run: the healthy banner
// or the warning list.
func renderReport(report *brew.DoctorReport) string {
	if report == nil || report.OK {
		return okStyle.Render("✓ Your system is healthy")
	}

	var b strings.Builder
	fmt.Fprintln(&b, warnStyle.Render(fmt.Sprintf("%d warning(s)", len(report.Warnings))))
	for _, w := range report.Warnings {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, warnStyle.Render("⚠ "+w.Title))
		for _, d := range w.Details {
			trimmed := strings.TrimSpace(d)
			if trimmed == "" {
				continue
			}
			// Lines brew indented are the affected items the warning is about —
			// highlight them as a bullet list; flush-left prose stays muted.
			if d != trimmed {
				fmt.Fprintln(&b, "    "+labelStyle.Render("• "+trimmed))
			} else {
				fmt.Fprintln(&b, "  "+mutedStyle.Render(trimmed))
			}
		}
	}
	return b.String()
}

// diagnoseOnStart/OnLine/OnDone/OnErr adapt stream.go's generic callbacks to
// this view's namespaced message types.
func diagnoseOnStart(ch <-chan brew.Event) tea.Msg { return diagnoseStartedMsg{ch} }
func diagnoseOnLine(line string) tea.Msg           { return diagnoseLineMsg(line) }
func diagnoseOnDone(err error) tea.Msg             { return diagnoseDoneMsg{err} }
func diagnoseOnErr(err error) tea.Msg              { return diagnoseErrMsg{err} }

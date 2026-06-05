package ui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type viewID int

const (
	browseView viewID = iota
	searchView
	upgradeView
	diagnoseView
)

// viewOrder fixes the tab order and is the iteration set for broadcasts.
var viewOrder = []viewID{browseView, searchView, upgradeView, diagnoseView}

func (v viewID) title() string {
	switch v {
	case browseView:
		return "Browse"
	case searchView:
		return "Search"
	case upgradeView:
		return "Upgrade"
	case diagnoseView:
		return "Diagnose"
	}
	return ""
}

// child is a screen managed by Root. It mirrors tea.Model but returns a body
// string (Root composes the final tea.View with shared chrome) and the child
// interface type so Root stores it back without a type assertion.
type child interface {
	Init() tea.Cmd
	Update(tea.Msg) (child, tea.Cmd)
	View() string
}

// inputCapturer is implemented by views with a focused text field. When the
// active view is capturing input, Root yields its single-letter shortcuts (q,
// ?) so the user can type them instead of triggering global commands.
type inputCapturer interface {
	capturingInput() bool
}

// Root owns the view set, shared chrome (tabs, help bar), and global dispatch.
type Root struct {
	views  map[viewID]child
	loaded map[viewID]bool
	active viewID
	keys   globalKeys
	help   help.Model
	width  int
	height int
}

// NewRoot wires every view to the same brew client. Browse is marked loaded
// because Init kicks it off immediately; the rest load lazily on first visit.
func NewRoot(b Brew) Root {
	return Root{
		views: map[viewID]child{
			browseView:   newBrowseView(b),
			searchView:   newSearchView(b),
			upgradeView:  newUpgradeView(b),
			diagnoseView: newDiagnoseView(b),
		},
		loaded: map[viewID]bool{browseView: true},
		active: browseView,
		keys:   defaultGlobalKeys(),
		help:   help.New(),
	}
}

func (r Root) Init() tea.Cmd {
	return r.views[browseView].Init()
}

func (r Root) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		r.width, r.height = msg.Width, msg.Height
		return r.broadcast(r.sizeMsg())

	case tea.KeyPressMsg:
		if key.Matches(msg, r.keys.ForceQuit) {
			return r, tea.Quit
		}
		capturing := r.activeCaptures()
		switch {
		case key.Matches(msg, r.keys.NextView):
			return r.switchTo(step(r.active, 1))
		case key.Matches(msg, r.keys.PrevView):
			return r.switchTo(step(r.active, -1))
		case !capturing && key.Matches(msg, r.keys.Quit):
			return r, tea.Quit
		case !capturing && key.Matches(msg, r.keys.Help):
			r.help.ShowAll = !r.help.ShowAll
			// Toggling full help changes the footer height, so re-size views.
			return r.broadcast(r.sizeMsg())
		}
		// Not a global key: route to the active view only, so typing never
		// leaks into background views.
		nv, cmd := r.views[r.active].Update(msg)
		r.views[r.active] = nv
		return r, cmd

	default:
		// Data, ticks, and stream messages go to every view: an async result
		// must reach its originating view even if the user has switched away.
		// Views ignore message types they don't own, so this is side-effect free.
		return r.broadcast(msg)
	}
}

func (r Root) View() tea.View {
	body := lipgloss.JoinVertical(lipgloss.Left,
		r.renderTabs(),
		r.views[r.active].View(),
		r.footer(),
	)
	v := tea.NewView(body)
	v.AltScreen = true
	// No mouse reporting: the terminal keeps native drag-to-select/copy. Nothing
	// here reads mouse events, and navigation is keyboard-driven anyway.
	return v
}

// broadcast updates every view with msg and batches their commands. The views
// map is shared across Root value copies, so the writes persist.
func (r Root) broadcast(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := make([]tea.Cmd, 0, len(viewOrder))
	for _, id := range viewOrder {
		nv, cmd := r.views[id].Update(msg)
		r.views[id] = nv
		cmds = append(cmds, cmd)
	}
	return r, tea.Batch(cmds...)
}

// switchTo activates a view, sizing and loading it on first visit.
func (r Root) switchTo(id viewID) (tea.Model, tea.Cmd) {
	r.active = id
	if r.loaded[id] {
		return r, nil
	}
	r.loaded[id] = true
	r.views[id], _ = r.views[id].Update(r.sizeMsg())
	return r, r.views[id].Init()
}

func (r Root) activeCaptures() bool {
	c, ok := r.views[r.active].(inputCapturer)
	return ok && c.capturingInput()
}

// sizeMsg is the content area available to a view: full width, height minus the
// tab bar and help bar.
func (r Root) sizeMsg() contentSizeMsg {
	h := r.height - lipgloss.Height(r.renderTabs()) - lipgloss.Height(r.footer())
	if h < 1 {
		h = 1
	}
	return contentSizeMsg{width: r.width, height: h}
}

func (r Root) footer() string {
	return helpBarStyle.Render(r.help.View(r.keys))
}

func (r Root) renderTabs() string {
	tabs := make([]string, 0, len(viewOrder))
	for _, id := range viewOrder {
		style := tabStyle
		if id == r.active {
			style = activeTabStyle
		}
		tabs = append(tabs, style.Render(id.title()))
	}
	return tabBarStyle.Render(lipgloss.JoinHorizontal(lipgloss.Top, tabs...))
}

// step moves delta positions around the view ring (wraps both directions).
func step(v viewID, delta int) viewID {
	n := len(viewOrder)
	return viewID((int(v) + delta + n) % n)
}

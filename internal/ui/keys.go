package ui

import "charm.land/bubbles/v2/key"

// globalKeys are handled by Root regardless of the active view. View-local keys
// (navigation, select, etc.) are owned by each view. q and ? yield to views
// that are capturing text input — see Root.activeCaptures.
type globalKeys struct {
	NextView  key.Binding
	PrevView  key.Binding
	Help      key.Binding
	Quit      key.Binding
	ForceQuit key.Binding
}

func defaultGlobalKeys() globalKeys {
	return globalKeys{
		NextView:  key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next view")),
		PrevView:  key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev view")),
		Help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:      key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		ForceQuit: key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	}
}

// ShortHelp / FullHelp satisfy help.KeyMap so the help bar renders these.
func (k globalKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.NextView, k.PrevView, k.Help, k.Quit}
}

func (k globalKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.NextView, k.PrevView}, {k.Help, k.Quit}}
}

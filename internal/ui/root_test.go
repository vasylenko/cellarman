package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/vasylenko/cellarman/internal/brew"
)

func sizedRoot(t *testing.T) Root {
	t.Helper()
	r := NewRoot(fullBrew())
	rm, _ := r.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return rm.(Root)
}

// Non-key messages broadcast to every view, so an async result still reaches its
// originating view after the user has tabbed away — the reason broadcast exists.
func TestRootBroadcastsDataToBackgroundView(t *testing.T) {
	r := sizedRoot(t)

	rm, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // browse -> search
	r = rm.(Root)
	if r.active != searchView {
		t.Fatalf("active = %d, want search", r.active)
	}

	// Browse is now backgrounded; its load result must still land on it.
	rm, _ = r.Update(browseInstalledMsg{formulae: []brew.Formula{{Name: "go"}}})
	r = rm.(Root)
	if r.views[browseView].(browseModel).state != stateLoaded {
		t.Fatal("broadcast should deliver browseInstalledMsg to the backgrounded browse view")
	}
}

// KeyPressMsg routes to the active view only — typing must never leak into a
// backgrounded view (here, search's focused input).
func TestRootRoutesKeysToActiveViewOnly(t *testing.T) {
	r := sizedRoot(t)

	// Resolve browse's primary load so it's interactive.
	rm, _ := r.Update(r.views[browseView].(browseModel).loadInstalled()())
	r = rm.(Root)

	rm, _ = r.Update(tea.KeyPressMsg{Code: 'l', Text: "l"}) // browse: next section
	r = rm.(Root)
	if r.views[browseView].(browseModel).section != sectionCasks {
		t.Fatal("the active browse view should have handled the section key")
	}
	if v := r.views[searchView].(searchModel).input.Value(); v != "" {
		t.Errorf("key leaked into the backgrounded search input: %q", v)
	}
}

func TestRootGlobalQuit(t *testing.T) {
	r := sizedRoot(t)
	_, cmd := r.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("q should issue a quit command from a non-input view")
	}
}

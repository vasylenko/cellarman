// Command brew-tui is a terminal UI for Homebrew: browse installed packages,
// search, upgrade, and run diagnostics.
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/vasylenko/brew-tui/internal/brew"
	"github.com/vasylenko/brew-tui/internal/ui"
)

func main() {
	if _, err := tea.NewProgram(ui.NewRoot(brew.New())).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "brew-tui:", err)
		os.Exit(1)
	}
}

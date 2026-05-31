// Command cellarman is a terminal UI for Homebrew: browse installed packages,
// search, upgrade, and run diagnostics.
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/vasylenko/cellarman/internal/brew"
	"github.com/vasylenko/cellarman/internal/ui"
)

func main() {
	if _, err := tea.NewProgram(ui.NewRoot(brew.New())).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "cellarman:", err)
		os.Exit(1)
	}
}

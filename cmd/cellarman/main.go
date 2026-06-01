// Command cellarman is a terminal UI for Homebrew: browse installed packages,
// search, upgrade, and run diagnostics.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/vasylenko/cellarman/internal/brew"
	"github.com/vasylenko/cellarman/internal/ui"
)

// version is injected at build time via -ldflags "-X main.version=...". The
// Homebrew formula sets it from the release tag; a plain `go build` leaves it "dev".
var version = "dev"

func main() {
	// Resolve --version before starting Bubble Tea: the program takes over the
	// alt-screen, so handling it inside the TUI would never print or return.
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("cellarman", version)
		return
	}

	if _, err := tea.NewProgram(ui.NewRoot(brew.New())).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "cellarman:", err)
		os.Exit(1)
	}
}

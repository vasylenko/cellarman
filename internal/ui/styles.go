package ui

import "charm.land/lipgloss/v2"

// Palette. ANSI 256 indices so colors render consistently across terminals
// without depending on truecolor support.
var (
	colorAccent = lipgloss.Color("39")  // blue — selection, labels, active tab
	colorSubtle = lipgloss.Color("240") // dim — inactive chrome
	colorFaint  = lipgloss.Color("245") // hints
	colorGood   = lipgloss.Color("42")  // green — healthy/up to date
	colorWarn   = lipgloss.Color("214") // orange — outdated
	colorBad    = lipgloss.Color("203") // red — errors
)

var (
	tabBarStyle    = lipgloss.NewStyle().Padding(0, 0, 1, 1)
	tabStyle       = lipgloss.NewStyle().Padding(0, 2).Foreground(colorSubtle)
	activeTabStyle = lipgloss.NewStyle().Padding(0, 2).Foreground(colorAccent).Bold(true).Underline(true)

	helpBarStyle = lipgloss.NewStyle().Padding(0, 1)
	hintStyle    = lipgloss.NewStyle().Foreground(colorFaint)
	mutedStyle   = lipgloss.NewStyle().Foreground(colorSubtle)

	labelStyle   = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	headingStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	errorStyle   = lipgloss.NewStyle().Foreground(colorBad).Bold(true)

	outdatedStyle = lipgloss.NewStyle().Foreground(colorWarn)
	okStyle       = lipgloss.NewStyle().Foreground(colorGood)
	warnStyle     = lipgloss.NewStyle().Foreground(colorWarn)

	// segment selector inside a view (e.g. Formulae | Casks | Taps)
	segmentStyle       = lipgloss.NewStyle().Padding(0, 1).Foreground(colorSubtle)
	activeSegmentStyle = lipgloss.NewStyle().Padding(0, 1).Foreground(colorAccent).Bold(true)
)

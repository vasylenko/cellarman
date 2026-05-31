package ui

// contentSizeMsg tells a view how much room it has below the tab bar and above
// the help bar. Root computes it and broadcasts it to every view so inactive
// views are correctly sized the moment they become active.
type contentSizeMsg struct {
	width  int
	height int
}

// loadState is the lifecycle every data-backed view moves through.
type loadState int

const (
	stateLoading loadState = iota
	stateLoaded
	stateError
)

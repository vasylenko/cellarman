package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/vasylenko/cellarman/internal/brew"
)

// streamStarter kicks off a long-running brew command (Upgrade, Cleanup, …).
type streamStarter func(context.Context) (<-chan brew.Event, error)

// startStream runs starter and routes the result back into the model: the live
// channel via onStart, or a startup failure via onErr. Callers then drive the
// channel with waitForEvent.
func startStream(
	ctx context.Context,
	starter streamStarter,
	onStart func(<-chan brew.Event) tea.Msg,
	onErr func(error) tea.Msg,
) tea.Cmd {
	return func() tea.Msg {
		ch, err := starter(ctx)
		if err != nil {
			return onErr(err)
		}
		return onStart(ch)
	}
}

// waitForEvent reads ONE event from a started stream and maps it to a
// view-specific message. A tea.Cmd yields exactly one message, so a view must
// re-issue this from its Update on every line to keep the stream flowing —
// forgetting to re-issue freezes output after the first line.
//
// Messages are view-specific (onLine/onDone build the view's own types) so that
// Root broadcasting a line to every view never lets one view's stream bleed
// into another's buffer.
func waitForEvent(
	ch <-chan brew.Event,
	onLine func(string) tea.Msg,
	onDone func(error) tea.Msg,
) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return onDone(nil)
		}
		if ev.Done {
			return onDone(ev.Err)
		}
		return onLine(ev.Line)
	}
}

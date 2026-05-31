# CLAUDE.md

README covers purpose, keys, architecture, testing, and limitations — read it for the obvious stuff. This file is only the non-obvious traps.

## Charm v2 stack (imports + API)

Two Charm namespaces coexist on purpose — don't "unify" them:
- **v2 libs** (bubbletea, bubbles, lipgloss) import from `charm.land/...` — see `go.mod`. The README's link to `github.com/charmbracelet/bubbletea` is a human-facing project page, **not** the import path. A v1 prior will reach for `github.com/charmbracelet/bubbletea` and break the build.
- **Charm `x/...` packages** (teatest, plus indirect ansi/term/etc.) still live on `github.com/charmbracelet/x/...`.

This is **Bubble Tea v2**, whose API differs from the v1 examples a model recalls. When writing UI code:
- Key handling is `tea.KeyPressMsg` (not `tea.KeyMsg`); tests build keys as `tea.KeyPressMsg{Code: ...}`.
- `View()` returns a `tea.View` struct via `tea.NewView(body)` — alt-screen and mouse are fields (`v.AltScreen`, `v.MouseMode`), not program options. `tea.NewProgram` is called with no options.
- Only `Root.View()` returns `tea.View`; the `child` interface returns a plain `string` body that Root wraps in chrome.

## Adding a view

Four things, spread across files, that the compiler/tests won't catch for a new view:
- Register it in `viewOrder` (`internal/ui/root.go`) — that slice drives both tab order and the broadcast set.
- It loads lazily on first visit via `switchTo`. Add it to `loaded` + `Init` **only** if it must load eagerly (Browse is the only one that does).
- Give its async/streamed messages a **view-unique type**. Root broadcasts every non-key message to all views; a shared `lineMsg` type silently corrupts another view's buffer.
- Streaming view: re-issue `waitForEvent` from `Update` on every line, or output freezes after line one (the `tea.Cmd`-yields-one-message contract in `stream.go`).
- Text-input view: implement `inputCapturer`, or global `q`/`?` will quit/toggle-help instead of typing into the field.

## Adding a brew subcommand

Guard every user-controlled operand with a `--` end-of-options separator before it (see `internal/brew/client.go`) — without it a package name like `-rf` or `--macports` is read as a brew flag. Existing commands are test-pinned; a new one is not.

## Committing

Commits are SSH-signed via 1Password — `git commit` fails cryptically if 1Password is locked. Unlock it first.

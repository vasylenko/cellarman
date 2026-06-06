# CLAUDE.md

README covers purpose, features, keys, and limitations — the user-facing stuff. This file covers architecture, testing, and the non-obvious traps.

## Architecture

- `internal/brew` — typed client over the `brew` CLI. No UI dependency; shells out, parses JSON (text for `doctor`), streams long-running commands over a channel. Unit-tested against fixtures.
- `internal/ui` — Bubble Tea root model plus one self-contained sub-model per view, composed by a root that owns navigation and shared chrome.
- `cmd/cellarman` — entrypoint.

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

## Testing

```sh
make test               # unit + view + e2e (teatest), no real brew
make test-integration   # exercises the real brew binary
```

Unit tests cover brew output parsing and each view's state transitions; teatest drives the full program through every view. The `integration` tag runs the same flows against your real Homebrew.

## Committing

Commits are SSH-signed via 1Password — `git commit` fails cryptically if 1Password is locked. Unlock it first.

## Releasing

`make release VERSION=vX.Y.Z` tags and pushes — that's the whole manual step. The tag-triggered `release` workflow refreshes the formula's `url`+`sha256` from the tag's source tarball (the sha only exists once the tag is published, so the bump is necessarily downstream of the tag), commits it to `main` as `github-actions[bot]`, and cuts the GitHub Release. `git pull` afterwards to pick up the bot's formula commit.

Tags must come from `main` — the workflow commits the bump to `main`, assuming the tag sits on it. `make formula VERSION=vX.Y.Z` runs just the formula step locally for an offline release (commit + push it yourself). If `main` ever gets branch protection requiring reviews or signed commits, the bot push needs a bypass.

# Cellarman

A terminal UI for [Homebrew](https://brew.sh): browse what's installed, search for new packages, upgrade, and run diagnostics — without memorizing `brew` subcommands.

The name fits the trade: a *cellarman* tends the casks in a cellar, and Homebrew keeps everything in its `Cellar`.

Built with [Bubble Tea v2](https://github.com/charmbracelet/bubbletea). It shells out to your `brew` binary and parses its JSON, so it always reflects exactly what Homebrew would do.

## Features

- **Browse** — installed formulae, casks, and taps with full detail (version, deps, license, homepage, caveats). Outdated packages are flagged.
- **Search** — query formulae or casks across official *and* third-party installed taps, with a one-line description per result; drill into any result's details.
- **Upgrade** — see what's outdated, select the ones you want (or upgrade all), and watch the upgrade stream live.
- **Diagnose** — run `brew doctor`, read the warnings, and apply safe fixes (`cleanup`, `autoremove`) with live output.

## Requirements

- Go 1.25+
- `brew` on your `PATH`

## Build & run

```sh
make run            # go run ./cmd/cellarman
make build          # -> bin/cellarman
```

## Keys

| Key | Action |
|---|---|
| `tab` / `shift+tab` | switch view |
| `↑`/`↓` (or `j`/`k`) | navigate lists |
| `enter` | open details / start an action |
| `esc` | back out |
| `←`/`→` | switch section (Browse, Search) |
| `space` / `x` | select a package (Upgrade) |
| `enter` / `U` | upgrade selected / upgrade all (Upgrade) |
| `r` | refresh the list (Browse, Upgrade) |
| `/` | edit the query (Search) |
| `c` / `a` / `r` | cleanup / autoremove / re-check (Diagnose) |
| `?` | toggle help |
| `q` / `ctrl+c` | quit |

## Architecture

- `internal/brew` — typed client over the `brew` CLI. No UI dependency; shells out, parses JSON (text for `doctor`), streams long-running commands over a channel. Unit-tested against fixtures.
- `internal/ui` — Bubble Tea root model plus one self-contained sub-model per view, composed by a root that owns navigation and shared chrome.
- `cmd/cellarman` — entrypoint.

## Testing

```sh
make test               # unit + view + e2e (teatest), no real brew
make test-integration   # exercises the real brew binary
```

Unit tests cover brew output parsing and each view's state transitions; teatest drives the full program through every view. The `integration` tag runs the same flows against your real Homebrew.

## Limitations

- Doctor offers only non-destructive fixes (`cleanup`, `autoremove`). It does not auto-remediate every warning — many need a human decision.
- Quitting does not cancel a running `brew` command — it keeps going in the background until it finishes. Abort an in-progress upgrade with `esc` first if you want to stop it; cleanup/autoremove in Diagnose run to completion.
- Outdated-cask parsing assumes the same `--json=v2` shape as formulae (Homebrew's documented schema).

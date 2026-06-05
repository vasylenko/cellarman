# Cellarman

A terminal UI for [Homebrew](https://brew.sh): browse what's installed, search for new packages, upgrade, and run diagnostics — without memorizing `brew` subcommands.

Built with [Bubble Tea v2](https://github.com/charmbracelet/bubbletea). It shells out to your `brew` binary and parses its JSON, so it always reflects exactly what Homebrew would do.

## Features

- **Browse** — installed formulae, casks, and taps with full detail (version, deps, license, homepage, caveats). Outdated packages are flagged.
- **Search** — query formulae or casks across official *and* third-party installed taps, with a one-line description per result; drill into any result's details and install it from there, watching the install stream live.
- **Upgrade** — see what's outdated, select the ones you want (or upgrade all), and watch the upgrade stream live.
- **Diagnose** — run `brew doctor`, read the warnings, and apply safe fixes (`cleanup`, `autoremove`) with live output.

## Install

```sh
brew tap vasylenko/cellarman https://github.com/vasylenko/cellarman
brew install cellarman
```

The tap URL is needed once per machine — modern Homebrew won't auto-tap a third-party repo on `install`. It builds from source (Homebrew pulls Go as a build-only dependency), so there's no signing/notarization in the way. After that, `brew upgrade cellarman` keeps it current.

## Requirements

- `brew` on your `PATH`
- Go 1.25+ — only to build it yourself; the Homebrew install pulls its own

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
| `i` | install the package shown in details (Search) |
| `space` / `x` | select a package (Upgrade) |
| `enter` / `U` | upgrade selected / upgrade all (Upgrade) |
| `r` | refresh the list (Browse, Upgrade) |
| `/` | edit the query (Search) |
| `c` / `a` / `r` | cleanup / autoremove / re-check (Diagnose) |
| `?` | toggle help |
| `q` / `ctrl+c` | quit |

## Limitations

- Doctor offers only non-destructive fixes (`cleanup`, `autoremove`). It does not auto-remediate every warning — many need a human decision.
- Quitting does not cancel a running `brew` command — it keeps going in the background until it finishes. Abort an in-progress upgrade or install with `esc` first if you want to stop it; cleanup/autoremove in Diagnose run to completion.
- Outdated-cask parsing assumes the same `--json=v2` shape as formulae (Homebrew's documented schema).

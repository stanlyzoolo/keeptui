# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
go build .          # build binary
go install .        # install to ~/go/bin/keys
go run .            # run without installing
go test ./...       # run all tests
go vet ./...        # static analysis
```

Release is triggered by pushing a `v*` tag; GitHub Actions builds for darwin/linux/windows via `.github/workflows/release.yml`.

## Architecture

**`keys`** is a terminal TUI tracker for CLI tools built with Bubble Tea. It is a pure TUI app — there is no CLI; running `keys` launches the interface directly.

### Entry point

`main.go` is a thin launcher: it loads tracker metadata via `loader.LoadMeta()` and starts the Bubble Tea TUI with `model.New(meta)`. There are no subcommands or flags.

### Package overview

| Package | Purpose |
|---|---|
| `internal/loader` | Load tool configs (embedded + user); validate YAML; manage tracker metadata; parse GitHub refs (`NormalizeRepo`, `ParseToolRef` in `github.go`) |
| `internal/model` | Entire Bubble Tea model — all TUI state, key handling, and rendering |
| `internal/ui` | Lip Gloss styles and `PlaceOverlay` helper |
| `internal/version` | Detect installed version (`version_cmd`), fetch latest from GitHub API with 24h cache |

### Data flow

1. `loader.LoadMeta()` reads the tool tracker from `~/.config/keys/meta.yaml`.
2. `model.New(meta)` builds the model from the tracker metadata. (`loader.Load()` still merges built-in configs — embedded via `//go:embed data/tools` — with user configs from `~/.config/keys/tools/<tool>/config.yaml`, user files winning.)
3. On `Init()`, the model fires goroutines to fetch installed/latest versions and repo cards asynchronously; results arrive as messages and update the UI.

### TUI state machine

The model is a three-panel layout with focus cycling via `→/←` between `focusTools` (tool list), `focusBrief` (the central info card), and `focusHelp` (the `--help` / `man` viewport).

- **Central panel actions (`focusBrief`)** operate on the data the card already shows: `o` opens the repo in the browser, `c` opens the changelog/releases page, `s` cycles the status (`loader.NextStatus`), `e` edits the note, `t` edits the tags. `o`/`c` go through `openURLCmd` (resolved per-`GOOS` by `browserCommand`); a tool with no `GitHub` sets `m.statusMsg` instead of launching. `s`/`e`/`t` mutate `m.meta` via `loader.UpsertMeta`, persist with `loader.SaveMeta`, then refresh the card with `m.briefViewport.SetContent(m.renderCard())`.
- **Tracking is managed from `focusTools`**: `t` track (add by GitHub URL or plain name), `u` untrack (with confirmation), `r` rename (fix the binary name when the repo name differs). Each is a mode flag (`tracking`/`confirmingUntrack`/`renaming`) handled by an early branch in `Update()` and a matching branch in `renderStatusBar()`, mirroring the `editingNote`/`editingTags` input pattern. Mutations go through `loader.UpsertMeta`/`RemoveMeta`, persist via `loader.SaveMeta`, then rebuild `m.tools = loader.ToolsFromMeta(m.meta)` and refresh the viewport.
- **Help bar** (`renderStatusBar()`) is per-focus; the `focusBrief` bar shows the action keys `[o] open repo  [c] changelog  [s] status  [e] note  [t] tags  [q] quit`.
- **Search** (`/`) and overlays such as the changelog popup are rendered via `ui.PlaceOverlay`.

### Adding a new built-in tool

Add `internal/loader/data/tools/<toolname>/config.yaml`. Required fields: `name`, at least one of `categories` or `command_groups`. `loader.Load()` validates configs on startup; run `go test ./...` to check before committing.

### File storage

| Data | Location |
|---|---|
| Built-in tool configs | Embedded in binary |
| User tool configs | `~/.config/keys/tools/<tool>/config.yaml` |
| Tracker metadata | `~/.config/keys/meta.yaml` |
| Version cache (24h TTL) | `~/.config/keys/cache.json` |

### GitHub API

`GITHUB_TOKEN` env var increases rate limits when fetching latest versions. The `version` package caches responses in `cache.json`; `FetchAndCache` bypasses the TTL for forced refresh. URL→`owner/repo` normalization lives in `loader.NormalizeRepo`; `version.extractRepo` delegates to it (the `loader` package owns GitHub-ref parsing to avoid an import cycle).

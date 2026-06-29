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

**`keys`** is a terminal TUI hotkey cheatsheet viewer built with Bubble Tea.

### Entry point

`main.go` parses CLI arguments and either dispatches to a subcommand in `internal/cmd/` or launches the Bubble Tea TUI with `model.New(...)`.

### Package overview

| Package | Purpose |
|---|---|
| `internal/loader` | Load tool configs (embedded + user); validate YAML; manage tracker metadata; parse GitHub refs (`NormalizeRepo`, `ParseToolRef` in `github.go`) |
| `internal/model` | Entire Bubble Tea model — all TUI state, key handling, and rendering |
| `internal/ui` | Lip Gloss styles and `PlaceOverlay` helper |
| `internal/version` | Detect installed version (`version_cmd`), fetch latest from GitHub API with 24h cache |
| `internal/tldr` | Fetch and parse tldr-pages for `keys fetch <tool>` |
| `internal/cmd` | One file per CLI subcommand (`check`, `edit`, `fetch`, `import`, `list`, `new`, `note`, `status`, `validate`) |

### Data flow

1. `loader.Load()` merges built-in configs (embedded via `//go:embed data/tools`) with user configs from `~/.config/keys/tools/<tool>/config.yaml` — user files win.
2. `loader.LoadMeta()` reads the tool tracker from `~/.config/keys/meta.yaml`.
3. Both slices are passed to `model.New(tools, meta, opts)`.
4. On `Init()`, the model fires one goroutine per tool to fetch installed/latest versions asynchronously; results arrive as `versionMsg` and update the UI.

### TUI state machine

The model has two top-level views (`viewHotkeys` / `viewMyTools`) toggled by `Tab`. Within `viewHotkeys`:
- Focus alternates between left panel (tool list) and right panel (bindings/commands viewport) via `→/←`.
- The right panel has two tabs: `[Keys]` (categories + bindings) and `[Commands]` (command groups from tldr).
- Overlays: changelog popup (`showChangelog`) and command detail popup (`showPopup`) are rendered via `ui.PlaceOverlay`.
- Search (`/`) filters across all tools and all bindings simultaneously; selection is disabled while searching.
- Tracking is managed from the left tool-list panel (`focusTools`), not the CLI: `t` track (add by GitHub URL or plain name), `u` untrack (with confirmation), `r` rename (fix the binary name when the repo name differs). Each is a mode flag (`tracking`/`confirmingUntrack`/`renaming`) handled by an early branch in `Update()` and a matching branch in `renderStatusBar()`, mirroring the `editingNote`/`editingTags` input pattern. Mutations go through `loader.UpsertMeta`/`RemoveMeta`, persist via `loader.SaveMeta`, then rebuild `m.tools = loader.ToolsFromMeta(m.meta)` and refresh the viewport.

### Adding a new built-in tool

Add `internal/loader/data/tools/<toolname>/config.yaml`. Required fields: `name`, at least one of `categories` or `command_groups`. Run `keys validate <path>` to check before committing.

### File storage

| Data | Location |
|---|---|
| Built-in tool configs | Embedded in binary |
| User tool configs | `~/.config/keys/tools/<tool>/config.yaml` |
| Tracker metadata | `~/.config/keys/meta.yaml` |
| Version cache (24h TTL) | `~/.config/keys/cache.json` |

### GitHub API

`GITHUB_TOKEN` env var increases rate limits when fetching latest versions. The `version` package caches responses in `cache.json`; `FetchAndCache` bypasses the TTL for forced refresh. URL→`owner/repo` normalization lives in `loader.NormalizeRepo`; `version.extractRepo` delegates to it (the `loader` package owns GitHub-ref parsing to avoid an import cycle).

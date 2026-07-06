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

**Async fetch responsibility split**: two paths must stay symmetric. Per tool there are four sources, split so local detection never waits on the network:

- `fetchInstalledCmd(t)` — always fired; runs `version.InstalledVersion` locally (subprocess) and emits `installedMsg{toolName, installed}`. The handler merges `Installed` into `m.versions[toolName]` without clobbering `Latest`, so the installed version renders immediately regardless of network state.
- `fetchRemoteCmd(t)` — fired only when `t.GitHub != ""`; makes a single network pass via `version.GetRepoData` (release + repo info + languages in one shot, no duplicate `fetchRepoInfo`) and emits `remoteMsg{toolName, latest, repoStatus, card, err}`. The handler merges `Latest` into `m.versions[toolName]` and writes `m.repoStatus` / `m.repoCards`.
- changelog (`fetchChangelogCmd`) and `--help` (`fetchHelpCmd`) round out the four.

`Init()` fires `fetchInstalledCmd` + (conditionally) `fetchRemoteCmd` for every tool, plus changelog/help for the selected one. `autoFetchCmdsForSelected()` runs after track/untrack/rename and selection changes; it re-fetches the same sources for the selected tool, guarded by the pure predicates `needsInstalled(t)` / `needsRemote(t)` (skip if already cached; `needsRemote` also requires `t.GitHub != ""` and a missing `Latest` or card). If a tool is added or renamed mid-session, this path populates its card without a restart. Rename also deletes the stale old-name entries from `m.repoCards` / `m.versions` / `m.repoStatus` / `m.changelogData` / `m.helpCache` so the tool re-fetches under its new name.

### TUI state machine

The model is a three-panel layout with focus cycling via `→/←` between `focusTools` (tool list), `focusBrief` (the central info card), and `focusHelp` (the `--help` / `man` viewport). In `focusTools` the list wraps: `j`/`↓` past the last tool jumps to the first and `k`/`↑` past the first jumps to the last (modular `metaSelected`, guarded against an empty list).

- **Central panel actions (`focusBrief`)** operate on the data the card already shows: `o` opens the repo in the browser, `c` opens the changelog/releases page, `r` force-refreshes the tool's data, `s` cycles the status (`loader.NextStatus`), `e` edits the note, `t` edits the tags. `o`/`c` go through `openURLCmd` (resolved per-`GOOS` by `browserCommand`); a tool with no `GitHub` sets `m.statusMsg` instead of launching. `s`/`e`/`t` mutate `m.meta` via `loader.UpsertMeta`, persist with `loader.SaveMeta`, then refresh the card with `m.briefViewport.SetContent(m.renderCard())`.
- **Refresh (`r` in `focusBrief`)**: `refreshSelectedCmd(t)` force-refreshes the selected tool bypassing the 24h cache TTL — the repo pass (`refreshRemoteCmd` → `version.RefreshRepoData`) + changelog (`refreshChangelogCmd` → `version.RefreshChangelog`) + a local installed re-detect (`fetchInstalledCmd`). It emits the same `remoteMsg`/`changelogMsg` as the startup path, so the merge/re-render logic is reused. While the repo pass is in flight `m.refreshingFor` (the tool name) turns the card title into a status line — `refreshing <name> data <spinner>` (`bubbles/spinner`, `MiniDot`; the about is hidden) — with no status-bar takeover; the `remoteMsg` handler clears `refreshingFor` on completion, which reverts the title to name+about and halts the `spinner.TickMsg` loop. `refreshingFor` doubles as the double-press guard; a tool with no `GitHub` only re-detects the installed version (`m.statusMsg = "no repo to refresh"`, no spinner). Note `r` is also rename in `focusTools` — same `case "r"`, branched on focus.
- **Tracking is managed from `focusTools`**: `t` track (add by GitHub URL or plain name), `u` untrack (with confirmation), `r` rename (fix the binary name when the repo name differs). Each is a mode flag (`tracking`/`confirmingUntrack`/`renaming`) handled by an early branch in `Update()` and a matching branch in `renderStatusBar()`, mirroring the `editingNote`/`editingTags` input pattern. Mutations go through `loader.UpsertMeta`/`RemoveMeta`, persist via `loader.SaveMeta`, then rebuild `m.tools = loader.ToolsFromMeta(m.meta)` and refresh the viewport.
- **Help bar** (`renderStatusBar()`) is per-focus; the `focusBrief` bar shows the action keys `[o] open repo  [c] changelog  [r] refresh  [s] status  [e] note  [t] tags  [q] quit`. In the three normal focus states `renderHintsBar` right-aligns a **GitHub API Usage gauge** to the corner (a fixed 12-cell yellow fill showing *used/limit*, e.g. `45/60`, plus `[L] details`). The bar width is constant regardless of the 60 vs 5000 limit; on narrow terminals it downgrades full → compact (`GH 45/60 [L]`) → hidden (`rateGaugeMinGap`). Colors are constant yellow — no rate-pressure recolor (the `⚠`/`✕` alarm lives only in the `[L]` overlay). Input/modal states show no gauge.
- **API-status overlay (`L`)**: opens a read-only view of the GitHub rate limit and token (source, masked value, used/limit with threshold icon, reset time) with token entry/removal/refresh. When no token is configured it leads with an `Add a GitHub token…` nudge (hidden once a token exists or while entering one). It is a modal (`showingAPIStatus`, plus a token-input sub-mode) handled by an early branch in `Update()` and a matching `renderStatusBar()` branch; `L` is guarded to fire only when no other input/modal mode is active. See the GitHub API section for the data flow.
- **Search** (`/`), the changelog popup, and the API-status overlay are rendered via `ui.PlaceOverlay` (a centered fg-over-bg compositor introduced with the rate-limit work).

### Adding a new built-in tool

Add `internal/loader/data/tools/<toolname>/config.yaml`. Required fields: `name`, at least one of `categories` or `command_groups`. `loader.Load()` validates configs on startup; run `go test ./...` to check before committing.

### File storage

| Data | Location |
|---|---|
| Built-in tool configs | Embedded in binary |
| User tool configs | `~/.config/keys/tools/<tool>/config.yaml` |
| Tracker metadata | `~/.config/keys/meta.yaml` |
| Version cache (24h TTL) | `~/.config/keys/cache.json` |
| GitHub token (`0600`) | `~/.config/keys/token` |

### GitHub API

Unauthenticated the REST API allows **60 requests/hour** per IP; with a token it is **5000/hour**. Each tool with a `GitHub` field costs 3 requests (`fetchRelease` + `fetchRepoInfo` + `fetchLanguages` inside `GetRepoData`), so a cold start with many tools and no token can exhaust the quota. A token raises the limit and is resolved with **env precedence**: `GITHUB_TOKEN` env var always wins, otherwise the config-file token (`~/.config/keys/token`, `0600`, loaded lazily once via `sync.Once`).

**Token (`token.go`):** `resolveToken()` returns env token else `tokenMem` (all `tokenMem` access under `tokenMu`, `-race` clean). `SetToken(t)` writes the `0600` file (`MkdirAll` for the dir) and updates memory; `ClearToken()` removes both; `TokenSource()` reports `"env"|"config"|"none"`; `Token()` returns the effective token (env precedence) for the overlay's masked preview. Env source never persists — the config file only holds a TUI-entered token.

**`doGH(req)`** is the single auth point: it sets `Accept` + `Authorization: Bearer <resolveToken()>` (only when non-empty), runs the request with the 5s-timeout client, and calls `updateRateFromHeaders(resp.Header)`. The 3 fetchers build a request then call `doGH` — no more duplicated header/client code or `os.Getenv` copies.

**Hybrid rate read:** response headers (`X-RateLimit-Limit`/`-Remaining`/`-Reset`) update the shared `RateLimit` snapshot (`rlMu`-guarded) in the background for free but only when a request happens; `FetchRate()` hits `GET /rate_limit` (decodes `resources.core`) for on-demand truth **without spending core quota** — used on overlay open, refresh, and startup seeding. `Rate()` returns the snapshot. Warm-cache starts make no request, so `Init()` fires one `fetchRateCmd` to seed the status-bar signal; snapshots with `Known==false` must never overwrite a known `m.rate` (non-clobber merge).

**`ErrRateLimited`** (typed) is returned by `classifyStatus(resp)` for 403/429 **only when the response's own `X-RateLimit-Remaining == 0`** (read from the per-response header, never global `rl`, since concurrent goroutines race on `rl`); a 403 with remaining>0 is a generic `HTTP %d`. `fetchRemoteCmd` maps `errors.Is(err, ErrRateLimited)` + no cached card to a `"rate-limited"` `repoStatus` so the card renders "rate limited — press [L]" instead of a bare error; known tags/cards survive a total failure.

**Token validation before persistence:** `FetchRateWithToken(token)` issues `/rate_limit` with an explicit `Authorization` header **without touching `tokenMem`/the file**; 401 → `ErrTokenInvalid`. `SetToken` runs only after a 200, so an invalid token is never written to disk.

**API-status overlay (`L`):** opens via `ui.PlaceOverlay`, shows an optional add-token nudge (only when `TokenSource()=="none"` and not entering one), token source (masked), a `Used: <used> / <limit>` line (used = `Limit-Remaining`, matching the status-bar gauge) with the shared `rateLowThreshold` icon (none / `⚠` / `✕`), and reset time; `[e]` enters a masked `textinput` to set a token (validated via `FetchRateWithToken`, then `SetToken` + `autoFetchCmdsForSelected()` backfill), `[d]` removes it (config source only), `[r]` refreshes, `[esc]` closes. `L` is guarded — ignored while any input/modal mode is active.

The `version` package caches responses in `cache.json`; `FetchAndCache` bypasses the TTL for forced refresh. URL→`owner/repo` normalization lives in `loader.NormalizeRepo`; `version.extractRepo` delegates to it (the `loader` package owns GitHub-ref parsing to avoid an import cycle).

**Cache writes must go through `updateCacheEntry`**. Every read-modify-write of `cache.json` (`GetLatest`, `GetRepoCard`, `GetChangelog`, `GetRepoData`, `FetchAndCache`) uses `updateCacheEntry(repo, mutate)`, which under `cacheMu` re-reads the current cache from disk, applies `mutate` to that single entry, and writes back. This makes each entry's update atomic and merge-on-write: `mutate` takes missing fields from the freshly-read `existing` entry, so parallel startup goroutines writing different repos no longer clobber each other (the old "last writer wins on a stale whole-file snapshot" bug that forced a full refetch every start). Never write the cache by hand with `LoadCache`/`SaveCache` outside this helper. `GetRepoData` is the single network pass (release + repo info + languages); `GetLatest`/`GetRepoCard` are thin wrappers over it to avoid a duplicate `fetchRepoInfo`.

**Force refresh** (`[r]` in the TUI): `GetRepoData`/`GetChangelog` are thin wrappers over `getRepoData(field, force)`/`getChangelog(field, force)`; `force` skips **only** the freshness short-circuit, reusing the same fetch + `updateCacheEntry` merge + conclusive-`CheckedAt` guard. `RefreshRepoData`/`RefreshChangelog` (`force=true`) re-fetch on demand while still respecting the poison-guard — a forced refresh that hits a rate limit on repo-info does not stamp the entry fresh-but-blank.

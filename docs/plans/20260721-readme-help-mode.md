# README as the default mode of panel [3]

## Overview

Panel `[3]` currently has two sources — `[h]` `--help` and `[m]` `man` — both local
subprocess captures. For a tracked-but-not-installed tool the panel is dead: neither
source can exist. This plan adds the repository README as a **third mode that is the
default**: selecting a tool immediately shows its rendered README; `[h]`/`[m]` switch
to help/man as before; a new lowercase `[r]` (free in `focusHelp`) returns to the
README. The README is fetched from the GitHub API (raw markdown, 1 request per tool
per 24h, cached in `cache.json`) and rendered with `glamour`.

Key benefits: the panel is meaningful from the first screen for every tool with a
GitHub ref, including uninstalled ones — exactly the tools being evaluated
(`trying`) where docs matter most.

## Context (from discovery)

- `internal/version/github.go` — `CacheEntry` (line ~265) with `CheckedAt`;
  changelog pattern `GetChangelog`/`RefreshChangelog`/`getChangelog(field, force)`
  (lines 439–490) is the template: freshness check `!force && cached &&
  time.Since(entry.CheckedAt) < cacheTTL && entry.Body != ""`, write via
  `updateCacheEntry` merge. `doGH` is the single auth/rate point but currently
  **overwrites** the `Accept` header — README needs `application/vnd.github.raw+json`.
- `internal/model/commands.go` — `fetchChangelogCmd`/`refreshChangelogCmd` (121–155),
  predicates `needsInstalled`/`needsRemote` (157–180), `refreshSelectedCmd` (189),
  `autoFetchCmdsForSelected` (~210).
- `internal/model/model.go` — `helpModeHelp=0`/`helpModeMan=1` (81–82), sticky
  `m.helpMode` field; `h`/`m` keys fire on `focusBrief || focusHelp` (902–944) and
  dismiss a completed update log; `case "r"` has only `focusTools` (rename) and
  `focusBrief` (refresh) branches — free in `focusHelp`; `m.changelogData
  map[string]changelogMsg` session cache; `setHelpContent()` (1220) is the single
  recompute point; rename clears per-tool maps.
- `internal/model/render.go` — `renderHelpContent` (~1050–1110) with update-log
  priority branch, `insetPanelTitle` for `[3] Help`/`[3] Man`, `renderStatusBar`
  per-focus hints, `renderHotkeys` overlay with a hard ≤20×76 budget.

## Development Approach

- **testing approach**: Regular (code first, then tests within the same task)
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
  - tests are not optional — they are a required part of the checklist
  - cover both success and error scenarios
- **CRITICAL: all tests must pass before starting next task** — run `go test -race ./...`
- **CRITICAL: update this plan file when scope changes during implementation**
- maintain backward compatibility (existing `[h]`/`[m]` behavior unchanged)

## Testing Strategy

- **unit tests**: required for every task; `version` tests go through the existing
  `testAPIBase`/`testCacheDir` httptest seams; `model` tests drive `Update()` with
  key/msg events like the existing mode tests
- **e2e**: project has none — manual TUI smoke run listed in Post-Completion

## Progress Tracking

- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix

## Solution Overview

- **Fetch**: `version.GetReadme(githubField)` / `RefreshReadme` over shared
  `getReadme(field, force)` — `GET /repos/{owner}/{repo}/readme` with
  `Accept: application/vnd.github.raw+json` through `doGH`; result cached in
  `cache.json` (`CacheEntry.Readme`) via `updateCacheEntry`, 24h TTL, same
  conclusive-write guard as changelog. 404 → typed `ErrNoReadme`.
- **Model**: mirror the changelog flow — `fetchReadmeCmd`/`refreshReadmeCmd` +
  `readmeMsg{toolName, content, err}`, session cache `m.readmeData
  map[string]readmeMsg`, predicate `needsReadme(t)`, fired from
  `autoFetchCmdsForSelected` (README loads on selection); `refreshSelectedCmd`
  also force-refreshes it. Rename clears `readmeData`.
- **Mode**: `helpModeReadme` third constant, the **startup default** in `New()`;
  `helpMode` stays a sticky global field; live update log keeps priority.
- **Render**: `glamour.Render` with `WithAutoStyle()` + `WithWordWrap(helpWrapWidth())`
  over `cleanTerminalOutput`-sanitized markdown; result cached in `helpBase` via
  `setHelpContent()`. No `parseHelpEntries`/`colorizeHelp` in readme mode — entries
  empty, `j`/`k` = plain scroll (same branch the update log uses). Help-search `/`
  is a no-op in readme mode (v1).
- **Keys/UI**: third branch in `case "r"` (`focusHelp` → readme + completed-update-log
  dismissal, same as `h`/`m`); panel title `[3] Readme`; `focusHelp` status bar gains
  `[r] readme`; `[?]` overlay gains one row.

## Technical Details

- `CacheEntry` gains `Readme string \`json:"readme,omitempty"\`` **and a dedicated
  `ReadmeCheckedAt time.Time`** — NOT the shared `CheckedAt`. Rationale (plan
  review): `getRepoData`'s freshness gate is `CheckedAt`-only with no content
  check; a successful README fetch stamping the shared timestamp would mark a
  rate-limited (deliberately stale) repo fetch as fresh → blank card, no retry
  for 24h. A separate timestamp keeps the two poison-guards independent. Hit
  requires `time.Since(ReadmeCheckedAt) < cacheTTL && Readme != ""`; a failed
  fetch never advances `ReadmeCheckedAt`.
- **`getChangelog`'s write is a `CacheEntry{...}` literal** (github.go:477-489)
  that copies only the fields it knows — it would silently wipe `Readme`/
  `ReadmeCheckedAt` on every changelog fetch. It must copy the new fields from
  `existing` (or switch to the `e := existing` style `getRepoData` uses). This is
  the only leaky writer; `getRepoData` mutates a copy and preserves new fields
  automatically.
- **`helpCache` is a fixed `[2]string` indexed by `m.helpMode`** — with
  `helpModeReadme = 2` every `cached[m.helpMode]` site panics or misroutes unless
  guarded. All index sites (verified): `autoFetchCmdsForSelected`'s switch
  (commands.go:236), `rawHelpText` (render.go:1056), `renderHelpContent`
  (render.go:1087/1102/1104), plus the `h`/`m` key handlers (already mode-explicit,
  safe). Each gets a readme early-return/branch *before* the array index.
- `doGH` change: set `Accept` **only when the request doesn't already carry one**
  (`req.Header.Get("Accept") == ""`), so `fetchReadme` can pre-set the raw media
  type without a second HTTP path. Existing fetchers are unaffected (they set none).
- `readmeMsg` mirrors `changelogMsg`: stored whole in `m.readmeData`, so a 404 or
  rate-limit error is a session-cached negative result — no refetch loop on every
  selection move; `[r]`-refresh in brief clears/overwrites it.
- Placeholders in `renderHelpContent` readme branch (tool-named, matching existing
  style): no GitHub ref → `No repo for <name>. Press [h] for --help.`; loading →
  existing `helpLoadingFor` path; `ErrNoReadme` → `No README in <owner/repo>.
  Press [h] for --help.`; rate-limited (`errors.Is ErrRateLimited`) →
  `rate limited — press [L]`; a cached README survives later network failures
  (known-content-wins merge, like repo cards).
- Width-change resize re-renders through the existing `setHelpContent` gate
  (`helpWrapWidth()` changed); height-only resize keeps scroll position.
- glamour output is ANSI: it bypasses `colorizeHelp` and never meets
  `applySpotlight`/`highlightMatch` (entries empty, `/` no-op), so no ANSI-tearing.
- **No `glamour.WithAutoStyle()`** — auto-style probes the live terminal via a
  termenv OSC background query reading stdin, which races Bubble Tea's input
  reader and violates the project's terminal-sandboxing policy. Instead resolve
  dark/light **once** at model construction via lipgloss's cached
  `HasDarkBackground()` and pass the fixed `glamour.WithStandardStyle("dark"|"light")`.
- **Rendered-output cache**: `setHelpContent` runs on every `j`/`k` selection move
  and every resize; re-parsing a large README through glamour each time is
  noticeably heavier than `colorizeHelp`. Cache the rendered ANSI keyed by
  `(name, width)` (single-entry cache is enough — invalidate when either changes).
- `/` help-search enters from `focusBrief || focusHelp` (model.go:884-892) — the
  readme no-op guard must cover **both** entry paths, not just `focusHelp`.

## What Goes Where

- **Implementation Steps** (`[ ]` checkboxes): code, tests, docs in this repo
- **Post-Completion** (no checkboxes): manual TUI smoke checks

## Implementation Steps

### Task 1: version.GetReadme with cache and raw Accept header

**Files:**
- Modify: `internal/version/github.go`
- Modify: `internal/version/github_test.go`

- [ ] make `doGH` set `Accept` only when the request has none (preserve pre-set header)
- [ ] add `Readme string` (json `readme,omitempty`) **and `ReadmeCheckedAt
      time.Time`** to `CacheEntry` — freshness independent from the repo-card
      `CheckedAt` (see Technical Details: shared timestamp would defeat
      `getRepoData`'s poison guard)
- [ ] fix `getChangelog`'s `CacheEntry{...}` write literal (github.go:477-489) to
      carry `Readme`/`ReadmeCheckedAt` over from `existing` so a changelog fetch
      never wipes a cached README
- [ ] add `fetchReadme(repo)` — `GET /repos/{repo}/readme`, `Accept:
      application/vnd.github.raw+json`, body is raw markdown; 404 → typed `ErrNoReadme`
- [ ] add `getReadme(field, force)` + public `GetReadme`/`RefreshReadme` following
      `getChangelog`: TTL short-circuit on `ReadmeCheckedAt` + `Readme != ""`, write
      via `updateCacheEntry`, failed fetch leaves `ReadmeCheckedAt` stale
- [ ] write tests against `testAPIBase` httptest: success (raw md round-trip,
      read-after-write), 404 → `ErrNoReadme`, rate-limited 403 → `ErrRateLimited`,
      force bypasses TTL, failure doesn't poison a cached README, Accept header
      asserted on the recorded request
- [ ] write cross-writer tests: a changelog fetch preserves an existing `Readme`;
      a successful README fetch does **not** mark a failed/stale repo fetch fresh
      (repo-card `CheckedAt` untouched)
- [ ] run `go test -race ./internal/version/...` — must pass before task 2

### Task 2: glamour rendering helper

**Files:**
- Modify: `go.mod`, `go.sum`
- Create: `internal/model/readme.go`
- Create: `internal/model/readme_test.go`

- [ ] `go get github.com/charmbracelet/glamour` (then `go mod tidy`)
- [ ] add `renderReadme(raw string, width int, dark bool) string`:
      `cleanTerminalOutput(raw)` → `glamour.Render` with
      `WithStandardStyle("dark"|"light")` + `WithWordWrap(width)`; on glamour error
      fall back to the sanitized plain text (never an empty panel). **No
      `WithAutoStyle()`** — it probes the terminal via OSC/stdin and races Bubble
      Tea's input reader; resolve dark/light once at model construction via
      lipgloss's cached `HasDarkBackground()` and store it on the model
- [ ] add a single-entry rendered cache keyed by `(name, width)` so selection moves
      and repaints don't re-parse the same README through glamour
- [ ] write tests: headings/lists/code render to non-empty output containing the
      expected text (do NOT assert ANSI escapes — under `NO_COLOR`/dumb term glamour
      emits plain text), wrap respects width, control characters in input are
      stripped, glamour failure falls back to plain text, render cache hit/invalidation
- [ ] run `go test -race ./internal/model/...` — must pass before task 3

### Task 3: model plumbing — fetch command, message, session cache

**Files:**
- Modify: `internal/model/commands.go`
- Modify: `internal/model/model.go`
- Modify: `internal/model/commands_test.go`, `internal/model/model_test.go`

- [ ] add `readmeMsg{toolName, content string, err error}` and
      `fetchReadmeCmd`/`refreshReadmeCmd` (mirroring changelog cmds, `safeCmd`-wrapped,
      `logx.Errorf` on failure)
- [ ] add `m.readmeData map[string]readmeMsg` (init in `New()`); handler stores the
      msg but keeps previously known content on error (known-wins merge); repaint via
      `setHelpContent()` when the msg is for the selected tool and mode is readme
- [ ] add `needsReadme(t)` predicate (`t.GitHub != ""` and no `readmeData` entry)
- [ ] restructure `autoFetchCmdsForSelected`'s help switch (commands.go:227-243):
      add a `case m.helpMode == helpModeReadme:` **before** the
      `m.helpCache[mt.Name][m.helpMode] == ""` case — repaint via `setHelpContent`,
      fire `fetchReadmeCmd` when `needsReadme`, and do **not** set `helpLoadingFor`
      or fire `fetchHelpCmd` (the existing case both indexes the `[2]string` out of
      range with mode 2 and would spawn a bogus subprocess)
- [ ] queue `fetchReadmeCmd` for the initially selected tool in `Init()`
      (model.go:283-292) — startup fires help/changelog directly, not via
      `autoFetchCmdsForSelected`, so without this the default readme panel shows a
      placeholder until the user moves the selection
- [ ] add `refreshReadmeCmd` to `refreshSelectedCmd` (force path, clears the session
      entry so a 404-negative can recover)
- [ ] clear `readmeData` old-name entry on rename alongside the existing map cleanups
- [ ] write tests: msg merge (error keeps known content), `needsReadme` gating,
      auto-fetch fires `fetchReadmeCmd` (and NOT `fetchHelpCmd`) in readme mode,
      `Init()` includes the readme fetch, rename invalidation
- [ ] run `go test -race ./internal/model/...` — must pass before task 4

### Task 4: helpModeReadme — default mode, [r] key, render branch

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/render.go`
- Modify: `internal/model/mode_test.go`, `internal/model/render_test.go`

- [ ] add `helpModeReadme` const; set `m.helpMode = helpModeReadme` in `New()`
- [ ] third branch in `case "r"`: `focus == focusHelp` → `helpMode = helpModeReadme`,
      dismiss a *completed* update log (same guard as `h`/`m`), `setHelpContent()` +
      `GotoTop()`; live update log stays on top
- [ ] guard **every** `[m.helpMode]` index against `helpModeReadme` (the
      `helpCache` value is `[2]string` — mode 2 panics): readme early-return in
      `rawHelpText` (render.go:1056) and a readme branch ahead of the array reads
      in `renderHelpContent` (render.go:1087/1102/1104); `autoFetchCmdsForSelected`
      is covered in task 3; audit for any remaining `helpCache[...][m.helpMode]`
      sites with grep before closing the task
- [ ] readme branch in `renderHelpContent`/`setHelpContent`: serve
      `renderReadme(content, helpWrapWidth(), m.darkBG)` as `helpBase`; entries stay
      empty (`j`/`k` plain scroll); update-log priority branch unchanged
- [ ] placeholders: no GitHub ref / loading (`helpLoadingFor`) / `ErrNoReadme` /
      rate-limited — tool-named messages per Technical Details
- [ ] make `/` (help search) a no-op while `helpMode == helpModeReadme` — guard the
      shared entry path that fires from `focusBrief || focusHelp` (model.go:884-892),
      not just a `focusHelp` branch
- [ ] width-change resize re-renders readme (existing `helpWrapWidth()` gate);
      height-only keeps scroll
- [ ] write tests: default mode at startup is readme **without panics across a full
      select→render cycle** (the `[2]string` guard), `r` in `focusHelp` switches
      mode (and is inert in tools/brief — rename/refresh untouched), `r` no-op with
      a live update log, placeholder texts, `/` no-op in readme mode from both
      brief and help focus, resize re-render
- [ ] run `go test -race ./internal/model/...` — must pass before task 5

### Task 5: UI surfaces — panel title, status bar, hotkeys overlay

**Files:**
- Modify: `internal/model/render.go`
- Modify: `internal/model/render_test.go`

- [ ] panel title `[3] Readme` via the existing `insetPanelTitle` switch on `helpMode`
- [ ] `focusHelp` status bar: add `[r] readme` alongside `[h] help` / `[m] man`
- [ ] `[?]` overlay: add the `r` row to the panel-[3] group; verify the ≤20×76 budget
      still holds (re-balance columns if needed); the `/` search row keeps its text —
      search simply doesn't apply in readme mode (v1)
- [ ] write tests: title follows the third mode, status bar shows the hint, hotkeys
      overlay fits the size budget
- [ ] run `go test -race ./internal/model/...` — must pass before task 6

### Task 6: Verify acceptance criteria

- [ ] verify all requirements from Overview are implemented (README default on
      selection, `[h]`/`[m]`/`[r]` switching, uninstalled-tool case, quota = 1 lazy
      request/tool/24h)
- [ ] verify edge cases: no GitHub ref, no README (404), rate limit with and without
      cached content, rename mid-session, live update log priority
- [ ] run full suite: `go test -race ./...`
- [ ] run `go vet ./...` and `golangci-lint run`

### Task 7: [Final] Update documentation

- [ ] update `CLAUDE.md`: panel `[3]` modes (readme default), new fetch path,
      `doGH` Accept rule, `readmeData` map, key table
- [ ] update `README.md` hotkeys section if it lists panel keys
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion

**Manual verification:**
- run the TUI: select an uninstalled tracked tool → README renders immediately;
  `[h]`/`[m]`/`[r]` cycle correctly; tool without GitHub ref shows the placeholder
- check a badge-heavy README (e.g. a popular repo) for visual noise in glamour output
- check dark and light terminal themes (`WithAutoStyle`)
- cold start with no token: confirm the rate gauge reflects the extra request only
  on selection, not for every tracked tool

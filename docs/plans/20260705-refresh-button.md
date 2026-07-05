# Refresh button (`r`) for the brief card with animated loader

## Overview
- Add an `r [refresh]` action to the `focusBrief` help bar that force-refreshes
  the selected tool's data, bypassing the 24h cache TTL.
- Solves: today the only way to re-fetch a tool's repo data is to wait out the
  24h cache TTL (or restart after the cache expires). A user who just published
  a release, fixed a rate-limited cold start, or renamed a repo has no way to
  pull fresh data on demand.
- Scope of a refresh: force `GetRepoData` (release + repo-info + languages, 3
  requests) + force `GetChangelog` (1 request) + re-detect the installed version
  (local subprocess, no cache). While the repo pass is in flight the card shows
  a minimalist animated spinner (`bubbles/spinner`, `MiniDot`).
- Integrates through the existing async message pipeline: force fetches emit the
  same `remoteMsg` / `changelogMsg` the startup path already handles, so the
  merge-into-state + re-render logic is reused unchanged.

## Context (from discovery)
- Files/components involved:
  - `internal/version/github.go` — `GetRepoData` (~288), `GetChangelog` (~411);
    freshness gates at ~303 (repo) and ~401 (changelog); shared
    `updateCacheEntry` + conclusive-`CheckedAt` logic at ~330-364.
  - `internal/model/model.go` — `fetchRemoteCmd` (~1732), `fetchChangelogCmd`
    (~1772), `fetchInstalledCmd` (~1719), `autoFetchCmdsForSelected` (~1811),
    `case "r"` key handler (634, rename in `focusTools` only), `focusBrief`
    action keys `o/c/s` (644-675), help bar for `focusBrief` (~1035), `renderCard`
    tool-name render (~1441), `remoteMsg` handler (~262), model struct/`New()`
    (~31/~87/~181).
  - Tests: `internal/version/github_test.go`, `internal/model/render_test.go`.
- Related patterns found:
  - Per-tool fetch split: local (`fetchInstalledCmd`) never waits on network;
    network pass is one `GetRepoData` call. Cmds emit typed msgs handled in
    `Update`, which merges into `m.versions` / `m.repoCards` / `m.repoStatus` /
    `m.changelogData` and calls `m.briefViewport.SetContent(m.renderCard())`.
  - `focusBrief` action keys already follow a uniform `case "X": if m.focus ==
    focusBrief { ... }` shape (mirror it for `r`).
  - Existing loading-state fields: `changelogLoadingFor`, `helpLoadingFor`,
    `statusMsg`.
- Dependencies identified:
  - `github.com/charmbracelet/bubbles v1.0.0` already required; `bubbles/spinner`
    present in the module (currently unused). No new dependency.
  - Recent `conclusive`-`CheckedAt` fix (PR #10) lives in the shared write path;
    the force path must reuse it, not duplicate it.

## Development Approach
- **Testing approach**: Regular (code first, then tests in the same task).
- Complete each task fully before moving to the next.
- Make small, focused changes; run tests after each change.
- **Every task MUST include new/updated tests** (success + error/edge cases) as
  separate checklist items.
- **All tests must pass before starting the next task.**
- Maintain backward compatibility: existing `GetRepoData` / `GetChangelog`
  signatures stay via thin wrappers; existing call sites unchanged.
- Update this plan when scope changes during implementation.

## Testing Strategy
- **Unit tests**: required for every task.
  - `version`: `httptest` server with a request counter to prove TTL bypass and
    the preserved conclusive-guard behavior.
  - `model`: drive `Update` with `tea.KeyMsg` and synthetic `remoteMsg` to assert
    state transitions (`refreshingFor`, `statusMsg`) and render output.
- **E2E tests**: project has no UI e2e harness. The spinner frame animation
  (tea.Tick timing) is verified manually by running the TUI (see Post-Completion).

## Progress Tracking
- Mark completed items with `[x]` immediately when done.
- Add newly discovered tasks with ➕ prefix; blockers with ⚠️ prefix.
- Keep the plan in sync with actual work.

## Solution Overview
- **Force path in `version`**: refactor `GetRepoData` → `getRepoData(field, force
  bool)` (public `GetRepoData` = `force=false`; new exported `RefreshRepoData` =
  `force=true`). `force` skips ONLY the freshness short-circuit; the network pass,
  `updateCacheEntry` merge, and conclusive-`CheckedAt` logic stay shared, so a
  force refresh that hits a rate limit on repo-info still does not poison the
  cache. Symmetric `getChangelog(field, force)` + `RefreshChangelog`.
- **Force commands in `model`**: parametrize the data source without duplicating
  message construction — `remoteCmd(t, force)` / `changelogCmd(field, name,
  force)`, with `fetchRemoteCmd`/`fetchChangelogCmd` as the `force=false` wrappers
  (existing call sites untouched) and `refreshRemoteCmd`/`refreshChangelogCmd` as
  the `force=true` wrappers.
- **Animated loader**: `spinner.Model` (`MiniDot`) + `refreshingFor string`
  (dual purpose: animation trigger + double-press guard). `r` in `focusBrief`
  starts the batch and the spinner; `spinner.TickMsg` re-renders while
  `refreshingFor != ""`; the `remoteMsg` handler clears `refreshingFor` when the
  repo pass returns, which halts the tick loop.

## Technical Details
- `RepoData` / `ReleaseInfo` shapes unchanged; no new message types.
- `refreshSelectedCmd(t)` (pointer receiver, sets state then returns a Cmd):
  - `t.GitHub != ""`: set `refreshingFor = t.Name`, `statusMsg = "refreshing " +
    t.Name + "…"`, `return tea.Batch(m.spinner.Tick, fetchInstalledCmd(t),
    refreshRemoteCmd(t), refreshChangelogCmd(t.GitHub, t.Name))`.
  - `t.GitHub == ""`: `statusMsg = "no repo to refresh"`, `return
    fetchInstalledCmd(t)`; do NOT set `refreshingFor` (nothing clears it → no
    perpetual spinner).
  - Guard: if `m.refreshingFor == t.Name`, ignore (return `nil`).
- `spinner.TickMsg` in `Update`: if `refreshingFor == ""` do nothing; else
  `m.spinner, cmd = m.spinner.Update(msg)`, re-render card, return `cmd`.
- `remoteMsg` handler: after the existing merge, if `msg.toolName ==
  m.refreshingFor` → `m.refreshingFor = ""`, `m.statusMsg = ""`.
- `renderCard`: if `m.refreshingFor == t.Name`, append `" " + m.spinner.View()`
  to the tool-name line.
- Help bar (`focusBrief`, ~1035): insert `[r] refresh` between `[c] changelog`
  and `[s] status`.

## What Goes Where
- **Implementation Steps** (checkboxes): all code + tests in this repo.
- **Post-Completion** (no checkboxes): manual TUI run to eyeball the spinner
  animation and the refreshed card.

## Implementation Steps

### Task 1: version force-refresh path

**Files:**
- Modify: `internal/version/github.go`
- Modify: `internal/version/github_test.go`

- [ ] refactor `GetRepoData` → `getRepoData(field string, force bool)`; keep
      public `func GetRepoData(field string) RepoData { return getRepoData(field,
      false) }`; add `func RefreshRepoData(field string) RepoData { return
      getRepoData(field, true) }`. `force` skips ONLY the `time.Since(CheckedAt) <
      cacheTTL` short-circuit (~303); leave the fetch/merge/conclusive block intact.
- [ ] refactor `GetChangelog` → `getChangelog(field string, force bool)` with
      public wrapper `force=false` and new `RefreshChangelog` `force=true`; `force`
      skips ONLY the `cached && fresh && Body != ""` short-circuit (~401); keep the
      on-error cached fallback.
- [ ] write `TestRefreshRepoDataBypassesTTL`: prime a fresh entry via
      `GetRepoData`, change the server response (e.g. stars 42→99), assert
      `RefreshRepoData` makes a new network pass (request counter increments) and
      returns the new value; assert a plain `GetRepoData` in the same state makes
      no request.
- [ ] write `TestRefreshRepoDataKeepsConclusiveGuard`: force pass with repo-info
      returning 403 + `X-RateLimit-Remaining: 0` while release/languages succeed →
      `CheckedAt` not advanced (next call re-fetches) and cached `About` not wiped.
- [ ] write `TestRefreshChangelogBypassesTTL`: fresh entry with `Body` present →
      `RefreshChangelog` still re-fetches and updates the body.
- [ ] run `go test ./internal/version/` - must pass before task 2

### Task 2: model force commands + spinner/refreshingFor state

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/render_test.go`

- [ ] add `remoteCmd(t loader.Tool, force bool) tea.Cmd` holding the current
      `fetchRemoteCmd` body but selecting `version.RefreshRepoData` when `force`,
      else `version.GetRepoData`; redefine `fetchRemoteCmd(t) = remoteCmd(t,
      false)` and add `refreshRemoteCmd(t) = remoteCmd(t, true)`.
- [ ] add `changelogCmd(field, name string, force bool) tea.Cmd` similarly;
      redefine `fetchChangelogCmd = changelogCmd(..., false)` and add
      `refreshChangelogCmd = changelogCmd(..., true)`.
- [ ] add model fields `spinner spinner.Model` and `refreshingFor string` to the
      struct; initialize the spinner in `New()` with `spinner.MiniDot` and a muted
      `ui` style.
- [ ] handle `spinner.TickMsg` in `Update`: no-op when `refreshingFor == ""`;
      otherwise advance the spinner, re-render the card, return the next tick cmd.
- [ ] write test: `refreshRemoteCmd(t)()` against an `httptest` server returns a
      `remoteMsg` with freshly-fetched data even when the cache entry is fresh
      (TTL bypass surfaces through the model cmd).
- [ ] write test: `spinner.TickMsg` with `refreshingFor == ""` returns no
      command (loop halts); with `refreshingFor` set returns a non-nil command.
- [ ] run `go test ./internal/model/` - must pass before task 3

### Task 3: `r` key handler, refresh batching, and stop-on-completion

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/render_test.go`

- [ ] add `refreshSelectedCmd(t loader.Tool) tea.Cmd` (pointer receiver) per
      Technical Details: guard on `refreshingFor == t.Name`; GitHub vs no-GitHub
      branches; sets `refreshingFor` / `statusMsg`; batches spinner tick + the
      three fetch cmds.
- [ ] extend the existing `case "r"` (line ~634): keep `focusTools` rename branch
      untouched; add `else if m.focus == focusBrief { return m,
      m.refreshSelectedCmd(t) }` using the selected tool.
- [ ] in the `remoteMsg` handler (~262), after the existing merge, clear
      `refreshingFor` and `statusMsg` when `msg.toolName == m.refreshingFor`.
- [ ] write test: `r` in `focusBrief` on a tool with `GitHub` sets
      `refreshingFor == t.Name`, `statusMsg` contains "refreshing", returns a
      non-nil command.
- [ ] write test: `remoteMsg{toolName: name}` clears `refreshingFor` to "".
- [ ] write test: `r` on a no-`GitHub` tool leaves `refreshingFor` empty and sets
      `statusMsg == "no repo to refresh"`.
- [ ] write test: second `r` while `refreshingFor == name` leaves the flag
      unchanged and does not panic; and `r` in `focusTools` still starts rename.
- [ ] run `go test ./internal/model/` - must pass before task 4

### Task 4: render spinner on the card and help-bar hint

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/render_test.go`

- [ ] in `renderCard` (~1441), when `m.refreshingFor == t.Name`, append `" " +
      m.spinner.View()` to the tool-name line (both the with-about and name-only
      branches).
- [ ] in the `focusBrief` help bar (~1035), insert `[r] refresh` between
      `[c] changelog` and `[s] status`.
- [ ] write test: `focusBrief` help bar output contains "refresh".
- [ ] write test: `renderCard` with `refreshingFor` set to the selected tool
      includes a spinner frame on the name line; with it unset it does not.
- [ ] run `go test ./internal/model/` - must pass before task 5

### Task 5: Verify acceptance criteria

- [ ] verify all Overview requirements: `r` in `focusBrief` force-refreshes repo
      data + changelog + installed, bypassing TTL, with a spinner during the repo
      pass and a silent stop on completion.
- [ ] verify edge cases: no-`GitHub` tool, double-press guard, `focusTools` `r`
      still renames, force refresh under rate limit does not poison the cache.
- [ ] run full suite: `go build ./... && go vet ./... && go test -race ./...`
- [ ] confirm no new `go vet` findings and `-race` is clean.

### Task 6: Update documentation

- [ ] update `CLAUDE.md`: document the `r` refresh action in the `focusBrief`
      section and note the `RefreshRepoData` / `RefreshChangelog` force path in the
      GitHub API section.
- [ ] update the help-bar key list mention in `CLAUDE.md` (`[o] open repo …`) to
      include `[r] refresh`.
- [ ] move this plan to `docs/plans/completed/`.

## Post-Completion
*Items requiring manual intervention - no checkboxes, informational only*

**Manual verification:**
- Run the TUI (`go run .`), select a GitHub-backed tool, press `r`, and confirm:
  the `MiniDot` spinner animates on the card name line, the "refreshing …"
  status shows, the card/latest/languages/stars update, and the spinner stops
  when data arrives.
- Press `r` on a tool with no repo → "no repo to refresh", no spinner.
- With a warm cache, press `r` and confirm a real network pass happens (e.g.
  observe an updated `latest` after publishing a release), proving TTL bypass.

**Branch / PR conventions:**
- Do the work on a dedicated branch (not `main`).
- PR body in English, no Claude Code footer; commit messages without co-author
  footer.

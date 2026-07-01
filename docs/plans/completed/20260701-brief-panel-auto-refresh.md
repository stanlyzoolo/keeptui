# Brief Panel Auto-Refresh on Track/Rename

## Overview

Fix a bug: when a tool is added (tracked) or renamed during a running session, the Central
Brief Panel does not auto-load all sections. The **About** line and the whole `[info]` block
(repo stars, latest, languages, maintenance) stay blank, and the header version info
(installed/latest) is missing, until the user quits and restarts the app.

- **Problem**: async fetches diverged into two asymmetric paths. `Init()` (startup) fetches
  four sources per tool — version (`versionMsg`), repo card (`repoCardMsg`), changelog, and
  `--help`. But `autoFetchCmdsForSelected()` — the function run after track/untrack/rename and
  selection changes — fetches only changelog and `--help`. So a tool added/renamed after
  startup never gets its repo card or version fetched; those sections stay empty until `Init()`
  re-runs on restart.
- **Benefit**: newly tracked/renamed tools show a complete card immediately, no restart needed.

## Context (from discovery)

Files/components involved:
- `internal/model/model.go`:
  - `Init()` (`model.go:161-193`) — inline version-fetch goroutine (`164-174`) and
    `fetchRepoCardCmd(t)` (`176`); the only place these two fire today.
  - `autoFetchCmdsForSelected()` (`model.go:1429-1450`) — fetches only changelog + help.
  - `renderCard()` (`model.go:1110`) — About + `[info]` sourced from `m.repoCards[t.Name]`
    (`1124`, `1134`, `1150-1172`); header version from `m.versions` / `m.repoStatus`.
  - `updateTrackInput` (`model.go:683-717`), `updateRenameInput` (`model.go:765-801`) — call
    `autoFetchCmdsForSelected()` after mutating meta.
  - `fetchRepoCardCmd` (`model.go:1405`), `versionMsg` (`model.go:35`).
- `internal/model/render_test.go` — table-driven tests, extended.

Related patterns found:
- `autoFetchCmdsForSelected()` already guards changelog with
  `if _, already := m.changelogData[t.Name]; !already && m.changelogLoadingFor != t.Name` — the
  same "skip if cached" pattern applies to repo card and version.
- Version fetch is currently an inline closure in `Init()`, not a reusable helper.
- `updateRenameInput` deletes `m.helpCache[old]` (`model.go:791`) but leaves
  `m.repoCards[old]` / `m.versions[old]` / `m.repoStatus[old]` / `m.changelogData[old]` as
  stale orphans keyed by the old name.

Dependencies identified: none new.

## Development Approach

- **testing approach**: Regular (code + tests together per task), matching the existing
  table-driven `render_test.go`.
- complete each task fully before moving to the next; small, focused changes.
- **every task includes new/updated tests** for its code changes (success + edge cases).
- **all tests must pass before starting the next task**.
- update this plan when scope changes during implementation.
- run `go build ./... && go vet ./... && go test ./...` after each change.
- maintain backward compatibility (startup behavior must not change).

## Testing Strategy

- **unit tests**: required for every task. The project has no e2e/UI harness — behavior is
  verified via `render_test.go` (model state and pure predicate assertions).
- Network fetches are not exercised in unit tests. Instead, the fetch decision is extracted into
  pure predicates (`needsRepoCard`, `needsVersion`) that are unit-tested directly; `tea.Cmd`
  assembly stays thin.
- Rename cache-cleanup is tested by driving `updateRenameInput` through `Update()` and asserting
  the old-name keys are removed from the cache maps.

## Progress Tracking

- mark completed items with `[x]` immediately when done.
- add newly discovered tasks with ➕ prefix; blockers with ⚠️ prefix.
- keep the plan in sync with actual work.

## Solution Overview

Make `Init()` and post-startup selection symmetric. Extract the inline version-fetch goroutine
into a reusable `fetchVersionCmd(t)` and have `autoFetchCmdsForSelected()` also queue the repo
card and version fetches for the selected tool when they are not already cached. Clean up the
stale per-name cache entries on rename so the renamed tool re-fetches under its new name.

Key design decisions:
- **Reuse `autoFetchCmdsForSelected()`, not the track/rename handlers** — all four call sites
  (track, untrack, rename, selection change) already route through it, so fixing it once covers
  every path.
- **Pure predicates for testability** — `needsRepoCard(t)` / `needsVersion(t)` encapsulate the
  "skip if cached / no GitHub" logic and are unit-tested without launching a fetch.
- **Version fetch fires even without GitHub** — installed version is detected locally, matching
  `Init()`, which fires the version goroutine for every tool regardless of `GitHub`.
- **Repo card requires GitHub** — matches the `Init()` guard (`t.GitHub != ""` at `model.go:175`).

## Technical Details

- `fetchVersionCmd(t loader.Tool) tea.Cmd`: returns the closure currently inline in `Init()`
  (`InstalledVersion` / `GetLatest` / `GetCachedRepoStatus` → `versionMsg`). `Init()` calls it
  instead of the inline literal; startup behavior is unchanged.
- Predicates (pointer or value receiver, pure):
  - `needsVersion(t loader.Tool) bool` → `_, ok := m.versions[t.Name]; return !ok`
  - `needsRepoCard(t loader.Tool) bool` → `t.GitHub != "" && !mapHas(m.repoCards, t.Name)`
- `autoFetchCmdsForSelected()`: after the existing changelog/help logic, for the selected tool:
  - if `m.needsVersion(t)` → append `fetchVersionCmd(t)`
  - if `m.needsRepoCard(t)` → append `fetchRepoCardCmd(t)`
  - guard uses map presence (same idea as the changelog guard). A fetch in flight is not yet in
    the map; a rare double-fetch is harmless (idempotent cached GET) and matches `Init()`, which
    has no in-flight dedup either.
- Rename cleanup in `updateRenameInput` (`model.go:791`): alongside `delete(m.helpCache, old)`,
  also `delete` `m.repoCards[old]`, `m.versions[old]`, `m.repoStatus[old]`,
  `m.changelogData[old]`. The new name re-fetches via `autoFetchCmdsForSelected()`.

## What Goes Where

- **Implementation Steps**: all code + tests + doc note below — achievable in this repo.
- **Post-Completion**: manual smoke test of the real fetch (network) in the running TUI.

## Implementation Steps

### Task 1: Extract fetchVersionCmd helper

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/render_test.go`

- [x] add `func fetchVersionCmd(t loader.Tool) tea.Cmd` returning the closure currently inline in
  `Init()` (`model.go:164-174`), producing `versionMsg`
- [x] replace the inline goroutine in `Init()` with `cmds = append(cmds, fetchVersionCmd(t))` —
  startup behavior unchanged
- [x] write a test asserting `fetchVersionCmd(t)` returns a non-nil `tea.Cmd` (no network assert)
- [x] run `go build ./... && go test ./...` — must pass before Task 2

### Task 2: Add needsVersion / needsRepoCard predicates

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/render_test.go`

- [x] add `func (m *Model) needsVersion(t loader.Tool) bool` (true when `m.versions` has no entry)
- [x] add `func (m *Model) needsRepoCard(t loader.Tool) bool` (true when `t.GitHub != ""` and
  `m.repoCards` has no entry)
- [x] write table tests: fresh tool → true; cached tool → false; repo card with empty `GitHub`
  → false; version with empty `GitHub` → true
- [x] run `go test ./...` — must pass before Task 3

### Task 3: Queue repo-card and version fetches in autoFetchCmdsForSelected

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/render_test.go`

- [x] in `autoFetchCmdsForSelected()` (`model.go:1429`), after the changelog/help block: for the
  selected tool, append `fetchVersionCmd(t)` when `m.needsVersion(t)` and `fetchRepoCardCmd(t)`
  when `m.needsRepoCard(t)`
- [x] write a test: on a model with an empty `repoCards`/`versions` cache and a selected tool
  with `GitHub`, `autoFetchCmdsForSelected()` returns a non-nil batched command
- [x] write a test: when `repoCards`/`versions` already hold the selected tool, the predicates
  report no fetch needed (guards prevent re-fetch)
- [x] run `go test ./...` — must pass before Task 4

### Task 4: Clean stale name-keyed caches on rename

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/render_test.go`

- [x] in `updateRenameInput` (`model.go:791`), alongside `delete(m.helpCache, old)`, delete
  `m.repoCards[old]`, `m.versions[old]`, `m.repoStatus[old]`, `m.changelogData[old]`
- [x] write a test driving `updateRenameInput` (enter with a new name) through `Update()` and
  asserting the old-name keys are gone from `m.repoCards` / `m.versions`
- [x] run `go test ./...` — must pass before Task 5

### Task 5: Verify acceptance criteria
- [x] manual test (skipped - not automatable): verify About + `[info]` + version populate after tracking a tool without restart
- [x] manual test (skipped - not automatable): verify rename re-fetches under the new name and drops stale old-name cache entries
- [x] manual test (skipped - not automatable): verify startup behavior is unchanged (all sections still load on launch)
- [x] run full suite: `go build ./... && go vet ./... && go test ./...` — passes

### Task 6: Update documentation
- [x] update `CLAUDE.md` data-flow note if the fetch responsibility split is now worth documenting
  (`Init()` vs `autoFetchCmdsForSelected()`); skip if it adds no clarity
- [x] move this plan to `docs/plans/completed/`

## Post-Completion

*Items requiring manual intervention — informational only*

**Manual verification:**
- In the running TUI, track a tool with a GitHub repo and confirm the About line, `[info]`
  block, and version populate without quitting. Unit tests assert the fetch is queued but do not
  hit the network.
- Rename a tracked tool and confirm the card re-populates under the new name.

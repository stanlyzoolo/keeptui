# Refactoring: Review Follow-up (CI, dead code, tests, model.go split, mode enum)

## Overview

Address the findings of the 2026-07-07 project review:

- No CI for tests — only `release.yml` exists; regressions can land on `main` unnoticed.
- `CLAUDE.md` describes a loader architecture (embedded configs, `config.yaml`, validation) that no longer exists; `schema.json` and `plan/` are stale relics.
- Dead code: `loader.Load`, `version.GetLatest`, `version.GetRepoCard`, `version.FetchAndCache` are unused in production; `FetchAndCache` also bypasses the conclusive-`CheckedAt` poison guard — a trap for future callers.
- Coverage gaps: `loader/meta.go` (user-data persistence) and `version/detect.go` (version comparison — the core purpose of the app) are at 0%.
- `internal/model/model.go` is a 2278-line monolith with nine mutually-exclusive boolean mode flags (`searching`, `editingNote`, `editingTags`, `tracking`, `confirmingUntrack`, `renaming`, `helpSearching`, `showingAPIStatus`, `enteringToken`) that already required a manual guard for `[L]`.
- `parseVersion` ignores pre-release suffixes (`1.2.3-rc1` compares equal to `1.2.3`).
- `SaveMeta` writes `meta.yaml` non-atomically (`os.WriteFile` in place) — a crash mid-write corrupts user data.

Benefits: CI safety net, honest docs, smaller and testable `model` package, a single `mode` enum instead of flag soup, correct semver comparison, crash-safe metadata writes.

## Context (from discovery)

- Files involved: `.github/workflows/`, `CLAUDE.md`, `schema.json`, `plan/`, `internal/loader/{loader,meta}.go`, `internal/version/{github,detect}.go`, `internal/model/model.go` (+ tests).
- Patterns found: test seams via package-level hooks (`testCacheDir`, `testAPIBase`, `testTokenDir` in `version`) — reuse the same pattern for `loader` config-dir injection.
- Existing tests: `render_test.go` (1692 lines) exercises real rendering; `github_test.go` uses `httptest` — both act as the refactoring safety net.
- Coverage baseline: `ui` 84.8%, `version` 74.5%, `model` 55.8%, `loader` 31.4%.
- Dependencies: adding `golang.org/x/mod` (semver) — small, stdlib-adjacent.

## Development Approach

- **testing approach**: Regular (code first, then tests in the same task)
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
  - tests are not optional - they are a required part of the checklist
  - write unit tests for new functions/methods
  - write unit tests for modified functions/methods
  - add new test cases for new code paths
  - update existing test cases if behavior changes
  - tests cover both success and error scenarios
- **CRITICAL: all tests must pass before starting next task** - no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- run tests after each change (`go test -race ./...`)
- maintain backward compatibility of on-disk formats (`meta.yaml`, `cache.json`, token file)

## Testing Strategy

- **unit tests**: required for every task (see Development Approach above)
- no e2e framework in this project; the TUI is covered by render/update tests in `internal/model` — treat those as the e2e equivalent and extend them where behavior moves
- run with `-race`: the `version` package has real mutex-guarded shared state (`tokenMem`, `rl`, `cacheMu`)

## Progress Tracking

- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- update plan if implementation deviates from original scope
- keep plan in sync with actual work done

## Solution Overview

Order is safety-first: put the net up (CI, tests for untested packages) before the risky work (behavioral semver change, `model.go` split, mode enum). The split is done in two separate tasks — a purely mechanical file move (zero logic change, diff reviewable as moves) and then the mode-enum rework on top of the already-split files. Documentation is updated last so it describes the final state.

Key design decisions:

- **Mode enum**: one `mode inputMode` field replaces the mutually-exclusive booleans. States: `modeNormal`, `modeSearch`, `modeEditNote`, `modeEditTags`, `modeTrack`, `modeConfirmUntrack`, `modeRename`, `modeHelpSearch`, `modeAPIStatus`, `modeTokenInput` (token entry is a sub-state of the API overlay — entering it from `modeAPIStatus`, `esc` returns to `modeAPIStatus`, not `modeNormal`). Guards like "`[L]` only when idle" become `m.mode == modeNormal`; the early-branch dispatch in `Update()` becomes a single `switch m.mode`.
- **Semver**: keep the `IsNewer(installed, latest string) bool` signature; internally canonicalize both sides (prefix `v`, trim build metadata) and delegate to `golang.org/x/mod/semver.Compare`. Pre-release ordering (`1.2.3-rc1` < `1.2.3`) comes for free. Inputs that fail `semver.IsValid` after canonicalization fall back to "not newer" (current behavior for empty strings).
- **Atomic SaveMeta**: write to `meta.yaml.tmp` in the same directory, then `os.Rename`. Same for nothing else — `cache.json` writes are already serialized and are a disposable cache.
- **Config-dir test seam in `loader`**: package-level `testConfigDir` hook mirroring `version.testCacheDir`, so `MetaPath` is injectable without touching `os.UserConfigDir` behavior in production.

## Technical Details

- New file layout for `internal/model` (all same package, no API change):
  - `model.go` — `Model` struct, `New`, `Init`, `Update` dispatch, msg types, selection helpers
  - `mode.go` — `inputMode` enum + per-mode key handlers (`updateNoteEdit`, `updateTagsEdit`, `updateTrackInput`, `updateUntrackConfirm`, `updateRenameInput`, `updateAPIStatus`)
  - `commands.go` — every `tea.Cmd` constructor (`fetchInstalledCmd`, `remoteCmd`, `fetchRateCmd`, `changelogCmd`, `fetchHelpCmd`, `validateTokenCmd`, `openURLCmd` stays in `browser.go`) + `needsInstalled`/`needsRemote`/`refreshSelectedCmd`/`autoFetchCmdsForSelected`
  - `render.go` — `View`, panel renderers, `renderCard`, status bar, gauge, scrollbar, `handleMouse`
  - `textutil.go` — `wrapText`, `stripMarkdown`, `stripANSI`, `cleanTerminalOutput`, `colorizeHelp`, `findMatches`, `highlightMatch`, `formatStars`, `languagePercents`, `renderLangBar`
- Dead-code removal: `loader.Load` (loader.go), `version.GetLatest`, `version.GetRepoCard`, `version.FetchAndCache` (github.go). Their tests are rewired to the surviving public API (`GetRepoData`/`RefreshRepoData`) rather than deleted — the scenarios they cover (cache TTL, merge-on-write, stale fallback) must keep their coverage.
- Repo hygiene: delete `schema.json` and the root `plan/` directory (superseded by `docs/plans/completed/`); rewrite the stale CLAUDE.md sections (loader architecture, "Adding a new built-in tool", file-storage table).
- CI: `.github/workflows/ci.yml` on push/PR to `main` — `go build ./...`, `go vet ./...`, `go test -race ./...`, `golangci-lint` via the official action; minimal `.golangci.yml` (default linters + `govet`, no style zealotry).

## What Goes Where

- **Implementation Steps** (`[ ]` checkboxes): code changes, tests, docs in this repo
- **Post-Completion** (no checkboxes): manual TUI verification, release considerations

## Implementation Steps

### Task 1: Add CI workflow with tests, vet, race and lint

**Files:**
- Create: `.github/workflows/ci.yml`
- Create: `.golangci.yml`

- [ ] create `ci.yml`: triggers on push/PR to `main`; steps: checkout, setup-go (from `go.mod`), `go build ./...`, `go vet ./...`, `go test -race ./...`
- [ ] add `golangci-lint` job via `golangci/golangci-lint-action` with a minimal `.golangci.yml`
- [ ] scope lint findings: fix only trivial ones (unused vars, err checks); for anything larger, disable the linter in `.golangci.yml` with a comment — no drive-by refactoring in this task
- [ ] verify the workflow YAML is valid (`gh workflow view` after push, or actionlint locally if available)
- [ ] run `go test -race ./...` locally - must pass before next task

### Task 2: Remove dead code from loader and version

**Files:**
- Modify: `internal/loader/loader.go`
- Modify: `internal/version/github.go`
- Modify: `internal/version/github_test.go`

- [ ] delete `loader.Load` (unused in production; `ToolsFromMeta` stays — used by `model`)
- [ ] delete `version.GetLatest`, `version.GetRepoCard`, `version.FetchAndCache` from `github.go` (`FetchAndCache` bypasses the poison guard — must not survive)
- [ ] rewire tests that called the deleted wrappers to `GetRepoData`/`RefreshRepoData`, preserving every covered scenario (TTL hit, merge-on-write, stale fallback on error)
- [ ] grep the repo to confirm no remaining references (`grep -rn "FetchAndCache\|GetRepoCard\|GetLatest\|loader.Load("`)
- [ ] run `go build ./... && go vet ./... && go test -race ./...` - must pass before next task

### Task 3: Test coverage for loader/meta.go

**Files:**
- Modify: `internal/loader/meta.go`
- Create: `internal/loader/meta_test.go`

- [ ] add package-level `testConfigDir` seam to `MetaPath` (mirror `version.testCacheDir` pattern)
- [ ] write tests for `LoadMeta`: missing file → empty slice, valid YAML round-trip, malformed YAML → error
- [ ] write tests for `SaveMeta` + `LoadMeta` round-trip (creates directory, preserves all fields incl. tags/note/github)
- [ ] write table tests for `FindMeta`, `UpsertMeta` (update-in-place + append), `RemoveMeta` (present, absent, empty slice)
- [ ] write tests for `NextStatus` (full cycle + unknown status → `StatusActive`)
- [ ] run tests - must pass before next task

### Task 4: Test coverage for version/detect.go

**Files:**
- Create: `internal/version/detect_test.go`

- [ ] write table tests for `IsNewer`: newer/older/equal on each of major/minor/patch, empty strings, `v`-prefix mix (`v1.2.3` vs `1.2.3`)
- [ ] pin current pre-release behavior in a test with a `// TODO: changes in Task 5` note (`1.2.3-rc1` vs `1.2.3`)
- [ ] write tests for `InstalledVersion`: fake tool script on a temp `PATH` (t.Setenv) returning a version string; tool not found → ""; tool exits non-zero → ""
- [ ] write test for `VersionCmd` override path (custom command used instead of `--version`/`-V` candidates; note: `VersionCmd` is never populated from `ToolMeta` today — the test pins the unit contract, not a production flow)
- [ ] run tests - must pass before next task

### Task 5: Semver-correct version comparison

**Files:**
- Modify: `internal/version/detect.go`
- Modify: `internal/version/detect_test.go`
- Modify: `go.mod`

- [ ] add `golang.org/x/mod` dependency
- [ ] rewrite `IsNewer` on top of `semver.Compare` with input canonicalization (ensure `v` prefix, strip build metadata); delete hand-rolled `parseVersion`
- [ ] ⚠️ canonicalization MUST also handle what `semver.IsValid` rejects but the current parser accepts, or the update indicator silently dies for those tools: strip leading zeros in segments (CalVer `2024.01.15`) and truncate a 4th segment (`1.2.3.4` → compare as `1.2.3`)
- [ ] keep fallback: any side still invalid after canonicalization → `false` (matches current empty-string behavior)
- [ ] update the pre-release test from Task 4 to the correct expectation (`1.2.3-rc1` < `1.2.3`), remove the TODO
- [ ] add test cases: build metadata (`1.2.3+build`), two pre-releases (`-rc1` vs `-rc2`), invalid input (`"abc"`), CalVer with zero-padding (`2024.01.15` vs `2024.02.01`), 4-part versions (`1.2.3.4` vs `1.2.3.5` — pin the chosen behavior explicitly)
- [ ] run `go test -race ./...` - must pass before next task

### Task 6: Atomic SaveMeta

**Files:**
- Modify: `internal/loader/meta.go`
- Modify: `internal/loader/meta_test.go`

- [ ] change `SaveMeta` to write `meta.yaml.tmp` in the target directory then `os.Rename` over `meta.yaml`
- [ ] preserve `0644` permissions and `MkdirAll` behavior
- [ ] write test: successful save leaves no `.tmp` file behind
- [ ] write test: save over an existing file replaces content fully (no partial merge)
- [ ] run tests - must pass before next task

### Task 7: Mechanical split of model.go

**Files:**
- Modify: `internal/model/model.go`
- Create: `internal/model/mode.go`
- Create: `internal/model/commands.go`
- Create: `internal/model/render.go`
- Create: `internal/model/textutil.go`

- [ ] move per-mode key handlers (`updateNoteEdit`, `updateTagsEdit`, `updateTrackInput`, `updateUntrackConfirm`, `updateRenameInput`, `updateAPIStatus`, `trackTool`, `renameTool`) to `mode.go` — **no logic changes**
- [ ] move all `tea.Cmd` constructors and fetch predicates (`fetch*Cmd`, `remoteCmd`, `changelogCmd`, `validateTokenCmd`, `needsInstalled`, `needsRemote`, `refreshSelectedCmd`, `autoFetchCmdsForSelected`) to `commands.go`
- [ ] move `View`, all `render*` functions, gauge/scrollbar/mouse helpers to `render.go`
- [ ] move pure text/format helpers (`wrapText`, `stripMarkdown`, `stripANSI`, `cleanTerminalOutput`, `colorizeHelp`, `findMatches`, `highlightMatch`, `formatStars`, `languagePercents`, `renderLangBar`) to `textutil.go`
- [ ] verify `model.go` retains only: struct, msg types, `New`, `Init`, `Update`, selection/filter helpers; confirm with `git diff --stat` that the change is move-only
- [ ] run `go build ./... && go vet ./... && go test -race ./...` - existing render/update tests are the safety net; must pass before next task

### Task 8: Replace boolean mode flags with a single mode enum

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/mode.go`
- Modify: `internal/model/render.go`
- Modify: `internal/model/render_test.go`
- Create: `internal/model/mode_test.go`

- [ ] verify mutual exclusivity of the nine booleans before collapsing: audit every place that sets one flag to confirm no pair can be true simultaneously (if a pair can co-exist, the enum changes behavior — stop and record it here with ⚠️)
- [ ] define `inputMode` enum in `mode.go`: `modeNormal`, `modeSearch`, `modeEditNote`, `modeEditTags`, `modeTrack`, `modeConfirmUntrack`, `modeRename`, `modeHelpSearch`, `modeAPIStatus`, `modeTokenInput`; `refreshingFor`/`helpMode`/`focus` are NOT input modes and stay as separate fields
- [ ] replace `editingNote`/`editingTags`/`tracking`/`confirmingUntrack`/`renaming`/`showingAPIStatus`/`enteringToken`/`searching`/`helpSearching` booleans with one `mode` field; `modeTokenInput` exits back to `modeAPIStatus` on esc/success
- [ ] rewrite the `tea.KeyMsg` early-branch chain in `Update()` as a single `switch m.mode`; the `[L]` guard becomes `m.mode == modeNormal`
- [ ] update `renderStatusBar()` and gauge visibility checks to switch on `m.mode`
- [ ] update existing tests in `render_test.go` that set the old booleans
- [ ] write `mode_test.go`: transition tests for each mode (enter via key, exit via esc, commit via enter) — this closes the 0% gap on the input handlers
- [ ] write tests for guard behavior: `[L]`/`[t]`/`[r]` ignored while another mode is active
- [ ] run `go test -race ./...` - must pass before next task

### Task 9: Repo hygiene and CLAUDE.md actualization

**Files:**
- Modify: `CLAUDE.md`
- Delete: `schema.json`
- Delete: `plan/` (entire directory)

- [ ] delete `schema.json` and `plan/` (superseded by `docs/plans/completed/`)
- [ ] rewrite CLAUDE.md loader/architecture sections: remove embedded-config (`//go:embed data/tools`), `config.yaml`, validation and "Adding a new built-in tool" content; describe the actual meta.yaml-only flow
- [ ] fix the file-storage table (drop "Built-in tool configs" / "User tool configs" rows)
- [ ] document the new `internal/model` file layout and the `inputMode` enum in CLAUDE.md
- [ ] add a CI note (ci.yml: build/vet/test-race/lint) to the Commands section
- [ ] verify every CLAUDE.md claim against the code (spot-check function names and file paths mentioned)

### Task 10: Verify acceptance criteria

- [ ] verify all requirements from Overview are implemented
- [ ] verify edge cases are handled (pre-release versions, crash-safe save, mode transitions)
- [ ] run full test suite: `go test -race ./...`
- [ ] run `go vet ./...` and `golangci-lint run` locally
- [ ] compare coverage to baseline (`loader` ≥ 70%, `version/detect.go` > 0 → covered, `model` ≥ 60%); no package below its baseline

### Task 11: [Final] Update documentation

- [ ] update README.md if any user-visible behavior changed (none expected)
- [ ] confirm CLAUDE.md matches final state (Task 9 done before the split settled — re-verify)
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion

**Manual verification:**
- launch `keys` and walk every mode: search, note/tags edit, track/untrack/rename, `[L]` overlay + token entry, `[r]` refresh, help search — confirm no mode gets stuck after the enum rework
- verify a warm-cache start still renders instantly and a cold start degrades gracefully when rate-limited

**External system updates:**
- first push after merging triggers the new CI workflow — check the Actions run goes green
- next `v*` tag exercises `release.yml` unchanged; CI does not interfere with releases

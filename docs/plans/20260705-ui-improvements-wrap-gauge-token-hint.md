# UI Improvements: List Wrap, API-Usage Gauge, Token Hint

## Overview

Three independent TUI polish changes for `keys`:

1. **Circular list navigation** — moving past the last tool wraps to the first (and vice versa) instead of stopping at the edge.
2. **"Github API Usage" gauge** — a fixed-width yellow fill bar pinned to the right corner of the status bar, always visible in the three normal focus states regardless of which panel is focused. Shows *used / limit* (e.g. `45/60`), independent of whether the limit is 60 (no token) or 5000 (with token). Collapses to a compact form, then hides entirely, on narrow terminals.
3. **Token hint in the API-status overlay** — when no GitHub token is configured, the `[L]` overlay shows a first-line prompt to add one; hidden once a token exists.

All changes are presentation-only in the Bubble Tea model + Lip Gloss styles. No data-flow, cache, or network changes.

## Context (from discovery)

- Files/components involved:
  - `internal/model/model.go` — `Update()` list navigation (`case "j","down"` @502, `case "k","up"` @527), `renderStatusBar()` @1001, `withRateSignal()`/`rateSignal()` @1085/1128, `classifyRate`/`rateIcon` @1112/1144, `renderAPIStatus()` @1236 (its `Limit:` line @1249 renders remaining/limit today).
  - `internal/ui/styles.go` — color constants (`ColorOrange` @9) and styles (`WarnStyle`, `GithubStyle`, `HelpStyle`).
  - `internal/model/render_test.go` — existing status-bar / rate tests (~228–311, ~1077–1095, ~1223).
- Related patterns found:
  - Status bar is per-focus with early `return`s for input/modal modes; the rate signal is currently appended inline via `withRateSignal()`.
  - `classifyRate`/`rateLowThreshold`/`rateIcon` are the single source of truth for rate pressure and are also used by the overlay — must be preserved.
  - `keyHint("x")` renders `[x]` in the prompt style; `lipgloss.Width` measures ANSI-aware width.
- Dependencies identified: `version.RateLimit{Known,Remaining,Limit,Reset}`, `version.TokenSource()` (`"env"|"config"|"none"`), `ui.PlaceOverlay` (unchanged).

## Development Approach

- **testing approach**: Regular (code first, then tests) — matches the project's table-driven `render_test.go` culture.
- complete each task fully before moving to the next
- make small, focused changes
- **every task MUST include new/updated tests** for code changes in that task (success + edge cases)
- **all tests must pass before starting the next task**
- run `go build .`, `go vet ./...`, `go test ./...` after each change
- maintain backward compatibility (no public API changes)

## Testing Strategy

- **unit tests**: required for every task, in `internal/model/render_test.go` (and `internal/ui` if a style helper warrants it).
- **e2e tests**: project has no UI e2e harness (pure terminal TUI); rendering is verified via string-assertion unit tests against `renderStatusBar()` / `renderAPIStatus()` / `Update()`.

## Progress Tracking

- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- keep this plan in sync with actual work

## Solution Overview

- **Navigation**: replace edge-clamp with modular arithmetic in the two `focusTools` branches, guarding against an empty list before the modulo. Shared post-move block (card refresh + `autoFetchCmdsForSelected`) is reused unchanged.
- **Gauge**: introduce `renderRateGauge(maxWidth)` returning the styled indicator in full or compact form (or `""`). `renderStatusBar()` composes the three normal focus lines as `hints` + right-aligned gauge, with a spacer computed from `inner = m.width - 2`; when the gauge does not fit it downgrades full → compact → hidden. The old inline `rateSignal()`/`withRateSignal()` are removed; `classifyRate`/`rateIcon`/`rateLowThreshold` stay (used by the overlay).
- **Token hint**: `renderAPIStatus()` conditionally prepends a highlighted prompt line when `version.TokenSource() == "none"` and not currently entering a token.
- **Consistent used/limit semantics** (decided post-review): the status-bar gauge shows *used/limit* (`45/60`), so the overlay's `Limit:` line is changed to match — `Used: <used> / <limit>` — instead of the current `Limit: <remaining> / <limit>`. Both adjacent surfaces then read the same numbers.
- **Constant yellow, no threshold recolor** (decided post-review): the always-visible bar stays yellow even at exhaustion (`60/60` = full bar). Rate-pressure alarm (`⚠`/`✕`) lives only in the `[L]` overlay via `rateIcon`/`classifyRate`; this is an accepted, conscious trade-off.

## Technical Details

- **Gauge data**: `used = Limit - Remaining`; `ratio = used / Limit` (float, clamp to `[0,1]`); bar is a **fixed 12 cells**; `filled = int(math.Round(ratio * 12))` clamped to `[0,12]`. Rendered only when `m.rate.Known`. Requires importing `math` in `model.go` (not currently imported) — or compute rounding without it (e.g. `(used*12 + Limit/2) / Limit` integer math) to avoid the dependency.
- **Gauge colors (constant yellow, no threshold recolor)**:
  - brackets `[` `]` → foreground `ui.ColorOrange` (`#E5A040`)
  - filled cells → background `ui.ColorOrange`
  - empty track cells → background new `ui.ColorOrangeDim` (`#7A5A1E`)
  - label `GitHub API Usage` → muted; number `used/limit` → orange
- **Gauge forms**:
  - full: `GitHub API Usage [<12-cell bar>] 45/60  [L] details`
  - compact: `GH 45/60 [L]`
  - hidden: `""`
- **Key**: display `[L]` (uppercase) — lowercase `l` is bound to focus-right (`model.go:482`); the existing `L` handler (`model.go:710`) is untouched.
- **Right alignment**: `inner := m.width - 2` (HelpStyle content width). Build `line := hints + strings.Repeat(" ", gap) + gauge` where `gap := inner - lipgloss.Width(hints) - lipgloss.Width(gauge)`. If `gap` below a minimum (e.g. 2), retry with the compact gauge; if still too small, drop the gauge and render hints alone. Then `HelpStyle.Width(inner).Render(line)`.
- **Visibility**: gauge only in the three normal focus branches (`focusTools` default, `focusBrief`, `focusHelp`). All input/modal early returns (`searching`, `helpSearching`, `editingNote`, `editingTags`, `tracking`, `confirmingUntrack`, `renaming`, `showingAPIStatus`, `enteringToken`, non-empty `statusMsg`) are unchanged — no gauge there.
- **Token hint**: rendered right after the `GitHub API status` section label, only when `version.TokenSource() == "none" && !m.enteringToken`; text `Add a GitHub token to raise the limit (60 → 5000/h)  [e]` in `ui.WarnStyle`, followed by a blank line separating it from the `Token:` block.

## What Goes Where

- **Implementation Steps** (checkboxes): all code + tests in this repo.
- **Post-Completion** (no checkboxes): manual visual smoke test across terminal widths.

## Implementation Steps

### Task 1: Circular list navigation

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/render_test.go`

- [ ] in `case "j","down"` (`focusTools` branch, ~model.go:502) replace the `metaSelected < len-1` clamp with `n := len(filtered); if n > 0 { m.metaSelected = (m.metaSelected + 1) % n; <shared refresh block>; return m, m.autoFetchCmdsForSelected() }`
- [ ] in `case "k","up"` (`focusTools` branch, ~model.go:527) replace the `metaSelected > 0` clamp with `n := len(filtered); if n > 0 { m.metaSelected = (m.metaSelected - 1 + n) % n; <shared refresh block>; return m, m.autoFetchCmdsForSelected() }`
- [ ] verify empty-list guard: `n == 0` is a no-op (no modulo, no panic); leave `focusBrief`/`focusHelp` scroll branches and `pgup/pgdn/home/end` clamps untouched
- [ ] add test: down from last index wraps to 0
- [ ] add test: up from index 0 wraps to last index
- [ ] add test: empty/single-item list does not panic and stays put
- [ ] run `go test ./...` — must pass before next task

### Task 2: Add `ColorOrangeDim` and the rate gauge renderer

**Files:**
- Modify: `internal/ui/styles.go`
- Modify: `internal/model/model.go`
- Modify: `internal/model/render_test.go`

- [ ] add `ColorOrangeDim = lipgloss.Color("#7A5A1E")` to the color block in `internal/ui/styles.go`
- [ ] add `renderRateGauge(compact bool) string` (or a form-selecting signature) in `model.go`: returns `""` when `!m.rate.Known`; computes `used/ratio/filled` (fixed 12 cells, clamped); builds the full or compact styled string per Technical Details
- [ ] render brackets in `ColorOrange` fg, filled cells with `ColorOrange` bg, empty cells with `ColorOrangeDim` bg; number in orange, label muted; use `[L]` uppercase hint
- [ ] add test: full form contains `GitHub API Usage`, `45/60`, `[L]`, and a 12-cell bar for `Limit:60, Remaining:15`
- [ ] add test: fixed bar width — same filled-cell count for equal ratios at `Limit:60` and `Limit:5000` (e.g. 25% used)
- [ ] add test: compact form is `GH <used>/<limit> [L]` and shorter than full form
- [ ] add test: exhausted snapshot (`Remaining:0, Limit:60`) → `60/60`, full 12-cell bar, still constant yellow (no danger recolor) — locks in the conscious no-alarm decision
- [ ] add test: `!Known` snapshot yields `""`
- [ ] run `go test ./...` — must pass before next task

### Task 3: Right-align the gauge in the status bar and wire compression

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/render_test.go`

- [ ] in `renderStatusBar()` (~model.go:1001) add a helper that, given the left `hints` string, appends the gauge right-aligned: compute `inner`, `gap`; downgrade full → compact → hidden based on available width; return `HelpStyle.Width(inner).Render(hints + spacer + gauge)`
- [ ] apply it to the three normal focus branches (`focusBrief` ~1067, `focusHelp` ~1071, `focusTools` default ~1075); leave every input/modal early return unchanged
- [ ] remove `withRateSignal()` and `rateSignal()` (inline signal); KEEP `classifyRate`, `rateIcon`, `rateLowThreshold` (used by the overlay)
- [ ] update existing status-bar/rate tests (~render_test.go 261–311, 1077–1095, 1223) to the new gauge format; drop assertions on the removed `⚠ GH`/`✕ exhausted` inline strings
- [ ] add test: wide width → full gauge present and right-aligned (line width ≈ `inner`) — pin to `focusTools` (its ~61-cell hints comfortably fit hints + full gauge, unlike the longer `focusBrief` line) so the assertion is deterministic
- [ ] add test: medium width → compact `GH 45/60 [L]` present, full form absent
- [ ] add test: narrow width → no gauge, hints intact and not truncated below the hint length
- [ ] add test: gauge absent while `searching`/`tracking`/`renaming`/`editingNote`/`editingTags`/`showingAPIStatus` (spot-check a couple of input modes)
- [ ] run `go build . && go vet ./... && go test ./...` — must pass before next task

### Task 4: API-status overlay — used/limit line + token hint

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/render_test.go`

- [ ] in `renderAPIStatus()` (~model.go:1249) change the `Limit: <remaining> / <limit>` line to `Used: <Limit-Remaining> / <Limit>` so it agrees with the gauge's used/limit semantics; keep the `rateIcon` prefix and the `Reset:` line unchanged
- [ ] in `renderAPIStatus()` right after the `GitHub API status` section label, conditionally write the hint line when `version.TokenSource() == "none" && !m.enteringToken`
- [ ] render text `Add a GitHub token to raise the limit (60 → 5000/h)  [e]` in `ui.WarnStyle`, followed by a blank line before the `Token:` block
- [ ] add test: overlay shows `Used: 45 / 60` for `Remaining:15, Limit:60` (not `15 / 60`)
- [ ] add test: overlay contains the hint when no token is set (`TokenSource()=="none"`)
- [ ] add test: overlay omits the hint when a token is present (config/env source) — set up via existing token test helpers
- [ ] add test: hint omitted while `enteringToken` is true
- [ ] run `go test ./...` — must pass before next task

### Task 5: Verify acceptance criteria

- [ ] verify all three Overview features work: list wraps both directions; gauge shows used/limit right-aligned in the three focus states with fixed bar width; overlay hint appears only when no token
- [ ] verify edge cases: empty list, unknown rate (`!Known` → no gauge), very narrow terminal (gauge hidden, no horizontal overflow)
- [ ] run full suite: `go build . && go vet ./... && go test ./...`
- [ ] confirm no test relies on removed `rateSignal`/`withRateSignal`

### Task 6: Update documentation

- [ ] update `CLAUDE.md` if the status-bar rate description (currently "the `focusBrief` bar shows … rate signal" and the GitHub API section's status-bar signal notes) needs to reflect the new right-aligned "GitHub API Usage" gauge, used/limit semantics (bar + overlay), and the overlay token hint
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion

*Informational — no checkboxes*

**Manual verification:**
- Run `go run .` and resize the terminal to confirm the gauge transitions full → compact → hidden cleanly with no horizontal overflow or wrapping of the status bar.
- With and without `GITHUB_TOKEN` set, confirm the gauge shows the correct limit (60 vs 5000) at fixed bar width, and the `[L]` overlay shows/hides the token hint accordingly.

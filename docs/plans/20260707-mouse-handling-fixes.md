# Mouse Handling Fixes: mode gating, click-selection parity, overlay polish

## Overview

Fix the two real defects found in the mouse-handling review, plus the minor
inconsistencies, in `internal/model`, and two visual overlay tweaks:

1. **Mouse bypasses input modes** — `Update()` dispatches `tea.MouseMsg` to
   `handleMouse` before the `m.mode` dispatch (model.go:227), so while a mode
   owns the keyboard the mouse still mutates state underneath. Worst case is
   data corruption: open the note editor (`[e]`) on tool A, click tool B in the
   list, press enter — `updateNoteEdit` reads `selectedMeta()` at commit time
   and saves A's note into B. Same wrong-target class for tags and rename.
   Clicks and wheel also act under the `[L]` overlay and shift selection during
   untrack confirmation.
2. **Click selection is second-class** — keyboard navigation returns
   `m.autoFetchCmdsForSelected()`; a click (render.go:668) does not. Selecting
   an uncached tool by mouse fetches nothing (card stays empty) and the help
   panel keeps showing the previous tool's `--help` even when the new tool's
   help is cached.
3. Minor: clicking an empty area of the tools panel does not focus it (brief
   and help focus on any click); `handleMouse` runs before the first
   `WindowSizeMsg` with zero panel widths; `handleMouse` has 0% test coverage.
4. **Overlay hints are italic** — the `[L]` overlay's hint line
   (`[e] set token  [d] remove token  [r] refresh  [esc] close` and the
   token-input variant `[enter] validate & save  [esc] cancel`) is rendered
   with `ui.MetaNoteStyle` (render.go:316), which carries `Italic(true)`.
   Hints must render without italics.
5. **Background is not dimmed under the overlay** — `ui.PlaceOverlay` strips
   styling only from the cells the modal covers; the rest of the background
   keeps full colors and competes with the overlay. When the overlay opens,
   the whole background should be dimmed (muted monochrome).

**Chosen policy (user decision): wheel — yes, clicks — no.** Wheel scrolling
works in every mode except while the `[L]` overlay is visible (scrolling under
the overlay is invisible); clicks that change selection or focus work only in
`modeNormal`.

## Context (from discovery)

- Files involved: `internal/model/render.go` (`handleMouse` at :658,
  `renderAPIStatus` hint line at :316), `internal/model/model.go` (`MouseMsg`
  dispatch at :227), `internal/model/commands.go` (`autoFetchCmdsForSelected`
  at :182), `internal/ui/overlay.go` (`PlaceOverlay`),
  `internal/ui/styles.go` (`MetaNoteStyle` is italic and shared with the
  brief card's note line — do not de-italicize the shared style; give the
  overlay hints a non-italic style instead).
- The mode enum (`inputMode`, `apiOverlayVisible()` in `mode.go`) and the
  keyboard-selection paths (`j`/`k` handlers returning
  `autoFetchCmdsForSelected()`) are the patterns to align with.
- Geometry (X panel spans, `msg.Y - 2 + YOffset`) verified correct in the
  review — do not touch it.
- Test harness: `mode_test.go` (`newTestModel`, `keyRunes`) drives `m.Update`
  directly; mouse tests can construct `tea.MouseMsg{X, Y, Button, Action}` the
  same way. `t.Setenv("HOME", t.TempDir())` isolates `SaveMeta`.

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
- no on-disk format changes; keyboard behavior unchanged

## Testing Strategy

- **unit tests**: required for every task; drive `m.Update(tea.MouseMsg{...})`
  end-to-end (dispatch + gating + handler), not `handleMouse` in isolation,
  so the mode gate itself is under test
- no e2e framework; the render/update tests in `internal/model` are the e2e
  equivalent

## Progress Tracking

- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- update plan if implementation deviates from original scope
- keep plan in sync with actual work done

## Solution Overview

Gate inside `handleMouse` (not in `Update()`'s dispatch): the function already
knows panels and buttons, so the policy lives next to the behavior it limits.
Order of guards:

1. `!m.ready` → no-op (panel widths are zero before the first `WindowSizeMsg`).
2. `m.apiOverlayVisible()` → no-op (nothing under the overlay may move).
3. Wheel branches stay reachable in every remaining mode.
4. Click branches (selection, focus changes) require `m.mode == modeNormal`.

Click-selection parity: after a valid click on a list row, do exactly what the
keyboard path does — update selection, re-render, and return
`m.autoFetchCmdsForSelected()` (which also re-renders the help viewport for
cached tools and starts fetches for uncached ones). Clicking anywhere in the
tools panel focuses it, matching brief/help.

## Technical Details

- `handleMouse` signature unchanged; `Update()` dispatch unchanged.
- Click on a tools row that is already selected: keep the current "focus only,
  no re-fetch" behavior (guard `m.metaSelected != toolIdx` stays for the
  content work, but `autoFetchCmdsForSelected` fires only on actual change).
- Click on empty tools area (`toolIdx` out of range): focus the panel, keep
  selection — new behavior, aligning with brief/help.
- Wheel under `modeSearch`/`modeHelpSearch`/editing modes: allowed by policy;
  wheel in the tools panel scrolls the viewport without moving selection
  (existing behavior, unchanged).

## What Goes Where

- **Implementation Steps** (`[ ]` checkboxes): code + tests + docs in this repo
- **Post-Completion** (no checkboxes): manual TUI verification

## Implementation Steps

### Task 1: Gate mouse events by input mode in handleMouse

**Files:**
- Modify: `internal/model/render.go`
- Create: `internal/model/mouse_test.go`

- [x] add the guard ladder at the top of `handleMouse`: `!m.ready` → return; `m.apiOverlayVisible()` → return
- [x] restrict the three click branches (tools row select, brief focus, help focus) to `m.mode == modeNormal`; wheel branches stay reachable in all non-overlay modes
- [x] write test: wrong-target repro — in `modeEditNote` a left click on another tools row must not change `metaSelected` (and enter still saves to the original tool)
- [x] write test: clicks in `modeConfirmUntrack`/`modeTokenInput` change neither selection nor focus
- [x] write test: wheel in `modeSearch` scrolls the tools viewport; any mouse event while the overlay is visible is a no-op (viewport offsets and focus unchanged)
- [x] write test: `!m.ready` mouse event is a no-op
- [x] run `go test -race ./...` - must pass before next task

### Task 2: Click selection parity with keyboard navigation

**Files:**
- Modify: `internal/model/render.go`
- Modify: `internal/model/mouse_test.go`

- [x] on a click that changes `metaSelected`, return `m.autoFetchCmdsForSelected()` (same as the `j`/`k` path); keep the no-op cmd when clicking the already-selected row
- [x] focus the tools panel on any left click inside it, including empty area below the list (selection unchanged there)
- [x] write test: click on a different row returns a non-nil cmd and re-renders the help viewport from the new tool's cached help (`GotoTop` + content switch)
- [x] write test: click on an uncached tool sets the changelog/help loading state (same fields the keyboard path sets)
- [x] write test: click on the already-selected row returns nil cmd; click on empty area sets `focus == focusTools` and leaves `metaSelected` intact
- [x] write test: click row mapping still honors `toolsViewport.YOffset` (scroll then click)
- [x] run `go test -race ./...` - must pass before next task

### Task 3: Overlay hint line without italics

**Files:**
- Modify: `internal/model/render.go`

- [ ] render the `[L]` overlay hint line (both variants: normal `[e] set token …` and token-input `[enter] validate & save …`) with the existing `ui.InfoStyle` (non-italic `ColorMuted`, styles.go:126) instead of `ui.MetaNoteStyle`; no new style — and do **not** change `MetaNoteStyle` itself, the brief card's note line keeps its italics
- [ ] write test: `renderAPIStatus()` output contains no italic ANSI sequence (`\x1b[3m`) in the hint line, in both `modeAPIStatus` and `modeTokenInput`
- [ ] run `go test -race ./...` - must pass before next task

### Task 4: Dim the background under the API-status overlay

**Files:**
- Modify: `internal/ui/overlay.go`
- Modify: `internal/ui/overlay_test.go` (or create if missing)

`PlaceOverlay` has a single caller today (the `[L]` overlay, render.go:29), but
dimming becomes part of its contract for any future caller.

**Approach note (from plan review):** the dim must be applied *after* the
ANSI-strip, not before. Overlay-covered rows go through `overlayLine`, whose
`truncateVisible`/`dropVisible` call `StripANSI` on the bg — a pre-applied dim
on the whole bg would be erased from the margins beside the modal, leaving an
undimmed band across exactly the rows the modal occupies. So: dim uncovered
rows via strip-then-restyle, and inside `overlayLine` wrap the `left`/`right`
segments in the dim style after truncation.

- [ ] dim the visible background: uncovered bg rows — strip ANSI, re-render in a muted foreground (`ColorDim`/faint); covered rows — apply the dim style to the `left`/`right` segments inside `overlayLine` (after `truncateVisible`/`dropVisible`), so the modal is the only full-color element on screen
- [ ] keep the fg (overlay) lines byte-for-byte untouched; geometry (centering, splice columns) unchanged
- [ ] write test: `PlaceOverlay` output — bg rows outside the modal carry the dim style and none of their original colors; fg content is preserved verbatim
- [ ] write test: a **covered** row's side margins (cells left/right of the fg splice) carry the dim style and none of their original colors — this is the case the naive pre-dim approach fails
- [ ] write test: alignment unchanged — visible width of every output row equals the bg width (dimming must not shift the splice)
- [ ] run `go test -race ./...` - must pass before next task

### Task 5: Verify acceptance criteria

- [ ] verify all requirements from Overview are implemented (mode gating policy, selection parity, minor fixes, non-italic overlay hints, dimmed background)
- [ ] verify edge cases: filtered list (`modeSearch` off after esc, click uses `filteredMeta` indices), clicks on borders/status bar remain no-ops
- [ ] run full test suite: `go test -race ./...`
- [ ] run `go vet ./...` and `golangci-lint run`
- [ ] `internal/model` coverage does not drop below 68.3% (should rise — `handleMouse` was at 0%)

### Task 6: [Final] Update documentation

- [ ] add a mouse-policy note to CLAUDE.md's TUI state machine section (wheel everywhere except under the overlay; clicks only in `modeNormal`; click selection fires `autoFetchCmdsForSelected`)
- [ ] note in CLAUDE.md's overlay description that `PlaceOverlay` dims the background; also fix the stale claim that search and the changelog popup render via `PlaceOverlay` — the API-status overlay is its only caller (render.go:29)
- [ ] README: no changes expected (behavior matches what a user would already assume)
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion

**Manual verification:**
- walk the TUI with the mouse: click-select an uncached tool (card + help load), click during note editing (selection must not move), wheel-scroll the card while editing a note, open `[L]` and confirm clicks/wheel do nothing underneath
- open `[L]`: hint line is not italic; everything behind the modal is dimmed and the modal reads as the only highlighted element

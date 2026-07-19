# Interactive Help/Man Navigation — spotlight cursor over flags and subcommands

## Overview

Make the `[3]` help/man panel actively navigable (design validated in a brainstorm session):

1. **Entry cursor**: in `focusHelp`, `j`/`k`/`↓`/`↑` move a cursor over *entries* — a flag (or subcommand) line together with its wrapped description lines forms one navigable unit.
2. **Spotlight dimming**: while the cursor is active, every line outside the current entry is repainted dim (strip + repaint, the same trick `overlayLine` uses for the `[L]` overlay); the current entry keeps its full `colorizeHelp` coloring. Reading focuses on one patch of color.
3. **Activation model**: focusing `[3]` alone changes nothing — the text renders full-color as today. The *first* `j`/`k` places the cursor on the first entry visible in the current window and turns dimming on. `esc` turns the cursor off (full color back, scroll position kept); a second `esc` moves focus to `[2]` as today. `PgUp`/`PgDn`/`g`/`G`/mouse wheel stay pure scroll and never touch the cursor.
4. **Read-only**: the cursor is a reading aid — no `enter` action, no clipboard, no section jumps in this version.

Non-goals (explicitly rejected in brainstorm): per-token navigation, a structural document parser, paragraph-based entries, entry markers/glyphs (full-color-on-dim is the highlight; no width math to worry about), cursor wrap-around at list edges (disorienting on multi-screen man pages).

## Context (from discovery)

- `internal/model/render.go:895` — `renderHelpContent()`: the pipeline `rawHelpText → wrapText → colorizeHelp`, with earlier branches for the update log (`updateLogFor`), `helpLoadingFor`, cache-miss placeholders, and the `modeHelpSearch` highlight path. `rawHelpText()` at `render.go:883`.
- `internal/model/textutil.go:221` — `helpTokenRe` (flag/`<meta>`/`[meta]` alternation) and `colorizeHelp` (per-line; section headers are unindented lines ending in `:`). `colorizeHelp` neither adds nor removes lines — wrapped-line indices survive it.
- `internal/model/model.go:715/735` — `j`/`down` and `k`/`up` in `focusHelp` currently scroll `helpViewport` (arrows step 3, `j`/`k` step 1). `esc` for `focusHelp` at `model.go:680` → `setFocus(focusBrief)`. `[h]`/`[m]` branches at `model.go:795/818`. `/` enters `modeHelpSearch` at `model.go:780` (sets mode + returns `textinput.Blink`, no help re-render). `setFocus` at `model.go:1101` only calls `setToolsContent()` — it never touches `helpViewport`.
- `m.helpViewport.SetContent(m.renderHelpContent())` call sites are scattered across `model.go`, `mode.go:272`, `commands.go:234-242` — the entry index must be recomputed at the sites where the *underlying text* changes, not on style-only re-renders. Notable: the `helpOutputMsg` **handler** is in `model.go:414-433` (SetContent at 431; `commands.go:445-447` only constructs the msg); the selection-change re-render lives in `autoFetchCmdsForSelected` (`commands.go:234-242`), whose branches also call `GotoBottom()` (update log) / `GotoTop()` (default) — `selectMeta` itself does not SetContent the help viewport. The ready-branch resize path (`model.go:541-548`) resizes `helpViewport.Width/Height` but today does **not** re-render help content at all (an existing asymmetry vs brief).
- `internal/ui/overlay.go:20` — `OverlayDimStyle` on `ColorDim` (#888888) + the `dimBG` strip-and-repaint precedent. `internal/ui/styles.go:14` — `ColorDim`.
- Tests: `internal/model/render_test.go` (renderHelpContent, colorizeHelp), `internal/model/mode_test.go` (focus/esc walking, help search), `internal/model/mouse_test.go` (wheel scroll). Existing help-search and update-log tests must pass untouched.

Key invariants to preserve:

- `parseHelpEntries` runs on the wrapped lines **before** `colorizeHelp` — plain text, no ANSI to confuse the regex; indices match the rendered content because `colorizeHelp` is line-count-preserving. To guarantee the parser and the renderer wrap identically (renderHelpContent uses `max(m.helpW-2, 20)`), both must go through one shared `wrappedHelpLines()` helper — any divergence desyncs entry indices from rendered lines.
- Two distinct operations on the help viewport: **`setHelpContent()`** (text changed → recompute entries, reset cursor, set content — never scrolls; callers keep their own `GotoTop`/`GotoBottom`) vs a plain style-only re-render (`SetContent(renderHelpContent())` — search highlight toggle, cursor move, per-chunk update-log append) which must *not* reset the cursor or scroll position.
- The update log and every placeholder ("Loading...", "Press [h]…", "No tool selected") produce an empty `helpEntries` — `j`/`k` keep their current scroll behavior there. Only the log *start*/*finish* transitions need `setHelpContent`; per-chunk `updateChunkMsg` renders keep the plain SetContent (entries are already empty, recomputing per chunk buys nothing).
- Entering `modeHelpSearch` and any focus change through `setFocus` reset the cursor (two competing highlights on one text would be noise; spotlight is an attribute of focused reading). Both sites must also re-render the help viewport when they clear an *active* cursor — otherwise the panel stays painted with stale dim (neither path re-renders help today).

## Development Approach

- **Testing approach**: Regular (code first, then tests) — matches repo convention.
- Complete each task fully before moving to the next; small, focused changes.
- **Every task includes new/updated tests** — success and edge cases, run with `go test -race ./...` (the version package has real mutex-guarded state — keep `-race`).
- All tests must pass before starting the next task.
- Update this plan file when scope changes during implementation.

## Testing Strategy

- **Unit tests**: required per task. No e2e framework in this repo; the TUI is covered by model-level tests that drive `Update`/render functions directly (existing pattern in `mode_test.go`/`render_test.go`).
- Static checks: `go vet ./...` and `golangci-lint run` before finishing.

## Progress Tracking

- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- update plan if implementation deviates from original scope

## Solution Overview

Approach A from brainstorm: a **heuristic entry index over the already-wrapped lines** — no structural parser, no pipeline changes. A pure function derives `[]entryRange{start, end}` (half-open line ranges) from the wrapped text; the model stores that slice plus a cursor index (`-1` = off). Rendering appends one step to the existing pipeline: when the cursor is active, lines outside the current range are stripped and repainted dim. The cost of a heuristic miss is only an imperfect highlight boundary — acceptable by design.

## Technical Details

**Entry detection heuristic** (`parseHelpEntries(lines []string) []entryRange`, pure, in `textutil.go`):

- An entry **starts** at a line whose first non-space token is:
  - (a) a flag — reuse the flag core of `helpTokenRe` (`--?[a-zA-Z][a-zA-Z0-9\-_]*`), anchored to "start of trimmed line"; or
  - (b) a subcommand — an indented word **not** starting with `-`, followed by 2+ spaces and description text (typical cobra/clap `commands` block). Requires indentation: unindented prose ("Usage: …", section text) never starts an entry.
- An entry **continues** through following lines indented *deeper* than the start line's indent (description continuations, wrap tails produced by `wrapText`), until the next entry start, a section header (unindented `X:` — same signal `colorizeHelp` uses), or a blank line.
- Section headers, the Usage block, and free prose belong to **no** entry — the cursor skips them; they just dim.
- Empty input → empty slice.

**Model state** (`model.go`): `helpEntries []entryRange`, `helpNavIdx int` (`-1` = off).

**Single recompute point** — new method `setHelpContent()`:
- computes the wrapped plain text via a new shared helper `wrappedHelpLines()` (extracted from `renderHelpContent`: `rawHelpText` + `wrapText` with `max(helpW-2, 20)`; `renderHelpContent` switches to the same helper so wrapping can never diverge), runs `parseHelpEntries`, clears entries when the update-log branch or a placeholder branch would render instead, resets `helpNavIdx = -1`, then `helpViewport.SetContent(m.renderHelpContent())`. It never scrolls — callers keep their surrounding `GotoTop`/`GotoBottom` calls.
- Replaces the existing `SetContent(renderHelpContent())` calls at *text-change* sites: selection change (the `autoFetchCmdsForSelected` switch in `commands.go:234-242`, preserving its per-branch `GotoBottom`/`GotoTop`), `[h]`/`[m]` handlers, `helpOutputMsg` handler (`model.go:431`), rename refresh (`mode.go:272`), update-log start/finish transitions. **Added** (not replaced) in the ready-branch resize path (`model.go:541-548`), which today never re-renders help — this is a deliberate behavior change: help re-wraps on resize, entries recompute, cursor resets. Style-only sites (help-search keystrokes, per-chunk `updateChunkMsg`, cursor moves) keep the plain `SetContent(renderHelpContent())` call.

**Rendering** (`render.go`): after `colorizeHelp`, if `helpNavIdx >= 0` (bounds-checked against `helpEntries`), apply spotlight: for each line outside `[start, end)` → `ui.HelpDimStyle.Render(stripANSI(line))`. New `ui.HelpDimStyle` in `internal/ui` on `ColorDim` — same color as `OverlayDimStyle`, separate name so the reading tint can be tuned later without touching the overlay.

**Keys** (`focusHelp`, `modeNormal` only):
- `j`/`↓`/`k`/`↑` with non-empty `helpEntries`:
  - `helpNavIdx == -1` → set to the first *visible* entry: first entry whose `end > YOffset` (partially visible counts); if the view is scrolled past all entries, the last entry.
  - else step ±1, clamped (no wrap; no-op at the edges).
  - after every change: re-render (style-only) + auto-scroll with **mutually exclusive** branches: `if start < YOffset { SetYOffset(start) } else if end > YOffset+Height { SetYOffset(min(end-Height, start)) }` — the `min` clamp pins a taller-than-window entry's start to the top instead of bottom-aligning it (sequential non-exclusive checks would scroll the start off-screen).
- `j`/`↓`/`k`/`↑` with empty `helpEntries` → current scroll behavior unchanged.
- `esc`: `helpNavIdx >= 0` → reset to `-1`, re-render, keep scroll; else `setFocus(focusBrief)` as today.
- `setFocus()` and the `/` (help search) entry path reset `helpNavIdx = -1`; when the cursor was active (`>= 0`) they also re-render the help viewport (`SetContent(renderHelpContent())`) — neither path re-renders help today, and skipping this leaves stale dim on screen.
- `PgUp`/`PgDn`/`g`/`G`/wheel: untouched — scroll only, cursor stays (possibly off-screen; next `j`/`k` scrolls back to it).

**Status bar** (`renderStatusBar`, `focusHelp` branch — currently `[↑↓] scroll … [←] back … [q] quit`, no esc hint exists): **add** `[j/k] navigate`; while `helpNavIdx >= 0` additionally show `[esc] exit nav`.

## What Goes Where

- **Implementation Steps** (`[ ]` checkboxes): code changes, tests, documentation updates in this repo.
- **Post-Completion** (no checkboxes): manual TUI verification on real tools.

## Implementation Steps

### Task 1: parseHelpEntries — entry detection over wrapped lines

**Files:**
- Modify: `internal/model/textutil.go`
- Modify: `internal/model/textutil_test.go` (create if the file does not exist; entry-parsing tests may also land in `render_test.go` next to the `colorizeHelp` tests — follow whichever file exists)

- [x] add `entryRange struct{ start, end int }` and `parseHelpEntries` to `textutil.go`, implementing the start/continuation heuristic from Technical Details (flag start via the `helpTokenRe` flag core anchored at trimmed-line start; subcommand start = indented non-dash word + 2+ spaces + text; continuation = deeper indent until next start / unindented `X:` header / blank line)
- ➕ signature deviation: `parseHelpEntries(raw string, width int)` parses the **source** (pre-wrap) lines and maps ranges to wrapped display indices via a `wrapLine` helper extracted from `wrapText`. Reason: `wrapText` rebuilds wrapped lines from `strings.Fields`, dropping leading indentation — the indent-based continuation heuristic is unusable on wrapped output. Sharing `wrapLine` preserves the index-sync invariant with the renderer (this also subsumes the `wrappedHelpLines()` width concern: the shared unit is the wrap algorithm itself).
- [x] write table-driven tests on realistic fixtures: clap-style, cobra subcommand block, GNU-style, man-page OPTIONS excerpt (`TestParseHelpEntries` in `render_test.go`)
- [x] write edge-case tests: multi-line description, wrap tails stay inside the entry (`TestParseHelpEntriesWrapMapping` pins positions against actual `wrapText` output), headers/Usage excluded, blank line terminates, empty input → nil, prose-only → nil
- [x] run `go test -race ./internal/model/` — passed

### Task 2: model state + setHelpContent single recompute point

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/commands.go`
- Modify: `internal/model/mode.go`
- Modify: `internal/model/model_test.go` (or the test file matching existing conventions)

- [x] add `helpEntries []entryRange` and `helpNavIdx int` to `Model`; initialize `helpNavIdx = -1` in `New`
- [x] shared wrapping invariant: instead of `wrappedHelpLines()`, a `helpWrapWidth()` method is the single width source (`max(helpW-2, 20)`) used by `renderHelpContent` (both branches) and `setHelpContent`; the wrap *algorithm* is shared via `wrapLine` (Task 1 deviation) — same invariant, smaller surface
- [x] implement `(*Model) setHelpContent()`: `helpEntries = parseHelpEntries(rawHelpText(), helpWrapWidth())` — nil when the update-log branch, `helpLoadingFor`, or no-selection would render (a cache-miss placeholder yields nil via `rawHelpText() == ""`); `helpNavIdx = -1`; then `SetContent(renderHelpContent())`; never scrolls
- [x] replace `SetContent(renderHelpContent())` with `setHelpContent()` at text-change sites: the `autoFetchCmdsForSelected` switch (per-branch `GotoBottom`/`GotoTop` kept), `[h]`/`[m]` handlers (both branches each), `helpOutputMsg` handler, update-log start (`mode.go` confirm-update enter — ➕ note: `mode.go:272` is the log *start*, not a rename site; rename flows through `autoFetchCmdsForSelected`); style-only sites unchanged (help-search keystrokes, per-chunk `updateChunkMsg`, `updateDoneMsg` error seed — log appends with entries already empty)
- [x] **add** `setHelpContent()` to the ready-branch resize path (new behavior: help re-wraps on resize) and switch the `!ready` initial SetContent to it too
- [x] reset `helpNavIdx = -1` in `setFocus()` and on `/` help-search entry; both re-render the help viewport when the cursor was active
- [x] write tests (`mode_test.go`): `TestSetHelpContentEntries`, `TestHelpOutputMsgRecomputesEntries`, `TestSetHelpContentEmptyStates` (update log / loading / cache miss), `TestHelpNavIdxResetTriggers` (selection change, `[h]`↔`[m]`, resize + recompute, focus away via digit, `/` entry)
- [x] run `go test -race ./internal/model/` — full `./...` + `go vet` passed

### Task 3: spotlight rendering — HelpDimStyle + applySpotlight

**Files:**
- Modify: `internal/ui/styles.go`
- Modify: `internal/model/render.go`
- Modify: `internal/model/render_test.go`

- [x] add `ui.HelpDimStyle` (foreground `ColorDim`) to `styles.go` with a comment distinguishing it from `OverlayDimStyle`
- [x] `applySpotlight` in `render.go`, applied to the `colorizeHelp` return (normal path only): repaints every line outside `[start, end)` with `ui.HelpDimStyle.Render(stripANSI(line))`; bounds-checked
- [x] write tests: `TestApplySpotlight` (dim outside incl. no original styling, full color inside incl. flag color, cursor-off has no dim), `TestApplySpotlightStaleIndex` (out-of-bounds → undimmed, no panic), `TestSpotlightClearedOnFocusAway` (viewport repainted undimmed after `2`)
- [x] verified existing help-search highlight tests and update-log render tests pass unchanged
- [x] run `go test -race ./internal/model/ ./internal/ui/` — passed

### Task 4: navigation keys + auto-scroll

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/mode_test.go`

- [x] `helpNavStep(delta)` + `firstVisibleEntry()` + `scrollToNavEntry()` in `model.go`; the `j`/`down` and `k`/`up` `focusHelp` branches route to `helpNavStep(±1)` when `helpEntries` is non-empty, plain scroll otherwise
- [x] auto-scroll with mutually exclusive branches and the `min(end-Height, start)` clamp
- [x] `esc` in `focusHelp`: cursor active → reset + re-render, scroll untouched; cursor off → `setFocus(focusBrief)` as today
- [x] `PgUp`/`PgDn`/`g`/`G`/wheel paths untouched — they never reference `helpNavIdx`
- [x] tests: `TestHelpNavFirstPress` (top / scrolled / past-all-entries), `TestHelpNavEdges` (no wrap), `TestHelpNavEscSemantics` (reset + scroll kept, second esc → `[2]`), `TestHelpNavAutoScroll` (down, up, tall entry pins start), `TestHelpNavEmptyEntriesScrolls`
- [x] run `go test -race ./internal/model/` — passed

### Task 5: status-bar hints

**Files:**
- Modify: `internal/model/render.go`
- Modify: `internal/model/render_test.go`

- [x] `focusHelp` hints: `[↑↓] scroll` is **replaced** by `[j/k] navigate` when `helpEntries` is non-empty (the scroll label would lie — arrows drive the cursor then); `[esc] exit nav` prepended while `helpNavIdx >= 0` (➕ deviation from "add": replacement keeps the label truthful and the bar short)
- [x] tests: `TestRenderStatusBarFocusHelp` — scroll label with no entries, `[j/k] navigate` with entries, `[esc] exit nav` only while active
- [x] run `go test -race ./internal/model/` — passed (full `./...`, `go vet`, `golangci-lint` — only 6 pre-existing SA5011 in test files, present on clean main too)

### Task 6: Verify acceptance criteria

- [ ] verify all Overview requirements: entry cursor over flags *and* subcommands, spotlight dimming, first-j/k activation, esc semantics, scroll keys untouched, read-only
- [ ] verify edge cases: empty help, tool with no man page, update log active, help search interleaving, resize mid-navigation, tool switch mid-navigation
- [ ] run full suite: `go test -race ./...`
- [ ] run `go vet ./...` and `golangci-lint run`

### Task 7: [Final] Update documentation

- [ ] update `CLAUDE.md`: extend the TUI state-machine section with the help-navigation behavior (activation, entry heuristic, spotlight, invalidation rules) and the `setHelpContent` vs style-only re-render split
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion

**Manual verification**:
- run `keys` against real tracked tools: `rg` (clap), a cobra CLI (e.g. `gh`), a GNU-style tool, plus `man` mode for each — check entry boundaries feel right, dimming is readable in the terminal's color profile, and navigation on a long man page (multi-screen entries) auto-scrolls sanely
- check a tool with plain-prose help (no flags) — j/k should still scroll, never trap the user

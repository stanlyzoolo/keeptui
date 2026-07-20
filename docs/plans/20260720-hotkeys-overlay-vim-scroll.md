# Hotkeys Help Overlay `[?]` + Vim-Compatible Unified Scrolling

## Overview

Two features validated in a brainstorm session (plan revised after plan-review round 1):

1. **`[?]` hotkeys overlay**: pressing `?` in any focus (modeNormal only) opens a centered, static, two-column overlay listing **every keybinding, annotated with the panel/mode it is active in** (see the full content spec in Technical Details). Styled exactly like the `[L]` API-status overlay (`ui.PlaceOverlay` dimmed background, `OverlayBorder`, `SectionLabelStyle` headers, `keyHint` keys, `InfoStyle` text). Closes with `esc`, `q`, or a second `?`; every other key is a no-op.
2. **Vim-compatible unified scrolling** (single always-on keymap, no vim-mode toggle): make every scroll/paging key explicit and identical across panels, and kill the *hidden* default bubbles viewport keymap that today makes uncaught keys (`d`/`u`/`f`/`b`/`space`/`h`/`l`) scroll implicitly and inconsistently. `space` is **kept** as a page-down synonym in brief/help (it works today via the hidden keymap, collides with nothing, and is pager muscle memory); the single letters `d`/`u`/`f`/`b` are dropped (`u` collides with update/untrack; `d`/`f`/`b` go for consistency with that).

Problems solved: keybindings are currently discoverable only via the status bar (partial) and CLAUDE.md (not user-facing); scrolling behavior differs between keys that should be synonyms (`j`=1 line vs `↓`=3 lines) and includes undocumented behavior leaking from the default viewport keymap (`u` half-pages in `[3]`, `h`/`l` horizontally shift a viewport *after* changing focus).

## Context (from discovery)

- `internal/model/model.go:580-1005` — the `tea.KeyMsg` branch of `Update()`: mode dispatch first (`:583-598`), then the normal-mode key switch (`:697-995`). New `case "?"` goes next to `case "L"` (`:988`). Input modes return before the normal switch, so `?` typed into search/note/token inputs stays text structurally — same guarantee digits and `L` already rely on.
- `internal/model/model.go:554-556` — `viewport.New(...)` for all three viewports; `viewport.New` installs `DefaultKeyMap()` (pager keys: `d`/`u`/`ctrl+d`/`ctrl+u` half page, `f`/`b`/`space`/`pgup`/`pgdown` page, `h`/`l`/`←`/`→` horizontal). Uncaught keys reach it via the fall-through `briefViewport.Update(msg)`/`helpViewport.Update(msg)` at `model.go:997-1001` — verified as the **only** keyboard path into the viewport keymap.
- **Keys that currently work in brief/help *only* via that hidden path** (and must be re-bound explicitly in the same task that removes it): `pgup`/`pgdown`, `ctrl+d`/`ctrl+u`, `ctrl+f`/`ctrl+b` (the `ctrl+f`/`ctrl+b` case at `:802-812` acts only in focusTools; in brief/help they do nothing even today — only `pgup`/`pgdn` page there), `space`.
- Current scroll cases: `j`/`k`/`↓`/`↑` `:747-800` (arrows step 3, letters step 1; in `focusHelp` with `helpEntries` the letters drive the spotlight cursor — **preserved as-is**), `pgup`/`ctrl+b` + `pgdown`/`ctrl+f` `:802-812` (focusTools only), `g`/`G` `:814-826` (brief/help only).
- `internal/model/mode.go` — `inputMode` enum (`:21-33`), `apiOverlayVisible()` (`:37-39`), per-mode handlers; `updateAPIStatus` (`:290`) is the close-key pattern (`esc`/`q`).
- `internal/model/render.go` — `View()` `:19-37` (single `PlaceOverlay` call site `:31-33`), `renderStatusBar()` `:39-149` (per-mode branches, then per-focus hint bars `:121-148`), `keyHint` `:180`, `renderAPIStatus()` `:315` (style reference), `handleMouse` `:836` with the overlay gate at `:841` (`!m.ready || m.apiOverlayVisible()`) and the stale-once-we-add-a-second-overlay comment at `:840` ("Under the `[L]` overlay nothing may move").
- Mouse wheel survives keymap zeroing because `handleMouse` forwards the `MouseMsg` to `viewport.Update`, whose wheel branch is gated on `MouseWheelEnabled` (still `true` after `viewport.New`) — a field entirely separate from `KeyMap`.
- `internal/ui/overlay.go` — `PlaceOverlay` clips fg lines below the bg height and passes fg through untouched; the two-column grid must stay compact (≈ 24 rows × ≤ 76 cols).
- bubbles v1.0.0 viewport methods (verified): `PageDown()/PageUp()`, `HalfPageDown()/HalfPageUp()`, `ScrollDown(n)/ScrollUp(n)`, `GotoTop()/GotoBottom()`.
- Tests: `internal/model/mode_test.go` (mode transitions, `L` overlay open/close, input-mode text integrity), `render_test.go` (status bar incl. `TestRenderStatusBarGauge` at `:1005` — renders the focusBrief bar at width 160 and asserts the full gauge fits; the helpNav suite incl. `TestHelpNavEmptyEntriesScrolls` at `:2947`, which pins the *old* 1-line `j` step and is **intentionally updated** by this plan), `mouse_test.go` (wheel/click policy; `:179` asserts wheel scrolls the tools viewport), `update_test.go`. `logx.SetDirForTesting` seam where logging could fire.

Key invariants to preserve:

- `j`/`k` spotlight navigation in `[3]` when `helpEntries` is non-empty (`helpNavStep`) — untouched; only the *scroll* step of `j`/`k` (no entries) changes from 1 → 3.
- `esc` two-stage behavior in `focusHelp` (`clearHelpNav` first, focus walk second) and `esc`-quits-from-`focusTools`.
- `q` quits in modeNormal; `q` closes the overlay only inside `updateHotkeys` (mirrors `updateAPIStatus`).
- Wheel scrolling in every mode (must keep working after `KeyMap` is zeroed — pinned by a new regression test); all mouse input no-op while *any* overlay is visible; clicks only in `modeNormal`.
- Status-bar width math: `renderHintsBar` drops the rate gauge before hints would collide; hint additions must keep working on narrow terminals (gauge downgrade is the pressure valve, unchanged) — and `TestRenderStatusBarGauge`'s width-160 expectation must be re-verified after the `[?]` hint lands.

## Development Approach

- **testing approach**: Regular (code first, then tests in the same task)
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task — success and error/edge scenarios, listed as separate checklist items
- **CRITICAL: all tests must pass (`go test -race ./...`) before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**
- maintain backward compatibility of documented keys (`h`/`m`/`l`/digits/esc semantics unchanged); no key that works today may go dead, even between tasks — which is why keymap removal and explicit re-binding live in **one** task

## Testing Strategy

- **unit tests**: required for every task; style follows existing `mode_test.go`/`render_test.go`/`mouse_test.go` (build a `Model` via `New(meta)`, feed `tea.KeyMsg`/`tea.MouseMsg`, assert on mode/viewport offsets/rendered strings)
- **e2e tests**: none in this project — the Bubble Tea model tests are the behavioral layer
- the helpNav **spotlight** tests (`TestHelpNavFirstPress`/`Edges`/`EscSemantics`/`AutoScroll`/`ArrowsKeepScrolling`/`StartDirection`, …) must pass **without modification** — that is the signal that scroll unification did not change `j`/`k` semantics in `[3]`. The one plain-scroll-step test, `TestHelpNavEmptyEntriesScrolls` (`render_test.go:2947`, expects `YOffset == 1` after `j`), is the *old behavior pinned* and is deliberately updated to expect `3`.

## Progress Tracking

- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- keep plan in sync with actual work done

## Solution Overview

- **Overlay**: a new `modeHotkeys` input mode twins `modeAPIStatus` — entered via `case "?"` in the normal switch (any focus), handled by `updateHotkeys` in `mode.go`, rendered by `renderHotkeys()` in `render.go`, composited in `View()` via `ui.PlaceOverlay`. A shared `overlayVisible()` helper (`apiOverlayVisible() || m.mode == modeHotkeys`) becomes the single "modal on screen" predicate for both `View()` and `handleMouse`.
- **Scrolling**: zero the viewport keymaps (`vp.KeyMap = viewport.KeyMap{}`), delete the keyboard fall-through into `viewport.Update`, and in the same change bind every scroll key explicitly, so no key regresses even transiently: line scroll step 3 for `j`/`k`/`↓`/`↑` alike; `ctrl+d`/`ctrl+u` half page; `ctrl+f`/`ctrl+b`/`PgDn`/`PgUp`/`space` full page; `g`/`G` gain first/last-tool in `focusTools`. Single letters `d`/`u`/`f`/`b` are deliberately **not** bound (`u` is update/untrack; `d`/`f`/`b` follow for consistency). `h` stays `--help`.

## Technical Details

### Overlay content spec (`renderHotkeys()`)

Layout: title row (with the close hint right-aligned in it — no separate bottom hint line, it costs a row that 80×24 doesn't have), then two columns via `lipgloss.JoinHorizontal(lipgloss.Top, left, gap, right)`. Every group header names the panel/mode the keys belong to (`SectionLabelStyle`); every row is `keyHint(...)` + `InfoStyle` description, key column left-aligned and padded so descriptions line up **within each column**.

**Hard size budget**: ≤ 18 content rows and ≤ 76 cols → ≤ 20 rows with the `OverlayBorder` frame, which is exactly the composited background height at 80×24 (`calcVpHeight = 24-7 = 17`, +2 panel border, +1 status bar = 20; the outer `Margin(1,0)` is applied *after* `PlaceOverlay`). `PlaceOverlay` clips fg rows off the **bottom** (`overlay.go:44`), so anything over budget silently eats the lowest rows — pinned by a Task 2 test that the close hint survives at 80×24.

**Glyph widths**: the arrow glyphs (`←→↑↓`) are East-Asian **Ambiguous** — same as the existing status-bar hints (`keyHint("↑↓")`), *not* the stricter single-width panel-title constraint; column alignment is best-effort under `RUNEWIDTH_EASTASIAN=1`, matching the status bar's accepted behavior.

Group order tells the reading story: what's global → the panel you start in (with its filter flow inline) → the content panels → their shared scrolling. Left column: `Global`, `[1] Tools`. Right column: `[2] Brief`, `[3] Help / Man`, `Scrolling ([2]/[3])`. The sketch below is illustrative (row offsets between the columns are not a layout contract — only the size budget and the per-column alignment are):

```
 Keyboard shortcuts                                       [esc] close

 Global                              [2] Brief
 [1] [2] [3]  focus panel            [o] open repo   [c] changelog
 [←] [→/l]    move focus             [r] refresh     [s] cycle status
 [esc]  back / exit nav / quit       [e] edit note   [t] edit tags
 [q]    quit  ([ctrl+c] anywhere)    [u] run update (when ↑ shown)
 [L]    GitHub API status            [h] --help      [m] man page
 [?]    this help
                                     [3] Help / Man
 [1] Tools                           [j/k]  entry nav (spotlight)
 [j/k] [↑/↓]  select tool (wrap)     [↑/↓]  scroll   [esc] exit nav
 [g/G]        first / last tool      [h/m]  --help / man page
 [PgUp/PgDn] [ctrl+d/u]  page/half   [/] search  [n/N] next (in search)
 [t] track  [u] untrack  [r] rename
 [/] filter by name or tag           Scrolling ([2]/[3])
     [enter] open  [↑/↓] move        [j/k] [↑/↓]  3 lines
     [esc] cancel                    [ctrl+d/u] half  [g/G] top/bottom
                                     [ctrl+f/b] [space] [PgUp/PgDn] page
```

(18 content rows; the `[1] Tools` paging row covers `ctrl+f`/`ctrl+b` too — they alias `PgDn`/`PgUp` everywhere; the filter sub-rows document `modeSearch` inline under `/`.)

Completeness rule: the overlay covers every binding of the normal mode and names its panel; keys of modal states (note/tags/track/rename/confirm dialogs, `[L]` overlay internals, token entry) are documented by their own status-bar branches at the moment they're active and deliberately stay out of the overlay — except the two search modes, which are the modal states users reach for daily (the filter sub-rows under `/` in `[1] Tools`; `/`+`n/N (in search)` in the `[3]` group).

### Other details

**Status bar**: new `modeHotkeys` branch returns `[esc] close`; the three normal focus bars get `keyHint("?")+" keys"` appended after `[q] quit`.

**Half-page selection step in focusTools**: `step := max(m.toolsViewport.Height/2, 1)`, clamped like the existing pgup/pgdown cases (`max(sel-step, 0)` / `min(sel+step, len-1)`), through `m.selectMeta` so auto-fetch/card sync fire.

**`g`/`G` guard shape in focusTools**: `if n := len(m.filteredMeta()); n > 0 { return m, m.selectMeta(0 /* or n-1 */) }` — mirrors the existing wrap-case guards; `selectMeta(-1)` must be unreachable.

**Processing flow for `?`**: `Update()` → normal switch → `case "?"` → `m.mode = modeHotkeys` → next frame `View()` composites `PlaceOverlay(layout, m.renderHotkeys())`; `updateHotkeys` owns all keys until close.

## Implementation Steps

### Task 1: modeHotkeys mode, `?` key, close handler, status-bar branch

**Files:**
- Modify: `internal/model/mode.go`
- Modify: `internal/model/model.go`
- Modify: `internal/model/render.go`
- Modify: `internal/model/mode_test.go`

- [ ] add `modeHotkeys` to the `inputMode` enum (`mode.go`) with a comment (`"?": hotkeys overlay`); add `overlayVisible()` helper next to `apiOverlayVisible()` returning `apiOverlayVisible() || m.mode == modeHotkeys`
- [ ] add `updateHotkeys(msg)` handler in `mode.go`: `esc`/`q`/`?` → `modeNormal`; any other key no-op (return `m, nil`)
- [ ] dispatch `case modeHotkeys: return m.updateHotkeys(msg)` in the mode switch of `Update()` (`model.go:583-598`)
- [ ] add `case "?"` to the normal-mode switch (next to `case "L"`, `model.go:988`): `m.mode = modeHotkeys; return m, nil`
- [ ] add the `modeHotkeys` branch to `renderStatusBar()` (next to the `modeAPIStatus` branch): `[esc] close`
- [ ] write tests: `?` opens `modeHotkeys` from each of the three focuses; `esc`, `q`, and `?` each close back to `modeNormal`; an unrelated key (e.g. `x`, `j`) keeps the overlay open and changes nothing
- [ ] write tests: `?` typed in `modeSearch`, `modeEditNote`, and `modeTokenInput` lands in the textinput as literal text and does not open the overlay; status bar renders `[esc] close` in `modeHotkeys`
- [ ] run `go test -race ./...` — must pass before task 2

### Task 2: renderHotkeys overlay, View compositing, mouse gate, `[?]` hints

**Files:**
- Modify: `internal/model/render.go`
- Modify: `internal/model/render_test.go`
- Modify: `internal/model/mouse_test.go`

- [ ] implement `renderHotkeys()` in `render.go` exactly per the content spec above: two-column static grid, per-group panel annotations, aligned key column, `ui.OverlayBorder` frame, `SectionLabelStyle`/`keyHint`/`InfoStyle` styling, `[esc] close` right-aligned in the title row; **hard budget ≤ 18 content rows × ≤ 76 cols** (≤ 20 rows framed — the 80×24 background height)
- [ ] composite in `View()`: replace the `apiOverlayVisible()` check with `overlayVisible()` and pick the fg by mode (`modeHotkeys` → `renderHotkeys()`, else `renderAPIStatus()`)
- [ ] extend the `handleMouse` gate (`render.go:841`) from `m.apiOverlayVisible()` to `m.overlayVisible()` and reword the `:840` comment from "[L] overlay" to "any overlay"
- [ ] append `keyHint("?") + " keys"` after `[q] quit` in all three normal focus bars (`renderStatusBar` focusTools/focusBrief/focusHelp branches)
- [ ] write tests: `View()` in `modeHotkeys` contains every group header (`Global`, `[1] Tools`, `[2] Brief`, `[3] Help / Man`, `Scrolling`) and a per-panel spot-check key from each group; background dimmed (mirror the existing `[L]` overlay render tests)
- [ ] write test (size budget): at 80×24 the composited `View()` still contains the `[esc] close` hint and the last `Scrolling` row — i.e. the overlay fits the 20-row background and `PlaceOverlay` clipped nothing
- [ ] write tests: mouse click and wheel are no-ops while `modeHotkeys` is open (selection, focus, and viewport offsets unchanged); `[?] keys` hint present in all three normal focus bars
- [ ] re-run `TestRenderStatusBarGauge` (`render_test.go:1005`): the focusBrief bar at width 160 must still fit the full gauge after the `[?] keys` hint lands — bump the test width only if genuinely needed and note it here
- [ ] run `go test -race ./...` — must pass before task 3

### Task 3: unified scrolling — zero the implicit keymap and bind every scroll key explicitly (one atomic change)

*Keymap removal and explicit re-binding are one task on purpose: `pgup`/`pgdn`, `ctrl+d/u`, `space` currently work in brief/help only via the hidden default keymap, and splitting the removal from the re-binding would ship a commit where they are dead.*

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/render_test.go` (the deliberate `TestHelpNavEmptyEntriesScrolls` update)
- Modify: `internal/model/mouse_test.go`
- Create or extend: `internal/model/scroll_test.go` (new home for the scroll-matrix tests; extend `update_test.go` instead if creating a file feels heavier than the repo style)

- [ ] zero the keymap of all three viewports right after `viewport.New` (`model.go:554-556`): `m.toolsViewport.KeyMap = viewport.KeyMap{}` (same for brief/help) with a comment: every keyboard scroll binding lives in `Update()`'s switch — the default pager keymap must never come back (hidden `d`/`u`/`f`/`b`/`space`/`h`/`l` bindings; wheel is `MouseWheelEnabled`, a separate field, and stays on)
- [ ] delete the keyboard fall-through `briefViewport.Update(msg)`/`helpViewport.Update(msg)` at `model.go:997-1001`
- [ ] unify the line step: `j`/`k` in brief/help scroll 3 lines (same as arrows) — drop the `step := 1` / conditional-3 logic in the `j`/`k` cases; `[3]` spotlight nav (`helpEntries` non-empty) stays exactly as-is
- [ ] add `case "ctrl+d"` / `case "ctrl+u"`: brief/help → `HalfPageDown()`/`HalfPageUp()`; focusTools → selection ±`max(toolsViewport.Height/2, 1)` clamped, via `m.selectMeta`
- [ ] extend `case "pgdown", "ctrl+f"` / `case "pgup", "ctrl+b"` (and add `" "` to the pgdown case): keep the focusTools behavior, add brief/help → `PageDown()`/`PageUp()`; `space` pages down in brief/help only (no tools binding — nothing to page there that `ctrl+f` doesn't already do; keep `space` out of focusTools so it can't collide with future list actions)
- [ ] extend `case "g"` / `case "G"`: focusTools → first/last tool with the guard shape from Technical Details; brief/help behavior unchanged
- [ ] update `TestHelpNavEmptyEntriesScrolls` (`render_test.go:2947`) to expect `YOffset == 3` — the old 1-line step is the behavior this plan changes; verify the helpNav **spotlight** tests pass unmodified
- [ ] write tests (scroll matrix): in `focusBrief` and `focusHelp` (entries empty) `j` and `↓` both move `YOffset` by 3 (`k`/`↑` symmetric); `ctrl+d`/`ctrl+u` half-page brief/help and move tools selection by half a page clamped at both ends; `ctrl+f`/`ctrl+b`/`pgup`/`pgdown`/`space` full-page brief/help; `g`/`G` in focusTools land on first/last (card follows via `selectMeta`) and are no-ops on an empty tool list
- [ ] write regression tests (hidden keymap is gone): `u`, `d`, `f`, `b` in `focusHelp` leave `YOffset` unchanged; `h`/`l` leave `XOffset` of brief/help viewports at 0 (no horizontal shift after the focus change)
- [ ] write regression test (wheel survives keymap zeroing): wheel-down over each of tools/brief/help advances that viewport's `YOffset` (extend `mouse_test.go:179`, which covers tools only)
- [ ] run `go test -race ./...` — must pass before task 4

### Task 4: Verify acceptance criteria

- [ ] verify all requirements from Overview are implemented (`?` overlay from every focus with full per-panel key inventory; unified steps; no hidden keymap; `space` kept, `d`/`u`/`f`/`b` unbound)
- [ ] verify edge cases: empty tool list (`g`/`G`/`ctrl+d`), tiny terminal (overlay clipping, hint-bar gauge downgrade), `?` inside every input mode
- [ ] run full suite: `go build . && go vet ./... && go test -race ./...`
- [ ] run `golangci-lint run`
- [ ] verify helpNav spotlight tests and existing mouse/overlay tests unchanged and green (`TestHelpNavEmptyEntriesScrolls` is the one sanctioned update)

### Task 5: [Final] Update documentation

- [ ] CLAUDE.md: overlay compositing section — rewrite the "the API-status overlay is the single `ui.PlaceOverlay` caller" sentence (two callers behind `overlayVisible()`); mouse policy (`overlayVisible()` gate); help-bar section (`[?] keys` hint); new bullet for the `[?]` overlay (modeHotkeys); scrolling-policy paragraph (empty `viewport.KeyMap`, why it must stay empty, unified steps table incl. `space`)
- [ ] README: sync the keybindings table if one exists
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion

**Manual verification**:
- visual pass in a real terminal: overlay centering/dimming at 120×40 and 80×24 (nothing clipped); `RUNEWIDTH_EASTASIAN=1` sanity check — arrow glyphs are East-Asian Ambiguous like the status-bar hints, so column alignment there is best-effort (may drift a cell), not broken
- feel-check the unified 3-line step on a long man page and the half/full-page keys against muscle memory

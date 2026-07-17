# Panel Titles with Focus Hotkeys

## Overview
- Give all three TUI panels inset border titles in the style `┌─ [1] Tools ─…─┐`: `[1] Tools`, `[2] Brief`, `[3] Help` / `[3] Man` (the Help title follows `m.helpMode`).
- Make digits `1`/`2`/`3` focus hotkeys in `modeNormal` — direct jump to the corresponding panel, including the Tools→Help jump the arrow keys can't do in one step.
- The titles document the hotkeys themselves; the status bar is unchanged (no digit hints duplication).

## Context (from discovery)
- Files involved: `internal/model/render.go` (panel renderers, `insetPanelTitle`), `internal/model/model.go` (focus switching in the `modeNormal` key switch), `internal/model/render_test.go`, `internal/model/model_test.go` / `mode_test.go`, `CLAUDE.md`.
- `insetPanelTitle(panel, title, focused)` (render.go:552) already does everything needed: ANSI-safe splice into the top border starting at the 3rd visible cell, focus-aware color (peach when focused), whole-title drop when the panel is too narrow. **It is not modified.** Today only `renderHelp` uses it (`--help` / `man`).
- Focus changes via `esc`/`left`/`right` (model.go ~509–543) refresh viewport content because styling depends on focus; each branch duplicates the "set focus + refresh" logic pairwise.
- Digit keys are unused in the key handling; `keyRunes` helper exists in `mode_test.go` for KeyMsg tests. `render_test.go` has two relevant tests: `TestInsetPanelTitle` (helper-level, unchanged by this plan) and `TestRenderHelpTitle` (asserts the old ` --help `/` man ` titles and *no* title on tools/brief — updated in task 2, not duplicated).
- All title characters (`[`, `]`, digits, latin) are single-width, non-East-Asian-Ambiguous — border width math stays stable (project invariant).

## Development Approach
- **testing approach**: Regular (code first, then tests in the same task)
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
  - tests are not optional - they are a required part of the checklist
  - cover both success and edge scenarios
- **CRITICAL: all tests must pass before starting next task** — `go test -race ./...` (real mutex-guarded state in the version package; keep `-race`)
- **CRITICAL: update this plan file when scope changes during implementation**
- maintain backward compatibility (arrow/esc focus behavior must not change observably)

## Testing Strategy
- **unit tests**: required for every task; table-driven where it fits the existing style
- no e2e infrastructure in this project — TUI behavior is covered by model-level `Update`/render tests, matching existing patterns in `mode_test.go` / `render_test.go`

## Progress Tracking
- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- update plan if implementation deviates from original scope

## Solution Overview
- **Rendering**: `renderTools` / `renderBrief` pipe their rendered panel through the existing `insetPanelTitle` — the exact pattern `renderHelp` uses today. `renderHelp` only changes its title strings. No changes to the splice helper or `ui` package.
- **Focus**: a new `setFocus(f)` helper owns "change focus + refresh viewports". On an actual change it refreshes **all three** viewports (`setToolsContent`, `renderCard`, `renderHelpContent`): a digit jump `1→3` skips a panel, so pairwise refresh logic would be fiddly; full re-render is cheap/local and viewport `SetContent` preserves the scroll offset. Digit cases call it; existing `esc`/`left`/`right` branches migrate to it, collapsing three copies of the refresh logic.
- Digits fire only in `modeNormal` — structurally guaranteed by the `switch m.mode` dispatch (in `modeSearch` etc. digits go to the textinput as query text).
- Mouse path (`handleMouse`) is out of scope — unchanged.

⚠️ **Corrected during review — the two bullets above were wrong as written:**
- `setFocus` refreshes **only** `setToolsContent()`. The premise that all three viewports carry focus-dependent styling is false: `m.focus` is read in exactly one content renderer, `renderLeftContent` (render.go:405, 428). Neither `renderCard` nor `renderHelpContent` — nor any helper they call (`hasUpdate`, `renderChangelogBlock`, `sectionDivider`, `selectedMeta`, `selectedTool`, `colorizeHelp`, `highlightMatch`) — reads it. The focus-dependent border/title is applied by `renderTools`/`renderBrief`/`renderHelp` **around** the viewport at `View` time. Re-rendering the card and the help text on every focus move was pure waste (`colorizeHelp` walks the whole man page on each `←`/`→`).
- The **mouse path was not safely out of scope**. `handleMouse` wrote `m.focus` directly in three places, so a click that moved focus off the tools list left it painted focused (peach selection bar) — a real pre-existing bug, confirmed by rendering with the color profile forced. All three sites now go through `setFocus`; `TestMouseFocusRefreshesToolsList` / `TestMouseFocusBackToToolsRefreshesList` fail against the old code and pass against the new.

## Technical Details
- Titles: `"[1] Tools"`, `"[2] Brief"`, `"[3] Help"` (helpModeHelp) / `"[3] Man"` (helpModeMan). Visual result: `┌─ [1] Tools ─────┐`. On panels too narrow for the label the title is dropped whole (existing `insetPanelTitle` behavior, accepted). Note: the man-mode title deliberately changes casing `man` → `Man` (title-style label, matching `Help`); the updated test must assert the exact new casing.
- `renderTools` / `renderBrief` gain a local `focused` bool (as `renderHelp` has) and end with `return insetPanelTitle(panel, title, focused)`.
- `setFocus` (⚠️ as implemented — see the correction above; the tools list is the only focus-dependent viewport content):

  ```go
  func (m *Model) setFocus(f int) {
      if m.focus == f {
          return
      }
      m.focus = f
      m.setToolsContent()
  }
  ```

- Key switch additions in `modeNormal`: `case "1"` → `m.setFocus(focusTools)`, `case "2"` → `m.setFocus(focusBrief)`, `case "3"` → `m.setFocus(focusHelp)`. No auto-fetch: the selected tool doesn't change, exactly like arrow-based focus moves.
- `esc` keeps its special behavior (quit from `focusTools`); its focus-moving branches and `left`/`right` compute the target and call `m.setFocus(target)`. ⚠️ The targets are named in a `switch m.focus` rather than computed as `m.focus ± 1`: `focusTools/Brief/Help` are explicit `0/1/2` constants (not `iota`), so arithmetic would break silently if they were ever reordered.

## What Goes Where
- **Implementation Steps** (`[ ]` checkboxes): code changes, tests, documentation updates in this repo
- **Post-Completion** (no checkboxes): manual visual verification in a real terminal

## Implementation Steps

### Task 1: setFocus helper + digit hotkeys

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/model_test.go` (or `mode_test.go`, wherever key-dispatch tests fit the existing layout)

- [x] add `setFocus(f int)` helper to `model.go` (early return on same focus; refresh all three viewports)
- [x] add `case "1"/"2"/"3"` to the `modeNormal` key switch calling `setFocus`
- [x] migrate `esc`/`left`/`right` focus branches to compute target + call `setFocus` (esc keeps `tea.Quit` from `focusTools`)
- [x] write test: from `modeNormal`, `keyRunes("3")` → `m.focus == focusHelp` (direct Tools→Help jump), then `keyRunes("1")` → back to `focusTools`
- [x] write test: digit for the already-focused panel is a no-op (focus unchanged)
- [x] write test: in `modeSearch` a digit is consumed as query text, not a focus change
- [x] run `go test -race ./...` — must pass before task 2

### Task 2: Panel titles in render.go

**Files:**
- Modify: `internal/model/render.go`
- Modify: `internal/model/render_test.go`

- [x] `renderTools`: add local `focused`, pipe result through `insetPanelTitle(panel, "[1] Tools", focused)`
- [x] `renderBrief`: same with `"[2] Brief"`
- [x] `renderHelp`: change title strings `"--help"` → `"[3] Help"`, `"man"` → `"[3] Man"` (no other changes)
- [x] update existing `TestRenderHelpTitle` (`render_test.go:520-546`): expect `[3] Help` / `[3] Man` (exact casing) instead of ` --help ` / ` man `, and flip the tools/brief block from "want no title" to asserting the first line contains `[1] Tools` / `[2] Brief`
- [x] run `go test -race ./...` — must pass before task 3

### Task 3.5: Review follow-up (➕ added during review — the Task 3 checks below were signed off by reading the code, not by exercising it, and missed all of this)
- [x] `setFocus`: drop the two useless `SetContent` calls, correct the comment (only `renderLeftContent` reads `m.focus`)
- [x] route `handleMouse`'s three direct `m.focus =` writes through `setFocus` (fixes the stale focused-list bug)
- [x] `esc`/`left`/`right`: name the focus targets instead of `m.focus ± 1` arithmetic
- [x] tests: `TestFocusArrowKeys`, `TestEscWalksFocusThenQuits` — the refactored branches had **zero** coverage before this
- [x] tests: `TestMouseFocusRefreshesToolsList`, `TestMouseFocusBackToToolsRefreshesList` (verified failing against the pre-fix code), `TestPanelTitleFollowsFocus` (covers the focused branch of every panel renderer)
- [x] `forceColor(t)` test helper: focus styling is color-only, and the default test color profile strips it — colorless assertions would pass against any staleness bug
- [x] CLAUDE.md: remove the false "all three viewports depend on focus" claim this plan introduced
- [x] `go test -race ./...`, `go vet ./...` — pass; model coverage 80.1% → 81.1%

### Task 3: Verify acceptance criteria
- [x] verify all requirements from Overview are implemented (three titles, dynamic Help title, 1/2/3 hotkeys, no status-bar changes)
- [x] verify edge cases (⚠️ done by reading + one manual render; the arrow/esc regression claim was **not** actually verified here — Task 3.5 added the missing tests): narrow panel drops the title whole; `TestInsetPanelTitle` (`render_test.go:552`, helper-level) passes unchanged while `TestRenderHelpTitle` is intentionally updated in task 2; arrow/esc focus behavior identical to before the refactor
- [x] run full suite: `go test -race ./...`
- [x] run `go vet ./...` (clean); ⚠️ `golangci-lint run` could not run locally — the installed binary is built with go1.24 while the module targets go1.25 (environment, not this change; CI runs its own lint)

### Task 4: [Final] Update documentation
- [x] CLAUDE.md: rewrite the "Help panel title" bullet to cover all three panel titles (`[1] Tools` / `[2] Brief` / `[3] Help|Man`, same `insetPanelTitle` mechanism)
- [x] CLAUDE.md: add the `1`/`2`/`3` focus hotkeys to the TUI state machine description (focus cycling paragraph)
- [x] move this plan to `docs/plans/completed/`

## Post-Completion
*Items requiring manual intervention — informational only*

**Manual verification:**
- run `keys` in a real terminal: check titles render into the borders, focus color follows `→`/`←` and `1`/`2`/`3`, Help title flips between `[3] Help` / `[3] Man` on `h`/`m`, and a very narrow terminal drops titles without breaking borders

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

## Technical Details
- Titles: `"[1] Tools"`, `"[2] Brief"`, `"[3] Help"` (helpModeHelp) / `"[3] Man"` (helpModeMan). Visual result: `┌─ [1] Tools ─────┐`. On panels too narrow for the label the title is dropped whole (existing `insetPanelTitle` behavior, accepted). Note: the man-mode title deliberately changes casing `man` → `Man` (title-style label, matching `Help`); the updated test must assert the exact new casing.
- `renderTools` / `renderBrief` gain a local `focused` bool (as `renderHelp` has) and end with `return insetPanelTitle(panel, title, focused)`.
- `setFocus`:

  ```go
  // setFocus moves focus to f and re-renders the panels whose
  // focus-dependent styling changed.
  func (m *Model) setFocus(f int) {
      if m.focus == f {
          return
      }
      m.focus = f
      m.setToolsContent()
      m.briefViewport.SetContent(m.renderCard())
      m.helpViewport.SetContent(m.renderHelpContent())
  }
  ```

- Key switch additions in `modeNormal`: `case "1"` → `m.setFocus(focusTools)`, `case "2"` → `m.setFocus(focusBrief)`, `case "3"` → `m.setFocus(focusHelp)`. No auto-fetch: the selected tool doesn't change, exactly like arrow-based focus moves.
- `esc` keeps its special behavior (quit from `focusTools`); its focus-moving branches and `left`/`right` compute the target and call `m.setFocus(target)`.

## What Goes Where
- **Implementation Steps** (`[ ]` checkboxes): code changes, tests, documentation updates in this repo
- **Post-Completion** (no checkboxes): manual visual verification in a real terminal

## Implementation Steps

### Task 1: setFocus helper + digit hotkeys

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/model_test.go` (or `mode_test.go`, wherever key-dispatch tests fit the existing layout)

- [ ] add `setFocus(f int)` helper to `model.go` (early return on same focus; refresh all three viewports)
- [ ] add `case "1"/"2"/"3"` to the `modeNormal` key switch calling `setFocus`
- [ ] migrate `esc`/`left`/`right` focus branches to compute target + call `setFocus` (esc keeps `tea.Quit` from `focusTools`)
- [ ] write test: from `modeNormal`, `keyRunes("3")` → `m.focus == focusHelp` (direct Tools→Help jump), then `keyRunes("1")` → back to `focusTools`
- [ ] write test: digit for the already-focused panel is a no-op (focus unchanged)
- [ ] write test: in `modeSearch` a digit is consumed as query text, not a focus change
- [ ] run `go test -race ./...` — must pass before task 2

### Task 2: Panel titles in render.go

**Files:**
- Modify: `internal/model/render.go`
- Modify: `internal/model/render_test.go`

- [ ] `renderTools`: add local `focused`, pipe result through `insetPanelTitle(panel, "[1] Tools", focused)`
- [ ] `renderBrief`: same with `"[2] Brief"`
- [ ] `renderHelp`: change title strings `"--help"` → `"[3] Help"`, `"man"` → `"[3] Man"` (no other changes)
- [ ] update existing `TestRenderHelpTitle` (`render_test.go:520-546`): expect `[3] Help` / `[3] Man` (exact casing) instead of ` --help ` / ` man `, and flip the tools/brief block from "want no title" to asserting the first line contains `[1] Tools` / `[2] Brief`
- [ ] run `go test -race ./...` — must pass before task 3

### Task 3: Verify acceptance criteria
- [ ] verify all requirements from Overview are implemented (three titles, dynamic Help title, 1/2/3 hotkeys, no status-bar changes)
- [ ] verify edge cases: narrow panel drops the title whole; `TestInsetPanelTitle` (`render_test.go:552`, helper-level) passes unchanged while `TestRenderHelpTitle` is intentionally updated in task 2; arrow/esc focus behavior identical to before the refactor
- [ ] run full suite: `go test -race ./...`
- [ ] run `go vet ./...` and `golangci-lint run`

### Task 4: [Final] Update documentation
- [ ] CLAUDE.md: rewrite the "Help panel title" bullet to cover all three panel titles (`[1] Tools` / `[2] Brief` / `[3] Help|Man`, same `insetPanelTitle` mechanism)
- [ ] CLAUDE.md: add the `1`/`2`/`3` focus hotkeys to the TUI state machine description (focus cycling paragraph)
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion
*Items requiring manual intervention — informational only*

**Manual verification:**
- run `keys` in a real terminal: check titles render into the borders, focus color follows `→`/`←` and `1`/`2`/`3`, Help title flips between `[3] Help` / `[3] Man` on `h`/`m`, and a very narrow terminal drops titles without breaking borders

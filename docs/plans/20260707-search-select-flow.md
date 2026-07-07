# Search select flow: enter to open, arrows to move, esc to rollback

## Overview

Finish the tool-list search chain. Today `/` in `focusTools` filters the list
live, but the flow dead-ends: `enter` is swallowed by the textinput, `↑`/`↓`
edit the input instead of moving the highlight, and `esc` drops the cursor to
the top of the full list, losing the pre-search position.

After this plan, search becomes a commit/rollback transaction:

- **`enter`** accepts the highlighted match: exits search, restores the full
  list with the cursor on the chosen tool, moves focus to the brief (central)
  panel, and fires the auto-fetch path so the card populates.
- **`↑`/`↓`** move the highlight through the filtered list (wrap-around, same
  modular pattern as `j`/`k`), without touching the query text.
- **`esc`** rolls back: full list, cursor restored to the tool selected before
  search started.

## Context (from discovery)

- `internal/model/model.go` — `modeSearch` key branch (~line 431: only `esc`
  handled, everything else goes to `m.search.Update` and resets
  `metaSelected = 0`); `case "/"` (~line 571) enters the mode;
  `filteredMeta()` (~line 736) filters by name only while `m.mode ==
  modeSearch`; `selectedMeta()` indexes into the filtered slice.
- `internal/model/render.go` — status-bar search branch (~line 55, shows only
  `[esc] exit search`); tools panel hides the selection marker during search
  (~line 371: `m.mode != modeSearch`).
- Navigation pattern to mirror: `j`/`k` handler (model.go ~487-534) —
  modular index, `setToolsContent()`, `briefViewport` re-render + `GotoTop`,
  `return m, m.autoFetchCmdsForSelected()`.
- Tests live in `internal/model/mode_test.go` (table-driven `tea.KeyMsg`
  patterns, helpers like `keyRunes` already exist).

## Development Approach

- **testing approach**: Regular (code first, then tests in the same task)
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in
  that task; tests cover both success and error scenarios
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**
- run `go test -race ./...` after each change (version package has real
  mutex-guarded state — keep `-race`)
- maintain backward compatibility of existing key handling

## Testing Strategy

- **unit tests**: required for every task; table-driven where the existing
  file already uses tables. No e2e infrastructure in this project.
- verification commands: `go test -race ./...`, `go vet ./...`,
  `golangci-lint run`.

## Progress Tracking

- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- update plan if implementation deviates from original scope

## Solution Overview

Approach A from the brainstorm (remember-by-name): names are the stable key
this codebase already routes on (`selectedMeta`, caches, `repoCards`), so both
`enter` (map filtered highlight → full-list index) and `esc` (restore
pre-search selection) share one helper instead of juggling two index spaces.

Key decisions:

- `searchPrevName string` on `Model` — captured in `case "/"`, cleared on any
  exit from `modeSearch`; makes `esc` a true rollback.
- `indexOfMeta(name string) int` — index of a tool by name in the full
  `m.meta`, fallback `0` when absent (tool untracked mid-search or empty name).
- All arrow/enter handling stays inside the `modeSearch` branch in
  `Update()` — the mode owns its input, consistent with the input-mode state
  machine (other modes' keys structurally cannot fire).
- Selection marker becomes visible during search because the cursor is now
  user-controlled there.

## Technical Details

Processing flow for `enter` (matches selected):

1. `mt, ok := m.selectedMeta()` — on `!ok` (no matches / empty filter): no-op,
   stay in search.
2. Exit mode: `m.mode = modeNormal`, `m.search.SetValue("")`,
   `m.search.Blur()`, `m.searchPrevName = ""`.
3. `m.metaSelected = m.indexOfMeta(mt.Name)` — filter is gone once mode is
   normal, so the index must be recomputed against full `m.meta`.
4. `m.focus = focusBrief`, `m.setToolsContent()`,
   `m.briefViewport.GotoTop()`, `m.briefViewport.SetContent(m.renderCard())`.
5. `return m, m.autoFetchCmdsForSelected()` — same contract as `j`/`k`, so a
   not-yet-cached card starts fetching.

`↑`/`↓` inside `modeSearch`: `n := len(m.filteredMeta())`; when `n > 0`,
`m.metaSelected = (m.metaSelected ± 1 + n) % n`, then full `j`/`k` parity:
`m.setToolsContent()`, `m.briefViewport.Height = m.calcVpHeight()`,
`m.briefViewport.GotoTop()`, `m.briefViewport.SetContent(m.renderCard())`,
`return m, m.autoFetchCmdsForSelected()`. The key is **not** forwarded to
`m.search.Update`, so the query text is untouched. When `n == 0` the keys are
consumed as no-ops (still not forwarded — the textinput must never see
arrows).

`esc`: as today (clear query, blur, re-render) but
`m.metaSelected = m.indexOfMeta(m.searchPrevName)` instead of `0`, then clear
`searchPrevName`.

Typing (default branch): unchanged — query updates and `metaSelected` resets
to 0 (first match highlighted), which now shows visibly via the marker.

## Implementation Steps

### Task 1: Search transaction state + enter/arrows/esc handling

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/mode_test.go`

- [x] add `searchPrevName string` field to `Model` (near `search`), with a
      comment stating the commit/rollback contract
- [x] add `indexOfMeta(name string) int` helper next to `filteredMeta()`
      (full-list lookup, fallback 0)
- [x] `case "/"` (focusTools branch): capture `m.searchPrevName` from
      `selectedMeta()` before entering `modeSearch` (empty string when list
      is empty)
- [x] `modeSearch` branch: add `enter` (accept: exit mode, remap cursor via
      `indexOfMeta`, focus brief, re-render, `autoFetchCmdsForSelected()`;
      no-op when no matches) and `up`/`down` (modular move over
      `filteredMeta()` with full `j`/`k` parity incl. `calcVpHeight()`;
      never forwarded to textinput)
- [x] `modeSearch` `esc`: restore cursor via `indexOfMeta(m.searchPrevName)`,
      clear `searchPrevName`
- [x] write tests: enter selects highlighted tool (cursor points at the right
      tool in the **full** list, `focus == focusBrief`, `mode == modeNormal`,
      query cleared — assert observable state, not cmd non-nil-ness)
- [x] write tests: esc restores the pre-search selection (move selection
      before `/`, type a query, esc → original tool selected again)
- [x] write tests: enter with zero matches is a no-op (stays in `modeSearch`)
- [x] write tests: `↑`/`↓` navigate the filtered list with wrap-around and do
      not change `m.search.Value()`
- [x] write tests: `indexOfMeta` table test (found, missing → 0, empty name
      → 0); a letter key (e.g. `j`) in `modeSearch` lands in
      `m.search.Value()` and does not move `metaSelected`
- [x] run `go test -race ./...` — must pass before task 2

### Task 2: Render polish — visible selection marker + status-bar hints

**Files:**
- Modify: `internal/model/render.go`
- Modify: `internal/model/render_test.go`

- [x] `renderToolsList` (~line 371): show the selection marker during
      `modeSearch` (drop the `m.mode != modeSearch` condition)
- [x] status-bar `modeSearch` branch (~line 55): **keep** the live query echo
      (`ui.SearchPromptStyle.Render("/")` + `m.search.View()`) and append the
      hints via `keyHint(...)` — `enter` open, `↑/↓` move, `esc` cancel
- [x] write/extend render tests: marker visible for the highlighted row while
      in `modeSearch`; status bar contains the new hints and still echoes the
      query; existing `modeSearch` gauge case keeps passing
- [x] run `go test -race ./...` — must pass before task 3

### Task 3: Verify acceptance criteria

- [ ] verify all requirements from Overview are implemented (enter → brief
      panel + full list + cursor on tool; arrows move highlight; esc rollback)
- [ ] verify edge cases: empty tool list, zero matches, tool untracked
      between search open and esc (fallback 0), single-match wrap-around
- [ ] run full suite: `go test -race ./...`
- [ ] run `go vet ./...` and `golangci-lint run`

### Task 4: [Final] Update documentation

- [ ] update the TUI state machine section of `CLAUDE.md` (search is now a
      commit/rollback transaction: enter/arrows/esc semantics)
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion

**Manual verification:**
- run `keys` in a real terminal: `/` → type a partial name → `↓`/`↑` to move
  the highlight → `enter` lands on the brief panel with the full list and the
  cursor on the chosen tool; `esc` mid-search returns to the pre-search tool;
  card auto-fetches for a freshly selected tool with no cached data.

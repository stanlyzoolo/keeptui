# Tools List Revision — ⏺ cursor, drop status edge, group updatable tools first

## Overview

Revise the left tools list (design validated in a brainstorm session):

1. **New selection cursor**: replace the `▸` marker with `⏺` (U+23FA, BLACK CIRCLE FOR RECORD) — peach (`SelectionBarStyle`) while the tools panel is focused, dim (`SelectionBarDimStyle`) otherwise. Width facts (measured via go-runewidth in this module): `⏺` is width 1 in the default condition lipgloss uses, but East-Asian **Ambiguous** — width 2 under `RUNEWIDTH_EASTASIAN=1`. This is consciously accepted: the existing `↑` suffix *and* the `▎` edge being removed are equally Ambiguous, so the change does not regress the list (CLAUDE.md's "non-East-Asian-Ambiguous" wording was already inaccurate for `↑`/`▎` and must be corrected, not copied). A width test pins default width 1 and documents the ambiguity.
2. **Remove the `▎` status edge** for both `trying` and `inactive`. The user disliked the look; tool status stays visible (and editable via `s`) in the brief card only.
3. **No up-to-date checkmark**: considered and rejected — the ` ↑` suffix alone marks outdated tools; absence of `↑` reads as "nothing to do". The marker column carries only the `⏺` cursor.
4. **Group tools with an available update at the top of the list**: a display-only stable partition — `hasUpdate` tools first, then the rest, `meta.yaml` order preserved inside each group. `meta.yaml` on disk is never re-sorted.

## Context (from discovery)

- `internal/model/render.go` — `renderLeftContent` (marker column: `▸` branches + `statusEdge` default), `statusEdge` func, `hasUpdate`, `setToolsContent`/`syncToolsViewport`.
- `internal/model/model.go` — `searchMatches()` (single projection point feeding both `renderLeftContent` and `filteredMeta()`), `filteredMeta()`, `indexOfMeta()` (currently iterates full `m.meta`), `selectMeta`, `installedMsg`/`remoteMsg` handlers in `Update()` (merge into `m.versions` → can flip `hasUpdate`).
- `internal/ui/styles.go` / `status.go` — `SelectionBarStyle`/`SelectionBarDimStyle` stay as-is (only the glyph changes); `StatusStyleTrying`/`StatusStyleInactive` are **kept** — `ui.StatusStyle` uses them for the brief card's status line; only the list edge goes away.
- Tests: `internal/model/render_test.go` (marker/edge expectations), `internal/model/mode_test.go` (search flow), `internal/model/mouse_test.go` (click row mapping).
- `CLAUDE.md` — "Tool-list rows" paragraph describes `▸`/`▎` and must be rewritten.

Key invariants to preserve:

- Sorting must live in `searchMatches()` so every consumer (renderer, `filteredMeta`, selection index semantics, mouse row mapping) sees one order.
- `indexOfMeta` must switch from `m.meta` to the sorted projection (`filteredMeta()`), or the search commit (`enter`, query cleared before the call) and rollback (`esc`) land on the wrong row.
- Async cursor stability: `installedMsg`/`remoteMsg` can reorder rows mid-session; the cursor must follow the *tool*, not the row index — capture the selected name before the merge, remap `m.metaSelected` via `indexOfMeta` after, refresh via `setToolsContent()` (syncs viewport YOffset). No auto-fetch on remap (do **not** go through `selectMeta`).

## Development Approach

- **Testing approach**: Regular (code first, then tests) — matches repo convention.
- Complete each task fully before moving to the next; small, focused changes.
- **Every task includes new/updated tests** — success and edge cases, run with `go test -race ./...` (the version package has real mutex-guarded state — keep `-race`).
- All tests must pass before starting the next task.
- Update this plan file when scope changes during implementation.

## Testing Strategy

- **Unit tests**: required per task (see above). No e2e framework in this repo; the TUI is covered by model-level tests that drive `Update`/render functions directly.
- Static checks: `go vet ./...` and `golangci-lint run` before finishing.

## Progress Tracking

- Mark completed items with `[x]` immediately when done.
- Add newly discovered tasks with ➕ prefix; blockers with ⚠️ prefix.
- Keep the plan in sync with actual work.

## Solution Overview

All changes are display-layer only — no persistence or fetch-path changes:

- **Marker column** (width 1, "one glyph, one meaning" kept): `⏺` on the selected row, plain space everywhere else. `statusEdge` and its branch are deleted.
- **Order** is a pure projection: `searchMatches()` filters (as today) and then stable-partitions the result — `hasUpdate` first. Two-pass append keeps it stable and allocation-cheap. **Critical (from plan review):** `filteredMeta()` does *not* inherit this for free — it has an empty-query fast path (`return m.meta`) that bypasses `searchMatches()` entirely. That fast path must be removed (route through `searchMatches()` unconditionally) or the renderer (grouped) and the whole selection/mouse system (ungrouped `m.meta`) see different row orders in normal mode and every click/cursor targets the wrong tool.
- **Cursor stability**: the two version-merge handlers remap the selection by name after the merge. During `modeSearch` the projection is the filtered list; membership cannot change from a version message (the filter is name/tag-based), only order — so the remap is safe there too.

## Technical Details

- `⏺` U+23FA: width 1 in go-runewidth's default condition (what lipgloss uses), East-Asian Ambiguous → 2 under `RUNEWIDTH_EASTASIAN=1`; emoji-capable but rendered in text presentation (no VS16 appended). Same ambiguity class as the existing `↑` and the removed `▎` — accepted, no regression. The width test asserts default width 1 via `runewidth` and carries a comment documenting the Ambiguous classification (a bare `lipgloss.Width == 1` check cannot detect ambiguity and must not claim to).
- Partition predicate is exactly `m.hasUpdate(mt.Name)` (installed known, latest known, installed older per `version.IsNewer`) — the same predicate that renders ` ↑`, so the group and the suffix can never disagree.
- `indexOfMeta` iterates `m.filteredMeta()`; its "fallback 0 when absent" contract is unchanged.
- Remap in `installedMsg`/`remoteMsg` handlers: read the selected name via `selectedMeta()` *before* mutating `m.versions`, then `m.metaSelected = m.indexOfMeta(name)` and `m.setToolsContent()` (replaces the bare `m.toolsViewport.SetContent(m.renderLeftContent())`). Skip the remap when the list is empty (`selectedMeta()` returns `false`).
- Mouse: `handleMouse` maps click Y → row index into the same projection; verify it resolves the clicked tool through `searchMatches()`/`filteredMeta()` order (it should already — one projection point), cover with a test.

## What Goes Where

- **Implementation Steps**: code, tests, docs — all in this repo, in the current worktree (branch `worktree-tools-list-update-grouping`).
- **Post-Completion**: visual check in a real terminal.

## Implementation Steps

### Task 1: Replace `▸` cursor with `⏺`

**Files:**
- Modify: `internal/model/render.go`
- Modify: `internal/model/render_test.go`

- [x] swap the glyph in both selected-row branches of `renderLeftContent` (`SelectionBarStyle.Render("⏺")` / `SelectionBarDimStyle.Render("⏺")`); update the marker-column comment
- [x] update every `▸` expectation in `render_test.go` (`TestRenderLeftContent*`, marker-survives-focus, selected-row-priority tests)
- [x] add a width-guard test asserting default-condition width 1 for `⏺` (and `↑`) via go-runewidth, with a comment stating both are East-Asian Ambiguous (2 cells under `RUNEWIDTH_EASTASIAN=1`) — accepted, same class as the `▎` being removed
- [x] run `go test -race ./internal/model/...` — must pass before task 2

### Task 2: Remove the `▎` status edge

**Files:**
- Modify: `internal/model/render.go`
- Modify: `internal/model/render_test.go`
- Modify: `CLAUDE.md`

- [x] delete the `statusEdge` func; the `default` branch of the marker switch becomes a plain space
- [x] delete/adjust `render_test.go` cases that assert the edge (`▎` for trying/inactive); keep the "cursor takes priority on the selected row" case, now asserting `⏺` over plain space
- [x] confirm `ui.StatusStyleTrying`/`StatusStyleInactive` are still referenced (brief card via `ui.StatusStyle`) — no ui-package changes
- [x] update the CLAUDE.md "Tool-list rows" paragraph (no status edge; `⏺` cursor) — and correct the "single-cell and non-East-Asian-Ambiguous" claim: `⏺` and `↑` are single-cell in the default condition but East-Asian Ambiguous (the old wording was already wrong for `↑`/`▎`)
- [x] run `go test -race ./internal/model/...` — must pass before task 3

### Task 3: Group updatable tools first (projection in `searchMatches`)

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/render_test.go` (or `model_test` additions in the file where list tests live)

- [x] stable-partition the result of `searchMatches()`: two-pass append — `m.hasUpdate(mt.Name)` rows first, others second, filter logic untouched
- [x] **remove the empty-query fast path in `filteredMeta()`** (`if m.searchQuery() == "" { return m.meta }`) so it always projects through the grouped `searchMatches()` — without this the renderer and the selection/mouse system desync in normal mode (critical plan-review finding)
- [x] switch `indexOfMeta` to iterate `m.filteredMeta()` instead of `m.meta`; update its comment (it now answers "index in the *displayed* order")
- [x] update `TestIndexOfMeta` (mode_test.go): fix its stale "full-list name lookup" comment and add a case where an updatable tool is grouped ahead of a `meta.yaml`-earlier tool, asserting the *displayed* index is returned
- [x] write tests: updatable tools render above the rest with `meta.yaml` order preserved inside each group; `m.meta` slice order is untouched by rendering
- [x] write test: grouping applies inside an active search filter too
- [x] write test: search commit (`enter`) and rollback (`esc`) land on the right tool with grouping active (via `indexOfMeta` on the projection)
- [x] run `go test -race ./internal/model/...` — must pass before task 4

### Task 4: Cursor follows the tool on async reorder

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/render_test.go` (message-handler tests live with the model tests)

- [x] `installedMsg` handler: capture selected name via `selectedMeta()` before merging into `m.versions`; after the merge remap `m.metaSelected = m.indexOfMeta(name)` and call `m.setToolsContent()` (replaces the bare `SetContent`); skip remap when the list is empty
- [x] `remoteMsg` handler: same capture/remap/`setToolsContent()` in the data-merging branch
- [x] write test: with tool B selected, a `remoteMsg` that lifts tool C above B keeps the selection on B (name-stable, index changed)
- [x] write test: a `remoteMsg` that lifts the *selected* tool to the top keeps it selected at its new index
- [x] write test: `installedMsg`/`remoteMsg` on an empty list does not panic
- [x] run `go test -race ./internal/model/...` — must pass before task 5

### Task 5: Verify mouse click row mapping against the projection

**Files:**
- Modify: `internal/model/mouse_test.go`
- Modify (only if a mismatch is found): `internal/model/render.go`

- [x] confirm `handleMouse` resolves the clicked row via `m.filteredMeta()` (render.go) — **depends on the Task 3 fast-path fix**: `handleMouse` itself needs no change once `filteredMeta()` is grouped; today they agree only because both collapse to `m.meta` with no query
- [x] write test: with grouping reordering rows, a click on row 0 selects the updatable tool shown there, not the first `meta.yaml` entry
- [x] run `go test -race ./internal/model/...` — must pass before task 6

### Task 6: Verify acceptance criteria

- [x] verify all four Overview items are implemented (⏺ cursor both focus states, no `▎`, no checkmark, updatable tools grouped on top)
- [x] verify edge cases: empty list, all tools up-to-date (no reorder), tool with no GitHub ref, mid-search reorder
- [x] run full suite: `go test -race ./...`
- [x] run `go vet ./...` and `golangci-lint run`

### Task 7: [Final] Update documentation

- [x] re-check CLAUDE.md fully reflects the new list behavior (marker column, grouping projection, `indexOfMeta` semantics, remap-on-message)
- [x] move this plan to `docs/plans/completed/`

## Post-Completion

**Manual verification:**
- run `go run .` in a real terminal: confirm `⏺` renders single-cell in the user's font, updatable tools sit on top, and the selection visibly follows a tool when async fetches reorder the list on a cold start
- optional: `RUNEWIDTH_EASTASIAN=1 go run .` to confirm the width math holds under East-Asian-Ambiguous terminals

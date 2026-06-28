# TUI track / untrack / rename

## Overview
- Move tool tracking out of the CLI and into the Bubble Tea TUI. The CLI `track`/`untrack`
  subcommands are removed entirely; tracking is managed from the left tool-list panel
  (`focusTools`).
- Problem it solves: today adding/removing a tracked tool requires dropping to the shell
  (`keys track …` / `keys untrack …`). That is a context switch away from the viewer where
  the tracked list actually lives.
- New left-panel actions: `t` track (add by GitHub URL or plain name), `u` untrack (with
  confirmation), `r` rename (fix the binary name when the repo name differs, e.g.
  `claude-code` → `claude`). The `focusTools` help bar is decluttered to
  `[/] search · [t] track · [u] untrack · [r] rename · [q] quit`, and the now-irrelevant
  list actions `f` (status filter), `v` (manual version check) and `o` (open GitHub) are
  removed.

## Context (from discovery)
- Files/components involved:
  - `internal/model/model.go` — all TUI state, key handling, status bar, rendering.
  - `internal/cmd/track.go`, `internal/cmd/untrack.go` — CLI subcommands to delete.
  - `main.go` — subcommand dispatch (`track` at lines 73-74, `untrack` at 82-83; usage text
    at lines 17, 20).
  - `internal/loader` — `ParseToolRef`, `UpsertMeta`, `RemoveMeta`, `FindMeta`, `SaveMeta`,
    `TodayDate` (all reused; `ParseToolRef`/`NormalizeRepo` in `internal/loader/github.go`).
- Related patterns found:
  - Input-mode pattern to mirror: `editingNote`/`editingTags` each own a `textinput.Model`
    (`noteInput`, `tagsInput`), with enter/esc handling in `Update()` and a dedicated render
    branch in `renderStatusBar()` (~line 740). Other inputs: `search`, `helpSearch`.
  - Focus states: `focusTools=0` (left), `focusBrief=1`, `focusHelp=2`.
  - Left list is driven by `m.meta []loader.ToolMeta`; `model.New(meta, opts)`. **But the
    brief/help panels are driven by `m.tools`** (built in `New()` via
    `loader.ToolsFromMeta(meta)`, loader.go:11; field assigned at model.go:141).
    `selectedTool()` matches by `Name` against `m.tools`. So adding/renaming a tracked tool
    means updating `m.meta` **and** rebuilding `m.tools = loader.ToolsFromMeta(m.meta)`, then
    re-rendering via `m.toolsViewport.SetContent(m.renderLeftContent())`.
- Dependencies identified:
  - `man`/`--help`/version detection use `ToolMeta.Name` as the executable name
    (`fetchHelpCmd` ~line 1486; `internal/version/detect.go`), so `Name` must be the binary
    name — this is exactly why `rename` exists.
  - Async data for a tool (version, repo card, changelog, help) is fetched by `Init()` once at
    startup and by `m.autoFetchCmdsForSelected()` on selection change (model.go ~412/436/458).
    A freshly tracked/renamed tool has none of it loaded, so track/rename must return
    `m.autoFetchCmdsForSelected()` (or an equivalent batched `tea.Cmd`) for the affected tool.
  - `metaFilter` (field at line 102) is read in `filteredMeta()` (~700-703) and
    `renderStatusBar()` (~784-787); removing the filter feature means removing all of these.
    With it gone, the `else` empty-state branch in `renderLeftContent` (~870-874, "No tools
    match current filter.") becomes dead and should be removed too.
  - `case "v"` (line 573) drives `checkVersionMsg` (43, 226, `fetchVersionCmd` ~1280) and the
    `checkingVersionTool` field (84, refs 227-229, 574-576); `case "o"` (581) calls
    `openBrowser` (1135). Removing them must NOT touch the separate async version fetch on
    `Init` (`versionMsg`, handler 214-224), which stays. The `checkingVersionTool` field is an
    unused-field trap: Go won't fail the build, so it must be deleted explicitly.
  - In-TUI strings still tell users to run the CLI: `renderLeftContent` (~871) "No tools
    tracked. Add one: keys track <tool>" and `renderCard` (~974) "...keys track <tool>
    --github <repo>". These must be updated to point at the new `t` action.
  - `splitTags` (`internal/cmd/track.go:80`) is only used by `track.go` — it dies with it.

## Development Approach
- **testing approach**: Regular (code first, then tests).
- complete each task fully before moving to the next; small, focused changes.
- **every task includes new/updated tests** for code it changes (success + error/edge).
- **all tests must pass before starting the next task.**
- run `go build ./... && go vet ./... && go test ./...` after each task.
- update this plan file if scope changes during implementation.
- backward compatibility: removing the CLI subcommands is an intentional breaking change for
  the CLI surface; the TUI and `meta.yaml` format are unchanged.

## Testing Strategy
- **Reality check**: `internal/model/render_test.go` contains ONLY pure-helper table tests
  (`wrapText`, `formatStars`, …). There is **no** `Model`/`Update()`/`tea.KeyMsg` harness to
  "follow" — driving the model is net-new test infrastructure.
- **Preferred approach (aligns with existing style)**: extract the mutation logic into small
  pure helpers that take/return `[]loader.ToolMeta` (e.g. `trackTool(meta, input)`,
  `renameTool(meta, old, new)`; untrack already has `loader.RemoveMeta`) and table-test those.
  Keep the `Update()` branches thin wrappers around them. This avoids a heavy KeyMsg harness
  and matches the table-driven convention.
- **Status bar**: unit-test `renderStatusBar()` output strings for each new mode (and the
  decluttered `focusTools` hint) — pure function of model state, easy to assert.
- **Disk isolation (required)**: `loader.SaveMeta` writes to the user's real
  `os.UserConfigDir()/keys/meta.yaml`. Any test that exercises a save MUST isolate it with
  `t.Setenv("HOME", t.TempDir())`. NOTE: on darwin `os.UserConfigDir` uses
  `$HOME/Library/Application Support` and ignores `XDG_CONFIG_HOME` — overriding `HOME` is the
  one that matters.
- **e2e tests**: project has no UI e2e harness (terminal TUI) — none required. Manual TUI
  smoke check is under Post-Completion.

## Progress Tracking
- mark completed items with `[x]` immediately when done.
- add newly discovered tasks with ➕ prefix; blockers with ⚠️ prefix.
- keep this plan in sync with actual work.

## Solution Overview
- Reuse the established input-mode pattern: each new interaction is a boolean mode flag plus
  (where text is entered) a dedicated `textinput.Model`, handled by an early branch in
  `Update()` and a matching branch in `renderStatusBar()`.
- All mutations go through existing `loader` helpers and persist with `loader.SaveMeta`, then
  refresh the list and clamp `metaSelected`. No new persistence code.
- `rename` exists because `ParseToolRef` can only guess the binary name from the repo
  segment; the user fixes it when repo ≠ binary.
- Decluttering (`f`/`v`/`o` + `metaFilter`) keeps the tools panel focused on tracking and
  frees help-bar space. Navigation keys (`j/k`, arrows, `←/→`) keep working but are implicit
  (not shown).

## Technical Details
- New model state (in the `Model` struct): `tracking bool`, `renaming bool`,
  `confirmingUntrack bool`, `untrackTarget string`, `trackInput textinput.Model`,
  `nameInput textinput.Model` (initialised in `New()` like `noteInput`/`tagsInput`).
- **Key dispatch (compile-collision warning):** the main key handler is a single
  `switch msg.String()` and `case "t":` ALREADY EXISTS (model.go:597, edit-tags in
  `focusBrief`). Do NOT add a second `case "t":` — extend the existing one with a focus
  branch: `if m.focus == focusBrief { …tags } else if m.focus == focusTools { …track }`.
  `u` and `r` have no existing cases, but the switch is global, so each new case must be
  gated with `if m.focus == focusTools`.
- **Refresh after every mutation:** track/untrack/rename must, after editing `m.meta` and
  calling `loader.SaveMeta`, run `m.tools = loader.ToolsFromMeta(m.meta)` and
  `m.toolsViewport.SetContent(m.renderLeftContent())`. Track/rename additionally return
  `m.autoFetchCmdsForSelected()` so the new/renamed tool's version/card/changelog/help load
  without a restart.
- Track flow (`t` in `focusTools`): open `tracking`, prompt
  `track (github url or tool name): `, NO live list filtering. On enter:
  `name, github, _ := loader.ParseToolRef(input)`; build
  `loader.ToolMeta{Name: name, GitHub: github, Status: loader.StatusTrying, Added: loader.TodayDate()}`;
  `m.meta = loader.UpsertMeta(m.meta, entry)`; `loader.SaveMeta`; refresh + select the new
  tool. Empty input → cancel. Already-tracked name → `UpsertMeta` updates existing +
  `statusMsg` `already tracked`. esc cancels.
- Untrack flow (`u`): open `confirmingUntrack`, store `untrackTarget = selected name`, prompt
  `Untrack <name>?  [enter] yes  [esc] no`. enter → `loader.RemoveMeta` + `SaveMeta` +
  refresh; keep `metaSelected` at the same index (so selection lands on the next item),
  clamped to `len-1`. esc/any other key → cancel.
- Rename flow (`r`): open `renaming`, `nameInput` prefilled with current `Name`, prompt
  `rename to: `. On enter: locate the entry, set `Name` (keep `GitHub`/`Status`/`Tags`/
  `Note`/`Added`), `SaveMeta`, and clear `helpCache` for the old name. Empty → cancel.
  Collision with another tracked tool's name → reject + `statusMsg` `name already exists`.
- Removal: delete `case "f"` + direct status-filter shortcuts, `case "v"` (+ `checkVersionMsg`
  type/handler/command), `case "o"` (+ `openBrowser` if orphaned), and the `metaFilter` field
  and its reads in `renderLeftContent` and `renderStatusBar`. Preserve the `Init` version
  fetch (`versionMsg`).

## What Goes Where
- **Implementation Steps**: CLI removal, declutter, three new modes, help-bar wording, verify,
  docs.
- **Post-Completion**: manual TUI smoke test in a throwaway config dir.

## Implementation Steps

### Task 1: Remove CLI track/untrack subcommands

**Files:**
- Delete: `internal/cmd/track.go`
- Delete: `internal/cmd/untrack.go`
- Modify: `main.go`

- [x] delete `internal/cmd/track.go` (incl. now-dead `splitTags`) and `internal/cmd/untrack.go`
- [x] remove the `track` and `untrack` dispatch cases in `main.go` (lines ~73-74, ~82-83)
- [x] remove the `track`/`untrack` lines from the usage/help text in `main.go` (~17, ~20)
- [x] `go build ./...` + `go vet ./...` — confirm no dangling references (e.g. `cmd.RunTrack`)
- [x] run `go test ./...` — must pass before next task

### Task 2: Remove f/v/o actions and metaFilter from the tools panel

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/render_test.go`

- [x] remove `case "f"` and the direct status-filter shortcut keys in the `focusTools` handler
- [x] remove the `metaFilter` field and its reads in `filteredMeta()` (~700-703) and
      `renderStatusBar` (~784-787), so the list always shows all tracked tools; remove the now
      dead `else` empty-state branch ("No tools match current filter.") in `renderLeftContent`
      (~870-874)
- [x] remove `case "v"`, its `checkVersionMsg` type/handler/`fetchVersionCmd`, AND the
      orphaned `checkingVersionTool` field (84) with its refs (227-229, 574-576); keep the
      `Init` `versionMsg` async fetch (214-224) intact
- [x] remove `case "o"` and `openBrowser` (delete `openBrowser` only if no longer referenced)
- [x] update the stale CLI-referencing strings in `renderLeftContent` (~871) and `renderCard`
      (~974) to point at the new `t` action instead of `keys track …`
- [x] simplify the `focusTools` default help bar to `[/] search · [q] quit` (the t/u/r hints
      are added in their tasks)
- [x] add/adjust a `renderStatusBar` test asserting the decluttered `focusTools` hint string
- [x] run `go build ./... && go vet ./... && go test ./...` — must pass before next task

### Task 3: Add track mode (`t`)

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/render_test.go`

- [x] add `tracking bool` and `trackInput textinput.Model` to the model; init in `New()`
- [x] extend the EXISTING `case "t"` (model.go:597) with a focus branch so `focusTools` enters
      `tracking` (focus `trackInput`, no live filter) while `focusBrief` keeps editing tags —
      do not add a second `case "t"`
- [x] extract `trackTool(meta, input) ([]loader.ToolMeta, statusMsg)` pure helper using
      `ParseToolRef` + `UpsertMeta` (status `trying`, `Added: TodayDate()`; duplicate →
      "already tracked")
- [x] add an early `Update()` branch for `tracking`: enter calls `trackTool`, then `SaveMeta`,
      `m.tools = loader.ToolsFromMeta(m.meta)`, refresh viewport, select the new tool, and
      return `m.autoFetchCmdsForSelected()`; esc/empty cancels
- [x] add a `renderStatusBar` branch for `tracking` (prompt `track (github url or tool name): `)
      and add `[t] track` to the `focusTools` help bar
- [x] write table tests for `trackTool` (URL → derived name + github; plain name → name only;
      empty → no-op; duplicate → updates, not duplicated)
- [x] write a save-path test with `t.Setenv("HOME", t.TempDir())` so `SaveMeta` does not touch
      the real `meta.yaml`
- [x] run `go build ./... && go vet ./... && go test ./...` — must pass before next task

### Task 4: Add untrack confirmation mode (`u`)

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/render_test.go`

- [ ] add `confirmingUntrack bool` and `untrackTarget string` to the model
- [ ] add `case "u"` gated by `if m.focus == focusTools` to enter confirmation for the
      selected tool
- [ ] add an early `Update()` branch: enter → `RemoveMeta` + `SaveMeta` +
      `m.tools = loader.ToolsFromMeta(m.meta)` + refresh viewport, keep `metaSelected` at same
      index clamped to `len-1`; esc/any other key cancels
- [ ] add a `renderStatusBar` branch (`Untrack <name>?  [enter] yes  [esc] no`) and `[u] untrack`
      to the help bar
- [ ] write tests (table-driven over `RemoveMeta` + clamp): removing lands selection on the
      next item (clamped at the end); cancel leaves the list unchanged; isolate any save with
      `t.Setenv("HOME", t.TempDir())`
- [ ] run `go build ./... && go vet ./... && go test ./...` — must pass before next task

### Task 5: Add rename mode (`r`)

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/render_test.go`

- [ ] add `renaming bool` and `nameInput textinput.Model`; init in `New()`
- [ ] add `case "r"` gated by `if m.focus == focusTools` to enter `renaming` with `nameInput`
      prefilled with the current `Name`
- [ ] extract `renameTool(meta, old, new) ([]loader.ToolMeta, error)` pure helper that updates
      the entry's `Name` (preserving `GitHub`/`Status`/`Tags`/`Note`/`Added`) and rejects a
      collision with another tracked name
- [ ] add an early `Update()` branch: enter calls `renameTool`; on success `SaveMeta`,
      `m.tools = loader.ToolsFromMeta(m.meta)`, clear `helpCache` for the old name, refresh,
      and return `m.autoFetchCmdsForSelected()`; empty cancels; collision → `statusMsg`
      `name already exists`
- [ ] add a `renderStatusBar` branch (prompt `rename to: `) and `[r] rename` to the help bar
- [ ] write table tests for `renameTool`: changes `Name` and preserves `GitHub`/`Status`;
      empty is a no-op; collision is rejected and leaves the entry unchanged; isolate any save
      with `t.Setenv("HOME", t.TempDir())`
- [ ] run `go build ./... && go vet ./... && go test ./...` — must pass before next task

### Task 6: Verify acceptance criteria
- [ ] CLI no longer accepts `track`/`untrack` (subcommands gone, usage text updated)
- [ ] in the TUI tools panel: `t` adds (URL or name), `u` removes after `enter` confirm, `r`
      renames; `f`/`v`/`o` no longer do anything and are absent from the help bar
- [ ] help bar reads `[/] search · [t] track · [u] untrack · [r] rename · [q] quit`
- [ ] run full suite: `go test ./...`; plus `go vet ./...` and `go build ./...`

### Task 7: Update documentation
- [ ] update `CLAUDE.md`: cmd subcommand list (drop `track`/`untrack`), TUI state-machine
      section (new track/untrack/rename modes; removed filter)
- [ ] update `README.md` if it documents the CLI `track`/`untrack` usage
- [ ] update `internal/ui/docs/glossary.md` (~215) which references the removed
      `metaFilter`/`filterLabel`
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion
*Items requiring manual intervention — informational only*

**Manual verification:**
- In a throwaway config dir (on darwin set `HOME` to a temp dir — `os.UserConfigDir` ignores
  `XDG_CONFIG_HOME` there — so the real `meta.yaml` under
  `$HOME/Library/Application Support/keys` is untouched), run `go run .`:
  - `t` → paste `https://github.com/anthropics/claude-code` → entry appears as `claude-code`;
    `r` → rename to `claude` → man/help resolves the real binary.
  - `t` → type `git` → tracked as `git` (no github), status `trying`.
  - `u` on a selected tool → `enter` removes it, selection moves to the next item; `esc`
    cancels.
  - confirm the help bar shows only `[/] search · [t] track · [u] untrack · [r] rename · [q] quit`.

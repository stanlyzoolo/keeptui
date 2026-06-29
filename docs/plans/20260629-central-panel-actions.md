# Central Panel Actions + Full CLI Removal

## Overview

Add data-actions to the central "brief" card panel (`focusBrief`) so the data it already
displays becomes actionable, and remove the CLI entirely — `keys` becomes a pure TUI app.

- **Actions on shown data**: the card shows status, repo, and changelog but offers no way to
  act on them. Add `o` (open repo in browser), `c` (open changelog/releases in browser), and
  `s` (cycle status). `e` (edit note) and `t` (edit tags) already exist and stay.
- **CLI removal**: all tracker editing/listing now lives in the TUI, so the `internal/cmd`
  package and every subcommand are removed. `main.go` collapses to a TUI launcher.
- **Help Bar**: the central-panel hint line drops navigation hints (scroll/help/back) and
  shows the action keys.

## Context (from discovery)

Files/components involved:
- `internal/model/model.go` — `Update()` `focusBrief` branch (key handling), `renderStatusBar()`
  `focusBrief` branch (Help Bar), `Options`/`New()`.
- `internal/model/browser.go` — new file for the open-URL helper.
- `internal/model/render_test.go` — table-driven tests, extended.
- `main.go` — CLI dispatch removed, collapses to TUI launch.
- `internal/cmd/` — deleted entirely (`status_cmd.go`, `note.go`, `list.go`).
- `README.md`, `CLAUDE.md`, `internal/ui/docs/glossary.md` — docs.

Related patterns found:
- Save pattern (note/tags, `model.go:585-626`): `selectedMeta()` returns a value; mutate the
  copy, `m.meta = loader.UpsertMeta(m.meta, mt)`, `loader.SaveMeta(m.meta)`,
  `m.briefViewport.SetContent(m.renderCard())`.
- `loader.NextStatus(s)` (`internal/loader/meta.go:112`) cycles
  `active → trying → forgotten → archived → active`.
- `selectedTool()` exposes `t.GitHub` (normalized `github.com/owner/repo`, no scheme).
- `m.statusMsg` is rendered by `renderStatusBar()` and cleared on the next key (`model.go:294`).
- Keys `o`/`c`/`s` are currently free in `focusBrief` (`/`,`h`,`m`,`e`,`t` are taken).

Dependencies identified: none new — no clipboard/browser library; `openURL` shells out via
`os/exec` per `runtime.GOOS`.

## Development Approach

- **testing approach**: Regular (code + tests together per task), matching the existing
  table-driven `render_test.go`.
- complete each task fully before moving to the next; small, focused changes.
- **every task includes new/updated tests** for its code changes (success + error/edge cases).
- **all tests must pass before starting the next task**.
- update this plan when scope changes during implementation.
- run `go build ./... && go vet ./... && go test ./...` after each change.

## Testing Strategy

- **unit tests**: required for every task. The project has no e2e/UI harness — TUI behavior is
  verified via `render_test.go` (status-bar string assertions, model state after key handling).
- `browserCommand(goos, url)` is a pure function tested per `GOOS` without launching a process.
- Action handlers (`o`/`c`/`s`) are tested by driving `Update()` with a `tea.KeyMsg` and
  asserting model state (`m.meta`, `m.statusMsg`) — no real browser launch.

## Progress Tracking

- mark completed items with `[x]` immediately when done.
- add newly discovered tasks with ➕ prefix; blockers with ⚠️ prefix.
- keep the plan in sync with actual work.

## Solution Overview

The central panel stays a read-mostly card; we wire three new keys in the existing
`focusBrief` switch branch, reusing the note/tags save pattern for `s` and a new
`openURLCmd` `tea.Cmd` for `o`/`c`. Browser launch is isolated behind a pure
`browserCommand(goos, url)` resolver so it is testable and cross-platform. The CLI is deleted
wholesale; `main.go` loses all dispatch and `model.New` loses its `Options` parameter since
the only callers of `InitialTool`/`InitialSearch` were the removed launch flags.

Key design decisions:
- **Cycle, not picker, for status** — minimal, consistent with the prior My Tools `s` behavior
  and `loader.NextStatus`.
- **Browser for changelog** — `c` opens `<repo>/releases`; avoids a duplicate in-TUI overlay
  (the card already shows changelog inline).
- **No repo → statusMsg** — `o`/`c` on a tool without `GitHub` set `m.statusMsg`
  `"no repo for <tool>"` and exec nothing.

## Technical Details

- `browserCommand(goos, url string) (string, []string)`:
  - `darwin` → `"open", []string{url}`
  - `windows` → `"rundll32", []string{"url.dll,FileProtocolHandler", url}`
  - default (linux/other) → `"xdg-open", []string{url}`
- `openURLCmd(url string) tea.Cmd` runs the resolved command via `exec.Command(...).Start()`
  and returns a new `openURLMsg{ err error }` message. A new `case openURLMsg` in the `Update`
  type-switch sets `m.statusMsg` (empty on success, error text on failure). This is required:
  `statusMsg` is only a `string` field (`model.go:82`), there is no `statusMsg` *message type*,
  and the `Update` type-switch has no `default` case — an unhandled message would silently drop
  the browser-launch error.
- `o` URL = `"https://" + t.GitHub`; `c` URL = `"https://" + t.GitHub + "/releases"`.
- `s`: `mt.Status = loader.NextStatus(mt.Status)` → `UpsertMeta` → `SaveMeta` →
  `m.briefViewport.SetContent(m.renderCard())`. No `ToolsFromMeta` — this mirrors the note/tags
  handlers (`model.go:590-595`), and `loader.Tool` has no `Status` field so rebuilding `m.tools`
  would be a no-op for status display.
- New `focusBrief` Help Bar string:
  `[o] open repo  [c] changelog  [s] status  [e] note  [t] tags  [q] quit`. Dropping the
  `→ help` / `← back` hints is an intentional product choice (requested in brainstorm); the keys
  still work, only the on-screen hints are removed.

## What Goes Where

- **Implementation Steps**: all code, tests, and doc updates below — achievable in this repo.
- **Post-Completion**: manual smoke test of the actual browser launch on this machine (exec is
  not exercised by unit tests).

## Implementation Steps

### Task 1: Remove the CLI and collapse main.go to a TUI launcher

**Files:**
- Delete: `internal/cmd/status_cmd.go`, `internal/cmd/note.go`, `internal/cmd/list.go` (whole `internal/cmd/` package)
- Modify: `main.go`

- [x] delete the `internal/cmd/` directory
- [x] rewrite `main.go`: `main()` loads meta and calls `runTUI()`; remove `runCommand`, `parseListFlags`, `helpText`, `tuiOptions`, the `-h/--help`, `-s`, and `<tool>` handling, and the `internal/cmd` import
- [x] keep `runTUI` but drop the `tuiOptions` parameter (temporary `model.New(meta, model.Options{})` call is fixed in Task 2)
- [x] `go build ./...` — will fail only on the `model.New` signature, fixed in Task 2; CLI removal itself must compile-clean otherwise
- [x] no unit tests for `main.go` (thin launcher); verify via build

### Task 2: Remove model.Options and simplify New

**Files:**
- Modify: `internal/model/model.go`
- Modify: `main.go`
- Modify: `internal/model/render_test.go` (only if any test references `Options`/`New`)

- [x] remove `Options` struct (`model.go:117-120`) and the `InitialTool`/`InitialSearch` blocks (`model.go:163-177`)
- [x] change signature to `func New(meta []loader.ToolMeta) Model`
- [x] update the `model.New(...)` call in `main.go` to `model.New(meta)`
- [x] grep for other `model.New`/`Options` references and fix call sites
- [x] run `go build ./...` and `go test ./...` — must pass before Task 3

### Task 3: Add the openURL browser helper

**Files:**
- Create: `internal/model/browser.go`
- Create or Modify: `internal/model/browser_test.go`

- [x] add pure `browserCommand(goos, url string) (string, []string)` covering darwin/windows/default
- [x] define `openURLMsg{ err error }` message type (in `browser.go` or alongside the other msg types in `model.go`)
- [x] add `openURLCmd(url string) tea.Cmd` that runs the resolved command via `exec.Command(...).Start()` and returns `openURLMsg{err}`
- [x] add `case openURLMsg` to the `Update` type-switch (`model.go:219`): on `err != nil` set `m.statusMsg` to the error text, else clear/leave it
- [x] write tests for `browserCommand` per `GOOS` (darwin, windows, linux/default) — assert binary + args, no process launch
- [x] write a test driving `openURLMsg{err: …}` through `Update` and asserting `m.statusMsg` is set
- [x] run `go test ./...` — must pass before Task 4

### Task 4: Wire `o` and `c` actions in the focusBrief branch

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/render_test.go`

- [x] add `case "o"` in the `Update()` key switch: when `focus == focusBrief`, use `selectedTool()`; if `GitHub == ""` set `m.statusMsg = "no repo for " + t.Name` and `return m, nil`, else `return m, openURLCmd("https://" + t.GitHub)` (explicit return matches the `e`/`t` handlers and avoids falling through to `briefViewport.Update`)
- [x] add `case "c"`: same guard and explicit returns, URL `"https://" + t.GitHub + "/releases"`
- [x] write a test: `o`/`c` on a tool with no `GitHub` sets `m.statusMsg` to `"no repo for <tool>"` and does not return an exec command
- [x] write a test: `o`/`c` on a tool with `GitHub` returns a non-nil command (exec not actually run)
- [x] run `go test ./...` — must pass before Task 5

### Task 5: Wire the `s` status-cycle action

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/render_test.go`

- [x] add `case "s"` in the `focusBrief` branch: `selectedMeta()` → `mt.Status = loader.NextStatus(mt.Status)` → `m.meta = loader.UpsertMeta(m.meta, mt)` → `loader.SaveMeta(m.meta)` → `m.briefViewport.SetContent(m.renderCard())` (no `ToolsFromMeta`, matching note/tags)
- [x] write a test driving `s` through the full cycle `active → trying → forgotten → archived → active` and asserting `m.meta` reflects each step
- [x] write a test that `s` outside `focusBrief` (e.g. `focusTools`) does not change status
- [x] run `go test ./...` — must pass before Task 6

### Task 6: Update the central-panel Help Bar

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/render_test.go`

- [x] replace the `focusBrief` branch in `renderStatusBar()` with `[o] open repo  [c] changelog  [s] status  [e] note  [t] tags  [q] quit` (using `keyHint`)
- [x] confirm navigation keys (`↑↓` scroll, `→/←` panel nav, `q`) still work — only the hints are removed
- [x] write/extend a test asserting the `focusBrief` status bar contains `[o]`,`[c]`,`[s]`,`[e]`,`[t]`,`[q]` and does NOT contain `scroll`, `help`, or `back`
- [x] run `go test ./...` — must pass before Task 7

### Task 7: Update documentation

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`
- Modify: `internal/ui/docs/glossary.md`

- [ ] `README.md`: remove the CLI commands section (`keys status`/`note`/`list`); document the app as TUI-only; update the central-panel Help Bar and key list
- [ ] `CLAUDE.md`: remove the stale `internal/cmd` subcommand row; document central-panel actions (`o`/`c`/`s`/`e`/`t`) and the CLI removal; fix data flow to `model.New(meta)`
- [ ] `internal/ui/docs/glossary.md`: update only affected entries (central Help Bar; remove `keys status/note/list` references) — full glossary rewrite is OUT OF SCOPE
- [ ] no unit tests (docs only)

### Task 8: Verify acceptance criteria
- [ ] verify all Overview requirements are implemented (`o`/`c`/`s` actions, CLI gone, Help Bar updated)
- [ ] verify edge cases: no-`GitHub` tool, status cycle wraps correctly, actions are inert outside `focusBrief`
- [ ] run full suite: `go build ./... && go vet ./... && go test ./...`
- [ ] confirm `internal/cmd/` is gone and nothing imports it

### Task 9: Finalize documentation and plan
- [ ] re-read README/CLAUDE/glossary diffs for consistency
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion

*Items requiring manual intervention — informational only*

**Manual verification:**
- Smoke-test the real browser launch: in the running TUI, focus the central panel on a tool
  with a GitHub repo and press `o` (repo opens) and `c` (releases page opens). Unit tests assert
  the command is built but do not launch a process.
- Confirm `s` persists across restarts (status written to `~/.config/keys/meta.yaml`).

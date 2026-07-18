# Tool Update from TUI

## Overview

- Add in-TUI updating of installed tools: when the brief card shows a newer release (`↑`), press `[u]` in `focusBrief`, confirm the detected command, and keys runs it, streaming live output into the `[3]` panel.
- Solves the "I see the update, read the changelog, but have to leave the app to install it" gap.
- Integrates with the existing async-fetch architecture: same msg-driven flow, same `proc.DetachTTY` sandbox, same `StripANSI`/`cleanTerminalOutput` sanitization, same spinner pattern as `refreshingFor`.

## Context (from discovery)

- Files/components involved: `internal/loader` (meta schema), new `internal/updater` (detection + plan), `internal/model` (`model.go`, `mode.go`, `commands.go`, `render.go` + tests), `internal/proc` (reused as-is).
- Related patterns found: `refreshingFor` spinner + double-press guard; `modeConfirmUntrack` confirm flow; `installedMsg`/`remoteMsg` merge + cursor remap by name; two-layer probe sandbox (`DetachTTY` + sanitization); test seams (`testConfigDir`, `testCacheDir`).
- Dependencies identified: no new external deps; `sh -c` for custom commands, package managers invoked as argv.

## Development Approach

- **Testing approach**: Regular (code first, then tests in the same task)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
  - tests are not optional — they are a required part of the checklist
  - unit tests for new and modified functions, new code paths, success and error scenarios
- **CRITICAL: all tests must pass before starting next task** (`go test -race ./...`) — no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Maintain backward compatibility (`meta.yaml` without `update_cmd` keeps working; field is `omitempty`)

## Testing Strategy

- **Unit tests**: required for every task; `go test -race ./...` is the gate (the model has real async state — keep `-race`).
- **E2E tests**: project has none — manual verification scenarios listed in Post-Completion.

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix
- Update plan if implementation deviates from original scope

## Solution Overview

- New bottom-layer package `internal/updater` (no TUI knowledge, like `version`): detects the package manager that owns the installed binary and produces an update `Plan`; an explicit `update_cmd` in `meta.yaml` always wins.
- The model runs the plan as a streamed subprocess via the Bubble Tea channel + re-subscribe idiom (`updateChunkMsg` loop → `updateDoneMsg`), showing live output in the `[3]` panel while the TUI stays fully navigable.
- Confirm-before-run: `modeConfirmUpdate` shows the exact command (`Plan.Display`) in the status bar. Detection itself is a `tea.Cmd` (`detectUpdateCmd` → `updateDetectedMsg{plan, err}`) — `Detect` spawns subprocesses (`go version -m`, `cargo install --list`) and must never run inside `Update()`, same as every other probe in the codebase. The msg handler enters the mode on success; `ErrUnknownManager` never opens a dead-end dialog, just a `statusMsg` hint.
- Deliberately out of scope: downloading binaries from GitHub Releases (manual installs get a hint + `[o]`), bulk "update all", update queue.

## Technical Details

**`updater.Plan`**:

```go
type Plan struct {
    Manager string   // "brew" | "go" | "cargo" | "npm" | "pipx" | "custom"
    Argv    []string // ["brew", "upgrade", "ripgrep"] — argv, not a shell string
    Display string   // "brew upgrade ripgrep" — what the confirm dialog shows
}
```

**Detection chain** (`Detect(t loader.Tool) (Plan, error)`), first hit wins:

1. `t.UpdateCmd != ""` → `Manager: "custom"`, `Argv: ["sh", "-c", t.UpdateCmd]` (user may write pipes/`&&`), `Display: t.UpdateCmd`. Detection is skipped entirely.
2. `exec.LookPath(t.Name)` → `filepath.EvalSymlinks` → real path, then:
   - path contains `/Cellar/<formula>/` → **brew**: `brew upgrade <formula>` (formula name from the path — it can differ from the binary name, e.g. `ripgrep`/`rg`);
   - `go version -m <path>` yields a `path <module>` line → **go**: `go install <module>@latest` (buildinfo works wherever the binary lives);
   - path under `~/.cargo/bin` → **cargo**: `cargo install <crate>` (crate from `cargo install --list`, fallback: binary name);
   - path under `~/.local/pipx/venvs/<pkg>/` → **pipx**: `pipx upgrade <pkg>`;
   - realpath under the global npm prefix with `node_modules/<pkg>` → **npm**: `npm install -g <pkg>`;
   - nothing matched → typed `ErrUnknownManager`.
   - **Order matters**: brew before go — a brew-installed Go binary has buildinfo and would otherwise be misrouted.
   - Pure core `detectFromPath(realPath, buildinfo string) (Plan, error)` so table tests need no real managers; `testHomeDir` seam mirrors `testConfigDir`.

**Streaming** (channel + re-subscribe, no `*tea.Program`):

- `updateDetectedMsg{tool string, plan updater.Plan, err error}` (detection result), `updateChunkMsg{tool, line string, replace bool, ch chan string}`, `updateDoneMsg{tool string, err error}`.
- `startUpdateCmd(plan, tool)`: `exec.Command` + `proc.DetachTTY` (a sudo prompt fails fast instead of hanging — deliberate), stdout+stderr merged into one pipe. **Reader ordering is load-bearing** (`os/exec` docs: `Wait` must not run until pipe reads finish): the goroutine scans the pipe to EOF → then `cmd.Wait()` → writes the exit error into a buffered `chan error` (cap 1) → then `close(lines)`. Returns `waitForChunkCmd`.
- `waitForChunkCmd`: one receive → `updateChunkMsg`; closed channel → read the error channel (safe: the error write happens-before `close`) → `updateDoneMsg`. The chunk handler appends to `m.updateLog`, refreshes the viewport if the updating tool is selected, and returns `waitForChunkCmd` again. Everything wrapped in `safeCmd`.
- Sanitization at the boundary: each segment through `cleanTerminalOutput` (it already calls `stripANSI` internally — no separate strip pass). Reader splits on `\n` AND `\r`; a `\r` segment **replaces** the last buffer line (`replace` flag on the msg; brew/npm progress bars render as one updating line, not hundreds of copies).
- Buffer capped at ~500 lines (tail matters, head doesn't). 10-min timeout (`cargo install` compiles for a long time); on timeout kill the **process group** (negative pid — `DetachTTY` sets `Setsid`, so the child is a session leader and `CommandContext`'s default kill would orphan `sh -c` grandchildren) + `updateDoneMsg{err}`.

**Completion** (`updateDoneMsg` handler):

- Clears `m.updatingFor`. Success → `fetchInstalledCmd(t)` + `statusMsg "updated <name>"`; the version merge extinguishes `↑` and moves the tool out of the update group — the existing by-name cursor remap handles it. Failure → `statusMsg "update failed — see [3]"` + `logx.Errorf` (manager, exit code, last lines; never the token).
- Tool untracked mid-update: process finishes, `updateDoneMsg` for an untracked tool just clears `updatingFor`, no re-fetch.

**UX**:

- `[u]` in `focusBrief` only (`u` in `focusTools` stays untrack — branch on focus, like `r`). Requires `hasUpdate(name)`, else `statusMsg`. Guard: `updatingFor != ""` → no-op (one update at a time, no queue). The key fires `detectUpdateCmd(t)`; the `updateDetectedMsg` handler enters `modeConfirmUpdate` (stale msg for a no-longer-selected tool is dropped).
- `modeConfirmUpdate` (modeled on `modeConfirmUntrack`, with its own `case` in the `Update()` mode dispatch): status bar shows `update <name>: <plan.Display> — [enter] run  [esc] cancel`; the pending plan lives in `m.updatePlan`. `ErrUnknownManager` → mode not entered, `statusMsg` hints at `update_cmd` + `[o]` releases.
- While running: `m.updatingFor` twins `refreshingFor` — card title `updating <name> <spinner>`; the `spinner.TickMsg` gate must extend to `refreshingFor != "" || updatingFor != ""` or the spinner freezes after one frame.
- Live log in panel `[3]`: single buffer `m.updateLog []string` (one active session — not a map), panel title `[3] Update` while the updating tool is selected, autoscroll to bottom; navigating away shows the other tool's normal help, navigating back shows the live log. The log branch lives **inside `renderHelpContent()` ahead of the `helpLoadingFor`/cache branches**, and `autoFetchCmdsForSelected` skips the help fetch for the tool whose log is showing — otherwise re-selecting it paints `Loading...` or a late `helpOutputMsg` clobbers the live log. Log persists after completion until the next update.
- `[u] update` hint in the `focusBrief` bar only when `hasUpdate` (bar is already the longest one).
- Windows: detection degrades softly to `ErrUnknownManager` for everything except go/cargo/npm — no dedicated work. The `update_cmd` override runs via `sh -c` and is unix-only for now (documented limitation; a `cmd /c` branch is future work).
- No `brew update` before `upgrade` (a full `brew update` takes minutes; an honest "already up to date" in the log is acceptable).

## What Goes Where

- **Implementation Steps** (`[ ]` checkboxes): code, tests, docs in this repo.
- **Post-Completion** (no checkboxes): manual TUI verification with real package managers.

## Implementation Steps

### Task 1: Add `update_cmd` field to meta schema

**Files:**
- Modify: `internal/loader/meta.go`
- Modify: `internal/loader/loader.go`
- Modify: `internal/loader/meta_test.go`

- [x] add `UpdateCmd string \`yaml:"update_cmd,omitempty"\`` to `ToolMeta`
- [x] add `UpdateCmd` to `Tool` and thread it through `ToolsFromMeta`
- [x] write test: `meta.yaml` round-trip preserves `update_cmd`; absent field stays empty and is not serialized (omitempty)
- [x] write test: `ToolsFromMeta` carries `UpdateCmd` into `Tool`
- [x] run `go test -race ./...` — must pass before task 2

### Task 2: `internal/updater` — Plan type and pure detection core

**Files:**
- Create: `internal/updater/updater.go`
- Create: `internal/updater/updater_test.go`

- [x] define `Plan{Manager, Argv, Display}` and typed `ErrUnknownManager`
- [x] implement pure `detectFromPath(realPath, buildinfo string) (Plan, error)` with the brew → go → cargo → pipx → npm chain; `testHomeDir` seam for `~`-relative checks (pattern: `loader.testConfigDir`)
- [x] `Display` built by joining `Argv` for auto-detected plans
- [x] write table tests: Cellar path → brew with formula name from path (`ripgrep` for an `rg` binary); buildinfo `path` line → `go install <module>@latest`; `~/.cargo/bin` → cargo; pipx venv path → pipx; npm global `node_modules/<pkg>` → npm; unmatched → `ErrUnknownManager`
- [x] write test pinning order: Cellar path WITH valid buildinfo must yield brew, not go
- [x] run `go test -race ./...` — must pass before task 3

### Task 3: `updater.Detect` — OS wrapper over the pure core

**Files:**
- Modify: `internal/updater/updater.go`
- Modify: `internal/updater/updater_test.go`

- [x] implement `Detect(t loader.Tool)`: `UpdateCmd` override → custom plan via `sh -c` (skip detection entirely); else `LookPath` + `EvalSymlinks` + collect buildinfo via `go version -m` (through `proc.DetachTTY`, short timeout) and feed `detectFromPath`
- [x] crate-name resolution for cargo via `cargo install --list` (fallback: binary name); package resolution for pipx/npm from the realpath segments
- [x] not-on-PATH → `ErrUnknownManager` wrapped with a "not installed" hint; helper failures (`go version -m` absent, `cargo` absent) degrade to the next check, never abort detection
- [x] write test: `UpdateCmd` override returns `custom` plan `["sh","-c",cmd]` and never calls detection
- [x] write test: missing binary → error; symlinked binary resolves through `EvalSymlinks` (use `t.TempDir()` fixtures)
- [x] run `go test -race ./...` — must pass before task 4

### Task 4: Streaming plumbing — msgs, reader, chunk loop

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/commands.go`
- Modify: `internal/model/commands_test.go`

- [x] add `updateDetectedMsg{tool, plan, err}` / `updateChunkMsg{tool, line string, replace bool, ch chan updateLine}` / `updateDoneMsg{tool string, err error}` and model fields `updatingFor string`, `updatePlan updater.Plan`, `updateLog []string`, `updateLogFor string` — ⚠️ deviation: the channel is `chan updateLine` (a `{text, replace, done, err}` struct), not `chan string`. A plain string channel can't carry the `replace` flag or the completion error; the typed channel folds the done+error item into the same channel so no second error channel needs threading through every re-subscribe (happens-before still trivial: done item sent before `close`).
- [x] implement `detectUpdateCmd(t)` in `commands.go`: runs `updater.Detect(t)` in a `tea.Cmd` (it spawns subprocesses — must never run inside `Update()`), emits `updateDetectedMsg`
- [x] implement `startUpdateCmd(plan, tool)` in `commands.go`: `exec.Command` (10-min deadline) + `proc.DetachTTY`, merged stdout+stderr pipe (`cmd.Stderr = cmd.Stdout`); reader goroutine: scan pipe to EOF → **then** `cmd.Wait()` (os/exec forbids Wait before pipe reads finish) → send the exit error as the final `updateLine{done:true,err}` → **then** `close(ch)`; splits on `\n` and `\r` via `streamLines` (replace reflects the *previous* segment's terminator — terminal-accurate progress-bar collapse); on deadline kill the process group via new `proc.KillGroup` (negative-pid SIGKILL — `Setsid` child is a session leader, plain kill orphans `sh -c` grandchildren); wrap via `safeCmd`; returns the first chunk by invoking `waitForChunkCmd(...)()` (returning the Cmd value would hand Update a `tea.Cmd` as a Msg)
- [x] implement `waitForChunkCmd`: receive → `updateChunkMsg`; done item or closed channel → `updateDoneMsg` (done item sent before `close`, race-free)
- [x] chunk handler in `Update()`: sanitize segment via `cleanTerminalOutput` (it already strips ANSI), append or `replace` last line in `m.updateLog`, cap at 500 lines, refresh `[3]` viewport + autoscroll only when the updating tool is selected, re-subscribe with `waitForChunkCmd`
- [x] write tests (msgs fed by hand, no subprocess): `\n` appends / `\r` replaces last line; 500-line cap keeps the tail; chunk for a non-selected tool leaves the selected tool's viewport untouched (plus: sanitization, foreign-tool drop, `streamLines` splitting, `waitForChunkCmd`, `detectUpdateCmd` custom plan, `proc.KillGroup` group-kill, and one real-subprocess streaming integration test)
- [x] run `go test -race ./...` — must pass before task 5

### Task 5: UX — `[u]` key, confirm mode, spinner

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/mode.go`
- Modify: `internal/model/render.go`
- Modify: `internal/model/mode_test.go`

- [x] add `modeConfirmUpdate` to `inputMode` **and** an explicit `case modeConfirmUpdate:` in the `switch m.mode` dispatch in `Update()` (every mode has one)
- [x] `case "u"` in `focusBrief` (branch on focus — `focusTools` keeps untrack): guard `hasUpdate(name)` (else `statusMsg`), guard `updatingFor != ""` (no-op), then fire `detectUpdateCmd(t)` — no subprocess work on the Update thread
- [x] `updateDetectedMsg` handler: drop if the tool is no longer selected or an update is already running; `ErrUnknownManager` → `statusMsg` hint (`set update_cmd or update manually, [o] opens releases`); success → store `m.updatePlan`, enter `modeConfirmUpdate`
- [x] `updateConfirmUpdate` handler in `mode.go`: `enter` → set `updatingFor`, reset `updateLog`/`updateLogFor`, fire `startUpdateCmd(m.updatePlan, name)` + spinner tick; `esc` → back to `modeNormal`
- [x] `renderStatusBar()` branch for `modeConfirmUpdate`: `update <name>: <plan.Display> — [enter] run  [esc] cancel`
- [x] card title while `updatingFor != ""`: `updating <name> <spinner>` (twin of `refreshingFor`; both set → refreshing wins, they're mutually exclusive anyway via guards); extend the `spinner.TickMsg` gate to `refreshingFor != "" || updatingFor != ""` — without this the spinner freezes after one frame
- [x] `[u] update` hint appended to the `focusBrief` bar only when `hasUpdate(selected)`
- [x] write tests: `[u]` without `hasUpdate` → `statusMsg`, mode unchanged; `[u]` while `updatingFor != ""` → no-op; `updateDetectedMsg` success → `modeConfirmUpdate` with plan in status bar; stale `updateDetectedMsg` (other tool selected) dropped; `esc` cancels without side effects; `enter` sets `updatingFor` and returns a command; `spinner.TickMsg` keeps ticking while `updatingFor != ""`
- [x] run `go test -race ./...` — must pass before task 6

### Task 6: `[3]` panel — live update log rendering

**Files:**
- Modify: `internal/model/render.go`
- Modify: `internal/model/model.go`
- Modify: `internal/model/render_test.go`

- [ ] log branch **inside `renderHelpContent()`, ahead of the `helpLoadingFor`/cache branches**: when `updateLogFor == selected`, return the log buffer — otherwise re-selecting the updating tool shows `Loading...`
- [ ] `autoFetchCmdsForSelected` skips the help fetch (and `helpLoadingFor` set) for the tool whose log is showing, so a late `helpOutputMsg` can't clobber the live log
- [ ] panel title `[3] Update` while the log is displayed (same `insetPanelTitle` path as `[3] Help`/`[3] Man`)
- [ ] autoscroll: viewport `GotoBottom()` on each appended chunk while the log is displayed; manual wheel-scroll still works between chunks
- [ ] `selectMeta` away → normal help for the newly selected tool; back → log again (buffer survives until the next update starts)
- [ ] write tests: title switches to `[3] Update` for the updating tool and back to help on another tool; re-selecting the updating tool shows the log, not `Loading...`; log persists after `updateDoneMsg`; new update resets the buffer
- [ ] run `go test -race ./...` — must pass before task 7

### Task 7: Completion handling — re-fetch, remap, errors, logging

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/commands_test.go`

- [ ] `updateDoneMsg` handler: clear `updatingFor`; success → `statusMsg "updated <name>"` + `fetchInstalledCmd(t)` (the merge handler's existing by-name cursor remap moves the tool out of the update group); failure → `statusMsg "update failed — see [3]"` + `logx.Errorf` with manager, exit code and last log lines (never the token)
- [ ] untracked-mid-update edge: `updateDoneMsg` for a tool no longer in `m.meta` clears `updatingFor` only — no re-fetch, no statusMsg crash
- [ ] write tests: success → `fetchInstalledCmd` returned and `updatingFor` empty; failure → statusMsg set, log written (via `logx.SetDirForTesting`); untracked tool → clean no-op
- [ ] write test: after a success merge flips `hasUpdate` off, selection still points at the same tool by name
- [ ] run `go test -race ./...` — must pass before task 8

### Task 8: Verify acceptance criteria

- [ ] verify all requirements from Overview are implemented (detect brew/go/cargo/pipx/npm, `update_cmd` override, confirm dialog, live `[3]` log, spinner, re-fetch on success)
- [ ] verify edge cases: no `hasUpdate`, unknown manager, double-press, untrack mid-update, `\r` progress bars, 500-line cap
- [ ] run full test suite: `go test -race ./...`
- [ ] run `go vet ./...` and `golangci-lint run`

### Task 9: [Final] Update documentation

- [ ] update CLAUDE.md: `internal/updater` row in the package table, `[u]`/`modeConfirmUpdate` in the TUI state machine section, streaming pattern note, `update_cmd` in the meta schema mention
- [ ] update README.md if it documents keys/features
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion

*Items requiring manual intervention — no checkboxes, informational only*

**Manual verification**:
- update a real brew-installed tool with a pending release; watch the live log, verify `↑` disappears and the tool leaves the update group
- update a `go install`-ed tool (buildinfo path) and a tool with `update_cmd` set in `meta.yaml`
- verify a manually-installed binary shows the `ErrUnknownManager` hint and `[o]` opens releases
- verify a long `cargo install` keeps the TUI responsive (navigate away and back mid-update)
- verify a sudo-requiring update fails fast with a readable error in `[3]` (DetachTTY, deliberate)

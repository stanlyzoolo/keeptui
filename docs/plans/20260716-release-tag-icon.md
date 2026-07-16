# Release Tag Icon in the Brief Card

## Overview
- Add a Nerd Font tag icon (`` U+F412, nf-oct-tag) to the `latest:` release line in the brief card's `[info]` section, mirroring the tag octicon GitHub shows next to a release tag.
- Purely cosmetic: makes the release line scannable at a glance and visually consistent with GitHub's release UI.
- Integrates into the existing `renderCard` `[info]` section; no data-flow, state, or fetch changes.

## Context (from discovery)
- Files/components involved: `internal/model/render.go` (the two `latest:` branches at lines 689–695), `internal/model/render_test.go` (`TestRenderCardInstalledLatest`, asserts at lines 2141 and 2154), `CLAUDE.md` ("Card versions" bullet).
- Related patterns found: all UI glyphs (`▸`, `▎`, `↑`) are inline string literals in `render.go` — no named glyph constants; the update branch renders the version + ` ↑` in `UpdateAvailableStyle`, the normal branch renders the whole line in `InfoStyle`.
- Dependencies identified: none — no new imports; `stripANSI` in the tests passes the glyph through untouched (it is not an escape sequence).

## Development Approach
- **Testing approach**: Regular (code first, then update tests) — the change is covered by updating the existing `strings.Contains` asserts, which become the regression coverage.
- Complete each task fully before moving to the next.
- Make small, focused changes.
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task.
- **CRITICAL: all tests must pass before starting next task** — no exceptions.
- **CRITICAL: update this plan file when scope changes during implementation.**
- Run tests after each change (`go test -race ./...` — the version package has real mutex-guarded state, keep `-race`).
- Maintain backward compatibility (rendering-only change; no storage or API impact).

## Testing Strategy
- **Unit tests**: update the two asserts in `TestRenderCardInstalledLatest` (`internal/model/render_test.go:2141`, `:2154`) to expect the glyph; also confirm no other test asserts the `latest:` line (verified: these are the only two).
- **E2E tests**: none in this project.

## Progress Tracking
- Mark completed items with `[x]` immediately when done.
- Add newly discovered tasks with ➕ prefix.
- Document issues/blockers with ⚠️ prefix.
- Update plan if implementation deviates from original scope.

## Solution Overview
- Insert the glyph between the `latest: ` label and the version, styled with whatever styles the version itself:
  - Update branch: `ui.InfoStyle.Render("latest: ") + ui.UpdateAvailableStyle.Render(" "+card.Latest+" ↑") + ui.InfoStyle.Render(suffix)` — the icon shares the peach highlight with the version and arrow.
  - Normal branch: `ui.InfoStyle.Render("latest:  "+card.Latest+suffix)` — the icon shares the muted line style.
- Rendered result: `latest:  v1.2.3 (2024-01-15)` / `latest:  v2.0.0 ↑ (2026-01-02)`.
- The icon renders only when `card.Latest != ""` — already guaranteed by the enclosing branch condition, so no "tag icon with no tag" state exists.
- Inline literal (no `ui` constant): matches the codebase convention for `▸`/`▎`/`↑` — chosen in the brainstorm over a named constant (YAGNI, single call site).

## Technical Details
- Glyph: `` U+F412 (Nerd Font nf-oct-tag). Requires a Nerd Font-patched terminal font; renders as tofu otherwise — accepted, no fallback (see out of scope).
- Known accepted risk: U+F412 is in the Private Use Area, which is East-Asian-Ambiguous; under `RUNEWIDTH_EASTASIAN=1` it may measure 2 cells. The icon lives in flowing viewport content (not the right-aligned status bar), so the worst case is a wrap one cell early — acceptable and documented here.
- No data-structure, message, cache, or storage changes.

## Out of Scope (YAGNI)
- No Nerd Font detection or fallback glyph, no configuration knob.
- No icon in the `[changelog]` section or the tools list.

## Implementation Steps

### Task 1: Add the tag glyph to the latest: line

**Files:**
- Modify: `internal/model/render.go`
- Modify: `internal/model/render_test.go`

- [ ] in `render.go` (lines 689–695), insert ` ` before `card.Latest` in the update branch, inside the `UpdateAvailableStyle.Render` call together with the version and ` ↑`
- [ ] in the same block, insert ` ` before `card.Latest` in the normal branch, inside the single `InfoStyle.Render` call
- [ ] update the assert at `render_test.go:2141` to `strings.Contains(card, "latest:  v2.0.0")`
- [ ] update the assert at `render_test.go:2154` to `strings.Contains(card, "latest:  v2.0.0 ↑ (2026-01-02)")`
- [ ] run `go test -race ./...` — must pass before task 2

### Task 2: Verify acceptance criteria

- [ ] verify the icon appears in both branches (with and without update) and only when `card.Latest != ""`
- [ ] verify no other tests or renderers assume the old `latest: <version>` format (`grep -rn '"latest: ' internal/`)
- [ ] run full suite: `go build .`, `go vet ./...`, `go test -race ./...`
- [ ] run `golangci-lint run`

### Task 3: [Final] Update documentation

- [ ] add one sentence to the "Card versions" bullet in `CLAUDE.md` noting the `latest:` value is prefixed with the Nerd Font tag glyph (U+F412, tofu without a Nerd Font — accepted)
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion
*Items requiring manual intervention or external systems — no checkboxes, informational only*

**Manual verification:**
- Run `keys` in a Nerd Font terminal and confirm the icon renders in the `latest:` line for a tool with a GitHub release, both up-to-date and update-available states.
- Optionally spot-check a non-Nerd-Font terminal to confirm the tofu glyph does not break the card layout.

# Rate Gauge Visibility Fix

## Overview
- The status-bar GitHub API Usage gauge renders its bar as background-colored spaces. On terminals where the color profile degrades (limited `TERM`, tmux/ssh edge cases) the background is stripped and the bar becomes 12 invisible blank cells.
- With a token (limit 5000) `gaugeFilled` rounds to the nearest of 12 cells, so the first cell appears only after 209 used requests — the bar looks permanently empty in a typical session.
- Fix: render the bar with glyphs (`█` fill / `░` track) styled by foreground color, guarantee at least 1 filled cell when `used > 0`, and pin the fill/track color distinction with a forced-truecolor ANSI test.
- The bar stays constant-yellow at every pressure level (no recolor); only the medium — glyphs instead of painted spaces — changes.

## Context (from discovery)
- Files/components involved:
  - `internal/model/render.go` — `renderRateGauge` (bar assembly, lines ~165-181), `gaugeFilled` (fill math, lines ~202-211), `gaugeCells = 12`.
  - `internal/ui/styles.go` — `RateGaugeFillStyle` / `RateGaugeTrackStyle` currently use `Background(...)`; palette has `ColorOrange = #E5A040`, `ColorOrangeDim = #7A5A1E`.
  - `internal/model/render_test.go` — `TestGaugeFilled`, `TestRenderRateGauge`, `TestRenderStatusBarGauge`.
- Related patterns found:
  - Color-sensitive tests already force a profile: `lipgloss.SetColorProfile(termenv.TrueColor)` in `TestColorizeHelp` and `TestRenderAPIStatusHintsNotItalic` (render_test.go) and in `internal/ui/overlay_test.go` (which also asserts a raw truecolor sequence, `38;2;136;136;136`).
  - Existing gauge tests compare stripped-ANSI content; the exhausted-bar assertion matches `"[" + strings.Repeat(" ", gaugeCells) + "]"` and must switch to glyphs.
- Dependencies identified: none outside the `model` and `ui` packages. `RateGaugeFillStyle`/`RateGaugeTrackStyle` have no other callers besides `renderRateGauge`. The `[L]` overlay shows a `Used: x / y` text line, not a bar — untouched.

## Development Approach
- **Testing approach**: Regular (code first, then tests in the same task)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
  - Tests are not optional — they are a required part of the checklist
  - Cover both success and edge scenarios
- **CRITICAL: all tests must pass before starting next task** — no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Run `go test -race ./...` after each change (version package has mutex-guarded state — keep `-race`)
- Maintain backward compatibility: gauge layout width must not change (`gaugeCells` stays 12; `renderHintsBar` downgrade logic untouched)

## Testing Strategy
- **Unit tests**: required for every task (see Development Approach above)
- **E2E tests**: none in this project (pure TUI, no e2e harness) — visual check via `go run .` is a post-completion item

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix
- Update plan if implementation deviates from original scope

## Solution Overview
- **Glyph bar**: `renderRateGauge` builds the bar from `strings.Repeat("█", filled)` + `strings.Repeat("░", gaugeCells-filled)`; `RateGaugeFillStyle`/`RateGaugeTrackStyle` switch from `Background(...)` to `Foreground(...)` with the same palette colors. If ANSI is stripped entirely, the glyphs alone still show fill vs track — the failure mode degrades to monochrome instead of invisible.
- **Fill math**: `gaugeFilled` keeps nearest-cell rounding but clamps the result into a truthful range: at least 1 cell when `used > 0`, and at most `gaugeCells-1` when `used < limit` (a full bar must mean exhaustion — the existing "exhausted → full bar" test semantics stay reliable). `used <= 0` → 0 and `used >= limit` → full are unchanged.
- **Color pin**: a new test forces `termenv.TrueColor` (with save/restore via `t.Cleanup`, mirroring `overlay_test.go`) and asserts the *isolated* fill/track renders — `ui.RateGaugeFillStyle.Render("█")` must emit the foreground sequence `38;2;229;160;64` (`#E5A040`) and `ui.RateGaugeTrackStyle.Render("░")` must emit `38;2;122;90;30` (`#7A5A1E`). Asserting on the whole gauge string would be confounded: `RateBracketStyle`/`RateUsageNumStyle` also emit foreground `#E5A040` for the brackets and the `used/limit` number, so a `Foreground→Background` fill regression (emitting `48;2;…`) would slip through. Checking the styles in isolation makes exactly that regression fail.

## Technical Details
- `internal/ui/styles.go`:
  - `RateGaugeFillStyle  = lipgloss.NewStyle().Foreground(ColorOrange)`
  - `RateGaugeTrackStyle = lipgloss.NewStyle().Foreground(ColorOrangeDim)`
  - Update the comment block above them (bar is glyph-based now).
- `internal/model/render.go`:
  - `renderRateGauge`: bar = `ui.RateGaugeFillStyle.Render(strings.Repeat("█", filled)) + ui.RateGaugeTrackStyle.Render(strings.Repeat("░", gaugeCells-filled))`; update the function's doc comment (glyphs, min-1-cell).
  - `gaugeFilled`: after rounding, apply `if used > 0 && filled < 1 { filled = 1 }` and `if used < limit && filled > gaugeCells-1 { filled = gaugeCells - 1 }` (order-safe with the existing `> gaugeCells` clamp; `used >= limit` keeps the full bar, preserving `gaugeFilled(99, 60) == gaugeCells`).
  - `█` (U+2588) / `░` (U+2591) are East-Asian *Ambiguous* width, the same category as the `RoundedBorder` box-drawing glyphs the app already renders everywhere — the ambiguous=1 assumption is pre-existing, so `lipgloss.Width` math in `renderHintsBar` is unaffected.
- Ratio-parity caveat: the old invariant "same percentage → same fill for limit 60 and 5000" now breaks *only* in the clamped edge bands (e.g. `1/60` → 1 cell vs old 0; `4999/5000` → 11 vs old 12). The parity test case `gaugeFilled(15, 60) == gaugeFilled(1250, 5000)` (25%) still holds and stays.

## What Goes Where
- **Implementation Steps** (`[ ]` checkboxes): code changes and tests in this repo
- **Post-Completion** (no checkboxes): manual visual verification in a real terminal

## Implementation Steps

### Task 1: Truthful fill math in `gaugeFilled` (min 1 cell, full = exhausted)

**Files:**
- Modify: `internal/model/render.go`
- Modify: `internal/model/render_test.go`

- [x] add min-1-cell clamp (`used > 0` → `filled >= 1`) and not-full clamp (`used < limit` → `filled <= gaugeCells-1`) to `gaugeFilled` in `internal/model/render.go`; update its doc comment
- [x] extend `TestGaugeFilled` table: `{1, 5000, 1}` and `{1, 60, 1}` (min cell), `{4999, 5000, gaugeCells - 1}` and `{59, 60, gaugeCells - 1}` (not full below limit), keep `{60, 60, gaugeCells}` and `{99, 60, gaugeCells}` (exhausted/over-limit stay full), keep `{0, 60, 0}` and the 25% parity check
- [x] run `go test -race ./internal/model/` — must pass before task 2

### Task 2: Glyph-based bar rendering with foreground colors

**Files:**
- Modify: `internal/ui/styles.go`
- Modify: `internal/model/render.go`
- Modify: `internal/model/render_test.go`

- [x] switch `RateGaugeFillStyle`/`RateGaugeTrackStyle` in `internal/ui/styles.go` from `Background` to `Foreground` (same `ColorOrange`/`ColorOrangeDim`); update the comment above them
- [x] build the bar in `renderRateGauge` (`internal/model/render.go`) from `█` (fill) and `░` (track) glyphs instead of background-painted spaces; update the doc comment
- [x] update `TestRenderRateGauge`: exhausted case asserts `"[" + strings.Repeat("█", gaugeCells) + "]"` in the stripped output; add a partial-fill case (e.g. `used=30, limit=60` → 6×`█` + 6×`░` between brackets)
- [x] add `TestRenderRateGaugeColors`: force `lipgloss.SetColorProfile(termenv.TrueColor)` with save/restore via `t.Cleanup` (pattern from `internal/ui/overlay_test.go:79`), then assert on the *isolated* styles — `ui.RateGaugeFillStyle.Render("█")` contains foreground `38;2;229;160;64` and does NOT contain background `48;2;`, `ui.RateGaugeTrackStyle.Render("░")` contains foreground `38;2;122;90;30`; do not assert fill color on the full gauge string (brackets and the used/limit number also emit foreground `#E5A040` via `RateBracketStyle`/`RateUsageNumStyle` and would mask a fill regression)
  - ⚠️ deviation: literal `38;2;…` byte assertions proved brittle — termenv's hex→RGB conversion rounds (`#7A5A1E` emits `38;2;121;89;30`, not `…122;90;30`). The test derives expected sequences via `termenv.TrueColor.Color(string(ui.ColorOrangeDim)).Sequence(false)` instead, which also pins fill=ColorOrange / track=ColorOrangeDim directly, plus asserts fill ≠ track.
- [x] check `TestRenderStatusBarGauge` and the `renderHintsBar` width test still pass unmodified (glyphs are single-cell; if a stripped-content assertion matched literal spaces, update it to glyphs)
- [x] run `go test -race ./internal/model/ ./internal/ui/` — must pass before task 3

### Task 3: Verify acceptance criteria

- [x] verify all requirements from Overview are implemented: glyph bar, min-1-cell fill, full-bar-only-at-exhaustion, truecolor color-distinction test
- [x] verify edge cases: `used=0` → all-track bar; `Known=false` → no gauge; compact form (`GH x/y [L]`) unchanged; narrow-terminal downgrade unchanged
- [x] run full test suite: `go test -race ./...`
- [x] run `go vet ./...` and `golangci-lint run`
  - ⚠️ local `golangci-lint` binary is built with go1.24 and refuses the project's go1.25 target ("can't load config") — lint will be verified by CI, which installs a matching version. `go vet` passed locally.

### Task 4: [Final] Update documentation

- [x] update the gauge description in `CLAUDE.md` (status-bar section: "fixed 12-cell yellow fill" → glyph bar `█`/`░`, min-1-cell behavior, full bar = exhausted)
- [x] move this plan to `docs/plans/completed/`

## Post-Completion
*Items requiring manual intervention — informational only*

**Manual verification:**
- run `go run .` in a real terminal: with no token (limit 60) confirm the bar visibly fills after a few requests; with a token (limit 5000) confirm at least one `█` appears after the first requests
- optionally sanity-check a degraded profile (`TERM=xterm` / tmux) — the bar must remain visible as glyphs

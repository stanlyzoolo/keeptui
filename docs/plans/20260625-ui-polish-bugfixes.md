# Plan: UI Polish & Bug Fixes — tool panel cleanup, brief panel formatting, right border

## Overview

Polish the three-panel layout with focused UX fixes:
1. Simplify Tool Panel: remove status symbols and tool count, keep only orange cursor dot
2. Redesign Tool Brief Panel first line: format as `tool_name (orange bold) about_text (gray italic)`
3. Add repo link to Tool Brief Panel (before Stars)
4. Add Status field to Tool Brief Panel (before Note)
5. Fix divider rendering in Tool Brief Panel: extend full width with right-edge alignment
6. Add right border to Tool Man Panel (align with Help Bar bottom border)

## Context

Current implementation state (after `20260625-ui-layout-redesign.md`):
- **Tool Panel (`renderTools()`)**: shows status symbol (•/○/✓/✗) + tool name + update mark (↑)
- **Tool Panel footer**: displays "N tools [filter]"
- **Tool Brief Panel (`renderBrief()`)**: About text, Repo URL, dividers, Stars, Languages, Changelog
- **Tool Man Panel (`renderHelp()`)**: --help/man output with left/top/bottom borders, **no right border**
- **Dividers**: currently simple lines, not aligned to panel edges

## Implementation Steps

### Task 1: Simplify Tool Panel — remove status symbol and tool count
- [x] Modify `renderLeftContent()` to only render `●` cursor + tool name
  - [x] Remove `sym` (status symbol) rendering from selected line
  - [x] Keep unselected lines as spaces (no symbol)
  - [x] Keep update mark `↑` if available
- [x] Remove footer line ("N tools [filter]") from `renderLeftContent()`
  - [x] Delete the section that builds `footer` string (current lines ~814-818)
  - [x] Return `sb.String()` without appending footer

### Task 2: Redesign Tool Brief Panel first line — formatted About
- [x] Update `renderBrief()` to format About line as: `name (bold orange) description (gray italic)`
  - [x] Extract tool name: `t.Name`
  - [x] Extract About text: `card.About`
  - [x] Apply styles:
    - [x] `ui.TitleStyle.Bold().Foreground(ui.ColorOrange).Render(t.Name)`
    - [x] Build divider or space between name and About
    - [x] `ui.DescStyle.Italic().Foreground(lipgloss.Color("8")).Render(about_text)` (or use existing dim/gray style)
  - [x] Join on same line with proper spacing
  - [x] If About is empty, show only name
  - [x] If name is very long, truncate to prevent panel overflow

### Task 3: Add repo link to Tool Brief Panel (before Stars)
- [x] Update `renderBrief()` to insert repo URL line after first divider
  - [x] Line format: `repo: github.com/owner/repo` (plain text)
  - [x] Position: between first divider and Stars line
  - [x] Apply `ui.GithubStyle` or plain gray for link
  - [x] If no repo (t.GitHub empty), omit this line

### Task 4: Add Status field to Tool Brief Panel (before Note)
- [x] Update `renderBrief()` to insert Status line before Note
  - [x] Get status: `mt.Status` from selected meta
  - [x] Format: `Status: [Active|Trying|Forgotten|Archived]` with appropriate color
  - [x] Use existing `ui.StatusStyle(mt.Status)` for color
  - [x] Position: before "Note:" line
  - [x] If no status or empty, still show label with placeholder

### Task 5: Fix dividers — extend full width with right-edge alignment
- [x] Update divider rendering in `renderBrief()`
  - [x] Current dividers use `divW = max(cardW-4, 1)` — too short
  - [x] Change to: `divW = briefW - 2` (full width within panel bounds, accounting for padding)
  - [x] Verify dividers span from left edge to right edge of content area
  - [x] Test on different panel widths (80, 120, 150 char screens)
  - [x] Ensure no overflow beyond panel boundary

### Task 6: Add right border to Tool Man Panel
- [x] Modify `renderHelp()` method (or inline in `View()`)
  - [x] Current render: `helpViewport.View()` without borders
  - [x] Add `lipgloss.NewStyle().BorderRight(true)` around help panel
  - [x] Use `ui.ColorBorder` for border color (matches left/top/bottom in other panels)
  - [x] Ensure right border aligns vertically with Help Bar bottom edge
  - [x] Test with different terminal widths to verify alignment
  - [x] Note: Help Bar is full-width; right border should align with its right edge

### Task 7: Update width calculations for dividers
- [ ] Review `calcPanelWidths()` to ensure `briefW` and `helpW` account for borders
  - [ ] Divider width should use `briefW - 2` (account for left/right padding)
  - [ ] Verify no overflow when rendering long text or dividers
  - [ ] Test formula on edge cases: very small screens (80 chars), very large (200+ chars)

### Task 8: Visual verification and testing
- [ ] Manual test on 80-char terminal:
  - [ ] Tool Panel shows only `●` and name (no status symbol, no footer)
  - [ ] Tool Brief Panel About line formatted correctly (name bold orange + description gray italic)
  - [ ] Repo link visible before Stars
  - [ ] Status field visible before Note
  - [ ] Dividers extend full width without overflow
  - [ ] Tool Man Panel has right border aligned with Help Bar
- [ ] Manual test on 120-char terminal: same as above
- [ ] Manual test on 150-char terminal: same as above
- [ ] `go build .` — no errors
- [ ] `go vet ./...` — no warnings

### Task 9: Code cleanup and style consistency
- [ ] Ensure all new styles used exist in `internal/ui/styles.go`
  - [ ] Verify `ColorOrange` or similar orange color is defined
  - [ ] Verify gray/dim style for italic About text exists (or add if needed)
  - [ ] Verify `ColorBorder` is used consistently for borders
- [ ] No breaking changes to existing hotkey bindings
- [ ] All viewport content still respects `wrapText()` boundaries

---

## Future Enhancement

After these polish fixes are verified:
- Consider adding tool icon or badge in Tool Panel
- Enhanced About formatting with syntax highlighting for key terms
- Configurable theme for brief panel colors

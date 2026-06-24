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
- [ ] Update `renderBrief()` to format About line as: `name (bold orange) description (gray italic)`
  - [ ] Extract tool name: `t.Name`
  - [ ] Extract About text: `card.About`
  - [ ] Apply styles:
    - [ ] `ui.TitleStyle.Bold().Foreground(ui.ColorOrange).Render(t.Name)`
    - [ ] Build divider or space between name and About
    - [ ] `ui.DescStyle.Italic().Foreground(lipgloss.Color("8")).Render(about_text)` (or use existing dim/gray style)
  - [ ] Join on same line with proper spacing
  - [ ] If About is empty, show only name
  - [ ] If name is very long, truncate to prevent panel overflow

### Task 3: Add repo link to Tool Brief Panel (before Stars)
- [ ] Update `renderBrief()` to insert repo URL line after first divider
  - [ ] Line format: `repo: github.com/owner/repo` (plain text)
  - [ ] Position: between first divider and Stars line
  - [ ] Apply `ui.GithubStyle` or plain gray for link
  - [ ] If no repo (t.GitHub empty), omit this line

### Task 4: Add Status field to Tool Brief Panel (before Note)
- [ ] Update `renderBrief()` to insert Status line before Note
  - [ ] Get status: `mt.Status` from selected meta
  - [ ] Format: `Status: [Active|Trying|Forgotten|Archived]` with appropriate color
  - [ ] Use existing `ui.StatusStyle(mt.Status)` for color
  - [ ] Position: before "Note:" line
  - [ ] If no status or empty, still show label with placeholder

### Task 5: Fix dividers — extend full width with right-edge alignment
- [ ] Update divider rendering in `renderBrief()`
  - [ ] Current dividers use `divW = max(cardW-4, 1)` — too short
  - [ ] Change to: `divW = briefW - 2` (full width within panel bounds, accounting for padding)
  - [ ] Verify dividers span from left edge to right edge of content area
  - [ ] Test on different panel widths (80, 120, 150 char screens)
  - [ ] Ensure no overflow beyond panel boundary

### Task 6: Add right border to Tool Man Panel
- [ ] Modify `renderHelp()` method (or inline in `View()`)
  - [ ] Current render: `helpViewport.View()` without borders
  - [ ] Add `lipgloss.NewStyle().BorderRight(true)` around help panel
  - [ ] Use `ui.ColorBorder` for border color (matches left/top/bottom in other panels)
  - [ ] Ensure right border aligns vertically with Help Bar bottom edge
  - [ ] Test with different terminal widths to verify alignment
  - [ ] Note: Help Bar is full-width; right border should align with its right edge

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

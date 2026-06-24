# Plan: UI Layout Redesign — 20%-40%-40% three-panel layout

## Overview

Redesign the TUI from a two-panel layout (Tools + Card/Help split) to a three-panel layout:
- **Left Panel (20%)**: Tools list
- **Middle Panel (40%)**: Tool metadata (About, Repo info, Changelog)
- **Right Panel (40%)**: Tool man/help output

Remove emoji from metadata display. All panels must be scrollable with proper text wrapping at panel boundaries. Update Help Bar with context-specific hotkeys per panel.

## Context

Current implementation (`internal/model/model.go`, `internal/ui/styles.go`):
- **Panel sizing**: hardcoded `leftWidth = 22` (tools) + remaining split 50/50 (card/help)
- **Viewports**: `leftViewport`, `cardViewport`, `helpViewport`
- **Focus states**: `focusLeft` (tools), `focusRight` (right side), `focusHeader` (tool name header)
- **Rendering**: `renderLeft()`, `renderRight()`, `renderCard()`, `renderHelpContent()`
- **Key bindings**: `→/←` toggle focus, `j/k` navigate left panel, various single-key commands

## Implementation Steps

### Task 1: Panel width calculation refactor
- [x] Remove hardcoded `leftWidth = 22` constant
- [x] Implement `calcPanelWidths()` to return three widths: `(toolsW, briefW, helpW)` based on 20%-40%-40% ratio
  - [x] Account for 2 border chars per panel = 6 chars overhead
  - [x] Formula: `available = width - 6`; `toolsW = available * 0.2`; `briefW = available * 0.4`; `helpW = available * 0.4`
  - [x] Ensure minimum widths (toolsW >= 15, briefW >= 30, helpW >= 30)
- [x] Update `calcVpHeight()` to account for new layout (currently uses `height - 10` for content)

### Task 2: Rename and restructure viewport fields
- [ ] Rename `leftViewport` → `toolsViewport` (list of tools)
- [ ] Rename `cardViewport` → `briefViewport` (tool metadata/about)
- [ ] Rename `helpViewport` → `helpViewport` (unchanged — tool man/help)
- [ ] Update `WindowSizeMsg` handler to initialize/resize all three with new widths from `calcPanelWidths()`
- [ ] Update `setLeftContent()` → `setToolsContent()` / `setToolsContent()` (reflects new purpose)
- [ ] Update `syncLeftViewport()` → `syncToolsViewport()`

### Task 3: Update focus state constants
- [ ] `focusLeft = 0` → `focusTools` (Tools Panel)
- [ ] `focusRight = 1` → `focusBrief` (Tool Brief Panel) — **new focus point**
- [ ] `focusHeader = 2` → `focusHeader` (tool name header — unchanged)
- [ ] Update all `switch m.focus` statements to use new constants

### Task 4: Restructure right-side rendering
Split current `renderRight()` into two methods:
- [ ] `renderBrief()` — render middle panel (Tool Brief Panel)
  - [ ] Keep current `renderCard()` logic (About, Repo, Stars, Languages, Changelog)
  - [ ] Remove emoji decorations
  - [ ] Ensure text wraps within `briefW` bounds
- [ ] `renderHelp()` — render right panel (Tool man/help output)
  - [ ] Move `renderHelpContent()` body here
  - [ ] Ensure proper word wrapping at `helpW` boundary
- [ ] Update `View()` to call: `left := renderTools()`, `middle := renderBrief()`, `right := renderHelp()`, then join horizontally

### Task 5: Render tools list (left panel)
- [ ] Create `renderTools()` method that wraps `toolsViewport.View()` with border
  - [ ] Content: current `renderLeftContent()` (tool names + status symbols)
  - [ ] Focus state: cyan border if `m.focus == focusTools`
  - [ ] Apply styles from `ui.PanelBorder` / `ui.PanelBorderFocused`
- [ ] Keep selection logic (orange dot `●` at cursor position)

### Task 6: Update Help Bar (renderHelp method)
Replace current context-sensitive Help Bar with panel-specific hints:
- [ ] **focusTools**: `j/k navigate  → details  f filter  a all  / search  v check  o github  q quit`
- [ ] **focusBrief**: `↑↓ scroll  → help  ← back  e edit note  t edit tags  q quit`
- [ ] **focusHelp**: `↑↓ scroll  h --help  m man  / search  ← back  q quit`
- [ ] Preserve existing search/edit mode hints (unchanged)

### Task 7: Update navigation and key bindings
- [ ] **Arrow keys**:
  - [ ] `→/l` from focusTools → focusBrief
  - [ ] `→/l` from focusBrief → focusHelp
  - [ ] `←/h` from focusHelp → focusBrief
  - [ ] `←/h` from focusBrief → focusTools
- [ ] **Vertical navigation**:
  - [ ] `j/k/↑/↓` only in focusTools (tool list)
  - [ ] `j/k/↑/↓/PgUp/PgDn` in focusBrief and focusHelp (panel scrolling)
- [ ] **Tab-based navigation** (optional enhancement):
  - [ ] Preserve existing left/right arrow semantics
  - [ ] Consider: Tab cycles focusTools → focusBrief → focusHelp → focusTools

### Task 8: Remove emoji from styles
- [ ] Delete emoji-heavy rendering from `renderCard()`:
  - [ ] Remove `★` prefix from stars line (replace with text "Stars: ")
  - [ ] Already handles Languages line without emoji (commit ba-qa-fixes shows this)
  - [ ] Verify changelog rendering has no emoji

### Task 9: Update text wrapping and viewport content synchronization
- [ ] Ensure all viewport content calls `wrapText(content, panelWidth-2)` before rendering
- [ ] Verify `wrapText()` is applied consistently:
  - [ ] Tools panel: list items (usually short, but apply for consistency)
  - [ ] Brief panel: About, Repo URL, Changelog body (high priority — long text)
  - [ ] Help panel: --help and man output (already done in prev commit, verify)
- [ ] Test on 80-char, 120-char, and full-width terminals

### Task 10: Mouse handling refactor
- [ ] Update `handleMouse()` to detect left panel vs. middle panel vs. right panel based on new widths
- [ ] Adjust Y-offset calculations for tool selection when clicking left panel
- [ ] Wheel scroll should route to focused panel's viewport

### Task 11: Update auto-fetch commands
- [ ] `autoFetchCmdsForSelected()` unchanged (fetches changelog/help for selected tool)
- [ ] Verify that switching tools via focusTools updates Brief and Help panels correctly
- [ ] Test that switching focus does NOT re-fetch data unnecessarily

### Task 12: Verification and testing
- [ ] `go build .` — no errors
- [ ] `go vet ./...` — no warnings
- [ ] Manual test (terminal width 80, 120, 150):
  - [ ] Left panel shows 8-12 tools (verify scrolls if needed)
  - [ ] Middle panel shows About + Repo info + Changelog (no emojis)
  - [ ] Right panel shows --help output, word-wrapped
  - [ ] Navigation: ↑↓ in left, ← / → switches panels
  - [ ] All panels have visible borders with correct focus highlight
  - [ ] Help Bar updates for each panel
  - [ ] Scrolling works in each panel independently

---

## Future Enhancement

After this redesign is stable:
- Consider Tab key for panel cycling (currently ← → is primary)
- Add mouse-click support to switch panels directly
- Status bar (number of tools, current filter) remains in Help Bar area

package model

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lepeshko/keys/internal/loader"
)

// newMouseTestModel builds a ready Model (viewports sized via WindowSizeMsg)
// with the given tools tracked and the first one selected.
func newMouseTestModel(t *testing.T, width, height int, names ...string) Model {
	t.Helper()
	metas := make([]loader.ToolMeta, len(names))
	for i, n := range names {
		metas[i] = loader.ToolMeta{Name: n, Note: "note of " + n}
	}
	m := New(metas)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return updated.(Model)
}

func leftClick(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
}

func wheelDown(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress}
}

// toolRowY maps a list index to the screen row handleMouse expects
// (row 0 = top margin, row 1 = border, row 2 = first list row).
func toolRowY(m Model, idx int) int {
	return 2 + idx - m.toolsViewport.YOffset
}

// TestMouseClickIgnoredWhileEditingNote is the wrong-target repro: with the
// note editor open on tool A, a click on tool B must not move the selection,
// and enter must still save into A.
func TestMouseClickIgnoredWhileEditingNote(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newMouseTestModel(t, 80, 24, "alpha", "beta")
	m.mode = modeEditNote
	m.noteInput.SetValue("edited during click")

	updated, _ := m.Update(leftClick(1, toolRowY(m, 1)))
	nm := updated.(Model)
	if nm.metaSelected != 0 {
		t.Fatalf("click during note edit moved selection: metaSelected = %d, want 0", nm.metaSelected)
	}

	committed, _ := nm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	cm := committed.(Model)
	if got := cm.meta[0].Note; got != "edited during click" {
		t.Errorf("alpha note = %q, want the edited note", got)
	}
	if got := cm.meta[1].Note; got != "note of beta" {
		t.Errorf("beta note = %q, the edit leaked into the clicked tool", got)
	}
}

// TestMouseClickIgnoredInModalModes verifies clicks change neither selection
// nor focus while a modal input mode owns the screen.
func TestMouseClickIgnoredInModalModes(t *testing.T) {
	for _, mode := range []inputMode{modeConfirmUntrack, modeTokenInput} {
		t.Run(fmt.Sprintf("mode_%d", mode), func(t *testing.T) {
			m := newMouseTestModel(t, 80, 24, "alpha", "beta")
			m.mode = mode
			wantFocus := m.focus

			// Click another tools row, then the brief and help panels.
			for _, msg := range []tea.MouseMsg{
				leftClick(1, toolRowY(m, 1)),
				leftClick(m.toolsW+3, 5),
				leftClick(m.toolsW+m.briefW+5, 5),
			} {
				updated, _ := m.Update(msg)
				m = updated.(Model)
			}
			if m.metaSelected != 0 {
				t.Errorf("metaSelected = %d, want 0", m.metaSelected)
			}
			if m.focus != wantFocus {
				t.Errorf("focus = %d, want unchanged %d", m.focus, wantFocus)
			}
		})
	}
}

// TestMouseWheelScrollsInSearch verifies the wheel stays live in modeSearch:
// scrolling the tools panel moves the viewport without touching selection.
func TestMouseWheelScrollsInSearch(t *testing.T) {
	names := make([]string, 20)
	for i := range names {
		names[i] = fmt.Sprintf("tool%02d", i)
	}
	m := newMouseTestModel(t, 80, 10, names...)
	m.mode = modeSearch

	updated, _ := m.Update(wheelDown(1, 3))
	nm := updated.(Model)
	if nm.toolsViewport.YOffset == 0 {
		t.Error("wheel in modeSearch did not scroll the tools viewport")
	}
	if nm.metaSelected != 0 {
		t.Errorf("wheel moved selection: metaSelected = %d, want 0", nm.metaSelected)
	}
}

// TestMouseNoOpUnderOverlay verifies that while the [L] overlay is visible
// neither clicks nor the wheel move anything underneath.
func TestMouseNoOpUnderOverlay(t *testing.T) {
	names := make([]string, 20)
	for i := range names {
		names[i] = fmt.Sprintf("tool%02d", i)
	}
	for _, mode := range []inputMode{modeAPIStatus, modeTokenInput} {
		t.Run(fmt.Sprintf("mode_%d", mode), func(t *testing.T) {
			m := newMouseTestModel(t, 80, 10, names...)
			m.mode = mode
			wantFocus := m.focus

			for _, msg := range []tea.MouseMsg{
				wheelDown(1, 3),
				leftClick(1, toolRowY(m, 1)),
				leftClick(m.toolsW+3, 5),
			} {
				updated, _ := m.Update(msg)
				m = updated.(Model)
			}
			if m.toolsViewport.YOffset != 0 {
				t.Errorf("tools viewport scrolled under the overlay: YOffset = %d", m.toolsViewport.YOffset)
			}
			if m.metaSelected != 0 || m.focus != wantFocus {
				t.Errorf("state moved under the overlay: metaSelected = %d, focus = %d", m.metaSelected, m.focus)
			}
		})
	}
}

// TestMouseNoOpBeforeReady verifies mouse events before the first
// WindowSizeMsg (zero panel widths) are ignored.
func TestMouseNoOpBeforeReady(t *testing.T) {
	m := New([]loader.ToolMeta{{Name: "alpha"}, {Name: "beta"}})

	updated, cmd := m.Update(leftClick(1, 3))
	nm := updated.(Model)
	if cmd != nil {
		t.Error("mouse before ready returned a command")
	}
	if nm.metaSelected != 0 || nm.focus != m.focus {
		t.Errorf("mouse before ready changed state: metaSelected = %d, focus = %d", nm.metaSelected, nm.focus)
	}
}

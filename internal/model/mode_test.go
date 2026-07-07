package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lepeshko/keys/internal/loader"
)

func keyRunes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// newTestModel builds a Model via New so every textinput is initialized, with
// one tracked tool selected.
func newTestModel(focus int) Model {
	m := New([]loader.ToolMeta{{Name: "git", Tags: []string{"vcs"}, Note: "old note"}})
	m.width = 80
	m.height = 24
	m.focus = focus
	return m
}

// TestModeEnterAndEsc drives Update from modeNormal: each mode's opening key
// must switch to that mode, and esc must return to modeNormal (modeTokenInput
// returns to modeAPIStatus — covered separately below).
func TestModeEnterAndEsc(t *testing.T) {
	esc := tea.KeyMsg{Type: tea.KeyEsc}

	tests := []struct {
		name  string
		focus int
		key   tea.KeyMsg
		want  inputMode
	}{
		{"slash in tools opens search", focusTools, keyRunes("/"), modeSearch},
		{"slash in brief opens help search", focusBrief, keyRunes("/"), modeHelpSearch},
		{"slash in help opens help search", focusHelp, keyRunes("/"), modeHelpSearch},
		{"e in brief opens note edit", focusBrief, keyRunes("e"), modeEditNote},
		{"t in brief opens tags edit", focusBrief, keyRunes("t"), modeEditTags},
		{"t in tools opens track", focusTools, keyRunes("t"), modeTrack},
		{"u in tools opens untrack confirm", focusTools, keyRunes("u"), modeConfirmUntrack},
		{"r in tools opens rename", focusTools, keyRunes("r"), modeRename},
		{"L opens api status", focusTools, keyRunes("L"), modeAPIStatus},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			m := newTestModel(tt.focus)

			updated, _ := m.Update(tt.key)
			nm := updated.(Model)
			if nm.mode != tt.want {
				t.Fatalf("after %q mode = %d, want %d", tt.key.String(), nm.mode, tt.want)
			}

			closed, _ := nm.Update(esc)
			if got := closed.(Model).mode; got != modeNormal {
				t.Errorf("after esc mode = %d, want modeNormal", got)
			}
		})
	}
}

// TestTokenInputSubMode verifies the overlay sub-state: [e] enters
// modeTokenInput and esc falls back to modeAPIStatus, not modeNormal.
func TestTokenInputSubMode(t *testing.T) {
	m := newTestModel(focusTools)
	m.mode = modeAPIStatus

	updated, _ := m.Update(keyRunes("e"))
	nm := updated.(Model)
	if nm.mode != modeTokenInput {
		t.Fatalf("after e mode = %d, want modeTokenInput", nm.mode)
	}

	closed, _ := nm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if got := closed.(Model).mode; got != modeAPIStatus {
		t.Errorf("esc from token input: mode = %d, want modeAPIStatus", got)
	}
}

// TestNoteEditCommit verifies enter in modeEditNote persists the trimmed note
// and returns to modeNormal.
func TestNoteEditCommit(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel(focusBrief)
	m.mode = modeEditNote
	m.noteInput.SetValue("  new note  ")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nm := updated.(Model)
	if nm.mode != modeNormal {
		t.Errorf("mode = %d, want modeNormal", nm.mode)
	}
	if got := nm.meta[0].Note; got != "new note" {
		t.Errorf("note = %q, want %q", got, "new note")
	}
}

// TestNoteEditEscDiscards verifies esc leaves the note untouched.
func TestNoteEditEscDiscards(t *testing.T) {
	m := newTestModel(focusBrief)
	m.mode = modeEditNote
	m.noteInput.SetValue("typed but discarded")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	nm := updated.(Model)
	if got := nm.meta[0].Note; got != "old note" {
		t.Errorf("note = %q, want unchanged %q", got, "old note")
	}
}

// TestTagsEditCommit verifies enter in modeEditTags parses the comma-separated
// input, dropping empty entries.
func TestTagsEditCommit(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel(focusBrief)
	m.mode = modeEditTags
	m.tagsInput.SetValue("cli, , scm ,")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nm := updated.(Model)
	if nm.mode != modeNormal {
		t.Errorf("mode = %d, want modeNormal", nm.mode)
	}
	want := []string{"cli", "scm"}
	got := nm.meta[0].Tags
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("tags = %v, want %v", got, want)
	}
}

// TestTrackCommit verifies enter in modeTrack adds the tool, selects it and
// returns to modeNormal.
func TestTrackCommit(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel(focusTools)
	m.mode = modeTrack
	m.trackInput.SetValue("github.com/junegunn/fzf")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nm := updated.(Model)
	if nm.mode != modeNormal {
		t.Errorf("mode = %d, want modeNormal", nm.mode)
	}
	mt := loader.FindMeta(nm.meta, "fzf")
	if mt == nil {
		t.Fatalf("fzf not tracked, meta = %v", nm.meta)
	}
	if mt.GitHub != "github.com/junegunn/fzf" {
		t.Errorf("GitHub = %q, want normalized ref", mt.GitHub)
	}
	if sel, ok := nm.selectedMeta(); !ok || sel.Name != "fzf" {
		t.Errorf("selection did not move to the new tool")
	}
}

// TestRenameCommit verifies enter in modeRename renames the selected tool and
// drops stale per-name caches.
func TestRenameCommit(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel(focusTools)
	m.mode = modeRename
	m.nameInput.SetValue("git2")
	m.helpCache["git"] = [2]string{"cached", ""}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nm := updated.(Model)
	if nm.mode != modeNormal {
		t.Errorf("mode = %d, want modeNormal", nm.mode)
	}
	if loader.FindMeta(nm.meta, "git2") == nil || loader.FindMeta(nm.meta, "git") != nil {
		t.Errorf("rename not applied, meta = %v", nm.meta)
	}
	if _, ok := nm.helpCache["git"]; ok {
		t.Errorf("stale helpCache entry survived the rename")
	}
}

// TestModeGuards verifies that while one mode owns the input, other modes'
// opening keys are consumed by the active input instead of switching state.
func TestModeGuards(t *testing.T) {
	tests := []struct {
		name string
		mode inputMode
		key  tea.KeyMsg
	}{
		{"L ignored while searching", modeSearch, keyRunes("L")},
		{"t ignored while api overlay open", modeAPIStatus, keyRunes("t")},
		{"r ignored while editing note", modeEditNote, keyRunes("r")},
		{"L ignored while entering token", modeTokenInput, keyRunes("L")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(focusTools)
			m.mode = tt.mode

			updated, _ := m.Update(tt.key)
			nm := updated.(Model)
			if nm.mode != tt.mode {
				t.Errorf("mode = %d, want unchanged %d", nm.mode, tt.mode)
			}
		})
	}
}

// newSearchTestModel builds a model with several tracked tools for exercising
// the search commit/rollback flow from focusTools.
func newSearchTestModel() Model {
	m := New([]loader.ToolMeta{
		{Name: "fzf"},
		{Name: "git"},
		{Name: "ripgrep"},
	})
	m.width = 80
	m.height = 24
	m.focus = focusTools
	return m
}

// typeRunes feeds each rune of s into Update as a separate key message.
func typeRunes(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		updated, _ := m.Update(keyRunes(string(r)))
		m = updated.(Model)
	}
	return m
}

// TestSearchEnterSelectsMatch verifies enter accepts the highlighted match:
// search exits, the cursor points at the chosen tool in the full list, focus
// moves to the brief panel and the query is cleared.
func TestSearchEnterSelectsMatch(t *testing.T) {
	m := newSearchTestModel()

	updated, _ := m.Update(keyRunes("/"))
	m = updated.(Model)
	m = typeRunes(t, m, "rip")

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nm := updated.(Model)

	if nm.mode != modeNormal {
		t.Errorf("mode = %d, want modeNormal", nm.mode)
	}
	if nm.focus != focusBrief {
		t.Errorf("focus = %d, want focusBrief", nm.focus)
	}
	if nm.metaSelected != 2 {
		t.Errorf("metaSelected = %d, want 2 (ripgrep in the full list)", nm.metaSelected)
	}
	if sel, ok := nm.selectedMeta(); !ok || sel.Name != "ripgrep" {
		t.Errorf("selectedMeta = %v, want ripgrep", sel)
	}
	if nm.search.Value() != "" {
		t.Errorf("query = %q, want cleared", nm.search.Value())
	}
	if nm.searchPrevName != "" {
		t.Errorf("searchPrevName = %q, want cleared", nm.searchPrevName)
	}
}

// TestSearchEscRestoresSelection verifies esc rolls the cursor back to the
// tool selected before the search started.
func TestSearchEscRestoresSelection(t *testing.T) {
	m := newSearchTestModel()
	m.metaSelected = 2 // ripgrep

	updated, _ := m.Update(keyRunes("/"))
	m = updated.(Model)
	m = typeRunes(t, m, "fz") // typing resets the highlight to 0

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	nm := updated.(Model)

	if nm.mode != modeNormal {
		t.Errorf("mode = %d, want modeNormal", nm.mode)
	}
	if nm.metaSelected != 2 {
		t.Errorf("metaSelected = %d, want restored 2", nm.metaSelected)
	}
	if sel, ok := nm.selectedMeta(); !ok || sel.Name != "ripgrep" {
		t.Errorf("selectedMeta = %v, want ripgrep restored", sel)
	}
	if nm.searchPrevName != "" {
		t.Errorf("searchPrevName = %q, want cleared", nm.searchPrevName)
	}
}

// TestSearchEnterNoMatches verifies enter with an empty filter is a no-op:
// search stays open and the query is kept.
func TestSearchEnterNoMatches(t *testing.T) {
	m := newSearchTestModel()

	updated, _ := m.Update(keyRunes("/"))
	m = updated.(Model)
	m = typeRunes(t, m, "zzz")

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nm := updated.(Model)

	if nm.mode != modeSearch {
		t.Errorf("mode = %d, want still modeSearch", nm.mode)
	}
	if nm.search.Value() != "zzz" {
		t.Errorf("query = %q, want kept %q", nm.search.Value(), "zzz")
	}
}

// TestSearchArrowsMoveHighlight verifies up/down move through the filtered
// list with wrap-around and never touch the query text.
func TestSearchArrowsMoveHighlight(t *testing.T) {
	m := newSearchTestModel()

	updated, _ := m.Update(keyRunes("/"))
	m = updated.(Model)
	m = typeRunes(t, m, "i") // filtered: [git, ripgrep]

	down := tea.KeyMsg{Type: tea.KeyDown}
	up := tea.KeyMsg{Type: tea.KeyUp}

	updated, _ = m.Update(down)
	m = updated.(Model)
	if m.metaSelected != 1 {
		t.Fatalf("after down metaSelected = %d, want 1", m.metaSelected)
	}
	updated, _ = m.Update(down)
	m = updated.(Model)
	if m.metaSelected != 0 {
		t.Fatalf("after wrap-around down metaSelected = %d, want 0", m.metaSelected)
	}
	updated, _ = m.Update(up)
	m = updated.(Model)
	if m.metaSelected != 1 {
		t.Fatalf("after wrap-around up metaSelected = %d, want 1", m.metaSelected)
	}
	if m.search.Value() != "i" {
		t.Errorf("query = %q, want untouched %q", m.search.Value(), "i")
	}
	if m.mode != modeSearch {
		t.Errorf("mode = %d, want still modeSearch", m.mode)
	}
	if sel, ok := m.selectedMeta(); !ok || sel.Name != "ripgrep" {
		t.Errorf("selectedMeta = %v, want ripgrep (second match)", sel)
	}
}

// TestSearchArrowsZeroMatches verifies arrows are consumed as no-ops when the
// filter has no matches (not forwarded to the textinput).
func TestSearchArrowsZeroMatches(t *testing.T) {
	m := newSearchTestModel()

	updated, _ := m.Update(keyRunes("/"))
	m = updated.(Model)
	m = typeRunes(t, m, "zzz")

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	nm := updated.(Model)
	if nm.metaSelected != 0 {
		t.Errorf("metaSelected = %d, want unchanged 0", nm.metaSelected)
	}
	if nm.search.Value() != "zzz" {
		t.Errorf("query = %q, want untouched %q", nm.search.Value(), "zzz")
	}
}

// TestIndexOfMeta covers the full-list name lookup and its fallbacks.
func TestIndexOfMeta(t *testing.T) {
	m := newSearchTestModel()

	tests := []struct {
		name string
		arg  string
		want int
	}{
		{"found first", "fzf", 0},
		{"found last", "ripgrep", 2},
		{"missing falls back to 0", "gone", 0},
		{"empty name falls back to 0", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.indexOfMeta(tt.arg); got != tt.want {
				t.Errorf("indexOfMeta(%q) = %d, want %d", tt.arg, got, tt.want)
			}
		})
	}
}

// TestSearchLetterKeyTypesIntoQuery verifies a letter that doubles as a nav
// key in modeNormal (j) lands in the query while searching and does not act
// as navigation.
func TestSearchLetterKeyTypesIntoQuery(t *testing.T) {
	m := newSearchTestModel()

	updated, _ := m.Update(keyRunes("/"))
	m = updated.(Model)

	updated, _ = m.Update(keyRunes("j"))
	nm := updated.(Model)
	if !strings.Contains(nm.search.Value(), "j") {
		t.Errorf("search value = %q, expected the j rune to land in the query", nm.search.Value())
	}
	if nm.metaSelected != 0 {
		t.Errorf("metaSelected = %d, want 0 (typing highlights the first match)", nm.metaSelected)
	}
	if nm.mode != modeSearch {
		t.Errorf("mode = %d, want still modeSearch", nm.mode)
	}
}

// TestQuitTypedIntoSearch verifies q does not quit while the search input is
// active — the rune lands in the query instead.
func TestQuitTypedIntoSearch(t *testing.T) {
	m := newTestModel(focusTools)
	m.mode = modeSearch
	m.search = textinput.New()
	m.search.Focus()

	updated, cmd := m.Update(keyRunes("q"))
	nm := updated.(Model)
	if nm.mode != modeSearch {
		t.Fatalf("mode = %d, want modeSearch", nm.mode)
	}
	if cmd != nil {
		if _, isQuit := cmd().(tea.QuitMsg); isQuit {
			t.Error("q while searching must not quit")
		}
	}
	if !strings.Contains(nm.search.Value(), "q") {
		t.Errorf("search value = %q, expected the q rune to land in the query", nm.search.Value())
	}
}

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

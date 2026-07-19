package model

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lepeshko/keys/internal/loader"
	"github.com/lepeshko/keys/internal/updater"
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
// the search commit/rollback flow from focusTools. Sizing goes through a real
// WindowSizeMsg so toolsW/ready match what the running app renders with.
func newSearchTestModel() Model {
	m := New([]loader.ToolMeta{
		{Name: "fzf"},
		{Name: "git"},
		{Name: "ripgrep"},
	})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
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
	// The commit must fire the auto-fetch path so the card and help panel
	// populate: with an empty helpCache it marks the tool's help as loading.
	if nm.helpLoadingFor != "ripgrep" {
		t.Errorf("helpLoadingFor = %q, want %q (enter fires auto-fetch)", nm.helpLoadingFor, "ripgrep")
	}
}

// TestSearchArrowThenEnterCommitsHighlight verifies the primary flow —
// / → type → down → enter — commits the arrow-moved highlight, not the
// first match of the filter.
func TestSearchArrowThenEnterCommitsHighlight(t *testing.T) {
	m := newSearchTestModel()

	updated, _ := m.Update(keyRunes("/"))
	m = updated.(Model)
	m = typeRunes(t, m, "i") // filtered: [git, ripgrep]

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
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
		t.Errorf("selectedMeta = %v, want the arrow-highlighted ripgrep", sel)
	}
}

// TestSearchEscRestoresSelection verifies esc rolls the cursor back to the
// tool selected before the search started, discarding arrow navigation, and
// re-syncs the help panel to the restored tool.
func TestSearchEscRestoresSelection(t *testing.T) {
	m := newSearchTestModel()
	m.metaSelected = 1 // git

	updated, _ := m.Update(keyRunes("/"))
	m = updated.(Model)
	m = typeRunes(t, m, "i") // filtered: [git, ripgrep], highlight reset to 0

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // highlight ripgrep
	m = updated.(Model)
	if sel, ok := m.selectedMeta(); !ok || sel.Name != "ripgrep" {
		t.Fatalf("selectedMeta before esc = %v, want ripgrep", sel)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	nm := updated.(Model)

	if nm.mode != modeNormal {
		t.Errorf("mode = %d, want modeNormal", nm.mode)
	}
	if nm.metaSelected != 1 {
		t.Errorf("metaSelected = %d, want restored 1 (git)", nm.metaSelected)
	}
	if sel, ok := nm.selectedMeta(); !ok || sel.Name != "git" {
		t.Errorf("selectedMeta = %v, want git restored", sel)
	}
	if nm.searchPrevName != "" {
		t.Errorf("searchPrevName = %q, want cleared", nm.searchPrevName)
	}
	// The rollback must re-sync the help panel: the arrow move loaded
	// ripgrep's help, esc has to re-target the restored tool.
	if nm.helpLoadingFor != "git" {
		t.Errorf("helpLoadingFor = %q, want %q (esc refreshes the help panel)", nm.helpLoadingFor, "git")
	}
}

// TestSearchEscCachedHelpNotStuckLoading reproduces the stuck-"Loading..."
// sequence: an arrow move in search onto a tool with uncached help fires a
// help fetch; esc then rolls back to a tool whose help IS cached while that
// fetch is still in flight. The help panel must show the restored tool's
// cached help immediately, and the stale fetch landing later (for a tool that
// is no longer selected) must not leave the panel on "Loading...".
func TestSearchEscCachedHelpNotStuckLoading(t *testing.T) {
	m := newSearchTestModel()
	m.metaSelected = 1 // git
	m.helpCache["git"] = [2]string{helpModeHelp: "GITHELP"}

	updated, _ := m.Update(keyRunes("/"))
	m = updated.(Model)
	m = typeRunes(t, m, "rip") // filtered: [ripgrep]

	// Arrow move onto ripgrep (uncached) fires its help fetch.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.helpLoadingFor != "ripgrep" {
		t.Fatalf("helpLoadingFor = %q, want %q (arrow move fires auto-fetch)", m.helpLoadingFor, "ripgrep")
	}

	// esc rolls back to git while ripgrep's fetch is still in flight: the
	// cached help must render, not "Loading..." for the foreign fetch.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	view := ansiCSI.ReplaceAllString(m.helpViewport.View(), "")
	if strings.Contains(view, "Loading") {
		t.Errorf("help panel after esc = %q, want git's cached help, not Loading", view)
	}
	if !strings.Contains(view, "GITHELP") {
		t.Errorf("help panel after esc = %q, want cached %q", view, "GITHELP")
	}
	// The stale in-flight fetch still belongs to ripgrep, not git.
	if m.helpLoadingFor != "ripgrep" {
		t.Errorf("helpLoadingFor = %q, want still %q (fetch in flight)", m.helpLoadingFor, "ripgrep")
	}

	// The stale fetch lands for the no-longer-selected tool: it must cache
	// its output without disturbing the panel showing git's help.
	updated, _ = m.Update(helpOutputMsg{toolName: "ripgrep", mode: helpModeHelp, output: "RGHELP"})
	nm := updated.(Model)
	view = ansiCSI.ReplaceAllString(nm.helpViewport.View(), "")
	if strings.Contains(view, "Loading") {
		t.Errorf("help panel after stale fetch = %q, stuck on Loading", view)
	}
	if !strings.Contains(view, "GITHELP") {
		t.Errorf("help panel after stale fetch = %q, want git's cached %q", view, "GITHELP")
	}
	if nm.helpLoadingFor != "" {
		t.Errorf("helpLoadingFor = %q, want cleared by its own result", nm.helpLoadingFor)
	}
	if got := nm.helpCache["ripgrep"][helpModeHelp]; got != "RGHELP" {
		t.Errorf("ripgrep cache = %q, want %q (stale result still cached)", got, "RGHELP")
	}
}

// TestSearchTypingResyncsHelpPanel reproduces the transient misleading
// "Loading..." during search typing: an arrow move onto a tool with uncached
// help fires a fetch and paints "Loading...", then a query keystroke resets
// the highlight to the first match (a cached tool). The help panel must
// repaint for the new selection immediately — and the stale fetch landing for
// the now-unselected tool must not leave "Loading..." on screen.
func TestSearchTypingResyncsHelpPanel(t *testing.T) {
	m := newSearchTestModel()
	m.helpCache["git"] = [2]string{helpModeHelp: "GITHELP"}

	updated, _ := m.Update(keyRunes("/"))
	m = updated.(Model)
	m = typeRunes(t, m, "i") // filtered: [git, ripgrep]

	// Arrow onto ripgrep (uncached) fires its help fetch; panel shows Loading.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.helpLoadingFor != "ripgrep" {
		t.Fatalf("helpLoadingFor = %q, want %q (arrow move fires auto-fetch)", m.helpLoadingFor, "ripgrep")
	}

	// Narrow the query: highlight resets to git (filtered: [git]) while
	// ripgrep's fetch is still in flight. The panel must show git's cached
	// help, not the unselected tool's "Loading...".
	m = typeRunes(t, m, "t")
	if sel, ok := m.selectedMeta(); !ok || sel.Name != "git" {
		t.Fatalf("selectedMeta after narrowing = %v, want git", sel)
	}
	view := ansiCSI.ReplaceAllString(m.helpViewport.View(), "")
	if strings.Contains(view, "Loading") {
		t.Errorf("help panel after query change = %q, want git's cached help, not Loading", view)
	}
	if !strings.Contains(view, "GITHELP") {
		t.Errorf("help panel after query change = %q, want cached %q", view, "GITHELP")
	}

	// The stale fetch lands for the no-longer-selected ripgrep: it must cache
	// quietly without repainting Loading over git's help.
	updated, _ = m.Update(helpOutputMsg{toolName: "ripgrep", mode: helpModeHelp, output: "RGHELP"})
	nm := updated.(Model)
	view = ansiCSI.ReplaceAllString(nm.helpViewport.View(), "")
	if strings.Contains(view, "Loading") {
		t.Errorf("help panel after stale fetch = %q, stuck on Loading", view)
	}
	if !strings.Contains(view, "GITHELP") {
		t.Errorf("help panel after stale fetch = %q, want git's cached %q", view, "GITHELP")
	}
	if nm.helpLoadingFor != "" {
		t.Errorf("helpLoadingFor = %q, want cleared by its own result", nm.helpLoadingFor)
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
	// Every arrow move must fire the auto-fetch path for the newly
	// highlighted tool (helpCache is empty, so it marks help as loading).
	if m.helpLoadingFor != "ripgrep" {
		t.Errorf("helpLoadingFor = %q, want %q (arrow move fires auto-fetch)", m.helpLoadingFor, "ripgrep")
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

// TestIndexOfMeta covers the displayed-order name lookup (m.filteredMeta, i.e.
// the grouped/filtered projection — not the raw m.meta) and its fallbacks.
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

	// An updatable tool is grouped ahead of a meta.yaml-earlier one: indexOfMeta
	// must return the *displayed* index, not the m.meta index. ripgrep (meta idx
	// 2) has an update → row 0; fzf (meta idx 0) → row 1.
	grouped := newSearchTestModel()
	grouped.versions["ripgrep"] = VersionInfo{Installed: "1.0", Latest: "2.0", InstalledKnown: true}
	if got := grouped.indexOfMeta("ripgrep"); got != 0 {
		t.Errorf("indexOfMeta(ripgrep) grouped = %d, want 0 (lifted to top)", got)
	}
	if got := grouped.indexOfMeta("fzf"); got != 1 {
		t.Errorf("indexOfMeta(fzf) grouped = %d, want 1 (below the updatable row)", got)
	}
}

// TestSearchCommitRollbackWithGrouping verifies both search exits land on the
// right tool when grouping has reordered the displayed list: the cursor is
// remapped by name through indexOfMeta (the displayed projection), so commit and
// rollback resolve the correct row even though it differs from the m.meta index.
func TestSearchCommitRollbackWithGrouping(t *testing.T) {
	metas := []loader.ToolMeta{{Name: "aa"}, {Name: "bb"}, {Name: "cc"}}
	// bb has an update → displayed order is [bb, aa, cc].
	m := updatableModel(t, metas, "bb")
	m.metaSelected = 2 // cc (displayed idx 2)

	// Commit: search "cc", enter → cursor on cc at its displayed index (2).
	updated, _ := m.Update(keyRunes("/"))
	sm := updated.(Model)
	sm = typeRunes(t, sm, "cc")
	updated, _ = sm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nm := updated.(Model)
	if sel, ok := nm.selectedMeta(); !ok || sel.Name != "cc" || nm.metaSelected != 2 {
		t.Errorf("commit landed on %v (idx %d), want cc at displayed idx 2", sel, nm.metaSelected)
	}

	// Rollback: from cc, search "aa", esc → cursor restored to cc (displayed 2).
	updated, _ = m.Update(keyRunes("/"))
	sm = updated.(Model)
	if sm.searchPrevName != "cc" {
		t.Fatalf("searchPrevName = %q, want cc", sm.searchPrevName)
	}
	sm = typeRunes(t, sm, "aa")
	updated, _ = sm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	nm = updated.(Model)
	if sel, ok := nm.selectedMeta(); !ok || sel.Name != "cc" || nm.metaSelected != 2 {
		t.Errorf("rollback landed on %v (idx %d), want cc at displayed idx 2", sel, nm.metaSelected)
	}
}

// TestSearchEmptyToolList verifies the whole search transaction is safe with
// no tracked tools: `/` opens with an empty rollback anchor, enter and arrows
// are no-ops, and esc closes cleanly with the cursor at 0.
func TestSearchEmptyToolList(t *testing.T) {
	m := New(nil)
	m.width = 80
	m.height = 24
	m.focus = focusTools

	updated, _ := m.Update(keyRunes("/"))
	m = updated.(Model)
	if m.mode != modeSearch {
		t.Fatalf("mode = %d, want modeSearch", m.mode)
	}
	if m.searchPrevName != "" {
		t.Errorf("searchPrevName = %q, want empty for an empty list", m.searchPrevName)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.mode != modeSearch {
		t.Errorf("after enter mode = %d, want still modeSearch (no matches)", m.mode)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.metaSelected != 0 {
		t.Errorf("after down metaSelected = %d, want unchanged 0", m.metaSelected)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.mode != modeNormal {
		t.Errorf("after esc mode = %d, want modeNormal", m.mode)
	}
	if m.metaSelected != 0 {
		t.Errorf("after esc metaSelected = %d, want 0", m.metaSelected)
	}
}

// TestSearchSingleMatchWrapAround verifies arrows on a single-match filter
// wrap onto the same tool (modular move over n=1) without touching the query.
func TestSearchSingleMatchWrapAround(t *testing.T) {
	m := newSearchTestModel()

	updated, _ := m.Update(keyRunes("/"))
	m = updated.(Model)
	m = typeRunes(t, m, "rip") // filtered: [ripgrep]

	for _, key := range []tea.KeyMsg{{Type: tea.KeyDown}, {Type: tea.KeyUp}} {
		updated, _ = m.Update(key)
		m = updated.(Model)
		if m.metaSelected != 0 {
			t.Errorf("after %s metaSelected = %d, want 0 (single match wraps onto itself)", key, m.metaSelected)
		}
	}
	if sel, ok := m.selectedMeta(); !ok || sel.Name != "ripgrep" {
		t.Errorf("selectedMeta = %v, want ripgrep", sel)
	}
	if m.search.Value() != "rip" {
		t.Errorf("query = %q, want untouched %q", m.search.Value(), "rip")
	}
}

// TestSearchMatchesByTag verifies the search predicate matches tags in
// addition to names: a tag-only match enters the filter flagged byTagOnly
// with the (case-insensitively) matching tag, and filteredMeta projects the
// same filtered list.
func TestSearchMatchesByTag(t *testing.T) {
	m := New([]loader.ToolMeta{
		{Name: "fzf", Tags: []string{"fuzzy", "finder"}},
		{Name: "lazygit", Tags: []string{"git", "TUI"}},
		{Name: "ripgrep"},
	})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.focus = focusTools

	updated, _ = m.Update(keyRunes("/"))
	m = updated.(Model)
	m = typeRunes(t, m, "tui") // lazygit matches only via its TUI tag

	matches := m.searchMatches()
	if len(matches) != 1 {
		t.Fatalf("searchMatches = %d entries, want 1", len(matches))
	}
	if got := matches[0]; got.meta.Name != "lazygit" || !got.byTagOnly || got.tag != "TUI" {
		t.Errorf("match = {%s byTagOnly=%v tag=%q}, want lazygit tag-only via TUI",
			got.meta.Name, got.byTagOnly, got.tag)
	}
	if got := m.filteredMeta(); len(got) != 1 || got[0].Name != "lazygit" {
		t.Errorf("filteredMeta = %v, want [lazygit]", got)
	}
}

// TestSearchNameMatchNotTagFlagged verifies a row whose name matches is never
// flagged byTagOnly, even when one of its tags matches the query too.
func TestSearchNameMatchNotTagFlagged(t *testing.T) {
	m := New([]loader.ToolMeta{
		{Name: "lazygit", Tags: []string{"git"}},
	})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.focus = focusTools

	updated, _ = m.Update(keyRunes("/"))
	m = updated.(Model)
	m = typeRunes(t, m, "git") // name and tag both contain "git"

	matches := m.searchMatches()
	if len(matches) != 1 {
		t.Fatalf("searchMatches = %d entries, want 1", len(matches))
	}
	if got := matches[0]; got.byTagOnly {
		t.Errorf("match = {%s byTagOnly=%v tag=%q}, name match must win over the tag",
			got.meta.Name, got.byTagOnly, got.tag)
	}
}

// TestSearchLetterKeyTypesIntoQuery verifies a letter that doubles as a nav
// key in modeNormal (j) lands in the query while searching and does not act
// as navigation — and that typing a query-changing rune resets an
// arrow-moved highlight to the first match (a stale index could fall out of
// the narrower filter's range and make enter a silent no-op).
func TestSearchLetterKeyTypesIntoQuery(t *testing.T) {
	m := newSearchTestModel()

	updated, _ := m.Update(keyRunes("/"))
	m = updated.(Model)
	m = typeRunes(t, m, "i") // filtered: [git, ripgrep]

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // arrow-move to index 1
	m = updated.(Model)
	if m.metaSelected != 1 {
		t.Fatalf("after down metaSelected = %d, want 1", m.metaSelected)
	}

	updated, _ = m.Update(keyRunes("j"))
	nm := updated.(Model)
	if nm.search.Value() != "ij" {
		t.Errorf("search value = %q, expected the j rune to land in the query (want %q)", nm.search.Value(), "ij")
	}
	if nm.metaSelected != 0 {
		t.Errorf("metaSelected = %d, want 0 (typing resets the highlight to the first match)", nm.metaSelected)
	}
	if nm.mode != modeSearch {
		t.Errorf("mode = %d, want still modeSearch", nm.mode)
	}
}

// TestSearchCursorKeyKeepsHighlight verifies pure cursor movement inside the
// query (left/right/home/end) does not reset an arrow-moved highlight — only
// a keystroke that actually changes the query text does.
func TestSearchCursorKeyKeepsHighlight(t *testing.T) {
	m := newSearchTestModel()

	updated, _ := m.Update(keyRunes("/"))
	m = updated.(Model)
	m = typeRunes(t, m, "i") // filtered: [git, ripgrep]

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // arrow-move to index 1
	m = updated.(Model)
	if m.metaSelected != 1 {
		t.Fatalf("after down metaSelected = %d, want 1", m.metaSelected)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft}) // move the input cursor
	nm := updated.(Model)
	if nm.metaSelected != 1 {
		t.Errorf("metaSelected = %d, want 1 kept (query unchanged)", nm.metaSelected)
	}
	if nm.search.Value() != "i" {
		t.Errorf("query = %q, want untouched %q", nm.search.Value(), "i")
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

// TestFocusDigitHotkeys drives the 1/2/3 hotkeys from every starting focus,
// including the tools→help jump the arrows cannot do in one step.
func TestFocusDigitHotkeys(t *testing.T) {
	tests := []struct {
		name string
		from int
		key  string
		want int
	}{
		{"tools to help", focusTools, "3", focusHelp},
		{"help to tools", focusHelp, "1", focusTools},
		{"tools to brief", focusTools, "2", focusBrief},
		{"brief to help", focusBrief, "3", focusHelp},
		{"help to brief", focusHelp, "2", focusBrief},
		{"brief to tools", focusBrief, "1", focusTools},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(tt.from)

			updated, _ := m.Update(keyRunes(tt.key))
			if got := updated.(Model).focus; got != tt.want {
				t.Errorf("after %q focus = %d, want %d", tt.key, got, tt.want)
			}
		})
	}
}

// TestFocusArrowKeys covers the arrow/l walk: one panel per press, no wrap at
// either edge.
func TestFocusArrowKeys(t *testing.T) {
	right := tea.KeyMsg{Type: tea.KeyRight}
	left := tea.KeyMsg{Type: tea.KeyLeft}

	tests := []struct {
		name string
		from int
		key  tea.KeyMsg
		want int
	}{
		{"right tools to brief", focusTools, right, focusBrief},
		{"right brief to help", focusBrief, right, focusHelp},
		{"right at help does not wrap", focusHelp, right, focusHelp},
		{"l tools to brief", focusTools, keyRunes("l"), focusBrief},
		{"left help to brief", focusHelp, left, focusBrief},
		{"left brief to tools", focusBrief, left, focusTools},
		{"left at tools does not wrap", focusTools, left, focusTools},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(tt.from)

			updated, _ := m.Update(tt.key)
			if got := updated.(Model).focus; got != tt.want {
				t.Errorf("focus = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestEscWalksFocusThenQuits verifies esc steps left panel by panel and only
// quits off the left edge (focusTools).
func TestEscWalksFocusThenQuits(t *testing.T) {
	esc := tea.KeyMsg{Type: tea.KeyEsc}

	m := newTestModel(focusHelp)
	for _, want := range []int{focusBrief, focusTools} {
		updated, cmd := m.Update(esc)
		m = updated.(Model)
		if m.focus != want {
			t.Fatalf("focus = %d, want %d", m.focus, want)
		}
		if cmd != nil {
			if _, isQuit := cmd().(tea.QuitMsg); isQuit {
				t.Fatalf("esc quit from focus %d, want a focus move", want)
			}
		}
	}

	_, cmd := m.Update(esc)
	if cmd == nil {
		t.Fatal("esc from focusTools returned no cmd, want quit")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Error("esc from focusTools did not quit")
	}
}

// TestFocusDigitSamePanelNoop verifies the digit of the already-focused panel
// leaves the focus (and the mode) alone.
func TestFocusDigitSamePanelNoop(t *testing.T) {
	m := newTestModel(focusBrief)

	updated, _ := m.Update(keyRunes("2"))
	nm := updated.(Model)
	if nm.focus != focusBrief {
		t.Errorf("focus = %d, want focusBrief unchanged", nm.focus)
	}
	if nm.mode != modeNormal {
		t.Errorf("mode = %d, want modeNormal", nm.mode)
	}
}

// TestFocusDigitTypedIntoSearch verifies a digit is query text while searching,
// not a focus jump — the mode dispatch owns the input.
func TestFocusDigitTypedIntoSearch(t *testing.T) {
	m := newTestModel(focusTools)
	m.mode = modeSearch
	m.search = textinput.New()
	m.search.Focus()

	updated, _ := m.Update(keyRunes("3"))
	nm := updated.(Model)
	if nm.focus != focusTools {
		t.Errorf("focus = %d, want focusTools unchanged while searching", nm.focus)
	}
	if nm.search.Value() != "3" {
		t.Errorf("search value = %q, expected the 3 rune to land in the query", nm.search.Value())
	}
}

// newUpdateTestModel builds a focusBrief model with one tool that has a pending
// release (installed older than latest, so hasUpdate is true). Sizing goes
// through a real WindowSizeMsg so the card/status bar render as in the app.
func newUpdateTestModel() Model {
	m := New([]loader.ToolMeta{{Name: "rg", GitHub: "github.com/BurntSushi/ripgrep"}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.focus = focusBrief
	m.versions["rg"] = VersionInfo{Installed: "1.0.0", Latest: "2.0.0", InstalledKnown: true}
	return m
}

// TestUpdateKeyWithoutUpdate: [u] in focusBrief on a tool with no pending
// release sets a hint and does not enter the confirm mode.
func TestUpdateKeyWithoutUpdate(t *testing.T) {
	m := newUpdateTestModel()
	m.versions["rg"] = VersionInfo{Installed: "2.0.0", Latest: "2.0.0", InstalledKnown: true}

	updated, _ := m.Update(keyRunes("u"))
	nm := updated.(Model)
	if nm.mode != modeNormal {
		t.Fatalf("mode = %d, want modeNormal", nm.mode)
	}
	if !strings.Contains(nm.statusMsg, "no update available") {
		t.Errorf("statusMsg = %q, want a 'no update available' hint", nm.statusMsg)
	}
}

// TestUpdateKeyWhileUpdatingNoop: [u] while an update is already running is a
// no-op — one update at a time, no queue.
func TestUpdateKeyWhileUpdatingNoop(t *testing.T) {
	m := newUpdateTestModel()
	m.updatingFor = "rg"

	updated, cmd := m.Update(keyRunes("u"))
	nm := updated.(Model)
	if nm.mode != modeNormal {
		t.Errorf("mode = %d, want modeNormal (no confirm)", nm.mode)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil (no detection while updating)", cmd)
	}
	if nm.statusMsg != "" {
		t.Errorf("statusMsg = %q, want empty", nm.statusMsg)
	}
}

// TestUpdateDetectedEntersConfirm: a successful detection for the selected tool
// stores the plan and opens modeConfirmUpdate; the status bar shows the command.
func TestUpdateDetectedEntersConfirm(t *testing.T) {
	m := newUpdateTestModel()
	plan := updater.Plan{Manager: "brew", Argv: []string{"brew", "upgrade", "ripgrep"}, Display: "brew upgrade ripgrep"}

	updated, _ := m.Update(updateDetectedMsg{tool: "rg", plan: plan})
	nm := updated.(Model)
	if nm.mode != modeConfirmUpdate {
		t.Fatalf("mode = %d, want modeConfirmUpdate", nm.mode)
	}
	if nm.updatePlan.Display != "brew upgrade ripgrep" {
		t.Errorf("updatePlan.Display = %q, want the detected command", nm.updatePlan.Display)
	}
	if bar := nm.renderStatusBar(); !strings.Contains(bar, "brew upgrade ripgrep") {
		t.Errorf("status bar = %q, want it to show the plan command", bar)
	}
}

// TestUpdateDetectedStaleDropped: a detection result for a tool that is no
// longer selected is ignored.
func TestUpdateDetectedStaleDropped(t *testing.T) {
	m := newUpdateTestModel()
	plan := updater.Plan{Display: "brew upgrade other", Argv: []string{"true"}}

	updated, _ := m.Update(updateDetectedMsg{tool: "other", plan: plan})
	nm := updated.(Model)
	if nm.mode != modeNormal {
		t.Errorf("mode = %d, want modeNormal (stale msg dropped)", nm.mode)
	}
	if nm.updatePlan.Display != "" {
		t.Errorf("updatePlan.Display = %q, want empty (plan not stored)", nm.updatePlan.Display)
	}
}

// TestUpdateDetectedUnknownManager: ErrUnknownManager does not open a dead-end
// dialog — it shows a hint and stays in modeNormal.
func TestUpdateDetectedUnknownManager(t *testing.T) {
	m := newUpdateTestModel()

	updated, _ := m.Update(updateDetectedMsg{tool: "rg", err: updater.ErrUnknownManager})
	nm := updated.(Model)
	if nm.mode != modeNormal {
		t.Fatalf("mode = %d, want modeNormal", nm.mode)
	}
	if !strings.Contains(nm.statusMsg, "no known updater") {
		t.Errorf("statusMsg = %q, want a 'no known updater' hint", nm.statusMsg)
	}
}

// TestUpdateKeyFiresDetect: [u] in focusBrief on a tool with a pending release
// fires detection (returns a non-nil command) and stays in modeNormal — the
// confirm dialog opens only after the async updateDetectedMsg lands.
func TestUpdateKeyFiresDetect(t *testing.T) {
	m := newUpdateTestModel()

	updated, cmd := m.Update(keyRunes("u"))
	nm := updated.(Model)
	if nm.mode != modeNormal {
		t.Errorf("mode = %d, want modeNormal (detection is async)", nm.mode)
	}
	if cmd == nil {
		t.Error("cmd = nil, want the detection command")
	}
	if nm.statusMsg != "" {
		t.Errorf("statusMsg = %q, want empty (no error, no hint)", nm.statusMsg)
	}
}

// TestUpdateDetectedGenericError: a non-ErrUnknownManager detection error shows
// the "update detect failed" status and stays in modeNormal (no confirm).
func TestUpdateDetectedGenericError(t *testing.T) {
	m := newUpdateTestModel()

	updated, _ := m.Update(updateDetectedMsg{tool: "rg", err: errors.New("boom")})
	nm := updated.(Model)
	if nm.mode != modeNormal {
		t.Fatalf("mode = %d, want modeNormal", nm.mode)
	}
	if !strings.Contains(nm.statusMsg, "update detect failed") {
		t.Errorf("statusMsg = %q, want an 'update detect failed' hint", nm.statusMsg)
	}
	if nm.updatePlan.Display != "" {
		t.Errorf("updatePlan.Display = %q, want empty (no plan stored)", nm.updatePlan.Display)
	}
}

// TestUpdateConfirmEnterStarts: enter in modeConfirmUpdate launches the update —
// sets updatingFor/updateLogFor, resets the log, and returns a command.
func TestUpdateConfirmEnterStarts(t *testing.T) {
	m := newUpdateTestModel()
	m.mode = modeConfirmUpdate
	m.updatePlan = updater.Plan{Argv: []string{"true"}, Display: "true"}
	m.updateLog = []string{"stale"}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nm := updated.(Model)
	if nm.mode != modeNormal {
		t.Errorf("mode = %d, want modeNormal after enter", nm.mode)
	}
	if nm.updatingFor != "rg" {
		t.Errorf("updatingFor = %q, want %q", nm.updatingFor, "rg")
	}
	if nm.updateLogFor != "rg" {
		t.Errorf("updateLogFor = %q, want %q", nm.updateLogFor, "rg")
	}
	if len(nm.updateLog) != 0 {
		t.Errorf("updateLog = %v, want reset to empty", nm.updateLog)
	}
	if cmd == nil {
		t.Error("cmd = nil, want a start+spinner command")
	}
}

// TestUpdateConfirmEscCancels: esc in modeConfirmUpdate returns to modeNormal
// without starting anything.
func TestUpdateConfirmEscCancels(t *testing.T) {
	m := newUpdateTestModel()
	m.mode = modeConfirmUpdate
	m.updatePlan = updater.Plan{Argv: []string{"true"}, Display: "true"}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	nm := updated.(Model)
	if nm.mode != modeNormal {
		t.Errorf("mode = %d, want modeNormal", nm.mode)
	}
	if nm.updatingFor != "" {
		t.Errorf("updatingFor = %q, want empty (nothing started)", nm.updatingFor)
	}
}

// TestSpinnerTicksWhileUpdating: the spinner tick loop keeps rescheduling while
// updatingFor is set (without it the card spinner freezes after one frame).
func TestSpinnerTicksWhileUpdating(t *testing.T) {
	m := newUpdateTestModel()
	m.updatingFor = "rg"

	updated, cmd := m.Update(m.spinner.Tick())
	nm := updated.(Model)
	if cmd == nil {
		t.Error("cmd = nil, want the tick loop to keep ticking while updating")
	}
	_ = nm

	// Once both refresh and update are idle, the loop stops.
	m.updatingFor = ""
	m.refreshingFor = ""
	_, cmd2 := m.Update(m.spinner.Tick())
	if cmd2 != nil {
		t.Error("cmd = non-nil, want the tick loop to stop when idle")
	}
}

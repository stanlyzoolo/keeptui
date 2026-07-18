package model

import (
	"errors"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"

	"github.com/lepeshko/keys/internal/loader"
	"github.com/lepeshko/keys/internal/logx"
	"github.com/lepeshko/keys/internal/ui"
	"github.com/lepeshko/keys/internal/version"
)

// TestUpdateViewNoPanicNoLog confirms a normal Update/View cycle writes no log
// file — logx.Recover is a no-op without a panic in flight, so View being hot
// does not create log churn.
func TestUpdateViewNoPanicNoLog(t *testing.T) {
	logDir := t.TempDir()
	restore := logx.SetDirForTesting(logDir)
	defer restore()

	m := New([]loader.ToolMeta{{Name: "git", Tags: []string{"vcs"}}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	_ = m.View()

	if entries, err := os.ReadDir(logDir); err == nil {
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "keeptui-") {
				t.Errorf("normal Update/View cycle created a log file: %s", e.Name())
			}
		}
	}
}

func TestWrapText(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		width int
		want  string
	}{
		{"fits", "hello world", 100, "hello world"},
		{"wraps on word boundary", "aaa bbb ccc", 7, "aaa bbb\nccc"},
		{"zero width returns input", "x y z", 0, "x y z"},
		{"keeps existing newlines", "ab\ncd", 100, "ab\ncd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wrapText(tt.in, tt.width); got != tt.want {
				t.Errorf("wrapText(%q, %d) = %q, want %q", tt.in, tt.width, got, tt.want)
			}
		})
	}
}

func TestFormatStars(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1.0k"},
		{1500, "1.5k"},
		{59400, "59.4k"},
	}
	for _, tt := range tests {
		if got := formatStars(tt.in); got != tt.want {
			t.Errorf("formatStars(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestLanguagePercents(t *testing.T) {
	t.Run("empty returns nil", func(t *testing.T) {
		if got := languagePercents(nil); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
		if got := languagePercents(map[string]int{}); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("sorted descending with correct percent", func(t *testing.T) {
		got := languagePercents(map[string]int{"Go": 3, "Rust": 1})
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		if got[0].Name != "Go" || got[0].Pct != 75 {
			t.Errorf("got[0] = %+v, want {Go 75}", got[0])
		}
		if got[1].Name != "Rust" || got[1].Pct != 25 {
			t.Errorf("got[1] = %+v, want {Rust 25}", got[1])
		}
	})

	t.Run("caps at top 5", func(t *testing.T) {
		langs := map[string]int{"a": 6, "b": 5, "c": 4, "d": 3, "e": 2, "f": 1}
		if got := languagePercents(langs); len(got) != 5 {
			t.Errorf("len = %d, want 5", len(got))
		}
	})
}

func TestCleanTerminalOutput(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "no change", "no change"},
		{"backspace overstrike removes prev rune", "a\bb", "b"},
		{"bold overstrike (man bold)", "N\bNA\bA", "NA"},
		{"carriage return dropped", "x\ry", "xy"},
		{"strips ANSI escapes", "\x1b[1mhi\x1b[0m", "hi"},
		// A TUI tool probed with --help can leave terminal-state escapes in
		// its captured output (the inertia incident): re-emitting them from
		// the help viewport flips the real terminal out of the alt screen.
		{"strips private-mode CSI (alt screen)", "\x1b[?1049lpanic: boom", "panic: boom"},
		{"strips OSC title", "\x1b]0;title\x07text", "text"},
		{"drops stray control chars, keeps \\n and \\t", "a\x07b\fc\nd\te", "abc\nd\te"},
		{"drops lone ESC from a truncated sequence", "cut\x1b", "cut"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanTerminalOutput(tt.in); got != tt.want {
				t.Errorf("cleanTerminalOutput(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// forceColorProfile forces truecolor so lipgloss actually emits ANSI escapes
// (a non-TTY test run strips them and hides regressions), restoring the
// previous profile on cleanup so the global doesn't leak into later tests.
func forceColorProfile(t *testing.T) {
	t.Helper()
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
}

func TestColorizeHelp(t *testing.T) {
	forceColorProfile(t)

	// A dash inside a word (e.g. "golangci-lint") must not be styled as a
	// short flag, which would inject an ANSI escape mid-word.
	got := colorizeHelp("golangci-lint runs linters")
	if strings.Contains(got, "golangci\x1b") {
		t.Errorf("colorizeHelp injected escape inside word: %q", got)
	}

	// A real flag preceded by whitespace should still be styled.
	got = colorizeHelp("use --verbose for details")
	if !strings.Contains(got, "\x1b") {
		t.Errorf("colorizeHelp did not style a real flag: %q", got)
	}

	// A flag followed by a [bracket]/<angle> meta token must not be corrupted:
	// the meta regex must never match the '[' inside the flag's ANSI escape,
	// which would leave a visible "[38;2;…m" and doubled ESC bytes.
	for _, in := range []string{
		"--foo [bar] enable it",
		"usage: tool [options] --verbose",
		"see <arg> and --flag here",
	} {
		got := colorizeHelp(in)
		if strings.Contains(got, "\x1b\x1b") {
			t.Errorf("colorizeHelp produced doubled ESC for %q: %q", in, got)
		}
		// After stripping every valid CSI escape, no raw "[…m" may remain.
		if leftover := ansiCSI.ReplaceAllString(got, ""); strings.Contains(leftover, "[38;") {
			t.Errorf("colorizeHelp leaked a stripped-ESC escape for %q: %q", in, got)
		}
	}
}

var ansiCSI = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestStripMarkdown(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"## Hello", "Hello"},
		{"**bold**", "bold"},
		{"`code`", "code"},
	}
	for _, tt := range tests {
		if got := stripMarkdown(tt.in); got != tt.want {
			t.Errorf("stripMarkdown(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRenderRepoStatus(t *testing.T) {
	tests := []struct {
		status string
		want   []string // substrings that must be present
	}{
		{"active", []string{"●", "active"}},
		{"archived", []string{"⚠", "archived"}},
		{"weird", []string{"weird"}},
	}
	for _, tt := range tests {
		got := renderRepoStatus(tt.status)
		for _, sub := range tt.want {
			if !strings.Contains(got, sub) {
				t.Errorf("renderRepoStatus(%q) = %q, missing %q", tt.status, got, sub)
			}
		}
	}
}

func TestRenderLangBar(t *testing.T) {
	t.Run("lowercases names and shows percent", func(t *testing.T) {
		got := renderLangBar(map[string]int{"Go": 1}, 40, 0)
		if !strings.Contains(got, "go") {
			t.Errorf("expected lowercase 'go' in %q", got)
		}
		if strings.Contains(got, "Go") {
			t.Errorf("expected no uppercase 'Go' in %q", got)
		}
		if !strings.Contains(got, "100%") {
			t.Errorf("expected '100%%' in %q", got)
		}
	})

	t.Run("wraps when over width", func(t *testing.T) {
		langs := map[string]int{"alpha": 30, "bravo": 25, "charlie": 25, "delta": 20}
		got := renderLangBar(langs, 12, 0)
		if !strings.Contains(got, "\n") {
			t.Errorf("expected wrapping (newline) in %q", got)
		}
	})

	t.Run("empty returns empty", func(t *testing.T) {
		if got := renderLangBar(nil, 40, 0); got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})
}

func TestFindMatches(t *testing.T) {
	if got := findMatches("a\nb\na", "a"); len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Errorf("findMatches = %v, want [0 2]", got)
	}
	if got := findMatches("x", "y"); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
	if got := findMatches("anything", ""); got != nil {
		t.Errorf("empty query should match nothing, got %v", got)
	}
}

func TestRenderStatusBarFocusTools(t *testing.T) {
	m := Model{width: 80, focus: focusTools}
	got := m.renderStatusBar()

	for _, want := range []string{"search", "track", "quit"} {
		if !strings.Contains(got, want) {
			t.Errorf("focusTools status bar = %q, missing %q", got, want)
		}
	}
	for _, absent := range []string{"filter", "github", "check", "navigate"} {
		if strings.Contains(got, absent) {
			t.Errorf("focusTools status bar = %q, should not contain %q", got, absent)
		}
	}
}

func TestRenderStatusBarFocusBrief(t *testing.T) {
	m := Model{width: 80, focus: focusBrief}
	got := m.renderStatusBar()

	for _, want := range []string{"[o]", "[c]", "[s]", "[e]", "[t]", "[q]"} {
		if !strings.Contains(got, want) {
			t.Errorf("focusBrief status bar = %q, missing %q", got, want)
		}
	}
	for _, absent := range []string{"scroll", "help", "back"} {
		if strings.Contains(got, absent) {
			t.Errorf("focusBrief status bar = %q, should not contain %q", got, absent)
		}
	}
}

// TestRenderStatusBarSearch verifies the modeSearch bar still echoes the live
// query and shows the commit/rollback hints: enter open, arrows move, esc
// cancel.
func TestRenderStatusBarSearch(t *testing.T) {
	m := newSearchTestModel()
	updated, _ := m.Update(keyRunes("/"))
	m = updated.(Model)
	m = typeRunes(t, m, "rip")

	got := m.renderStatusBar()
	for _, want := range []string{"rip", "[enter]", "open", "[↑/↓]", "move", "[esc]", "cancel"} {
		if !strings.Contains(got, want) {
			t.Errorf("search status bar = %q, missing %q", got, want)
		}
	}
}

// TestRenderStatusBarSearchCounter verifies the N/M match counter in the
// search bar: matches over the full list size, including 0/M when the query
// filters everything out.
func TestRenderStatusBarSearchCounter(t *testing.T) {
	m := newSearchTestModel() // fzf, git, ripgrep
	updated, _ := m.Update(keyRunes("/"))
	m = updated.(Model)

	m = typeRunes(t, m, "rip")
	if got := m.renderStatusBar(); !strings.Contains(got, "1/3") {
		t.Errorf("search status bar = %q, want counter 1/3", got)
	}

	m = typeRunes(t, m, "zzz")
	if got := m.renderStatusBar(); !strings.Contains(got, "0/3") {
		t.Errorf("search status bar = %q, want counter 0/3", got)
	}
}

// TestRenderLeftContentSearchMarker verifies the selection marker stays
// visible while searching and follows the arrow-moved highlight through the
// filtered list.
func TestRenderLeftContentSearchMarker(t *testing.T) {
	m := newSearchTestModel()
	updated, _ := m.Update(keyRunes("/"))
	m = updated.(Model)
	m = typeRunes(t, m, "g") // matches git and ripgrep

	lines := strings.Split(m.renderLeftContent(), "\n")
	if len(lines) < 2 {
		t.Fatalf("renderLeftContent = %q, want at least 2 rows", lines)
	}
	if !strings.Contains(lines[0], "⏺") || !strings.Contains(lines[0], "git") {
		t.Errorf("first match row = %q, want marker on git", lines[0])
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	lines = strings.Split(m.renderLeftContent(), "\n")
	if strings.Contains(lines[0], "⏺") {
		t.Errorf("first row = %q, marker should move away after down", lines[0])
	}
	if !strings.Contains(lines[1], "⏺") || !strings.Contains(lines[1], "ripgrep") {
		t.Errorf("second match row = %q, want marker on ripgrep", lines[1])
	}
}

// TestRenderLeftContentTagMatchSuffix verifies rows that matched only by tag
// show the dim #tag suffix, name-matched rows do not, and the suffix is
// dropped (without panicking) when the name column is too narrow for it.
func TestRenderLeftContentTagMatchSuffix(t *testing.T) {
	m := New([]loader.ToolMeta{
		{Name: "gitui", Tags: []string{"git"}},
		{Name: "lazygit", Tags: []string{"tui"}},
	})
	// 100 cols → toolsW 18: wide enough for "lazygit #tui" (at 80 the layout
	// minimums squeeze toolsW to 14 and the suffix is legitimately dropped).
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = updated.(Model)
	m.focus = focusTools
	updated, _ = m.Update(keyRunes("/"))
	m = updated.(Model)
	m = typeRunes(t, m, "tui") // gitui matches by name, lazygit only by tag

	lines := strings.Split(stripANSI(m.renderLeftContent()), "\n")
	if !strings.Contains(lines[1], "lazygit") || !strings.Contains(lines[1], "#tui") {
		t.Errorf("tag-only row = %q, want lazygit with #tui suffix", lines[1])
	}
	if strings.Contains(lines[0], "#") {
		t.Errorf("name-match row = %q, want no tag suffix", lines[0])
	}

	// A name column too narrow for the suffix drops it instead of wrapping
	// the row.
	m.toolsW = 8 // maxName = 3
	lines = strings.Split(stripANSI(m.renderLeftContent()), "\n")
	for i, line := range lines {
		if strings.Contains(line, "#") {
			t.Errorf("narrow row %d = %q, want tag suffix dropped", i, line)
		}
	}
}

// TestRenderLeftContentSearchHighlight verifies the matched substring of a
// non-selected row's name is wrapped in the peach-bold highlight while
// searching.
func TestRenderLeftContentSearchHighlight(t *testing.T) {
	forceColorProfile(t)
	m := newSearchTestModel() // fzf, git, ripgrep
	updated, _ := m.Update(keyRunes("/"))
	m = updated.(Model)
	m = typeRunes(t, m, "i") // matches git (selected) and ripgrep

	lines := strings.Split(m.renderLeftContent(), "\n")
	if want := ui.SelectedNameStyle.Render("i"); !strings.Contains(lines[1], want) {
		t.Errorf("non-selected match row = %q, want highlighted substring %q", lines[1], want)
	}
	if !strings.Contains(stripANSI(lines[1]), "ripgrep") {
		t.Errorf("non-selected match row = %q, highlight corrupted the name", stripANSI(lines[1]))
	}
}

// TestHighlightNameMatch pins the helper: case-insensitive match, untouched
// non-match, and per-line behavior (a match split across a wrap boundary is
// left unhighlighted).
func TestHighlightNameMatch(t *testing.T) {
	forceColorProfile(t)
	styled := ui.SelectedNameStyle.Render("ip")
	if got := highlightNameMatch("ripgrep", "ip"); got != "r"+styled+"grep" {
		t.Errorf("highlightNameMatch(ripgrep, ip) = %q, want %q", got, "r"+styled+"grep")
	}
	if got := highlightNameMatch("RipGrep", "ipg"); got != "R"+ui.SelectedNameStyle.Render("ipG")+"rep" {
		t.Errorf("highlightNameMatch(RipGrep, ipg) = %q, case-insensitive match expected", got)
	}
	if got := highlightNameMatch("ripgrep", "zz"); got != "ripgrep" {
		t.Errorf("highlightNameMatch(ripgrep, zz) = %q, want untouched", got)
	}
	if got := highlightNameMatch("ab\ncd", "bc"); got != "ab\ncd" {
		t.Errorf("highlightNameMatch(ab\\ncd, bc) = %q, want untouched across the wrap boundary", got)
	}
	// Only the first occurrence per line is highlighted.
	if got := highlightNameMatch("gogo", "go"); got != ui.SelectedNameStyle.Render("go")+"go" {
		t.Errorf("highlightNameMatch(gogo, go) = %q, want only the first occurrence styled", got)
	}
	// Rune safety: Ⱥ (2 bytes) lowercases to ⱥ (3 bytes), so byte offsets
	// found in strings.ToLower(line) would slice the original out of range.
	if got := highlightNameMatch("Ⱥx", "x"); stripANSI(got) != "Ⱥx" || !utf8.ValidString(got) {
		t.Errorf("highlightNameMatch(Ⱥx, x) = %q, want the name intact and valid UTF-8", got)
	}
	if got := highlightNameMatch("Ⱥx", "ⱥ"); stripANSI(got) != "Ⱥx" || !strings.Contains(got, "Ⱥ") {
		t.Errorf("highlightNameMatch(Ⱥx, ⱥ) = %q, want case-insensitive match on the original rune", got)
	}
}

// TestRenderLeftContentMarkerSurvivesFocus verifies the ⏺ marker on the
// selected row does not disappear when focus moves to the brief/help panels
// (it renders dim there, but stays in the output).
func TestRenderLeftContentMarkerSurvivesFocus(t *testing.T) {
	m := New([]loader.ToolMeta{
		{Name: "fzf", Status: loader.StatusActive},
		{Name: "git", Status: loader.StatusActive},
	})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.metaSelected = 1

	for _, f := range []int{focusTools, focusBrief, focusHelp} {
		m.focus = f
		lines := strings.Split(stripANSI(m.renderLeftContent()), "\n")
		if !strings.HasPrefix(lines[1], "⏺ git") {
			t.Errorf("focus %v: selected row = %q, want ⏺ marker on git", f, lines[1])
		}
		if strings.Contains(lines[0], "⏺") {
			t.Errorf("focus %v: unselected row = %q, should carry no marker", f, lines[0])
		}
	}
}

// TestRenderLeftContentMarkerColumn verifies the marker column carries only the
// ⏺ cursor: the selected row gets it regardless of status, every other row
// (active, trying, inactive, unknown) gets a plain space — the status edge is
// gone (tool status lives in the brief card only).
func TestRenderLeftContentMarkerColumn(t *testing.T) {
	m := New([]loader.ToolMeta{
		{Name: "fzf", Status: loader.StatusActive},
		{Name: "git", Status: loader.StatusTrying},
		{Name: "ripgrep", Status: loader.StatusInactive},
		{Name: "yq", Status: loader.Status("bogus")},
	})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.focus = focusTools
	m.metaSelected = 0

	lines := strings.Split(stripANSI(m.renderLeftContent()), "\n")
	if !strings.HasPrefix(lines[0], "⏺ fzf") {
		t.Errorf("selected active row = %q, want ⏺ marker", lines[0])
	}
	for i, name := range []string{"git", "ripgrep", "yq"} {
		if !strings.HasPrefix(lines[i+1], "  "+name) {
			t.Errorf("non-selected row = %q, want plain space in the marker column", lines[i+1])
		}
		if strings.Contains(lines[i+1], "⏺") {
			t.Errorf("non-selected row = %q, should carry no cursor", lines[i+1])
		}
	}

	// The ⏺ cursor takes priority on the selected row regardless of status, and
	// the row it left behind falls back to a plain-space marker column.
	m.metaSelected = 1
	lines = strings.Split(stripANSI(m.renderLeftContent()), "\n")
	if !strings.HasPrefix(lines[1], "⏺ git") {
		t.Errorf("selected trying row = %q, want ⏺ cursor", lines[1])
	}
	if !strings.HasPrefix(lines[0], "  fzf") {
		t.Errorf("active row = %q, want plain space in the marker column", lines[0])
	}
}

// TestRenderLeftContentRowWidth verifies the marker column glyphs are all
// single-cell, so every row keeps the same visible width prefix (1 marker
// cell + 1 space) regardless of selection or status.
func TestRenderLeftContentRowWidth(t *testing.T) {
	m := New([]loader.ToolMeta{
		{Name: "aa", Status: loader.StatusActive},
		{Name: "bb", Status: loader.StatusTrying},
		{Name: "cc", Status: loader.StatusInactive},
		{Name: "dd", Status: loader.Status("bogus")},
	})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.focus = focusTools
	m.metaSelected = 0

	for i, line := range strings.Split(strings.TrimRight(stripANSI(m.renderLeftContent()), "\n"), "\n") {
		if w := lipgloss.Width(line); w != 4 { // marker + space + 2-rune name
			t.Errorf("row %d = %q, visible width = %d, want 4", i, line, w)
		}
	}
}

// updatableModel builds a model with tools where the named ones have an
// available update (Installed older than Latest), so the grouping partition
// lifts them to the top of the list. Sizing goes through a real WindowSizeMsg.
func updatableModel(t *testing.T, metas []loader.ToolMeta, updatable ...string) Model {
	t.Helper()
	m := New(metas)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.focus = focusTools
	for _, name := range updatable {
		m.versions[name] = VersionInfo{Installed: "1.0", Latest: "2.0", InstalledKnown: true}
	}
	return m
}

// filteredNames projects the displayed (grouped/filtered) order to a name slice.
func filteredNames(m Model) []string {
	var out []string
	for _, mt := range m.filteredMeta() {
		out = append(out, mt.Name)
	}
	return out
}

// TestToolsListGrouping verifies updatable tools are stable-partitioned to the
// top of the displayed list (meta.yaml order preserved inside each group) while
// the underlying m.meta slice is left untouched.
func TestToolsListGrouping(t *testing.T) {
	metas := []loader.ToolMeta{
		{Name: "aa"}, {Name: "bb"}, {Name: "cc"}, {Name: "dd"},
	}
	// bb and dd have updates → they float up, keeping their relative order.
	m := updatableModel(t, metas, "bb", "dd")

	if got, want := filteredNames(m), []string{"bb", "dd", "aa", "cc"}; !slices.Equal(got, want) {
		t.Errorf("displayed order = %v, want %v", got, want)
	}
	// m.meta on disk order is never reordered by the display projection.
	if got, want := (func() []string {
		var s []string
		for _, mt := range m.meta {
			s = append(s, mt.Name)
		}
		return s
	})(), []string{"aa", "bb", "cc", "dd"}; !slices.Equal(got, want) {
		t.Errorf("m.meta order = %v, want %v (untouched)", got, want)
	}
	// The rendered rows follow the same order.
	rows := strings.Split(strings.TrimRight(stripANSI(m.renderLeftContent()), "\n"), "\n")
	for i, want := range []string{"bb", "dd", "aa", "cc"} {
		if !strings.Contains(rows[i], want) {
			t.Errorf("row %d = %q, want %s", i, rows[i], want)
		}
	}
}

// TestToolsListGroupingWithinSearch verifies the grouping partition applies
// inside an active search filter too: matched updatable tools sort above matched
// non-updatable ones.
func TestToolsListGroupingWithinSearch(t *testing.T) {
	metas := []loader.ToolMeta{
		{Name: "git"}, {Name: "gitui"}, {Name: "lazygit"},
	}
	m := updatableModel(t, metas, "lazygit") // lazygit has an update
	updated, _ := m.Update(keyRunes("/"))
	m = updated.(Model)
	m = typeRunes(t, m, "git") // all three match by name

	if got, want := filteredNames(m), []string{"lazygit", "git", "gitui"}; !slices.Equal(got, want) {
		t.Errorf("filtered+grouped order = %v, want %v", got, want)
	}
}

// TestRemoteMsgKeepsSelectionOnReorder verifies a remoteMsg that flips another
// tool into the updatable group (lifting it above the selected one) keeps the
// cursor on the *selected tool*, not its old row index.
func TestRemoteMsgKeepsSelectionOnReorder(t *testing.T) {
	m := updatableModel(t, []loader.ToolMeta{{Name: "aa"}, {Name: "bb"}, {Name: "cc"}})
	m.versions["cc"] = VersionInfo{Installed: "1.0", InstalledKnown: true} // no Latest yet
	m.metaSelected = 1                                                     // bb (displayed idx 1)

	// cc gets a newer release → hasUpdate(cc) true → cc floats to the top,
	// pushing bb to displayed idx 2.
	updated, _ := m.Update(remoteMsg{toolName: "cc", latest: "2.0", card: version.RepoCard{About: "x"}})
	nm := updated.(Model)

	if got, want := filteredNames(nm), []string{"cc", "aa", "bb"}; !slices.Equal(got, want) {
		t.Fatalf("displayed order = %v, want %v", got, want)
	}
	if sel, ok := nm.selectedMeta(); !ok || sel.Name != "bb" || nm.metaSelected != 2 {
		t.Errorf("selection = %v (idx %d), want bb at idx 2 (followed the tool)", sel, nm.metaSelected)
	}
}

// TestRemoteMsgSelectedToolLiftedToTop verifies that when the *selected* tool is
// the one gaining an update, it stays selected at its new top-of-list index.
func TestRemoteMsgSelectedToolLiftedToTop(t *testing.T) {
	m := updatableModel(t, []loader.ToolMeta{{Name: "aa"}, {Name: "bb"}, {Name: "cc"}})
	m.versions["cc"] = VersionInfo{Installed: "1.0", InstalledKnown: true}
	m.metaSelected = 2 // cc

	updated, _ := m.Update(remoteMsg{toolName: "cc", latest: "2.0", card: version.RepoCard{About: "x"}})
	nm := updated.(Model)

	if sel, ok := nm.selectedMeta(); !ok || sel.Name != "cc" || nm.metaSelected != 0 {
		t.Errorf("selection = %v (idx %d), want cc at idx 0 (lifted, still selected)", sel, nm.metaSelected)
	}
}

// TestInstalledMsgKeepsSelectionOnReorder verifies the installedMsg handler
// remaps the cursor by name too: a fresh installed version that makes another
// tool updatable reorders the list without dragging the selection off its tool.
func TestInstalledMsgKeepsSelectionOnReorder(t *testing.T) {
	m := updatableModel(t, []loader.ToolMeta{{Name: "aa"}, {Name: "bb"}, {Name: "cc"}})
	m.versions["cc"] = VersionInfo{Latest: "2.0"} // Latest known, Installed pending
	m.metaSelected = 1                            // bb

	// cc's installed version arrives, older than Latest → cc becomes updatable
	// and floats to the top, pushing bb to idx 2.
	updated, _ := m.Update(installedMsg{toolName: "cc", installed: "1.0"})
	nm := updated.(Model)

	if sel, ok := nm.selectedMeta(); !ok || sel.Name != "bb" || nm.metaSelected != 2 {
		t.Errorf("selection = %v (idx %d), want bb at idx 2", sel, nm.metaSelected)
	}
}

// TestVersionMsgsEmptyListNoPanic verifies installedMsg/remoteMsg on an empty
// tool list are safe: the remap is skipped when there is no selection.
func TestVersionMsgsEmptyListNoPanic(t *testing.T) {
	m := New(nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, _ = m.Update(installedMsg{toolName: "ghost", installed: "1.0"})
	m = updated.(Model)
	updated, _ = m.Update(remoteMsg{toolName: "ghost", latest: "2.0", card: version.RepoCard{About: "x"}})
	m = updated.(Model)
	if m.metaSelected != 0 {
		t.Errorf("metaSelected = %d, want 0 on empty list", m.metaSelected)
	}
}

// TestMarkerGlyphWidth pins the marker/suffix glyphs at width 1 in
// go-runewidth's default condition (the one lipgloss measures with), keeping the
// list's row-width math stable. Note: both ⏺ (U+23FA, the selection cursor) and
// ↑ (U+2191, the update-available suffix) are East-Asian **Ambiguous** — they
// measure 2 cells under RUNEWIDTH_EASTASIAN=1. This is consciously accepted:
// the removed ▎ status edge was in the same class, so the change is not a
// regression. A bare lipgloss.Width==1 check cannot detect the ambiguity, so the
// test measures both conditions explicitly.
func TestMarkerGlyphWidth(t *testing.T) {
	for _, r := range []rune{'⏺', '↑'} {
		def := runewidth.Condition{EastAsianWidth: false}
		if w := def.RuneWidth(r); w != 1 {
			t.Errorf("RuneWidth(%q) default = %d, want 1", r, w)
		}
		// Document (not enforce) the Ambiguous classification: width 2 under the
		// East-Asian condition. If a future Unicode table narrowed this to 1 the
		// comment above would be stale — surface it as a heads-up, not a failure.
		ea := runewidth.Condition{EastAsianWidth: true}
		if w := ea.RuneWidth(r); w != 2 {
			t.Logf("RuneWidth(%q) east-asian = %d, expected 2 (Ambiguous) — table may have changed", r, w)
		}
	}
}

// TestRenderPanelTitles verifies every panel's top border carries its title
// with the focus hotkey, that the help title follows m.helpMode, and that the
// splice leaves the border's visible width alone.
func TestRenderPanelTitles(t *testing.T) {
	m := New([]loader.ToolMeta{{Name: "fzf", Status: loader.StatusActive}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m = updated.(Model)

	m.helpMode = helpModeHelp
	top := stripANSI(strings.SplitN(m.renderHelp(), "\n", 2)[0])
	if !strings.Contains(top, " [3] Help ") {
		t.Errorf("help-mode top border = %q, want [3] Help title", top)
	}

	m.helpMode = helpModeMan
	lines := strings.Split(m.renderHelp(), "\n")
	topMan := stripANSI(lines[0])
	if !strings.Contains(topMan, " [3] Man ") || strings.Contains(topMan, "Help") {
		t.Errorf("man-mode top border = %q, want [3] Man title", topMan)
	}
	if bottom := stripANSI(lines[len(lines)-1]); lipgloss.Width(topMan) != lipgloss.Width(bottom) {
		t.Errorf("top border width = %d, want %d (unchanged)", lipgloss.Width(topMan), lipgloss.Width(bottom))
	}

	for _, tt := range []struct{ name, panel, want string }{
		{"tools", m.renderTools(), " [1] Tools "},
		{"brief", m.renderBrief(), " [2] Brief "},
	} {
		panelLines := strings.Split(tt.panel, "\n")
		got := stripANSI(panelLines[0])
		if !strings.Contains(got, tt.want) {
			t.Errorf("%s top border = %q, want %q title", tt.name, got, tt.want)
		}
		if bottom := stripANSI(panelLines[len(panelLines)-1]); lipgloss.Width(got) != lipgloss.Width(bottom) {
			t.Errorf("%s top border width = %d, want %d (unchanged)", tt.name, lipgloss.Width(got), lipgloss.Width(bottom))
		}
	}
}

// TestPanelTitleFollowsFocus verifies each panel's title is painted with the
// focus-aware border color: same visible text, different styling, so the
// focused branch of every panel renderer is exercised.
func TestPanelTitleFollowsFocus(t *testing.T) {
	forceColor(t)

	tests := []struct {
		name   string
		focus  int
		render func(Model) string
	}{
		{"tools", focusTools, Model.renderTools},
		{"brief", focusBrief, Model.renderBrief},
		{"help", focusHelp, Model.renderHelp},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New([]loader.ToolMeta{{Name: "fzf", Status: loader.StatusActive}})
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
			m = updated.(Model)

			m.focus = focusTools
			if tt.focus == focusTools {
				m.focus = focusHelp // ensure the panel under test starts unfocused
			}
			blurred, _, _ := strings.Cut(tt.render(m), "\n")

			m.focus = tt.focus
			focused, _, _ := strings.Cut(tt.render(m), "\n")

			if focused == blurred {
				t.Errorf("%s top border identical focused and unfocused (%q), want a focus-aware color", tt.name, blurred)
			}
			if stripANSI(focused) != stripANSI(blurred) {
				t.Errorf("%s visible top border changed with focus:\n focused = %q\n blurred = %q",
					tt.name, stripANSI(focused), stripANSI(blurred))
			}
		})
	}
}

// TestInsetPanelTitle exercises the splice helper directly: a too-narrow
// panel is returned unchanged (no panic), a title that does not fit whole is
// dropped rather than truncated, and a fitting title is inset without
// changing the top line's visible width.
func TestInsetPanelTitle(t *testing.T) {
	narrow := "╭─╮\n│ │\n╰─╯"
	if got := insetPanelTitle(narrow, "--help", false); got != narrow {
		t.Errorf("narrow panel = %q, want unchanged", got)
	}
	if got := insetPanelTitle("", "--help", false); got != "" {
		t.Errorf("empty panel = %q, want unchanged", got)
	}

	// " --help " needs 8 cells; this top offers 3 — dropped whole, not chopped.
	partial := "╭────╮\n│    │\n╰────╯"
	if got := insetPanelTitle(partial, "--help", true); got != partial {
		t.Errorf("partial-fit panel = %q, want unchanged (title dropped whole)", got)
	}

	fits := "╭──────────╮\n│          │\n╰──────────╯"
	top := stripANSI(strings.SplitN(insetPanelTitle(fits, "--help", true), "\n", 2)[0])
	if len([]rune(top)) != 12 {
		t.Errorf("titled top = %q, visible width = %d, want 12", top, len([]rune(top)))
	}
	if !strings.HasPrefix(top, "╭─ --help ") || !strings.HasSuffix(top, "╮") {
		t.Errorf("titled top = %q, want inset title with corners intact", top)
	}
}

func TestRenderStatusBarGauge(t *testing.T) {
	known := version.RateLimit{Known: true, Remaining: 15, Limit: 60} // used 45/60

	t.Run("unknown renders no gauge", func(t *testing.T) {
		m := Model{width: 120, focus: focusTools}
		m.rate = version.RateLimit{Known: false, Remaining: 0, Limit: 60}
		got := m.renderStatusBar()
		for _, absent := range []string{"GitHub API Usage", "GH ", "45/60"} {
			if strings.Contains(got, absent) {
				t.Errorf("unknown rate status bar = %q, should not contain %q", got, absent)
			}
		}
	})

	t.Run("wide width shows full gauge (pinned to focusTools)", func(t *testing.T) {
		m := Model{width: 120, focus: focusTools, rate: known}
		got := m.renderStatusBar()
		for _, want := range []string{"GitHub API Usage", "45/60", "[L]", "search"} {
			if !strings.Contains(got, want) {
				t.Errorf("wide status bar = %q, missing %q", got, want)
			}
		}
	})

	t.Run("medium width collapses to compact", func(t *testing.T) {
		m := Model{width: 90, focus: focusTools, rate: known}
		got := m.renderStatusBar()
		if strings.Contains(got, "GitHub API Usage") {
			t.Errorf("medium status bar unexpectedly full: %q", got)
		}
		for _, want := range []string{"GH ", "45/60", "[L]"} {
			if !strings.Contains(got, want) {
				t.Errorf("medium status bar = %q, missing %q", got, want)
			}
		}
	})

	t.Run("narrow width hides gauge but keeps hints", func(t *testing.T) {
		m := Model{width: 62, focus: focusTools, rate: known}
		got := m.renderStatusBar()
		for _, absent := range []string{"GitHub API Usage", "GH ", "45/60"} {
			if strings.Contains(got, absent) {
				t.Errorf("narrow status bar = %q, should not contain %q", got, absent)
			}
		}
		for _, want := range []string{"search", "track", "quit"} {
			if !strings.Contains(got, want) {
				t.Errorf("narrow status bar = %q, missing hint %q", got, want)
			}
		}
	})

	t.Run("focusBrief also right-aligns the gauge", func(t *testing.T) {
		// focusBrief hints are longer than focusTools, so a wider terminal is
		// needed for the full form — exercises the per-focus downgrade threshold.
		m := Model{width: 160, focus: focusBrief, rate: known}
		got := m.renderStatusBar()
		for _, want := range []string{"GitHub API Usage", "45/60", "[L]", "open repo"} {
			if !strings.Contains(got, want) {
				t.Errorf("focusBrief status bar = %q, missing %q", got, want)
			}
		}
	})

	t.Run("input and modal modes suppress the gauge", func(t *testing.T) {
		for _, m := range []Model{
			{width: 120, mode: modeTrack, trackInput: textinput.New(), rate: known},
			{width: 120, mode: modeRename, nameInput: textinput.New(), rate: known},
			{width: 120, mode: modeSearch, search: textinput.New(), rate: known},
			{width: 120, mode: modeEditNote, rate: known},
			{width: 120, mode: modeEditTags, rate: known},
			{width: 120, mode: modeAPIStatus, rate: known},
		} {
			got := m.renderStatusBar()
			if strings.Contains(got, "GitHub API Usage") || strings.Contains(got, "GH ") {
				t.Errorf("input/modal status bar leaked gauge: %q", got)
			}
		}
	})
}

func TestRenderHintsBarAlignment(t *testing.T) {
	known := version.RateLimit{Known: true, Remaining: 15, Limit: 60} // used 45/60
	m := Model{width: 120, rate: known}
	hints := "abc"

	// A plain (border-less) style isolates the laid-out content; the gap logic
	// inside renderHintsBar uses m.width-2 regardless of the style passed.
	out := m.renderHintsBar(lipgloss.NewStyle(), hints)
	plain := ansiCSI.ReplaceAllString(out, "")
	inner := m.width - 2

	// Fills exactly to the right edge — the gauge is pinned to the corner.
	if w := lipgloss.Width(out); w != inner {
		t.Errorf("laid-out width = %d, want inner %d (gauge not right-aligned)", w, inner)
	}
	// Gauge sits at the far right (full form ends with " details").
	if !strings.HasSuffix(plain, "details") {
		t.Errorf("hints bar = %q, gauge not at the right end", plain)
	}
	// Hints stay on the left with a spacer before the gauge.
	if !strings.HasPrefix(plain, "abc  ") {
		t.Errorf("hints bar = %q, want hints then a spacer on the left", plain)
	}
}

func TestRenderStatusBarTracking(t *testing.T) {
	m := Model{width: 80, mode: modeTrack, trackInput: textinput.New()}
	got := m.renderStatusBar()
	if !strings.Contains(got, "track") {
		t.Errorf("tracking status bar = %q, missing prompt", got)
	}
}

func TestTrackTool(t *testing.T) {
	tests := []struct {
		name       string
		meta       []loader.ToolMeta
		input      string
		wantName   string
		wantGitHub string
		wantLen    int
		wantStatus string
	}{
		{
			name:       "github url derives name and github",
			input:      "https://github.com/anthropics/claude-code",
			wantName:   "claude-code",
			wantGitHub: "github.com/anthropics/claude-code",
			wantLen:    1,
		},
		{
			name:     "plain name has no github",
			input:    "git",
			wantName: "git",
			wantLen:  1,
		},
		{
			name:    "empty input is a no-op",
			input:   "   ",
			wantLen: 0,
		},
		{
			name:       "duplicate updates not duplicates",
			meta:       []loader.ToolMeta{{Name: "git", Status: loader.StatusActive}},
			input:      "git",
			wantName:   "git",
			wantLen:    1,
			wantStatus: "already tracked",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, status := trackTool(tt.meta, tt.input)
			if len(got) != tt.wantLen {
				t.Fatalf("len = %d, want %d", len(got), tt.wantLen)
			}
			if status != tt.wantStatus {
				t.Errorf("status = %q, want %q", status, tt.wantStatus)
			}
			if tt.wantName == "" {
				return
			}
			e := loader.FindMeta(got, tt.wantName)
			if e == nil {
				t.Fatalf("expected entry %q in result", tt.wantName)
			}
			if e.GitHub != tt.wantGitHub {
				t.Errorf("github = %q, want %q", e.GitHub, tt.wantGitHub)
			}
			if e.Status != loader.StatusTrying {
				t.Errorf("status field = %q, want %q", e.Status, loader.StatusTrying)
			}
			if e.Added == "" {
				t.Errorf("Added should be set")
			}
		})
	}
}

func TestTrackToolSavePath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	meta, _ := trackTool(nil, "git")
	if err := loader.SaveMeta(meta); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	loaded, err := loader.LoadMeta()
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if loader.FindMeta(loaded, "git") == nil {
		t.Errorf("expected git in saved meta")
	}
}

func TestRenderStatusBarConfirmUntrack(t *testing.T) {
	m := Model{width: 80, mode: modeConfirmUntrack, untrackTarget: "git"}
	got := m.renderStatusBar()
	for _, want := range []string{"Untrack", "git", "yes", "no"} {
		if !strings.Contains(got, want) {
			t.Errorf("confirm untrack status bar = %q, missing %q", got, want)
		}
	}
}

func TestRenderStatusBarFocusToolsUntrackHint(t *testing.T) {
	m := Model{width: 80, focus: focusTools}
	if !strings.Contains(m.renderStatusBar(), "untrack") {
		t.Errorf("focusTools status bar missing untrack hint: %q", m.renderStatusBar())
	}
}

func TestUpdateUntrackConfirm(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	enter := tea.KeyMsg{Type: tea.KeyEnter}
	esc := tea.KeyMsg{Type: tea.KeyEsc}

	t.Run("enter removes and clamps selection to next item", func(t *testing.T) {
		m := Model{
			meta: []loader.ToolMeta{
				{Name: "a"}, {Name: "b"}, {Name: "c"},
			},
			metaSelected:  1,
			mode:          modeConfirmUntrack,
			untrackTarget: "b",
		}
		m.tools = loader.ToolsFromMeta(m.meta)

		updated, _ := m.updateUntrackConfirm(enter)
		nm := updated.(Model)

		if nm.mode == modeConfirmUntrack {
			t.Errorf("confirmingUntrack should be false after enter")
		}
		if loader.FindMeta(nm.meta, "b") != nil {
			t.Errorf("b should be removed")
		}
		if len(nm.meta) != 2 {
			t.Fatalf("len = %d, want 2", len(nm.meta))
		}
		// selection stays at index 1, now pointing at "c".
		if nm.metaSelected != 1 {
			t.Errorf("metaSelected = %d, want 1", nm.metaSelected)
		}
	})

	t.Run("enter on last item clamps to new last index", func(t *testing.T) {
		m := Model{
			meta:          []loader.ToolMeta{{Name: "a"}, {Name: "b"}},
			metaSelected:  1,
			mode:          modeConfirmUntrack,
			untrackTarget: "b",
		}
		m.tools = loader.ToolsFromMeta(m.meta)

		updated, _ := m.updateUntrackConfirm(enter)
		nm := updated.(Model)

		if nm.metaSelected != 0 {
			t.Errorf("metaSelected = %d, want 0", nm.metaSelected)
		}
	})

	t.Run("esc cancels and leaves list unchanged", func(t *testing.T) {
		m := Model{
			meta:          []loader.ToolMeta{{Name: "a"}, {Name: "b"}},
			metaSelected:  0,
			mode:          modeConfirmUntrack,
			untrackTarget: "a",
		}
		m.tools = loader.ToolsFromMeta(m.meta)

		updated, _ := m.updateUntrackConfirm(esc)
		nm := updated.(Model)

		if nm.mode == modeConfirmUntrack {
			t.Errorf("confirmingUntrack should be false after esc")
		}
		if len(nm.meta) != 2 || loader.FindMeta(nm.meta, "a") == nil {
			t.Errorf("list should be unchanged after esc, got %v", nm.meta)
		}
	})
}

func TestGaugeFilled(t *testing.T) {
	// Fixed width independent of the limit: 25% used fills the same at 60 and 5000.
	if a, b := gaugeFilled(15, 60), gaugeFilled(1250, 5000); a != b {
		t.Errorf("25%% fill differs by limit: 60→%d, 5000→%d", a, b)
	}
	tests := []struct {
		used, limit, want int
	}{
		{0, 60, 0},
		{60, 60, gaugeCells}, // exhausted → full bar
		{30, 60, gaugeCells / 2},
		{-5, 60, 0},                  // never negative
		{99, 60, gaugeCells},         // over-limit stays full
		{5, 0, 0},                    // no divide-by-zero
		{1, 60, 1},                   // any usage shows at least one cell…
		{1, 5000, 1},                 // …even when the ratio rounds to zero
		{59, 60, gaugeCells - 1},     // full bar means exhaustion only…
		{4999, 5000, gaugeCells - 1}, // …however close the ratio rounds to it
	}
	for _, tt := range tests {
		if got := gaugeFilled(tt.used, tt.limit); got != tt.want {
			t.Errorf("gaugeFilled(%d,%d) = %d, want %d", tt.used, tt.limit, got, tt.want)
		}
	}
}

func TestRenderRateGauge(t *testing.T) {
	t.Run("full form shows label, used/limit and [L]", func(t *testing.T) {
		m := Model{rate: version.RateLimit{Known: true, Remaining: 15, Limit: 60}}
		got := m.renderRateGauge(false)
		for _, want := range []string{"GitHub API Usage", "45/60", "[L]"} {
			if !strings.Contains(got, want) {
				t.Errorf("full gauge = %q, missing %q", got, want)
			}
		}
	})

	t.Run("compact form is shorter and shows GH used/limit [L]", func(t *testing.T) {
		m := Model{rate: version.RateLimit{Known: true, Remaining: 15, Limit: 60}}
		full := m.renderRateGauge(false)
		compact := m.renderRateGauge(true)
		if !strings.Contains(compact, "GH ") || !strings.Contains(compact, "45/60") || !strings.Contains(compact, "[L]") {
			t.Errorf("compact gauge = %q, missing parts", compact)
		}
		if lipgloss.Width(compact) >= lipgloss.Width(full) {
			t.Errorf("compact (%d) not shorter than full (%d)", lipgloss.Width(compact), lipgloss.Width(full))
		}
	})

	t.Run("exhausted shows 60/60 with a full bar", func(t *testing.T) {
		// Constant-yellow-at-exhaustion is structural (renderRateGauge has no
		// pressure branch); gaugeFilled(60,60)==gaugeCells is covered separately.
		m := Model{rate: version.RateLimit{Known: true, Remaining: 0, Limit: 60}}
		got := m.renderRateGauge(false)
		if !strings.Contains(got, "60/60") {
			t.Errorf("exhausted gauge = %q, want 60/60", got)
		}
		// Strip ANSI before checking the bar is a full gaugeCells-wide fill
		// block between the brackets.
		plain := ansiCSI.ReplaceAllString(got, "")
		if !strings.Contains(plain, "["+strings.Repeat(gaugeFillGlyph, gaugeCells)+"]") {
			t.Errorf("exhausted gauge = %q, want a full %d-cell bar", plain, gaugeCells)
		}
	})

	t.Run("partial fill draws fill and track glyphs", func(t *testing.T) {
		// 30/60 → exactly half the bar; glyphs survive ANSI stripping, so the
		// bar stays visible on degraded color profiles.
		m := Model{rate: version.RateLimit{Known: true, Remaining: 30, Limit: 60}}
		plain := ansiCSI.ReplaceAllString(m.renderRateGauge(false), "")
		want := "[" + strings.Repeat(gaugeFillGlyph, gaugeCells/2) + strings.Repeat(gaugeTrackGlyph, gaugeCells/2) + "]"
		if !strings.Contains(plain, want) {
			t.Errorf("half-used gauge = %q, want bar %q", plain, want)
		}
	})

	t.Run("unknown snapshot renders nothing", func(t *testing.T) {
		m := Model{rate: version.RateLimit{Known: false, Remaining: 0, Limit: 60}}
		if got := m.renderRateGauge(false); got != "" {
			t.Errorf("unknown gauge = %q, want empty", got)
		}
	})
}

// TestRenderRateGaugeColors pins the gauge's fill/track color distinction. It
// asserts the isolated styles, not the full gauge string: the brackets and the
// used/limit number also emit foreground ColorOrange (RateBracketStyle /
// RateUsageNumStyle), so a fill regression — colorless, merged into the track,
// or back to a painted background — would be masked by them.
func TestRenderRateGaugeColors(t *testing.T) {
	forceColorProfile(t)

	// Expected sequences come from termenv (its hex→RGB conversion rounds, so
	// literal palette bytes would be brittle): "38;2;r;g;b" foreground form.
	fgSeq := func(c lipgloss.Color) string {
		return termenv.TrueColor.Color(string(c)).Sequence(false)
	}
	fillSeq, trackSeq := fgSeq(ui.ColorOrange), fgSeq(ui.ColorOrangeDim)

	fill := ui.RateGaugeFillStyle.Render(gaugeFillGlyph)
	if !strings.Contains(fill, fillSeq) {
		t.Errorf("fill = %q, missing foreground sequence %q", fill, fillSeq)
	}
	if strings.Contains(fill, "48;2;") {
		t.Errorf("fill = %q, must color the glyph, not paint a background", fill)
	}

	track := ui.RateGaugeTrackStyle.Render(gaugeTrackGlyph)
	if !strings.Contains(track, trackSeq) {
		t.Errorf("track = %q, missing foreground sequence %q", track, trackSeq)
	}
	if strings.Contains(track, "48;2;") {
		t.Errorf("track = %q, must color the glyph, not paint a background", track)
	}

	if fillSeq == trackSeq {
		t.Error("fill and track resolve to the same color — the empty track would be indistinguishable")
	}
}

// TestGaugeGlyphWidthsStable pins that the bar glyphs are not
// East-Asian-Ambiguous: lipgloss.Width must report one cell per glyph even
// under RUNEWIDTH_EASTASIAN=1, or renderHintsBar's gap math would inflate and
// wrongly downgrade or mis-pad the gauge (a full block █ measures as two cells
// there). The width tables read the env var once at package init, so the
// ambiguous-width variant re-runs this test in a child process.
func TestGaugeGlyphWidthsStable(t *testing.T) {
	for _, g := range []string{gaugeFillGlyph, gaugeTrackGlyph} {
		if w := lipgloss.Width(g); w != 1 {
			t.Errorf("glyph %q width = %d, want 1", g, w)
		}
	}
	if os.Getenv("KEYS_WIDTH_CHECK_CHILD") == "1" {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	cmd := exec.Command(exe, "-test.run", "^TestGaugeGlyphWidthsStable$")
	cmd.Env = append(os.Environ(), "KEYS_WIDTH_CHECK_CHILD=1", "RUNEWIDTH_EASTASIAN=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("glyph widths change under RUNEWIDTH_EASTASIAN=1:\n%s", out)
	}
}

func TestListNavigationWraps(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	keyJ := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}
	keyK := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")}

	newModel := func(meta []loader.ToolMeta, sel int) Model {
		m := Model{width: 80, height: 24, focus: focusTools, ready: true, meta: meta, metaSelected: sel}
		m.tools = loader.ToolsFromMeta(m.meta)
		return m
	}

	t.Run("down from last wraps to first", func(t *testing.T) {
		m := newModel([]loader.ToolMeta{{Name: "a"}, {Name: "b"}, {Name: "c"}}, 2)
		nm := mustModel(m.Update(keyJ))
		if nm.metaSelected != 0 {
			t.Errorf("metaSelected = %d, want 0 (wrap to first)", nm.metaSelected)
		}
	})

	t.Run("up from first wraps to last", func(t *testing.T) {
		m := newModel([]loader.ToolMeta{{Name: "a"}, {Name: "b"}, {Name: "c"}}, 0)
		nm := mustModel(m.Update(keyK))
		if nm.metaSelected != 2 {
			t.Errorf("metaSelected = %d, want 2 (wrap to last)", nm.metaSelected)
		}
	})

	t.Run("single item stays put both directions", func(t *testing.T) {
		m := newModel([]loader.ToolMeta{{Name: "a"}}, 0)
		if nm := mustModel(m.Update(keyJ)); nm.metaSelected != 0 {
			t.Errorf("down: metaSelected = %d, want 0", nm.metaSelected)
		}
		if nm := mustModel(m.Update(keyK)); nm.metaSelected != 0 {
			t.Errorf("up: metaSelected = %d, want 0", nm.metaSelected)
		}
	})

	t.Run("empty list does not panic", func(t *testing.T) {
		m := newModel(nil, 0)
		_ = mustModel(m.Update(keyJ))
		_ = mustModel(m.Update(keyK))
	})
}

func mustModel(tm tea.Model, _ tea.Cmd) Model {
	return tm.(Model)
}

// TestHelpMissingSourceMessages verifies that a mode whose source is absent
// (no man page, or no --help output) surfaces an explicit, tool-named message
// with a cross-hint to the other mode — instead of silently showing nothing or
// the other mode's content.
func TestHelpMissingSourceMessages(t *testing.T) {
	base := func(mode int) Model {
		m := Model{
			width: 120, height: 24, helpW: 60,
			meta:         []loader.ToolMeta{{Name: "agterm"}},
			metaSelected: 0,
			focus:        focusHelp,
			helpMode:     mode,
			helpCache:    map[string][2]string{},
		}
		m.tools = loader.ToolsFromMeta(m.meta)
		return m
	}

	t.Run("man mode with no page names the tool and points to [h]", func(t *testing.T) {
		m := base(helpModeMan)
		nm := mustModel(m.Update(helpOutputMsg{toolName: "agterm", mode: helpModeMan, err: errBoom}))
		plain := ansiCSI.ReplaceAllString(nm.renderHelpContent(), "")
		if !strings.Contains(plain, "No man page for agterm") {
			t.Errorf("man message = %q, want explicit no-man-page", plain)
		}
		if !strings.Contains(plain, "[h]") {
			t.Errorf("man message = %q, want cross-hint to --help", plain)
		}
	})

	t.Run("help mode with no output names the tool and points to [m]", func(t *testing.T) {
		m := base(helpModeHelp)
		nm := mustModel(m.Update(helpOutputMsg{toolName: "agterm", mode: helpModeHelp, err: errBoom}))
		plain := ansiCSI.ReplaceAllString(nm.renderHelpContent(), "")
		if !strings.Contains(plain, "No --help output for agterm") {
			t.Errorf("help message = %q, want explicit no-help", plain)
		}
		if !strings.Contains(plain, "[m]") {
			t.Errorf("help message = %q, want cross-hint to man", plain)
		}
	})
}

func TestRenderStatusBarRenaming(t *testing.T) {
	m := Model{width: 80, mode: modeRename, nameInput: textinput.New()}
	got := m.renderStatusBar()
	if !strings.Contains(got, "rename to") {
		t.Errorf("renaming status bar = %q, missing prompt", got)
	}
}

func TestRenderStatusBarFocusToolsRenameHint(t *testing.T) {
	m := Model{width: 80, focus: focusTools}
	if !strings.Contains(m.renderStatusBar(), "rename") {
		t.Errorf("focusTools status bar missing rename hint: %q", m.renderStatusBar())
	}
}

func TestRenameTool(t *testing.T) {
	t.Run("changes name and preserves other fields", func(t *testing.T) {
		meta := []loader.ToolMeta{
			{Name: "claude-code", GitHub: "github.com/anthropics/claude-code", Status: loader.StatusActive, Tags: []string{"ai"}, Note: "n", Added: "2026-01-01"},
		}
		got, err := renameTool(meta, "claude-code", "claude")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		e := loader.FindMeta(got, "claude")
		if e == nil {
			t.Fatalf("expected entry 'claude'")
		}
		if e.GitHub != "github.com/anthropics/claude-code" {
			t.Errorf("github = %q, want preserved", e.GitHub)
		}
		if e.Status != loader.StatusActive {
			t.Errorf("status = %q, want preserved", e.Status)
		}
		if len(e.Tags) != 1 || e.Tags[0] != "ai" || e.Note != "n" || e.Added != "2026-01-01" {
			t.Errorf("fields not preserved: %+v", e)
		}
		if loader.FindMeta(got, "claude-code") != nil {
			t.Errorf("old name should be gone")
		}
	})

	t.Run("empty is a no-op", func(t *testing.T) {
		meta := []loader.ToolMeta{{Name: "git"}}
		got, err := renameTool(meta, "git", "   ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if loader.FindMeta(got, "git") == nil {
			t.Errorf("git should be unchanged")
		}
	})

	t.Run("collision is rejected and leaves entry unchanged", func(t *testing.T) {
		meta := []loader.ToolMeta{{Name: "a", GitHub: "x"}, {Name: "b"}}
		got, err := renameTool(meta, "a", "b")
		if err == nil {
			t.Fatalf("expected collision error")
		}
		e := loader.FindMeta(got, "a")
		if e == nil || e.GitHub != "x" {
			t.Errorf("entry 'a' should be unchanged, got %+v", e)
		}
	})
}

func TestRenameToolSavePath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	meta := []loader.ToolMeta{{Name: "git", Status: loader.StatusActive}}
	got, err := renameTool(meta, "git", "g")
	if err != nil {
		t.Fatalf("renameTool: %v", err)
	}
	if err := loader.SaveMeta(got); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	loaded, err := loader.LoadMeta()
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if loader.FindMeta(loaded, "g") == nil {
		t.Errorf("expected renamed 'g' in saved meta")
	}
	if loader.FindMeta(loaded, "git") != nil {
		t.Errorf("old 'git' should not be in saved meta")
	}
}

func TestUpdateBriefOpenActions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	keyO := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")}
	keyC := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")}

	t.Run("no repo sets status message and no command", func(t *testing.T) {
		for _, key := range []tea.KeyMsg{keyO, keyC} {
			m := Model{
				meta:         []loader.ToolMeta{{Name: "tool-x"}},
				metaSelected: 0,
				focus:        focusBrief,
			}
			m.tools = loader.ToolsFromMeta(m.meta)

			updated, cmd := m.Update(key)
			nm := updated.(Model)

			if nm.statusMsg != "no repo for tool-x" {
				t.Errorf("key %q: statusMsg = %q, want %q", key.String(), nm.statusMsg, "no repo for tool-x")
			}
			if cmd != nil {
				t.Errorf("key %q: cmd = %v, want nil for no-repo tool", key.String(), cmd)
			}
		}
	})

	t.Run("repo set returns a non-nil command", func(t *testing.T) {
		for _, key := range []tea.KeyMsg{keyO, keyC} {
			m := Model{
				meta:         []loader.ToolMeta{{Name: "tool-x", GitHub: "github.com/owner/tool-x"}},
				metaSelected: 0,
				focus:        focusBrief,
			}
			m.tools = loader.ToolsFromMeta(m.meta)

			updated, cmd := m.Update(key)
			nm := updated.(Model)

			if nm.statusMsg != "" {
				t.Errorf("key %q: statusMsg = %q, want empty", key.String(), nm.statusMsg)
			}
			if cmd == nil {
				t.Errorf("key %q: cmd = nil, want non-nil for tool with repo", key.String())
			}
		}
	})
}

func TestUpdateBriefStatusCycle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	keyS := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")}

	t.Run("cycles status through the full loop", func(t *testing.T) {
		m := Model{
			meta:         []loader.ToolMeta{{Name: "tool-x", Status: loader.StatusActive}},
			metaSelected: 0,
			focus:        focusBrief,
		}
		m.tools = loader.ToolsFromMeta(m.meta)

		want := []loader.Status{
			loader.StatusTrying,
			loader.StatusInactive,
			loader.StatusActive,
		}

		var cur tea.Model = m
		for i, w := range want {
			updated, _ := cur.(Model).Update(keyS)
			nm := updated.(Model)
			got := loader.FindMeta(nm.meta, "tool-x").Status
			if got != w {
				t.Errorf("step %d: status = %q, want %q", i, got, w)
			}
			cur = nm
		}
	})

	t.Run("inert outside focusBrief", func(t *testing.T) {
		m := Model{
			meta:         []loader.ToolMeta{{Name: "tool-x", Status: loader.StatusActive}},
			metaSelected: 0,
			focus:        focusTools,
		}
		m.tools = loader.ToolsFromMeta(m.meta)

		updated, _ := m.Update(keyS)
		nm := updated.(Model)
		if got := loader.FindMeta(nm.meta, "tool-x").Status; got != loader.StatusActive {
			t.Errorf("status = %q, want %q (unchanged outside focusBrief)", got, loader.StatusActive)
		}
	})
}

func TestScrollColumn(t *testing.T) {
	const thumb = "▐"

	t.Run("no thumb when content fits", func(t *testing.T) {
		vp := viewport.New(10, 5)
		vp.SetContent("one\ntwo")
		if got := scrollColumn(vp, true); strings.Contains(got, thumb) {
			t.Errorf("expected no thumb for non-scrollable content, got %q", got)
		}
	})

	t.Run("thumb when content overflows", func(t *testing.T) {
		vp := viewport.New(10, 3)
		vp.SetContent(strings.Repeat("line\n", 20))
		if got := scrollColumn(vp, true); !strings.Contains(got, thumb) {
			t.Errorf("expected thumb for scrollable content, got %q", got)
		}
	})
}

// countBatchedCmds executes cmd and reports how many commands it batches.
// A nil cmd counts as 0; a single non-batch cmd counts as 1. Only call this
// when the batched cmds are side-effect free to execute (or when a BatchMsg is
// expected), since tea.Batch collapses a lone cmd into that cmd directly.
func countBatchedCmds(cmd tea.Cmd) int {
	if cmd == nil {
		return 0
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		return len(msg)
	default:
		return 1
	}
}

func TestFetchInstalledCmd(t *testing.T) {
	// A nonexistent name makes InstalledVersion skip exec, so the closure runs
	// with no network I/O and never touches GitHub.
	cmd := fetchInstalledCmd(loader.Tool{Name: "nonexistent-tool-xyz", GitHub: ""})
	if cmd == nil {
		t.Fatal("expected non-nil tea.Cmd from fetchInstalledCmd")
	}
	msg, ok := cmd().(installedMsg)
	if !ok {
		t.Fatalf("expected installedMsg, got %T", cmd())
	}
	if msg.toolName != "nonexistent-tool-xyz" {
		t.Errorf("toolName = %q, want %q", msg.toolName, "nonexistent-tool-xyz")
	}
}

func TestNeedsInstalled(t *testing.T) {
	tests := []struct {
		name     string
		tool     loader.Tool
		versions map[string]VersionInfo
		want     bool
	}{
		{
			name: "fresh tool needs installed",
			tool: loader.Tool{Name: "git", GitHub: "cli/cli"},
			want: true,
		},
		{
			name:     "known installed does not need refetch",
			tool:     loader.Tool{Name: "git", GitHub: "cli/cli"},
			versions: map[string]VersionInfo{"git": {Installed: "1.0", InstalledKnown: true}},
			want:     false,
		},
		{
			name:     "probed-but-missing does not need refetch",
			tool:     loader.Tool{Name: "git", GitHub: "cli/cli"},
			versions: map[string]VersionInfo{"git": {Installed: "", InstalledKnown: true}},
			want:     false,
		},
		{
			name:     "entry with only Latest still needs installed",
			tool:     loader.Tool{Name: "git", GitHub: "cli/cli"},
			versions: map[string]VersionInfo{"git": {Latest: "2.0"}},
			want:     true,
		},
		{
			name: "installed fires even without GitHub",
			tool: loader.Tool{Name: "git", GitHub: ""},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Model{versions: tt.versions}
			if got := m.needsInstalled(tt.tool); got != tt.want {
				t.Errorf("needsInstalled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNeedsRemote(t *testing.T) {
	tests := []struct {
		name      string
		tool      loader.Tool
		repoCards map[string]version.RepoCard
		versions  map[string]VersionInfo
		want      bool
	}{
		{
			name: "fresh tool with GitHub needs remote",
			tool: loader.Tool{Name: "git", GitHub: "cli/cli"},
			want: true,
		},
		{
			name:      "card and latest present: no remote",
			tool:      loader.Tool{Name: "git", GitHub: "cli/cli"},
			repoCards: map[string]version.RepoCard{"git": {}},
			versions:  map[string]VersionInfo{"git": {Latest: "2.0"}},
			want:      false,
		},
		{
			name:      "card present but latest empty: needs remote",
			tool:      loader.Tool{Name: "git", GitHub: "cli/cli"},
			repoCards: map[string]version.RepoCard{"git": {}},
			versions:  map[string]VersionInfo{"git": {Installed: "1.0"}},
			want:      true,
		},
		{
			name: "remote not needed without GitHub",
			tool: loader.Tool{Name: "git", GitHub: ""},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Model{repoCards: tt.repoCards, versions: tt.versions}
			if got := m.needsRemote(tt.tool); got != tt.want {
				t.Errorf("needsRemote() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAutoFetchCmdsForSelected_QueuesFetches(t *testing.T) {
	name := "git"
	// Changelog and --help are already cached so those branches append nothing;
	// only version + repo card are missing. This isolates the new fetch block:
	// a non-nil batch alone would pass even without it (changelog/help fire too),
	// so assert the batch holds exactly the two expected commands.
	m := &Model{
		meta:          []loader.ToolMeta{{Name: name, GitHub: "cli/cli"}},
		tools:         []loader.Tool{{Name: name, GitHub: "cli/cli"}},
		metaSelected:  0,
		changelogData: map[string]changelogMsg{name: {}},
		helpCache:     map[string][2]string{name: {helpModeHelp: "cached"}},
	}
	cmd := m.autoFetchCmdsForSelected()
	if cmd == nil {
		t.Fatal("expected non-nil batched Cmd queuing version + repo card fetches")
	}
	if got := countBatchedCmds(cmd); got != 2 {
		t.Fatalf("expected exactly 2 queued cmds (version + repo card), got %d", got)
	}
}

func TestAutoFetchCmdsForSelected_NoFetchWhenCached(t *testing.T) {
	name := "git"
	m := &Model{
		meta:          []loader.ToolMeta{{Name: name, GitHub: "cli/cli"}},
		tools:         []loader.Tool{{Name: name, GitHub: "cli/cli"}},
		metaSelected:  0,
		changelogData: map[string]changelogMsg{name: {}},
		helpCache:     map[string][2]string{name: {helpModeHelp: "cached help"}},
		versions:      map[string]VersionInfo{name: {Installed: "1.0", Latest: "2.0", InstalledKnown: true}},
		repoCards:     map[string]version.RepoCard{name: {}},
	}
	if m.needsInstalled(m.tools[0]) {
		t.Error("needsInstalled should be false when installed version is cached")
	}
	if m.needsRemote(m.tools[0]) {
		t.Error("needsRemote should be false when repo card and latest are cached")
	}
	if cmd := m.autoFetchCmdsForSelected(); cmd != nil {
		t.Fatal("expected nil Cmd when all sources are already cached")
	}
}

func TestUpdateRenameInputClearsStaleCaches(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	old := "cli"
	newName := "gh"
	m := Model{
		meta:          []loader.ToolMeta{{Name: old, GitHub: "cli/cli"}},
		metaSelected:  0,
		mode:          modeRename,
		nameInput:     textinput.New(),
		repoCards:     map[string]version.RepoCard{old: {}},
		versions:      map[string]VersionInfo{old: {}},
		repoStatus:    map[string]string{old: "ok"},
		changelogData: map[string]changelogMsg{old: {}},
		helpCache:     map[string][2]string{old: {helpModeHelp: "cached"}},
	}
	m.tools = loader.ToolsFromMeta(m.meta)
	m.nameInput.SetValue(newName)

	updated, _ := m.updateRenameInput(tea.KeyMsg{Type: tea.KeyEnter})
	nm := updated.(Model)

	if _, ok := nm.repoCards[old]; ok {
		t.Errorf("repoCards still holds stale old-name key %q after rename", old)
	}
	if _, ok := nm.versions[old]; ok {
		t.Errorf("versions still holds stale old-name key %q after rename", old)
	}
	if _, ok := nm.repoStatus[old]; ok {
		t.Errorf("repoStatus still holds stale old-name key %q after rename", old)
	}
	if _, ok := nm.changelogData[old]; ok {
		t.Errorf("changelogData still holds stale old-name key %q after rename", old)
	}
	if _, ok := nm.helpCache[old]; ok {
		t.Errorf("helpCache still holds stale old-name key %q after rename", old)
	}
}

// TestUpdateInstalledAndRemoteMsgPopulateCaches closes the loop the rename test
// opens: after stale keys are cleared, the async fetch results must repopulate
// the caches under the (new) tool name. It also proves installedMsg and
// remoteMsg merge into one VersionInfo without either clobbering the other's
// field, in both arrival orders.
func TestUpdateInstalledAndRemoteMsgPopulateCaches(t *testing.T) {
	newModel := func() Model {
		m := Model{
			meta:          []loader.ToolMeta{{Name: "gh", GitHub: "cli/cli"}},
			metaSelected:  0,
			versions:      map[string]VersionInfo{},
			repoStatus:    map[string]string{},
			repoCards:     map[string]version.RepoCard{},
			changelogData: map[string]changelogMsg{},
		}
		m.tools = loader.ToolsFromMeta(m.meta)
		return m
	}

	// installed first, then remote.
	m := newModel()
	updated, _ := m.Update(installedMsg{toolName: "gh", installed: "1.0"})
	nm := updated.(Model)
	if got := nm.versions["gh"]; got.Installed != "1.0" {
		t.Errorf("after installedMsg versions[gh].Installed = %q, want 1.0", got.Installed)
	}
	updated, _ = nm.Update(remoteMsg{toolName: "gh", latest: "2.0", repoStatus: "active", card: version.RepoCard{About: "x"}})
	nm = updated.(Model)
	if got := nm.versions["gh"]; got.Installed != "1.0" || got.Latest != "2.0" {
		t.Errorf("versions[gh] = %+v, want {Installed:1.0 Latest:2.0}", got)
	}
	if got := nm.repoStatus["gh"]; got != "active" {
		t.Errorf("repoStatus[gh] = %q, want active", got)
	}
	if got, ok := nm.repoCards["gh"]; !ok || got.About != "x" {
		t.Errorf("repoCards[gh] = %+v (ok=%v), want About:x", got, ok)
	}

	// remote first, then installed — installed must not wipe Latest.
	m = newModel()
	updated, _ = m.Update(remoteMsg{toolName: "gh", latest: "2.0", card: version.RepoCard{}})
	nm = updated.(Model)
	updated, _ = nm.Update(installedMsg{toolName: "gh", installed: "1.0"})
	nm = updated.(Model)
	if got := nm.versions["gh"]; got.Installed != "1.0" || got.Latest != "2.0" {
		t.Errorf("reversed order versions[gh] = %+v, want {Installed:1.0 Latest:2.0}", got)
	}

	// remoteMsg with err set must not touch the caches.
	m = newModel()
	updated, _ = m.Update(remoteMsg{toolName: "gh", latest: "2.0", err: errBoom})
	nm = updated.(Model)
	if _, ok := nm.repoCards["gh"]; ok {
		t.Errorf("repoCards populated despite remoteMsg error")
	}
	if got := nm.versions["gh"]; got.Latest != "" {
		t.Errorf("versions[gh].Latest = %q, want empty on remoteMsg error", got.Latest)
	}
}

var errBoom = errors.New("boom")

func newRateModel() Model {
	m := Model{
		meta:          []loader.ToolMeta{{Name: "gh", GitHub: "cli/cli"}},
		metaSelected:  0,
		versions:      map[string]VersionInfo{},
		repoStatus:    map[string]string{},
		repoCards:     map[string]version.RepoCard{},
		changelogData: map[string]changelogMsg{},
	}
	m.tools = loader.ToolsFromMeta(m.meta)
	return m
}

// TestRemoteMsgRateMerge verifies the non-clobber merge: a Known snapshot is
// stored, and a later Known==false snapshot (a cache-hit remote fetch) does not
// wipe it.
func TestRemoteMsgRateMerge(t *testing.T) {
	known := version.RateLimit{Limit: 5000, Remaining: 4999, Known: true}
	m := newRateModel()
	updated, _ := m.Update(remoteMsg{toolName: "gh", latest: "2.0", rate: known})
	nm := updated.(Model)
	if nm.rate != known {
		t.Fatalf("m.rate = %+v, want %+v", nm.rate, known)
	}

	// A Known==false snapshot must not overwrite the known value.
	updated, _ = nm.Update(remoteMsg{toolName: "gh", latest: "2.1", rate: version.RateLimit{}})
	nm = updated.(Model)
	if nm.rate != known {
		t.Errorf("Known==false remoteMsg clobbered m.rate: got %+v, want %+v", nm.rate, known)
	}
}

// TestRateMsgHandler verifies rateMsg stores a Known snapshot and that an error
// (or Known==false) leaves a previously known m.rate untouched.
func TestRateMsgHandler(t *testing.T) {
	known := version.RateLimit{Limit: 5000, Remaining: 100, Known: true}
	m := newRateModel()
	updated, _ := m.Update(rateMsg{rate: known})
	nm := updated.(Model)
	if nm.rate != known {
		t.Fatalf("after rateMsg m.rate = %+v, want %+v", nm.rate, known)
	}

	// An error snapshot must not clobber the known value.
	updated, _ = nm.Update(rateMsg{rate: version.RateLimit{Limit: 60, Known: true}, err: errBoom})
	nm = updated.(Model)
	if nm.rate != known {
		t.Errorf("errored rateMsg clobbered m.rate: got %+v, want %+v", nm.rate, known)
	}
}

// TestRemoteMsgRateLimitedHint verifies that a rate-limited remoteMsg with no
// card sets the "rate-limited" repoStatus so the card can render a hint.
func TestRemoteMsgRateLimitedHint(t *testing.T) {
	m := newRateModel()
	updated, _ := m.Update(remoteMsg{
		toolName:   "gh",
		repoStatus: "rate-limited",
		rate:       version.RateLimit{Limit: 60, Remaining: 0, Known: true},
		err:        version.ErrRateLimited,
	})
	nm := updated.(Model)
	if got := nm.repoStatus["gh"]; got != "rate-limited" {
		t.Errorf("repoStatus[gh] = %q, want rate-limited", got)
	}
	if _, ok := nm.repoCards["gh"]; ok {
		t.Errorf("repoCards populated despite rate-limited error")
	}
	if !nm.rate.Known || nm.rate.Remaining != 0 {
		t.Errorf("m.rate = %+v, want Known with Remaining 0", nm.rate)
	}
	// The card must actually render the hint, not just set the internal map.
	if card := stripANSI(nm.renderCard()); !strings.Contains(card, "rate limited — press [L]") {
		t.Errorf("renderCard() missing rate-limit hint; got:\n%s", card)
	}
}

// TestRemoteMsgRateLimitedKeepsStaleData verifies that a rate-limit error
// accompanied by usable stale/partial cache data still populates the caches
// (known tags and cards must survive a rate-limited outage), rather than being
// dropped in favour of the empty "rate limited" hint.
func TestRemoteMsgRateLimitedKeepsStaleData(t *testing.T) {
	m := newRateModel()
	updated, _ := m.Update(remoteMsg{
		toolName:   "gh",
		latest:     "2.0",
		repoStatus: "active",
		card:       version.RepoCard{About: "stale about", Latest: "2.0"},
		err:        version.ErrRateLimited,
	})
	nm := updated.(Model)
	if got := nm.versions["gh"]; got.Latest != "2.0" {
		t.Errorf("versions[gh].Latest = %q, want 2.0 (stale data dropped)", got.Latest)
	}
	if got := nm.repoStatus["gh"]; got != "active" {
		t.Errorf("repoStatus[gh] = %q, want active", got)
	}
	if got, ok := nm.repoCards["gh"]; !ok || got.About != "stale about" {
		t.Errorf("repoCards[gh] = %+v (ok=%v), want About:stale about", got, ok)
	}
}

func TestMaskToken(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"ghp_1234567890abcdef3f2a", "ghp_••••••••3f2a"},
		{"12345678", "••••••••"},
		{"abc", "•••"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := maskToken(tt.in); got != tt.want {
			t.Errorf("maskToken(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRenderAPIStatusOverlay(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_1234567890abcdef3f2a")

	m := Model{width: 80, height: 24, mode: modeAPIStatus}
	m.rate = version.RateLimit{Known: true, Remaining: 0, Limit: 60}
	got := m.renderAPIStatus()

	// Used/limit (not remaining): Remaining 0 of 60 → "60 / 60".
	for _, want := range []string{"GitHub API status", "env", "ghp_••••••••3f2a", "Used: 60 / 60", "✕", "[e]", "[r]", "[esc]"} {
		if !strings.Contains(got, want) {
			t.Errorf("overlay = %q, missing %q", got, want)
		}
	}
	// [d] remove token is hidden for the env source.
	if strings.Contains(got, "remove token") {
		t.Errorf("overlay = %q, should not offer remove token for env source", got)
	}
	// Token hint is hidden when a token is configured (env source here).
	if strings.Contains(got, "raise the limit") {
		t.Errorf("overlay = %q, should not show token hint when a token exists", got)
	}
}

func TestRenderAPIStatusUsedLimit(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_1234567890abcdef3f2a")
	m := Model{width: 80, height: 24, mode: modeAPIStatus}
	m.rate = version.RateLimit{Known: true, Remaining: 15, Limit: 60}
	got := m.renderAPIStatus()
	if !strings.Contains(got, "Used: 45 / 60") {
		t.Errorf("overlay = %q, want used/limit line 'Used: 45 / 60'", got)
	}
	if strings.Contains(got, "Limit: 15") {
		t.Errorf("overlay = %q, should not show remaining as the count", got)
	}
}

func TestRenderAPIStatusTokenHint(t *testing.T) {
	t.Run("shown when no token", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("GITHUB_TOKEN", "")
		if version.TokenSource() != "none" {
			t.Skipf("precondition: TokenSource() = %q, want none", version.TokenSource())
		}
		m := Model{width: 80, height: 24, mode: modeAPIStatus}
		m.rate = version.RateLimit{Known: true, Remaining: 30, Limit: 60}
		if got := m.renderAPIStatus(); !strings.Contains(got, "raise the limit") {
			t.Errorf("overlay = %q, missing token hint when no token", got)
		}
	})

	t.Run("hidden while entering a token", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("GITHUB_TOKEN", "")
		m := Model{width: 80, height: 24, mode: modeTokenInput, tokenInput: textinput.New()}
		if got := m.renderAPIStatus(); strings.Contains(got, "raise the limit") {
			t.Errorf("overlay = %q, should hide token hint while entering a token", got)
		}
	})
}

// sgrParamRe captures the parameter list of each SGR escape sequence.
var sgrParamRe = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// hasItalic reports whether s contains an SGR sequence enabling italics
// (parameter 3, possibly merged with colors, e.g. "\x1b[3;38;5;145m").
func hasItalic(s string) bool {
	for _, match := range sgrParamRe.FindAllStringSubmatch(s, -1) {
		if slices.Contains(strings.Split(match[1], ";"), "3") {
			return true
		}
	}
	return false
}

// TestRenderAPIStatusHintsNotItalic pins the overlay styling: no part of the
// overlay (in particular the hint line) may render in italics, in either the
// read-only view or the token-input sub-state.
func TestRenderAPIStatusHintsNotItalic(t *testing.T) {
	forceColorProfile(t)
	t.Setenv("GITHUB_TOKEN", "ghp_1234567890abcdef3f2a")

	for _, mode := range []inputMode{modeAPIStatus, modeTokenInput} {
		m := Model{width: 80, height: 24, mode: mode, tokenInput: textinput.New()}
		m.rate = version.RateLimit{Known: true, Remaining: 15, Limit: 60}
		if got := m.renderAPIStatus(); hasItalic(got) {
			t.Errorf("mode %d: overlay contains italic styling: %q", mode, got)
		}
	}
}

func TestRenderAPIStatusWarnIcon(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")

	m := Model{width: 80, height: 24, mode: modeAPIStatus}
	m.rate = version.RateLimit{Known: true, Remaining: rateLowThreshold, Limit: 60}
	got := m.renderAPIStatus()
	if !strings.Contains(got, "⚠") {
		t.Errorf("overlay = %q, missing warn icon", got)
	}
}

func TestAPIStatusOverlayToggle(t *testing.T) {
	m := Model{width: 80, height: 24, focus: focusTools, ready: true}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("L")})
	nm := updated.(Model)
	if nm.mode != modeAPIStatus {
		t.Fatalf("pressing L did not open the API-status overlay")
	}
	if cmd == nil {
		t.Errorf("pressing L should fire a rate fetch cmd")
	}
	// esc closes it.
	updated2, _ := nm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated2.(Model).apiOverlayVisible() {
		t.Errorf("esc did not close the API-status overlay")
	}
}

// TestUpdateAPIStatusOpensTokenEntry verifies [e] switches the overlay into the
// masked token-input sub-mode.
func TestUpdateAPIStatusOpensTokenEntry(t *testing.T) {
	m := Model{width: 80, height: 24, mode: modeAPIStatus, tokenInput: textinput.New()}
	updated, cmd := m.updateAPIStatus(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	nm := updated.(Model)
	if nm.mode != modeTokenInput {
		t.Fatal("pressing e did not enter token-input mode")
	}
	if cmd == nil {
		t.Error("expected a blink cmd when entering token mode")
	}
}

// TestTokenValidatedMsgInvalid verifies a 401 result shows the inline error,
// keeps the input open, and never persists a token.
func TestTokenValidatedMsgInvalid(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	version.ClearToken() //nolint:errcheck

	m := Model{width: 80, height: 24, mode: modeTokenInput, tokenInput: textinput.New()}
	updated, _ := m.Update(tokenValidatedMsg{token: "ghp_bad", err: version.ErrTokenInvalid})
	nm := updated.(Model)
	if nm.tokenError != "token invalid" {
		t.Errorf("tokenError = %q, want %q", nm.tokenError, "token invalid")
	}
	if nm.mode != modeTokenInput {
		t.Error("invalid token should keep the input open for a retry")
	}
	if src := version.TokenSource(); src != "none" {
		t.Errorf("invalid token must not be stored, TokenSource() = %q", src)
	}
}

// TestTokenValidatedMsgValid verifies a 200 result persists the token, exits the
// input, updates the rate snapshot, and fires the card-backfill cmd.
func TestTokenValidatedMsgValid(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	version.ClearToken()                       //nolint:errcheck
	t.Cleanup(func() { version.ClearToken() }) //nolint:errcheck

	name := "git"
	m := Model{
		width: 80, height: 24, mode: modeTokenInput,
		tokenInput:    textinput.New(),
		tokenError:    "token invalid",
		meta:          []loader.ToolMeta{{Name: name, GitHub: "cli/cli"}},
		tools:         []loader.Tool{{Name: name, GitHub: "cli/cli"}},
		changelogData: map[string]changelogMsg{name: {}},
		helpCache:     map[string][2]string{name: {helpModeHelp: "cached"}},
		versions:      map[string]VersionInfo{},
		repoCards:     map[string]version.RepoCard{},
	}
	rate := version.RateLimit{Known: true, Remaining: 4999, Limit: 5000}
	updated, cmd := m.Update(tokenValidatedMsg{token: "ghp_goodtoken1234", rate: rate})
	nm := updated.(Model)
	if version.TokenSource() != "config" {
		t.Fatalf("valid token was not stored, TokenSource() = %q", version.TokenSource())
	}
	if nm.mode == modeTokenInput {
		t.Error("valid token should exit the token-input mode")
	}
	if nm.tokenError != "" {
		t.Errorf("tokenError should be cleared, got %q", nm.tokenError)
	}
	if nm.rate.Limit != 5000 {
		t.Errorf("m.rate not updated from the validated snapshot, got %+v", nm.rate)
	}
	if cmd == nil {
		t.Error("valid token should fire the backfill cmd")
	}
}

// TestUpdateAPIStatusRemoveToken verifies [d] clears a config-sourced token.
func TestUpdateAPIStatusRemoveToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := version.SetToken("ghp_config1234567"); err != nil {
		t.Fatalf("SetToken: %v", err)
	}
	t.Cleanup(func() { version.ClearToken() }) //nolint:errcheck
	if version.TokenSource() != "config" {
		t.Fatalf("precondition: TokenSource() = %q, want config", version.TokenSource())
	}

	m := Model{width: 80, height: 24, mode: modeAPIStatus, tokenInput: textinput.New()}
	m.updateAPIStatus(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if src := version.TokenSource(); src != "none" {
		t.Errorf("[d] did not clear the token, TokenSource() = %q", src)
	}
}

// TestRenderStatusBarTokenInput verifies the status bar reflects the token-input
// sub-mode.
func TestRenderStatusBarTokenInput(t *testing.T) {
	m := Model{width: 80, height: 24, mode: modeTokenInput, tokenInput: textinput.New()}
	got := m.renderStatusBar()
	if !strings.Contains(got, "validate & save") {
		t.Errorf("status bar = %q, missing token-input hint", got)
	}
}

// TestRenderAPIStatusTokenEntry verifies the overlay shows the masked input and
// the inline error while entering a token.
func TestRenderAPIStatusTokenEntry(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	m := Model{width: 80, height: 24, mode: modeTokenInput, tokenInput: textinput.New()}
	m.tokenError = "token invalid"
	got := m.renderAPIStatus()
	for _, want := range []string{"token:", "token invalid", "validate & save"} {
		if !strings.Contains(got, want) {
			t.Errorf("overlay = %q, missing %q", got, want)
		}
	}
}

// TestRefreshCmdsEmitTypedMsgs verifies the force-refresh commands emit the same
// message types as their non-force variants, carrying the tool name. Uses an
// empty GitHub field so the version layer returns without any network call.
func TestRefreshCmdsEmitTypedMsgs(t *testing.T) {
	t.Run("remote", func(t *testing.T) {
		msg := refreshRemoteCmd(loader.Tool{Name: "tool"})()
		rm, ok := msg.(remoteMsg)
		if !ok {
			t.Fatalf("refreshRemoteCmd emitted %T, want remoteMsg", msg)
		}
		if rm.toolName != "tool" {
			t.Errorf("remoteMsg.toolName = %q, want tool", rm.toolName)
		}
	})
	t.Run("changelog", func(t *testing.T) {
		msg := refreshChangelogCmd("", "tool")()
		cm, ok := msg.(changelogMsg)
		if !ok {
			t.Fatalf("refreshChangelogCmd emitted %T, want changelogMsg", msg)
		}
		if cm.toolName != "tool" {
			t.Errorf("changelogMsg.toolName = %q, want tool", cm.toolName)
		}
	})
}

// TestSpinnerTickGateWhenIdle verifies a spinner tick is a no-op (no rescheduled
// command) when no refresh is in flight, so the animation loop halts when idle.
func TestSpinnerTickGateWhenIdle(t *testing.T) {
	m := Model{width: 80, focus: focusBrief}
	_, cmd := m.Update(spinner.TickMsg{})
	if cmd != nil {
		t.Errorf("idle spinner tick returned a command %v, want nil (loop should halt)", cmd)
	}
}

// TestUpdateBriefRefresh covers the [r] refresh action in the brief panel: it
// starts a refresh (sets refreshingFor + status) for a repo-backed tool, the
// remoteMsg completion clears it, a no-repo tool only reports status, a repeat
// press is a no-op guard, and [r] in the tool list still starts a rename.
func TestUpdateBriefRefresh(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	keyR := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")}

	t.Run("repo tool starts refresh", func(t *testing.T) {
		m := Model{
			meta:         []loader.ToolMeta{{Name: "tool-x", GitHub: "github.com/owner/tool-x"}},
			metaSelected: 0,
			focus:        focusBrief,
		}
		m.tools = loader.ToolsFromMeta(m.meta)

		updated, cmd := m.Update(keyR)
		nm := updated.(Model)

		if nm.refreshingFor != "tool-x" {
			t.Errorf("refreshingFor = %q, want tool-x", nm.refreshingFor)
		}
		// The status bar is not taken over — the "refreshing" hint lives in the
		// card title, not the status bar.
		if nm.statusMsg != "" {
			t.Errorf("statusMsg = %q, want empty (no status-bar takeover)", nm.statusMsg)
		}
		if cmd == nil {
			t.Error("cmd = nil, want a non-nil refresh batch")
		}
	})

	t.Run("remoteMsg completion clears refresh state", func(t *testing.T) {
		m := Model{
			meta:          []loader.ToolMeta{{Name: "tool-x", GitHub: "github.com/owner/tool-x"}},
			metaSelected:  0,
			focus:         focusBrief,
			refreshingFor: "tool-x",
			versions:      map[string]VersionInfo{},
			repoStatus:    map[string]string{},
			repoCards:     map[string]version.RepoCard{},
		}
		m.tools = loader.ToolsFromMeta(m.meta)

		updated, _ := m.Update(remoteMsg{toolName: "tool-x", latest: "v1.0.0"})
		nm := updated.(Model)

		if nm.refreshingFor != "" {
			t.Errorf("refreshingFor = %q, want cleared after remoteMsg", nm.refreshingFor)
		}
	})

	t.Run("no-repo tool reports status without refresh state", func(t *testing.T) {
		m := Model{
			meta:         []loader.ToolMeta{{Name: "tool-x"}},
			metaSelected: 0,
			focus:        focusBrief,
		}
		m.tools = loader.ToolsFromMeta(m.meta)

		updated, _ := m.Update(keyR)
		nm := updated.(Model)

		if nm.refreshingFor != "" {
			t.Errorf("refreshingFor = %q, want empty for no-repo tool", nm.refreshingFor)
		}
		if nm.statusMsg != "no repo to refresh" {
			t.Errorf("statusMsg = %q, want \"no repo to refresh\"", nm.statusMsg)
		}
	})

	t.Run("repeat press while refreshing is a no-op guard", func(t *testing.T) {
		m := Model{
			meta:          []loader.ToolMeta{{Name: "tool-x", GitHub: "github.com/owner/tool-x"}},
			metaSelected:  0,
			focus:         focusBrief,
			refreshingFor: "tool-x",
		}
		m.tools = loader.ToolsFromMeta(m.meta)

		updated, cmd := m.Update(keyR)
		nm := updated.(Model)

		if nm.refreshingFor != "tool-x" {
			t.Errorf("refreshingFor = %q, want tool-x unchanged", nm.refreshingFor)
		}
		if cmd != nil {
			t.Errorf("cmd = %v, want nil (guarded second press)", cmd)
		}
	})

	t.Run("r in tool list still starts rename", func(t *testing.T) {
		m := Model{
			meta:         []loader.ToolMeta{{Name: "tool-x"}},
			metaSelected: 0,
			focus:        focusTools,
			nameInput:    textinput.New(),
		}
		m.tools = loader.ToolsFromMeta(m.meta)

		updated, _ := m.Update(keyR)
		nm := updated.(Model)

		if nm.mode != modeRename {
			t.Error("renaming = false, want true (r in focusTools opens rename)")
		}
		if nm.refreshingFor != "" {
			t.Errorf("refreshingFor = %q, want empty in focusTools", nm.refreshingFor)
		}
	})
}

// TestBriefHelpBarHasRefresh verifies the focusBrief help bar advertises [r] refresh.
func TestBriefHelpBarHasRefresh(t *testing.T) {
	m := Model{width: 120, focus: focusBrief}
	bar := m.renderStatusBar()
	if !strings.Contains(bar, "refresh") {
		t.Errorf("focusBrief help bar = %q, want it to mention \"refresh\"", bar)
	}
}

// TestRenderCardSpinner verifies that while a tool is refreshing the card title
// becomes "refreshing <name> data <spinner>" (hiding the about), and reverts to
// name + about when idle.
func TestRenderCardSpinner(t *testing.T) {
	m := Model{
		meta:         []loader.ToolMeta{{Name: "tool-x", GitHub: "github.com/owner/tool-x"}},
		metaSelected: 0,
		briefW:       80,
		repoCards:    map[string]version.RepoCard{"tool-x": {About: "a fine tool"}},
	}
	m.tools = loader.ToolsFromMeta(m.meta)
	m.spinner = spinner.New()
	m.spinner.Spinner = spinner.MiniDot
	frame := m.spinner.View()

	m.refreshingFor = "tool-x"
	withSpin := m.renderCard()
	for _, want := range []string{"refreshing ", "tool-x", " data ", frame} {
		if !strings.Contains(withSpin, want) {
			t.Errorf("refreshing card = %q, want it to contain %q", withSpin, want)
		}
	}
	if strings.Contains(withSpin, "a fine tool") {
		t.Errorf("refreshing card = %q, want the about hidden while refreshing", withSpin)
	}

	m.refreshingFor = ""
	noSpin := m.renderCard()
	if strings.Contains(noSpin, frame) {
		t.Errorf("idle card = %q, want no spinner frame %q", noSpin, frame)
	}
	if !strings.Contains(noSpin, "a fine tool") {
		t.Errorf("idle card = %q, want the about shown when not refreshing", noSpin)
	}
}

// TestRenderCardInstalledLatest covers the [info] version lines: installed:
// renders whenever the section is open (muted "detecting…" while the local
// probe is in flight, "✕ not found" once it reported empty), the section
// opens for a GitHub-less tool once an installed version is known, and
// latest: gains the update highlight + ↑ only when the installed version is
// older. The model goes through New + WindowSizeMsg so renderCard sees the
// same initialized state (spinner, widths) as the running app.
func TestRenderCardInstalledLatest(t *testing.T) {
	newCardModel := func(github string) Model {
		m := New([]loader.ToolMeta{{Name: "gh", GitHub: github}})
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
		return updated.(Model)
	}

	t.Run("up to date: both lines, no arrow", func(t *testing.T) {
		m := newCardModel("cli/cli")
		m.versions["gh"] = VersionInfo{Installed: "v2.0.0", Latest: "v2.0.0", InstalledKnown: true}
		m.repoCards["gh"] = version.RepoCard{Latest: "v2.0.0"}
		card := stripANSI(m.renderCard())
		if !strings.Contains(card, "installed: v2.0.0") {
			t.Errorf("card missing installed line; got:\n%s", card)
		}
		if !strings.Contains(card, "latest:  v2.0.0") {
			t.Errorf("card missing latest line; got:\n%s", card)
		}
		if strings.Contains(card, "↑") {
			t.Errorf("up-to-date card shows update arrow; got:\n%s", card)
		}
	})

	t.Run("update available: arrow and date suffix", func(t *testing.T) {
		m := newCardModel("cli/cli")
		m.versions["gh"] = VersionInfo{Installed: "v1.0.0", Latest: "v2.0.0", InstalledKnown: true}
		m.repoCards["gh"] = version.RepoCard{Latest: "v2.0.0", PublishedAt: "2026-01-02T15:04:05Z"}
		card := stripANSI(m.renderCard())
		if !strings.Contains(card, "latest:  v2.0.0 ↑ (2026-01-02)") {
			t.Errorf("card missing highlighted latest with arrow; got:\n%s", card)
		}
		if !strings.Contains(card, "installed: v1.0.0") {
			t.Errorf("card missing installed line; got:\n%s", card)
		}
	})

	t.Run("detection reported empty: not found", func(t *testing.T) {
		m := newCardModel("cli/cli")
		m.versions["gh"] = VersionInfo{Latest: "v2.0.0", InstalledKnown: true}
		m.repoCards["gh"] = version.RepoCard{Latest: "v2.0.0"}
		card := stripANSI(m.renderCard())
		if !strings.Contains(card, "installed: ✕ not found") {
			t.Errorf("card missing installed fallback with ✕ marker; got:\n%s", card)
		}
	})

	t.Run("detection pending: detecting, not \"not found\"", func(t *testing.T) {
		m := newCardModel("cli/cli")
		m.versions["gh"] = VersionInfo{Latest: "v2.0.0"}
		m.repoCards["gh"] = version.RepoCard{Latest: "v2.0.0"}
		card := stripANSI(m.renderCard())
		if !strings.Contains(card, "installed: detecting…") {
			t.Errorf("card missing pending installed line; got:\n%s", card)
		}
		if strings.Contains(card, "not found") || strings.Contains(card, "✕") {
			t.Errorf("card claims not found before detection finished; got:\n%s", card)
		}
	})

	t.Run("no version data at all: detecting, no latest", func(t *testing.T) {
		m := newCardModel("cli/cli")
		card := stripANSI(m.renderCard())
		if !strings.Contains(card, "installed: detecting…") {
			t.Errorf("card missing pending installed line; got:\n%s", card)
		}
		if strings.Contains(card, "latest:") {
			t.Errorf("card shows latest with no card data; got:\n%s", card)
		}
	})

	// A repo card can exist with no release at all (repo info + stars fetched,
	// no tagged release): the latest: line and its tag icon are gated on
	// card.Latest != "", so neither may appear.
	t.Run("card without a release: no latest line, no tag icon", func(t *testing.T) {
		m := newCardModel("cli/cli")
		m.versions["gh"] = VersionInfo{Installed: "v1.0.0", InstalledKnown: true}
		m.repoCards["gh"] = version.RepoCard{Stars: 42}
		card := stripANSI(m.renderCard())
		if !strings.Contains(card, "installed: v1.0.0") {
			t.Errorf("card missing installed line; got:\n%s", card)
		}
		if strings.Contains(card, "latest:") {
			t.Errorf("card shows latest line with no release; got:\n%s", card)
		}
		if strings.Contains(card, "") {
			t.Errorf("card shows the tag icon with no release; got:\n%s", card)
		}
	})

	t.Run("no github with installed: info section opens", func(t *testing.T) {
		m := newCardModel("")
		m.versions["gh"] = VersionInfo{Installed: "v1.0.0", InstalledKnown: true}
		card := stripANSI(m.renderCard())
		if !strings.Contains(card, "[info]") || !strings.Contains(card, "installed: v1.0.0") {
			t.Errorf("card missing [info]/installed for GitHub-less tool; got:\n%s", card)
		}
	})

	t.Run("no github no installed: no info section", func(t *testing.T) {
		m := newCardModel("")
		card := stripANSI(m.renderCard())
		if strings.Contains(card, "[info]") || strings.Contains(card, "installed:") {
			t.Errorf("card renders [info] with nothing to show; got:\n%s", card)
		}
	})
}

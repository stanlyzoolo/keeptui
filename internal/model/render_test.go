package model

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/lepeshko/keys/internal/loader"
	"github.com/lepeshko/keys/internal/version"
)

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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanTerminalOutput(tt.in); got != tt.want {
				t.Errorf("cleanTerminalOutput(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestColorizeHelp(t *testing.T) {
	// Force truecolor so styled tokens actually emit ANSI escapes; otherwise
	// lipgloss strips color under a non-TTY test run and hides the bug.
	lipgloss.SetColorProfile(termenv.TrueColor)

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

func TestRenderStatusBarRateSignal(t *testing.T) {
	t.Run("unknown renders no signal", func(t *testing.T) {
		m := Model{width: 80, focus: focusTools}
		m.rate = version.RateLimit{Known: false, Remaining: 0, Limit: 60}
		got := m.renderStatusBar()
		for _, absent := range []string{"GH", "⚠", "✕"} {
			if strings.Contains(got, absent) {
				t.Errorf("unknown rate status bar = %q, should not contain %q", got, absent)
			}
		}
	})

	t.Run("normal renders quiet count", func(t *testing.T) {
		m := Model{width: 80, focus: focusTools}
		m.rate = version.RateLimit{Known: true, Remaining: 4800, Limit: 5000}
		got := m.renderStatusBar()
		if !strings.Contains(got, "GH 4800/5000") {
			t.Errorf("normal rate status bar = %q, missing count", got)
		}
		for _, absent := range []string{"⚠", "✕"} {
			if strings.Contains(got, absent) {
				t.Errorf("normal rate status bar = %q, should not contain %q", got, absent)
			}
		}
	})

	t.Run("warning icon at threshold", func(t *testing.T) {
		m := Model{width: 80, focus: focusTools}
		m.rate = version.RateLimit{Known: true, Remaining: rateLowThreshold, Limit: 60}
		got := m.renderStatusBar()
		if !strings.Contains(got, "⚠") {
			t.Errorf("warning rate status bar = %q, missing warn icon", got)
		}
		if !strings.Contains(got, "[L]") {
			t.Errorf("warning rate status bar = %q, missing [L] hint", got)
		}
	})

	t.Run("danger icon at zero", func(t *testing.T) {
		m := Model{width: 80, focus: focusTools}
		m.rate = version.RateLimit{Known: true, Remaining: 0, Limit: 60}
		got := m.renderStatusBar()
		if !strings.Contains(got, "✕") {
			t.Errorf("danger rate status bar = %q, missing danger icon", got)
		}
		if !strings.Contains(got, "[L]") {
			t.Errorf("danger rate status bar = %q, missing [L] hint", got)
		}
	})
}

func TestRenderStatusBarTracking(t *testing.T) {
	m := Model{width: 80, tracking: true, trackInput: textinput.New()}
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
	m := Model{width: 80, confirmingUntrack: true, untrackTarget: "git"}
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
			metaSelected:      1,
			confirmingUntrack: true,
			untrackTarget:     "b",
		}
		m.tools = loader.ToolsFromMeta(m.meta)

		updated, _ := m.updateUntrackConfirm(enter)
		nm := updated.(Model)

		if nm.confirmingUntrack {
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
			meta:              []loader.ToolMeta{{Name: "a"}, {Name: "b"}},
			metaSelected:      1,
			confirmingUntrack: true,
			untrackTarget:     "b",
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
			meta:              []loader.ToolMeta{{Name: "a"}, {Name: "b"}},
			metaSelected:      0,
			confirmingUntrack: true,
			untrackTarget:     "a",
		}
		m.tools = loader.ToolsFromMeta(m.meta)

		updated, _ := m.updateUntrackConfirm(esc)
		nm := updated.(Model)

		if nm.confirmingUntrack {
			t.Errorf("confirmingUntrack should be false after esc")
		}
		if len(nm.meta) != 2 || loader.FindMeta(nm.meta, "a") == nil {
			t.Errorf("list should be unchanged after esc, got %v", nm.meta)
		}
	})
}

func TestRenderStatusBarRenaming(t *testing.T) {
	m := Model{width: 80, renaming: true, nameInput: textinput.New()}
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
			loader.StatusForgotten,
			loader.StatusArchived,
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
			versions: map[string]VersionInfo{"git": {Installed: "1.0"}},
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
		versions:      map[string]VersionInfo{name: {Installed: "1.0", Latest: "2.0"}},
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
		renaming:      true,
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

	m := Model{width: 80, height: 24, showingAPIStatus: true}
	m.rate = version.RateLimit{Known: true, Remaining: 0, Limit: 60}
	got := m.renderAPIStatus()

	for _, want := range []string{"GitHub API status", "env", "ghp_••••••••3f2a", "0 / 60", "✕", "[e]", "[r]", "[esc]"} {
		if !strings.Contains(got, want) {
			t.Errorf("overlay = %q, missing %q", got, want)
		}
	}
	// [d] remove token is hidden for the env source.
	if strings.Contains(got, "remove token") {
		t.Errorf("overlay = %q, should not offer remove token for env source", got)
	}
}

func TestRenderAPIStatusWarnIcon(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")

	m := Model{width: 80, height: 24, showingAPIStatus: true}
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
	if !nm.showingAPIStatus {
		t.Fatalf("pressing L did not open the API-status overlay")
	}
	if cmd == nil {
		t.Errorf("pressing L should fire a rate fetch cmd")
	}
	// esc closes it.
	updated2, _ := nm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated2.(Model).showingAPIStatus {
		t.Errorf("esc did not close the API-status overlay")
	}
}

// TestUpdateAPIStatusOpensTokenEntry verifies [e] switches the overlay into the
// masked token-input sub-mode.
func TestUpdateAPIStatusOpensTokenEntry(t *testing.T) {
	m := Model{width: 80, height: 24, showingAPIStatus: true, tokenInput: textinput.New()}
	updated, cmd := m.updateAPIStatus(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	nm := updated.(Model)
	if !nm.enteringToken {
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

	m := Model{width: 80, height: 24, showingAPIStatus: true, enteringToken: true, tokenInput: textinput.New()}
	updated, _ := m.Update(tokenValidatedMsg{token: "ghp_bad", err: version.ErrTokenInvalid})
	nm := updated.(Model)
	if nm.tokenError != "token invalid" {
		t.Errorf("tokenError = %q, want %q", nm.tokenError, "token invalid")
	}
	if !nm.enteringToken {
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
	version.ClearToken() //nolint:errcheck
	t.Cleanup(func() { version.ClearToken() }) //nolint:errcheck

	name := "git"
	m := Model{
		width: 80, height: 24, showingAPIStatus: true, enteringToken: true,
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
	if nm.enteringToken {
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

	m := Model{width: 80, height: 24, showingAPIStatus: true, tokenInput: textinput.New()}
	m.updateAPIStatus(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if src := version.TokenSource(); src != "none" {
		t.Errorf("[d] did not clear the token, TokenSource() = %q", src)
	}
}

// TestRenderStatusBarTokenInput verifies the status bar reflects the token-input
// sub-mode.
func TestRenderStatusBarTokenInput(t *testing.T) {
	m := Model{width: 80, height: 24, showingAPIStatus: true, enteringToken: true, tokenInput: textinput.New()}
	got := m.renderStatusBar()
	if !strings.Contains(got, "validate & save") {
		t.Errorf("status bar = %q, missing token-input hint", got)
	}
}

// TestRenderAPIStatusTokenEntry verifies the overlay shows the masked input and
// the inline error while entering a token.
func TestRenderAPIStatusTokenEntry(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	m := Model{width: 80, height: 24, showingAPIStatus: true, enteringToken: true, tokenInput: textinput.New()}
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

		if !nm.renaming {
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

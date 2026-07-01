package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

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

func TestFetchVersionCmd(t *testing.T) {
	// GitHub "" keeps GetLatest/GetCachedRepoStatus offline and a nonexistent
	// name makes InstalledVersion skip exec, so the closure runs with no I/O.
	cmd := fetchVersionCmd(loader.Tool{Name: "nonexistent-tool-xyz", GitHub: ""})
	if cmd == nil {
		t.Fatal("expected non-nil tea.Cmd from fetchVersionCmd")
	}
	msg, ok := cmd().(versionMsg)
	if !ok {
		t.Fatalf("expected versionMsg, got %T", cmd())
	}
	if msg.toolName != "nonexistent-tool-xyz" {
		t.Errorf("toolName = %q, want %q", msg.toolName, "nonexistent-tool-xyz")
	}
}

func TestNeedsVersion(t *testing.T) {
	tests := []struct {
		name     string
		tool     loader.Tool
		versions map[string]VersionInfo
		want     bool
	}{
		{
			name: "fresh tool needs version",
			tool: loader.Tool{Name: "git", GitHub: "cli/cli"},
			want: true,
		},
		{
			name:     "cached tool does not need version",
			tool:     loader.Tool{Name: "git", GitHub: "cli/cli"},
			versions: map[string]VersionInfo{"git": {}},
			want:     false,
		},
		{
			name: "version fires even without GitHub",
			tool: loader.Tool{Name: "git", GitHub: ""},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Model{versions: tt.versions}
			if got := m.needsVersion(tt.tool); got != tt.want {
				t.Errorf("needsVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNeedsRepoCard(t *testing.T) {
	tests := []struct {
		name      string
		tool      loader.Tool
		repoCards map[string]version.RepoCard
		want      bool
	}{
		{
			name: "fresh tool with GitHub needs repo card",
			tool: loader.Tool{Name: "git", GitHub: "cli/cli"},
			want: true,
		},
		{
			name:      "cached tool does not need repo card",
			tool:      loader.Tool{Name: "git", GitHub: "cli/cli"},
			repoCards: map[string]version.RepoCard{"git": {}},
			want:      false,
		},
		{
			name: "repo card not needed without GitHub",
			tool: loader.Tool{Name: "git", GitHub: ""},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Model{repoCards: tt.repoCards}
			if got := m.needsRepoCard(tt.tool); got != tt.want {
				t.Errorf("needsRepoCard() = %v, want %v", got, tt.want)
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
		versions:      map[string]VersionInfo{name: {}},
		repoCards:     map[string]version.RepoCard{name: {}},
	}
	if m.needsVersion(m.tools[0]) {
		t.Error("needsVersion should be false when version is cached")
	}
	if m.needsRepoCard(m.tools[0]) {
		t.Error("needsRepoCard should be false when repo card is cached")
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

// TestUpdateVersionAndRepoCardMsgPopulateCaches closes the loop the rename test
// opens: after stale keys are cleared, the async fetch results must repopulate
// the caches under the (new) tool name. This drives the messages through
// Update() to prove the toolName keying is correct end to end.
func TestUpdateVersionAndRepoCardMsgPopulateCaches(t *testing.T) {
	m := Model{
		meta:          []loader.ToolMeta{{Name: "gh", GitHub: "cli/cli"}},
		metaSelected:  0,
		versions:      map[string]VersionInfo{},
		repoStatus:    map[string]string{},
		repoCards:     map[string]version.RepoCard{},
		changelogData: map[string]changelogMsg{},
	}
	m.tools = loader.ToolsFromMeta(m.meta)

	updated, _ := m.Update(versionMsg{toolName: "gh", installed: "1.0", latest: "2.0", repoStatus: "active"})
	nm := updated.(Model)
	if got, ok := nm.versions["gh"]; !ok || got.Installed != "1.0" || got.Latest != "2.0" {
		t.Errorf("versions[gh] = %+v (ok=%v), want {Installed:1.0 Latest:2.0}", got, ok)
	}
	if got := nm.repoStatus["gh"]; got != "active" {
		t.Errorf("repoStatus[gh] = %q, want %q", got, "active")
	}

	updated2, _ := nm.Update(repoCardMsg{toolName: "gh", card: version.RepoCard{}})
	nm2 := updated2.(Model)
	if _, ok := nm2.repoCards["gh"]; !ok {
		t.Errorf("repoCards not populated under name 'gh' after repoCardMsg")
	}
}

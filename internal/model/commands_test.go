package model

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lepeshko/keys/internal/loader"
	"github.com/lepeshko/keys/internal/logx"
	"github.com/lepeshko/keys/internal/updater"
)

func TestFetchHelpTakeoverLogs(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "faketui")
	script := "#!/bin/sh\nprintf '\\033[?1049lpanic: no tty\\n'\nexit 101\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	logDir := t.TempDir()
	restore := logx.SetDirForTesting(logDir)
	defer restore()

	msg, ok := fetchHelpCmd("faketui", helpModeHelp)().(helpOutputMsg)
	if !ok {
		t.Fatalf("unexpected msg type %T", msg)
	}
	if msg.output != "" {
		t.Errorf("takeover output leaked into help: %q", msg.output)
	}
	out := logx.ReadAllForTesting(logDir)
	if !strings.Contains(out, "faketui") || !strings.Contains(out, "TUI takeover") {
		t.Errorf("log should record the takeover and tool name, got:\n%s", out)
	}
}

func TestFetchHelpSuccessNoLog(t *testing.T) {
	dir := t.TempDir()
	sane := filepath.Join(dir, "fakecli")
	if err := os.WriteFile(sane, []byte("#!/bin/sh\necho 'Usage: fakecli'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	logDir := t.TempDir()
	restore := logx.SetDirForTesting(logDir)
	defer restore()

	msg := fetchHelpCmd("fakecli", helpModeHelp)().(helpOutputMsg)
	if msg.output != "Usage: fakecli\n" {
		t.Fatalf("plain help lost: %q, err %v", msg.output, msg.err)
	}
	if out := logx.ReadAllForTesting(logDir); out != "" {
		t.Errorf("a successful help capture must not log, got:\n%s", out)
	}
}

type safeCmdMsg struct{ v int }

func TestSafeCmdPassesMsgThrough(t *testing.T) {
	cmd := safeCmd("ctx", func() tea.Msg { return safeCmdMsg{v: 7} })
	msg, ok := cmd().(safeCmdMsg)
	if !ok {
		t.Fatalf("unexpected msg type %T", msg)
	}
	if msg.v != 7 {
		t.Errorf("msg not passed through untouched: %+v", msg)
	}
}

func safeCmdPanicSite() tea.Msg {
	panic("cmd boom")
}

func TestSafeCmdLogsAndRePanics(t *testing.T) {
	logDir := t.TempDir()
	restore := logx.SetDirForTesting(logDir)
	defer restore()

	cmd := safeCmd("panic.ctx", safeCmdPanicSite)

	var caught any
	func() {
		defer func() { caught = recover() }()
		cmd()
	}()

	if caught != "cmd boom" {
		t.Errorf("expected re-panic with %q, caught %v", "cmd boom", caught)
	}
	out := logx.ReadAllForTesting(logDir)
	if !strings.Contains(out, "panic in panic.ctx") {
		t.Errorf("log missing context, got:\n%s", out)
	}
	if !strings.Contains(out, "safeCmdPanicSite") {
		t.Errorf("trace should name the real panic site, got:\n%s", out)
	}
}

func TestIsTUITakeover(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"plain help text", "Usage: tool [OPTIONS]", false},
		{"SGR-colored help", "\x1b[1mUsage:\x1b[0m tool", false},
		{"leave alt screen (inertia panic prefix)", "\x1b[?1049l\nthread 'main' panicked at …", true},
		{"enter alt screen (TUI ran until the timeout)", "\x1b[?1049h\x1b[2J\x1b[?25l frames…", true},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTUITakeover([]byte(tt.in)); got != tt.want {
				t.Errorf("isTUITakeover(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestFetchHelpCmdRejectsTUITakeover drives fetchHelpCmd against a fake tool
// that answers every help flag the way inertia does — alt-screen escape plus
// a crash trace — and expects the empty-output error path (which the
// helpOutputMsg handler turns into "No --help output for <name>…"), not the
// captured garbage.
func TestFetchHelpCmdRejectsTUITakeover(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake tool")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "faketui")
	script := "#!/bin/sh\nprintf '\\033[?1049lpanic: no tty\\n'\nexit 101\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	msg, ok := fetchHelpCmd("faketui", helpModeHelp)().(helpOutputMsg)
	if !ok {
		t.Fatalf("unexpected msg type %T", msg)
	}
	if msg.output != "" {
		t.Errorf("takeover output leaked into help: %q", msg.output)
	}

	// Control: a fake tool with normal help output still comes through.
	sane := filepath.Join(dir, "fakecli")
	if err := os.WriteFile(sane, []byte("#!/bin/sh\necho 'Usage: fakecli'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	msg = fetchHelpCmd("fakecli", helpModeHelp)().(helpOutputMsg)
	if msg.output != "Usage: fakecli\n" {
		t.Errorf("plain help lost: %q, err %v", msg.output, msg.err)
	}
}

// updateDoneModel builds a fully-initialized two-tool model with rg's installed
// version older than latest (so hasUpdate(rg) is true and rg sorts to the top of
// the update-grouped list). rg is mid-update: updatingFor/updateLogFor set.
func updateDoneModel(t *testing.T) Model {
	t.Helper()
	m := New([]loader.ToolMeta{
		{Name: "rg", GitHub: "github.com/BurntSushi/ripgrep"},
		{Name: "fzf"},
	})
	m = mustModel(m.Update(tea.WindowSizeMsg{Width: 120, Height: 24}))
	m.versions["rg"] = VersionInfo{Installed: "1.0.0", Latest: "2.0.0", InstalledKnown: true}
	m.updatePlan = updater.Plan{Manager: "brew", Argv: []string{"brew", "upgrade", "ripgrep"}, Display: "brew upgrade ripgrep"}
	m.updatingFor = "rg"
	m.updateLogFor = "rg"
	m.updateLog = []string{"==> Upgrading ripgrep", "==> Pouring ripgrep"}
	m.metaSelected = m.indexOfMeta("rg")
	return m
}

// TestUpdateDoneSuccess: a successful updateDoneMsg clears updatingFor, sets the
// "updated <name>" status and returns a command that re-detects the installed
// version (installedMsg for the same tool).
func TestUpdateDoneSuccess(t *testing.T) {
	m := updateDoneModel(t)
	nm, cmd := m.Update(updateDoneMsg{tool: "rg", err: nil})
	m2 := nm.(Model)
	if m2.updatingFor != "" {
		t.Errorf("updatingFor = %q, want empty after done", m2.updatingFor)
	}
	if m2.statusMsg != "updated rg" {
		t.Errorf("statusMsg = %q, want %q", m2.statusMsg, "updated rg")
	}
	if cmd == nil {
		t.Fatal("want a re-fetch command, got nil")
	}
	msg, ok := cmd().(installedMsg)
	if !ok || msg.toolName != "rg" {
		t.Errorf("cmd produced %T (%+v), want installedMsg for rg", msg, msg)
	}
}

// TestUpdateDoneFailure: a failed updateDoneMsg clears updatingFor, sets the
// "see [3]" status, writes a log line (manager + tool, never a token) and does
// not re-fetch.
func TestUpdateDoneFailure(t *testing.T) {
	logDir := t.TempDir()
	restore := logx.SetDirForTesting(logDir)
	defer restore()

	m := updateDoneModel(t)
	nm, cmd := m.Update(updateDoneMsg{tool: "rg", err: errUpdateTest})
	m2 := nm.(Model)
	if m2.updatingFor != "" {
		t.Errorf("updatingFor = %q, want empty after failure", m2.updatingFor)
	}
	if m2.statusMsg != "update failed — see [3]" {
		t.Errorf("statusMsg = %q, want the see-[3] hint", m2.statusMsg)
	}
	if cmd != nil {
		t.Error("failure must not re-fetch the installed version")
	}
	out := logx.ReadAllForTesting(logDir)
	if !strings.Contains(out, "rg") || !strings.Contains(out, "brew") {
		t.Errorf("log = %q, want tool and manager recorded", out)
	}
	if strings.Contains(out, "token") {
		t.Errorf("log leaked a token-ish word: %q", out)
	}
}

// TestUpdateDoneUntracked: a done for a tool no longer tracked (untracked
// mid-update) just clears the guard — no re-fetch, no crash, no status.
func TestUpdateDoneUntracked(t *testing.T) {
	m := updateDoneModel(t)
	m.updatingFor = "gone"
	nm, cmd := m.Update(updateDoneMsg{tool: "gone", err: nil})
	m2 := nm.(Model)
	if m2.updatingFor != "" {
		t.Errorf("updatingFor = %q, want cleared", m2.updatingFor)
	}
	if cmd != nil {
		t.Error("untracked done must not re-fetch")
	}
	if m2.statusMsg != "" {
		t.Errorf("statusMsg = %q, want empty for an untracked tool", m2.statusMsg)
	}
}

// TestUpdateDoneCursorFollowsTool: after the success re-fetch merges an
// up-to-date installed version, hasUpdate flips off and rg leaves the update
// group — but the selection still points at rg by name, not at a stale index.
func TestUpdateDoneCursorFollowsTool(t *testing.T) {
	m := updateDoneModel(t)
	// rg has an update → sorts to the top; selection is on rg (index 0).
	if got, _ := m.selectedMeta(); got.Name != "rg" {
		t.Fatalf("precondition: selected = %q, want rg", got.Name)
	}
	nm, cmd := m.Update(updateDoneMsg{tool: "rg", err: nil})
	m2 := nm.(Model)
	// Run the re-fetch to get the installedMsg, then feed it the now-current
	// installed version — this flips hasUpdate off and reorders the list.
	if cmd == nil {
		t.Fatal("want a re-fetch command")
	}
	m3 := mustModel(m2.Update(installedMsg{toolName: "rg", installed: "2.0.0"}))
	if m3.hasUpdate("rg") {
		t.Fatal("hasUpdate(rg) still true after installing latest")
	}
	if got, _ := m3.selectedMeta(); got.Name != "rg" {
		t.Errorf("selected = %q after regroup, want the cursor to follow rg", got.Name)
	}
}

var errUpdateTest = updateTestError("exit status 1")

type updateTestError string

func (e updateTestError) Error() string { return string(e) }

package model

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lepeshko/keys/internal/logx"
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

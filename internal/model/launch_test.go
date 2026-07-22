package model

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/stanlyzoolo/keeptui/internal/launcher"
	"github.com/stanlyzoolo/keeptui/internal/logx"
)

// clearTerminalEnv blanks every variable launcher.Detect inspects, so the
// detection chain lands on the fallback plan regardless of the terminal the
// tests actually run in. planFor is unexported and cross-package — env is the
// seam.
func clearTerminalEnv(t *testing.T) {
	t.Helper()
	t.Setenv("TMUX", "")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("KITTY_WINDOW_ID", "")
}

// enterRun drives updateRunInput's enter with value as the typed command and
// returns the resulting model and dispatched cmd.
func enterRun(m Model, value string) (Model, tea.Cmd) {
	m.mode = modeRunInput
	m.runInput.SetValue(value)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return updated.(Model), cmd
}

// TestShellCommand pins both branches of the goos-parameterized shell seam:
// unix runs the command through sh -c, Windows through cmd /c.
func TestShellCommand(t *testing.T) {
	tests := []struct {
		goos     string
		wantName string
		wantArgs []string
	}{
		{"linux", "sh", []string{"-c", "git status"}},
		{"darwin", "sh", []string{"-c", "git status"}},
		{"windows", "cmd", []string{"/c", "git status"}},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			name, args := shellCommand(tt.goos, "git status")
			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
			if len(args) != len(tt.wantArgs) {
				t.Fatalf("args = %v, want %v", args, tt.wantArgs)
			}
			for i := range args {
				if args[i] != tt.wantArgs[i] {
					t.Errorf("args[%d] = %q, want %q", i, args[i], tt.wantArgs[i])
				}
			}
		})
	}
}

// TestStartLaunchCmd runs the adapter command constructor against harmless
// argvs: a succeeding binary yields a nil-err launchDoneMsg carrying the tool
// and command, a missing binary a non-nil err, and an empty argv (which a
// fallback plan would have — dispatch must never route one here) errors
// instead of panicking on Argv[0].
func TestStartLaunchCmd(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		msg, ok := startLaunchCmd(launcher.Plan{Argv: []string{"true"}}, "git", "git status")().(launchDoneMsg)
		if !ok {
			t.Fatalf("unexpected msg type %T", msg)
		}
		if msg.err != nil {
			t.Errorf("err = %v, want nil", msg.err)
		}
		if msg.toolName != "git" || msg.command != "git status" {
			t.Errorf("msg = %+v, want toolName git and command carried", msg)
		}
	})
	t.Run("missing adapter binary", func(t *testing.T) {
		msg, ok := startLaunchCmd(launcher.Plan{Argv: []string{"keeptui-no-such-adapter-xyz"}}, "git", "git status")().(launchDoneMsg)
		if !ok {
			t.Fatalf("unexpected msg type %T", msg)
		}
		if msg.err == nil {
			t.Error("err = nil, want the exec failure")
		}
	})
	t.Run("empty argv", func(t *testing.T) {
		msg, ok := startLaunchCmd(launcher.Plan{}, "git", "git status")().(launchDoneMsg)
		if !ok {
			t.Fatalf("unexpected msg type %T", msg)
		}
		if msg.err == nil {
			t.Error("err = nil, want an empty-command error")
		}
	})
}

// TestRunInputDispatchFallback: with no detectable terminal (env cleared) the
// enter dispatch takes the ExecProcess path — the returned cmd yields Bubble
// Tea's internal exec message, not a launchDoneMsg — and records lastRun.
func TestRunInputDispatchFallback(t *testing.T) {
	clearTerminalEnv(t)
	m := newTestModel(focusTools)
	nm, cmd := enterRun(m, "git status")
	if nm.mode != modeNormal {
		t.Errorf("mode = %d, want modeNormal", nm.mode)
	}
	if got := nm.lastRun["git"]; got != "git status" {
		t.Errorf("lastRun[git] = %q, want %q", got, "git status")
	}
	if cmd == nil {
		t.Fatal("cmd = nil, want the ExecProcess dispatch")
	}
	msg := cmd()
	if _, isLaunch := msg.(launchDoneMsg); isLaunch {
		t.Fatalf("fallback path dispatched the adapter cmd: %+v", msg)
	}
	// Pin the exec path via Bubble Tea's internal message type name; invoking
	// the cmd only builds the message — the process runs later, when the
	// runtime handles it — so nothing is spawned here.
	if got := fmt.Sprintf("%T", msg); got != "tea.execMsg" {
		t.Errorf("msg type = %s, want tea.execMsg", got)
	}
}

// TestRunInputDispatchAdapter: with $TMUX set (pointing at a bogus socket, so
// invoking the cmd cannot touch a real tmux server) the enter dispatch takes
// the adapter path — the returned cmd emits launchDoneMsg carrying the tool
// name and command for the auto-fallback.
func TestRunInputDispatchAdapter(t *testing.T) {
	clearTerminalEnv(t)
	t.Setenv("TMUX", "/nonexistent/keeptui-test-socket,99999,0")
	m := newTestModel(focusTools)
	nm, cmd := enterRun(m, "git status")
	if got := nm.lastRun["git"]; got != "git status" {
		t.Errorf("lastRun[git] = %q, want %q", got, "git status")
	}
	if cmd == nil {
		t.Fatal("cmd = nil, want the adapter dispatch")
	}
	msg, ok := cmd().(launchDoneMsg)
	if !ok {
		t.Fatalf("unexpected msg type %T, want launchDoneMsg", msg)
	}
	if msg.toolName != "git" || msg.command != "git status" {
		t.Errorf("msg = %+v, want toolName and command carried for the fallback", msg)
	}
	// err is not asserted: tmux may be missing entirely or fail to connect to
	// the bogus socket — either way the adapter path emitted its message.
}

// TestLaunchDoneMsgFallback: an adapter failure sets an explanatory statusMsg
// and auto-falls back to the ExecProcess path, so the tool still launches.
// Nothing is logged — the fallback makes this degraded, not broken.
func TestLaunchDoneMsgFallback(t *testing.T) {
	logDir := t.TempDir()
	restore := logx.SetDirForTesting(logDir)
	defer restore()

	m := newTestModel(focusTools)
	updated, cmd := m.Update(launchDoneMsg{toolName: "git", command: "git status", err: errors.New("osascript: not authorized")})
	nm := updated.(Model)
	if nm.statusMsg == "" || !strings.Contains(nm.statusMsg, "git") {
		t.Errorf("statusMsg = %q, want a tab-failure explanation naming the tool", nm.statusMsg)
	}
	if cmd == nil {
		t.Fatal("cmd = nil, want the ExecProcess auto-fallback")
	}
	msg := cmd()
	if _, isLaunch := msg.(launchDoneMsg); isLaunch {
		t.Fatalf("fallback re-dispatched the adapter: %+v", msg)
	}
	if got := fmt.Sprintf("%T", msg); got != "tea.execMsg" {
		t.Errorf("msg type = %s, want tea.execMsg", got)
	}
	if out := logx.ReadAllForTesting(logDir); out != "" {
		t.Errorf("adapter failure must not log (auto-fallback handles it), got:\n%s", out)
	}
}

// TestLaunchDoneMsgSuccess: a clean adapter run reports "launched <name>"
// (mode-neutral — Terminal.app and tmux open a window, not a tab) and
// dispatches nothing further.
func TestLaunchDoneMsgSuccess(t *testing.T) {
	m := newTestModel(focusTools)
	updated, cmd := m.Update(launchDoneMsg{toolName: "git", command: "git status"})
	nm := updated.(Model)
	if nm.statusMsg != "launched git" {
		t.Errorf("statusMsg = %q, want %q", nm.statusMsg, "launched git")
	}
	if cmd != nil {
		t.Error("cmd != nil, want no follow-up on success")
	}
}

// TestExecDoneMsg: the ExecProcess callback result. A non-zero exit surfaces
// as a statusMsg and is never logged (the tool's exit status is not a keeptui
// anomaly); a clean exit is silent.
func TestExecDoneMsg(t *testing.T) {
	t.Run("non-zero exit", func(t *testing.T) {
		logDir := t.TempDir()
		restore := logx.SetDirForTesting(logDir)
		defer restore()

		m := newTestModel(focusTools)
		updated, cmd := m.Update(execDoneMsg{toolName: "git", err: errors.New("exit status 1")})
		nm := updated.(Model)
		if want := "git exited: exit status 1"; nm.statusMsg != want {
			t.Errorf("statusMsg = %q, want %q", nm.statusMsg, want)
		}
		if cmd != nil {
			t.Error("cmd != nil, want none")
		}
		if out := logx.ReadAllForTesting(logDir); out != "" {
			t.Errorf("a tool's non-zero exit must not log, got:\n%s", out)
		}
	})
	t.Run("clean exit", func(t *testing.T) {
		m := newTestModel(focusTools)
		updated, cmd := m.Update(execDoneMsg{toolName: "git"})
		nm := updated.(Model)
		if nm.statusMsg != "" {
			t.Errorf("statusMsg = %q, want silence on a clean exit", nm.statusMsg)
		}
		if cmd != nil {
			t.Error("cmd != nil, want none")
		}
	})
}

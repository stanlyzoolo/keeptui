// Package launcher decides how to run a tracked tool in a new terminal tab.
// It sits at the bottom of the import graph like internal/updater: no TUI
// knowledge, a pure core (planFor over an injected env lookup) plus a thin
// os.Getenv-facing wrapper (Detect).
package launcher

import (
	"fmt"
	"os"
	"strings"
)

// Plan describes how to open the user's command in a new terminal tab.
// When no supported terminal is detected, Fallback is true and Argv is empty —
// the caller runs the command in the current window via tea.ExecProcess.
type Plan struct {
	Argv     []string // adapter command, executed directly (not through a shell)
	Fallback bool     // no scripting API available; run in the current window
	Terminal string   // human-readable adapter name ("tmux", "iTerm2", …)
}

// planFor is the pure detection core. The priority chain is deliberate:
// $TMUX first, because inside tmux TERM_PROGRAM names the *outer* terminal and
// a tmux window is the correct "tab" there; then TERM_PROGRAM/KITTY_WINDOW_ID
// checks; anything else falls back.
//
// The user command always executes as `sh -c <cmd>` (tmux runs the string via
// the user's shell itself). For tmux/kitty/wezterm the command and tool name
// travel as argv elements — no escaping. For the two AppleScript paths the
// command is interpolated into the script source, with appleScriptQuote as the
// single escaping point.
func planFor(env func(string) string, command, toolName string) Plan {
	switch {
	case env("TMUX") != "":
		return Plan{
			Terminal: "tmux",
			Argv:     []string{"tmux", "new-window", "-n", toolName, command},
		}
	case env("TERM_PROGRAM") == "iTerm.app":
		script := fmt.Sprintf(`tell application "iTerm2"
	tell current window
		set newTab to (create tab with default profile)
		tell current session of newTab
			set name to "%s"
			write text "%s"
		end tell
	end tell
end tell`, appleScriptQuote(toolName), appleScriptQuote(command))
		return Plan{
			Terminal: "iTerm2",
			Argv:     []string{"osascript", "-e", script},
		}
	case env("TERM_PROGRAM") == "Apple_Terminal":
		// Terminal.app opens a *window*, not a tab: tabs are not scriptable
		// without System Events. Honest degradation, documented in the plan.
		script := fmt.Sprintf(`tell application "Terminal" to do script "%s"`, appleScriptQuote(command))
		return Plan{
			Terminal: "Terminal.app",
			Argv:     []string{"osascript", "-e", script},
		}
	case env("KITTY_WINDOW_ID") != "":
		return Plan{
			Terminal: "kitty",
			Argv:     []string{"kitten", "@", "launch", "--type=tab", "--tab-title", toolName, "sh", "-c", command},
		}
	case env("TERM_PROGRAM") == "WezTerm":
		// Tab title left to wezterm defaults; naming needs a second pane-id
		// round-trip — deliberately skipped.
		return Plan{
			Terminal: "WezTerm",
			Argv:     []string{"wezterm", "cli", "spawn", "--", "sh", "-c", command},
		}
	default:
		return Plan{Fallback: true}
	}
}

// appleScriptQuote escapes a string for interpolation inside a double-quoted
// AppleScript string literal: backslashes first, then double quotes.
func appleScriptQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}

// Detect resolves the launch Plan for the current environment. Env-only — no
// subprocesses — so it is safe to call inside Bubble Tea's Update.
func Detect(command, toolName string) Plan {
	return planFor(os.Getenv, command, toolName)
}

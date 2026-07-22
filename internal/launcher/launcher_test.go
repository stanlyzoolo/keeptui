package launcher

import (
	"reflect"
	"strings"
	"testing"
)

// envFrom builds an env lookup over a fixed map; missing keys return "".
func envFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestPlanFor(t *testing.T) {
	tests := []struct {
		name         string
		env          map[string]string
		command      string
		toolName     string
		wantTerminal string
		wantFallback bool
		wantArgv     []string
	}{
		{
			name:         "tmux",
			env:          map[string]string{"TMUX": "/tmp/tmux-501/default,1234,0"},
			command:      "yazi",
			toolName:     "yazi",
			wantTerminal: "tmux",
			wantArgv:     []string{"tmux", "new-window", "-n", "yazi", "--", "yazi"},
		},
		{
			name: "tmux wins over TERM_PROGRAM",
			env: map[string]string{
				"TMUX":         "/tmp/tmux-501/default,1234,0",
				"TERM_PROGRAM": "iTerm.app",
			},
			command:      "fzf",
			toolName:     "fzf",
			wantTerminal: "tmux",
			wantArgv:     []string{"tmux", "new-window", "-n", "fzf", "--", "fzf"},
		},
		{
			name:         "kitty",
			env:          map[string]string{"KITTY_WINDOW_ID": "3"},
			command:      "dive nginx:latest",
			toolName:     "dive",
			wantTerminal: "kitty",
			wantArgv:     []string{"kitten", "@", "launch", "--type=tab", "--tab-title", "dive", "sh", "-c", "dive nginx:latest"},
		},
		{
			name:         "wezterm",
			env:          map[string]string{"TERM_PROGRAM": "WezTerm"},
			command:      "btop",
			toolName:     "btop",
			wantTerminal: "WezTerm",
			wantArgv:     []string{"wezterm", "cli", "spawn", "--", "sh", "-c", "btop"},
		},
		{
			name:         "empty env falls back",
			env:          map[string]string{},
			command:      "yazi",
			toolName:     "yazi",
			wantFallback: true,
		},
		{
			name:         "unknown TERM_PROGRAM falls back",
			env:          map[string]string{"TERM_PROGRAM": "ghostty"},
			command:      "yazi",
			toolName:     "yazi",
			wantFallback: true,
		},
		{
			name:         "tool name with spaces stays one argv element (tmux)",
			env:          map[string]string{"TMUX": "x"},
			command:      "docker run -it alpine",
			toolName:     "my tool",
			wantTerminal: "tmux",
			wantArgv:     []string{"tmux", "new-window", "-n", "my tool", "--", "docker run -it alpine"},
		},
		{
			name:         "dash-leading command survives tmux option parsing",
			env:          map[string]string{"TMUX": "x"},
			command:      "-la",
			toolName:     "ls",
			wantTerminal: "tmux",
			wantArgv:     []string{"tmux", "new-window", "-n", "ls", "--", "-la"},
		},
		{
			name:         "unicode tool name stays intact (kitty)",
			env:          map[string]string{"KITTY_WINDOW_ID": "1"},
			command:      "ls",
			toolName:     "инструмент",
			wantTerminal: "kitty",
			wantArgv:     []string{"kitten", "@", "launch", "--type=tab", "--tab-title", "инструмент", "sh", "-c", "ls"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := planFor(envFrom(tc.env), tc.command, tc.toolName)
			if got.Fallback != tc.wantFallback {
				t.Fatalf("Fallback = %v, want %v", got.Fallback, tc.wantFallback)
			}
			if got.Terminal != tc.wantTerminal {
				t.Errorf("Terminal = %q, want %q", got.Terminal, tc.wantTerminal)
			}
			if tc.wantFallback {
				if len(got.Argv) != 0 {
					t.Errorf("fallback plan carries Argv %v, want empty", got.Argv)
				}
				return
			}
			if !reflect.DeepEqual(got.Argv, tc.wantArgv) {
				t.Errorf("Argv = %#v, want %#v", got.Argv, tc.wantArgv)
			}
		})
	}
}

func TestPlanForITerm(t *testing.T) {
	got := planFor(envFrom(map[string]string{"TERM_PROGRAM": "iTerm.app"}), `echo "hi"`, "echo tool")
	if got.Terminal != "iTerm2" || got.Fallback {
		t.Fatalf("plan = %+v, want iTerm2 non-fallback", got)
	}
	if len(got.Argv) != 3 || got.Argv[0] != "osascript" || got.Argv[1] != "-e" {
		t.Fatalf("Argv = %#v, want [osascript -e <script>]", got.Argv)
	}
	script := got.Argv[2]
	if !strings.Contains(script, `tell application "iTerm2"`) {
		t.Errorf("script missing iTerm2 tell block:\n%s", script)
	}
	if !strings.Contains(script, `write text "echo \"hi\""`) {
		t.Errorf("script does not embed the escaped command:\n%s", script)
	}
	if !strings.Contains(script, `set name to "echo tool"`) {
		t.Errorf("script does not set the session name:\n%s", script)
	}
}

func TestPlanForAppleTerminal(t *testing.T) {
	got := planFor(envFrom(map[string]string{"TERM_PROGRAM": "Apple_Terminal"}), `say "hi" \now`, "say")
	if got.Terminal != "Terminal.app" || got.Fallback {
		t.Fatalf("plan = %+v, want Terminal.app non-fallback", got)
	}
	want := `tell application "Terminal" to do script "say \"hi\" \\now"`
	if len(got.Argv) != 3 || got.Argv[0] != "osascript" || got.Argv[1] != "-e" || got.Argv[2] != want {
		t.Fatalf("Argv = %#v, want [osascript -e %q]", got.Argv, want)
	}
}

func TestAppleScriptQuote(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{``, ``},
		{`plain`, `plain`},
		{`say "hi"`, `say \"hi\"`},
		{`back\slash`, `back\\slash`},
		{`\"`, `\\\"`},
		{`a\\b"c`, `a\\\\b\"c`},
		// Control characters an AppleScript literal cannot carry raw: a pasted
		// newline must become the \n escape, not split the script source.
		{"a\nb", `a\nb`},
		{"a\rb", `a\rb`},
		{"a\tb", `a\tb`},
	}
	for _, tc := range tests {
		if got := appleScriptQuote(tc.in); got != tc.want {
			t.Errorf("appleScriptQuote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDetect(t *testing.T) {
	// Clear every detection variable, then set TMUX: Detect must route through
	// the real os.Getenv and yield the tmux plan.
	t.Setenv("TMUX", "")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("KITTY_WINDOW_ID", "")

	if got := Detect("ls", "ls"); !got.Fallback {
		t.Fatalf("Detect with empty env = %+v, want fallback", got)
	}

	t.Setenv("TMUX", "/tmp/tmux-1/default,1,0")
	got := Detect("ls", "ls")
	if got.Fallback || got.Terminal != "tmux" {
		t.Fatalf("Detect with TMUX set = %+v, want tmux plan", got)
	}
}

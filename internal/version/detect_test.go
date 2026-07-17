package version

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lepeshko/keys/internal/loader"
	"github.com/lepeshko/keys/internal/logx"
)

func TestIsNewer(t *testing.T) {
	tests := []struct {
		name      string
		installed string
		latest    string
		want      bool
	}{
		{"newer major", "1.2.3", "2.0.0", true},
		{"newer minor", "1.2.3", "1.3.0", true},
		{"newer patch", "1.2.3", "1.2.4", true},
		{"older major", "2.0.0", "1.9.9", false},
		{"older minor", "1.3.0", "1.2.9", false},
		{"older patch", "1.2.4", "1.2.3", false},
		{"equal", "1.2.3", "1.2.3", false},
		{"multi-digit segments", "0.9.9", "0.10.0", true},
		{"empty installed", "", "1.0.0", false},
		{"empty latest", "1.0.0", "", false},
		{"both empty", "", "", false},
		{"v-prefix installed", "v1.2.3", "1.2.4", true},
		{"v-prefix latest", "1.2.3", "v1.2.4", true},
		{"v-prefix both equal", "v1.2.3", "v1.2.3", false},
		{"release newer than its rc", "1.2.3-rc1", "1.2.3", true},
		{"rc not newer than release", "1.2.3", "1.2.3-rc1", false},
		{"rc ordering", "1.2.3-rc1", "1.2.3-rc2", true},
		{"build metadata ignored", "1.2.3+build7", "1.2.3+build9", false},
		{"CalVer zero-padded segments", "2024.01.15", "2024.02.01", true},
		{"CalVer equal after zero-strip", "2024.01.15", "2024.1.15", false},
		{"4th segment truncated: equal", "1.2.3.4", "1.2.3.5", false},
		{"4th segment truncated: patch decides", "1.2.3.9", "1.2.4.0", true},
		{"invalid installed", "abc", "1.2.3", false},
		{"invalid latest", "1.2.3", "abc", false},
		{"two segments", "1.2", "1.3", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNewer(tt.installed, tt.latest); got != tt.want {
				t.Errorf("IsNewer(%q, %q) = %v, want %v", tt.installed, tt.latest, got, tt.want)
			}
		})
	}
}

// writeFakeTool creates an executable shell script named `name` in dir.
func writeFakeTool(t *testing.T, dir, name, script string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0755); err != nil {
		t.Fatal(err)
	}
}

func TestInstalledVersion(t *testing.T) {
	dir := t.TempDir()
	writeFakeTool(t, dir, "goodtool", `echo "goodtool version 1.2.3"`)
	writeFakeTool(t, dir, "flagvtool", `if [ "$1" = "-V" ]; then echo "2.0.1"; else exit 1; fi`)
	writeFakeTool(t, dir, "brokentool", `exit 1`)
	writeFakeTool(t, dir, "customtool", `if [ "$1" = "version" ]; then echo "v3.4.5"; else exit 1; fi`)
	t.Setenv("PATH", dir)

	tests := []struct {
		name string
		tool loader.Tool
		want string
	}{
		{"--version output", loader.Tool{Name: "goodtool"}, "1.2.3"},
		{"-V fallback when --version fails", loader.Tool{Name: "flagvtool"}, "2.0.1"},
		{"tool not on PATH", loader.Tool{Name: "missingtool"}, ""},
		{"tool exits non-zero on all candidates", loader.Tool{Name: "brokentool"}, ""},
		// VersionCmd is never populated from ToolMeta today; this pins the
		// unit contract of the override path, not a production flow.
		{"VersionCmd override", loader.Tool{Name: "customtool", VersionCmd: "customtool version"}, "v3.4.5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InstalledVersion(tt.tool); got != tt.want {
				t.Errorf("InstalledVersion(%+v) = %q, want %q", tt.tool, got, tt.want)
			}
		})
	}
}

func TestInstalledVersionLoggingMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)

	logDir := t.TempDir()
	restore := logx.SetDirForTesting(logDir)
	defer restore()

	if got := InstalledVersion(loader.Tool{Name: "missingtool"}); got != "" {
		t.Fatalf("expected empty version, got %q", got)
	}
	out := logx.ReadAllForTesting(logDir)
	// Exactly one log line despite two candidates (--version and -V).
	lines := strings.Count(out, "version.InstalledVersion")
	if lines != 1 {
		t.Fatalf("expected exactly one log line, got %d:\n%s", lines, out)
	}
	if !strings.Contains(out, "missingtool") {
		t.Errorf("log should name the tool, got:\n%s", out)
	}
}

func TestInstalledVersionLoggingFallbackNoLog(t *testing.T) {
	dir := t.TempDir()
	writeFakeTool(t, dir, "flagvtool", `if [ "$1" = "-V" ]; then echo "2.0.1"; else exit 1; fi`)
	t.Setenv("PATH", dir)

	logDir := t.TempDir()
	restore := logx.SetDirForTesting(logDir)
	defer restore()

	if got := InstalledVersion(loader.Tool{Name: "flagvtool"}); got != "2.0.1" {
		t.Fatalf("expected 2.0.1 via -V fallback, got %q", got)
	}
	if out := logx.ReadAllForTesting(logDir); out != "" {
		t.Errorf("a successful -V fallback must not log, got:\n%s", out)
	}
}

func TestInstalledVersionLoggingSuccessNoLog(t *testing.T) {
	dir := t.TempDir()
	writeFakeTool(t, dir, "goodtool", `echo "goodtool version 1.2.3"`)
	t.Setenv("PATH", dir)

	logDir := t.TempDir()
	restore := logx.SetDirForTesting(logDir)
	defer restore()

	if got := InstalledVersion(loader.Tool{Name: "goodtool"}); got != "1.2.3" {
		t.Fatalf("expected 1.2.3, got %q", got)
	}
	if out := logx.ReadAllForTesting(logDir); out != "" {
		t.Errorf("a first-candidate success must not log, got:\n%s", out)
	}
}

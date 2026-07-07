package version

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lepeshko/keys/internal/loader"
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
		// TODO: changes in Task 5 (semver) — pre-release suffixes are currently
		// ignored, so 1.2.3-rc1 compares equal to 1.2.3. With semver.Compare a
		// release is newer than its own rc: this case flips to true.
		{"pre-release ignored", "1.2.3-rc1", "1.2.3", false},
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

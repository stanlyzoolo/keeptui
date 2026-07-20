package updater

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stanlyzoolo/keeptui/internal/loader"
)

func TestDetectFromPath(t *testing.T) {
	home := "/home/tester"
	origHome := testHomeDir
	testHomeDir = home
	defer func() { testHomeDir = origHome }()

	goBuildinfo := "\tpath\tgithub.com/junegunn/fzf\n\tmod\tgithub.com/junegunn/fzf\tv0.55.0\n"

	tests := []struct {
		name        string
		realPath    string
		buildinfo   string
		wantManager string
		wantArgv    []string
		wantErr     error
	}{
		{
			name:        "brew cellar formula from path",
			realPath:    "/opt/homebrew/Cellar/ripgrep/14.1.0/bin/rg",
			wantManager: "brew",
			wantArgv:    []string{"brew", "upgrade", "ripgrep"},
		},
		{
			name:        "go buildinfo module",
			realPath:    "/home/tester/go/bin/fzf",
			buildinfo:   goBuildinfo,
			wantManager: "go",
			wantArgv:    []string{"go", "install", "github.com/junegunn/fzf@latest"},
		},
		{
			name:        "cargo bin dir",
			realPath:    filepath.Join(home, ".cargo", "bin", "exa"),
			wantManager: "cargo",
			wantArgv:    []string{"cargo", "install", "exa"},
		},
		{
			name:        "pipx venv package",
			realPath:    filepath.Join(home, ".local", "pipx", "venvs", "black", "bin", "black"),
			wantManager: "pipx",
			wantArgv:    []string{"pipx", "upgrade", "black"},
		},
		{
			name:        "npm global node_modules",
			realPath:    "/usr/local/lib/node_modules/typescript/bin/tsc",
			wantManager: "npm",
			wantArgv:    []string{"npm", "install", "-g", "typescript"},
		},
		{
			name:        "npm scoped package",
			realPath:    "/usr/local/lib/node_modules/@angular/cli/bin/ng",
			wantManager: "npm",
			wantArgv:    []string{"npm", "install", "-g", "@angular/cli"},
		},
		{
			name:      "unmatched path",
			realPath:  "/usr/local/bin/mytool",
			buildinfo: "",
			wantErr:   ErrUnknownManager,
		},
		{
			name:        "order: cellar with buildinfo yields brew not go",
			realPath:    "/opt/homebrew/Cellar/gopls/0.16.1/bin/gopls",
			buildinfo:   "\tpath\tgolang.org/x/tools/gopls\n",
			wantManager: "brew",
			wantArgv:    []string{"brew", "upgrade", "gopls"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := detectFromPath(tt.realPath, tt.buildinfo)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if plan.Manager != tt.wantManager {
				t.Errorf("Manager = %q, want %q", plan.Manager, tt.wantManager)
			}
			if !equalStrings(plan.Argv, tt.wantArgv) {
				t.Errorf("Argv = %v, want %v", plan.Argv, tt.wantArgv)
			}
			wantDisplay := join(tt.wantArgv)
			if plan.Display != wantDisplay {
				t.Errorf("Display = %q, want %q", plan.Display, wantDisplay)
			}
		})
	}
}

func TestDetectUpdateCmdOverride(t *testing.T) {
	// A tool with UpdateCmd set returns a custom plan even when the binary is
	// not on PATH — proving detection (LookPath) is skipped entirely.
	tool := loader.Tool{Name: "definitely-not-a-real-binary-xyz", UpdateCmd: "brew upgrade rg && echo done"}
	plan, err := Detect(tool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Manager != "custom" {
		t.Errorf("Manager = %q, want %q", plan.Manager, "custom")
	}
	wantArgv := []string{"sh", "-c", "brew upgrade rg && echo done"}
	if !equalStrings(plan.Argv, wantArgv) {
		t.Errorf("Argv = %v, want %v", plan.Argv, wantArgv)
	}
	if plan.Display != "brew upgrade rg && echo done" {
		t.Errorf("Display = %q, want %q", plan.Display, "brew upgrade rg && echo done")
	}
}

func TestDetectMissingBinary(t *testing.T) {
	tool := loader.Tool{Name: "definitely-not-a-real-binary-xyz"}
	_, err := Detect(tool)
	if !errors.Is(err, ErrUnknownManager) {
		t.Fatalf("err = %v, want ErrUnknownManager", err)
	}
}

func TestDetectResolvesSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink/PATH fixture is unix-only")
	}
	tmp := t.TempDir()

	// Real binary under a Homebrew Cellar-shaped path.
	cellarBin := filepath.Join(tmp, "opt", "Cellar", "mytool", "1.0.0", "bin", "mytool")
	if err := os.MkdirAll(filepath.Dir(cellarBin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cellarBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// A bin dir on PATH holds a symlink to the real binary.
	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(binDir, "mytool")
	if err := os.Symlink(cellarBin, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	plan, err := Detect(loader.Tool{Name: "mytool"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// EvalSymlinks must have followed the link into the Cellar path → brew.
	if plan.Manager != "brew" {
		t.Errorf("Manager = %q, want %q", plan.Manager, "brew")
	}
	wantArgv := []string{"brew", "upgrade", "mytool"}
	if !equalStrings(plan.Argv, wantArgv) {
		t.Errorf("Argv = %v, want %v", plan.Argv, wantArgv)
	}
}

func TestCargoCrateFromList(t *testing.T) {
	list := "" +
		"exa v0.10.1:\n" +
		"    exa\n" +
		"ripgrep v14.1.0 (https://github.com/BurntSushi/ripgrep):\n" +
		"    rg\n" +
		"bat v0.24.0:\n" +
		"    bat\n"

	tests := []struct {
		binName string
		want    string
	}{
		{"rg", "ripgrep"},
		{"exa", "exa"},
		{"bat", "bat"},
		{"nonexistent", ""},
	}
	for _, tt := range tests {
		if got := cargoCrateFromList(list, tt.binName); got != tt.want {
			t.Errorf("cargoCrateFromList(_, %q) = %q, want %q", tt.binName, got, tt.want)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func join(argv []string) string {
	return strings.Join(argv, " ")
}

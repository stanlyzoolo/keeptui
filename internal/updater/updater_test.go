package updater

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
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

// Package updater detects the package manager that owns an installed tool
// binary and produces an update Plan. It sits at the bottom of the import
// graph like internal/version: it has no TUI knowledge and depends only on
// internal/loader for the Tool type.
package updater

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ErrUnknownManager is returned when no package manager can be attributed to a
// tool's installed binary. Callers treat it as "no automatic update available"
// rather than a hard failure.
var ErrUnknownManager = errors.New("no known package manager for tool")

// Plan describes how to update a tool. Argv is executed directly (not through a
// shell) except for the "custom" manager, where it is ["sh", "-c", <cmd>].
type Plan struct {
	Manager string   // "brew" | "go" | "cargo" | "npm" | "pipx" | "custom"
	Argv    []string // e.g. ["brew", "upgrade", "ripgrep"]
	Display string   // human-facing command shown in the confirm dialog
}

// testHomeDir overrides the home directory in tests (mirrors
// loader.testConfigDir / version.testCacheDir).
var testHomeDir string

func homeDir() string {
	if testHomeDir != "" {
		return testHomeDir
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}

// cellarRe extracts the formula name from a Homebrew Cellar path segment:
// .../Cellar/<formula>/<version>/bin/<binary>.
var cellarRe = regexp.MustCompile(`/Cellar/([^/]+)/`)

// goPathRe matches the `path <module>` line emitted by `go version -m`.
var goPathRe = regexp.MustCompile(`(?m)^\s*path\s+(\S+)\s*$`)

// detectFromPath is the pure detection core: given a binary's real (symlink
// resolved) path and the output of `go version -m <path>` (may be empty), it
// returns the update Plan. It performs no I/O and spawns no subprocesses, so
// table tests need no real package managers installed.
//
// Order matters: brew is checked before go because a brew-installed Go binary
// carries buildinfo and would otherwise be misrouted to `go install`.
func detectFromPath(realPath, buildinfo string) (Plan, error) {
	// 1. Homebrew Cellar.
	if m := cellarRe.FindStringSubmatch(realPath); m != nil {
		formula := m[1]
		return autoPlan("brew", []string{"brew", "upgrade", formula}), nil
	}

	// 2. Go buildinfo module path.
	if m := goPathRe.FindStringSubmatch(buildinfo); m != nil {
		module := m[1]
		return autoPlan("go", []string{"go", "install", module + "@latest"}), nil
	}

	home := homeDir()

	// 3. Cargo (~/.cargo/bin). Crate name defaults to the binary name; the OS
	// wrapper refines it via `cargo install --list`.
	if home != "" {
		cargoBin := filepath.Join(home, ".cargo", "bin")
		if underDir(realPath, cargoBin) {
			crate := binaryName(realPath)
			return autoPlan("cargo", []string{"cargo", "install", crate}), nil
		}
	}

	// 4. pipx (~/.local/pipx/venvs/<pkg>/...).
	if home != "" {
		pipxVenvs := filepath.Join(home, ".local", "pipx", "venvs")
		if pkg := segmentUnder(realPath, pipxVenvs); pkg != "" {
			return autoPlan("pipx", []string{"pipx", "upgrade", pkg}), nil
		}
	}

	// 5. npm global (.../node_modules/<pkg>/...).
	if pkg := npmPackage(realPath); pkg != "" {
		return autoPlan("npm", []string{"npm", "install", "-g", pkg}), nil
	}

	return Plan{}, ErrUnknownManager
}

// autoPlan builds a Plan whose Display is the joined Argv.
func autoPlan(manager string, argv []string) Plan {
	return Plan{Manager: manager, Argv: argv, Display: strings.Join(argv, " ")}
}

// binaryName returns the file name of a path without directory.
func binaryName(p string) string {
	return filepath.Base(p)
}

// underDir reports whether path p lives under directory dir.
func underDir(p, dir string) bool {
	rel, err := filepath.Rel(dir, p)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// segmentUnder returns the first path segment of p immediately below dir, or ""
// if p is not under dir. E.g. segmentUnder(".../venvs/black/bin/black",
// ".../venvs") == "black".
func segmentUnder(p, dir string) string {
	rel, err := filepath.Rel(dir, p)
	if err != nil {
		return ""
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) == 0 || parts[0] == "." || parts[0] == "" {
		return ""
	}
	return parts[0]
}

// npmPackage returns the npm package name for a binary whose realpath goes
// through a node_modules directory (.../node_modules/<pkg>/...), handling
// scoped packages (@scope/name). Returns "" when there is no node_modules
// segment.
func npmPackage(p string) string {
	parts := strings.Split(filepath.ToSlash(p), "/")
	for i, seg := range parts {
		if seg != "node_modules" {
			continue
		}
		if i+1 >= len(parts) {
			return ""
		}
		pkg := parts[i+1]
		if strings.HasPrefix(pkg, "@") && i+2 < len(parts) {
			return pkg + "/" + parts[i+2]
		}
		return pkg
	}
	return ""
}

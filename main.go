package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stanlyzoolo/keeptui/internal/loader"
	"github.com/stanlyzoolo/keeptui/internal/logx"
	"github.com/stanlyzoolo/keeptui/internal/model"
	verpkg "github.com/stanlyzoolo/keeptui/internal/version"
)

// version is overridden at release time via -ldflags "-X main.version=<tag>"
// (see .github/workflows/release.yml). It defaults to "dev" for local builds.
var version = "dev"

func main() {
	runTUI()
}

// migrateConfigDir renames the pre-rename config directory (<UserConfigDir>/keys,
// from when the app was called "keys") to <UserConfigDir>/keeptui. One-shot and
// conservative: if the new directory already exists — even empty — nothing is
// touched, so a half-adopted new install is never overwritten by old data.
func migrateConfigDir() {
	base, err := os.UserConfigDir()
	if err != nil {
		return
	}
	oldDir := filepath.Join(base, "keys")
	newDir := filepath.Join(base, "keeptui")
	if _, err := os.Stat(newDir); err == nil {
		return
	}
	if _, err := os.Stat(oldDir); err != nil {
		return
	}
	if err := os.Rename(oldDir, newDir); err != nil {
		logx.Errorf("config migration %s -> %s: %v", oldDir, newDir, err)
	}
}

func runTUI() {
	// Before logx.Cleanup: the log directory itself lives inside the config
	// directory being migrated.
	migrateConfigDir()
	logx.Cleanup()
	// Partial header first, so even a LoadMeta failure gets a non-blank header.
	logx.SetHeader(fmt.Sprintf("keeptui %s %s/%s", version, runtime.GOOS, runtime.GOARCH))

	meta, err := loader.LoadMeta()
	if err != nil {
		logx.Errorf("loader.LoadMeta: %v", err)
		fmt.Fprintf(os.Stderr, "error loading tools: %v\n", err)
		os.Exit(1)
	}
	// Enrich the header with tool count and token source now that meta loaded.
	logx.SetHeader(fmt.Sprintf("keeptui %s %s/%s tools=%d token=%s",
		version, runtime.GOOS, runtime.GOARCH, len(meta), verpkg.TokenSource()))

	p := tea.NewProgram(
		model.New(meta),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := p.Run(); err != nil {
		if errors.Is(err, tea.ErrProgramPanic) {
			logx.Errorf("tea.Run ended in panic: %v", err)
		} else {
			logx.Errorf("tea.Run: %v", err)
		}
		fmt.Fprintf(os.Stderr, "error running: %v\n", err)
		os.Exit(1)
	}
}

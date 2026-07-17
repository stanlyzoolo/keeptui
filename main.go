package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lepeshko/keys/internal/loader"
	"github.com/lepeshko/keys/internal/logx"
	"github.com/lepeshko/keys/internal/model"
	verpkg "github.com/lepeshko/keys/internal/version"
)

// version is overridden at release time via -ldflags "-X main.version=<tag>"
// (see .github/workflows/release.yml). It defaults to "dev" for local builds.
var version = "dev"

func main() {
	runTUI()
}

func runTUI() {
	logx.Cleanup()
	// Partial header first, so even a LoadMeta failure gets a non-blank header.
	logx.SetHeader(fmt.Sprintf("keys %s %s/%s", version, runtime.GOOS, runtime.GOARCH))

	meta, err := loader.LoadMeta()
	if err != nil {
		logx.Errorf("loader.LoadMeta: %v", err)
		fmt.Fprintf(os.Stderr, "error loading tools: %v\n", err)
		os.Exit(1)
	}
	// Enrich the header with tool count and token source now that meta loaded.
	logx.SetHeader(fmt.Sprintf("keys %s %s/%s tools=%d token=%s",
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

package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lepeshko/keys/internal/loader"
	"github.com/lepeshko/keys/internal/model"
)

func main() {
	runTUI()
}

func runTUI() {
	meta, err := loader.LoadMeta()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading tools: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(
		model.New(meta),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error running: %v\n", err)
		os.Exit(1)
	}
}

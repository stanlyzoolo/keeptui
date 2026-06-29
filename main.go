package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lepeshko/keys/internal/cmd"
	"github.com/lepeshko/keys/internal/loader"
	"github.com/lepeshko/keys/internal/model"
)

const helpText = `keys — personal CLI/TUI tool registry

Usage:
  keys                          open interactive TUI
  keys status <tool> active|trying|forgotten|archived
  keys note <tool> "text"
  keys list, --list             list tracked tools
  keys list --active|--trying|--forgotten|--archived  filter by status
  keys list --tag <name>        filter by tag

Flags:
  -h, --help                    show this help
`

type tuiOptions struct {
	initialTool   string
	initialSearch string
}

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		runTUI(tuiOptions{})
		return
	}

	var opts tuiOptions
	var remaining []string
	for i := 0; i < len(args); i++ {
		if args[i] == "-s" && i+1 < len(args) {
			opts.initialSearch = args[i+1]
			i++
		} else {
			remaining = append(remaining, args[i])
		}
	}

	if len(remaining) == 0 {
		runTUI(opts)
		return
	}

	if err := runCommand(remaining, opts); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runCommand(args []string, opts tuiOptions) error {
	switch args[0] {
	case "-h", "--help":
		fmt.Print(helpText)
		return nil

	case "--list":
		return cmd.RunList()

	case "status":
		return cmd.RunStatus(args[1:])

	case "note":
		return cmd.RunNote(args[1:])

	case "list":
		flags := parseListFlags(args[1:])
		return cmd.RunListWithFlags(flags)

	default:
		opts.initialTool = args[0]
		runTUI(opts)
		return nil
	}
}

func parseListFlags(args []string) cmd.ListFlags {
	var flags cmd.ListFlags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--active":
			flags.Active = true
		case "--trying":
			flags.Trying = true
		case "--forgotten":
			flags.Forgotten = true
		case "--archived":
			flags.Archived = true
		case "--tag":
			if i+1 < len(args) {
				flags.Tag = args[i+1]
				i++
			}
		}
	}
	return flags
}

func runTUI(opts tuiOptions) {
	meta, err := loader.LoadMeta()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading tools: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(
		model.New(meta, model.Options{
			InitialTool:   opts.initialTool,
			InitialSearch: opts.initialSearch,
		}),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error running: %v\n", err)
		os.Exit(1)
	}
}

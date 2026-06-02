package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lepeshko/keys/internal/cmd"
	"github.com/lepeshko/keys/internal/loader"
	"github.com/lepeshko/keys/internal/model"
)

const helpText = `keys — terminal hotkey cheatsheet viewer

Usage:
  keys                          open interactive TUI
  keys <tool>                   open TUI directly on a tool (e.g. keys yazi)
  keys -s <query>               open TUI with search pre-filled
  keys new <tool>               create ~/.config/keys/tools/<tool>/config.yaml from template and open in $EDITOR
  keys import <url|path>        import a YAML file (validates before saving)
  keys edit <tool>              open ~/.config/keys/tools/<tool>/config.yaml in $EDITOR
  keys edit --builtin <tool>    copy built-in config to ~/.config/keys/tools/ and open in $EDITOR
  keys validate <path>          validate a YAML file without importing
  keys list, --list             list all available tools
  keys list --active|--trying|--forgotten|--archived  filter by status
  keys list --tag <name>        filter by tag
  keys check <tool>             check installed and latest version of a tool
  keys check --all              check all tools
  keys check --outdated         show only tools with available updates
  keys fetch <tool>             fetch commands from tldr-pages and add to tool config
  keys track <tool> [--status trying] [--tags a,b] [--note "..."]
  keys status <tool> active|trying|forgotten|archived
  keys note <tool> "text"
  keys untrack <tool>

Flags:
  -s <query>                    pre-fill TUI search
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

	// Strip -s <query> flag from args before dispatching.
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

	// Only the -s flag was given, no subcommand or tool name.
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

	case "track":
		return cmd.RunTrack(args[1:])

	case "status":
		return cmd.RunStatus(args[1:])

	case "note":
		return cmd.RunNote(args[1:])

	case "untrack":
		return cmd.RunUntrack(args[1:])

	case "new":
		if len(args) < 2 {
			return fmt.Errorf("usage: keys new <tool>")
		}
		return cmd.RunNew(args[1])

	case "import":
		if len(args) < 2 {
			return fmt.Errorf("usage: keys import <url|path>")
		}
		return cmd.RunImport(args[1])

	case "edit":
		if len(args) < 2 {
			return fmt.Errorf("usage: keys edit [--builtin] <tool>")
		}
		if args[1] == "--builtin" {
			if len(args) < 3 {
				return fmt.Errorf("usage: keys edit --builtin <tool>")
			}
			return cmd.RunEditBuiltin(args[2], loader.Embedded)
		}
		return cmd.RunEdit(args[1])

	case "validate":
		if len(args) < 2 {
			return fmt.Errorf("usage: keys validate <path>")
		}
		return cmd.RunValidate(args[1])

	case "list":
		flags := parseListFlags(args[1:])
		return cmd.RunListWithFlags(flags)

	case "check":
		if len(args) < 2 {
			return fmt.Errorf("usage: keys check <tool> | --all | --outdated")
		}
		switch args[1] {
		case "--all":
			return cmd.RunCheck("", true, false)
		case "--outdated":
			return cmd.RunCheck("", false, true)
		default:
			return cmd.RunCheck(args[1], false, false)
		}

	case "fetch":
		if len(args) < 2 {
			return fmt.Errorf("usage: keys fetch <tool>")
		}
		return cmd.RunFetch(args[1])

	default:
		// Treat as tool name: open TUI directly on that tool.
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
	tools, err := loader.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading tools: %v\n", err)
		os.Exit(1)
	}

	meta, _ := loader.LoadMeta()

	p := tea.NewProgram(
		model.New(tools, meta, model.Options{
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

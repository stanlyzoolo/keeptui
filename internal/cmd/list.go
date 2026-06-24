package cmd

import (
	"fmt"
	"strings"

	"github.com/lepeshko/keys/internal/loader"
)

type ListFlags struct {
	Active    bool
	Trying    bool
	Forgotten bool
	Archived  bool
	Tag       string
}

func RunList() error {
	return RunListWithFlags(ListFlags{})
}

func RunListWithFlags(flags ListFlags) error {
	filtering := flags.Active || flags.Trying || flags.Forgotten || flags.Archived || flags.Tag != ""

	if filtering {
		return runMetaList(flags)
	}

	tools, err := loader.Load()
	if err != nil {
		return fmt.Errorf("cannot load tools: %v", err)
	}

	if len(tools) == 0 {
		fmt.Println("No tools found.")
		return nil
	}

	nameWidth := columnWidths(tools)
	for _, t := range tools {
		github := t.GitHub
		if github == "" {
			github = "—"
		}
		fmt.Printf("%-*s  %s\n", nameWidth, t.Name, github)
	}
	return nil
}

func runMetaList(flags ListFlags) error {
	meta, err := loader.LoadMeta()
	if err != nil {
		return fmt.Errorf("cannot load meta: %v", err)
	}

	var allowed []loader.Status
	if flags.Active {
		allowed = append(allowed, loader.StatusActive)
	}
	if flags.Trying {
		allowed = append(allowed, loader.StatusTrying)
	}
	if flags.Forgotten {
		allowed = append(allowed, loader.StatusForgotten)
	}
	if flags.Archived {
		allowed = append(allowed, loader.StatusArchived)
	}

	allowedSet := make(map[loader.Status]bool, len(allowed))
	for _, s := range allowed {
		allowedSet[s] = true
	}

	var filtered []loader.ToolMeta
	for _, m := range meta {
		if len(allowedSet) > 0 && !allowedSet[m.Status] {
			continue
		}
		if flags.Tag != "" && !hasTag(m.Tags, flags.Tag) {
			continue
		}
		filtered = append(filtered, m)
	}

	if len(filtered) == 0 {
		fmt.Println("No tools match the filter.")
		return nil
	}

	for _, m := range filtered {
		sym := loader.StatusSymbol[m.Status]
		tags := strings.Join(m.Tags, ", ")
		note := ""
		if m.Note != "" {
			note = fmt.Sprintf("  %q", m.Note)
		}
		fmt.Printf("%s %-9s  %-16s  %-24s%s\n", sym, m.Status, m.Name, tags, note)
	}
	return nil
}

func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

func columnWidths(tools []loader.Tool) (nameW int) {
	for _, t := range tools {
		if len(t.Name) > nameW {
			nameW = len(t.Name)
		}
	}
	return
}

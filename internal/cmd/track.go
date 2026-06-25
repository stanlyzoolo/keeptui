package cmd

import (
	"flag"
	"fmt"
	"strings"

	"github.com/lepeshko/keys/internal/loader"
)

func RunTrack(args []string) error {
	if len(args) < 1 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("usage: keys track <tool> [--status trying] [--tags a,b] [--note \"...\"]")
	}

	name := args[0]
	rest := args[1:]

	fs := flag.NewFlagSet("track", flag.ContinueOnError)
	statusFlag := fs.String("status", "trying", "status: active|trying|forgotten|archived")
	tagsFlag := fs.String("tags", "", "comma-separated tags")
	noteFlag := fs.String("note", "", "note text")
	githubFlag := fs.String("github", "", "GitHub repo, e.g. github.com/owner/repo")

	if err := fs.Parse(rest); err != nil {
		return err
	}

	status := loader.Status(*statusFlag)

	validStatuses := map[loader.Status]bool{
		loader.StatusActive:    true,
		loader.StatusTrying:    true,
		loader.StatusForgotten: true,
		loader.StatusArchived:  true,
	}
	if !validStatuses[status] {
		return fmt.Errorf("invalid status %q, use: active, trying, forgotten, archived", status)
	}

	meta, err := loader.LoadMeta()
	if err != nil {
		return err
	}

	existing := loader.FindMeta(meta, name)
	var entry loader.ToolMeta
	if existing != nil {
		entry = *existing
		entry.Status = status
		if *tagsFlag != "" {
			entry.Tags = splitTags(*tagsFlag)
		}
		if *noteFlag != "" {
			entry.Note = *noteFlag
		}
		if *githubFlag != "" {
			entry.GitHub = *githubFlag
		}
	} else {
		entry = loader.ToolMeta{
			Name:   name,
			Status: status,
			Added:  loader.TodayDate(),
		}
		if *tagsFlag != "" {
			entry.Tags = splitTags(*tagsFlag)
		}
		if *noteFlag != "" {
			entry.Note = *noteFlag
		}
		if *githubFlag != "" {
			entry.GitHub = *githubFlag
		}
	}

	meta = loader.UpsertMeta(meta, entry)
	if err := loader.SaveMeta(meta); err != nil {
		return err
	}

	sym := loader.StatusSymbol[status]
	fmt.Printf("Tracked %s (%s %s)\n", name, sym, status)
	return nil
}

func splitTags(s string) []string {
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

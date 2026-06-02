package cmd

import (
	"fmt"

	"github.com/lepeshko/keys/internal/loader"
)

func RunNote(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: keys note <tool> \"text\"")
	}

	name := args[0]
	note := args[1]

	meta, err := loader.LoadMeta()
	if err != nil {
		return err
	}

	existing := loader.FindMeta(meta, name)
	if existing == nil {
		return fmt.Errorf("tool %q not found — run: keys track %s", name, name)
	}

	existing.Note = note
	meta = loader.UpsertMeta(meta, *existing)
	if err := loader.SaveMeta(meta); err != nil {
		return err
	}

	fmt.Printf("Note updated for %s\n", name)
	return nil
}

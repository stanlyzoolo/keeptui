package cmd

import (
	"fmt"

	"github.com/lepeshko/keys/internal/loader"
)

func RunStatus(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: keys status <tool> active|trying|forgotten|archived")
	}

	name := args[0]
	status := loader.Status(args[1])

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
	if existing == nil {
		return fmt.Errorf("tool %q not found — run: keys track %s", name, name)
	}

	existing.Status = status
	meta = loader.UpsertMeta(meta, *existing)
	if err := loader.SaveMeta(meta); err != nil {
		return err
	}

	sym := loader.StatusSymbol[status]
	fmt.Printf("Updated %s → %s %s\n", name, sym, status)
	return nil
}

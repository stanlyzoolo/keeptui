package cmd

import (
	"fmt"
	"os"

	"github.com/lepeshko/keys/internal/loader"
)

func RunValidate(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read file: %v", err)
	}

	tool, errs := loader.Validate(data)
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "ERROR: %s\n", e)
		}
		return fmt.Errorf("validation failed")
	}

	totalBindings := 0
	for _, cat := range tool.Categories {
		totalBindings += len(cat.Bindings)
	}
	fmt.Printf("OK: %d categories, %d bindings\n", len(tool.Categories), totalBindings)
	return nil
}

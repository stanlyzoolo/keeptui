package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/lepeshko/keys/internal/loader"
)

func RunUntrack(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: keys untrack <tool>")
	}

	name := args[0]

	meta, err := loader.LoadMeta()
	if err != nil {
		return err
	}

	if loader.FindMeta(meta, name) == nil {
		return fmt.Errorf("tool %q not found", name)
	}

	fmt.Printf("Untrack %s? (y/N) ", name)
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line != "y" {
		fmt.Println("Aborted.")
		return nil
	}

	meta = loader.RemoveMeta(meta, name)
	if err := loader.SaveMeta(meta); err != nil {
		return err
	}

	fmt.Printf("Untracked %s\n", name)
	return nil
}

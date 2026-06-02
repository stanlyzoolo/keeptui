package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lepeshko/keys/internal/loader"
)

func validateToolName(name string) error {
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return fmt.Errorf("tool name %q must not contain path separators or \"..\"", name)
	}
	return nil
}

func userToolsDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine config dir: %v", err)
	}
	return filepath.Join(base, "keys", "tools"), nil
}

func userToolPath(toolName string) (string, error) {
	dir, err := userToolsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, toolName, "config.yaml"), nil
}

func openEditor(path string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func editAndValidate(path string) error {
	for {
		if err := openEditor(path); err != nil {
			return err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("cannot read file after editing: %v", err)
		}

		_, errs := loader.Validate(data)
		if len(errs) == 0 {
			return nil
		}

		fmt.Fprintln(os.Stderr, "\nValidation errors:")
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  • %s\n", e)
		}

		reopen, err := confirm("\nReopen editor to fix? [Y/n] ", true)
		if err != nil || !reopen {
			return nil
		}
	}
}

func confirm(prompt string, defaultYes bool) (bool, error) {
	fmt.Print(prompt)
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil {
		return false, err
	}
	s := strings.ToLower(strings.TrimSpace(line))
	if s == "" {
		return defaultYes, nil
	}
	return s == "y", nil
}

func confirmOverwrite(path string) (bool, error) {
	return confirm(fmt.Sprintf("File already exists: %s\nOverwrite? [y/N] ", path), false)
}

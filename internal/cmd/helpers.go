package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func validateToolName(name string) error {
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return fmt.Errorf("tool name %q must not contain path separators or \"..\"", name)
	}
	return nil
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

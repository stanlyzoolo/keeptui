package cmd

import (
	"fmt"
	"os"
	"path/filepath"
)

const yamlTemplate = `name: %s
description: Short description of the tool
github: github.com/user/%s
categories:
  - name: Navigation
    bindings:
      - key: "j / k"
        desc: move down / up
      - key: "Enter"
        desc: confirm / open
  - name: Actions
    bindings:
      - key: "q"
        desc: quit
`

func RunNew(toolName string) error {
	if err := validateToolName(toolName); err != nil {
		return err
	}
	path, err := userToolPath(toolName)
	if err != nil {
		return err
	}

	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("file already exists: %s\nuse 'keys edit %s' to open it", path, toolName)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("cannot create config dir: %v", err)
	}

	content := fmt.Sprintf(yamlTemplate, toolName, toolName)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("cannot write file: %v", err)
	}

	fmt.Printf("Created %s\n", path)
	return editAndValidate(path)
}

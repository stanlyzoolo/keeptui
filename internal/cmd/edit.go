package cmd

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func RunEdit(toolName string) error {
	path, err := userToolPath(toolName)
	if err != nil {
		return err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf(
			"no user config for %q\nuse 'keys new %s' to create one, or 'keys edit --builtin %s' to start from the built-in",
			toolName, toolName, toolName,
		)
	}

	return editAndValidate(path)
}

func RunEditBuiltin(toolName string, embedded embed.FS) error {
	if err := validateToolName(toolName); err != nil {
		return err
	}
	srcPath := "data/tools/" + toolName + "/config.yaml"
	data, err := embedded.ReadFile(srcPath)
	if err != nil {
		entries, _ := fs.ReadDir(embedded, "data/tools")
		return fmt.Errorf("no built-in config for %q\navailable: %s", toolName, joinDirNames(entries))
	}

	dest, err := userToolPath(toolName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("cannot create config dir: %v", err)
	}

	if _, err := os.Stat(dest); err == nil {
		ok, err := confirmOverwrite(dest)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Println("Aborted.")
			return nil
		}
	}

	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return fmt.Errorf("cannot write file: %v", err)
	}

	fmt.Printf("Copied built-in %q → %s\n", toolName, dest)
	return editAndValidate(dest)
}

func joinDirNames(entries []fs.DirEntry) string {
	var names []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	return strings.Join(names, ", ")
}

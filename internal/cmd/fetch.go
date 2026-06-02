package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/lepeshko/keys/internal/loader"
	"github.com/lepeshko/keys/internal/tldr"
)

func RunFetch(toolName string) error {
	if err := validateToolName(toolName); err != nil {
		return err
	}

	fmt.Printf("Fetching tldr page for %q...\n", toolName)
	content, err := tldr.Fetch(toolName)
	if err != nil {
		return err
	}

	groups := tldr.Parse(content)
	if len(groups) == 0 {
		return fmt.Errorf("no commands found in tldr page for %q", toolName)
	}

	fmt.Printf("Found %d command(s):\n\n", countCommands(groups))
	for _, g := range groups {
		fmt.Printf("  [%s]\n", g.Name)
		for _, c := range g.Commands {
			fmt.Printf("    %-40s %s\n", c.Cmd, c.Desc)
		}
	}
	fmt.Println()

	ok, err := confirm("Add to tool config? [Y/n] ", true)
	if err != nil || !ok {
		fmt.Println("Aborted.")
		return nil
	}

	configPath, err := resolveConfigPath(toolName)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("cannot read config %s: %v", configPath, err)
	}

	var t loader.Tool
	if err := yaml.Unmarshal(data, &t); err != nil {
		return fmt.Errorf("cannot parse config: %v", err)
	}

	t.CommandGroups = groups

	out, err := yaml.Marshal(t)
	if err != nil {
		return fmt.Errorf("cannot marshal config: %v", err)
	}

	if err := os.WriteFile(configPath, out, 0644); err != nil {
		return fmt.Errorf("cannot write config: %v", err)
	}

	fmt.Printf("Saved to %s\n", configPath)
	return nil
}

func countCommands(groups []loader.CommandGroup) int {
	n := 0
	for _, g := range groups {
		n += len(g.Commands)
	}
	return n
}

// resolveConfigPath returns the user config path, copying from built-in if needed.
func resolveConfigPath(toolName string) (string, error) {
	userPath, err := userToolPath(toolName)
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(userPath); err == nil {
		return userPath, nil
	}

	// try built-in
	builtinData, builtinErr := loader.Embedded.ReadFile("data/tools/" + toolName + "/config.yaml")
	if builtinErr != nil {
		// neither user nor built-in — create minimal stub
		dir := filepath.Dir(userPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("cannot create config dir: %v", err)
		}
		stub := fmt.Sprintf("name: %s\ndescription: \"\"\ncategories: []\n", toolName)
		if err := os.WriteFile(userPath, []byte(stub), 0644); err != nil {
			return "", fmt.Errorf("cannot write stub config: %v", err)
		}
		return userPath, nil
	}

	// copy built-in to user dir
	dir := filepath.Dir(userPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("cannot create config dir: %v", err)
	}
	if err := os.WriteFile(userPath, builtinData, 0644); err != nil {
		return "", fmt.Errorf("cannot copy built-in config: %v", err)
	}
	fmt.Printf("Copied built-in config to %s\n", userPath)
	return userPath, nil
}

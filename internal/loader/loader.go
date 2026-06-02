package loader

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Binding struct {
	Key  string `yaml:"key"`
	Desc string `yaml:"desc"`
}

type Category struct {
	Name     string    `yaml:"name"`
	Bindings []Binding `yaml:"bindings"`
}

type Command struct {
	Cmd  string `yaml:"cmd"`
	Desc string `yaml:"desc"`
}

type CommandGroup struct {
	Name     string    `yaml:"name"`
	Commands []Command `yaml:"commands"`
}

type Tool struct {
	Name          string         `yaml:"name"`
	Description   string         `yaml:"description"`
	GitHub        string         `yaml:"github"`
	VersionCmd    string         `yaml:"version_cmd"`
	Categories    []Category     `yaml:"categories"`
	CommandGroups []CommandGroup `yaml:"command_groups,omitempty"`
	Source        string         `yaml:"-"` // "built-in" or "user"
}

//go:embed data/tools
var Embedded embed.FS

func Load() ([]Tool, error) {
	tools := make(map[string]Tool)
	order := []string{}

	entries, err := fs.ReadDir(Embedded, "data/tools")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, err := Embedded.ReadFile("data/tools/" + e.Name() + "/config.yaml")
		if err != nil {
			continue
		}
		var t Tool
		if err := yaml.Unmarshal(data, &t); err != nil {
			continue
		}
		t.Source = "built-in"
		tools[e.Name()] = t
		order = append(order, e.Name())
	}

	configDir, err := os.UserConfigDir()
	if err == nil {
		userToolsDir := filepath.Join(configDir, "keys", "tools")
		userEntries, _ := os.ReadDir(userToolsDir)
		for _, e := range userEntries {
			if !e.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(userToolsDir, e.Name(), "config.yaml"))
			if err != nil {
				continue
			}
			var t Tool
			if err := yaml.Unmarshal(data, &t); err != nil {
				continue
			}
			_, exists := tools[e.Name()]
			if !exists {
				order = append(order, e.Name())
			}
			t.Source = "user"
			tools[e.Name()] = t
		}
	}

	result := make([]Tool, 0, len(order))
	for _, name := range order {
		if t, ok := tools[name]; ok {
			result = append(result, t)
		}
	}
	return result, nil
}

package loader

type Tool struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	GitHub      string `yaml:"github"`
	VersionCmd  string `yaml:"version_cmd"`
	Source      string `yaml:"-"`
}

// ToolsFromMeta converts tracked ToolMeta entries to Tool structs.
func ToolsFromMeta(meta []ToolMeta) []Tool {
	tools := make([]Tool, len(meta))
	for i, m := range meta {
		tools[i] = Tool{
			Name:   m.Name,
			GitHub: m.GitHub,
			Source: "meta",
		}
	}
	return tools
}

// Load builds []Tool from meta.yaml (tracked tools).
func Load() ([]Tool, error) {
	meta, err := LoadMeta()
	if err != nil {
		return nil, err
	}
	return ToolsFromMeta(meta), nil
}

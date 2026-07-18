package loader

type Tool struct {
	Name       string
	GitHub     string
	VersionCmd string
	Source     string
	UpdateCmd  string
}

// ToolsFromMeta converts tracked ToolMeta entries to Tool structs.
func ToolsFromMeta(meta []ToolMeta) []Tool {
	tools := make([]Tool, len(meta))
	for i, m := range meta {
		tools[i] = Tool{
			Name:      m.Name,
			GitHub:    m.GitHub,
			Source:    "meta",
			UpdateCmd: m.UpdateCmd,
		}
	}
	return tools
}

package loader

type Tool struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	GitHub      string `yaml:"github"`
	VersionCmd  string `yaml:"version_cmd"`
	Source      string `yaml:"-"`
}

// Load returns an empty slice; tools are now built from meta.yaml (see Task 2).
func Load() ([]Tool, error) {
	return []Tool{}, nil
}

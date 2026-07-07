package loader

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Status string

const (
	StatusActive    Status = "active"
	StatusTrying    Status = "trying"
	StatusForgotten Status = "forgotten"
	StatusArchived  Status = "archived"
)

var StatusSymbol = map[Status]string{
	StatusActive:    "●",
	StatusTrying:    "○",
	StatusForgotten: "~",
	StatusArchived:  "✕",
}

var StatusCycle = []Status{
	StatusActive,
	StatusTrying,
	StatusForgotten,
	StatusArchived,
}

type ToolMeta struct {
	Name   string   `yaml:"name"`
	Status Status   `yaml:"status"`
	Added  string   `yaml:"added"`
	Tags   []string `yaml:"tags,omitempty"`
	Note   string   `yaml:"note,omitempty"`
	GitHub string   `yaml:"github,omitempty"`
}

// testConfigDir overrides the config directory in tests.
var testConfigDir string

func MetaPath() string {
	if testConfigDir != "" {
		return filepath.Join(testConfigDir, "keys", "meta.yaml")
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(configDir, "keys", "meta.yaml")
}

func LoadMeta() ([]ToolMeta, error) {
	path := MetaPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []ToolMeta{}, nil
	}
	if err != nil {
		return nil, err
	}
	var meta []ToolMeta
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return meta, nil
}

// SaveMeta writes meta.yaml atomically: the data lands in a temp file in the
// same directory first, then rename replaces the old file in one step, so a
// crash mid-write can never leave a truncated meta.yaml behind.
func SaveMeta(meta []ToolMeta) error {
	path := MetaPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(meta)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func FindMeta(meta []ToolMeta, name string) *ToolMeta {
	for i := range meta {
		if meta[i].Name == name {
			return &meta[i]
		}
	}
	return nil
}

func UpsertMeta(meta []ToolMeta, entry ToolMeta) []ToolMeta {
	for i := range meta {
		if meta[i].Name == entry.Name {
			meta[i] = entry
			return meta
		}
	}
	return append(meta, entry)
}

func RemoveMeta(meta []ToolMeta, name string) []ToolMeta {
	out := meta[:0]
	for _, m := range meta {
		if m.Name != name {
			out = append(out, m)
		}
	}
	return out
}

func TodayDate() string {
	return time.Now().Format("2006-01-02")
}

func NextStatus(s Status) Status {
	for i, cs := range StatusCycle {
		if cs == s {
			return StatusCycle[(i+1)%len(StatusCycle)]
		}
	}
	return StatusActive
}

package loader

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/stanlyzoolo/keeptui/internal/logx"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusTrying   Status = "trying"
	StatusInactive Status = "inactive"
)

var StatusSymbol = map[Status]string{
	StatusActive:   "●",
	StatusTrying:   "○",
	StatusInactive: "✕",
}

var StatusCycle = []Status{
	StatusActive,
	StatusTrying,
	StatusInactive,
}

type ToolMeta struct {
	Name   string   `yaml:"name"`
	Status Status   `yaml:"status"`
	Added  string   `yaml:"added"`
	Tags   []string `yaml:"tags,omitempty"`
	Note   string   `yaml:"note,omitempty"`
	GitHub string   `yaml:"github,omitempty"`
	// UpdateCmd is an explicit update command that overrides package-manager
	// detection. omitempty keeps meta.yaml written without the field backward
	// compatible.
	UpdateCmd string `yaml:"update_cmd,omitempty"`
}

// testConfigDir overrides the config directory in tests.
var testConfigDir string

func MetaPath() string {
	if testConfigDir != "" {
		return filepath.Join(testConfigDir, "keeptui", "meta.yaml")
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(configDir, "keeptui", "meta.yaml")
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
	// In-memory migration of retired statuses; the file keeps the old value
	// until the next SaveMeta. Unknown statuses pass through untouched —
	// NextStatus already falls back to active for them.
	for i := range meta {
		switch meta[i].Status {
		case "forgotten", "archived":
			meta[i].Status = StatusInactive
		}
	}
	return meta, nil
}

// SaveMeta writes meta.yaml atomically: the data lands in a temp file in the
// same directory first, then rename replaces the old file in one step, so a
// crash mid-write can never leave a truncated meta.yaml behind.
func SaveMeta(meta []ToolMeta) error {
	path := MetaPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		logx.Errorf("loader.SaveMeta: mkdir %s: %v", filepath.Dir(path), err)
		return err
	}
	data, err := yaml.Marshal(meta)
	if err != nil {
		logx.Errorf("loader.SaveMeta: marshal: %v", err)
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		logx.Errorf("loader.SaveMeta: write %s: %v", tmp, err)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		logx.Errorf("loader.SaveMeta: rename %s -> %s: %v", tmp, path, err)
		return err
	}
	return nil
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

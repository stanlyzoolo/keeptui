package loader

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// useTempConfigDir points MetaPath at a per-test directory and restores the
// override on cleanup.
func useTempConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := testConfigDir
	testConfigDir = dir
	t.Cleanup(func() { testConfigDir = orig })
	return dir
}

func TestLoadMetaMissingFile(t *testing.T) {
	useTempConfigDir(t)

	meta, err := LoadMeta()
	if err != nil {
		t.Fatalf("LoadMeta on missing file: %v", err)
	}
	if len(meta) != 0 {
		t.Errorf("LoadMeta on missing file = %v, want empty slice", meta)
	}
}

func TestLoadMetaMalformedYAML(t *testing.T) {
	dir := useTempConfigDir(t)

	path := filepath.Join(dir, "keys", "meta.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not yaml: ["), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadMeta(); err == nil {
		t.Error("LoadMeta on malformed YAML: want error, got nil")
	}
}

func TestSaveMetaLoadMetaRoundTrip(t *testing.T) {
	useTempConfigDir(t)

	want := []ToolMeta{
		{
			Name:   "ripgrep",
			Status: StatusActive,
			Added:  "2026-01-15",
			Tags:   []string{"search", "cli"},
			Note:   "fast grep",
			GitHub: "github.com/BurntSushi/ripgrep",
		},
		{Name: "jq", Status: StatusTrying, Added: "2026-02-01"},
	}

	// SaveMeta must create the config directory itself.
	if err := SaveMeta(want); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	got, err := LoadMeta()
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

func TestSaveMetaLeavesNoTempFile(t *testing.T) {
	dir := useTempConfigDir(t)

	if err := SaveMeta([]ToolMeta{{Name: "a", Status: StatusActive}}); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "keys"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "meta.yaml" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("config dir contains %v, want only meta.yaml", names)
	}
}

func TestSaveMetaReplacesExistingFile(t *testing.T) {
	useTempConfigDir(t)

	if err := SaveMeta([]ToolMeta{{Name: "old1"}, {Name: "old2"}}); err != nil {
		t.Fatalf("first SaveMeta: %v", err)
	}
	if err := SaveMeta([]ToolMeta{{Name: "new"}}); err != nil {
		t.Fatalf("second SaveMeta: %v", err)
	}

	got, err := LoadMeta()
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if len(got) != 1 || got[0].Name != "new" {
		t.Errorf("after overwrite LoadMeta = %+v, want single entry new (no partial merge)", got)
	}
}

func TestFindMeta(t *testing.T) {
	meta := []ToolMeta{{Name: "a"}, {Name: "b"}}

	if got := FindMeta(meta, "b"); got == nil || got.Name != "b" {
		t.Errorf("FindMeta(b) = %v, want entry b", got)
	}
	if got := FindMeta(meta, "missing"); got != nil {
		t.Errorf("FindMeta(missing) = %v, want nil", got)
	}
	// The pointer must alias the slice entry so callers can mutate in place.
	FindMeta(meta, "a").Note = "changed"
	if meta[0].Note != "changed" {
		t.Error("FindMeta result does not alias the slice entry")
	}
}

func TestUpsertMeta(t *testing.T) {
	meta := []ToolMeta{{Name: "a", Note: "old"}}

	meta = UpsertMeta(meta, ToolMeta{Name: "a", Note: "new"})
	if len(meta) != 1 || meta[0].Note != "new" {
		t.Errorf("update in place: got %+v, want single entry with new note", meta)
	}

	meta = UpsertMeta(meta, ToolMeta{Name: "b"})
	if len(meta) != 2 || meta[1].Name != "b" {
		t.Errorf("append: got %+v, want a then b", meta)
	}
}

func TestRemoveMeta(t *testing.T) {
	tests := []struct {
		name   string
		meta   []ToolMeta
		remove string
		want   []string
	}{
		{"present", []ToolMeta{{Name: "a"}, {Name: "b"}, {Name: "c"}}, "b", []string{"a", "c"}},
		{"absent", []ToolMeta{{Name: "a"}}, "x", []string{"a"}},
		{"empty", []ToolMeta{}, "a", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RemoveMeta(tt.meta, tt.remove)
			names := make([]string, 0, len(got))
			for _, m := range got {
				names = append(names, m.Name)
			}
			if !reflect.DeepEqual(names, tt.want) {
				t.Errorf("RemoveMeta(%q) = %v, want %v", tt.remove, names, tt.want)
			}
		})
	}
}

func TestNextStatus(t *testing.T) {
	tests := []struct {
		in, want Status
	}{
		{StatusActive, StatusTrying},
		{StatusTrying, StatusForgotten},
		{StatusForgotten, StatusArchived},
		{StatusArchived, StatusActive}, // cycle wraps
		{Status("bogus"), StatusActive},
		{Status(""), StatusActive},
	}
	for _, tt := range tests {
		if got := NextStatus(tt.in); got != tt.want {
			t.Errorf("NextStatus(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

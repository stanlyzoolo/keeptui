package loader

import "testing"

func TestToolsFromMetaCarriesUpdateCmd(t *testing.T) {
	meta := []ToolMeta{
		{Name: "ripgrep", GitHub: "github.com/BurntSushi/ripgrep", UpdateCmd: "brew upgrade ripgrep"},
		{Name: "jq"},
	}

	tools := ToolsFromMeta(meta)
	if len(tools) != 2 {
		t.Fatalf("ToolsFromMeta returned %d tools, want 2", len(tools))
	}
	if tools[0].UpdateCmd != "brew upgrade ripgrep" {
		t.Errorf("tools[0].UpdateCmd = %q, want %q", tools[0].UpdateCmd, "brew upgrade ripgrep")
	}
	if tools[0].GitHub != "github.com/BurntSushi/ripgrep" {
		t.Errorf("tools[0].GitHub = %q, want the meta value", tools[0].GitHub)
	}
	if tools[0].Source != "meta" {
		t.Errorf("tools[0].Source = %q, want meta", tools[0].Source)
	}
	if tools[1].UpdateCmd != "" {
		t.Errorf("tools[1].UpdateCmd = %q, want empty", tools[1].UpdateCmd)
	}
}

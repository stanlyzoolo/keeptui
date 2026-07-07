package model

import (
	"errors"
	"reflect"
	"testing"
)

func TestBrowserCommand(t *testing.T) {
	const url = "https://github.com/owner/repo"
	tests := []struct {
		goos     string
		wantName string
		wantArgs []string
	}{
		{"darwin", "open", []string{url}},
		{"windows", "rundll32", []string{"url.dll,FileProtocolHandler", url}},
		{"linux", "xdg-open", []string{url}},
		{"freebsd", "xdg-open", []string{url}},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			name, args := browserCommand(tt.goos, url)
			if name != tt.wantName {
				t.Errorf("browserCommand(%q) name = %q, want %q", tt.goos, name, tt.wantName)
			}
			if !reflect.DeepEqual(args, tt.wantArgs) {
				t.Errorf("browserCommand(%q) args = %v, want %v", tt.goos, args, tt.wantArgs)
			}
		})
	}
}

func TestUpdateOpenURLMsg(t *testing.T) {
	t.Run("error sets statusMsg", func(t *testing.T) {
		m := New(nil)
		updated, _ := m.Update(openURLMsg{err: errors.New("boom")})
		got := updated.(Model).statusMsg
		if got != "boom" {
			t.Errorf("statusMsg = %q, want %q", got, "boom")
		}
	})

	t.Run("success leaves statusMsg empty", func(t *testing.T) {
		m := New(nil)
		updated, _ := m.Update(openURLMsg{err: nil})
		if got := updated.(Model).statusMsg; got != "" {
			t.Errorf("statusMsg = %q, want empty", got)
		}
	})
}

// openURLCmd must return a non-nil command; we deliberately do not invoke it
// here because doing so would launch a real browser process.
func TestOpenURLCmdNonNil(t *testing.T) {
	cmd := openURLCmd("https://example.com")
	if cmd == nil {
		t.Fatal("openURLCmd returned nil")
	}
}

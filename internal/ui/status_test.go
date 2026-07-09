package ui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/lepeshko/keys/internal/loader"
)

func TestStatusStyle(t *testing.T) {
	tests := []struct {
		name   string
		status loader.Status
		want   lipgloss.Style
	}{
		{"active", loader.StatusActive, StatusStyleActive},
		{"trying", loader.StatusTrying, StatusStyleTrying},
		{"inactive", loader.StatusInactive, StatusStyleInactive},
		{"unknown falls back to trying", loader.Status("bogus"), StatusStyleTrying},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StatusStyle(tt.status)
			if got.Render("x") != tt.want.Render("x") {
				t.Errorf("StatusStyle(%q) renders %q, want %q", tt.status, got.Render("x"), tt.want.Render("x"))
			}
		})
	}
}

func TestStatusStyleInactiveIsMuted(t *testing.T) {
	if StatusColorInactive != ColorMuted {
		t.Errorf("StatusColorInactive = %v, want ColorMuted (%v)", StatusColorInactive, ColorMuted)
	}
}

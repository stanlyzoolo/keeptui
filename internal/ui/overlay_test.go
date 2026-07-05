package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestStripANSI verifies escape sequences are removed while visible text and
// non-SGR content survive.
func TestStripANSI(t *testing.T) {
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("hello")
	if got := StripANSI(styled); got != "hello" {
		t.Errorf("StripANSI(styled) = %q, want %q", got, "hello")
	}
	if got := StripANSI("plain"); got != "plain" {
		t.Errorf("StripANSI(plain) = %q, want %q", got, "plain")
	}
}

// TestPlaceOverlayCenters verifies the foreground box is spliced into the middle
// of the background, preserving background cells to the left and right of the
// overlaid span.
func TestPlaceOverlayCenters(t *testing.T) {
	bg := strings.Join([]string{
		"..........",
		"..........",
		"..........",
	}, "\n")
	fg := "XX"

	out := PlaceOverlay(bg, fg)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("line count = %d, want 3", len(lines))
	}
	// fg height 1 over bg height 3 => centered on row 1.
	if lines[0] != ".........." || lines[2] != ".........." {
		t.Errorf("uncovered rows changed: %q / %q", lines[0], lines[2])
	}
	// fg width 2 over bg width 10 => x = 4.
	if lines[1] != "....XX...." {
		t.Errorf("overlaid row = %q, want %q", lines[1], "....XX....")
	}
}

// TestPlaceOverlayPreservesRowWidth verifies the overlaid row keeps the same
// visible width as the background row it replaced.
func TestPlaceOverlayPreservesRowWidth(t *testing.T) {
	bg := strings.Repeat("-", 20)
	fg := "MODAL"
	out := PlaceOverlay(bg, fg)
	if w := lipgloss.Width(out); w != 20 {
		t.Errorf("overlaid width = %d, want 20", w)
	}
	if !strings.Contains(out, "MODAL") {
		t.Errorf("overlay content missing from %q", out)
	}
}

// TestPlaceOverlayWiderThanBackground verifies an overlay wider or taller than
// the background does not panic and renders the foreground.
func TestPlaceOverlayWiderThanBackground(t *testing.T) {
	bg := "ab"
	fg := strings.Join([]string{"WIDE-OVERLAY", "SECOND-LINE"}, "\n")
	out := PlaceOverlay(bg, fg)
	if !strings.Contains(out, "WIDE-OVERLAY") {
		t.Errorf("wide overlay content missing from %q", out)
	}
}

// TestDropVisible verifies visible columns are dropped from the left and the
// remaining tail is returned intact.
func TestDropVisible(t *testing.T) {
	if got := dropVisible("abcdef", 2); got != "cdef" {
		t.Errorf("dropVisible(abcdef, 2) = %q, want %q", got, "cdef")
	}
	if got := dropVisible("abc", 5); got != "" {
		t.Errorf("dropVisible(abc, 5) = %q, want empty", got)
	}
}

// TestTruncateVisible verifies truncation to a visible-column budget.
func TestTruncateVisible(t *testing.T) {
	if got := truncateVisible("abcdef", 3); got != "abc" {
		t.Errorf("truncateVisible(abcdef, 3) = %q, want %q", got, "abc")
	}
}

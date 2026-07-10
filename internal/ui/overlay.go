package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

// OverlayBorder frames a centered modal (e.g. the API-status overlay) so it
// stands out from the panels behind it.
var OverlayBorder = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(ColorPrimary).
	Padding(0, 1)

// OverlayDimStyle repaints the background behind a modal so the overlay is
// the only full-color element on screen.
var OverlayDimStyle = lipgloss.NewStyle().Foreground(ColorDim)

// PlaceOverlay renders fg centered over bg, replacing the background cells it
// covers so the modal reads as floating on top. The visible background is
// dimmed: original styling is stripped and repainted with OverlayDimStyle.
// fg is passed through untouched.
func PlaceOverlay(bg, fg string) string {
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")

	bgW, bgH := lipgloss.Width(bg), len(bgLines)
	fgW, fgH := lipgloss.Width(fg), len(fgLines)

	x := max((bgW-fgW)/2, 0)
	y := max((bgH-fgH)/2, 0)

	out := make([]string, len(bgLines))
	for i, line := range bgLines {
		// Covered rows are handled below via overlayLine, which strips the bg
		// styling itself; dimming them here first would be undone by that strip.
		out[i] = dimBG(line)
	}
	for i, fgLine := range fgLines {
		row := y + i
		if row >= len(out) {
			break
		}
		out[row] = overlayLine(bgLines[row], fgLine, x)
	}
	return strings.Join(out, "\n")
}

// overlayLine splices fg into bg starting at visual column x, dimming the
// visible bg margins to the left and right of the overlaid span.
func overlayLine(bg, fg string, x int) string {
	fgW := lipgloss.Width(fg)
	left := truncateVisible(bg, x)
	leftW := lipgloss.Width(left)
	if leftW < x {
		left += strings.Repeat(" ", x-leftW)
	}
	right := dropVisible(bg, x+fgW)
	return dimBG(left) + fg + dimBG(right)
}

// dimBG strips s of its own styling and repaints it with OverlayDimStyle.
func dimBG(s string) string {
	if s == "" {
		return ""
	}
	return OverlayDimStyle.Render(StripANSI(s))
}

// truncateVisible returns the prefix of s spanning the first w visible columns,
// dropping ANSI styling (best-effort; overlay content sits above it anyway).
func truncateVisible(s string, w int) string {
	return runewidth.Truncate(StripANSI(s), w, "")
}

// dropVisible returns s with its first w visible columns removed.
func dropVisible(s string, w int) string {
	plain := StripANSI(s)
	total := runewidth.StringWidth(plain)
	if w >= total {
		return ""
	}
	// Keep the tail: total-w visible columns from the right.
	keep := total - w
	var b strings.Builder
	width := 0
	runes := []rune(plain)
	// Walk from the end accumulating `keep` columns.
	start := len(runes)
	for i := len(runes) - 1; i >= 0; i-- {
		rw := runewidth.RuneWidth(runes[i])
		if width+rw > keep {
			break
		}
		width += rw
		start = i
	}
	b.WriteString(string(runes[start:]))
	return b.String()
}

// StripANSI removes ANSI escape sequences from s. It is the single ANSI-strip
// helper shared across packages (the model layer delegates to it). It must
// cover the full escape grammar, not just SGR: captured tool output can carry
// private-mode CSI (ESC[?1049l — leave alternate screen), OSC, DCS, … which a
// simple [0-9;]* regex misses; anything left unstripped is re-emitted by the
// renderer and flips the real terminal's state mid-frame.
func StripANSI(s string) string {
	return ansi.Strip(s)
}

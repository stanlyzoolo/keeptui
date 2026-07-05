package ui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// OverlayBorder frames a centered modal (e.g. the API-status overlay) so it
// stands out from the panels behind it.
var OverlayBorder = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(ColorPrimary).
	Padding(0, 1)

// PlaceOverlay renders fg centered over bg, replacing the background cells it
// covers so the modal reads as floating on top. Both are treated as plain,
// already-styled multi-line strings; ANSI escape sequences in bg lines that
// the overlay covers are dropped for the covered span only.
func PlaceOverlay(bg, fg string) string {
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")

	bgW, bgH := lipgloss.Width(bg), len(bgLines)
	fgW, fgH := lipgloss.Width(fg), len(fgLines)

	x := max((bgW-fgW)/2, 0)
	y := max((bgH-fgH)/2, 0)

	out := make([]string, len(bgLines))
	copy(out, bgLines)
	for i, fgLine := range fgLines {
		row := y + i
		if row >= len(out) {
			break
		}
		out[row] = overlayLine(out[row], fgLine, x)
	}
	return strings.Join(out, "\n")
}

// overlayLine splices fg into bg starting at visual column x, preserving the
// visible bg to the left and right of the overlaid span.
func overlayLine(bg, fg string, x int) string {
	fgW := lipgloss.Width(fg)
	left := truncateVisible(bg, x)
	leftW := lipgloss.Width(left)
	if leftW < x {
		left += strings.Repeat(" ", x-leftW)
	}
	right := dropVisible(bg, x+fgW)
	return left + fg + right
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

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// StripANSI removes ANSI escape sequences from s. It is the single ANSI-strip
// helper shared across packages (the model layer delegates to it).
func StripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

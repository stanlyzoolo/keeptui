package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Overlay renders popup on top of base at position (x, y) in visual columns/rows.
// Background content remains visible around the popup.
func Overlay(base, popup string, x, y int) string {
	baseLines := strings.Split(base, "\n")
	popupLines := strings.Split(popup, "\n")

	for r, pLine := range popupLines {
		targetRow := y + r
		for len(baseLines) <= targetRow {
			baseLines = append(baseLines, "")
		}

		bLine := baseLines[targetRow]
		bWidth := ansi.StringWidth(bLine)
		pWidth := ansi.StringWidth(pLine)

		// Pad base line if shorter than popup x offset.
		if bWidth < x {
			bLine += strings.Repeat(" ", x-bWidth)
			bWidth = x
		}

		left := ansi.Cut(bLine, 0, x)
		right := ""
		if bWidth > x+pWidth {
			right = ansi.Cut(bLine, x+pWidth, bWidth)
		}

		baseLines[targetRow] = left + pLine + right
	}

	return strings.Join(baseLines, "\n")
}

// PlaceOverlay centers popup over base within the given terminal dimensions.
func PlaceOverlay(totalW, totalH int, base, popup string) string {
	popupLines := strings.Split(popup, "\n")
	popupH := len(popupLines)
	popupW := 0
	for _, l := range popupLines {
		if w := ansi.StringWidth(l); w > popupW {
			popupW = w
		}
	}

	x := (totalW - popupW) / 2
	y := (totalH - popupH) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	return Overlay(base, popup, x, y)
}

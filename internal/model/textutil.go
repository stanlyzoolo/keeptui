package model

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"

	"github.com/lepeshko/keys/internal/ui"
)

// formatStars formats a star count with K suffix for thousands.
func formatStars(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

// languagePercent holds a language name and its percentage share.
type languagePercent struct {
	Name string
	Pct  float64
}

// languagePercents converts raw byte counts to sorted percentage slice (top 5).
func languagePercents(langs map[string]int) []languagePercent {
	if len(langs) == 0 {
		return nil
	}
	total := 0
	for _, v := range langs {
		total += v
	}
	if total == 0 {
		return nil
	}
	out := make([]languagePercent, 0, len(langs))
	for name, bytes := range langs {
		out = append(out, languagePercent{Name: name, Pct: float64(bytes) / float64(total) * 100})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Pct > out[j].Pct })
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

// renderLangBar renders a horizontal language bar with percentages, wrapping by
// words at width. firstLineUsed is the column budget already consumed on the
// first line (e.g. by an inline "languages: " label) so wrapping lines up.
func renderLangBar(langs map[string]int, width, firstLineUsed int) string {
	percents := languagePercents(langs)
	if len(percents) == 0 {
		return ""
	}
	// Language names lowercase in the normal note color; percentages dimmed.
	pctStyle := lipgloss.NewStyle().Foreground(ui.ColorDim)

	var lines []string
	var cur strings.Builder
	curW := firstLineUsed
	for _, lp := range percents {
		name := strings.ToLower(lp.Name)
		pct := fmt.Sprintf("%.0f%%", lp.Pct)
		tokenW := utf8.RuneCountInString(name) + 1 + utf8.RuneCountInString(pct)

		sep := 0
		if cur.Len() > 0 {
			sep = 2
		}
		// Wrap to a new line only when the token would overflow the width.
		if curW+sep+tokenW > width && cur.Len() > 0 {
			lines = append(lines, cur.String())
			cur.Reset()
			curW = 0
			sep = 0
		}
		if sep > 0 {
			cur.WriteString("  ")
		}
		cur.WriteString(ui.InfoStyle.Render(name) + " " + pctStyle.Render(pct))
		curW += sep + tokenW
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	return strings.Join(lines, "\n")
}

func wrapText(s string, width int) string {
	if width <= 0 {
		return s
	}
	var result strings.Builder
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if i > 0 {
			result.WriteByte('\n')
		}
		if utf8.RuneCountInString(line) <= width {
			result.WriteString(line)
			continue
		}
		words := strings.Fields(line)
		col := 0
		for j, word := range words {
			wl := utf8.RuneCountInString(word)
			if j == 0 {
				result.WriteString(word)
				col = wl
			} else if col+1+wl > width {
				result.WriteByte('\n')
				result.WriteString(word)
				col = wl
			} else {
				result.WriteByte(' ')
				result.WriteString(word)
				col += 1 + wl
			}
		}
	}
	return result.String()
}

func stripMarkdown(s string) string {
	var sb strings.Builder
	lines := strings.Split(s, "\n")
	blankCount := 0

	for _, line := range lines {
		line = strings.TrimLeft(line, "#")
		line = strings.TrimSpace(line)

		for _, marker := range []string{"**", "__"} {
			line = strings.ReplaceAll(line, marker, "")
		}
		line = strings.Trim(line, "*_")
		line = strings.ReplaceAll(line, "`", "")

		for strings.Contains(line, "<") && strings.Contains(line, ">") {
			start := strings.Index(line, "<")
			end := strings.Index(line[start:], ">")
			if end < 0 {
				break
			}
			line = line[:start] + line[start+end+1:]
		}

		line = strings.TrimSpace(line)

		if line == "" {
			blankCount++
			if blankCount <= 1 {
				sb.WriteString("\n")
			}
		} else {
			blankCount = 0
			sb.WriteString(line + "\n")
		}
	}
	return strings.TrimSpace(sb.String())
}

func stripANSI(s string) string {
	return ui.StripANSI(s)
}

// cleanTerminalOutput strips ANSI escapes, carriage returns, and backspace
// overstrike (man pages render bold/underline as "x\bx"/"_\bx"). Leaving the
// backspaces in makes lipgloss miscount widths and overflow the panel.
func cleanTerminalOutput(s string) string {
	s = stripANSI(s)
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch r {
		case '\r':
			// drop
		case '\b':
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
		default:
			out = append(out, r)
		}
	}
	return string(out)
}

// helpTokenRe matches every highlightable token (flag, <meta>, [meta]) in one
// alternation so colorizeHelp can style them in a single pass over the original
// line. Scanning once is essential: styled tokens embed ANSI escapes that
// contain '[', so a second regex pass (e.g. the bracket pattern) would match
// inside those escapes and corrupt them into visible "[38;2;…m" garbage.
//   - group 1: word boundary before a flag (line start, whitespace, or '(')
//   - group 2: the flag itself; a boundary is required so a dash inside a word
//     like "golangci-lint" is not mistaken for a short flag
//   - group 3: <angle> meta token
//   - group 4: [bracket] meta token
var helpTokenRe = regexp.MustCompile(`(^|[\s(])(--?[a-zA-Z][a-zA-Z0-9\-_]*)|(<[^>]+>)|(\[[^\]]+\])`)

// stylePrefix returns the raw ANSI prefix a lipgloss style emits, so base text
// color can be re-asserted after nested styled tokens reset it.
func stylePrefix(s lipgloss.Style) string {
	r := s.Render("\x00")
	if pre, _, ok := strings.Cut(r, "\x00"); ok {
		return pre
	}
	return ""
}

func colorizeHelp(s string) string {
	base := stylePrefix(ui.InfoStyle)
	const reset = "\x1b[0m"
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		trimmed := strings.TrimRight(line, " ")
		if trimmed != "" && trimmed[0] != ' ' && trimmed[0] != '\t' && strings.HasSuffix(trimmed, ":") {
			lines[i] = ui.HelpSectionStyle.Render(line)
			continue
		}
		if line == "" {
			continue
		}
		// Single pass over the ORIGINAL line. Each styled token re-asserts the
		// base color after it so the rest of the line stays the unified content
		// color (matching the changelog body). We never re-scan styled output,
		// so the '[' inside injected escapes can't be mis-matched as meta.
		var b strings.Builder
		last := 0
		for _, m := range helpTokenRe.FindAllStringSubmatchIndex(line, -1) {
			b.WriteString(line[last:m[0]])
			switch {
			case m[4] >= 0: // flag: group 1 = boundary, group 2 = flag text
				b.WriteString(line[m[2]:m[3]])
				b.WriteString(ui.HelpFlagStyle.Render(line[m[4]:m[5]]))
			case m[6] >= 0: // <angle> meta
				b.WriteString(ui.HelpMetaStyle.Render(line[m[6]:m[7]]))
			case m[8] >= 0: // [bracket] meta
				b.WriteString(ui.HelpMetaStyle.Render(line[m[8]:m[9]]))
			}
			b.WriteString(base)
			last = m[1]
		}
		b.WriteString(line[last:])
		lines[i] = base + b.String() + reset
	}
	return strings.Join(lines, "\n")
}

func findMatches(text, query string) []int {
	if query == "" {
		return nil
	}
	lq := strings.ToLower(query)
	var matches []int
	for i, line := range strings.Split(strings.ToLower(text), "\n") {
		if strings.Contains(line, lq) {
			matches = append(matches, i)
		}
	}
	return matches
}

func highlightMatch(line, query string) string {
	if query == "" {
		return line
	}
	lr := []rune(line)
	qr := []rune(strings.ToLower(query))
	idx := runeIndexFold(lr, qr)
	if idx < 0 {
		return line
	}
	end := idx + len(qr)
	return string(lr[:idx]) + ui.SearchMatchStyle.Render(string(lr[idx:end])) + string(lr[end:])
}

// runeIndexFold returns the rune index of the first occurrence of query
// (already lowercase) in line, comparing case-insensitively rune by rune.
// Working in runes keeps the offsets valid for slicing line itself:
// strings.Index over strings.ToLower(line) yields byte offsets into the
// lowered string, which drift — and can slice out of range — when
// lowercasing changes a rune's UTF-8 length (e.g. Ⱥ U+023A is 2 bytes, its
// lowercase ⱥ U+2C65 is 3).
func runeIndexFold(line, query []rune) int {
	if len(query) == 0 {
		return -1
	}
	for i := 0; i+len(query) <= len(line); i++ {
		hit := true
		for j, q := range query {
			if unicode.ToLower(line[i+j]) != q {
				hit = false
				break
			}
		}
		if hit {
			return i
		}
	}
	return -1
}

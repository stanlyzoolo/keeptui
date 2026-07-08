package model

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lepeshko/keys/internal/loader"
	"github.com/lepeshko/keys/internal/ui"
	"github.com/lepeshko/keys/internal/version"
)

func (m Model) View() string {
	if !m.ready {
		return "Loading..."
	}

	left := m.renderTools()
	middle := m.renderBrief()
	right := m.renderHelp()
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, middle, right)
	layout := lipgloss.JoinVertical(lipgloss.Left, body, m.renderStatusBar())
	if m.apiOverlayVisible() {
		layout = ui.PlaceOverlay(layout, m.renderAPIStatus())
	}
	// Vertical margin only; no horizontal margin so panels/status bar reach the
	// terminal edges.
	return lipgloss.NewStyle().Margin(1, 0).Render(layout)
}

func (m Model) renderStatusBar() string {
	style := ui.HelpStyle.Width(m.width - 2)
	if m.mode == modeHelpSearch {
		matchInfo := ""
		if len(m.helpMatches) > 0 {
			matchInfo = fmt.Sprintf("  %d/%d matches", m.helpMatchIdx+1, len(m.helpMatches))
		} else if m.helpSearch.Value() != "" {
			matchInfo = "  no matches"
		}
		return style.Render(fmt.Sprintf(
			"%s %s  %s next  %s prev  %s exit%s",
			ui.SearchPromptStyle.Render("/"),
			m.helpSearch.View(),
			keyHint("n"),
			keyHint("N"),
			keyHint("esc"),
			matchInfo,
		))
	}
	if m.mode == modeSearch {
		return style.Render(fmt.Sprintf(
			"%s %s  %s open  %s move  %s cancel",
			ui.SearchPromptStyle.Render("/"),
			m.search.View(),
			keyHint("enter"),
			keyHint("↑/↓"),
			keyHint("esc"),
		))
	}
	if m.mode == modeEditNote {
		return style.Render(keyHint("enter") + " save  " + keyHint("esc") + " cancel")
	}
	if m.mode == modeEditTags {
		return style.Render(keyHint("enter") + " save  " + keyHint("esc") + " cancel  " + ui.MetaNoteStyle.Render("comma-separated"))
	}
	if m.mode == modeTrack {
		return style.Render(fmt.Sprintf(
			"%s %s  %s cancel",
			ui.SearchPromptStyle.Render("track (github url or tool name):"),
			m.trackInput.View(),
			keyHint("esc"),
		))
	}
	if m.mode == modeConfirmUntrack {
		return style.Render(fmt.Sprintf(
			"%s  %s yes  %s no",
			ui.SearchPromptStyle.Render("Untrack "+m.untrackTarget+"?"),
			keyHint("enter"),
			keyHint("esc"),
		))
	}
	if m.mode == modeRename {
		return style.Render(fmt.Sprintf(
			"%s %s  %s cancel",
			ui.SearchPromptStyle.Render("rename to:"),
			m.nameInput.View(),
			keyHint("esc"),
		))
	}
	if m.mode == modeTokenInput {
		return style.Render(keyHint("enter") + " validate & save  " + keyHint("esc") + " cancel")
	}
	if m.mode == modeAPIStatus {
		return style.Render(keyHint("r") + " refresh  " + keyHint("esc") + " close")
	}
	if m.statusMsg != "" {
		return style.Render(ui.SearchPromptStyle.Render(m.statusMsg))
	}
	if m.focus == focusBrief {
		hints := keyHint("o") + " open repo  " + keyHint("c") + " changelog  " + keyHint("r") + " refresh  " + keyHint("s") + " status  " + keyHint("e") + " note  " + keyHint("t") + " tags  " + keyHint("q") + " quit"
		return m.renderHintsBar(style, hints)
	}
	if m.focus == focusHelp {
		hints := keyHint("↑↓") + " scroll  " + keyHint("h") + " --help  " + keyHint("m") + " man  " + keyHint("/") + " search  " + keyHint("←") + " back  " + keyHint("q") + " quit"
		return m.renderHintsBar(style, hints)
	}
	return m.renderHintsBar(style,
		keyHint("/")+" search  "+
			keyHint("t")+" track  "+
			keyHint("u")+" untrack  "+
			keyHint("r")+" rename  "+
			keyHint("q")+" quit",
	)
}

// rateGaugeMinGap is the minimum blank columns between the hint bar and the
// right-aligned API-usage gauge; below it the gauge is downgraded or dropped.
const rateGaugeMinGap = 2

// renderHintsBar lays out the left-aligned hints with the API-usage gauge pinned
// to the right corner. It downgrades full → compact → hidden as terminal width
// shrinks so the hints are never truncated. inner is HelpStyle's content width
// (m.width-2, the border sits outside it).
func (m Model) renderHintsBar(style lipgloss.Style, hints string) string {
	inner := m.width - 2
	place := func(gauge string) (string, bool) {
		if gauge == "" {
			return "", false
		}
		gap := inner - lipgloss.Width(hints) - lipgloss.Width(gauge)
		if gap < rateGaugeMinGap {
			return "", false
		}
		return hints + strings.Repeat(" ", gap) + gauge, true
	}
	if line, ok := place(m.renderRateGauge(false)); ok {
		return style.Render(line)
	}
	if line, ok := place(m.renderRateGauge(true)); ok {
		return style.Render(line)
	}
	return style.Render(hints)
}

func keyHint(k string) string {
	return ui.SearchPromptStyle.Render("[" + k + "]")
}

// gaugeCells is the fixed width of the API-usage bar, independent of whether the
// limit is 60 (no token) or 5000 (with token) — the fill tracks the used ratio,
// not an absolute count.
const gaugeCells = 12

// gaugeFillGlyph / gaugeTrackGlyph draw the bar. Both must stay width-stable:
// neither is East-Asian-Ambiguous, so lipgloss.Width counts them as one cell
// even under RUNEWIDTH_EASTASIAN=1. A full block (█ U+2588) is Ambiguous and
// would measure as two cells there, inflating the gap math in renderHintsBar
// and wrongly downgrading or mis-padding the bar.
const (
	gaugeFillGlyph  = "▮" // U+25AE black vertical rectangle
	gaugeTrackGlyph = "░" // U+2591 light shade
)

// renderRateGauge builds the right-corner "GitHub API Usage" indicator for the
// current rate snapshot, or "" when there is no known snapshot. It shows
// used/limit (used = Limit-Remaining), matching the API-status overlay. The bar
// is ▮ fill / ░ track glyphs (visible even if ANSI is stripped; any usage shows
// at least one filled cell via gaugeFilled) and constant yellow at every
// pressure level — exhaustion (used==limit) simply renders a full bar; the ⚠/✕
// alarm lives only in the [L] overlay. compact drops the label and bar, keeping
// "GH used/limit [L]" for narrow terminals.
func (m Model) renderRateGauge(compact bool) string {
	r := m.rate
	if !r.Known || r.Limit <= 0 {
		return ""
	}
	used := usedOf(r)
	nums := ui.RateUsageNumStyle.Render(fmt.Sprintf("%d/%d", used, r.Limit))
	if compact {
		return ui.GithubStyle.Render("GH ") + nums + " " + keyHint("L")
	}
	filled := gaugeFilled(used, r.Limit)
	bar := ui.RateGaugeFillStyle.Render(strings.Repeat(gaugeFillGlyph, filled)) +
		ui.RateGaugeTrackStyle.Render(strings.Repeat(gaugeTrackGlyph, gaugeCells-filled))
	return ui.GithubStyle.Render("GitHub API Usage ") +
		ui.RateBracketStyle.Render("[") + bar + ui.RateBracketStyle.Render("]") +
		" " + nums + "  " + keyHint("L") + ui.GithubStyle.Render(" details")
}

// usedOf returns consumed requests (Limit-Remaining) clamped to [0,Limit], the
// single source of used/limit for both the status-bar gauge and the [L] overlay.
// GitHub always reports Remaining in [0,Limit]; the clamp is defensive against a
// malformed snapshot.
func usedOf(r version.RateLimit) int {
	used := r.Limit - r.Remaining
	if used < 0 {
		return 0
	}
	if used > r.Limit {
		return r.Limit
	}
	return used
}

// gaugeFilled maps used/limit onto the fixed gaugeCells-wide bar, rounding to the
// nearest cell with integer math (no math import) and clamping to [0,gaugeCells].
// The rounded ratio is then clamped into a truthful range: any usage shows at
// least one cell (with limit 5000 the first cell would otherwise need ~209
// requests), and only exhaustion renders a full bar (used < limit caps at
// gaugeCells-1). Outside those edge bands the fill tracks the used ratio only,
// so limit 60 and limit 5000 fill identically at the same percentage.
func gaugeFilled(used, limit int) int {
	if limit <= 0 || used <= 0 {
		return 0
	}
	filled := (used*gaugeCells + limit/2) / limit
	if filled < 1 {
		filled = 1
	}
	maxFill := gaugeCells
	if used < limit {
		maxFill = gaugeCells - 1
	}
	if filled > maxFill {
		filled = maxFill
	}
	return filled
}

// rateLowThreshold is the number of remaining GitHub API requests at or below
// which the status bar (and API-status overlay) flags rate-limit pressure.
const rateLowThreshold = 10

// rateLevel classifies a rate snapshot's pressure so the status-bar signal and
// the overlay icon share one decision. It is the single source of truth for the
// none/warn/exhausted thresholds.
type rateLevel int

func classifyRate(r version.RateLimit) rateLevel {
	if !r.Known {
		return rateUnknown
	}
	switch {
	case r.Remaining == 0:
		return rateExhausted
	case r.Remaining <= rateLowThreshold:
		return rateWarn
	default:
		return rateOK
	}
}

// rateIcon returns the styled indicator (none / ⚠ / ✕) for a rate snapshot,
// sharing classifyRate with the status-bar signal so the overlay and the bar
// never disagree. Returns "" when nothing should flag.
func rateIcon(rate version.RateLimit) string {
	switch classifyRate(rate) {
	case rateExhausted:
		return ui.DangerStyle.Render("✕")
	case rateWarn:
		return ui.WarnStyle.Render("⚠")
	default:
		return ""
	}
}

// maskToken renders a token as its first 4 and last 4 characters joined by
// bullets (e.g. "ghp_••••••••3f2a"), never exposing the middle. Short tokens
// are fully masked.
func maskToken(t string) string {
	if len(t) <= 8 {
		return strings.Repeat("•", len(t))
	}
	return t[:4] + strings.Repeat("•", 8) + t[len(t)-4:]
}

// renderAPIStatus builds the API-status overlay body: an optional add-token
// nudge (when none is configured), the token source (masked), used/limit with
// the shared icon, and the reset time.
func (m Model) renderAPIStatus() string {
	var b strings.Builder
	b.WriteString(ui.SectionLabelStyle.Render("GitHub API status") + "\n\n")

	source := version.TokenSource()
	// Nudge the user to add a token when none is configured — it lifts the hourly
	// limit from 60 to 5000. Hidden once a token exists or while entering one.
	if source == "none" && m.mode != modeTokenInput {
		b.WriteString(ui.WarnStyle.Render("Add a GitHub token to raise the limit (60 → 5000/h)  ") + keyHint("e") + "\n\n")
	}

	tokenLine := "Token: " + source
	if tok := version.Token(); tok != "" {
		tokenLine += " (" + maskToken(tok) + ")"
	}
	b.WriteString(ui.InfoStyle.Render(tokenLine) + "\n")

	if m.rate.Known {
		icon := rateIcon(m.rate)
		// Used/limit matches the status-bar gauge so the two surfaces agree.
		usedLine := fmt.Sprintf("Used: %d / %d", usedOf(m.rate), m.rate.Limit)
		if icon != "" {
			usedLine = icon + " " + usedLine
		}
		b.WriteString(ui.InfoStyle.Render(usedLine) + "\n")
		if !m.rate.Reset.IsZero() {
			mins := int(time.Until(m.rate.Reset).Minutes())
			if mins < 0 {
				mins = 0
			}
			b.WriteString(ui.InfoStyle.Render(fmt.Sprintf(
				"Reset: in %d min (%s)", mins, m.rate.Reset.Format("15:04"))) + "\n")
		}
	} else {
		b.WriteString(ui.InfoStyle.Render("Limit: unknown") + "\n")
	}

	if m.mode == modeTokenInput {
		b.WriteString("\n" + ui.SearchPromptStyle.Render("token: ") + m.tokenInput.View() + "\n")
	}
	if m.tokenError != "" {
		b.WriteString(ui.DangerStyle.Render(m.tokenError) + "\n")
	}

	b.WriteString("\n")
	var hints string
	if m.mode == modeTokenInput {
		hints = keyHint("enter") + " validate & save  " + keyHint("esc") + " cancel"
	} else {
		hints = keyHint("e") + " set token  "
		if source == "config" {
			hints += keyHint("d") + " remove token  "
		}
		hints += keyHint("r") + " refresh  " + keyHint("esc") + " close"
	}
	b.WriteString(ui.InfoStyle.Render(hints))

	return ui.OverlayBorder.Render(b.String())
}

func (m Model) calcVpHeight() int {
	// Match the panel's inner content height. lipgloss adds borders outside the
	// configured Height, so Height(m.height-7) gives exactly m.height-7 content
	// rows; the viewport must fill them so the scrollbar reaches the bottom.
	return max(m.height-7, 1)
}

func (m Model) calcPanelWidths() (toolsW, briefW, helpW int) {
	// 20%-40%-40% layout. lipgloss adds borders OUTSIDE the configured Width,
	// so Width(panelW) renders as panelW+2 on screen, and panel content fills
	// the full panelW (dividers/viewports use panelW, not panelW-2).
	// Horizontal overhead reserved here = 6: 2 border cols x 3 panels. There is
	// no outer horizontal margin and panels sit flush against each other.
	available := max(m.width-6, 1)
	toolsW = max((available*20)/100, 15)
	briefW = max((available*40)/100, 30)
	helpW = available - toolsW - briefW
	if helpW < 30 {
		helpW = 30
		briefW = available - toolsW - helpW
		if briefW < 30 {
			briefW = 30
			toolsW = available - briefW - helpW
			// Ensure toolsW doesn't go negative on very small terminals
			if toolsW < 1 {
				toolsW = 1
				// Reduce other panels proportionally
				briefW = max((available-toolsW-5)/2, 1)
				helpW = available - toolsW - briefW
			}
		}
	}
	return
}

func (m Model) renderLeftContent() string {
	var sb strings.Builder
	filtered := m.filteredMeta()
	maxName := m.toolsW - 5

	for i, mt := range filtered {
		name := wrapText(mt.Name, maxName)
		name = strings.TrimRight(name, "\n")

		hasUpdate := m.hasUpdate(mt.Name)
		updateMark := ""
		if hasUpdate {
			updateMark = " " + ui.UpdateAvailableStyle.Render("↑")
		}

		// The marker stays visible in modeSearch: the cursor there is
		// user-controlled (arrows move the highlight through the matches).
		isSelected := i == m.metaSelected && m.focus == focusTools
		if isSelected {
			circle := ui.SelectionBarStyle.Render("●")
			sb.WriteString(circle + " " + name + updateMark + "\n")
		} else {
			sb.WriteString("  " + name + updateMark + "\n")
		}
	}

	if len(filtered) == 0 {
		if m.mode == modeSearch {
			sb.WriteString(ui.DescStyle.Render("  No matches.") + "\n")
		} else {
			sb.WriteString(ui.DescStyle.Render("  No tools tracked.\n  Press t to add one.") + "\n")
		}
	}

	return sb.String()
}

// syncToolsViewport adjusts YOffset so that metaSelected is visible.
func (m *Model) syncToolsViewport() {
	vpH := m.toolsViewport.Height
	if vpH <= 0 {
		return
	}
	if m.metaSelected < m.toolsViewport.YOffset {
		m.toolsViewport.SetYOffset(m.metaSelected)
	} else if m.metaSelected >= m.toolsViewport.YOffset+vpH {
		m.toolsViewport.SetYOffset(m.metaSelected - vpH + 1)
	}
}

// setToolsContent refreshes viewport content and syncs scroll position.
func (m *Model) setToolsContent() {
	m.toolsViewport.SetContent(m.renderLeftContent())
	m.syncToolsViewport()
}

func (m Model) renderTools() string {
	panelStyle := ui.PanelBorder
	if m.focus == focusTools {
		panelStyle = ui.PanelBorderFocused
	}

	return panelStyle.
		Width(m.toolsW).
		Height(max(m.height-7, 1)).
		Render(withScrollbar(m.toolsViewport, m.toolsW, m.focus == focusTools))
}

func (m Model) renderBrief() string {
	panelStyle := ui.PanelBorder
	if m.focus == focusBrief {
		panelStyle = ui.PanelBorderFocused
	}

	return panelStyle.
		Width(m.briefW).
		Height(max(m.height-7, 1)).
		Render(withScrollbar(m.briefViewport, m.briefW, m.focus == focusBrief))
}

func (m Model) renderHelp() string {
	panelStyle := ui.PanelBorder
	if m.focus == focusHelp {
		panelStyle = ui.PanelBorderFocused
	}

	return panelStyle.
		Width(m.helpW).
		Height(max(m.height-7, 1)).
		Render(withScrollbar(m.helpViewport, m.helpW, m.focus == focusHelp))
}

// withScrollbar renders a viewport with a 1-col scrollbar gutter on its right
// edge. The gutter stays blank unless the content is taller than the viewport,
// in which case a thumb (no track) is drawn proportional to the scroll position.
// The thumb is peach when the panel is focused, dim otherwise.
func withScrollbar(vp viewport.Model, panelWidth int, focused bool) string {
	left := lipgloss.NewStyle().Width(max(panelWidth-1, 1)).Render(vp.View())
	return lipgloss.JoinHorizontal(lipgloss.Top, left, scrollColumn(vp, focused))
}

func scrollColumn(vp viewport.Model, focused bool) string {
	height := vp.Height
	if height <= 0 {
		return ""
	}
	rows := make([]string, height)
	for i := range rows {
		rows[i] = " "
	}
	total := vp.TotalLineCount()
	if total > height {
		thumbStyle := ui.ScrollThumbDimStyle
		if focused {
			thumbStyle = ui.ScrollThumbStyle
		}
		thumb := max(height*height/total, 1)
		pos := 0
		if maxOff := total - height; maxOff > 0 {
			pos = vp.YOffset * (height - thumb) / maxOff
		}
		for i := pos; i < pos+thumb && i < height; i++ {
			// Right half block: a half-width thumb hugging the panel border.
			rows[i] = thumbStyle.Render("▐")
		}
	}
	return strings.Join(rows, "\n")
}

func (m Model) renderCard() string {
	if len(m.meta) == 0 {
		return ui.DescStyle.Render("no tools tracked.\npress t to add one.")
	}

	t, ok := m.selectedTool()
	if !ok {
		return ui.DescStyle.Render("select a tool from the left panel.")
	}

	inner := max(m.briefW-2, 1)

	var sb strings.Builder

	card, hasCard := m.repoCards[t.Name]

	// Title line: tool name (bold orange) + about (gray italic). Name is always
	// shown; about is appended when available.
	name := t.Name
	maxNameLen := 30
	if utf8.RuneCountInString(name) > maxNameLen {
		name = name[:maxNameLen-3] + "..."
	}
	nameRendered := lipgloss.NewStyle().Bold(true).Foreground(ui.ColorOrange).Render(name)
	var title string
	if m.refreshingFor == t.Name {
		// While a force refresh is in flight, the title line becomes a status
		// line: "refreshing <name> data <spinner>" (name keeps its bold style,
		// spinner frames advance on spinner.TickMsg). The about is hidden until
		// the refreshed card lands.
		title = ui.InfoStyle.Render("refreshing ") + nameRendered + ui.InfoStyle.Render(" data ") + m.spinner.View()
	} else {
		title = nameRendered
		if hasCard && card.About != "" {
			aboutWidth := max(inner-utf8.RuneCountInString(name)-3, 20)
			aboutWrapped := wrapText(card.About, aboutWidth)
			title += " — " + ui.MetaNoteStyle.Render(aboutWrapped)
		}
	}
	sb.WriteString(title + "\n")

	// [info] section: repo / stars / latest / languages / repo status.
	hasInfo := t.GitHub != "" ||
		(hasCard && (card.Stars > 0 || card.Latest != "" || len(card.Languages) > 0 || card.RepoStatus != ""))
	if hasInfo {
		sb.WriteString(m.sectionDivider("info"))
		if t.GitHub != "" {
			sb.WriteString(ui.GithubStyle.Render("repo: "+t.GitHub) + "\n")
			if !hasCard && m.repoStatus[t.Name] == "rate-limited" {
				sb.WriteString(ui.WarnStyle.Render("rate limited — press [L]") + "\n")
			}
		}
		if hasCard {
			if card.Stars > 0 {
				sb.WriteString(ui.InfoStyle.Render(fmt.Sprintf("stars: %s", formatStars(card.Stars))) + "\n")
			}
			if card.Latest != "" {
				line := "latest: " + card.Latest
				if card.PublishedAt != "" {
					date := card.PublishedAt
					if len(date) > 10 {
						date = date[:10]
					}
					line += " (" + date + ")"
				}
				sb.WriteString(ui.InfoStyle.Render(line) + "\n")
			}
			if len(card.Languages) > 0 {
				label := "languages: "
				bar := renderLangBar(card.Languages, inner, utf8.RuneCountInString(label))
				sb.WriteString(ui.InfoStyle.Render(label) + bar + "\n")
			}
			if card.RepoStatus != "" {
				sb.WriteString(ui.InfoStyle.Render("maintenance:") + " " + renderRepoStatus(card.RepoStatus) + "\n")
			}
		}
	}

	// [notes] section: status / note / tags (with inline editing via e/t).
	if mt, ok := m.selectedMeta(); ok {
		sb.WriteString(m.sectionDivider("notes"))
		sym := loader.StatusSymbol[mt.Status]
		symStyled := ui.StatusStyle(mt.Status).Render(sym + " " + string(mt.Status))
		sb.WriteString(ui.MetaDetailLabelStyle.Render("status:") + " " + symStyled + "\n")

		if m.mode == modeEditNote {
			sb.WriteString(ui.MetaDetailLabelStyle.Render("note:") + " " + m.noteInput.View() + "\n")
		} else {
			noteText := mt.Note
			if noteText == "" {
				noteText = "— (press e to edit)"
			}
			wrapped := wrapText(noteText, inner)
			sb.WriteString(ui.MetaDetailLabelStyle.Render("note:") + " " + ui.MetaNoteStyle.Render(wrapped) + "\n")
		}

		if m.mode == modeEditTags {
			sb.WriteString(ui.MetaDetailLabelStyle.Render("tags:") + " " + m.tagsInput.View() + "\n")
		} else {
			tagsText := strings.Join(mt.Tags, ", ")
			if tagsText == "" {
				tagsText = "— (press t to edit)"
			}
			wrapped := wrapText(tagsText, inner)
			sb.WriteString(ui.MetaDetailLabelStyle.Render("tags:") + " " + ui.MetaTagStyle.Render(wrapped) + "\n")
		}
	}

	// [changelog] section (only when there is content to show).
	var changelogContent string
	if m.changelogLoadingFor == t.Name {
		changelogContent = ui.DescStyle.Render("loading changelog...") + "\n"
	} else if data, ok := m.changelogData[t.Name]; ok {
		changelogContent = m.renderChangelogBlock(data)
	} else if t.GitHub != "" {
		changelogContent = ui.DescStyle.Render("loading changelog...") + "\n"
	}
	if changelogContent != "" {
		sb.WriteString(m.sectionDivider("changelog"))
		sb.WriteString(changelogContent)
	}

	return sb.String()
}

// renderRepoStatus highlights the maintenance state of the upstream repo: a
// green dot for an active repo, a yellow warning sign for an archived one.
func renderRepoStatus(status string) string {
	switch status {
	case "active":
		return lipgloss.NewStyle().Foreground(ui.StatusColorActive).Render("● active")
	case "archived":
		return lipgloss.NewStyle().Foreground(ui.StatusColorTrying).Render("⚠ archived")
	default:
		return ui.RepoStatusStyle.Render(status)
	}
}

// sectionDivider renders a labeled section header that spans the panel's content
// width, e.g. "[info] ───────────". The label is rendered only by callers when
// the section actually has content, so no empty dividers are produced.
func (m Model) sectionDivider(label string) string {
	tag := "[" + label + "] "
	// briefW-1 to leave room for the scrollbar gutter; leading blank line adds
	// breathing room between sections.
	dashes := max(m.briefW-1-utf8.RuneCountInString(tag), 0)
	// Blank line above and below the header for breathing room.
	return "\n" + ui.SectionLabelStyle.Render(tag) +
		lipgloss.NewStyle().Foreground(ui.ColorBorder).Render(strings.Repeat("─", dashes)) + "\n\n"
}

func (m Model) renderChangelogBlock(msg changelogMsg) string {
	if msg.err != nil {
		return ui.InfoStyle.Render("changelog unavailable: "+msg.err.Error()) + "\n"
	}
	var sb strings.Builder
	// Only the link to the tag + the changelog text; version/date are already
	// shown in [info]. Unified muted style (InfoStyle), same as the [info] text.
	if msg.htmlUrl != "" {
		sb.WriteString(ui.InfoStyle.Render(msg.htmlUrl) + "\n\n")
	}
	body := wrapText(stripMarkdown(msg.body), max(m.briefW-2, 10))
	if body == "" {
		sb.WriteString(ui.InfoStyle.Render("no release notes available.") + "\n")
	} else {
		sb.WriteString(ui.InfoStyle.Render(body) + "\n")
	}
	return sb.String()
}

func (m Model) hasUpdate(toolName string) bool {
	vi, ok := m.versions[toolName]
	return ok && version.IsNewer(vi.Installed, vi.Latest)
}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Mouse policy: wheel scrolls in every mode, clicks (selection/focus) only
	// in modeNormal — while an input mode owns the keyboard a click must not
	// move selectedMeta() under it (wrong-target note/tags/rename commits).
	// Under the [L] overlay nothing may move: scrolling there is invisible.
	if !m.ready || m.apiOverlayVisible() {
		return m, nil
	}
	clickable := m.mode == modeNormal

	// Panels sit flush (each is panelW+2 wide incl. borders) with no outer
	// horizontal margin, so screen X maps directly to panel spans.
	toolsPanelEnd := m.toolsW + 2
	briefPanelEnd := toolsPanelEnd + m.briefW + 2

	// Detect which panel the click is in
	var cmd tea.Cmd
	if msg.X < toolsPanelEnd {
		// Left panel (Tools)
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && clickable {
			// Any click in the panel focuses it, matching brief/help.
			m.focus = focusTools
			// Row 0 = top margin, row 1 = panel border, row 2 = first list row.
			toolIdx := msg.Y - 2 + m.toolsViewport.YOffset
			filtered := m.filteredMeta()
			if toolIdx >= 0 && toolIdx < len(filtered) && m.metaSelected != toolIdx {
				// Mirror the keyboard j/k path (shared selectMeta helper,
				// including the auto-fetch).
				return m, m.selectMeta(toolIdx)
			}
		} else if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
			m.toolsViewport, cmd = m.toolsViewport.Update(msg)
		}
	} else if msg.X < briefPanelEnd {
		// Middle panel (Brief)
		switch msg.Button {
		case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
			m.briefViewport, cmd = m.briefViewport.Update(msg)
		case tea.MouseButtonLeft:
			if msg.Action == tea.MouseActionPress && clickable && m.focus != focusBrief {
				m.focus = focusBrief
				m.briefViewport.SetContent(m.renderCard())
			}
		}
	} else {
		// Right panel (Help)
		switch msg.Button {
		case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
			m.helpViewport, cmd = m.helpViewport.Update(msg)
		case tea.MouseButtonLeft:
			if msg.Action == tea.MouseActionPress && clickable && m.focus != focusHelp {
				m.focus = focusHelp
				m.helpViewport.SetContent(m.renderHelpContent())
			}
		}
	}
	return m, cmd
}

func (m Model) rawHelpText() string {
	mt, ok := m.selectedMeta()
	if !ok {
		return ""
	}
	cached, has := m.helpCache[mt.Name]
	if !has {
		return ""
	}
	return cached[m.helpMode]
}

func (m Model) renderHelpContent() string {
	mt, ok := m.selectedMeta()
	if !ok {
		return ui.MetaNoteStyle.Render("No tool selected")
	}

	// Gate per tool, not on "any fetch in flight": another tool's fetch may
	// still be running (fast j/k, search arrows then esc/enter) while the
	// selected tool's help is already cached — that cache must render, or the
	// panel would stick on "Loading..." when the stale fetch lands unselected.
	if m.helpLoadingFor == mt.Name {
		return ui.MetaNoteStyle.Render("Loading...")
	}

	cached, has := m.helpCache[mt.Name]
	if !has || cached[m.helpMode] == "" {
		if m.helpMode == helpModeHelp {
			return ui.MetaNoteStyle.Render("Press [h] for --help\nPress [m] for man page")
		}
		return ui.MetaNoteStyle.Render("Press [m] for man page\nPress [h] for --help")
	}
	text := cached[m.helpMode]
	if innerW := max(m.helpW-2, 20); innerW > 0 {
		text = wrapText(text, innerW)
	}
	if m.mode != modeHelpSearch || m.helpSearch.Value() == "" {
		return colorizeHelp(text)
	}
	query := m.helpSearch.Value()
	lines := strings.Split(text, "\n")
	result := make([]string, len(lines))
	for i, line := range lines {
		result[i] = highlightMatch(line, query)
	}
	return strings.Join(result, "\n")
}

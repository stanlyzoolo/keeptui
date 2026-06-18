package model

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lepeshko/keys/internal/loader"
	"github.com/lepeshko/keys/internal/ui"
	"github.com/lepeshko/keys/internal/version"
)

const (
	focusLeft   = 0
	focusRight  = 1
	focusHeader = 2
	leftWidth   = 22
)

type VersionInfo struct {
	Installed string
	Latest    string
}

type versionMsg struct {
	toolName   string
	installed  string
	latest     string
	repoStatus string
}

type checkVersionMsg struct {
	toolName   string
	latest     string
	repoStatus string
	err        error
}

type changelogMsg struct {
	toolName    string
	tag         string
	body        string
	htmlUrl     string
	publishedAt string
	err         error
}

type repoCardMsg struct {
	toolName string
	card     version.RepoCard
	err      error
}

type Model struct {
	tools               []loader.Tool
	versions            map[string]VersionInfo
	repoStatus          map[string]string
	repoCards           map[string]version.RepoCard
	checkingVersionTool string
	focus               int
	viewport            viewport.Model
	search              textinput.Model
	searching           bool
	statusMsg           string
	width               int
	height              int
	ready               bool

	showChangelog     bool
	changelogLoading  bool
	changelogViewport viewport.Model
	changelogReady    bool
	changelogToolName string
	changelogHtmlUrl  string

	meta         []loader.ToolMeta
	metaFilter   loader.Status
	metaSelected int
}

type Options struct {
	InitialTool   string
	InitialSearch string
}

func New(meta []loader.ToolMeta, opts Options) Model {
	ti := textinput.New()
	ti.Placeholder = "search..."
	ti.CharLimit = 64

	m := Model{
		tools:      loader.ToolsFromMeta(meta),
		versions:   make(map[string]VersionInfo),
		repoStatus: make(map[string]string),
		repoCards:  make(map[string]version.RepoCard),
		search:     ti,
		meta:       meta,
	}

	if opts.InitialTool != "" {
		for i, mt := range m.meta {
			if strings.EqualFold(mt.Name, opts.InitialTool) {
				m.metaSelected = i
				m.focus = focusRight
				break
			}
		}
	}

	if opts.InitialSearch != "" {
		m.searching = true
		m.search.SetValue(opts.InitialSearch)
		m.search.Focus()
	}

	return m
}

func (m Model) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.tools)*2)
	for _, t := range m.tools {
		cmds = append(cmds, func() tea.Msg {
			installed := version.InstalledVersion(t)
			latest := version.GetLatest(t.GitHub)
			repoStatus := version.GetCachedRepoStatus(t.GitHub)
			return versionMsg{
				toolName:   t.Name,
				installed:  installed,
				latest:     latest,
				repoStatus: repoStatus,
			}
		})
		if t.GitHub != "" {
			cmds = append(cmds, fetchRepoCardCmd(t))
		}
	}
	if m.searching {
		cmds = append(cmds, textinput.Blink)
	}
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.MouseMsg:
		return m.handleMouse(msg)

	case versionMsg:
		m.versions[msg.toolName] = VersionInfo{
			Installed: msg.installed,
			Latest:    msg.latest,
		}
		if msg.repoStatus != "" {
			m.repoStatus[msg.toolName] = msg.repoStatus
		}
		m.viewport.SetContent(m.renderContent())
		return m, nil

	case checkVersionMsg:
		if msg.toolName == m.checkingVersionTool {
			m.checkingVersionTool = ""
		}
		if msg.err == nil {
			vi := m.versions[msg.toolName]
			vi.Latest = msg.latest
			m.versions[msg.toolName] = vi
			if msg.repoStatus != "" {
				m.repoStatus[msg.toolName] = msg.repoStatus
			}
		} else {
			m.statusMsg = "Version check failed: " + msg.err.Error()
		}
		m.viewport.SetContent(m.renderContent())
		return m, nil

	case changelogMsg:
		if msg.toolName == m.changelogToolName {
			m.changelogLoading = false
			m.changelogHtmlUrl = msg.htmlUrl
			m.changelogViewport.SetContent(m.renderChangelogContent(msg))
			m.changelogViewport.GotoTop()
		}
		return m, nil

	case repoCardMsg:
		if msg.err == nil {
			m.repoCards[msg.toolName] = msg.card
			m.viewport.SetContent(m.renderContent())
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		rightWidth := max(m.width-leftWidth-6, 1)
		if !m.ready {
			m.viewport = viewport.New(rightWidth, m.calcVpHeight())
			m.ready = true
		} else {
			m.viewport.Width = rightWidth
			m.viewport.Height = m.calcVpHeight()
		}
		m.viewport.SetContent(m.renderContent())

		clW := min(80, m.width-10)
		clH := min(24, m.height-10)
		if !m.changelogReady {
			m.changelogViewport = viewport.New(max(clW-6, 1), max(clH-6, 1))
			m.changelogReady = true
		} else {
			m.changelogViewport.Width = max(clW-6, 1)
			m.changelogViewport.Height = max(clH-6, 1)
		}
		return m, nil

	case tea.KeyMsg:
		m.statusMsg = ""

		if m.showChangelog {
			switch msg.String() {
			case "esc", "q":
				m.showChangelog = false
				m.changelogLoading = false
				return m, nil
			case "o":
				if m.changelogHtmlUrl != "" {
					openBrowser(m.changelogHtmlUrl)
				}
				return m, nil
			default:
				var cmd tea.Cmd
				m.changelogViewport, cmd = m.changelogViewport.Update(msg)
				return m, cmd
			}
		}

		if m.focus == focusHeader {
			return m.updateHeaderFocus(msg)
		}

		if m.searching {
			switch msg.String() {
			case "esc":
				m.searching = false
				m.search.SetValue("")
				m.search.Blur()
				m.metaSelected = 0
				m.viewport.SetContent(m.renderContent())
				return m, nil
			default:
				m.search, cmd = m.search.Update(msg)
				m.metaSelected = 0
				m.viewport.SetContent(m.renderContent())
				m.viewport.GotoTop()
				return m, cmd
			}
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "esc":
			if m.focus == focusRight {
				m.focus = focusLeft
				m.viewport.SetContent(m.renderContent())
			} else {
				return m, tea.Quit
			}

		case "right", "l":
			if m.focus == focusLeft {
				m.focus = focusHeader
				m.viewport.SetContent(m.renderContent())
			}

		case "left", "h":
			if m.focus == focusRight {
				m.focus = focusLeft
				m.viewport.SetContent(m.renderContent())
			}

		case "j", "down":
			if m.focus == focusLeft {
				filtered := m.filteredMeta()
				if m.metaSelected < len(filtered)-1 {
					m.metaSelected++
					m.viewport.Height = m.calcVpHeight()
					m.viewport.GotoTop()
					m.viewport.SetContent(m.renderContent())
				}
			}

		case "k", "up":
			if m.focus == focusLeft {
				if m.metaSelected > 0 {
					m.metaSelected--
					m.viewport.Height = m.calcVpHeight()
					m.viewport.GotoTop()
					m.viewport.SetContent(m.renderContent())
				}
			}

		case "g":
			m.viewport.GotoTop()

		case "G":
			m.viewport.GotoBottom()

		case "/":
			m.searching = true
			m.search.Focus()
			return m, textinput.Blink

		case "f":
			if m.focus == focusLeft {
				switch m.metaFilter {
				case "":
					m.metaFilter = loader.StatusActive
				case loader.StatusActive:
					m.metaFilter = loader.StatusTrying
				case loader.StatusTrying:
					m.metaFilter = loader.StatusForgotten
				case loader.StatusForgotten:
					m.metaFilter = loader.StatusArchived
				default:
					m.metaFilter = ""
				}
				m.metaSelected = 0
				m.viewport.SetContent(m.renderContent())
			}

		case "1":
			m.metaFilter = loader.StatusActive
			m.metaSelected = 0
			m.viewport.SetContent(m.renderContent())
		case "2":
			m.metaFilter = loader.StatusTrying
			m.metaSelected = 0
			m.viewport.SetContent(m.renderContent())
		case "3":
			m.metaFilter = loader.StatusForgotten
			m.metaSelected = 0
			m.viewport.SetContent(m.renderContent())
		case "4":
			m.metaFilter = loader.StatusArchived
			m.metaSelected = 0
			m.viewport.SetContent(m.renderContent())
		case "a":
			m.metaFilter = ""
			m.metaSelected = 0
			m.viewport.SetContent(m.renderContent())

		case "v":
			if m.focus == focusLeft && m.checkingVersionTool == "" {
				if t, ok := m.selectedTool(); ok && t.GitHub != "" {
					m.checkingVersionTool = t.Name
					return m, fetchVersionCmd(t)
				}
			}

		case "o":
			if t, ok := m.selectedTool(); ok && t.GitHub != "" {
				openBrowser("https://" + t.GitHub)
			}

		case "c":
			if m.focus == focusRight && !m.searching {
				if t, ok := m.selectedTool(); ok && t.GitHub != "" {
					m.showChangelog = true
					m.changelogLoading = true
					m.changelogToolName = t.Name
					m.changelogViewport.SetContent("")
					m.changelogViewport.GotoTop()
					return m, fetchChangelogCmd(t.GitHub, t.Name)
				}
			}
		}

		if m.focus == focusRight {
			m.viewport, cmd = m.viewport.Update(msg)
		}
	}

	return m, cmd
}

func (m Model) selectedMeta() (loader.ToolMeta, bool) {
	filtered := m.filteredMeta()
	if m.metaSelected < 0 || m.metaSelected >= len(filtered) {
		return loader.ToolMeta{}, false
	}
	return filtered[m.metaSelected], true
}

func (m Model) selectedTool() (loader.Tool, bool) {
	mt, ok := m.selectedMeta()
	if !ok {
		return loader.Tool{}, false
	}
	for _, t := range m.tools {
		if t.Name == mt.Name {
			return t, true
		}
	}
	return loader.Tool{}, false
}

func (m Model) filteredMeta() []loader.ToolMeta {
	source := m.meta
	if m.metaFilter != "" {
		var filtered []loader.ToolMeta
		for _, mt := range m.meta {
			if mt.Status == m.metaFilter {
				filtered = append(filtered, mt)
			}
		}
		source = filtered
	}

	if m.searching {
		query := strings.ToLower(strings.TrimSpace(m.search.Value()))
		if query != "" {
			var out []loader.ToolMeta
			for _, mt := range source {
				if strings.Contains(strings.ToLower(mt.Name), query) {
					out = append(out, mt)
				}
			}
			return out
		}
	}
	return source
}

func (m Model) View() string {
	if !m.ready {
		return "Loading..."
	}

	left := m.renderLeft()
	right := m.renderRight()
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	layout := lipgloss.JoinVertical(lipgloss.Left, body, m.renderHelp())
	base := lipgloss.NewStyle().Margin(1).Render(layout)

	if m.showChangelog {
		return ui.PlaceOverlay(m.width, m.height, base, m.renderChangelog())
	}
	return base
}

func (m Model) renderHelp() string {
	style := ui.HelpStyle.Width(m.width - 4)
	if m.showChangelog {
		hints := keyHint("j/k") + " scroll  " + keyHint("esc") + " close"
		if m.changelogHtmlUrl != "" {
			hints += "  " + keyHint("o") + " open in browser"
		}
		return style.Render(hints)
	}
	if m.searching {
		return style.Render(fmt.Sprintf(
			"%s %s  %s exit search",
			ui.SearchPromptStyle.Render("/"),
			m.search.View(),
			keyHint("esc"),
		))
	}
	if m.statusMsg != "" {
		return style.Render(ui.SearchPromptStyle.Render(m.statusMsg))
	}
	if m.focus == focusHeader {
		hints := keyHint("↓/j") + " select  " + keyHint("←/esc") + " back  " + keyHint("q") + " quit"
		if t, ok := m.selectedTool(); ok && t.GitHub != "" {
			hints = keyHint("v") + " check version  " + keyHint("c") + " changelog  " + hints
		}
		return style.Render(hints)
	}
	if m.focus == focusRight {
		changelogHint := ""
		if t, ok := m.selectedTool(); ok && t.GitHub != "" {
			changelogHint = keyHint("c") + " changelog  "
			_ = t
		}
		return style.Render(
			keyHint("j/k") + " scroll  " +
				keyHint("o") + " github  " +
				changelogHint +
				keyHint("←/esc") + " back  " +
				keyHint("q") + " quit",
		)
	}
	filterHint := ""
	if m.metaFilter != "" {
		filterHint = keyHint("a") + " all  "
	}
	versionHint := ""
	if t, ok := m.selectedTool(); ok && t.GitHub != "" {
		versionHint = keyHint("v") + " check version  "
	}
	return style.Render(
		keyHint("j/k") + " navigate  " +
			keyHint("→") + " details  " +
			keyHint("f") + " filter  " +
			filterHint +
			keyHint("/") + " search  " +
			keyHint("o") + " github  " +
			versionHint +
			keyHint("q") + " quit",
	)
}

func keyHint(k string) string {
	return ui.SearchPromptStyle.Render("[" + k + "]")
}

func (m Model) calcVpHeight() int {
	return max(m.height-9, 1)
}

func (m Model) renderLeft() string {
	var sb strings.Builder

	filtered := m.filteredMeta()
	maxName := leftWidth - 5

	for i, mt := range filtered {
		sym := loader.StatusSymbol[mt.Status]
		symStyled := ui.StatusStyle(mt.Status).Render(sym)

		name := mt.Name
		if len(name) > maxName {
			name = name[:maxName]
		}

		hasUpdate := m.hasUpdate(mt.Name)
		updateMark := ""
		if hasUpdate {
			updateMark = " " + ui.UpdateAvailableStyle.Render("↑")
		}

		isSelected := i == m.metaSelected && m.focus == focusLeft && !m.searching
		if isSelected {
			circle := ui.SelectionBarStyle.Render("●")
			sb.WriteString(circle + " " + symStyled + " " + name + updateMark + "\n")
		} else {
			sb.WriteString("  " + symStyled + " " + name + updateMark + "\n")
		}
	}

	if len(filtered) == 0 {
		if m.searching {
			sb.WriteString(ui.DescStyle.Render("  No matches.") + "\n")
		} else if len(m.meta) == 0 {
			sb.WriteString(ui.DescStyle.Render("  No tools tracked.\n  Add one:\n  keys track <tool>\n  --github ...") + "\n")
		} else {
			sb.WriteString(ui.DescStyle.Render("  No tools match\n  current filter.") + "\n")
		}
	}

	total := len(m.meta)
	footer := fmt.Sprintf("  %d tools", total)
	if m.metaFilter != "" {
		footer += " [" + string(m.metaFilter) + "]"
	}
	content := sb.String() + "\n" + ui.MetaNoteStyle.Render(footer)

	panelStyle := ui.PanelBorder
	if m.focus == focusLeft && !m.showChangelog {
		panelStyle = ui.PanelBorderFocused
	}

	return panelStyle.
		Width(leftWidth).
		Height(max(m.height-7, 1)).
		Render(content)
}

func (m Model) renderRight() string {
	rightWidth := m.width - leftWidth - 6

	title := ""
	if m.searching {
		query := m.search.Value()
		title = ui.TitleStyle.Render("Search: ") + ui.SearchMatchStyle.Render(query)
	} else if t, ok := m.selectedTool(); ok {
		title = m.renderHeader(t)
	}

	panelStyle := ui.PanelBorder
	if (m.focus == focusRight || m.focus == focusHeader) && !m.showChangelog {
		panelStyle = ui.PanelBorderFocused
	}

	dividerWidth := max(rightWidth-2, 0)
	divider := lipgloss.NewStyle().Foreground(ui.ColorBorder).Render(strings.Repeat("─", dividerWidth))

	inner := lipgloss.JoinVertical(lipgloss.Left, title, divider, m.viewport.View())
	return panelStyle.
		Width(rightWidth).
		Height(max(m.height-7, 1)).
		Render(inner)
}

func (m Model) renderHeader(t loader.Tool) string {
	prefix := ""
	if m.focus == focusHeader {
		prefix = ui.SelectionBarStyle.Render("●") + " "
	}

	line := prefix + ui.TitleStyle.Render(t.Name)

	if m.checkingVersionTool == t.Name {
		line += " " + ui.VersionInstalledStyle.Render("checking...")
	} else if vi, ok := m.versions[t.Name]; ok {
		if version.IsNewer(vi.Installed, vi.Latest) {
			line += " " + ui.UpdateAvailableStyle.Render(vi.Installed+" -> "+vi.Latest)
		} else if vi.Installed != "" {
			line += " " + ui.VersionInstalledStyle.Render(vi.Installed)
			if vi.Latest != "" {
				line += " " + ui.VersionOkStyle.Render("✓")
			}
		} else {
			line += " " + ui.MetaNoteStyle.Render("not installed")
		}
	}

	line += "  " + ui.HeaderDescStyle.Render(t.Description)
	if t.GitHub != "" {
		line += "  " + ui.GithubStyle.Render("↗ "+t.GitHub)
		if status := m.repoStatus[t.Name]; status != "" {
			line += " " + ui.RepoStatusStyle.Render("(" + status + ")")
		}
	}

	return line
}

func (m Model) renderContent() string {
	if len(m.meta) == 0 {
		return ui.DescStyle.Render("No tools tracked. Add one: keys track <tool> --github <repo>")
	}

	t, ok := m.selectedTool()
	if !ok {
		return ui.DescStyle.Render("Select a tool from the left panel.")
	}

	var sb strings.Builder
	if t.Description != "" {
		sb.WriteString(ui.DescStyle.Render(t.Description) + "\n")
	}
	if t.GitHub != "" {
		sb.WriteString(ui.GithubStyle.Render("↗ https://"+t.GitHub) + "\n")
	}

	if card, ok := m.repoCards[t.Name]; ok {
		if card.About != "" {
			sb.WriteString("\n" + ui.DescStyle.Render(card.About) + "\n")
		}
		if card.Stars > 0 {
			sb.WriteString(ui.MetaNoteStyle.Render(fmt.Sprintf("★ %s stars", formatStars(card.Stars))) + "\n")
		}
		if len(card.Languages) > 0 {
			sb.WriteString(renderLangBar(card.Languages, m.viewport.Width) + "\n")
		}
		if card.Latest != "" {
			line := ui.MetaNoteStyle.Render("Latest: " + card.Latest)
			if card.PublishedAt != "" {
				date := card.PublishedAt
				if len(date) > 10 {
					date = date[:10]
				}
				line += " " + ui.MetaNoteStyle.Render("("+date+")")
			}
			sb.WriteString(line + "\n")
		}
	}

	if mt, ok := m.selectedMeta(); ok {
		if mt.Note != "" {
			sb.WriteString("\n" + ui.MetaDetailLabelStyle.Render("Note:") + " " + ui.MetaNoteStyle.Render(mt.Note) + "\n")
		}
		if len(mt.Tags) > 0 {
			sb.WriteString(ui.MetaDetailLabelStyle.Render("Tags:") + " " + ui.MetaTagStyle.Render(strings.Join(mt.Tags, ", ")) + "\n")
		}
	}

	return sb.String()
}

func (m Model) hasUpdate(toolName string) bool {
	vi, ok := m.versions[toolName]
	return ok && version.IsNewer(vi.Installed, vi.Latest)
}

func openBrowser(url string) {
	var cmd string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd = "start"
	default:
		cmd = "xdg-open"
	}
	exec.Command(cmd, url).Start() //nolint:errcheck
}

const leftPanelEdge = leftWidth + 3

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.X >= leftPanelEdge {
		switch msg.Button {
		case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		case tea.MouseButtonLeft:
			if msg.Action == tea.MouseActionPress && m.focus != focusRight {
				m.focus = focusRight
				m.viewport.SetContent(m.renderContent())
			}
		}
		return m, nil
	}

	if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
		// margin(1) + border(1) = offset 2; no tab headers now
		toolIdx := msg.Y - 3
		filtered := m.filteredMeta()
		if toolIdx >= 0 && toolIdx < len(filtered) {
			if m.metaSelected != toolIdx {
				m.metaSelected = toolIdx
				m.viewport.Height = m.calcVpHeight()
				m.viewport.GotoTop()
				m.viewport.SetContent(m.renderContent())
			}
			m.focus = focusLeft
		}
	}
	return m, nil
}

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
	for i := 0; i < len(out)-1; i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].Pct > out[i].Pct {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

// renderLangBar renders a horizontal language bar with percentages.
func renderLangBar(langs map[string]int, width int) string {
	percents := languagePercents(langs)
	if len(percents) == 0 {
		return ""
	}
	var parts []string
	for _, lp := range percents {
		parts = append(parts, fmt.Sprintf("%s %.0f%%", lp.Name, lp.Pct))
	}
	line := strings.Join(parts, "  ")
	if len(line) > width && width > 3 {
		line = line[:width-3] + "..."
	}
	return ui.MetaNoteStyle.Render(line)
}

func (m Model) updateHeaderFocus(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "j", "down":
		m.focus = focusRight
		m.viewport.GotoTop()
		m.viewport.SetContent(m.renderContent())

	case "left", "h", "esc":
		m.focus = focusLeft
		m.viewport.SetContent(m.renderContent())

	case "v":
		if m.checkingVersionTool == "" {
			if t, ok := m.selectedTool(); ok && t.GitHub != "" {
				m.checkingVersionTool = t.Name
				return m, fetchVersionCmd(t)
			}
		}

	case "c":
		if t, ok := m.selectedTool(); ok && t.GitHub != "" {
			m.showChangelog = true
			m.changelogLoading = true
			m.changelogToolName = t.Name
			m.changelogViewport.SetContent("")
			m.changelogViewport.GotoTop()
			return m, fetchChangelogCmd(t.GitHub, t.Name)
		}

	case "o":
		if t, ok := m.selectedTool(); ok && t.GitHub != "" {
			openBrowser("https://" + t.GitHub)
		}
	}
	return m, nil
}

func fetchVersionCmd(t loader.Tool) tea.Cmd {
	return func() tea.Msg {
		latest, err := version.FetchAndCache(t.GitHub)
		repoStatus := version.GetCachedRepoStatus(t.GitHub)
		return checkVersionMsg{
			toolName:   t.Name,
			latest:     latest,
			repoStatus: repoStatus,
			err:        err,
		}
	}
}

func fetchRepoCardCmd(t loader.Tool) tea.Cmd {
	return func() tea.Msg {
		card := version.GetRepoCard(t.GitHub)
		return repoCardMsg{toolName: t.Name, card: card}
	}
}

// --- Changelog ---

func fetchChangelogCmd(githubField, toolName string) tea.Cmd {
	return func() tea.Msg {
		info, err := version.GetChangelog(githubField)
		return changelogMsg{
			toolName:    toolName,
			tag:         info.Tag,
			body:        info.Body,
			htmlUrl:     info.HtmlUrl,
			publishedAt: info.PublishedAt,
			err:         err,
		}
	}
}

func (m Model) renderChangelogContent(msg changelogMsg) string {
	if msg.err != nil {
		return ui.DescStyle.Render("Failed to load changelog: " + msg.err.Error())
	}

	var sb strings.Builder
	sb.WriteString(ui.TitleStyle.Render(msg.tag) + "\n")
	if msg.publishedAt != "" {
		date := msg.publishedAt
		if len(date) > 10 {
			date = date[:10]
		}
		sb.WriteString(ui.MetaNoteStyle.Render("Released: "+date) + "\n")
	}
	sb.WriteString("\n")

	body := wrapText(stripMarkdown(msg.body), m.changelogViewport.Width)
	if body == "" {
		sb.WriteString(ui.DescStyle.Render("No release notes available.") + "\n")
	} else {
		sb.WriteString(ui.DescStyle.Render(body))
	}
	return sb.String()
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
		if len(line) <= width {
			result.WriteString(line)
			continue
		}
		words := strings.Fields(line)
		col := 0
		for j, word := range words {
			wl := len(word)
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

func (m Model) renderChangelog() string {
	popupWidth := min(80, m.width-10)
	innerWidth := max(popupWidth-8, 10)

	m.changelogViewport.Width = innerWidth

	var body string
	if m.changelogLoading {
		body = ui.DescStyle.Render("Loading changelog...")
	} else {
		body = m.changelogViewport.View()
	}

	hintStr := "[j/k] scroll  [esc] close"
	if m.changelogHtmlUrl != "" {
		hintStr += "  [o] open in browser"
	}
	hint := ui.TabInactiveStyle.Render(hintStr)
	content := lipgloss.JoinVertical(lipgloss.Left,
		ui.TitleStyle.Render("Changelog: "+m.changelogToolName),
		"",
		body,
		"",
		hint,
	)
	focusedStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ui.ColorPrimary).
		Padding(1, 2)
	return focusedStyle.Width(popupWidth).Render(content)
}

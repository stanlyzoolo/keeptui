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

type viewMode int

const (
	viewHotkeys viewMode = 0
	viewMyTools viewMode = 1
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

type Model struct {
	tools               []loader.Tool
	versions            map[string]VersionInfo
	repoStatus          map[string]string
	checkingVersionTool string
	selected            int
	focus               int
	viewport            viewport.Model
	search              textinput.Model
	searching           bool
	statusMsg           string
	width               int
	height              int
	ready               bool

	view viewMode

	showChangelog     bool
	changelogLoading  bool
	changelogViewport viewport.Model
	changelogReady    bool
	changelogToolName string
	changelogHtmlUrl  string

	meta         []loader.ToolMeta
	metaFilter   loader.Status
	metaSelected int
	metaDetail   bool
	editingNote  bool
	editingTags  bool
	noteInput    textinput.Model
	tagsInput    textinput.Model
}

type Options struct {
	InitialTool   string
	InitialSearch string
}

func New(meta []loader.ToolMeta, opts Options) Model {
	ti := textinput.New()
	ti.Placeholder = "search..."
	ti.CharLimit = 64

	noteInput := textinput.New()
	noteInput.Placeholder = "note text..."
	noteInput.CharLimit = 256

	tagsInput := textinput.New()
	tagsInput.Placeholder = "tag1, tag2..."
	tagsInput.CharLimit = 128

	m := Model{
		tools:      loader.ToolsFromMeta(meta),
		versions:   make(map[string]VersionInfo),
		repoStatus: make(map[string]string),
		search:     ti,
		meta:       meta,
		noteInput:  noteInput,
		tagsInput:  tagsInput,
	}

	if opts.InitialTool != "" {
		for i, t := range m.tools {
			if strings.EqualFold(t.Name, opts.InitialTool) {
				m.selected = i
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
	cmds := make([]tea.Cmd, len(m.tools))
	for i, t := range m.tools {
		cmds[i] = func() tea.Msg {
			installed := version.InstalledVersion(t)
			latest := version.GetLatest(t.GitHub)
			repoStatus := version.GetCachedRepoStatus(t.GitHub)
			return versionMsg{
				toolName:   t.Name,
				installed:  installed,
				latest:     latest,
				repoStatus: repoStatus,
			}
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
		if m.view == viewHotkeys {
			return m.handleMouse(msg)
		}
		return m, nil

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

		if m.view == viewMyTools {
			return m.updateMyTools(msg)
		}

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
				m.viewport.SetContent(m.renderContent())
				return m, nil
			default:
				m.search, cmd = m.search.Update(msg)
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

		case "tab":
			if m.focus == focusLeft && !m.searching {
				m.view = viewMyTools
				m.metaSelected = 0
				m.metaDetail = false
				return m, nil
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
				if m.selected < len(m.tools)-1 {
					m.selected++
					m.viewport.Height = m.calcVpHeight()
					m.viewport.GotoTop()
					m.viewport.SetContent(m.renderContent())
				}
			}

		case "k", "up":
			if m.focus == focusLeft {
				if m.selected > 0 {
					m.selected--
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

		case "v":
			if m.focus == focusLeft && len(m.tools) > 0 && m.checkingVersionTool == "" {
				t := m.tools[m.selected]
				if t.GitHub != "" {
					m.checkingVersionTool = t.Name
					return m, fetchVersionCmd(t)
				}
			}

		case "o":
			if len(m.tools) > 0 {
				t := m.tools[m.selected]
				if t.GitHub != "" {
					openBrowser("https://" + t.GitHub)
				}
			}

		case "c":
			if m.focus == focusRight && !m.searching && len(m.tools) > 0 {
				t := m.tools[m.selected]
				if t.GitHub != "" {
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

func (m Model) updateMyTools(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	filtered := m.filteredMeta()

	if m.editingNote {
		switch msg.String() {
		case "enter", "esc":
			if msg.String() == "enter" && len(filtered) > 0 {
				entry := filtered[m.metaSelected]
				entry.Note = m.noteInput.Value()
				m.meta = loader.UpsertMeta(m.meta, entry)
				loader.SaveMeta(m.meta) //nolint:errcheck
			}
			m.editingNote = false
			m.noteInput.Blur()
			return m, nil
		default:
			var cmd tea.Cmd
			m.noteInput, cmd = m.noteInput.Update(msg)
			return m, cmd
		}
	}

	if m.editingTags {
		switch msg.String() {
		case "enter", "esc":
			if msg.String() == "enter" && len(filtered) > 0 {
				entry := filtered[m.metaSelected]
				entry.Tags = splitTagsStr(m.tagsInput.Value())
				m.meta = loader.UpsertMeta(m.meta, entry)
				loader.SaveMeta(m.meta) //nolint:errcheck
			}
			m.editingTags = false
			m.tagsInput.Blur()
			return m, nil
		default:
			var cmd tea.Cmd
			m.tagsInput, cmd = m.tagsInput.Update(msg)
			return m, cmd
		}
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "tab":
		m.view = viewHotkeys
		m.metaDetail = false
		return m, nil

	case "esc":
		if m.metaDetail {
			m.metaDetail = false
		} else {
			m.view = viewHotkeys
		}

	case "enter":
		if !m.metaDetail && len(filtered) > 0 {
			m.metaDetail = true
		}

	case "j", "down":
		if !m.metaDetail {
			if m.metaSelected < len(filtered)-1 {
				m.metaSelected++
			}
		}

	case "k", "up":
		if !m.metaDetail {
			if m.metaSelected > 0 {
				m.metaSelected--
			}
		}

	case "s":
		if len(filtered) > 0 {
			entry := filtered[m.metaSelected]
			entry.Status = loader.NextStatus(entry.Status)
			m.meta = loader.UpsertMeta(m.meta, entry)
			loader.SaveMeta(m.meta) //nolint:errcheck
			newFiltered := m.filteredMeta()
			if m.metaSelected >= len(newFiltered) && len(newFiltered) > 0 {
				m.metaSelected = len(newFiltered) - 1
			}
		}

	case "e":
		if m.metaDetail && len(filtered) > 0 {
			entry := filtered[m.metaSelected]
			m.noteInput.SetValue(entry.Note)
			m.noteInput.Focus()
			m.editingNote = true
			return m, textinput.Blink
		}

	case "t":
		if m.metaDetail && len(filtered) > 0 {
			entry := filtered[m.metaSelected]
			m.tagsInput.SetValue(strings.Join(entry.Tags, ", "))
			m.tagsInput.Focus()
			m.editingTags = true
			return m, textinput.Blink
		}

	case "f":
		if !m.metaDetail {
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
		}

	case "1":
		m.metaFilter = loader.StatusActive
		m.metaSelected = 0
	case "2":
		m.metaFilter = loader.StatusTrying
		m.metaSelected = 0
	case "3":
		m.metaFilter = loader.StatusForgotten
		m.metaSelected = 0
	case "4":
		m.metaFilter = loader.StatusArchived
		m.metaSelected = 0
	case "a":
		if !m.metaDetail {
			m.metaFilter = ""
			m.metaSelected = 0
		}
	}

	return m, nil
}

func (m Model) filteredMeta() []loader.ToolMeta {
	if m.metaFilter == "" {
		return m.meta
	}
	var out []loader.ToolMeta
	for _, mt := range m.meta {
		if mt.Status == m.metaFilter {
			out = append(out, mt)
		}
	}
	return out
}

func (m Model) View() string {
	if !m.ready {
		return "Loading..."
	}

	if m.view == viewMyTools {
		return m.renderMyToolsView()
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

// --- My Tools rendering ---

func (m Model) renderMyToolsView() string {
	if m.metaDetail {
		return m.renderMyToolsDetail()
	}
	return m.renderMyToolsList()
}

func (m Model) renderMyToolsList() string {
	filtered := m.filteredMeta()

	filterLabel := "all"
	if m.metaFilter != "" {
		filterLabel = string(m.metaFilter)
	}

	var sb strings.Builder
	sb.WriteString(ui.TitleStyle.Render("My Tools") + "  ")

	hotkeysTab := ui.TopTabInactiveStyle.Render("Tools")
	myToolsTab := ui.TopTabActiveStyle.Render("[My Tools]")
	sb.WriteString(hotkeysTab + "  " + myToolsTab + "\n\n")

	filterStr := ui.MetaNoteStyle.Render("Filter: " + filterLabel)
	sb.WriteString("  " + filterStr + "\n\n")

	if len(filtered) == 0 {
		sb.WriteString(ui.DescStyle.Render("  No tools. Add one: keys track <tool> --github <repo>") + "\n")
	} else {
		for i, mt := range filtered {
			sym := loader.StatusSymbol[mt.Status]
			symStyled := ui.StatusStyle(mt.Status).Render(sym)
			statusStr := ui.StatusStyle(mt.Status).Width(9).Render(string(mt.Status))
			tags := ui.MetaTagStyle.Render(strings.Join(mt.Tags, ", "))
			name := mt.Name

			line := fmt.Sprintf("  %s %s  %-16s  %s", symStyled, statusStr, name, tags)
			if i == m.metaSelected {
				line = ui.SelectionBarStyle.Render("●") + line[1:]
			}
			sb.WriteString(line + "\n")
		}
	}

	sb.WriteString("\n")
	active, trying, forgotten, archived := countStatuses(m.meta)
	total := len(m.meta)
	stats := fmt.Sprintf("  %d tools  ·  %d active  ·  %d trying  ·  %d forgotten  ·  %d archived",
		total, active, trying, forgotten, archived)
	sb.WriteString(ui.MetaNoteStyle.Render(stats) + "\n")

	content := sb.String()

	panelStyle := ui.PanelBorderFocused.
		Width(m.width - 4).
		Height(max(m.height-7, 1))

	help := m.renderMyToolsHelp(false)
	body := lipgloss.JoinVertical(lipgloss.Left, panelStyle.Render(content), help)
	return lipgloss.NewStyle().Margin(1).Render(body)
}

func (m Model) renderMyToolsDetail() string {
	filtered := m.filteredMeta()
	if len(filtered) == 0 {
		return ""
	}
	mt := filtered[m.metaSelected]

	sym := loader.StatusSymbol[mt.Status]
	symStyled := ui.StatusStyle(mt.Status).Render(sym + "  " + string(mt.Status))

	title := ui.TitleStyle.Render(mt.Name) + "  " + symStyled

	var sb strings.Builder
	sb.WriteString(title + "\n\n")

	added := mt.Added
	if added == "" {
		added = "unknown"
	}
	sb.WriteString(ui.MetaDetailLabelStyle.Render("Added:") + "  " + ui.MetaDetailValueStyle.Render(added) + "\n")

	tags := strings.Join(mt.Tags, ", ")
	if tags == "" {
		tags = "—"
	}
	if m.editingTags {
		sb.WriteString(ui.MetaDetailLabelStyle.Render("Tags:") + "  " + m.tagsInput.View() + "\n")
	} else {
		sb.WriteString(ui.MetaDetailLabelStyle.Render("Tags:") + "  " + ui.MetaTagStyle.Render(tags) + "\n")
	}

	note := mt.Note
	if note == "" {
		note = "—"
	}
	if m.editingNote {
		sb.WriteString(ui.MetaDetailLabelStyle.Render("Note:") + "  " + m.noteInput.View() + "\n")
	} else {
		sb.WriteString(ui.MetaDetailLabelStyle.Render("Note:") + "  " + ui.MetaNoteStyle.Render(note) + "\n")
	}

	popupWidth := min(70, m.width-10)
	panel := ui.PopupStyle.Width(popupWidth).Render(sb.String())

	help := m.renderMyToolsHelp(true)
	body := lipgloss.JoinVertical(lipgloss.Left, panel, "\n", help)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, body,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("#0A0A0A")),
	)
}

func (m Model) renderMyToolsHelp(detail bool) string {
	style := ui.HelpStyle.Width(m.width - 4)
	if detail {
		return style.Render(
			keyHint("s") + " status  " +
				keyHint("e") + " edit note  " +
				keyHint("t") + " edit tags  " +
				keyHint("esc") + " back  " +
				keyHint("q") + " quit",
		)
	}
	return style.Render(
		keyHint("j/k") + " navigate  " +
			keyHint("enter") + " details  " +
			keyHint("s") + " status  " +
			keyHint("f") + " filter  " +
			keyHint("[1-4]") + " filter by status  " +
			keyHint("tab") + " tools  " +
			keyHint("q") + " quit",
	)
}

func countStatuses(meta []loader.ToolMeta) (active, trying, forgotten, archived int) {
	for _, m := range meta {
		switch m.Status {
		case loader.StatusActive:
			active++
		case loader.StatusTrying:
			trying++
		case loader.StatusForgotten:
			forgotten++
		case loader.StatusArchived:
			archived++
		}
	}
	return
}

func splitTagsStr(s string) []string {
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// --- Hotkeys view rendering ---

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
		if len(m.tools) > 0 && m.tools[m.selected].GitHub != "" {
			hints = keyHint("v") + " check version  " + keyHint("c") + " changelog  " + hints
		}
		return style.Render(hints)
	}
	if m.focus == focusRight {
		changelogHint := ""
		if len(m.tools) > 0 && m.tools[m.selected].GitHub != "" {
			changelogHint = keyHint("c") + " changelog  "
		}
		return style.Render(
			keyHint("j/k") + " scroll  " +
				keyHint("o") + " github  " +
				changelogHint +
				keyHint("←/esc") + " back  " +
				keyHint("q") + " quit",
		)
	}
	versionHint := ""
	if len(m.tools) > 0 && m.tools[m.selected].GitHub != "" {
		versionHint = keyHint("v") + " check version  "
	}
	return style.Render(
		keyHint("j/k") + " navigate  " +
			keyHint("→") + " details  " +
			keyHint("tab") + " my tools  " +
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

	toolsTab := ui.TopTabActiveStyle.Render("[Tools]")
	myToolsTab := ui.TopTabInactiveStyle.Render("My Tools")
	sb.WriteString(toolsTab + "  " + myToolsTab + "\n\n")

	maxName := leftWidth - 5

	for i, t := range m.tools {
		hasUpdate := m.hasUpdate(t.Name)

		name := t.Name
		if len(name) > maxName {
			name = name[:maxName]
		}

		updateMark := ""
		if hasUpdate {
			updateMark = " " + ui.UpdateAvailableStyle.Render("↑")
		}

		isSelected := i == m.selected && m.focus == focusLeft && !m.searching
		if isSelected {
			circle := ui.SelectionBarStyle.Render("●")
			sb.WriteString(circle + "  " + name + updateMark + "\n")
		} else {
			sb.WriteString(ui.ToolNormalStyle.Render("   "+name) + updateMark + "\n")
		}
	}

	if len(m.tools) == 0 {
		sb.WriteString(ui.DescStyle.Render("  No tools.") + "\n")
	}

	panelStyle := ui.PanelBorder
	if m.focus == focusLeft && !m.searching && !m.showChangelog {
		panelStyle = ui.PanelBorderFocused
	}

	return panelStyle.
		Width(leftWidth).
		Height(max(m.height-7, 1)).
		Render(sb.String())
}

func (m Model) renderRight() string {
	rightWidth := m.width - leftWidth - 6

	title := ""
	if len(m.tools) > 0 && !m.searching {
		title = m.renderHeader(m.tools[m.selected])
	} else if m.searching {
		query := m.search.Value()
		title = ui.TitleStyle.Render("Search: ") + ui.SearchMatchStyle.Render(query)
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
	if len(m.tools) == 0 {
		return ui.DescStyle.Render("No tools tracked. Add one: keys track <tool> --github <repo>")
	}

	if m.searching {
		query := strings.ToLower(strings.TrimSpace(m.search.Value()))
		if query != "" {
			return m.renderSearchResults(query)
		}
	}

	t := m.tools[m.selected]
	var sb strings.Builder
	if t.Description != "" {
		sb.WriteString(ui.DescStyle.Render(t.Description) + "\n")
	}
	if t.GitHub != "" {
		sb.WriteString(ui.GithubStyle.Render("↗ https://"+t.GitHub) + "\n")
	}
	return sb.String()
}

func (m Model) renderSearchResults(query string) string {
	var sb strings.Builder
	found := 0

	for _, t := range m.tools {
		if strings.Contains(strings.ToLower(t.Name), query) ||
			strings.Contains(strings.ToLower(t.Description), query) {
			line := ui.TitleStyle.Render(t.Name) + "  " + ui.DescStyle.Render(t.Description)
			sb.WriteString(line + "\n")
			found++
		}
	}

	if found == 0 {
		sb.WriteString(ui.DescStyle.Render("No matches found."))
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
		toolIdx := msg.Y - 4
		if toolIdx >= 0 && toolIdx < len(m.tools) {
			if m.selected != toolIdx {
				m.selected = toolIdx
				m.viewport.Height = m.calcVpHeight()
				m.viewport.GotoTop()
				m.viewport.SetContent(m.renderContent())
			}
			m.focus = focusLeft
		}
	}
	return m, nil
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
		if len(m.tools) > 0 && m.checkingVersionTool == "" {
			t := m.tools[m.selected]
			if t.GitHub != "" {
				m.checkingVersionTool = t.Name
				return m, fetchVersionCmd(t)
			}
		}

	case "c":
		if len(m.tools) > 0 {
			t := m.tools[m.selected]
			if t.GitHub != "" {
				m.showChangelog = true
				m.changelogLoading = true
				m.changelogToolName = t.Name
				m.changelogViewport.SetContent("")
				m.changelogViewport.GotoTop()
				return m, fetchChangelogCmd(t.GitHub, t.Name)
			}
		}

	case "o":
		if len(m.tools) > 0 {
			t := m.tools[m.selected]
			if t.GitHub != "" {
				openBrowser("https://" + t.GitHub)
			}
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

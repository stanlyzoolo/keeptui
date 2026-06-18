package model

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/atotto/clipboard"
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

	tabKeys     = 0
	tabCommands = 1
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
	// hotkeys view
	tools               []loader.Tool
	versions            map[string]VersionInfo
	repoStatus          map[string]string
	checkingVersionTool string
	selected            int
	focus               int
	selectedBinding     int
	rightTab            int
	selectedCommand     int
	showPopup           bool
	popupCommand        loader.Command
	viewport            viewport.Model
	search              textinput.Model
	searching           bool
	statusMsg           string
	width               int
	height              int
	ready               bool

	// top-level view
	view viewMode

	// changelog overlay
	showChangelog     bool
	changelogLoading  bool
	changelogViewport viewport.Model
	changelogReady    bool
	changelogToolName string
	changelogHtmlUrl  string

	// my tools view
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

func New(tools []loader.Tool, meta []loader.ToolMeta, opts Options) Model {
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
		tools:      tools,
		versions:   make(map[string]VersionInfo),
		repoStatus: make(map[string]string),
		search:     ti,
		meta:       meta,
		noteInput:  noteInput,
		tagsInput:  tagsInput,
	}

	if opts.InitialTool != "" {
		for i, t := range tools {
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
			latest := version.GetLatest(t.GitHub) // also fetches and caches repo status
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

		// --- Changelog overlay ---
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

		// --- Header focus ---
		if m.focus == focusHeader {
			return m.updateHeaderFocus(msg)
		}

		// --- Hotkeys view ---
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
			if m.showPopup {
				m.showPopup = false
				m.viewport.SetContent(m.renderContent())
			} else if m.focus == focusRight {
				m.focus = focusLeft
				m.viewport.SetContent(m.renderContent())
			} else {
				return m, tea.Quit
			}

		case "tab":
			if m.focus == focusLeft && !m.searching {
				// top-level switch: Hotkeys → My Tools
				m.view = viewMyTools
				m.metaSelected = 0
				m.metaDetail = false
				return m, nil
			}
			if m.focus == focusRight && !m.searching {
				if len(m.tools) > 0 && len(m.tools[m.selected].CommandGroups) > 0 {
					if m.rightTab == tabKeys {
						m.rightTab = tabCommands
					} else {
						m.rightTab = tabKeys
					}
					m.selectedBinding = 0
					m.selectedCommand = 0
					m.viewport.GotoTop()
					m.viewport.SetContent(m.renderContent())
				}
			}

		case "enter":
			if m.focus == focusRight && !m.searching && !m.showPopup && m.rightTab == tabCommands {
				if c := m.commandAt(m.selectedCommand); c != nil {
					m.popupCommand = *c
					m.showPopup = true
				}
			}

		case "right", "l":
			if m.focus == focusLeft {
				m.focus = focusHeader
				m.selectedBinding = 0
				m.selectedCommand = 0
				m.rightTab = tabKeys
				m.viewport.SetContent(m.renderContent())
			}

		case "left", "h":
			if m.focus == focusRight && !m.showPopup {
				m.focus = focusLeft
				m.viewport.SetContent(m.renderContent())
			}

		case "j", "down":
			if m.showPopup {
				break
			}
			if m.focus == focusLeft {
				if m.selected < len(m.tools)-1 {
					m.selected++
					m.selectedBinding = 0
					m.selectedCommand = 0
					m.rightTab = tabKeys
					m.viewport.Height = m.calcVpHeight()
					m.viewport.GotoTop()
					m.viewport.SetContent(m.renderContent())
				}
			} else if m.rightTab == tabKeys {
				m.selectedBinding = min(m.selectedBinding+1, m.totalBindings()-1)
				m.viewport.SetContent(m.renderContent())
				m.scrollToBinding()
			} else {
				m.selectedCommand = min(m.selectedCommand+1, m.totalCommands()-1)
				m.viewport.SetContent(m.renderContent())
				m.scrollToCommand()
			}

		case "k", "up":
			if m.showPopup {
				break
			}
			if m.focus == focusLeft {
				if m.selected > 0 {
					m.selected--
					m.selectedBinding = 0
					m.selectedCommand = 0
					m.rightTab = tabKeys
					m.viewport.Height = m.calcVpHeight()
					m.viewport.GotoTop()
					m.viewport.SetContent(m.renderContent())
				}
			} else if m.focus == focusRight {
				if m.rightTab == tabKeys {
					if m.selectedBinding > 0 {
						m.selectedBinding--
						m.viewport.SetContent(m.renderContent())
						m.scrollToBinding()
					} else {
						m.focus = focusHeader
						m.viewport.SetContent(m.renderContent())
					}
				} else {
					if m.selectedCommand > 0 {
						m.selectedCommand--
						m.viewport.SetContent(m.renderContent())
						m.scrollToCommand()
					} else {
						m.focus = focusHeader
						m.viewport.SetContent(m.renderContent())
					}
				}
			}

		case "g":
			if m.focus == focusRight {
				m.selectedBinding = 0
				m.selectedCommand = 0
				m.viewport.GotoTop()
				m.viewport.SetContent(m.renderContent())
			} else {
				m.viewport.GotoTop()
			}

		case "G":
			if m.focus == focusRight {
				if m.rightTab == tabKeys {
					m.selectedBinding = max(m.totalBindings()-1, 0)
					m.viewport.SetContent(m.renderContent())
					m.scrollToBinding()
				} else {
					m.selectedCommand = max(m.totalCommands()-1, 0)
					m.viewport.SetContent(m.renderContent())
					m.scrollToCommand()
				}
			} else {
				m.viewport.GotoBottom()
			}

		case "/":
			m.searching = true
			m.search.Focus()
			return m, textinput.Blink

		case "y":
			if m.focus == focusRight && !m.searching {
				if m.showPopup || m.rightTab == tabCommands {
					c := &m.popupCommand
					if !m.showPopup {
						c = m.commandAt(m.selectedCommand)
					}
					if c != nil {
						if err := clipboard.WriteAll(c.Cmd); err == nil {
							m.statusMsg = "Copied: " + c.Cmd
							m.showPopup = false
						}
					}
				} else {
					if b := m.bindingAt(m.selectedBinding); b != nil {
						if err := clipboard.WriteAll(b.Key); err == nil {
							m.statusMsg = "Copied: " + b.Key
						}
					}
				}
			}

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
			if m.focus == focusRight && !m.searching && !m.showPopup && len(m.tools) > 0 {
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

// updateMyTools handles all key events in the My Tools view.
func (m Model) updateMyTools(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	filtered := m.filteredMeta()

	// inline note editing
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

	// inline tags editing
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
		// return to Hotkeys view
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
			// re-apply filter and keep selection in bounds
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
			// cycle filter: "" → active → trying → forgotten → archived → ""
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
	if m.showPopup {
		return ui.PlaceOverlay(m.width, m.height, base, m.renderPopup())
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

	// top tabs hint
	hotkeysTab := ui.TopTabInactiveStyle.Render("Hotkeys")
	myToolsTab := ui.TopTabActiveStyle.Render("[My Tools]")
	sb.WriteString(hotkeysTab + "  " + myToolsTab + "\n\n")

	filterStr := ui.MetaNoteStyle.Render("Filter: " + filterLabel)
	sb.WriteString("  " + filterStr + "\n\n")

	if len(filtered) == 0 {
		sb.WriteString(ui.DescStyle.Render("  No tools. Add one: keys track <tool>") + "\n")
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

	// stats footer
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
			keyHint("tab") + " hotkeys  " +
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
	if m.showPopup {
		return style.Render(
			keyHint("y") + " copy  " +
				keyHint("esc") + " close",
		)
	}
	if m.focus == focusHeader {
		hints := keyHint("↓/j") + " bindings  " + keyHint("←/esc") + " back  " + keyHint("q") + " quit"
		if len(m.tools) > 0 && m.tools[m.selected].GitHub != "" {
			hints = keyHint("v") + " check version  " + keyHint("c") + " changelog  " + hints
		}
		return style.Render(hints)
	}
	if m.focus == focusRight {
		if m.rightTab == tabCommands {
			return style.Render(
				keyHint("j/k") + " scroll  " +
					keyHint("enter") + " details  " +
					keyHint("y") + " copy cmd  " +
					keyHint("tab") + " keys tab  " +
					keyHint("←/esc") + " back  " +
					keyHint("q") + " quit",
			)
		}
		changelogHint := ""
		if len(m.tools) > 0 && m.tools[m.selected].GitHub != "" {
			changelogHint = keyHint("c") + " changelog  "
		}
		return style.Render(
			keyHint("j/k") + " scroll  " +
				keyHint("y") + " copy hotkey  " +
				keyHint("tab") + " cmds tab  " +
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
			keyHint("→") + " scroll panel  " +
			keyHint("tab") + " my tools  " +
			keyHint("/") + " search  " +
			keyHint("o") + " github  " +
			versionHint +
			keyHint("q") + " quit",
	)
}

func keyHint(k string) string {
	return ui.SearchPromptStyle.Render("["+k+"]")
}

// calcVpHeight returns the correct viewport height for the current state,
// accounting for header lines (title-only vs title+divider+tabs).
func (m Model) calcVpHeight() int {
	h := m.height - 9
	if !m.searching && len(m.tools) > 0 && len(m.tools[m.selected].CommandGroups) > 0 {
		h -= 2 // inner-tabs divider + tabs line
	}
	return max(h, 1)
}

func (m Model) renderLeft() string {
	var sb strings.Builder

	hotkeysTab := ui.TopTabActiveStyle.Render("[Hotkeys]")
	myToolsTab := ui.TopTabInactiveStyle.Render("My Tools")
	sb.WriteString(hotkeysTab + "  " + myToolsTab + "\n\n")

	maxName := leftWidth - 5 // space for ● + 2 spaces + optional ↑

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
			line += " " + ui.RepoStatusStyle.Render("("+status+")")
		}
	}

	if len(t.CommandGroups) > 0 {
		rightWidth := m.width - leftWidth - 6
		dividerWidth := max(rightWidth-2, 0)
		tabDivider := lipgloss.NewStyle().Foreground(ui.ColorBorder).Render(strings.Repeat("─", dividerWidth))

		var keysTab, cmdsTab string
		if m.rightTab == tabKeys {
			keysTab = ui.TabActiveStyle.Render("[Keys]")
			cmdsTab = ui.TabInactiveStyle.Render("Commands")
		} else {
			keysTab = ui.TabInactiveStyle.Render("Keys")
			cmdsTab = ui.TabActiveStyle.Render("[Commands]")
		}
		line += "\n" + tabDivider + "\n" + ui.TitleStyle.PaddingLeft(1).Render("") + keysTab + "  " + cmdsTab
	}

	return line
}

func (m Model) renderContent() string {
	if len(m.tools) == 0 {
		return ui.DescStyle.Render("No tools loaded.")
	}

	query := strings.ToLower(strings.TrimSpace(m.search.Value()))
	if m.searching && query != "" {
		return m.renderSearchResults(query)
	}

	if m.rightTab == tabCommands {
		return m.renderCommandsTab(m.tools[m.selected])
	}
	return m.renderTool(m.tools[m.selected])
}

func (m Model) renderTool(t loader.Tool) string {
	var sb strings.Builder
	bindingIdx := 0
	for i, cat := range t.Categories {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(ui.CategoryStyle.Render(cat.Name) + "\n")
		for _, b := range cat.Bindings {
			isSelected := m.focus == focusRight && !m.searching && bindingIdx == m.selectedBinding

			keyStr := b.Key
			descStr := b.Desc

			if isSelected {
				circle := ui.SelectionBarStyle.Render("●")
				line := lipgloss.JoinHorizontal(lipgloss.Top,
					ui.KeyStyle.Render(keyStr),
					ui.DescStyle.Render(descStr),
				)
				sb.WriteString(circle + " " + line + "\n")
			} else {
				line := lipgloss.JoinHorizontal(lipgloss.Top,
					ui.KeyStyle.Render(keyStr),
					ui.DescStyle.Render(descStr),
				)
				sb.WriteString("  " + line + "\n")
			}
			bindingIdx++
		}
	}
	return sb.String()
}

func (m Model) renderSearchResults(query string) string {
	var sb strings.Builder
	found := 0

	for _, t := range m.tools {
		var matches strings.Builder
		for _, cat := range t.Categories {
			for _, b := range cat.Bindings {
				if strings.Contains(strings.ToLower(b.Key), query) ||
					strings.Contains(strings.ToLower(b.Desc), query) {
					line := lipgloss.JoinHorizontal(lipgloss.Top,
						ui.KeyStyle.Render(highlightMatch(b.Key, query)),
						ui.DescStyle.Render(highlightMatch(b.Desc, query)),
					)
					matches.WriteString("  " + line + "\n")
					found++
				}
			}
		}
		if matches.Len() > 0 {
			sb.WriteString(ui.CategoryStyle.Render(t.Name) + "\n")
			sb.WriteString(matches.String())
			sb.WriteString("\n")
		}
	}

	if found == 0 {
		sb.WriteString(ui.DescStyle.Render("No matches found."))
	}
	return sb.String()
}

func highlightMatch(s, query string) string {
	lower := strings.ToLower(s)
	idx := strings.Index(lower, query)
	if idx < 0 {
		return s
	}
	return s[:idx] +
		ui.SearchMatchStyle.Render(s[idx:idx+len(query)]) +
		s[idx+len(query):]
}

// --- helpers ---

func (m Model) totalBindings() int {
	if len(m.tools) == 0 {
		return 0
	}
	return totalBindingsForTool(m.tools[m.selected])
}

func totalBindingsForTool(t loader.Tool) int {
	n := 0
	for _, cat := range t.Categories {
		n += len(cat.Bindings)
	}
	return n
}

func (m Model) bindingAt(idx int) *loader.Binding {
	if len(m.tools) == 0 {
		return nil
	}
	t := m.tools[m.selected]
	i := 0
	for _, cat := range t.Categories {
		for bi := range cat.Bindings {
			if i == idx {
				return &cat.Bindings[bi]
			}
			i++
		}
	}
	return nil
}

func (m Model) hasUpdate(toolName string) bool {
	vi, ok := m.versions[toolName]
	return ok && version.IsNewer(vi.Installed, vi.Latest)
}

func bindingLine(t loader.Tool, bindingIdx int) int {
	line := 0
	bidx := 0
	for i, cat := range t.Categories {
		if i > 0 {
			line++
		}
		line++
		for range cat.Bindings {
			if bidx == bindingIdx {
				return line
			}
			line++
			bidx++
		}
	}
	return 0
}

func (m *Model) scrollToBinding() {
	if len(m.tools) == 0 {
		return
	}
	lineNum := bindingLine(m.tools[m.selected], m.selectedBinding)
	if lineNum < m.viewport.YOffset {
		m.viewport.SetYOffset(lineNum)
	} else if lineNum >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.SetYOffset(lineNum - m.viewport.Height + 1)
	}
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

func (m Model) renderCommandsTab(t loader.Tool) string {
	if len(t.CommandGroups) == 0 {
		return ui.DescStyle.Render("No commands available. Run: keys fetch " + t.Name)
	}

	rightWidth := m.width - leftWidth - 6
	cmdIdx := 0
	var sb strings.Builder

	for i, cg := range t.CommandGroups {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(ui.CategoryStyle.Render(cg.Name) + "\n")
		for _, c := range cg.Commands {
			isSelected := m.focus == focusRight && cmdIdx == m.selectedCommand

			cmdStr := c.Cmd
			maxDesc := rightWidth - len(cmdStr) - 6
			if maxDesc < 10 {
				maxDesc = 10
			}
			descStr := c.Desc
			if len(descStr) > maxDesc {
				descStr = descStr[:maxDesc] + "…"
			}

			rendered := lipgloss.JoinHorizontal(lipgloss.Top,
				ui.CommandCmdStyle.Width(30).Render(cmdStr),
				ui.CommandDescStyle.Render(descStr),
			)

			if isSelected {
				circle := ui.SelectionBarStyle.Render("●")
				sb.WriteString(circle + " " + rendered + "\n")
			} else {
				sb.WriteString("  " + rendered + "\n")
			}
			cmdIdx++
		}
	}
	return sb.String()
}

func (m Model) renderPopup() string {
	popupWidth := min(80, m.width-10)

	cmd := ui.CommandCmdStyle.Render(m.popupCommand.Cmd)
	desc := ui.CommandDescStyle.Width(popupWidth - 4).Render(m.popupCommand.Desc)
	hint := ui.TabInactiveStyle.Render("[y] copy  [esc] close")

	content := lipgloss.JoinVertical(lipgloss.Left,
		cmd,
		"",
		desc,
		"",
		hint,
	)

	return ui.PopupStyle.Width(popupWidth).Render(content)
}

func (m Model) totalCommands() int {
	if len(m.tools) == 0 {
		return 0
	}
	return totalCommandsForTool(m.tools[m.selected])
}

func totalCommandsForTool(t loader.Tool) int {
	n := 0
	for _, cg := range t.CommandGroups {
		n += len(cg.Commands)
	}
	return n
}

func (m Model) commandAt(idx int) *loader.Command {
	if len(m.tools) == 0 {
		return nil
	}
	t := m.tools[m.selected]
	i := 0
	for _, cg := range t.CommandGroups {
		for ci := range cg.Commands {
			if i == idx {
				return &cg.Commands[ci]
			}
			i++
		}
	}
	return nil
}

func commandLine(t loader.Tool, cmdIdx int) int {
	line := 0
	cidx := 0
	for i, cg := range t.CommandGroups {
		if i > 0 {
			line++
		}
		line++
		for range cg.Commands {
			if cidx == cmdIdx {
				return line
			}
			line++
			cidx++
		}
	}
	return 0
}

func (m *Model) scrollToCommand() {
	if len(m.tools) == 0 {
		return
	}
	lineNum := commandLine(m.tools[m.selected], m.selectedCommand)
	if lineNum < m.viewport.YOffset {
		m.viewport.SetYOffset(lineNum)
	} else if lineNum >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.SetYOffset(lineNum - m.viewport.Height + 1)
	}
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
				m.selectedBinding = 0
				m.selectedCommand = 0
				m.rightTab = tabKeys
				m.viewport.Height = m.calcVpHeight()
				m.viewport.GotoTop()
				m.viewport.SetContent(m.renderContent())
			}
			m.focus = focusLeft
		}
	}
	return m, nil
}

// updateHeaderFocus handles key events when the header element is focused.
func (m Model) updateHeaderFocus(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "j", "down":
		m.focus = focusRight
		m.selectedBinding = 0
		m.selectedCommand = 0
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
		// Remove heading markers.
		line = strings.TrimLeft(line, "#")
		line = strings.TrimSpace(line)

		// Remove bold/italic markers ** * __ _
		for _, marker := range []string{"**", "__"} {
			line = strings.ReplaceAll(line, marker, "")
		}
		// Single * and _ only when they appear as emphasis markers (not in middle of words).
		// Simple approach: strip leading/trailing occurrences.
		line = strings.Trim(line, "*_")

		// Remove inline code backticks.
		line = strings.ReplaceAll(line, "`", "")

		// Remove HTML tags.
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
	// 2 border + 2 padding each side = 8 columns overhead for PopupStyle-like style
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

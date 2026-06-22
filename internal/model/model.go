package model

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

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

type helpOutputMsg struct {
	toolName string
	mode     int
	output   string
	err      error
}

const (
	helpModeHelp = 0
	helpModeMan  = 1
)

type Model struct {
	tools               []loader.Tool
	versions            map[string]VersionInfo
	repoStatus          map[string]string
	repoCards           map[string]version.RepoCard
	changelogData       map[string]changelogMsg
	changelogLoadingFor string
	checkingVersionTool string
	focus               int
	cardViewport        viewport.Model
	helpViewport        viewport.Model
	search              textinput.Model
	searching           bool
	statusMsg           string
	width               int
	height              int
	ready               bool

	editingNote bool
	editingTags bool
	noteInput   textinput.Model
	tagsInput   textinput.Model

	meta         []loader.ToolMeta
	metaFilter   loader.Status
	metaSelected int

	helpMode      int
	helpLoadingFor string
	helpCache     map[string][2]string
	helpSearching bool
	helpSearch    textinput.Model
	helpMatches   []int
	helpMatchIdx  int
}

type Options struct {
	InitialTool   string
	InitialSearch string
}

func New(meta []loader.ToolMeta, opts Options) Model {
	ti := textinput.New()
	ti.Placeholder = "search..."
	ti.CharLimit = 64

	ni := textinput.New()
	ni.Placeholder = "note..."
	ni.CharLimit = 256

	tgi := textinput.New()
	tgi.Placeholder = "tag1, tag2..."
	tgi.CharLimit = 256

	hsi := textinput.New()
	hsi.Placeholder = "search help..."
	hsi.CharLimit = 128

	m := Model{
		tools:         loader.ToolsFromMeta(meta),
		versions:      make(map[string]VersionInfo),
		repoStatus:    make(map[string]string),
		repoCards:     make(map[string]version.RepoCard),
		changelogData: make(map[string]changelogMsg),
		helpCache:     make(map[string][2]string),
		search:        ti,
		noteInput:     ni,
		tagsInput:     tgi,
		helpSearch:    hsi,
		meta:          meta,
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
	// Auto-fetch --help for initial selected tool
	if mt, ok := m.selectedMeta(); ok {
		cached := m.helpCache[mt.Name]
		if cached[helpModeHelp] == "" {
			cmds = append(cmds, fetchHelpCmd(mt.Name, helpModeHelp))
		}
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
		m.cardViewport.SetContent(m.renderCard())
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
		m.cardViewport.SetContent(m.renderCard())
		return m, nil

	case changelogMsg:
		if msg.toolName == m.changelogLoadingFor {
			m.changelogLoadingFor = ""
		}
		m.changelogData[msg.toolName] = msg
		m.cardViewport.SetContent(m.renderCard())
		return m, nil

	case repoCardMsg:
		if msg.err == nil {
			m.repoCards[msg.toolName] = msg.card
			m.cardViewport.SetContent(m.renderCard())
		}
		return m, nil

	case helpOutputMsg:
		m.helpLoadingFor = ""
		cached := m.helpCache[msg.toolName]
		if msg.err == nil && msg.output != "" {
			cached[msg.mode] = msg.output
		} else if msg.mode == helpModeHelp {
			cached[msg.mode] = "--help not available"
		} else {
			cached[msg.mode] = "man page not available"
		}
		m.helpCache[msg.toolName] = cached
		if mt, ok := m.selectedMeta(); ok && mt.Name == msg.toolName {
			m.helpViewport.SetContent(m.renderHelpContent())
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		cardW, helpW := m.calcPanelWidths()
		vpH := m.calcVpHeight()
		if !m.ready {
			m.cardViewport = viewport.New(cardW, vpH)
			m.helpViewport = viewport.New(helpW, vpH)
			m.helpViewport.SetContent(m.renderHelpContent())
			m.ready = true
		} else {
			m.cardViewport.Width = cardW
			m.cardViewport.Height = vpH
			m.helpViewport.Width = helpW
			m.helpViewport.Height = vpH
		}
		m.cardViewport.SetContent(m.renderCard())
		return m, nil

	case tea.KeyMsg:
		m.statusMsg = ""

		if m.focus == focusHeader {
			return m.updateHeaderFocus(msg)
		}

		if m.editingNote {
			return m.updateNoteEdit(msg)
		}
		if m.editingTags {
			return m.updateTagsEdit(msg)
		}

		if m.helpSearching {
			switch msg.String() {
			case "esc":
				m.helpSearching = false
				m.helpSearch.SetValue("")
				m.helpSearch.Blur()
				m.helpMatches = nil
				m.helpMatchIdx = 0
				m.helpViewport.SetContent(m.renderHelpContent())
				return m, nil
			case "n":
				if len(m.helpMatches) > 0 {
					m.helpMatchIdx = (m.helpMatchIdx + 1) % len(m.helpMatches)
					m.helpViewport.SetYOffset(m.helpMatches[m.helpMatchIdx])
				}
				return m, nil
			case "N":
				if len(m.helpMatches) > 0 {
					m.helpMatchIdx = (m.helpMatchIdx - 1 + len(m.helpMatches)) % len(m.helpMatches)
					m.helpViewport.SetYOffset(m.helpMatches[m.helpMatchIdx])
				}
				return m, nil
			default:
				m.helpSearch, cmd = m.helpSearch.Update(msg)
				query := m.helpSearch.Value()
				m.helpMatches = findMatches(m.rawHelpText(), query)
				m.helpMatchIdx = 0
				m.helpViewport.SetContent(m.renderHelpContent())
				if len(m.helpMatches) > 0 {
					m.helpViewport.SetYOffset(m.helpMatches[0])
				}
				return m, cmd
			}
		}

		if m.searching {
			switch msg.String() {
			case "esc":
				m.searching = false
				m.search.SetValue("")
				m.search.Blur()
				m.metaSelected = 0
				m.cardViewport.SetContent(m.renderCard())
				return m, nil
			default:
				m.search, cmd = m.search.Update(msg)
				m.metaSelected = 0
				m.cardViewport.SetContent(m.renderCard())
				m.cardViewport.GotoTop()
				return m, cmd
			}
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "esc":
			if m.focus == focusRight {
				m.focus = focusLeft
				m.cardViewport.SetContent(m.renderCard())
			} else {
				return m, tea.Quit
			}

		case "right", "l":
			if m.focus == focusLeft {
				m.focus = focusHeader
				m.cardViewport.SetContent(m.renderCard())
			}

		case "left":
			if m.focus == focusRight {
				m.focus = focusLeft
				m.cardViewport.SetContent(m.renderCard())
			}

		case "j", "down":
			if m.focus == focusLeft {
				filtered := m.filteredMeta()
				if m.metaSelected < len(filtered)-1 {
					m.metaSelected++
					m.cardViewport.Height = m.calcVpHeight()
					m.cardViewport.GotoTop()
					m.cardViewport.SetContent(m.renderCard())
					// auto-fetch --help for newly selected tool
					if mt, ok := m.selectedMeta(); ok {
						cached := m.helpCache[mt.Name]
						if cached[m.helpMode] == "" {
							m.helpLoadingFor = mt.Name
							m.helpViewport.SetContent(m.renderHelpContent())
							return m, fetchHelpCmd(mt.Name, m.helpMode)
						}
						m.helpViewport.SetContent(m.renderHelpContent())
						m.helpViewport.GotoTop()
					}
				}
			}

		case "k", "up":
			if m.focus == focusLeft {
				if m.metaSelected > 0 {
					m.metaSelected--
					m.cardViewport.Height = m.calcVpHeight()
					m.cardViewport.GotoTop()
					m.cardViewport.SetContent(m.renderCard())
					// auto-fetch --help for newly selected tool
					if mt, ok := m.selectedMeta(); ok {
						cached := m.helpCache[mt.Name]
						if cached[m.helpMode] == "" {
							m.helpLoadingFor = mt.Name
							m.helpViewport.SetContent(m.renderHelpContent())
							return m, fetchHelpCmd(mt.Name, m.helpMode)
						}
						m.helpViewport.SetContent(m.renderHelpContent())
						m.helpViewport.GotoTop()
					}
				}
			}

		case "g":
			m.cardViewport.GotoTop()

		case "G":
			m.cardViewport.GotoBottom()

		case "/":
			if m.focus == focusRight {
				m.helpSearching = true
				m.helpSearch.Focus()
				return m, textinput.Blink
			}
			m.searching = true
			m.search.Focus()
			return m, textinput.Blink

		case "h":
			if m.focus == focusRight {
				m.helpMode = helpModeHelp
				if mt, ok := m.selectedMeta(); ok {
					cached := m.helpCache[mt.Name]
					if cached[helpModeHelp] == "" {
						m.helpLoadingFor = mt.Name
						m.helpViewport.SetContent(m.renderHelpContent())
						return m, fetchHelpCmd(mt.Name, helpModeHelp)
					}
					m.helpViewport.SetContent(m.renderHelpContent())
					m.helpViewport.GotoTop()
				}
			}

		case "m":
			if m.focus == focusRight {
				m.helpMode = helpModeMan
				if mt, ok := m.selectedMeta(); ok {
					cached := m.helpCache[mt.Name]
					if cached[helpModeMan] == "" {
						m.helpLoadingFor = mt.Name
						m.helpViewport.SetContent(m.renderHelpContent())
						return m, fetchHelpCmd(mt.Name, helpModeMan)
					}
					m.helpViewport.SetContent(m.renderHelpContent())
					m.helpViewport.GotoTop()
				}
			}

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
				m.cardViewport.SetContent(m.renderCard())
			}

		case "1":
			m.metaFilter = loader.StatusActive
			m.metaSelected = 0
			m.cardViewport.SetContent(m.renderCard())
		case "2":
			m.metaFilter = loader.StatusTrying
			m.metaSelected = 0
			m.cardViewport.SetContent(m.renderCard())
		case "3":
			m.metaFilter = loader.StatusForgotten
			m.metaSelected = 0
			m.cardViewport.SetContent(m.renderCard())
		case "4":
			m.metaFilter = loader.StatusArchived
			m.metaSelected = 0
			m.cardViewport.SetContent(m.renderCard())
		case "a":
			m.metaFilter = ""
			m.metaSelected = 0
			m.cardViewport.SetContent(m.renderCard())

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
			if t, ok := m.selectedTool(); ok && t.GitHub != "" {
				if _, already := m.changelogData[t.Name]; !already && m.changelogLoadingFor != t.Name {
					m.changelogLoadingFor = t.Name
					m.cardViewport.SetContent(m.renderCard())
					return m, fetchChangelogCmd(t.GitHub, t.Name)
				}
			}

		case "e":
			if m.focus == focusRight {
				if mt, ok := m.selectedMeta(); ok {
					m.editingNote = true
					m.noteInput.SetValue(mt.Note)
					m.noteInput.Focus()
					m.cardViewport.SetContent(m.renderCard())
					return m, textinput.Blink
				}
			}

		case "t":
			if m.focus == focusRight {
				if mt, ok := m.selectedMeta(); ok {
					m.editingTags = true
					m.tagsInput.SetValue(strings.Join(mt.Tags, ", "))
					m.tagsInput.Focus()
					m.cardViewport.SetContent(m.renderCard())
					return m, textinput.Blink
				}
			}
		}

		if m.focus == focusRight {
			m.cardViewport, cmd = m.cardViewport.Update(msg)
		}
	}

	return m, cmd
}

func (m Model) updateNoteEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.editingNote = false
		m.noteInput.Blur()
		if mt, ok := m.selectedMeta(); ok {
			mt.Note = strings.TrimSpace(m.noteInput.Value())
			m.meta = loader.UpsertMeta(m.meta, mt)
			loader.SaveMeta(m.meta) //nolint:errcheck
		}
		m.cardViewport.SetContent(m.renderCard())
		return m, nil
	case "esc":
		m.editingNote = false
		m.noteInput.Blur()
		m.cardViewport.SetContent(m.renderCard())
		return m, nil
	default:
		var cmd tea.Cmd
		m.noteInput, cmd = m.noteInput.Update(msg)
		m.cardViewport.SetContent(m.renderCard())
		return m, cmd
	}
}

func (m Model) updateTagsEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.editingTags = false
		m.tagsInput.Blur()
		if mt, ok := m.selectedMeta(); ok {
			raw := strings.TrimSpace(m.tagsInput.Value())
			var tags []string
			for _, t := range strings.Split(raw, ",") {
				t = strings.TrimSpace(t)
				if t != "" {
					tags = append(tags, t)
				}
			}
			mt.Tags = tags
			m.meta = loader.UpsertMeta(m.meta, mt)
			loader.SaveMeta(m.meta) //nolint:errcheck
		}
		m.cardViewport.SetContent(m.renderCard())
		return m, nil
	case "esc":
		m.editingTags = false
		m.tagsInput.Blur()
		m.cardViewport.SetContent(m.renderCard())
		return m, nil
	default:
		var cmd tea.Cmd
		m.tagsInput, cmd = m.tagsInput.Update(msg)
		m.cardViewport.SetContent(m.renderCard())
		return m, cmd
	}
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
	return lipgloss.NewStyle().Margin(1).Render(layout)
}

func (m Model) renderHelp() string {
	style := ui.HelpStyle.Width(m.width - 4)
	if m.helpSearching {
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
	if m.searching {
		return style.Render(fmt.Sprintf(
			"%s %s  %s exit search",
			ui.SearchPromptStyle.Render("/"),
			m.search.View(),
			keyHint("esc"),
		))
	}
	if m.editingNote {
		return style.Render(keyHint("enter") + " save  " + keyHint("esc") + " cancel")
	}
	if m.editingTags {
		return style.Render(keyHint("enter") + " save  " + keyHint("esc") + " cancel  " + ui.MetaNoteStyle.Render("comma-separated"))
	}
	if m.statusMsg != "" {
		return style.Render(ui.SearchPromptStyle.Render(m.statusMsg))
	}
	if m.focus == focusHeader {
		hints := keyHint("↓/j") + " select  " + keyHint("←/esc") + " back  " + keyHint("q") + " quit"
		if t, ok := m.selectedTool(); ok && t.GitHub != "" {
			hints = keyHint("v") + " check version  " + hints
		}
		return style.Render(hints)
	}
	if m.focus == focusRight {
		hints := keyHint("h") + " --help  " + keyHint("m") + " man  " + keyHint("/") + " search  " + keyHint("o") + " github  " + keyHint("c") + " changelog  " + keyHint("e") + " edit note  " + keyHint("t") + " edit tags  " + keyHint("←/esc") + " back  " + keyHint("q") + " quit"
		return style.Render(hints)
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
	return max(m.height-10, 1)
}

func (m Model) calcPanelWidths() (cardW, helpW int) {
	rightTotal := max(m.width-leftWidth-6, 4)
	cardW = rightTotal / 2
	helpW = rightTotal - cardW
	return
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
	if m.focus == focusLeft {
		panelStyle = ui.PanelBorderFocused
	}

	return panelStyle.
		Width(leftWidth).
		Height(max(m.height-7, 1)).
		Render(content)
}

func (m Model) renderRight() string {
	rightTotal := max(m.width-leftWidth-6, 4)
	cardW, _ := m.calcPanelWidths()

	header := m.renderRightHeader()

	dividerWidth := max(rightTotal-2, 0)
	divider := lipgloss.NewStyle().Foreground(ui.ColorBorder).Render(strings.Repeat("─", dividerWidth))

	cardBox := lipgloss.NewStyle().
		Width(cardW).
		BorderRight(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(ui.ColorBorder).
		Render(m.cardViewport.View())

	helpBox := m.helpViewport.View()

	panels := lipgloss.JoinHorizontal(lipgloss.Top, cardBox, helpBox)

	panelStyle := ui.PanelBorder
	if m.focus == focusRight || m.focus == focusHeader {
		panelStyle = ui.PanelBorderFocused
	}

	inner := lipgloss.JoinVertical(lipgloss.Left, header, divider, panels)
	return panelStyle.
		Width(rightTotal).
		Height(max(m.height-7, 1)).
		Render(inner)
}

func (m Model) renderRightHeader() string {
	prefix := ""
	if m.focus == focusHeader {
		prefix = ui.SelectionBarStyle.Render("●") + " "
	}

	if m.searching {
		query := m.search.Value()
		return ui.TitleStyle.Render("Search: ") + ui.SearchMatchStyle.Render(query)
	}

	mt, ok := m.selectedMeta()
	if !ok {
		return ui.TitleStyle.Render("No tool selected")
	}

	sym := loader.StatusSymbol[mt.Status]
	symStyled := ui.StatusStyle(mt.Status).Render(sym)
	return prefix + symStyled + " " + ui.TitleStyle.Render(mt.Name)
}

func (m Model) renderCard() string {
	if len(m.meta) == 0 {
		return ui.DescStyle.Render("No tools tracked.\nAdd one: keys track <tool> --github <repo>")
	}

	t, ok := m.selectedTool()
	if !ok {
		return ui.DescStyle.Render("Select a tool from the left panel.")
	}

	cardW, _ := m.calcPanelWidths()
	inner := max(cardW-2, 1)
	divW := max(cardW-4, 1)
	divider := "\n" + lipgloss.NewStyle().Foreground(ui.ColorBorder).Render(strings.Repeat("─", divW)) + "\n"

	var sb strings.Builder

	// About block
	if card, ok := m.repoCards[t.Name]; ok && card.About != "" {
		sb.WriteString(ui.DescStyle.Render(wrapText(card.About, inner)) + "\n")
	}
	if t.GitHub != "" {
		sb.WriteString(ui.GithubStyle.Render(wrapText("↗ https://"+t.GitHub, inner)) + "\n")
	}

	sb.WriteString(divider)

	// Stars + Release + Languages block
	if card, ok := m.repoCards[t.Name]; ok {
		if card.Stars > 0 {
			sb.WriteString(ui.MetaNoteStyle.Render(fmt.Sprintf("★ %s stars", formatStars(card.Stars))) + "\n")
		}
		if card.Latest != "" {
			line := "Latest: " + card.Latest
			if card.PublishedAt != "" {
				date := card.PublishedAt
				if len(date) > 10 {
					date = date[:10]
				}
				line += " (" + date + ")"
			}
			sb.WriteString(ui.MetaNoteStyle.Render(line) + "\n")
		}
		if len(card.Languages) > 0 {
			sb.WriteString(renderLangBar(card.Languages, cardW-4) + "\n")
		}
		if card.RepoStatus != "" {
			sb.WriteString(ui.RepoStatusStyle.Render(card.RepoStatus) + "\n")
		}
	}

	sb.WriteString(divider)

	// Note + Tags block (with inline editing)
	if mt, ok := m.selectedMeta(); ok {
		if m.editingNote {
			sb.WriteString(ui.MetaDetailLabelStyle.Render("Note:") + " " + m.noteInput.View() + "\n")
		} else {
			noteText := mt.Note
			if noteText == "" {
				noteText = "— (press e to edit)"
			}
			sb.WriteString(ui.MetaDetailLabelStyle.Render("Note:") + " " + ui.MetaNoteStyle.Render(noteText) + "\n")
		}

		if m.editingTags {
			sb.WriteString(ui.MetaDetailLabelStyle.Render("Tags:") + " " + m.tagsInput.View() + "\n")
		} else {
			tagsText := strings.Join(mt.Tags, ", ")
			if tagsText == "" {
				tagsText = "— (press t to edit)"
			}
			sb.WriteString(ui.MetaDetailLabelStyle.Render("Tags:") + " " + ui.MetaTagStyle.Render(tagsText) + "\n")
		}
	}

	sb.WriteString(divider)

	// Changelog block
	if m.changelogLoadingFor == t.Name {
		sb.WriteString(ui.DescStyle.Render("Loading changelog...") + "\n")
	} else if data, ok := m.changelogData[t.Name]; ok {
		sb.WriteString(m.renderChangelogBlock(data))
	} else if t.GitHub != "" {
		sb.WriteString(ui.MetaNoteStyle.Render("Press [c] to load changelog") + "\n")
	}

	return sb.String()
}

func (m Model) renderChangelogBlock(msg changelogMsg) string {
	if msg.err != nil {
		return ui.DescStyle.Render("Changelog unavailable: " + msg.err.Error()) + "\n"
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
	if msg.htmlUrl != "" {
		sb.WriteString(ui.GithubStyle.Render("↗ "+msg.htmlUrl) + "\n")
	}
	sb.WriteString("\n")
	cardW, _ := m.calcPanelWidths()
	body := wrapText(stripMarkdown(msg.body), max(cardW-6, 10))
	if body == "" {
		sb.WriteString(ui.DescStyle.Render("No release notes available.") + "\n")
	} else {
		sb.WriteString(ui.DescStyle.Render(body) + "\n")
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
			m.cardViewport, cmd = m.cardViewport.Update(msg)
			return m, cmd
		case tea.MouseButtonLeft:
			if msg.Action == tea.MouseActionPress && m.focus != focusRight {
				m.focus = focusRight
				m.cardViewport.SetContent(m.renderCard())
			}
		}
		return m, nil
	}

	if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
		toolIdx := msg.Y - 3
		filtered := m.filteredMeta()
		if toolIdx >= 0 && toolIdx < len(filtered) {
			if m.metaSelected != toolIdx {
				m.metaSelected = toolIdx
				m.cardViewport.Height = m.calcVpHeight()
				m.cardViewport.GotoTop()
				m.cardViewport.SetContent(m.renderCard())
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
	sort.Slice(out, func(i, j int) bool { return out[i].Pct > out[j].Pct })
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
		m.cardViewport.GotoTop()
		m.cardViewport.SetContent(m.renderCard())

	case "left", "esc":
		m.focus = focusLeft
		m.cardViewport.SetContent(m.renderCard())

	case "v":
		if m.checkingVersionTool == "" {
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
		if t, ok := m.selectedTool(); ok && t.GitHub != "" {
			if _, already := m.changelogData[t.Name]; !already && m.changelogLoadingFor != t.Name {
				m.changelogLoadingFor = t.Name
				m.cardViewport.SetContent(m.renderCard())
				return m, fetchChangelogCmd(t.GitHub, t.Name)
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

func fetchRepoCardCmd(t loader.Tool) tea.Cmd {
	return func() tea.Msg {
		card := version.GetRepoCard(t.GitHub)
		return repoCardMsg{toolName: t.Name, card: card}
	}
}

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

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

func fetchHelpCmd(name string, mode int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var output []byte
		var err error

		if mode == helpModeHelp {
			cmd := exec.CommandContext(ctx, name, "--help")
			output, err = cmd.CombinedOutput()
			if (err != nil || len(output) == 0) && ctx.Err() == nil {
				cmd2 := exec.CommandContext(ctx, name, "-h")
				out2, err2 := cmd2.CombinedOutput()
				if len(out2) > 0 {
					output, err = out2, err2
				}
			}
		} else {
			cmd := exec.CommandContext(ctx, "man", name)
			cmd.Env = append(os.Environ(), "MANPAGER=cat", "MANWIDTH=80", "TERM=dumb")
			output, err = cmd.Output()
		}

		if len(output) == 0 {
			return helpOutputMsg{toolName: name, mode: mode, err: err}
		}
		return helpOutputMsg{toolName: name, mode: mode, output: stripANSI(string(output))}
	}
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
	ll := strings.ToLower(line)
	lq := strings.ToLower(query)
	idx := strings.Index(ll, lq)
	if idx < 0 {
		return line
	}
	return line[:idx] + ui.SearchMatchStyle.Render(line[idx:idx+len(query)]) + line[idx+len(query):]
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

	if m.helpLoadingFor != "" {
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
	_, helpW := m.calcPanelWidths()
	if innerW := max(helpW-2, 20); innerW > 0 {
		text = wrapText(text, innerW)
	}
	if !m.helpSearching || m.helpSearch.Value() == "" {
		return text
	}
	query := m.helpSearch.Value()
	lines := strings.Split(text, "\n")
	result := make([]string, len(lines))
	for i, line := range lines {
		result[i] = highlightMatch(line, query)
	}
	return strings.Join(result, "\n")
}

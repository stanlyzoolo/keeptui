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
	focusTools  = 0
	focusBrief  = 1
	focusHeader = 2
	focusHelp   = 3
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
	toolsViewport       viewport.Model
	briefViewport       viewport.Model
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

	toolsW int
	briefW int
	helpW  int
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
				m.focus = focusBrief
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
	// Auto-fetch --help and changelog for initial selected tool
	if mt, ok := m.selectedMeta(); ok {
		cached := m.helpCache[mt.Name]
		if cached[helpModeHelp] == "" {
			cmds = append(cmds, fetchHelpCmd(mt.Name, helpModeHelp))
		}
	}
	if t, ok := m.selectedTool(); ok && t.GitHub != "" {
		cmds = append(cmds, fetchChangelogCmd(t.GitHub, t.Name))
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
		m.toolsViewport.SetContent(m.renderLeftContent())
		m.briefViewport.SetContent(m.renderCard())
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
		m.toolsViewport.SetContent(m.renderLeftContent())
		m.briefViewport.SetContent(m.renderCard())
		return m, nil

	case changelogMsg:
		if msg.toolName == m.changelogLoadingFor {
			m.changelogLoadingFor = ""
		}
		m.changelogData[msg.toolName] = msg
		m.briefViewport.SetContent(m.renderCard())
		return m, nil

	case repoCardMsg:
		if msg.err == nil {
			m.repoCards[msg.toolName] = msg.card
			m.briefViewport.SetContent(m.renderCard())
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
		m.toolsW, m.briefW, m.helpW = m.calcPanelWidths()
		vpH := m.calcVpHeight()
		leftVpH := max(max(m.height-7, 1)-2, 1)
		if !m.ready {
			m.toolsViewport = viewport.New(m.toolsW-2, leftVpH)
			m.briefViewport = viewport.New(m.briefW, vpH)
			m.helpViewport = viewport.New(m.helpW, vpH)
			m.helpViewport.SetContent(m.renderHelpContent())
			m.ready = true
		} else {
			m.toolsViewport.Width = m.toolsW - 2
			m.toolsViewport.Height = leftVpH
			m.briefViewport.Width = m.briefW
			m.briefViewport.Height = vpH
			m.helpViewport.Width = m.helpW
			m.helpViewport.Height = vpH
		}
		m.toolsViewport.SetContent(m.renderLeftContent())
		m.syncToolsViewport()
		m.briefViewport.SetContent(m.renderCard())
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
				m.setToolsContent()
				m.briefViewport.SetContent(m.renderCard())
				return m, nil
			default:
				m.search, cmd = m.search.Update(msg)
				m.metaSelected = 0
				m.setToolsContent()
				m.briefViewport.SetContent(m.renderCard())
				m.briefViewport.GotoTop()
				return m, cmd
			}
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "esc":
			if m.focus == focusHelp {
				m.focus = focusBrief
				m.briefViewport.SetContent(m.renderCard())
			} else if m.focus == focusBrief {
				m.focus = focusTools
				m.setToolsContent()
				m.briefViewport.SetContent(m.renderCard())
			} else {
				return m, tea.Quit
			}

		case "right", "l":
			if m.focus == focusTools {
				m.focus = focusBrief
				m.setToolsContent()
				m.briefViewport.SetContent(m.renderCard())
			} else if m.focus == focusBrief {
				m.focus = focusHelp
				m.helpViewport.SetContent(m.renderHelpContent())
			}

		case "left":
			if m.focus == focusHelp {
				m.focus = focusBrief
				m.briefViewport.SetContent(m.renderCard())
			} else if m.focus == focusBrief {
				m.focus = focusTools
				m.setToolsContent()
				m.briefViewport.SetContent(m.renderCard())
			}

		case "j", "down":
			if m.focus == focusTools {
				filtered := m.filteredMeta()
				if m.metaSelected < len(filtered)-1 {
					m.metaSelected++
					m.setToolsContent()
					m.briefViewport.Height = m.calcVpHeight()
					m.briefViewport.GotoTop()
					m.briefViewport.SetContent(m.renderCard())
					return m, m.autoFetchCmdsForSelected()
				}
			}

		case "k", "up":
			if m.focus == focusTools {
				if m.metaSelected > 0 {
					m.metaSelected--
					m.setToolsContent()
					m.briefViewport.Height = m.calcVpHeight()
					m.briefViewport.GotoTop()
					m.briefViewport.SetContent(m.renderCard())
					return m, m.autoFetchCmdsForSelected()
				}
			}

		case "pgup", "ctrl+b":
			if m.focus == focusTools {
				step := max(m.toolsViewport.Height, 1)
				m.metaSelected = max(m.metaSelected-step, 0)
				m.setToolsContent()
				m.briefViewport.GotoTop()
				m.briefViewport.SetContent(m.renderCard())
				return m, m.autoFetchCmdsForSelected()
			}

		case "pgdown", "ctrl+f":
			if m.focus == focusTools {
				filtered := m.filteredMeta()
				step := max(m.toolsViewport.Height, 1)
				m.metaSelected = min(m.metaSelected+step, max(len(filtered)-1, 0))
				m.setToolsContent()
				m.briefViewport.GotoTop()
				m.briefViewport.SetContent(m.renderCard())
				return m, m.autoFetchCmdsForSelected()
			}

		case "g":
			if m.focus == focusBrief {
				m.briefViewport.GotoTop()
			} else if m.focus == focusHelp {
				m.helpViewport.GotoTop()
			}

		case "G":
			if m.focus == focusBrief {
				m.briefViewport.GotoBottom()
			} else if m.focus == focusHelp {
				m.helpViewport.GotoBottom()
			}

		case "/":
			if m.focus == focusBrief || m.focus == focusHelp {
				m.helpSearching = true
				m.helpSearch.Focus()
				return m, textinput.Blink
			}
			m.searching = true
			m.search.Focus()
			return m, textinput.Blink

		case "h":
			if m.focus == focusBrief || m.focus == focusHelp {
				m.focus = focusHelp
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
			if m.focus == focusBrief || m.focus == focusHelp {
				m.focus = focusHelp
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
			if m.focus == focusTools {
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
				m.setToolsContent()
				m.briefViewport.SetContent(m.renderCard())
			}

		case "1":
			m.metaFilter = loader.StatusActive
			m.metaSelected = 0
			m.setToolsContent()
			m.briefViewport.SetContent(m.renderCard())
		case "2":
			m.metaFilter = loader.StatusTrying
			m.metaSelected = 0
			m.setToolsContent()
			m.briefViewport.SetContent(m.renderCard())
		case "3":
			m.metaFilter = loader.StatusForgotten
			m.metaSelected = 0
			m.setToolsContent()
			m.briefViewport.SetContent(m.renderCard())
		case "4":
			m.metaFilter = loader.StatusArchived
			m.metaSelected = 0
			m.setToolsContent()
			m.briefViewport.SetContent(m.renderCard())
		case "a":
			m.metaFilter = ""
			m.metaSelected = 0
			m.setToolsContent()
			m.briefViewport.SetContent(m.renderCard())

		case "v":
			if m.focus == focusTools && m.checkingVersionTool == "" {
				if t, ok := m.selectedTool(); ok && t.GitHub != "" {
					m.checkingVersionTool = t.Name
					return m, fetchVersionCmd(t)
				}
			}

		case "o":
			if t, ok := m.selectedTool(); ok && t.GitHub != "" {
				openBrowser("https://" + t.GitHub)
			}

		case "e":
			if m.focus == focusBrief {
				if mt, ok := m.selectedMeta(); ok {
					m.editingNote = true
					m.noteInput.SetValue(mt.Note)
					m.noteInput.Focus()
					m.briefViewport.SetContent(m.renderCard())
					return m, textinput.Blink
				}
			}

		case "t":
			if m.focus == focusBrief {
				if mt, ok := m.selectedMeta(); ok {
					m.editingTags = true
					m.tagsInput.SetValue(strings.Join(mt.Tags, ", "))
					m.tagsInput.Focus()
					m.briefViewport.SetContent(m.renderCard())
					return m, textinput.Blink
				}
			}
		}

		if m.focus == focusBrief {
			m.briefViewport, cmd = m.briefViewport.Update(msg)
		} else if m.focus == focusHelp {
			m.helpViewport, cmd = m.helpViewport.Update(msg)
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
		m.briefViewport.SetContent(m.renderCard())
		return m, nil
	case "esc":
		m.editingNote = false
		m.noteInput.Blur()
		m.briefViewport.SetContent(m.renderCard())
		return m, nil
	default:
		var cmd tea.Cmd
		m.noteInput, cmd = m.noteInput.Update(msg)
		m.briefViewport.SetContent(m.renderCard())
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
		m.briefViewport.SetContent(m.renderCard())
		return m, nil
	case "esc":
		m.editingTags = false
		m.tagsInput.Blur()
		m.briefViewport.SetContent(m.renderCard())
		return m, nil
	default:
		var cmd tea.Cmd
		m.tagsInput, cmd = m.tagsInput.Update(msg)
		m.briefViewport.SetContent(m.renderCard())
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

	left := m.renderTools()
	middle := m.renderBrief()
	right := m.renderHelp()
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, middle, right)
	layout := lipgloss.JoinVertical(lipgloss.Left, body, m.renderStatusBar())
	return lipgloss.NewStyle().Margin(1).Render(layout)
}

func (m Model) renderStatusBar() string {
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
	if m.focus == focusBrief {
		hints := keyHint("↑↓") + " scroll  " + keyHint("→") + " help  " + keyHint("←") + " back  " + keyHint("e") + " edit note  " + keyHint("t") + " edit tags  " + keyHint("q") + " quit"
		return style.Render(hints)
	}
	if m.focus == focusHelp {
		hints := keyHint("↑↓") + " scroll  " + keyHint("h") + " --help  " + keyHint("m") + " man  " + keyHint("/") + " search  " + keyHint("←") + " back  " + keyHint("q") + " quit"
		return style.Render(hints)
	}
	filterHint := ""
	if m.metaFilter != "" {
		filterHint = keyHint("a") + " all  "
	}
	versionHint := ""
	if t, ok := m.selectedTool(); ok && t.GitHub != "" {
		versionHint = keyHint("v") + " check  "
	}
	return style.Render(
		keyHint("j/k") + " navigate  " +
			keyHint("→") + " details  " +
			keyHint("f") + " filter  " +
			filterHint +
			keyHint("/") + " search  " +
			versionHint +
			keyHint("o") + " github  " +
			keyHint("q") + " quit",
	)
}

func keyHint(k string) string {
	return ui.SearchPromptStyle.Render("[" + k + "]")
}

func (m Model) calcVpHeight() int {
	return max(m.height-10, 1)
}

func (m Model) calcPanelWidths() (toolsW, briefW, helpW int) {
	// 20%-40%-40% layout with 6 chars overhead (2 border chars per panel)
	available := max(m.width-6, 1)
	toolsW = max((available * 20) / 100, 15)
	briefW = max((available * 40) / 100, 30)
	helpW = available - toolsW - briefW
	if helpW < 30 {
		helpW = 30
		briefW = available - toolsW - helpW
		if briefW < 30 {
			briefW = 30
			toolsW = available - briefW - helpW
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

		isSelected := i == m.metaSelected && m.focus == focusTools && !m.searching
		if isSelected {
			circle := ui.SelectionBarStyle.Render("●")
			sb.WriteString(circle + " " + name + updateMark + "\n")
		} else {
			sb.WriteString("  " + name + updateMark + "\n")
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
		Render(m.toolsViewport.View())
}

func (m Model) renderBrief() string {
	header := m.renderRightHeader()
	dividerWidth := max(m.briefW-2, 0)
	divider := lipgloss.NewStyle().Foreground(ui.ColorBorder).Render(strings.Repeat("─", dividerWidth))

	panelStyle := ui.PanelBorder
	if m.focus == focusBrief || m.focus == focusHeader {
		panelStyle = ui.PanelBorderFocused
	}

	inner := lipgloss.JoinVertical(lipgloss.Left, header, divider, m.briefViewport.View())
	return panelStyle.
		Width(m.briefW).
		Height(max(m.height-7, 1)).
		Render(inner)
}

func (m Model) renderHelp() string {
	panelStyle := ui.PanelBorder
	if m.focus == focusHelp {
		panelStyle = ui.PanelBorderFocused
	}

	panelStyle = panelStyle.BorderRight(true)

	return panelStyle.
		Width(m.helpW).
		Height(max(m.height-7, 1)).
		Render(m.helpViewport.View())
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

	inner := max(m.briefW-2, 1)
	divW := max(m.briefW-2, 1)
	divider := "\n" + lipgloss.NewStyle().Foreground(ui.ColorBorder).Render(strings.Repeat("─", divW)) + "\n"

	var sb strings.Builder

	// About block - format as: name (orange bold) about_text (gray italic)
	if card, ok := m.repoCards[t.Name]; ok && card.About != "" {
		// Tool name in bold orange, truncate if very long to prevent overflow
		name := t.Name
		maxNameLen := 30
		if utf8.RuneCountInString(name) > maxNameLen {
			name = name[:maxNameLen-3] + "..."
		}
		nameStyle := lipgloss.NewStyle().Bold(true).Foreground(ui.ColorOrange)
		nameRendered := nameStyle.Render(name)

		// About text in gray italic, wrapped to fit within panel
		aboutWidth := max(inner-utf8.RuneCountInString(name)-1, 20)
		aboutWrapped := wrapText(card.About, aboutWidth)
		aboutRendered := ui.MetaNoteStyle.Render(aboutWrapped)

		sb.WriteString(nameRendered + " " + aboutRendered + "\n")
	}

	sb.WriteString(divider)

	// Repo link
	if t.GitHub != "" {
		sb.WriteString(ui.GithubStyle.Render("repo: "+t.GitHub) + "\n")
	}

	// Stars + Release + Languages block
	if card, ok := m.repoCards[t.Name]; ok {
		if card.Stars > 0 {
			sb.WriteString(ui.MetaNoteStyle.Render(fmt.Sprintf("Stars: %s", formatStars(card.Stars))) + "\n")
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
			sb.WriteString(renderLangBar(card.Languages, inner) + "\n")
		}
		if card.RepoStatus != "" {
			sb.WriteString(ui.RepoStatusStyle.Render(card.RepoStatus) + "\n")
		}
	}

	sb.WriteString(divider)

	// Status + Note + Tags block (with inline editing)
	if mt, ok := m.selectedMeta(); ok {
		// Status field
		sym := loader.StatusSymbol[mt.Status]
		symStyled := ui.StatusStyle(mt.Status).Render(sym + " " + string(mt.Status))
		sb.WriteString(ui.MetaDetailLabelStyle.Render("Status:") + " " + symStyled + "\n")

		if m.editingNote {
			sb.WriteString(ui.MetaDetailLabelStyle.Render("Note:") + " " + m.noteInput.View() + "\n")
		} else {
			noteText := mt.Note
			if noteText == "" {
				noteText = "— (press e to edit)"
			}
			wrapped := wrapText(noteText, inner)
			sb.WriteString(ui.MetaDetailLabelStyle.Render("Note:") + " " + ui.MetaNoteStyle.Render(wrapped) + "\n")
		}

		if m.editingTags {
			sb.WriteString(ui.MetaDetailLabelStyle.Render("Tags:") + " " + m.tagsInput.View() + "\n")
		} else {
			tagsText := strings.Join(mt.Tags, ", ")
			if tagsText == "" {
				tagsText = "— (press t to edit)"
			}
			wrapped := wrapText(tagsText, inner)
			sb.WriteString(ui.MetaDetailLabelStyle.Render("Tags:") + " " + ui.MetaTagStyle.Render(wrapped) + "\n")
		}
	}

	// Changelog block (divider only shown when there's content)
	var changelogContent string
	if m.changelogLoadingFor == t.Name {
		changelogContent = ui.DescStyle.Render("Loading changelog...") + "\n"
	} else if data, ok := m.changelogData[t.Name]; ok {
		changelogContent = m.renderChangelogBlock(data)
	} else if t.GitHub != "" {
		changelogContent = ui.DescStyle.Render("Loading changelog...") + "\n"
	}
	if changelogContent != "" {
		sb.WriteString(divider)
		sb.WriteString(changelogContent)
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
		sb.WriteString(ui.GithubStyle.Render(msg.htmlUrl) + "\n")
	}
	sb.WriteString("\n")
	body := wrapText(stripMarkdown(msg.body), max(m.briefW-2, 10))
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

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	toolsPanelEnd := m.toolsW + 1
	briefPanelEnd := toolsPanelEnd + m.briefW + 1

	// Detect which panel the click is in
	var cmd tea.Cmd
	if msg.X < toolsPanelEnd {
		// Left panel (Tools)
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			toolIdx := msg.Y - 3
			filtered := m.filteredMeta()
			if toolIdx >= 0 && toolIdx < len(filtered) {
				if m.metaSelected != toolIdx {
					m.metaSelected = toolIdx
					m.setToolsContent()
					m.briefViewport.Height = m.calcVpHeight()
					m.briefViewport.GotoTop()
					m.briefViewport.SetContent(m.renderCard())
				}
				m.focus = focusTools
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
			if msg.Action == tea.MouseActionPress && m.focus != focusBrief {
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
			if msg.Action == tea.MouseActionPress && m.focus != focusHelp {
				m.focus = focusHelp
				m.helpViewport.SetContent(m.renderHelpContent())
			}
		}
	}
	return m, cmd
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
		m.focus = focusBrief
		m.setToolsContent()
		m.briefViewport.GotoTop()
		m.briefViewport.SetContent(m.renderCard())

	case "left", "esc":
		m.focus = focusTools
		m.setToolsContent()
		m.briefViewport.SetContent(m.renderCard())

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

// autoFetchCmdsForSelected returns a batched Cmd that auto-fetches changelog
// and --help for the currently selected tool if not yet cached.
// Uses a pointer receiver so it can update loading state fields on m.
func (m *Model) autoFetchCmdsForSelected() tea.Cmd {
	var cmds []tea.Cmd
	if t, ok := m.selectedTool(); ok && t.GitHub != "" {
		if _, already := m.changelogData[t.Name]; !already && m.changelogLoadingFor != t.Name {
			m.changelogLoadingFor = t.Name
			m.briefViewport.SetContent(m.renderCard())
			cmds = append(cmds, fetchChangelogCmd(t.GitHub, t.Name))
		}
	}
	if mt, ok := m.selectedMeta(); ok {
		cached := m.helpCache[mt.Name]
		if cached[m.helpMode] == "" {
			m.helpLoadingFor = mt.Name
			m.helpViewport.SetContent(m.renderHelpContent())
			cmds = append(cmds, fetchHelpCmd(mt.Name, m.helpMode))
		} else {
			m.helpViewport.SetContent(m.renderHelpContent())
			m.helpViewport.GotoTop()
		}
	}
	return tea.Batch(cmds...)
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

var (
	helpFlagRe    = regexp.MustCompile(`(--?[a-zA-Z][a-zA-Z0-9\-_]*)`)
	helpMetaAngle = regexp.MustCompile(`<[^>]+>`)
	helpMetaBrack = regexp.MustCompile(`\[[^\]]+\]`)
)

func colorizeHelp(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		trimmed := strings.TrimRight(line, " ")
		if trimmed != "" && trimmed[0] != ' ' && trimmed[0] != '\t' && strings.HasSuffix(trimmed, ":") {
			lines[i] = ui.HelpSectionStyle.Render(line)
			continue
		}
		line = helpFlagRe.ReplaceAllStringFunc(line, func(m string) string {
			return ui.HelpFlagStyle.Render(m)
		})
		line = helpMetaAngle.ReplaceAllStringFunc(line, func(m string) string {
			return ui.HelpMetaStyle.Render(m)
		})
		line = helpMetaBrack.ReplaceAllStringFunc(line, func(m string) string {
			return ui.HelpMetaStyle.Render(m)
		})
		lines[i] = line
	}
	return strings.Join(lines, "\n")
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
	if innerW := max(m.helpW-2, 20); innerW > 0 {
		text = wrapText(text, innerW)
	}
	if !m.helpSearching || m.helpSearch.Value() == "" {
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

package model

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
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
	focusTools = 0
	focusBrief = 1
	focusHelp  = 2
)

type VersionInfo struct {
	Installed string
	Latest    string
}

// installedMsg carries the locally detected installed version for a tool.
// It is emitted by fetchInstalledCmd independently of any network activity so
// the installed version renders immediately, without waiting on GitHub.
type installedMsg struct {
	toolName  string
	installed string
}

// remoteMsg carries the result of a single network pass (release + repo info +
// languages) for a tool with a GitHub ref. It merges the latest tag, repo
// status and repo card in one message.
type remoteMsg struct {
	toolName   string
	latest     string
	repoStatus string
	card       version.RepoCard
	rate       version.RateLimit
	err        error
}

// rateMsg carries a rate-limit snapshot fetched from GET /rate_limit, which
// does not spend core quota. It seeds/refreshes m.rate independently of any
// per-tool remote fetch (e.g. on startup and on the API-status overlay refresh).
type rateMsg struct {
	rate version.RateLimit
	err  error
}

type changelogMsg struct {
	toolName    string
	tag         string
	body        string
	htmlUrl     string
	publishedAt string
	err         error
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

	tracking   bool
	trackInput textinput.Model

	confirmingUntrack bool
	untrackTarget     string

	renaming  bool
	nameInput textinput.Model

	meta         []loader.ToolMeta
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

	// rate is the latest GitHub rate-limit snapshot, seeded on startup by
	// fetchRateCmd and refreshed by remote fetches. A Known==false snapshot
	// never overwrites a previously-known value.
	rate version.RateLimit

	// showingAPIStatus toggles the L-triggered API-status overlay, which shows
	// the token source, current rate limits, and reset time.
	showingAPIStatus bool

	// enteringToken is the overlay's token-input sub-mode ([e]); tokenInput is
	// the masked field and tokenError holds the inline "token invalid" message.
	enteringToken bool
	tokenInput    textinput.Model
	tokenError    string
}

func New(meta []loader.ToolMeta) Model {
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

	tri := textinput.New()
	tri.Placeholder = "github url or tool name..."
	tri.CharLimit = 256

	nmi := textinput.New()
	nmi.Placeholder = "new name..."
	nmi.CharLimit = 256

	tki := textinput.New()
	tki.Placeholder = "ghp_..."
	tki.CharLimit = 256
	tki.EchoMode = textinput.EchoPassword
	tki.EchoCharacter = '•'

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
		trackInput:    tri,
		nameInput:     nmi,
		tokenInput:    tki,
		meta:          meta,
	}

	return m
}

func (m Model) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.tools)*2+1)
	// Seed the rate-limit signal up front; on warm-cache starts remote fetches
	// make no request, so this is the only observation that populates m.rate.
	cmds = append(cmds, fetchRateCmd())
	for _, t := range m.tools {
		cmds = append(cmds, fetchInstalledCmd(t))
		if t.GitHub != "" {
			cmds = append(cmds, fetchRemoteCmd(t))
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

	case installedMsg:
		info := m.versions[msg.toolName]
		info.Installed = msg.installed
		m.versions[msg.toolName] = info
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

	case remoteMsg:
		// Merge the rate snapshot without clobbering a known value with an
		// unknown one (cache-hit remote fetches make no request, so carry
		// Known==false).
		if msg.rate.Known {
			m.rate = msg.rate
		}
		// Data is displayable when the fetch succeeded, or when a rate-limit error
		// still carried usable cache values: a fresh tag from a partial fetch, or
		// the stale card kept on a total failure. In those cases render the data so
		// known tags/cards survive the outage. Only a rate-limit failure with
		// nothing to show falls back to the "rate limited — press [L]" hint. A
		// generic error carries no data and must not touch the caches.
		hasData := msg.latest != "" || msg.card.About != ""
		switch {
		case msg.err == nil, errors.Is(msg.err, version.ErrRateLimited) && hasData:
			info := m.versions[msg.toolName]
			info.Latest = msg.latest
			m.versions[msg.toolName] = info
			if msg.repoStatus != "" {
				m.repoStatus[msg.toolName] = msg.repoStatus
			}
			m.repoCards[msg.toolName] = msg.card
			m.toolsViewport.SetContent(m.renderLeftContent())
			m.briefViewport.SetContent(m.renderCard())
		case msg.repoStatus == "rate-limited":
			// Rate-limited with no card to show: mark the tool so the card can
			// render "rate limited — press [L]" instead of a bare failure.
			m.repoStatus[msg.toolName] = "rate-limited"
			m.briefViewport.SetContent(m.renderCard())
		}
		return m, nil

	case rateMsg:
		// Non-clobber merge: only a successful, Known snapshot updates m.rate.
		if msg.err == nil && msg.rate.Known {
			m.rate = msg.rate
		}
		return m, nil

	case tokenValidatedMsg:
		// Validation result for a candidate token entered in the overlay. On
		// failure nothing is written to disk; the inline message stays visible
		// and the input remains open for a retry.
		if msg.err != nil {
			m.tokenError = "token invalid"
			return m, nil
		}
		if err := version.SetToken(msg.token); err != nil {
			m.tokenError = "could not save token"
			return m, nil
		}
		m.enteringToken = false
		m.tokenInput.Blur()
		m.tokenInput.SetValue("")
		m.tokenError = ""
		if msg.rate.Known {
			m.rate = msg.rate
		}
		// Backfill cards now that the higher limit is available.
		return m, m.autoFetchCmdsForSelected()

	case openURLMsg:
		if msg.err != nil {
			m.statusMsg = msg.err.Error()
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
		leftVpH := vpH
		if !m.ready {
			// Viewports are 1 col narrower than their panel to leave a gutter
			// for the scrollbar rendered by withScrollbar.
			m.toolsViewport = viewport.New(m.toolsW-1, leftVpH)
			m.briefViewport = viewport.New(m.briefW-1, vpH)
			m.helpViewport = viewport.New(m.helpW-1, vpH)
			m.helpViewport.SetContent(m.renderHelpContent())
			m.ready = true
		} else {
			m.toolsViewport.Width = m.toolsW - 1
			m.toolsViewport.Height = leftVpH
			m.briefViewport.Width = m.briefW - 1
			m.briefViewport.Height = vpH
			m.helpViewport.Width = m.helpW - 1
			m.helpViewport.Height = vpH
		}
		m.toolsViewport.SetContent(m.renderLeftContent())
		m.syncToolsViewport()
		m.briefViewport.SetContent(m.renderCard())
		return m, nil

	case tea.KeyMsg:
		m.statusMsg = ""

		if m.editingNote {
			return m.updateNoteEdit(msg)
		}
		if m.editingTags {
			return m.updateTagsEdit(msg)
		}
		if m.tracking {
			return m.updateTrackInput(msg)
		}
		if m.confirmingUntrack {
			return m.updateUntrackConfirm(msg)
		}
		if m.renaming {
			return m.updateRenameInput(msg)
		}
		if m.showingAPIStatus {
			return m.updateAPIStatus(msg)
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
			} else {
				// Arrows scroll faster than line-by-line; j stays per-line.
				step := 1
				if msg.String() == "down" {
					step = 3
				}
				if m.focus == focusBrief {
					m.briefViewport.ScrollDown(step)
				} else if m.focus == focusHelp {
					m.helpViewport.ScrollDown(step)
				}
				return m, nil
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
			} else {
				step := 1
				if msg.String() == "up" {
					step = 3
				}
				if m.focus == focusBrief {
					m.briefViewport.ScrollUp(step)
				} else if m.focus == focusHelp {
					m.helpViewport.ScrollUp(step)
				}
				return m, nil
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
			} else if m.focus == focusTools {
				m.tracking = true
				m.trackInput.SetValue("")
				m.trackInput.Focus()
				return m, textinput.Blink
			}

		case "u":
			if m.focus == focusTools {
				if mt, ok := m.selectedMeta(); ok {
					m.confirmingUntrack = true
					m.untrackTarget = mt.Name
					return m, nil
				}
			}

		case "r":
			if m.focus == focusTools {
				if mt, ok := m.selectedMeta(); ok {
					m.renaming = true
					m.nameInput.SetValue(mt.Name)
					m.nameInput.Focus()
					return m, textinput.Blink
				}
			}

		case "o":
			if m.focus == focusBrief {
				if t, ok := m.selectedTool(); ok {
					if t.GitHub == "" {
						m.statusMsg = "no repo for " + t.Name
						return m, nil
					}
					return m, openURLCmd("https://" + t.GitHub)
				}
			}

		case "c":
			if m.focus == focusBrief {
				if t, ok := m.selectedTool(); ok {
					if t.GitHub == "" {
						m.statusMsg = "no repo for " + t.Name
						return m, nil
					}
					return m, openURLCmd("https://" + t.GitHub + "/releases")
				}
			}

		case "s":
			if m.focus == focusBrief {
				if mt, ok := m.selectedMeta(); ok {
					mt.Status = loader.NextStatus(mt.Status)
					m.meta = loader.UpsertMeta(m.meta, mt)
					loader.SaveMeta(m.meta) //nolint:errcheck
					m.briefViewport.SetContent(m.renderCard())
					return m, nil
				}
			}

		case "L":
			// Open the API-status overlay and refresh the rate numbers on demand
			// (GET /rate_limit does not spend quota). Reached only when no other
			// input/modal mode is active — those branches return earlier.
			m.showingAPIStatus = true
			m.enteringToken = false
			m.tokenError = ""
			return m, fetchRateCmd()
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

// trackTool adds (or updates) a tracked tool from a GitHub URL or plain name.
// It returns the updated meta slice and a status message ("" on a fresh add,
// "already tracked" when the name was already present). Empty input is a no-op.
func trackTool(meta []loader.ToolMeta, input string) ([]loader.ToolMeta, string) {
	input = strings.TrimSpace(input)
	if input == "" {
		return meta, ""
	}
	name, github, _ := loader.ParseToolRef(input)
	status := ""
	if loader.FindMeta(meta, name) != nil {
		status = "already tracked"
	}
	entry := loader.ToolMeta{
		Name:   name,
		GitHub: github,
		Status: loader.StatusTrying,
		Added:  loader.TodayDate(),
	}
	return loader.UpsertMeta(meta, entry), status
}

func (m Model) updateTrackInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.tracking = false
		m.trackInput.Blur()
		input := strings.TrimSpace(m.trackInput.Value())
		if input == "" {
			return m, nil
		}
		name, _, _ := loader.ParseToolRef(input)
		var status string
		m.meta, status = trackTool(m.meta, input)
		loader.SaveMeta(m.meta) //nolint:errcheck
		m.tools = loader.ToolsFromMeta(m.meta)
		for i, mt := range m.meta {
			if mt.Name == name {
				m.metaSelected = i
				break
			}
		}
		m.setToolsContent()
		m.briefViewport.GotoTop()
		m.briefViewport.SetContent(m.renderCard())
		m.statusMsg = status
		return m, m.autoFetchCmdsForSelected()
	case "esc":
		m.tracking = false
		m.trackInput.Blur()
		return m, nil
	default:
		var cmd tea.Cmd
		m.trackInput, cmd = m.trackInput.Update(msg)
		return m, cmd
	}
}

func (m Model) updateUntrackConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.confirmingUntrack = false
		m.meta = loader.RemoveMeta(m.meta, m.untrackTarget)
		loader.SaveMeta(m.meta) //nolint:errcheck
		m.tools = loader.ToolsFromMeta(m.meta)
		m.untrackTarget = ""
		// Keep metaSelected at the same index so selection lands on the next
		// item; clamp to the new last index (or 0 when the list is empty).
		if m.metaSelected > len(m.meta)-1 {
			m.metaSelected = max(len(m.meta)-1, 0)
		}
		m.setToolsContent()
		m.briefViewport.GotoTop()
		m.briefViewport.SetContent(m.renderCard())
		return m, m.autoFetchCmdsForSelected()
	default:
		// esc or any other key cancels.
		m.confirmingUntrack = false
		m.untrackTarget = ""
		return m, nil
	}
}

// renameTool changes a tracked tool's Name from old to newName, preserving its
// GitHub/Status/Tags/Note/Added fields. An empty newName (after trimming) or a
// newName equal to old is a no-op. A collision with another tracked tool's name
// is rejected with an error and leaves meta unchanged.
func renameTool(meta []loader.ToolMeta, old, newName string) ([]loader.ToolMeta, error) {
	newName = strings.TrimSpace(newName)
	if newName == "" || newName == old {
		return meta, nil
	}
	if loader.FindMeta(meta, newName) != nil {
		return meta, fmt.Errorf("name already exists")
	}
	for i := range meta {
		if meta[i].Name == old {
			meta[i].Name = newName
			return meta, nil
		}
	}
	return meta, nil
}

func (m Model) updateRenameInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		mt, ok := m.selectedMeta()
		if !ok {
			m.renaming = false
			m.nameInput.Blur()
			return m, nil
		}
		old := mt.Name
		newName := strings.TrimSpace(m.nameInput.Value())
		updated, err := renameTool(m.meta, old, newName)
		if err != nil {
			m.renaming = false
			m.nameInput.Blur()
			m.statusMsg = err.Error()
			return m, nil
		}
		m.renaming = false
		m.nameInput.Blur()
		if newName == "" || newName == old {
			return m, nil
		}
		m.meta = updated
		loader.SaveMeta(m.meta) //nolint:errcheck
		m.tools = loader.ToolsFromMeta(m.meta)
		delete(m.helpCache, old)
		delete(m.repoCards, old)
		delete(m.versions, old)
		delete(m.repoStatus, old)
		delete(m.changelogData, old)
		for i, e := range m.meta {
			if e.Name == newName {
				m.metaSelected = i
				break
			}
		}
		m.setToolsContent()
		m.briefViewport.GotoTop()
		m.briefViewport.SetContent(m.renderCard())
		return m, m.autoFetchCmdsForSelected()
	case "esc":
		m.renaming = false
		m.nameInput.Blur()
		return m, nil
	default:
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
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
	if m.showingAPIStatus {
		layout = ui.PlaceOverlay(layout, m.renderAPIStatus())
	}
	// Vertical margin only; no horizontal margin so panels/status bar reach the
	// terminal edges.
	return lipgloss.NewStyle().Margin(1, 0).Render(layout)
}

func (m Model) renderStatusBar() string {
	style := ui.HelpStyle.Width(m.width - 2)
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
	if m.tracking {
		return style.Render(fmt.Sprintf(
			"%s %s  %s cancel",
			ui.SearchPromptStyle.Render("track (github url or tool name):"),
			m.trackInput.View(),
			keyHint("esc"),
		))
	}
	if m.confirmingUntrack {
		return style.Render(fmt.Sprintf(
			"%s  %s yes  %s no",
			ui.SearchPromptStyle.Render("Untrack "+m.untrackTarget+"?"),
			keyHint("enter"),
			keyHint("esc"),
		))
	}
	if m.renaming {
		return style.Render(fmt.Sprintf(
			"%s %s  %s cancel",
			ui.SearchPromptStyle.Render("rename to:"),
			m.nameInput.View(),
			keyHint("esc"),
		))
	}
	if m.showingAPIStatus && m.enteringToken {
		return style.Render(keyHint("enter") + " validate & save  " + keyHint("esc") + " cancel")
	}
	if m.showingAPIStatus {
		return style.Render(keyHint("r") + " refresh  " + keyHint("esc") + " close")
	}
	if m.statusMsg != "" {
		return style.Render(ui.SearchPromptStyle.Render(m.statusMsg))
	}
	if m.focus == focusBrief {
		hints := keyHint("o") + " open repo  " + keyHint("c") + " changelog  " + keyHint("s") + " status  " + keyHint("e") + " note  " + keyHint("t") + " tags  " + keyHint("q") + " quit"
		return style.Render(m.withRateSignal(hints))
	}
	if m.focus == focusHelp {
		hints := keyHint("↑↓") + " scroll  " + keyHint("h") + " --help  " + keyHint("m") + " man  " + keyHint("/") + " search  " + keyHint("←") + " back  " + keyHint("q") + " quit"
		return style.Render(m.withRateSignal(hints))
	}
	return style.Render(m.withRateSignal(
		keyHint("/") + " search  " +
			keyHint("t") + " track  " +
			keyHint("u") + " untrack  " +
			keyHint("r") + " rename  " +
			keyHint("q") + " quit",
	))
}

// withRateSignal appends the rate-limit signal to a hint bar when there is one.
func (m Model) withRateSignal(hints string) string {
	if sig := m.rateSignal(); sig != "" {
		return hints + "   " + sig
	}
	return hints
}

func keyHint(k string) string {
	return ui.SearchPromptStyle.Render("[" + k + "]")
}

// rateLowThreshold is the number of remaining GitHub API requests at or below
// which the status bar (and API-status overlay) flags rate-limit pressure.
const rateLowThreshold = 10

// rateSignal returns the status-bar rate-limit indicator for the current
// snapshot, or "" when nothing should be shown (no observation yet). It is the
// single source of truth for the status-bar signal; the overlay reuses the
// same thresholds via rateIcon.
func (m Model) rateSignal() string {
	if !m.rate.Known {
		return ""
	}
	switch {
	case m.rate.Remaining == 0:
		return ui.DangerStyle.Render("✕ GH limit exhausted") + " · " + keyHint("L")
	case m.rate.Remaining <= rateLowThreshold:
		return ui.WarnStyle.Render(fmt.Sprintf("⚠ GH %d/%d", m.rate.Remaining, m.rate.Limit)) + " · " + keyHint("L") + " details"
	default:
		return ui.GithubStyle.Render(fmt.Sprintf("GH %d/%d", m.rate.Remaining, m.rate.Limit))
	}
}

// rateIcon returns the styled indicator (none / ⚠ / ✕) for a rate snapshot,
// sharing rateLowThreshold with the status-bar signal so the overlay and the
// bar never disagree. Returns "" when nothing should flag.
func rateIcon(rate version.RateLimit) string {
	if !rate.Known {
		return ""
	}
	switch {
	case rate.Remaining == 0:
		return ui.DangerStyle.Render("✕")
	case rate.Remaining <= rateLowThreshold:
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

// updateAPIStatus handles keys while the API-status overlay is open: [e] opens
// the masked token-input sub-mode, [d] removes a config-sourced token, [r]
// refreshes the numbers, [esc] closes. While entering a token, submit validates
// via version.FetchRateWithToken before anything is persisted.
func (m Model) updateAPIStatus(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.enteringToken {
		switch msg.String() {
		case "enter":
			candidate := strings.TrimSpace(m.tokenInput.Value())
			if candidate == "" {
				m.enteringToken = false
				m.tokenInput.Blur()
				m.tokenError = ""
				return m, nil
			}
			// Validate the candidate against /rate_limit; SetToken runs only
			// after a 200 in the tokenValidatedMsg handler.
			return m, validateTokenCmd(candidate)
		case "esc":
			m.enteringToken = false
			m.tokenInput.Blur()
			m.tokenInput.SetValue("")
			m.tokenError = ""
			return m, nil
		default:
			var cmd tea.Cmd
			m.tokenInput, cmd = m.tokenInput.Update(msg)
			return m, cmd
		}
	}
	switch msg.String() {
	case "esc", "q":
		m.showingAPIStatus = false
		m.tokenError = ""
		return m, nil
	case "r":
		return m, fetchRateCmd()
	case "e":
		m.enteringToken = true
		m.tokenError = ""
		m.tokenInput.SetValue("")
		m.tokenInput.Focus()
		return m, textinput.Blink
	case "d":
		if version.TokenSource() == "config" {
			version.ClearToken() //nolint:errcheck
			return m, fetchRateCmd()
		}
	}
	return m, nil
}

// tokenValidatedMsg carries the result of validating a candidate token against
// GET /rate_limit. token is the candidate to persist on success.
type tokenValidatedMsg struct {
	token string
	rate  version.RateLimit
	err   error
}

// validateTokenCmd checks a candidate token against /rate_limit without touching
// package token state; the handler persists it only on success.
func validateTokenCmd(token string) tea.Cmd {
	return func() tea.Msg {
		rate, err := version.FetchRateWithToken(token)
		return tokenValidatedMsg{token: token, rate: rate, err: err}
	}
}

// renderAPIStatus builds the API-status overlay body: token source (masked),
// current limits with the shared icon, and the reset time.
func (m Model) renderAPIStatus() string {
	var b strings.Builder
	b.WriteString(ui.SectionLabelStyle.Render("GitHub API status") + "\n\n")

	source := version.TokenSource()
	tokenLine := "Token: " + source
	if tok := version.Token(); tok != "" {
		tokenLine += " (" + maskToken(tok) + ")"
	}
	b.WriteString(ui.InfoStyle.Render(tokenLine) + "\n")

	if m.rate.Known {
		icon := rateIcon(m.rate)
		limitLine := fmt.Sprintf("Limit: %d / %d", m.rate.Remaining, m.rate.Limit)
		if icon != "" {
			limitLine = icon + " " + limitLine
		}
		b.WriteString(ui.InfoStyle.Render(limitLine) + "\n")
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

	if m.enteringToken {
		b.WriteString("\n" + ui.SearchPromptStyle.Render("token: ") + m.tokenInput.View() + "\n")
	}
	if m.tokenError != "" {
		b.WriteString(ui.DangerStyle.Render(m.tokenError) + "\n")
	}

	b.WriteString("\n")
	var hints string
	if m.enteringToken {
		hints = keyHint("enter") + " validate & save  " + keyHint("esc") + " cancel"
	} else {
		hints = keyHint("e") + " set token  "
		if source == "config" {
			hints += keyHint("d") + " remove token  "
		}
		hints += keyHint("r") + " refresh  " + keyHint("esc") + " close"
	}
	b.WriteString(ui.MetaNoteStyle.Render(hints))

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
	toolsW = max((available * 20) / 100, 15)
	briefW = max((available * 40) / 100, 30)
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
				briefW = max((available - toolsW - 5) / 2, 1)
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
	if hasCard && card.About != "" {
		aboutWidth := max(inner-utf8.RuneCountInString(name)-3, 20)
		aboutWrapped := wrapText(card.About, aboutWidth)
		sb.WriteString(nameRendered + " — " + ui.MetaNoteStyle.Render(aboutWrapped) + "\n")
	} else {
		sb.WriteString(nameRendered + "\n")
	}

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

		if m.editingNote {
			sb.WriteString(ui.MetaDetailLabelStyle.Render("note:") + " " + m.noteInput.View() + "\n")
		} else {
			noteText := mt.Note
			if noteText == "" {
				noteText = "— (press e to edit)"
			}
			wrapped := wrapText(noteText, inner)
			sb.WriteString(ui.MetaDetailLabelStyle.Render("note:") + " " + ui.MetaNoteStyle.Render(wrapped) + "\n")
		}

		if m.editingTags {
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
	// Panels sit flush (each is panelW+2 wide incl. borders) with no outer
	// horizontal margin, so screen X maps directly to panel spans.
	toolsPanelEnd := m.toolsW + 2
	briefPanelEnd := toolsPanelEnd + m.briefW + 2

	// Detect which panel the click is in
	var cmd tea.Cmd
	if msg.X < toolsPanelEnd {
		// Left panel (Tools)
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			// Row 0 = top margin, row 1 = panel border, row 2 = first list row.
			toolIdx := msg.Y - 2 + m.toolsViewport.YOffset
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

// fetchInstalledCmd returns a Cmd that detects the installed version of t
// locally (subprocess) and emits an installedMsg. It never touches the network,
// so the installed version can render before any GitHub fetch completes.
func fetchInstalledCmd(t loader.Tool) tea.Cmd {
	return func() tea.Msg {
		installed := version.InstalledVersion(t)
		return installedMsg{
			toolName:  t.Name,
			installed: installed,
		}
	}
}

// fetchRemoteCmd returns a Cmd that makes a single network pass over t's
// repository via version.GetRepoData (release + repo info + languages) and emits
// a remoteMsg carrying the latest tag, repo status and repo card together.
func fetchRemoteCmd(t loader.Tool) tea.Cmd {
	return func() tea.Msg {
		d := version.GetRepoData(t.GitHub)
		repoStatus := d.RepoStatus
		// Rate-limited and no data came back: signal the card to render a
		// "rate limited" hint. This gives ErrRateLimited a real runtime consumer.
		if errors.Is(d.Err, version.ErrRateLimited) && d.Latest == "" && d.About == "" {
			repoStatus = "rate-limited"
		}
		return remoteMsg{
			toolName:   t.Name,
			latest:     d.Latest,
			repoStatus: repoStatus,
			card: version.RepoCard{
				About:       d.About,
				Stars:       d.Stars,
				Languages:   d.Languages,
				Latest:      d.Latest,
				PublishedAt: d.PublishedAt,
				HtmlUrl:     d.HtmlUrl,
				Body:        d.Body,
				RepoStatus:  d.RepoStatus,
			},
			rate: version.Rate(),
			err:  d.Err,
		}
	}
}

// fetchRateCmd queries GET /rate_limit, which reports the current quota without
// spending it, and emits a rateMsg. Fired on startup to seed the status-bar
// signal even on warm-cache starts (where remote fetches make no request) and
// on demand from the API-status overlay.
func fetchRateCmd() tea.Cmd {
	return func() tea.Msg {
		rate, err := version.FetchRate()
		return rateMsg{rate: rate, err: err}
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

// needsInstalled reports whether the installed version for t is not yet known.
// Detected locally, so it fires regardless of GitHub, matching Init(). Guards
// against re-running the subprocess on every cursor movement.
func (m *Model) needsInstalled(t loader.Tool) bool {
	info, ok := m.versions[t.Name]
	return !ok || info.Installed == ""
}

// needsRemote reports whether the network pass for t must still run.
// Requires a GitHub ref (matching the Init() guard) and either an unknown
// latest tag or a missing repo card.
func (m *Model) needsRemote(t loader.Tool) bool {
	if t.GitHub == "" {
		return false
	}
	if _, ok := m.repoCards[t.Name]; !ok {
		return true
	}
	info := m.versions[t.Name]
	return info.Latest == ""
}

// autoFetchCmdsForSelected returns a batched Cmd that auto-fetches changelog
// and --help for the currently selected tool if not yet cached.
// Uses a pointer receiver so it can update loading state fields on m.
func (m *Model) autoFetchCmdsForSelected() tea.Cmd {
	var cmds []tea.Cmd
	if t, ok := m.selectedTool(); ok {
		if t.GitHub != "" {
			if _, already := m.changelogData[t.Name]; !already && m.changelogLoadingFor != t.Name {
				m.changelogLoadingFor = t.Name
				m.briefViewport.SetContent(m.renderCard())
				cmds = append(cmds, fetchChangelogCmd(t.GitHub, t.Name))
			}
		}
		if m.needsInstalled(t) {
			cmds = append(cmds, fetchInstalledCmd(t))
		}
		if m.needsRemote(t) {
			cmds = append(cmds, fetchRemoteCmd(t))
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

func fetchHelpCmd(name string, mode int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var output []byte
		var err error

		// In man mode, try the man page first.
		if mode == helpModeMan {
			cmd := exec.CommandContext(ctx, "man", name)
			cmd.Env = append(os.Environ(), "MANPAGER=cat", "MANWIDTH=80", "TERM=dumb")
			output, err = cmd.Output()
		}

		// Fall back through the tool's own help flags. This is the only source
		// for --help mode, and the fallback when `man` has no page.
		if len(output) == 0 {
			for _, args := range [][]string{{"--help"}, {"-h"}, {"help"}} {
				if ctx.Err() != nil {
					break
				}
				out, e := exec.CommandContext(ctx, name, args...).CombinedOutput()
				err = e
				if len(out) > 0 {
					output = out
					break
				}
			}
		}

		if len(output) == 0 {
			return helpOutputMsg{toolName: name, mode: mode, err: err}
		}
		return helpOutputMsg{toolName: name, mode: mode, output: cleanTerminalOutput(string(output))}
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

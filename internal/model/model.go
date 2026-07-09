package model

import (
	"errors"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
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
	// searchPrevName is the commit/rollback anchor for modeSearch: captured
	// (from the current selection) when "/" opens the search and cleared on any
	// exit. enter commits to the highlighted match; esc rolls the cursor back
	// to this tool in the full list.
	searchPrevName string
	statusMsg      string
	width          int
	height         int
	ready          bool

	// mode is the single input/modal state (see inputMode in mode.go). The
	// zero value modeNormal is the base state; per-mode key handlers own the
	// input while any other mode is active.
	mode inputMode

	noteInput textinput.Model
	tagsInput textinput.Model

	trackInput textinput.Model

	untrackTarget string

	nameInput textinput.Model

	// spinner animates while a force refresh ([r]) is in flight; refreshingFor
	// holds the name of the tool being refreshed (empty = idle). refreshingFor
	// doubles as the double-press guard and as the tick-loop / render gate.
	spinner       spinner.Model
	refreshingFor string

	meta         []loader.ToolMeta
	metaSelected int

	helpMode       int
	helpLoadingFor string
	helpCache      map[string][2]string
	helpSearch     textinput.Model
	helpMatches    []int
	helpMatchIdx   int

	toolsW int
	briefW int
	helpW  int

	// rate is the latest GitHub rate-limit snapshot, seeded on startup by
	// fetchRateCmd and refreshed by remote fetches. A Known==false snapshot
	// never overwrites a previously-known value.
	rate version.RateLimit

	// tokenInput is the overlay's masked token field (modeTokenInput) and
	// tokenError holds the inline "token invalid" message.
	tokenInput textinput.Model
	tokenError string
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

	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = lipgloss.NewStyle().Foreground(ui.ColorPrimary)

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
		spinner:       sp,
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
	if m.mode == modeSearch {
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
		// A refresh's repo pass has landed (success or error): clear the flag so
		// the card title drops the "refreshing … data" status back to name+about.
		// This also halts the tick loop.
		if msg.toolName == m.refreshingFor {
			m.refreshingFor = ""
			m.briefViewport.SetContent(m.renderCard())
		}
		return m, nil

	case rateMsg:
		// Non-clobber merge: only a successful, Known snapshot updates m.rate.
		if msg.err == nil && msg.rate.Known {
			m.rate = msg.rate
		}
		return m, nil

	case spinner.TickMsg:
		// Animate only while a refresh is in flight; once refreshingFor is
		// cleared (by the remoteMsg handler) the loop stops rescheduling itself.
		if m.refreshingFor == "" {
			return m, nil
		}
		m.spinner, cmd = m.spinner.Update(msg)
		m.briefViewport.SetContent(m.renderCard())
		return m, cmd

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
		// The result may land after the user already left the token input via
		// esc; only a still-active modeTokenInput falls back to the overlay.
		if m.mode == modeTokenInput {
			m.mode = modeAPIStatus
		}
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
		// Only the named tool's result retires the loading marker: a stale
		// fetch for a previously highlighted tool must not clear the flag
		// while the currently selected tool's fetch is still in flight.
		if m.helpLoadingFor == msg.toolName {
			m.helpLoadingFor = ""
		}
		cached := m.helpCache[msg.toolName]
		if msg.err == nil && msg.output != "" {
			cached[msg.mode] = msg.output
		} else if msg.mode == helpModeHelp {
			cached[msg.mode] = "No --help output for " + msg.toolName + ".\nPress [m] for the man page."
		} else {
			cached[msg.mode] = "No man page for " + msg.toolName + ".\nPress [h] for --help."
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

		switch m.mode {
		case modeEditNote:
			return m.updateNoteEdit(msg)
		case modeEditTags:
			return m.updateTagsEdit(msg)
		case modeTrack:
			return m.updateTrackInput(msg)
		case modeConfirmUntrack:
			return m.updateUntrackConfirm(msg)
		case modeRename:
			return m.updateRenameInput(msg)
		case modeAPIStatus, modeTokenInput:
			return m.updateAPIStatus(msg)
		}

		if m.mode == modeHelpSearch {
			switch msg.String() {
			case "esc":
				m.mode = modeNormal
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

		if m.mode == modeSearch {
			switch msg.String() {
			case "esc":
				// Rollback: restore the cursor to the tool selected before the
				// search started (fallback 0 when it was untracked mid-search).
				// selectMeta refreshes the help viewport too — arrow moves may
				// have loaded another tool's help mid-search.
				m.mode = modeNormal
				m.search.SetValue("")
				m.search.Blur()
				prev := m.searchPrevName
				m.searchPrevName = ""
				return m, m.selectMeta(m.indexOfMeta(prev))
			case "enter":
				// Commit: accept the highlighted match. With no matches the key
				// is a no-op and search stays open.
				mt, ok := m.selectedMeta()
				if !ok {
					return m, nil
				}
				m.mode = modeNormal
				m.search.SetValue("")
				m.search.Blur()
				m.searchPrevName = ""
				m.focus = focusBrief
				// The filter is gone once the mode is normal, so remap the
				// cursor onto the full list by name.
				return m, m.selectMeta(m.indexOfMeta(mt.Name))
			case "up", "down":
				// Move the highlight through the filtered list (wrap-around,
				// j/k parity). Never forwarded to the textinput, so the query
				// text is untouched; with zero matches the key is consumed.
				if n := len(m.filteredMeta()); n > 0 {
					if msg.String() == "down" {
						return m, m.selectMeta((m.metaSelected + 1) % n)
					}
					return m, m.selectMeta((m.metaSelected - 1 + n) % n)
				}
				return m, nil
			default:
				prevQuery := m.search.Value()
				m.search, cmd = m.search.Update(msg)
				if m.search.Value() != prevQuery {
					// The filter changed: reset the highlight to the first
					// match (a stale index could fall out of a narrower
					// filter's range). Pure cursor movement (left/right/
					// home/end) keeps a user-moved highlight.
					m.metaSelected = 0
					m.setToolsContent()
					m.briefViewport.GotoTop()
					m.briefViewport.SetContent(m.renderCard())
					// Re-sync the help panel like selectMeta does: a prior
					// arrow move may have left it on "Loading..." for a tool
					// this reset just unselected, and the stale fetch landing
					// later won't repaint an unselected tool's panel.
					m.helpViewport.SetContent(m.renderHelpContent())
					m.helpViewport.GotoTop()
				}
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
				// Wrap around to the top when moving past the last tool.
				if n := len(m.filteredMeta()); n > 0 {
					return m, m.selectMeta((m.metaSelected + 1) % n)
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
				// Wrap around to the bottom when moving above the first tool.
				if n := len(m.filteredMeta()); n > 0 {
					return m, m.selectMeta((m.metaSelected - 1 + n) % n)
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
				return m, m.selectMeta(max(m.metaSelected-step, 0))
			}

		case "pgdown", "ctrl+f":
			if m.focus == focusTools {
				step := max(m.toolsViewport.Height, 1)
				return m, m.selectMeta(min(m.metaSelected+step, max(len(m.filteredMeta())-1, 0)))
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
				m.mode = modeHelpSearch
				m.helpSearch.Focus()
				return m, textinput.Blink
			}
			// Remember the current selection so esc can roll back to it.
			m.searchPrevName = ""
			if mt, ok := m.selectedMeta(); ok {
				m.searchPrevName = mt.Name
			}
			m.mode = modeSearch
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
					m.mode = modeEditNote
					m.noteInput.SetValue(mt.Note)
					m.noteInput.Focus()
					m.briefViewport.SetContent(m.renderCard())
					return m, textinput.Blink
				}
			}

		case "t":
			if m.focus == focusBrief {
				if mt, ok := m.selectedMeta(); ok {
					m.mode = modeEditTags
					m.tagsInput.SetValue(strings.Join(mt.Tags, ", "))
					m.tagsInput.Focus()
					m.briefViewport.SetContent(m.renderCard())
					return m, textinput.Blink
				}
			} else if m.focus == focusTools {
				m.mode = modeTrack
				m.trackInput.SetValue("")
				m.trackInput.Focus()
				return m, textinput.Blink
			}

		case "u":
			if m.focus == focusTools {
				if mt, ok := m.selectedMeta(); ok {
					m.mode = modeConfirmUntrack
					m.untrackTarget = mt.Name
					return m, nil
				}
			}

		case "r":
			if m.focus == focusTools {
				if mt, ok := m.selectedMeta(); ok {
					m.mode = modeRename
					m.nameInput.SetValue(mt.Name)
					m.nameInput.Focus()
					return m, textinput.Blink
				}
			} else if m.focus == focusBrief {
				if t, ok := m.selectedTool(); ok {
					return m, m.refreshSelectedCmd(t)
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
			// (GET /rate_limit does not spend quota). Reached only in modeNormal —
			// every other mode's handler returns earlier.
			m.mode = modeAPIStatus
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

// searchQuery returns the normalized (lowercase, trimmed) live query, or ""
// when the tool-list search is not active — the empty string doubles as the
// "no filter" signal for searchMatches and the list renderer.
func (m Model) searchQuery() string {
	if m.mode != modeSearch {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(m.search.Value()))
}

// searchMatch pairs a tool that passed the search filter with how the query
// matched it: byTagOnly flags rows whose name did not match (a tag did), and
// tag carries the first matching tag so the renderer can show what earned the
// row its place in the list.
type searchMatch struct {
	meta      loader.ToolMeta
	byTagOnly bool
	tag       string
}

// searchMatches is the search predicate: a tool matches when its name OR any
// of its tags contains the query (case-insensitive substring). With no active
// query every tool passes unmarked. filteredMeta projects this to the plain
// meta slice for callers that only need the filtered list.
func (m Model) searchMatches() []searchMatch {
	query := m.searchQuery()
	out := make([]searchMatch, 0, len(m.meta))
	for _, mt := range m.meta {
		if query == "" {
			out = append(out, searchMatch{meta: mt})
			continue
		}
		nameHit := strings.Contains(strings.ToLower(mt.Name), query)
		tag := matchingTag(mt.Tags, query)
		if !nameHit && tag == "" {
			continue
		}
		out = append(out, searchMatch{meta: mt, byTagOnly: !nameHit, tag: tag})
	}
	return out
}

// matchingTag returns the first tag containing the query, or "".
func matchingTag(tags []string, query string) string {
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return tag
		}
	}
	return ""
}

func (m Model) filteredMeta() []loader.ToolMeta {
	matches := m.searchMatches()
	out := make([]loader.ToolMeta, len(matches))
	for i, sm := range matches {
		out[i] = sm.meta
	}
	return out
}

// selectMeta moves the cursor to idx in the current (possibly filtered) list
// and refreshes every panel that tracks the selection: the tools list, the
// brief card (scrolled to top) and the help viewport via the auto-fetch path.
// Shared by keyboard navigation (j/k, arrows, pgup/pgdown), the search
// commit/rollback exits and mouse clicks so the post-move refresh ritual
// cannot drift between call sites.
func (m *Model) selectMeta(idx int) tea.Cmd {
	m.metaSelected = idx
	m.setToolsContent()
	m.briefViewport.GotoTop()
	m.briefViewport.SetContent(m.renderCard())
	return m.autoFetchCmdsForSelected()
}

// indexOfMeta returns the index of the named tool in the full m.meta list,
// falling back to 0 when the name is empty or the tool is absent (e.g.
// untracked mid-search).
func (m Model) indexOfMeta(name string) int {
	for i, mt := range m.meta {
		if mt.Name == name {
			return i
		}
	}
	return 0
}

const (
	rateUnknown rateLevel = iota
	rateOK
	rateWarn
	rateExhausted
)

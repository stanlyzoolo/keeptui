package model

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lepeshko/keys/internal/loader"
	"github.com/lepeshko/keys/internal/proc"
	"github.com/lepeshko/keys/internal/version"
)

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
func fetchRemoteCmd(t loader.Tool) tea.Cmd { return remoteCmd(t, false) }

// refreshRemoteCmd is the force variant of fetchRemoteCmd: it bypasses the cache
// TTL via version.RefreshRepoData. Emits the same remoteMsg.
func refreshRemoteCmd(t loader.Tool) tea.Cmd { return remoteCmd(t, true) }

func remoteCmd(t loader.Tool, force bool) tea.Cmd {
	return func() tea.Msg {
		var d version.RepoData
		if force {
			d = version.RefreshRepoData(t.GitHub)
		} else {
			d = version.GetRepoData(t.GitHub)
		}
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
	return changelogCmd(githubField, toolName, false)
}

// refreshChangelogCmd is the force variant of fetchChangelogCmd: it bypasses the
// cache TTL via version.RefreshChangelog. Emits the same changelogMsg.
func refreshChangelogCmd(githubField, toolName string) tea.Cmd {
	return changelogCmd(githubField, toolName, true)
}

func changelogCmd(githubField, toolName string, force bool) tea.Cmd {
	return func() tea.Msg {
		var (
			info version.ReleaseInfo
			err  error
		)
		if force {
			info, err = version.RefreshChangelog(githubField)
		} else {
			info, err = version.GetChangelog(githubField)
		}
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

// refreshSelectedCmd force-refreshes t's data, bypassing the cache TTL: the repo
// pass (release + repo info + languages) and the changelog are re-fetched and the
// installed version is re-detected locally. While the repo pass is in flight
// refreshingFor drives the card spinner; the remoteMsg handler clears it on
// completion, which halts the tick loop. A second press while the same tool is
// already refreshing is ignored. A tool with no GitHub ref only re-detects the
// installed version (no spinner, nothing to clear).
func (m *Model) refreshSelectedCmd(t loader.Tool) tea.Cmd {
	if m.refreshingFor == t.Name {
		return nil
	}
	if t.GitHub == "" {
		m.statusMsg = "no repo to refresh"
		return fetchInstalledCmd(t)
	}
	m.refreshingFor = t.Name
	m.briefViewport.SetContent(m.renderCard())
	return tea.Batch(
		m.spinner.Tick,
		fetchInstalledCmd(t),
		refreshRemoteCmd(t),
		refreshChangelogCmd(t.GitHub, t.Name),
	)
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

func fetchHelpCmd(name string, mode int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var output []byte
		var err error

		// Each mode has exactly one source — no silent cross-fallback — so [m]
		// and [h] are distinct and a missing page/flag surfaces its own message
		// instead of masquerading as the other.
		// Every probe runs detached from the controlling terminal: a tool
		// that answers --help/-h/help by booting its own TUI would otherwise
		// grab /dev/tty and shred keys' screen (see internal/proc).
		if mode == helpModeMan {
			cmd := exec.CommandContext(ctx, "man", name)
			cmd.Env = append(os.Environ(), "MANPAGER=cat", "MANWIDTH=80", "TERM=dumb")
			proc.DetachTTY(cmd)
			output, err = cmd.Output()
		} else {
			for _, args := range [][]string{{"--help"}, {"-h"}, {"help"}} {
				if ctx.Err() != nil {
					break
				}
				cmd := exec.CommandContext(ctx, name, args...)
				proc.DetachTTY(cmd)
				out, e := cmd.CombinedOutput()
				err = e
				// A tool that answered the flag by booting its own TUI leaves
				// the alt-screen signature in the capture (plus a crash trace,
				// since DetachTTY cut it off from /dev/tty). That is not help
				// text — fall through to the "No --help output" message.
				if isTUITakeover(out) {
					continue
				}
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

package model

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/stanlyzoolo/keeptui/internal/loader"
	"github.com/stanlyzoolo/keeptui/internal/version"
)

// inputMode is the single input/modal state of the TUI. Exactly one mode is
// active at a time; modeNormal is the base state where the per-focus key map
// applies. modeTokenInput is a sub-state of the API-status overlay: it is
// entered from modeAPIStatus via [e] and exits back to modeAPIStatus, and the
// overlay stays visible in both (apiOverlayVisible).
type inputMode int

const (
	modeNormal inputMode = iota
	modeSearch         // "/" in focusTools: filter the tool list
	modeHelpSearch     // "/" in focusBrief/focusHelp: search the help viewport
	modeEditNote       // "e" in focusBrief
	modeEditTags       // "t" in focusBrief
	modeTrack          // "t" in focusTools
	modeConfirmUntrack // "u" in focusTools
	modeRename         // "r" in focusTools
	modeConfirmUpdate  // "u" in focusBrief: confirm the detected update command
	modeAPIStatus      // "L": rate-limit / token overlay
	modeTokenInput     // "e" inside the overlay: masked token entry
	modeHotkeys        // "?": static hotkeys-help overlay
)

// apiOverlayVisible reports whether the API-status overlay is on screen —
// true both while browsing it and while entering a token.
func (m Model) apiOverlayVisible() bool {
	return m.mode == modeAPIStatus || m.mode == modeTokenInput
}

// overlayVisible reports whether any modal overlay is composited over the
// layout — the [L] API-status overlay (incl. token entry) or the [?] hotkeys
// overlay. It is the single "modal on screen" predicate for View() and the
// mouse gate, so a new overlay only has to extend this one helper.
func (m Model) overlayVisible() bool {
	return m.apiOverlayVisible() || m.mode == modeHotkeys
}

// updateHotkeys handles keys while the [?] hotkeys overlay is open: esc, q, or
// a second ? closes it back to modeNormal; every other key is a no-op. The
// overlay is static (no scrolling), mirroring the updateAPIStatus close
// pattern. ctrl+c still quits — this handler runs before the global quit case,
// so it must honor the overlay's own "ctrl+c anywhere" hint explicitly.
func (m Model) updateHotkeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q", "?":
		m.mode = modeNormal
		return m, nil
	}
	return m, nil
}

func (m Model) updateNoteEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.mode = modeNormal
		m.noteInput.Blur()
		if mt, ok := m.selectedMeta(); ok {
			mt.Note = strings.TrimSpace(m.noteInput.Value())
			m.meta = loader.UpsertMeta(m.meta, mt)
			loader.SaveMeta(m.meta) //nolint:errcheck
		}
		m.briefViewport.SetContent(m.renderCard())
		return m, nil
	case "esc":
		m.mode = modeNormal
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
		m.mode = modeNormal
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
		m.mode = modeNormal
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
		m.mode = modeNormal
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
		m.mode = modeNormal
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
		m.mode = modeNormal
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
		m.mode = modeNormal
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
			m.mode = modeNormal
			m.nameInput.Blur()
			return m, nil
		}
		old := mt.Name
		newName := strings.TrimSpace(m.nameInput.Value())
		updated, err := renameTool(m.meta, old, newName)
		if err != nil {
			m.mode = modeNormal
			m.nameInput.Blur()
			m.statusMsg = err.Error()
			return m, nil
		}
		m.mode = modeNormal
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
		m.mode = modeNormal
		m.nameInput.Blur()
		return m, nil
	default:
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		return m, cmd
	}
}

// updateConfirmUpdate handles the modeConfirmUpdate dialog (modeled on
// modeConfirmUntrack): enter launches the update — set updatingFor, reset the
// live log to the target tool, and fire the streaming command plus the spinner
// tick; esc (or any other key) cancels back to modeNormal. The plan awaiting
// confirmation lives in m.updatePlan.
func (m Model) updateConfirmUpdate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.mode = modeNormal
		mt, ok := m.selectedMeta()
		if !ok {
			return m, nil
		}
		m.updatingFor = mt.Name
		m.updateLog = nil
		m.updateLogFor = mt.Name
		m.briefViewport.SetContent(m.renderCard())
		// Text-change transition: [3] switches from help to the live log, so
		// the entry index empties and any spotlight cursor resets.
		m.setHelpContent()
		return m, tea.Batch(
			m.spinner.Tick,
			startUpdateCmd(m.updatePlan, mt.Name),
		)
	default:
		// esc or any other key cancels.
		m.mode = modeNormal
		return m, nil
	}
}

// updateAPIStatus handles keys while the API-status overlay is open: [e] opens
// the masked token-input sub-mode, [d] removes a config-sourced token, [r]
// refreshes the numbers, [esc] closes. While entering a token, submit validates
// via version.FetchRateWithToken before anything is persisted.
func (m Model) updateAPIStatus(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mode == modeTokenInput {
		switch msg.String() {
		case "enter":
			candidate := strings.TrimSpace(m.tokenInput.Value())
			if candidate == "" {
				m.mode = modeAPIStatus
				m.tokenInput.Blur()
				m.tokenError = ""
				return m, nil
			}
			// Validate the candidate against /rate_limit; SetToken runs only
			// after a 200 in the tokenValidatedMsg handler.
			return m, validateTokenCmd(candidate)
		case "esc":
			m.mode = modeAPIStatus
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
		m.mode = modeNormal
		m.tokenError = ""
		return m, nil
	case "r":
		return m, fetchRateCmd()
	case "e":
		m.mode = modeTokenInput
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

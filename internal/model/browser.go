package model

import (
	"os/exec"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"
)

// openURLMsg is returned after attempting to launch the system browser.
// err is nil on success and carries the launch error otherwise.
type openURLMsg struct {
	err error
}

// browserCommand resolves the binary and arguments used to open url in the
// default browser for the given GOOS. It is pure so it can be tested without
// launching a process.
func browserCommand(goos, url string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{url}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		return "xdg-open", []string{url}
	}
}

// openURLCmd builds a tea.Cmd that launches the system browser for url and
// reports the outcome via openURLMsg.
func openURLCmd(url string) tea.Cmd {
	return func() tea.Msg {
		name, args := browserCommand(runtime.GOOS, url)
		return openURLMsg{err: exec.Command(name, args...).Start()}
	}
}

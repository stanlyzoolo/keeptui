//go:build unix

// Package proc hardens tool subprocesses (help/version probes) against
// misbehaving CLI tools. A tracked tool that ignores its arguments and boots
// a TUI (ratatui/crossterm, tcell, bubbletea …) opens /dev/tty directly and
// toggles raw mode / the alternate screen on the terminal keys itself is
// drawing on, tearing the UI apart. Detaching the child from the controlling
// terminal makes that open fail (ENXIO), so such a tool exits immediately and
// harmlessly instead.
package proc

import (
	"os/exec"
	"syscall"
)

// DetachTTY runs cmd in its own session so it has no controlling terminal:
// any attempt to open /dev/tty fails instead of reaching keys' screen.
func DetachTTY(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

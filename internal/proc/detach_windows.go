//go:build windows

package proc

import (
	"os/exec"
	"syscall"
)

// DetachTTY starts cmd without a console (DETACHED_PROCESS), the Windows
// analog of dropping the controlling terminal: a child that tries to draw a
// TUI cannot reach keys' console via CONOUT$. Captured stdout/stderr pipes
// keep working.
func DetachTTY(cmd *exec.Cmd) {
	const detachedProcess = 0x00000008
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedProcess, HideWindow: true}
}

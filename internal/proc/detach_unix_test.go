//go:build unix

package proc

import (
	"os"
	"os/exec"
	"testing"
)

func TestDetachTTYSetsOwnSession(t *testing.T) {
	cmd := exec.Command("true")
	DetachTTY(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setsid {
		t.Fatalf("DetachTTY must set Setsid, got %+v", cmd.SysProcAttr)
	}
}

// TestDetachTTYBlocksControllingTerminal proves the property the whole fix
// rests on: a detached child cannot open /dev/tty. Meaningful only when the
// test itself runs on a terminal — without one the open fails either way.
func TestDetachTTYBlocksControllingTerminal(t *testing.T) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no controlling terminal in this environment: %v", err)
	}
	_ = tty.Close()

	if err := exec.Command("sh", "-c", ": < /dev/tty").Run(); err != nil {
		t.Fatalf("attached child should reach /dev/tty, got %v", err)
	}

	cmd := exec.Command("sh", "-c", ": < /dev/tty")
	DetachTTY(cmd)
	if err := cmd.Run(); err == nil {
		t.Fatal("detached child opened /dev/tty; expected failure")
	}
}

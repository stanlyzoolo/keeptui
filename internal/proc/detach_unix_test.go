//go:build unix

package proc

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestDetachTTYSetsOwnSession(t *testing.T) {
	cmd := exec.Command("true")
	DetachTTY(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setsid {
		t.Fatalf("DetachTTY must set Setsid, got %+v", cmd.SysProcAttr)
	}
}

// TestKillGroupNilProcess: KillGroup is a no-op (no panic, no error) on a
// command that was never started, so callers can defer it unconditionally.
func TestKillGroupNilProcess(t *testing.T) {
	if err := KillGroup(nil); err != nil {
		t.Errorf("KillGroup(nil) = %v, want nil", err)
	}
	if err := KillGroup(exec.Command("true")); err != nil {
		t.Errorf("KillGroup(unstarted) = %v, want nil", err)
	}
}

// TestKillGroupTerminatesGroup starts a detached child that spawns a
// long-sleeping grandchild and verifies KillGroup (negative-pid SIGKILL) tears
// down the whole group, not just the direct child.
func TestKillGroupTerminatesGroup(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 60 & wait")
	DetachTTY(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := KillGroup(cmd); err != nil {
		t.Fatalf("KillGroup: %v", err)
	}
	// Wait must return (killed), not hang until the 60s sleep elapses.
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("process group still alive after KillGroup")
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

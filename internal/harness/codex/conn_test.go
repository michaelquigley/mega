package codex

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// a child that starts its own session, as codex-linux-sandbox does, is found
// while its parent lives and reported after the parent exits.
func TestDescendantsOutlivingTheirParentAreReported(t *testing.T) {
	cmd := exec.Command("sh", "-c", "setsid sleep 30 & sleep 0.5")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	c := &conn{cmd: cmd}
	var seen []proc
	deadline := time.Now().Add(2 * time.Second)
	for len(seen) < 2 && time.Now().Before(deadline) {
		seen = c.descendants()
		time.Sleep(20 * time.Millisecond)
	}
	if len(seen) < 2 {
		t.Fatalf("descendants = %+v", seen)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	alive := survivors(seen, 300*time.Millisecond)
	if len(alive) != 1 {
		t.Fatalf("survivors = %v from %+v", alive, seen)
	}
	if err := syscall.Kill(alive[0], syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	if left := survivors(seen, 2*time.Second); len(left) != 0 {
		t.Errorf("still reported after it was killed: %v", left)
	}
}

package environ

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

const ttyChild = "MEGA_ENVIRON_TTY_CHILD"

// a mega run from a terminal resolves as one run without: the resolution's
// interactive shell must never wait on that terminal. the test runs itself
// again as a child whose controlling terminal is a new pseudo-terminal, and
// the child resolves there, whatever terminal the test itself has or lacks.
func TestResolvesUnderATerminal(t *testing.T) {
	if os.Getenv(ttyChild) == "1" {
		r := &Resolver{Shell: "/bin/bash", Base: []string{"HOME=" + os.Getenv("MEGA_ENVIRON_TTY_HOME"), "PATH=" + systemPath}, Timeout: 3 * time.Second}
		res, err := r.Resolve(context.Background(), os.Getenv("MEGA_ENVIRON_TTY_HOME"), "export MEGA_MARKER=tty")
		if err != nil {
			t.Fatal(err)
		}
		if v, _ := Value(res.Env, "MEGA_MARKER"); v != "tty" {
			t.Fatalf("the marker is %q", v)
		}
		return
	}
	if _, err := os.Stat("/bin/bash"); err != nil {
		t.Skip("bash is not installed")
	}
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer master.Close()
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	n, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	slave, err := os.OpenFile("/dev/pts/"+strconv.Itoa(n), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer slave.Close()
	go func() { _, _ = io.Copy(io.Discard, master) }()

	cmd := exec.Command(os.Args[0], "-test.run=^TestResolvesUnderATerminal$", "-test.count=1")
	cmd.Env = append(os.Environ(), ttyChild+"=1", "MEGA_ENVIRON_TTY_HOME="+t.TempDir())
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	// the child leads a new session with the pseudo-terminal as its
	// controlling terminal, in the foreground, as a mega in a terminal is.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("resolving under a terminal failed: %v", err)
		}
	case <-time.After(20 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("resolving under a terminal hung")
	}
}

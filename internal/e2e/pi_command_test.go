//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/service"
)

// a command pi runs with no dialog, stopped mid-way: by an interrupt (R1), and
// by pi itself being killed (R4). the command is `timeout N tail -f FILE` on
// a file of the test's own, named for this run: it holds until stopped, it
// says no "sleep", so the fixture's gate does not ask, and the file's name
// in a process's command line says the command is still running.

// heldFile makes the file a held command follows, named for this run, and
// returns its name in the fixture.
func heldFile(t *testing.T, fixture, scenario string) string {
	t.Helper()
	name := fmt.Sprintf("pi-%s-%d.txt", scenario, time.Now().UnixNano()%1000000)
	path := filepath.Join(fixture, name)
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
	return name
}

// commandRuns waits until a process holding name runs, the command the turn
// was asked for. a turn that ends first skips when pi's model did not answer
// and fails otherwise; a dialog fails the test unanswered.
func (s *session) commandRuns(name string) {
	s.t.Helper()
	view := s.waitFor("the command to run", func(v service.ConversationView) bool {
		return len(processesNaming(name)) > 0 || len(v.Pending) > 0 || (v.State == service.StateRunning && s.settled(v))
	})
	switch {
	case len(view.Pending) > 0:
		s.t.Fatalf("pi raised a dialog for an ungated command: %+v; it was not answered", view.Pending)
	case len(processesNaming(name)) == 0:
		s.skipUnanswered()
		s.t.Fatalf("the turn ended before the command ran; the reply was %q", s.lastAssistant())
	}
}

// gone waits a few seconds for no process to hold name, and says whether
// none does.
func gone(name string) bool {
	deadline := time.Now().Add(5 * time.Second)
	for len(processesNaming(name)) > 0 {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(250 * time.Millisecond)
	}
	return true
}

// TestPiInterruptsARunningCommand (R1): an interrupt while an ungated command
// runs ends the turn interrupted and stops the command, and the next turn is
// answered.
func TestPiInterruptsARunningCommand(t *testing.T) {
	fixture := fixture(t, "MEGA_E2E_PI_FIXTURE")
	name := heldFile(t, fixture, "r1")
	svc := newE2EService(t, newPiHarness())
	project, err := svc.RegisterProject("pi-rt1", fixture)
	if err != nil {
		t.Fatal(err)
	}
	conv, err := svc.StartConversation(project.ID, model.HarnessPi, "e2e interrupt a command", "Using the bash tool, run exactly `timeout 45 tail -f "+name+"` and then reply INT-ACTIVE-1. Nothing else.", service.LaunchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{t: t, svc: svc, id: conv.ID}
	s.commandRuns(name)
	if err := svc.Interrupt(s.id); err != nil {
		t.Fatal(err)
	}
	s.waitFor("the interrupted turn to end", func(v service.ConversationView) bool { return !v.TurnActive })
	evs, err := svc.Events(s.id, 0)
	if err != nil {
		t.Fatal(err)
	}
	if last := evs[len(evs)-1]; last.Kind != model.EventTurnEnd || last.Status != model.TurnInterrupted {
		t.Errorf("the turn ended %s %s, want interrupted", last.Kind, last.Status)
	}
	if !gone(name) {
		t.Errorf("the command still runs after the interrupt: pids %v", processesNaming(name))
	}
	s.prompt("Reply with exactly AFTER-INTERRUPT-dd44 and nothing else.")
	s.idle()
	s.expectReply("AFTER-INTERRUPT-dd44")
	shutdown(t, svc, s)
}

// TestPiResumesAfterPiIsKilled (R4): pi is killed while an ungated command
// runs. the runtime is seen to exit, and the next prompt resumes that
// session by its exact id, which remembers the command it was asked for.
// what pi left in its session file is not read. only the pi this test's own
// service started is killed: the one process whose parent is this test's
// process and whose working directory is the fixture, never one found by its
// name. pi 1.6.1's process title hides the session id it was started with.
func TestPiResumesAfterPiIsKilled(t *testing.T) {
	fixture := fixture(t, "MEGA_E2E_PI_FIXTURE")
	name := heldFile(t, fixture, "r4")
	svc := newE2EService(t, newPiHarness())
	project, err := svc.RegisterProject("pi-rt1", fixture)
	if err != nil {
		t.Fatal(err)
	}
	conv, err := svc.StartConversation(project.ID, model.HarnessPi, "e2e pi killed", "Using the bash tool, run exactly `timeout 30 tail -f "+name+"` and then reply OWNER-KILL-1. Nothing else.", service.LaunchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{t: t, svc: svc, id: conv.ID}
	s.commandRuns(name)
	pids := piChildren(t, fixture)
	if len(pids) != 1 {
		t.Fatalf("expected exactly one process of this test's own in the fixture, its pi, got pids %v; none was killed", pids)
	}
	if err := syscall.Kill(pids[0], syscall.SIGKILL); err != nil {
		t.Fatalf("killing pi %d: %v", pids[0], err)
	}
	s.waitFor("the runtime to exit", func(v service.ConversationView) bool { return v.State == model.RuntimeNone })
	evs, err := svc.Events(s.id, 0)
	if err != nil {
		t.Fatal(err)
	}
	last := evs[len(evs)-1]
	t.Logf("pi %v killed; the conversation recorded %s %s %q; the command still runs: %v", pids, last.Kind, last.Status, last.Text, !gone(name))

	s.prompt("Reply with the exact command you were asked to run in this conversation, then RECOVERED-ff66 on its own line. Nothing else.")
	s.idle()
	s.skipUnanswered()
	s.expectReply(name, "RECOVERED-ff66")
	if facts := s.view().Facts; facts == nil || facts.Session != conv.NativeID {
		t.Errorf("resumed session = %+v, bound to %s", facts, conv.NativeID)
	}
	shutdown(t, svc, s)
}

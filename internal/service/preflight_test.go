package service

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

// a harness whose executable cannot be found is named on every workspace
// read with mega's PATH, refuses launch, prompt and Start with its own code
// naming the executable, and comes back when it can be found again.
func TestAMissingExecutableIsPreflightedAndRefused(t *testing.T) {
	r := newRig(t)
	running := r.start("one")
	rt := r.fake.Last()
	started(rt)
	finishTurn(rt, "a-1", "done")
	stopped := r.start("two")
	stoppedRT := r.fake.Last()
	started(stoppedRT)
	finishTurn(stoppedRT, "b-1", "done")
	if err := r.svc.Release(stopped.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the release", stoppedRT.Exited)
	if p := r.svc.Workspace().Preflight; len(p.Missing) != 0 || p.Path != os.Getenv("PATH") {
		t.Fatalf("all found: %+v", p)
	}

	r.fake.SetMissing(&harness.MissingError{Harness: model.HarnessClaude, Executable: "node", Reason: "'node' was not found on mega's PATH"})
	p := r.svc.Workspace().Preflight
	if len(p.Missing) != 1 || p.Missing[0].Executable != "node" || p.Missing[0].Harness != model.HarnessClaude || p.Path != os.Getenv("PATH") {
		t.Fatalf("missing node: %+v", p)
	}
	want := func(err error) {
		t.Helper()
		wantCode(t, err, CodeExecutableMissing)
		if err == nil || !strings.Contains(err.Error(), "'node' was not found on mega's PATH") {
			t.Errorf("message = %v", err)
		}
	}
	launched := len(r.fake.Runtimes())
	_, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "", "new", LaunchChoice{})
	want(err)
	want(r.svc.Prompt(running.ID, "more"))
	want(r.svc.Prompt(stopped.ID, "more"))
	want(r.svc.Start(stopped.ID))
	if len(r.fake.Runtimes()) != launched || len(rt.Prompts()) != 1 {
		t.Error("work reached a harness that cannot run")
	}

	r.fake.SetMissing(nil)
	if p := r.svc.Workspace().Preflight; len(p.Missing) != 0 {
		t.Errorf("found again: %+v", p)
	}
	if err := r.svc.Start(stopped.ID); err != nil {
		t.Errorf("start once found: %v", err)
	}
}

// reading native history with an executable missing names the condition,
// never an internal error: a raw exec failure is checked against the harness.
func TestAMissingExecutableWhileReadingHistoryIsNamed(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	finishTurn(rt, "a-1", "done")
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the release", rt.Exited)

	r.fake.FailHistory(&exec.Error{Name: "node", Err: exec.ErrNotFound})
	r.fake.SetMissing(&harness.MissingError{Harness: model.HarnessClaude, Executable: "node", Reason: "'node' was not found on mega's PATH"})
	err := r.svc.Resume(conv.ID)
	wantCode(t, err, CodeExecutableMissing)
	if err == nil || !strings.Contains(err.Error(), "node") {
		t.Errorf("resume: %v", err)
	}

	// a history failure that is not a missing executable passes through as
	// it was.
	r.fake.SetMissing(nil)
	r.fake.FailHistory(errors.New("the transcript is unreadable"))
	if err := r.svc.Resume(conv.ID); err == nil || ErrorCode(err) == CodeExecutableMissing {
		t.Errorf("another failure: %v", err)
	}
}

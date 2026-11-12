package service

import (
	"errors"
	"github.com/michaelquigley/mega/internal/environ"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

// a harness whose executable cannot be found in the project's resolved
// environment is named on every workspace read, refuses launch, prompt and
// Start with its own code naming the executable, and comes back when it can
// be found again. the check runs in the project's environment, never mega's.
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
	if st := r.svc.Workspace().Environments[r.project.ID]; len(st.Missing) != 0 || st.Resolved == nil {
		t.Fatalf("all found: %+v", st)
	}
	checked := r.fake.Checked()
	if len(checked) == 0 {
		t.Fatal("the harness was never checked")
	}
	for _, env := range checked {
		if home, _ := environ.Value(env, "HOME"); home == os.Getenv("HOME") || home == "" {
			t.Fatalf("checked in mega's own environment, or none: HOME is %q", home)
		}
	}

	r.fake.SetMissing(&harness.MissingError{Harness: model.HarnessClaude, Executable: "node", Reason: "'node' was not found on the project's PATH"})
	st := r.svc.Workspace().Environments[r.project.ID]
	if len(st.Missing) != 1 || st.Missing[0].Executable != "node" || st.Missing[0].Harness != model.HarnessClaude {
		t.Fatalf("missing node: %+v", st.Missing)
	}
	want := func(err error) {
		t.Helper()
		wantCode(t, err, CodeExecutableMissing)
		if err == nil || !strings.Contains(err.Error(), "'node' was not found on the project's PATH") {
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
	if st := r.svc.Workspace().Environments[r.project.ID]; len(st.Missing) != 0 {
		t.Errorf("found again: %+v", st.Missing)
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
	r.fake.SetMissing(&harness.MissingError{Harness: model.HarnessClaude, Executable: "node", Reason: "'node' was not found on the project's PATH"})
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

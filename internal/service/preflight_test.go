package service

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/michaelquigley/mega/internal/environ"
	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/harness/harnesstest"
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
	if st := workspaceOf(t, r.svc).Environments[r.project.ID]; len(st.Missing) != 0 || st.Resolved == nil {
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
	st := workspaceOf(t, r.svc).Environments[r.project.ID]
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
	if st := workspaceOf(t, r.svc).Environments[r.project.ID]; len(st.Missing) != 0 {
		t.Errorf("found again: %+v", st.Missing)
	}
	if err := r.svc.Start(stopped.ID); err != nil {
		t.Errorf("start once found: %v", err)
	}
}

// reading native history for an adoption with an executable missing names
// the condition, never an internal error: a raw exec failure is checked
// against the harness.
func TestAMissingExecutableWhileReadingHistoryIsNamed(t *testing.T) {
	r := newRig(t)
	r.fake.SetSessions([]harness.Session{{NativeID: "terminal-1", Preview: "from the terminal"}})

	r.fake.FailHistory(&exec.Error{Name: "node", Err: exec.ErrNotFound})
	r.fake.SetMissing(&harness.MissingError{Harness: model.HarnessClaude, Executable: "node", Reason: "'node' was not found on the project's PATH"})
	_, err := r.svc.Adopt(r.project.ID, model.HarnessClaude, "terminal-1", "")
	wantCode(t, err, CodeExecutableMissing)
	if err == nil || !strings.Contains(err.Error(), "node") {
		t.Errorf("adoption: %v", err)
	}

	// a history failure that is not a missing executable passes through as
	// it was.
	r.fake.SetMissing(nil)
	r.fake.FailHistory(errors.New("the transcript is unreadable"))
	if _, err := r.svc.Adopt(r.project.ID, model.HarnessClaude, "terminal-1", ""); err == nil || ErrorCode(err) == CodeExecutableMissing {
		t.Errorf("another failure: %v", err)
	}
	if n := len(workspaceOf(t, r.svc).Conversations); n != 0 {
		t.Errorf("a failed adoption left %d conversations", n)
	}
}

// findsByShell is a resolver whose shell, before any activation, has
// MEGA_TEST_FIND=old while the activation that runs names old-shell: where
// mega finds its programs then differs between two activations, as it can
// when the shell's own PATH changed between their resolutions.
type findsByShell struct{ inner Resolver }

func (f findsByShell) Resolve(ctx context.Context, dir, activate string) (*environ.Resolution, error) {
	res, err := f.inner.Resolve(ctx, dir, activate)
	if err == nil && strings.Contains(activate, "old-shell") {
		res.Before = append(slices.Clone(res.Before), "MEGA_TEST_FIND=old")
	}
	return res, err
}

// missingInOldShell is a fake harness that cannot be found where the shell
// has MEGA_TEST_FIND=old.
type missingInOldShell struct{ *harnesstest.Harness }

func (h missingInOldShell) Check(env []string) error {
	if v, _ := environ.Value(env, "MEGA_TEST_FIND"); v == "old" {
		return &harness.MissingError{Harness: h.Name(), Executable: "node", Reason: "'node' was not found on the project's PATH"}
	}
	return h.Harness.Check(env)
}

// a resolution counts only while it came from the activation that would run
// now. once the override changes the activation, what the older resolution
// found missing refuses nothing: the project reads as not resolved yet, and
// the next start resolves afresh and starts.
func TestAStartAfterTheActivationChangedIsNotRefusedOnTheOldResult(t *testing.T) {
	resolver := &countingResolver{inner: findsByShell{inner: testResolver(t)}, calls: map[string]int{}}
	fake := missingInOldShell{harnesstest.New(model.HarnessClaude)}
	svc := countingService(t, filepath.Join(t.TempDir(), "registry.yaml"), resolver, fake)
	checkout := t.TempDir()
	project, err := svc.RegisterProject("fixture", checkout)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetEnvironmentOverride(project.ID, &model.EnvOverride{Activate: "export MEGA_A=1 # old-shell"}); err != nil {
		t.Fatal(err)
	}
	_, err = svc.StartConversation(project.ID, model.HarnessClaude, "", "hello", LaunchChoice{})
	wantCode(t, err, CodeExecutableMissing)
	if st := workspaceOf(t, svc).Environments[project.ID]; st.Resolved == nil || len(st.Missing) != 1 {
		t.Fatalf("under the old activation: resolved %+v, missing %+v", st.Resolved, st.Missing)
	}

	if _, err := svc.SetEnvironmentOverride(project.ID, &model.EnvOverride{Activate: "export MEGA_A=2"}); err != nil {
		t.Fatal(err)
	}
	if st := workspaceOf(t, svc).Environments[project.ID]; st.Resolved != nil || len(st.Missing) != 0 {
		t.Fatalf("the old activation's resolution still counts: resolved %+v, missing %+v", st.Resolved, st.Missing)
	}
	before := resolver.count(checkout)
	if _, err := svc.StartConversation(project.ID, model.HarnessClaude, "", "hello", LaunchChoice{}); err != nil {
		t.Fatalf("the start was refused on the old result: %v", err)
	}
	if n := resolver.count(checkout) - before; n != 1 {
		t.Errorf("the start resolved %d times, want 1", n)
	}
	if len(fake.Runtimes()) != 1 {
		t.Errorf("%d runtimes launched, want 1", len(fake.Runtimes()))
	}
}

package service

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/environ"
	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/harness/harnesstest"
	"github.com/michaelquigley/mega/internal/model"
)

// the faults of the fourth full review (roadmap, harden-requests-stops-and-
// restarts): each test fails without its fix.

// stoppedWithSession leaves a conversation with a session and no runtime.
func (r *rig) stoppedWithSession() ConversationView {
	r.t.Helper()
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	finishTurn(rt, "a-1", "done")
	if err := r.svc.Release(conv.ID); err != nil {
		r.t.Fatal(err)
	}
	waitFor(r.t, "the release", func() bool { return r.view(conv.ID).State == model.RuntimeNone })
	return r.view(conv.ID)
}

func (r *rig) isClosed() bool {
	r.svc.mu.Lock()
	defer r.svc.mu.Unlock()
	return r.svc.closed
}

// a shutdown that begins while a start is reading the harness's version, the
// last step before the launch, starts no harness.
func TestNoLaunchAfterAShutdownBegins(t *testing.T) {
	r := newRig(t)
	conv := r.stoppedWithSession()
	done := make(chan struct{})
	r.fake.OnVersion(func() {
		go func() {
			r.svc.Shutdown(context.Background())
			close(done)
		}()
		waitFor(t, "the shutdown to begin", r.isClosed)
	})
	launched := len(r.fake.Runtimes())
	wantCode(t, r.svc.Start(conv.ID), CodeUnavailable)
	if len(r.fake.Runtimes()) != launched {
		t.Error("a harness was launched after the shutdown began")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the shutdown did not finish")
	}
	if view := r.view(conv.ID); view.Runtime != model.RuntimeNone {
		t.Errorf("a start refused at the launch left the conversation %q", view.Runtime)
	}
}

// a launch already under way when the shutdown begins is waited for: the
// shutdown does not finish while the runtime it started is still running.
func TestAShutdownWaitsForALaunchUnderWay(t *testing.T) {
	r := newRig(t)
	conv := r.stoppedWithSession()
	done := make(chan struct{})
	var late *harnesstest.Runtime
	r.fake.DuringLaunch(func(rt *harnesstest.Runtime) {
		late = rt
		rt.HoldOnClose()
		go func() {
			r.svc.Shutdown(context.Background())
			close(done)
		}()
		waitFor(t, "the shutdown to begin", r.isClosed)
	})
	wantCode(t, r.svc.Start(conv.ID), CodeUnavailable)
	select {
	case <-done:
		t.Fatal("the shutdown finished while the runtime launched during it still ran")
	case <-time.After(200 * time.Millisecond):
	}
	late.Exit(nil)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the shutdown did not finish once the runtime exited")
	}
}

// an environment file that changes while the harness's version is read
// refuses the launch: the check is the last thing before the harness starts.
func TestAnEnvironmentChangedBeforeTheLaunchRefusesIt(t *testing.T) {
	r := newRig(t)
	writeEnvFile(t, r.project.Checkout, "export MEGA_MARKER="+markerOne)
	r.approve(r.project.ID)
	r.fake.OnVersion(func() {
		writeEnvFile(t, r.project.Checkout, "export MEGA_MARKER="+markerTwo)
	})
	launched := len(r.fake.Runtimes())
	_, err := r.startIn(r.project.ID, "hello")
	wantCode(t, err, CodeEnvironmentChanged)
	if len(r.fake.Runtimes()) != launched {
		t.Error("a runtime launched on a file that changed before the launch")
	}
}

// changingResolver changes the project's environment file as a resolution
// finishes, once when is true, as an edit landing while the activation ran.
type changingResolver struct {
	inner  Resolver
	when   func() bool
	change func()
}

func (c *changingResolver) Resolve(ctx context.Context, dir, activate string) (*environ.Resolution, error) {
	res, err := c.inner.Resolve(ctx, dir, activate)
	if c.when != nil && c.when() {
		c.when = nil
		c.change()
	}
	return res, err
}

// adoption reads native history by running the harness in the project's
// environment: a file that changes while that environment is resolved
// refuses the read, as it refuses a launch.
func TestAHistoryReadChecksTheEnvironmentItRunsIn(t *testing.T) {
	r := newRig(t)
	r.stoppedWithSession()
	fresh := helperFresh
	helperFresh = 0
	t.Cleanup(func() { helperFresh = fresh })
	writeEnvFile(t, r.project.Checkout, "export MEGA_MARKER="+markerOne)
	r.approve(r.project.ID)
	resolver := &changingResolver{inner: r.svc.resolver, change: func() {
		writeEnvFile(t, r.project.Checkout, "export MEGA_MARKER="+markerTwo)
	}}
	r.svc.resolver = resolver

	r.fake.SetSessions([]harness.Session{{NativeID: "terminal-1", Preview: "from the terminal"}})
	r.fake.SetHistory("terminal-1", []*model.Event{{Kind: model.EventUser, Text: "from the terminal", NativeID: "t-1"}})
	// Resolve discovery in the approved environment. Change the file during
	// the second resolution, just before the adoption snapshot would read it.
	resolutions := 0
	resolver.when = func() bool {
		resolutions++
		return resolutions == 2
	}
	reads := len(r.fake.Reads())
	_, err := r.svc.Adopt(r.project.ID, model.HarnessClaude, "terminal-1", "adopted")
	wantCode(t, err, CodeEnvironmentChanged)
	if len(r.svc.Workspace().Conversations) != 1 {
		t.Error("adoption published a conversation before the approval check")
	}
	if len(r.fake.Reads()) != reads {
		t.Error("an adoption read history in an environment whose file changed")
	}
}

// a request whose abort could not be recorded when its runtime exited stays
// listed as last recorded, and takes no further answer: none reaches the
// runtime that replaces it.
func TestADeadRuntimesRequestTakesNoFurtherAnswer(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	id := rt.Ask(model.RequestPermission, map[string]any{"tool": "Bash", "input": map[string]any{"command": "echo"}})
	r.svc.failRecord = func(ev *model.Event) error {
		if ev.Kind == model.EventRequestResolved {
			r.svc.failRecord = nil
			return errors.New("the log could not be written")
		}
		return nil
	}
	rt.Exit(nil)
	if pending := r.view(conv.ID).Pending; !slices.ContainsFunc(pending, func(req model.Request) bool { return req.ID == id }) {
		t.Fatalf("an abort that could not be recorded hid the request: %+v", pending)
	}
	if err := r.svc.Start(conv.ID); err != nil {
		t.Fatal(err)
	}
	next := r.fake.Last()
	if next == rt {
		t.Fatal("no new runtime")
	}
	reached := 0
	next.OnAnswer(func() { reached++ })
	wantCode(t, r.svc.Answer(conv.ID, id, harness.Answer{Decision: harness.DecisionDeny}), CodeNotPending)
	if reached != 0 {
		t.Errorf("an answer to the dead runtime's request reached its replacement %d times", reached)
	}
}

// a turn's output whose unread marker the registry refused is counted at the
// next start, from the log, which recorded it: the marker is derived from
// the log, and nothing retries the write meanwhile.
func TestAnUnreadMarkerTheRegistryRefusedIsReconciledAtStart(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	r.readAll(conv.ID)
	rt.Emit(&model.Event{Kind: model.EventAssistant, Text: "the answer", NativeID: "a-1"})
	refused := 0
	r.svc.failUpdate = func() error {
		refused++
		return errors.New("the registry could not be written")
	}
	rt.Emit(&model.Event{Kind: model.EventTurnEnd, Status: model.TurnSuccess})
	r.svc.failUpdate = nil
	if refused == 0 {
		t.Fatal("the turn's end wrote nothing to the registry")
	}
	if view := r.view(conv.ID); view.OutputSeq != 0 {
		t.Fatalf("the refused marker was kept: %d", view.OutputSeq)
	}
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the release", func() bool { return r.view(conv.ID).State == model.RuntimeNone })
	r.restart()
	out := seqOf(r.events(conv.ID), "a-1")
	if view := r.view(conv.ID); view.OutputSeq != out || view.Attention != model.AttentionUnread {
		t.Errorf("after the restart: output seq %d (want %d), attention %q", view.OutputSeq, out, view.Attention)
	}
}

// the fifth full review: what start-up rebuilds must be in the log, and an
// error a retry has made good is cleared.

// output an adapter marks as after its turn, a codex background command
// finishing, is counted from the log at the next start: that it came after
// its turn is in the log, not only in memory.
func TestLateOutputsMarkerIsRebuiltFromTheLog(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	finishTurn(rt, "a-1", "done")
	r.readAll(conv.ID)
	r.svc.failUpdate = func() error { return errors.New("the registry could not be written") }
	rt.Emit(&model.Event{Kind: model.EventToolResult, Text: "the background command finished", NativeID: "late-1", AfterTurn: true})
	r.svc.failUpdate = nil
	late := seqOf(r.events(conv.ID), "late-1")
	if late == 0 || r.view(conv.ID).OutputSeq == late {
		t.Fatalf("the refused marker: late output %d, output seq %d", late, r.view(conv.ID).OutputSeq)
	}
	r.restart() // a crash: nothing ends the turn the late output would open
	if view := r.view(conv.ID); view.OutputSeq != late {
		t.Errorf("after the restart: output seq %d, want the late output's %d", view.OutputSeq, late)
	}
}

// an error that came from the runtime is not cleared by a resume, which
// starts nothing: only a runtime that starts clears it.
func TestAResumeLeavesARuntimesError(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	rt.Exit(errors.New("the process died"))
	if got := r.attention(conv.ID); got != model.AttentionError {
		t.Fatalf("after the crash: %q", got)
	}
	if err := r.svc.Resume(conv.ID); err != nil {
		t.Fatal(err)
	}
	if got := r.attention(conv.ID); got != model.AttentionError {
		t.Errorf("a resume cleared the runtime's error: %q", got)
	}
}

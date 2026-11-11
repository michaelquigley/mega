package service

import (
	"context"
	"slices"
	"testing"
	"time"

	"git.hq.quigley.com/research/roundhouse/internal/harness"
	"git.hq.quigley.com/research/roundhouse/internal/harness/harnesstest"
	"git.hq.quigley.com/research/roundhouse/internal/model"
)

// idleConversation starts a conversation whose first turn has ended, so its
// runtime is warm and idle.
func idleConversation(r *rig, prompt string) (ConversationView, *harnesstest.Runtime) {
	r.t.Helper()
	conv := r.start(prompt)
	rt := r.fake.Last()
	started(rt)
	finishTurn(rt, "a-"+prompt, "done")
	return conv, rt
}

func kindsOf(b Blocker) []string {
	var out []string
	if b.Turn {
		out = append(out, BlockerTurn)
	}
	if b.Requests > 0 {
		out = append(out, BlockerRequest)
	}
	if b.Background > 0 {
		out = append(out, BlockerBackground)
	}
	if b.Settling != "" {
		out = append(out, BlockerSettling)
	}
	return out
}

func (r *rig) gate() Gate {
	r.t.Helper()
	return r.svc.Workspace().Gate
}

// armed, the gate refuses everything that would start new work, with its own
// code, and leaves alone everything that lets work finish.
func TestArmedGateRefusesNewWorkOnly(t *testing.T) {
	r := newRig(t)
	running, rt := idleConversation(r, "one")
	stopped, stoppedRT := idleConversation(r, "two")
	if err := r.svc.Release(stopped.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the release", stoppedRT.Exited)
	r.fake.SetSessions([]harness.Session{{NativeID: "terminal-1", Preview: "from a terminal"}})
	if err := r.svc.Prompt(running.ID, "a turn in flight"); err != nil {
		t.Fatal(err)
	}
	request := rt.Ask(model.RequestPermission, map[string]any{"tool": "Bash"})

	if err := r.svc.ArmShutdown(); err != nil {
		t.Fatal(err)
	}
	launched := len(r.fake.Runtimes())
	_, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "", "new", LaunchChoice{})
	wantCode(t, err, CodeShutdownPending)
	_, err = r.svc.Adopt(r.project.ID, model.HarnessClaude, "terminal-1", "")
	wantCode(t, err, CodeShutdownPending)
	wantCode(t, r.svc.Prompt(stopped.ID, "wake up"), CodeShutdownPending)
	wantCode(t, r.svc.Start(stopped.ID), CodeShutdownPending)
	if len(r.fake.Runtimes()) != launched {
		t.Error("the gate let a launch through")
	}
	if err := r.svc.Prompt(running.ID, "more"); ErrorCode(err) != CodeShutdownPending && ErrorCode(err) != CodeConflict {
		t.Errorf("prompt to a running conversation: %v", err)
	}

	// everything that lets work finish stays available.
	if err := r.svc.Answer(running.ID, request, harness.Answer{Decision: "allow"}); err != nil {
		t.Errorf("answer: %v", err)
	}
	if err := r.svc.Interrupt(running.ID); err != nil {
		t.Errorf("interrupt: %v", err)
	}
	if _, err := r.svc.Events(running.ID, 0); err != nil {
		t.Errorf("read: %v", err)
	}
	if err := r.svc.Release(running.ID); err != nil {
		t.Errorf("release: %v", err)
	}

	// cancelling lifts it at once.
	if err := r.svc.CancelShutdown(); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Start(stopped.ID); err != nil {
		t.Errorf("start after cancel: %v", err)
	}
}

// idle is computed from turns, pending requests, background jobs and
// conversations still starting, in any conversation; a warm runtime that is
// merely idle is not in the way.
func TestIdleIsComputedFromWhatIsInFlight(t *testing.T) {
	r := newRig(t)
	a, rtA := idleConversation(r, "a")
	b, rtB := idleConversation(r, "b")
	c, rtC := idleConversation(r, "c")
	if g := r.gate(); !g.Idle() || g.Armed {
		t.Fatalf("three warm idle runtimes: %+v", g)
	}

	if err := r.svc.Prompt(a.ID, "turn"); err != nil {
		t.Fatal(err)
	}
	rtB.Emit(&model.Event{Kind: model.EventAssistant, Text: "working", NativeID: "b-2"})
	request := rtB.Ask(model.RequestPermission, map[string]any{"tool": "Bash"})
	rtC.Background([]model.BackgroundJob{{ID: "j-1", Description: "sleep 30", Kind: "local_bash", StartedAt: time.Now()}})

	g := r.gate()
	got := map[string][]string{}
	for _, blocker := range g.Blockers {
		got[blocker.ConversationID] = kindsOf(blocker)
		if blocker.Name == "" || blocker.ProjectID != r.project.ID {
			t.Errorf("blocker without its name or project: %+v", blocker)
		}
	}
	if g.Idle() || !slices.Equal(got[a.ID], []string{BlockerTurn}) || !slices.Contains(got[b.ID], BlockerRequest) || !slices.Equal(got[c.ID], []string{BlockerBackground}) {
		t.Fatalf("blockers = %v", got)
	}

	// as each settles, it leaves the gate; nothing settles it for the operator.
	finishTurn(rtA, "a-2", "done")
	if err := r.svc.Answer(b.ID, request, harness.Answer{Decision: "allow"}); err != nil {
		t.Fatal(err)
	}
	finishTurn(rtB, "b-3", "done")
	rtC.Background(nil)
	if g := r.gate(); !g.Idle() {
		t.Fatalf("after everything settled: %+v", g.Blockers)
	}
	for _, rt := range []*harnesstest.Runtime{rtA, rtB, rtC} {
		if rt.Interrupts() != 0 || rt.Closed() {
			t.Error("the gate interrupted or released a runtime")
		}
	}
}

// shut down is refused unless armed and idle at that moment; accepted, it
// asks for the stop once, and the orderly stop records every conversation
// with no runtime.
func TestShutDownNeedsArmedAndIdle(t *testing.T) {
	r := newRig(t)
	a, rtA := idleConversation(r, "a")
	b, _ := idleConversation(r, "b")
	sub := r.svc.Subscribe()
	defer sub.Close()

	wantCode(t, r.svc.ShutDown(), CodeConflict)
	if err := r.svc.ArmShutdown(); err != nil {
		t.Fatal(err)
	}
	expectChanged(t, sub)
	if g := r.gate(); !g.Armed || g.ArmedAt.IsZero() {
		t.Fatalf("armed: %+v", g)
	}

	request := rtA.Ask(model.RequestPermission, map[string]any{"tool": "Bash"})
	wantCode(t, r.svc.ShutDown(), CodeConflict)
	select {
	case <-r.svc.ShutdownRequested():
		t.Fatal("a refused shutdown was requested")
	default:
	}
	if err := r.svc.Answer(a.ID, request, harness.Answer{Decision: "deny"}); err != nil {
		t.Fatal(err)
	}
	finishTurn(rtA, "a-3", "denied")

	if err := r.svc.ShutDown(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-r.svc.ShutdownRequested():
	default:
		t.Fatal("the shutdown was not requested")
	}
	// accepted: nothing new starts and the gate cannot be lifted any more.
	wantCode(t, r.svc.CancelShutdown(), CodeUnavailable)
	wantCode(t, r.svc.ShutDown(), CodeUnavailable)

	// the stop serve runs: every runtime released, each recorded with none.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r.svc.Shutdown(ctx)
	for _, id := range []string{a.ID, b.ID} {
		if view := r.view(id); view.Runtime != model.RuntimeNone {
			t.Errorf("%s after the stop: %q", id, view.Runtime)
		}
	}
	r.restart()
	for _, id := range []string{a.ID, b.ID} {
		if view := r.view(id); view.Runtime != model.RuntimeNone || view.State != model.RuntimeNone {
			t.Errorf("%s after a restart: runtime %q, state %q", id, view.Runtime, view.State)
		}
	}
	if g := r.gate(); g.Armed {
		t.Error("the gate survived a restart")
	}
}

// cancelling is announced, and arming twice or cancelling twice changes
// nothing.
func TestArmAndCancelAreAnnounced(t *testing.T) {
	r := newRig(t)
	sub := r.svc.Subscribe()
	defer sub.Close()
	if err := r.svc.ArmShutdown(); err != nil {
		t.Fatal(err)
	}
	expectChanged(t, sub)
	at := r.gate().ArmedAt
	if err := r.svc.ArmShutdown(); err != nil || r.gate().ArmedAt != at {
		t.Errorf("arming again: %v, armed at %v then %v", err, at, r.gate().ArmedAt)
	}
	if err := r.svc.CancelShutdown(); err != nil {
		t.Fatal(err)
	}
	expectChanged(t, sub)
	if g := r.gate(); g.Armed || !g.ArmedAt.IsZero() {
		t.Errorf("cancelled: %+v", g)
	}
	if err := r.svc.CancelShutdown(); err != nil {
		t.Errorf("cancelling again: %v", err)
	}
}

func expectChanged(t *testing.T, sub *Subscription) {
	t.Helper()
	select {
	case update := <-sub.Updates():
		if !update.WorkspaceChanged {
			t.Errorf("update = %+v", update)
		}
	case <-time.After(time.Second):
		t.Fatal("no workspace_changed")
	}
}

// a launch whose runtime reports started before Launch returns is still in
// the way: the service does not hold the runtime it would have to stop.
func TestALaunchUnderWayHoldsTheGate(t *testing.T) {
	r := newRig(t)
	var during Gate
	var refused error
	r.fake.DuringLaunch(func(rt *harnesstest.Runtime) {
		started(rt)
		if err := r.svc.ArmShutdown(); err != nil {
			t.Error(err)
		}
		during = r.svc.Workspace().Gate
		refused = r.svc.ShutDown()
	})
	r.start("one")
	r.fake.DuringLaunch(nil)
	if len(during.Blockers) != 1 || during.Blockers[0].Settling != StateStarting {
		t.Errorf("gate inside the launch: %+v", during.Blockers)
	}
	wantCode(t, refused, CodeConflict)
	finishTurn(r.fake.Last(), "a-1", "done")
	if g := r.gate(); !g.Idle() {
		t.Fatalf("after the launch and its turn: %+v", g.Blockers)
	}
	if err := r.svc.ShutDown(); err != nil {
		t.Errorf("at idle: %v", err)
	}
}

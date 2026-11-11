package service

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/harness/harnesstest"
	"github.com/michaelquigley/mega/internal/model"
)

// a start after a terminal visit imports first, then resumes the exact session
// with no prompt, recorded starting then started, and leaves the conversation
// idle with a runtime the next prompt goes to.
func TestStartImportsThenResumesTheExactSessionIdle(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	finishTurn(rt, "a-1", "done")
	if err := r.svc.Read(conv.ID, r.view(conv.ID).LastSeq); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the runtime to exit", rt.Exited)

	r.fake.SetHistory(conv.NativeID, []*model.Event{{Kind: model.EventUser, Text: "from the terminal", NativeID: "t-1"}})
	read, readAtLaunch := false, false
	r.fake.OnHistory(func() { read = true })
	r.fake.OnLaunch(func() { readAtLaunch = read })
	before := len(r.events(conv.ID))
	if err := r.svc.Start(conv.ID); err != nil {
		t.Fatal(err)
	}
	next := r.fake.Last()
	if next == rt || !readAtLaunch {
		t.Fatalf("launched a new runtime: %v; history read before the launch: %v", next != rt, readAtLaunch)
	}
	if next.Spec.NativeID != conv.NativeID || !next.Spec.Idle {
		t.Errorf("spec = %+v, want the exact session %q, idle", next.Spec, conv.NativeID)
	}
	if len(next.Prompts()) != 0 {
		t.Errorf("a start sent %v", next.Prompts())
	}
	if view := r.view(conv.ID); view.State != StateStarting || view.Runtime != model.RuntimeRunning {
		t.Errorf("before the runtime reports: state %q, runtime %q", view.State, view.Runtime)
	}
	next.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: conv.NativeID, Model: "m", Version: "v", PermissionMode: "default", ModeInherited: true, ModeRequested: true}})
	tail := r.events(conv.ID)[before:]
	if got := kinds(tail); !slices.Equal(got, []string{"runtime:starting", "user", "runtime:started"}) {
		t.Errorf("events = %v", got)
	}
	if tail[1].Origin != model.OriginNative || tail[1].Text != "from the terminal" {
		t.Errorf("imported = %+v", tail[1])
	}
	view := r.view(conv.ID)
	if view.State != StateRunning || view.TurnActive || view.Attention != model.AttentionIdle || view.Facts == nil || !view.Facts.ModeRequested {
		t.Errorf("after start: state %q, turn %v, attention %q, facts %+v", view.State, view.TurnActive, view.Attention, view.Facts)
	}

	// claude's own report of the same mode replaces the requested one, still
	// inherited.
	next.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusModeChanged, Facts: &model.RuntimeFacts{PermissionMode: "default", ModeInherited: true}})
	if f := r.view(conv.ID).Facts; f.ModeRequested || !f.ModeInherited || f.PermissionMode != "default" {
		t.Errorf("after the report: %+v", f)
	}

	// started again: refused. a prompt goes to the runtime already there.
	wantCode(t, r.svc.Start(conv.ID), CodeConflict)
	if err := r.svc.Prompt(conv.ID, "two"); err != nil {
		t.Fatal(err)
	}
	if r.fake.Last() != next || len(next.Prompts()) != 1 {
		t.Errorf("the prompt launched again, or did not reach the started runtime")
	}
}

// a start is refused for an unsupervised conversation, for one with no
// session, and for one whose session went unrecorded; none launches anything.
func TestStartIsRefusedWithoutASessionToResume(t *testing.T) {
	r := newRig(t)

	unsupervised := r.start("one")
	started(r.fake.Last())
	r.restart()
	launched := len(r.fake.Runtimes())
	err := r.svc.Start(unsupervised.ID)
	wantCode(t, err, CodeConflict)
	if err == nil || !strings.Contains(err.Error(), "resume the conversation first") {
		t.Errorf("unsupervised: %v", err)
	}

	r.fake.FailLaunch(fmt.Errorf("%w: no node", harness.ErrNotStarted))
	fresh, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "fresh", "one", LaunchChoice{})
	if err == nil || fresh.NativeID != "" {
		t.Fatalf("expected a conversation with no session: %+v, %v", fresh, err)
	}
	r.fake.FailLaunch(errors.New("lost on the way"))
	unknown, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "unknown", "one", LaunchChoice{})
	if err == nil || unknown.NativeID != "" {
		t.Fatalf("expected a conversation whose session went unrecorded: %+v, %v", unknown, err)
	}
	r.fake.FailLaunch(nil)
	launched = len(r.fake.Runtimes())

	err = r.svc.Start(fresh.ID)
	wantCode(t, err, CodeConflict)
	if err == nil || !strings.Contains(err.Error(), "opening prompt") {
		t.Errorf("no session: %v", err)
	}
	if err := r.svc.Start(unknown.ID); !errors.Is(err, errSessionUnknown) {
		t.Errorf("unrecorded session: %v", err)
	}
	if len(r.fake.Runtimes()) != launched {
		t.Error("a refused start launched a runtime")
	}
	if got := kinds(r.events(fresh.ID)); !slices.Equal(got, []string{"runtime:starting", "runtime:not_started"}) {
		t.Errorf("a refused start recorded something: %v", got)
	}
}

// a release asked for before an idle start's report keeps the stopping it
// recorded when the report arrives late; the exit settles it.
func TestLateStartedReportKeepsAReleaseStopping(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	finishTurn(rt, "a-1", "done")
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the runtime to exit", rt.Exited)
	if err := r.svc.Start(conv.ID); err != nil {
		t.Fatal(err)
	}
	next := r.fake.Last()
	next.HoldOnClose()
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	if view := r.view(conv.ID); view.State != StateStopping {
		t.Fatalf("after release: %q", view.State)
	}
	next.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: conv.NativeID, Model: "m"}})
	if view := r.view(conv.ID); view.State != StateStopping {
		t.Errorf("a late started report showed %q while stopping", view.State)
	}
	next.Exit(nil)
	waitFor(t, "the release to settle", func() bool { return r.view(conv.ID).State == model.RuntimeNone })
}

// a runtime that reports another session while still launching is recorded
// and stopped once Launch returns, never shown running, and the start fails;
// a prompted launch sends no prompt to it either.
func TestWrongSessionReportedDuringLaunchIsStopped(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	finishTurn(rt, "a-1", "done")
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the runtime to exit", rt.Exited)

	r.fake.DuringLaunch(func(next *harnesstest.Runtime) {
		next.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: "another"}})
		// a later report naming no session, as an idle claude launch's ready.
		next.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Model: "m"}})
	})
	for _, attempt := range []func() error{
		func() error { return r.svc.Start(conv.ID) },
		func() error { return r.svc.Prompt(conv.ID, "two") },
	} {
		err := attempt()
		wantCode(t, err, CodeConflict)
		next := r.fake.Last()
		if view := r.view(conv.ID); view.State != StateStopping {
			t.Errorf("after the refused launch: %q", view.State)
		}
		waitFor(t, "the refused runtime to be closed", next.Closed)
		if len(next.Prompts()) != 0 {
			t.Errorf("the refused runtime got %v", next.Prompts())
		}
		var refused bool
		for _, ev := range r.events(conv.ID) {
			refused = refused || (ev.Status == model.StatusError && strings.Contains(ev.Text, "'another'"))
		}
		if !refused {
			t.Error("the wrong session was not recorded")
		}
		next.Exit(nil)
		waitFor(t, "the stop to settle", func() bool { return r.view(conv.ID).State == model.RuntimeNone })
	}
}

// an idle start whose runtime exits before it reports (the adapter stopping
// one claude never answered) records the error through a transition, leaves
// the conversation with no runtime, and can be started again.
func TestAnUnansweredStartEndsWithNoRuntime(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	finishTurn(rt, "a-1", "done")
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the runtime to exit", rt.Exited)

	if err := r.svc.Start(conv.ID); err != nil {
		t.Fatal(err)
	}
	stalled := r.fake.Last()
	if view := r.view(conv.ID); view.State != StateStarting {
		t.Fatalf("before the timeout: %q", view.State)
	}
	stalled.Exit(errors.New("signal: killed: claude did not answer within 1m0s of a start with no prompt, so the runtime was stopped"))
	view := r.view(conv.ID)
	if view.State != model.RuntimeNone || view.Runtime != model.RuntimeNone {
		t.Fatalf("after the timeout: state %q, runtime %q", view.State, view.Runtime)
	}
	evs := r.events(conv.ID)
	last := evs[len(evs)-1]
	if last.Kind != model.EventRuntime || last.Status != model.StatusError || !strings.Contains(last.Text, "did not answer within") {
		t.Errorf("recorded = %s %s %q", last.Kind, last.Status, last.Text)
	}
	if err := r.svc.Start(conv.ID); err != nil {
		t.Errorf("starting again: %v", err)
	}
}

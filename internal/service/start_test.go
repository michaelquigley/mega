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

// a start after a terminal visit resumes the exact session
// with no prompt, recorded starting then started, and leaves the conversation
// idle with a runtime the next prompt goes to.
func TestStartResumesTheExactSessionIdleWithoutReadingHistory(t *testing.T) {
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
	before := len(r.events(conv.ID))
	if err := r.svc.Start(conv.ID); err != nil {
		t.Fatal(err)
	}
	next := r.fake.Last()
	if next == rt || len(r.fake.Reads()) != 0 {
		t.Fatalf("new runtime: %v; history reads: %v", next != rt, r.fake.Reads())
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
	next.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: conv.NativeID, Model: "m", Version: "v", PermissionMode: "default"}})
	tail := r.events(conv.ID)[before:]
	if got := kinds(tail); !slices.Equal(got, []string{"runtime:starting", "runtime:started"}) {
		t.Errorf("events = %v", got)
	}
	view := r.view(conv.ID)
	if view.State != StateRunning || view.TurnActive || view.Attention != model.AttentionIdle || view.Facts == nil || view.Facts.PermissionMode != "default" {
		t.Errorf("after start: state %q, turn %v, attention %q, facts %+v", view.State, view.TurnActive, view.Attention, view.Facts)
	}

	// claude's own report of another mode moves the live mode.
	next.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusModeChanged, Facts: &model.RuntimeFacts{PermissionMode: "plan"}})
	if f := r.view(conv.ID).Facts; f.PermissionMode != "plan" {
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

// a start is refused for an unsupervised conversation, which Resume settles
// first; it launches nothing.
func TestStartIsRefusedForAnUnsupervisedConversation(t *testing.T) {
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
	if len(r.fake.Runtimes()) != launched {
		t.Error("a refused start launched a runtime")
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
		// held open on close, so it stays stopping until the test exits it;
		// otherwise the fake's close settles it to none before the test can
		// look, as it did under load.
		next.HoldOnClose()
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

// an answer to a request raised while its runtime is still launching is
// refused, and the request stays pending until the launch has returned and
// the answer can reach the harness.
func TestAnAnswerDuringALaunchIsRefusedAndTheRequestStaysPending(t *testing.T) {
	r := newRig(t)
	var id string
	r.fake.DuringLaunch(func(rt *harnesstest.Runtime) {
		id = rt.Ask(model.RequestPermission, map[string]any{"tool": "Bash"})
		conv := r.svc.reg.Snapshot().Conversations[0]
		wantCode(t, r.svc.Answer(conv.ID, id, harness.Answer{Decision: harness.DecisionDeny}), CodeConflict)
		if pending := r.view(conv.ID).Pending; len(pending) != 1 || pending[0].ID != id {
			t.Errorf("during the launch, pending = %+v", pending)
		}
		if len(rt.Deliveries()) != 0 {
			t.Errorf("the refused answer reached the runtime: %+v", rt.Deliveries())
		}
	})
	conv, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "asks while launching", "opening prompt", LaunchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	rt := r.fake.Last()
	if pending := r.view(conv.ID).Pending; len(pending) != 1 || pending[0].ID != id {
		t.Fatalf("after the launch, pending = %+v", pending)
	}
	if err := r.svc.Answer(conv.ID, id, harness.Answer{Decision: harness.DecisionDeny}); err != nil {
		t.Fatal(err)
	}
	if got := rt.Deliveries(); len(got) != 1 || got[0].Answer.Decision != harness.DecisionDeny {
		t.Errorf("deliveries = %+v", got)
	}
	if pending := r.view(conv.ID).Pending; len(pending) != 0 {
		t.Errorf("the answered request is still pending: %+v", pending)
	}
}

func TestSendRefusesAStoppingOrMismatchedRuntimeBeforeRecording(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		t.Run(fmt.Sprintf("wrong-session=%t", mismatch), func(t *testing.T) {
			r := adoptionRig(t)
			conv, err := r.adoptTerminal()
			if err != nil {
				t.Fatal(err)
			}
			if err := r.svc.Start(conv.ID); err != nil {
				t.Fatal(err)
			}
			rt := r.fake.Last()
			started(rt)
			rt.HoldOnClose()
			defer rt.Exit(nil)
			if mismatch {
				rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: "another"}})
			} else if err := r.svc.Release(conv.ID); err != nil {
				t.Fatal(err)
			}
			waitFor(t, "the runtime to begin stopping", rt.Closed)
			before := r.view(conv.ID).LastSeq
			// This is the interval after the caller's launch or prompt check
			// and before it records and hands the prompt to the runtime.
			wantCode(t, r.svc.send(conv.ID, "opening prompt"), CodeConflict)
			if after := r.view(conv.ID); after.LastSeq != before || after.TurnActive || len(rt.Prompts()) != 0 {
				t.Errorf("refused send changed the log or started a turn: seq %d → %d, active %t, prompts %d",
					before, after.LastSeq, after.TurnActive, len(rt.Prompts()))
			}
		})
	}
}

type promptHookRuntime struct {
	harness.Runtime
	beforePrompt func()
}

func (rt promptHookRuntime) Prompt(uuid, text string) error {
	rt.beforePrompt()
	return rt.Runtime.Prompt(uuid, text)
}

func TestSendSettlesAPromptRefusedAfterItWasRecorded(t *testing.T) {
	r := adoptionRig(t)
	conv, err := r.adoptTerminal()
	if err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Start(conv.ID); err != nil {
		t.Fatal(err)
	}
	rt := r.fake.Last()
	started(rt)
	r.svc.mu.Lock()
	r.svc.live[conv.ID].runtime = promptHookRuntime{Runtime: rt, beforePrompt: func() {
		events := r.events(conv.ID)
		if last := events[len(events)-1]; last.Kind != model.EventUser || last.Text != "opening prompt" || !r.view(conv.ID).TurnActive {
			t.Errorf("the prompt had not been recorded when the runtime refused it: %+v", last)
		}
		rt.RefusePrompts(fmt.Errorf("%w: %w", harness.ErrPromptRefused, harness.ErrWrongSession))
	}}
	r.svc.mu.Unlock()

	before := len(r.events(conv.ID))
	if err := r.svc.Prompt(conv.ID, "opening prompt"); !errors.Is(err, harness.ErrPromptRefused) {
		t.Errorf("send returned %v", err)
	}
	if got := kinds(r.events(conv.ID)[before:]); !slices.Equal(got, []string{"user", "turn_end:error"}) {
		t.Errorf("prompt refusal rows = %v", got)
	}
	if r.view(conv.ID).TurnActive || len(rt.Prompts()) != 0 {
		t.Error("the refusal left an active turn or delivered a prompt")
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

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

func TestFirstLaunchRejectsAnEarlyReportOfAnotherSession(t *testing.T) {
	for _, tc := range []struct {
		name       string
		earlyExit  bool
		exitErr    error
		exitStatus string
		exitText   string
	}{
		{name: "exit after Launch returns", exitStatus: model.StatusStopped},
		{name: "exit before Launch returns", earlyExit: true, exitStatus: model.StatusStopped},
		{
			name: "early exit with surviving processes", earlyExit: true,
			exitErr: fmt.Errorf("%w: child 42", harness.ErrLeftRunning), exitStatus: model.StatusError, exitText: "child 42",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			const disputed = "another-session"
			const reply = "reply from the disputed session"
			sub := r.svc.Subscribe()
			defer sub.Close()
			r.fake.DuringLaunch(func(rt *harnesstest.Runtime) {
				rt.HoldOnClose()
				startedWith := &model.Event{
					Kind: model.EventRuntime, Status: model.StatusStarted,
					Facts: &model.RuntimeFacts{Session: disputed},
				}
				rt.Emit(startedWith)
				rt.Emit(&model.Event{Kind: model.EventAssistant, Text: reply, NativeID: "disputed-reply"})
				rt.Delta(reply)
				if id := rt.Ask(model.RequestQuestion, nil); id == "" {
					t.Error("the request callback returned no id")
				}
				// Claude can then report ready without naming a session.
				rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted})
				r.svc.mu.Lock()
				early := r.svc.viewLocked(r.svc.reg.Snapshot().Conversations[0])
				r.svc.mu.Unlock()
				if early.NativeID != "" || early.State != StateStarting || len(early.Pending) != 0 {
					t.Errorf("before Launch returned, view exposed id %q in state %q with %d requests", early.NativeID, early.State, len(early.Pending))
				}
				if tc.earlyExit {
					rt.Exit(tc.exitErr)
				}
			})
			conv, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "mismatched launch", "opening prompt", LaunchChoice{})
			rt := r.fake.Last()
			if ErrorCode(err) != CodeConflict {
				rt.Exit(nil)
				t.Fatalf("first launch returned %v with binding %q and %d prompts", err, conv.NativeID, len(rt.Prompts()))
			}
			defer rt.Exit(nil)
			if !tc.earlyExit {
				waitFor(t, "the mismatched runtime to be closed", rt.Closed)
				rt.Exit(tc.exitErr)
			}
			if !rt.Exited() {
				t.Fatal("the exit did not settle")
			}
			if len(rt.Prompts()) != 0 {
				t.Errorf("the mismatched runtime received %d prompts", len(rt.Prompts()))
			}

			for range len(sub.Updates()) {
				update := <-sub.Updates()
				if update.Delta == reply || (update.Event != nil && (update.Event.Text == reply || update.Event.Kind == model.EventRequest)) {
					t.Error("the disputed runtime's callback reached the live stream")
				}
			}

			for _, phase := range []string{"after stop", "after restart"} {
				if phase == "after restart" {
					r.restart()
				}
				if view := r.view(conv.ID); view.NativeID != "" {
					t.Errorf("%s: visible binding = %q", phase, view.NativeID)
				}
				if got := r.svc.reg.Snapshot().Conversations[0].NativeID; got != "" {
					t.Errorf("%s: registry binding = %q", phase, got)
				}
				events := r.events(conv.ID)
				refusals := 0
				for i, ev := range events {
					if ev.Kind == model.EventUser || ev.Kind == model.EventRequest || ev.Text == reply || ev.Status == model.StatusStarted {
						t.Errorf("%s: unexpected event %s %s", phase, ev.Kind, ev.Status)
					}
					if ev.Kind == model.EventRuntime && ev.Status == model.StatusError &&
						strings.Contains(ev.Text, disputed) && strings.Contains(ev.Text, rt.NativeID()) {
						refusals++
						if i == len(events)-1 {
							t.Errorf("%s: refusal replaced the exit record", phase)
						}
					}
				}
				if refusals != 1 {
					t.Errorf("%s: %d runtime errors explain the refused binding, want one", phase, refusals)
				}
				if last := events[len(events)-1]; last.Kind != model.EventRuntime || last.Status != tc.exitStatus || !strings.Contains(last.Text, tc.exitText) {
					t.Errorf("%s: exit record is %s %s %q, want %s containing %q", phase, last.Kind, last.Status, last.Text, tc.exitStatus, tc.exitText)
				}
				_, err := r.svc.NativeTranscript(conv.ID)
				wantCode(t, err, CodeConflict)
				wantCode(t, r.svc.Start(conv.ID), CodeConflict)
				wantCode(t, r.svc.Prompt(conv.ID, "retry"), CodeConflict)
				if reads, launches := r.fake.Reads(), len(r.fake.Runtimes()); len(reads) != 0 || launches != 1 {
					t.Fatalf("%s: transcript reads = %v, launches = %d", phase, reads, launches)
				}
			}
		})
	}
}

func TestFirstLaunchDeliversMatchingCallbacksInOrder(t *testing.T) {
	for _, earlyExit := range []bool{false, true} {
		t.Run(fmt.Sprintf("early-exit=%t", earlyExit), func(t *testing.T) {
			r := newRig(t)
			sub := r.svc.Subscribe()
			defer sub.Close()
			const reply = "early reply from the matching session"
			var requestID string
			r.fake.DuringLaunch(func(rt *harnesstest.Runtime) {
				started(rt)
				rt.Emit(&model.Event{Kind: model.EventAssistant, Text: reply, NativeID: "matched-reply"})
				requestID = rt.Ask(model.RequestQuestion, nil)
				if requestID == "" {
					t.Error("the request callback returned no id")
				}
				r.svc.mu.Lock()
				early := r.svc.viewLocked(r.svc.reg.Snapshot().Conversations[0])
				r.svc.mu.Unlock()
				if early.TurnActive || len(early.Pending) != 0 {
					t.Error("unconfirmed output or request reached the view")
				}
				rt.Settle(requestID, model.OutcomeAborted)
				if earlyExit {
					rt.Exit(nil)
				}
			})
			conv, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "matching launch", "opening prompt", LaunchChoice{})
			if earlyExit {
				if err == nil || r.svc.view(conv.ID).State != model.RuntimeNone || len(r.fake.Last().Prompts()) != 0 {
					t.Fatalf("early exit did not settle: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if len(r.svc.view(conv.ID).Pending) != 0 {
				t.Fatal("the settled early request is still pending")
			}
			want := []string{"runtime:starting", "runtime:started", "assistant", "request", "request_resolved:aborted"}
			if earlyExit {
				want = append(want, "runtime:error")
			} else {
				want = append(want, "user")
			}
			var streamed []*model.Event
			for range len(sub.Updates()) {
				if update := <-sub.Updates(); update.Event != nil {
					streamed = append(streamed, update.Event)
				}
			}
			for _, events := range [][]*model.Event{r.events(conv.ID), streamed} {
				if got := kinds(events); !slices.Equal(got, want) {
					t.Fatalf("events = %v, want %v", got, want)
				}
				if events[2].Text != reply || events[3].Req.ID != requestID || events[4].Req.ID != requestID {
					t.Fatal("the reply or request identity changed in delivery")
				}
			}
		})
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

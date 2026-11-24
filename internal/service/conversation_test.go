package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/harness/harnesstest"
	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/registry"
)

// rig is a service over a fake claude harness with one registered project.
type rig struct {
	t       *testing.T
	path    string
	fake    *harnesstest.Harness
	svc     *Service
	project model.Project
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{t: t, path: filepath.Join(t.TempDir(), "registry.yaml"), fake: harnesstest.New(model.HarnessClaude)}
	r.svc = newService(t, r.path, allTrusted, r.fake)
	project, err := r.svc.RegisterProject("fixture", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r.project = project
	// the project's background resolution settles before a test begins.
	r.svc.background.Wait()
	return r
}

// restart closes the service as a crash would (no release) and starts another
// over the same registry and logs.
func (r *rig) restart() {
	r.t.Helper()
	r.svc.Close()
	r.svc = newService(r.t, r.path, allTrusted, r.fake)
}

// crashed starts another service over the same registry and logs after the
// last one ended at a failed write (exits), which is never closed.
func (r *rig) crashed() {
	r.t.Helper()
	r.svc = newService(r.t, r.path, allTrusted, r.fake)
}

func (r *rig) start(prompt string) ConversationView {
	r.t.Helper()
	conv, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "", prompt, LaunchChoice{})
	if err != nil {
		r.t.Fatal(err)
	}
	return conv
}

func (r *rig) view(id string) ConversationView {
	r.t.Helper()
	view, err := r.svc.view(id)
	if err != nil {
		r.t.Fatal(err)
	}
	return view
}

func (r *rig) events(id string) []*model.Event {
	r.t.Helper()
	evs, err := r.svc.Events(id, 0)
	if err != nil {
		r.t.Fatal(err)
	}
	return evs
}

func kinds(evs []*model.Event) []string {
	var out []string
	for _, ev := range evs {
		k := ev.Kind
		if ev.Status != "" {
			k += ":" + ev.Status
		}
		if ev.Req != nil && ev.Req.Outcome != "" {
			k += ":" + ev.Req.Outcome
		}
		out = append(out, k)
	}
	return out
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// looked asserts that the request is pending and is the one the test means
// to answer, its payload holding want: no test answers a request it has not
// looked at.
func (r *rig) looked(conversationID, id, want string) {
	r.t.Helper()
	for _, req := range r.view(conversationID).Pending {
		if req.ID != id {
			continue
		}
		payload, err := json.Marshal(req.Payload)
		if err != nil || !strings.Contains(string(payload), want) {
			r.t.Fatalf("request %s is not the one meant: %s", id, payload)
		}
		return
	}
	r.t.Fatalf("request %s is not pending", id)
}

func started(rt *harnesstest.Runtime) {
	rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: rt.NativeID(), Model: "m", Version: "v"}})
}

func finishTurn(rt *harnesstest.Runtime, nativeID, text string) {
	rt.Emit(&model.Event{Kind: model.EventAssistant, Text: text, NativeID: nativeID})
	rt.Emit(&model.Event{Kind: model.EventTurnEnd, Status: model.TurnSuccess})
}

func TestStartLaunchesNewSessionAndSendsPromptWithItsIdentity(t *testing.T) {
	r := newRig(t)
	conv := r.start("Reply with ALPHA\nand more")
	rt := r.fake.Last()
	if rt.Spec.NativeID != "" || rt.Spec.Cwd != r.project.Checkout {
		t.Errorf("spec = %+v", rt.Spec)
	}
	if conv.Name != "Reply with ALPHA" || conv.NativeID != rt.NativeID() || conv.Runtime != model.RuntimeRunning || !conv.TurnActive {
		t.Errorf("conversation = %+v", conv)
	}
	prompts := rt.Prompts()
	evs := r.events(conv.ID)
	if got := kinds(evs); !slices.Equal(got, []string{"runtime:starting", "user"}) {
		t.Fatalf("events = %v", got)
	}
	if len(prompts) != 1 || evs[1].NativeID != prompts[0].UUID || prompts[0].UUID == "" {
		t.Fatalf("prompts = %+v, events = %+v", prompts, evs)
	}
	if conv.State != StateStarting {
		t.Errorf("before the runtime reports its session the state is %q", conv.State)
	}

	started(rt)
	finishTurn(rt, "a-1", "ALPHA")
	view := r.view(conv.ID)
	if view.TurnActive || view.Facts == nil || view.Facts.Model != "m" || view.State != StateRunning || view.LastSeq != 5 {
		t.Errorf("view after turn = %+v", view)
	}
}

func TestPromptWhileTurnActiveIsRefused(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	wantCode(t, r.svc.Prompt(conv.ID, "two"), CodeConflict)
	finishTurn(r.fake.Last(), "a-1", "done")
	if err := r.svc.Prompt(conv.ID, "two"); err != nil {
		t.Fatal(err)
	}
	if got := len(r.fake.Last().Prompts()); got != 2 {
		t.Errorf("prompts = %d", got)
	}
}

func TestSessionMismatchStopsTheRuntime(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: "someone-else"}})
	waitFor(t, "the mismatched runtime to stop", rt.Exited)
	evs := r.events(conv.ID)
	if !slices.Contains(kinds(evs), "runtime:error") || slices.Contains(kinds(evs), "runtime:started") {
		t.Errorf("events = %v", kinds(evs))
	}
	if view := r.view(conv.ID); view.NativeID != rt.NativeID() {
		t.Errorf("the bound identity changed to %q", view.NativeID)
	}
}

func TestAnswerIsDeliveredOnceAndThenNotPending(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	id := rt.Ask(model.RequestPermission, map[string]any{"tool": "Bash"})

	view := r.view(conv.ID)
	if len(view.Pending) != 1 || view.Pending[0].ID != id {
		t.Fatalf("pending = %+v", view.Pending)
	}
	wantCode(t, r.svc.Answer(conv.ID, id, harness.Answer{Decision: "maybe"}), CodeInvalid)
	if err := r.svc.Answer(conv.ID, id, harness.Answer{Decision: harness.DecisionDeny}); err != nil {
		t.Fatal(err)
	}
	wantCode(t, r.svc.Answer(conv.ID, id, harness.Answer{Decision: harness.DecisionDeny}), CodeNotPending)

	deliveries := rt.Deliveries()
	if len(deliveries) != 1 || deliveries[0].Answer.Decision != harness.DecisionDeny || deliveries[0].Answer.Message == "" {
		t.Errorf("deliveries = %+v", deliveries)
	}
	if got := kinds(r.events(conv.ID)); !slices.Equal(got[2:], []string{"request", "request_resolved:answered"}) {
		t.Errorf("events = %v", got)
	}
	if len(r.view(conv.ID).Pending) != 0 {
		t.Error("an answered request is still pending")
	}
}

func TestInterruptAbortsRequestAndLateAnswerIsRejected(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	id := rt.Ask(model.RequestQuestion, map[string]any{"questions": []any{}})
	if err := r.svc.Interrupt(conv.ID); err != nil {
		t.Fatal(err)
	}
	wantCode(t, r.svc.Answer(conv.ID, id, harness.Answer{Decision: harness.DecisionDismiss}), CodeNotPending)
	if len(rt.Deliveries()) != 0 {
		t.Error("a late answer reached the runtime")
	}
	if got := kinds(r.events(conv.ID)); got[len(got)-1] != "request_resolved:aborted" {
		t.Errorf("events = %v", got)
	}
}

func TestAnswerSettledByHarnessFirstIsNotPending(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	id := rt.Ask(model.RequestPermission, nil)
	// the harness has aborted the request but its notice has not reached the
	// service yet: the runtime refuses the answer, and the service says so.
	rt.Settle(id, model.OutcomeAborted)
	wantCode(t, r.svc.Answer(conv.ID, id, harness.Answer{Decision: harness.DecisionDeny}), CodeNotPending)
}

func TestReleaseInterruptsOnlyWhenActiveAndReportsStoppingUntilExit(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	finishTurn(rt, "a-1", "done")
	rt.HoldOnClose()
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "input to close", rt.Closed)
	if rt.Interrupts() != 0 {
		t.Error("an idle runtime was interrupted on release")
	}
	if view := r.view(conv.ID); view.State != StateStopping || view.Runtime != model.RuntimeRunning {
		t.Errorf("before exit: state %q, recorded %q", view.State, view.Runtime)
	}
	wantCode(t, r.svc.Prompt(conv.ID, "two"), CodeConflict)

	rt.Exit(nil)
	view := r.view(conv.ID)
	if view.State != model.RuntimeNone || view.Runtime != model.RuntimeNone {
		t.Errorf("after exit: state %q, recorded %q", view.State, view.Runtime)
	}
	got := kinds(r.events(conv.ID))
	if !slices.Equal(got[len(got)-2:], []string{"runtime:stopping", "runtime:stopped"}) {
		t.Errorf("events = %v", got)
	}
}

func TestReleaseWithRequestPendingInterruptsFirst(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	rt.Ask(model.RequestPermission, nil)
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the runtime to exit", rt.Exited)
	if rt.Interrupts() != 1 {
		t.Errorf("interrupts = %d", rt.Interrupts())
	}
	if got := kinds(r.events(conv.ID)); !slices.Contains(got, "request_resolved:aborted") {
		t.Errorf("events = %v", got)
	}
}

func TestStaleRequestFromEarlierRuntimeNeverMatchesNewOne(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	first := r.fake.Last()
	old := first.Ask(model.RequestPermission, nil)
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the first runtime to exit", first.Exited)

	if err := r.svc.Prompt(conv.ID, "two"); err != nil {
		t.Fatal(err)
	}
	second := r.fake.Last()
	if second == first || second.Spec.NativeID != conv.NativeID {
		t.Fatalf("second runtime spec = %+v", second.Spec)
	}
	fresh := second.Ask(model.RequestPermission, nil) // native number 0 again
	if fresh == old {
		t.Fatal("request ids repeated across runtimes")
	}
	wantCode(t, r.svc.Answer(conv.ID, old, harness.Answer{Decision: harness.DecisionDeny}), CodeNotPending)
	if len(second.Deliveries()) != 0 || len(r.view(conv.ID).Pending) != 1 {
		t.Fatal("the stale answer touched the new runtime's request")
	}
	// a denial proves the delivery as well as an allow would, and allows
	// nothing the test never looked at.
	if err := r.svc.Answer(conv.ID, fresh, harness.Answer{Decision: harness.DecisionDeny}); err != nil {
		t.Fatal(err)
	}
	if d := second.Deliveries(); len(d) != 1 || d[0].Native != "0" {
		t.Errorf("deliveries = %+v", d)
	}
}

func TestFailureStatusAfterIdleReleaseIsStillAStop(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	rt.Emit(&model.Event{Kind: model.EventTurnEnd, Status: model.TurnInterrupted})
	rt.HoldOnClose()
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "input to close", rt.Closed)
	rt.Exit(errors.New("exit status 1"))
	evs := r.events(conv.ID)
	last := evs[len(evs)-1]
	if last.Status != model.StatusStopped || !strings.Contains(last.Text, "exit status 1") {
		t.Errorf("last event = %+v", last)
	}

	// an exit nobody asked for is an error.
	if err := r.svc.Prompt(conv.ID, "two"); err != nil {
		t.Fatal(err)
	}
	second := r.fake.Last()
	second.Emit(&model.Event{Kind: model.EventTurnEnd, Status: model.TurnSuccess})
	second.Exit(errors.New("exit status 1"))
	evs = r.events(conv.ID)
	if last := evs[len(evs)-1]; last.Status != model.StatusError {
		t.Errorf("unrequested exit = %+v", last)
	}
}

func TestExitDuringTurnIsAnErrorAndAbortsPending(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	rt.Ask(model.RequestPermission, nil)
	rt.Exit(nil)
	got := kinds(r.events(conv.ID))
	if !slices.Equal(got[len(got)-2:], []string{"request_resolved:aborted", "runtime:error"}) {
		t.Errorf("events = %v", got)
	}
	if view := r.view(conv.ID); view.Runtime != model.RuntimeNone || len(view.Pending) != 0 || view.TurnActive {
		t.Errorf("view = %+v", view)
	}
}

// a first launch that fails, before the harness starts or in it, records one
// error and leaves a conversation that is never launched again: a prompt and
// a start are refused, across a restart too, and it can be removed.
func TestAFailedFirstLaunchIsNeverLaunchedAgain(t *testing.T) {
	for name, fail := range map[string]func(r *rig) (undo func()){
		"in the harness": func(r *rig) func() {
			r.fake.FailLaunch(errors.New("the harness did not confirm the launch"))
			return func() { r.fake.FailLaunch(nil) }
		},
		"before the harness starts": func(r *rig) func() {
			writeEnvFile(t, r.project.Checkout, "export MEGA_MARKER="+markerOne)
			r.approve(r.project.ID)
			r.fake.OnVersion(func() { writeEnvFile(t, r.project.Checkout, "export MEGA_MARKER="+markerTwo) })
			return func() { r.fake.OnVersion(nil); r.approve(r.project.ID) }
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			undo := fail(r)
			conv, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "failed", "one", LaunchChoice{})
			if err == nil {
				t.Fatal("expected the launch to fail")
			}
			undo()
			if got := kinds(r.events(conv.ID)); !slices.Equal(got, []string{"runtime:starting", "runtime:error"}) {
				t.Errorf("events = %v", got)
			}
			if view := r.view(conv.ID); view.NativeID != "" || view.State != model.RuntimeNone || view.Runtime != model.RuntimeNone {
				t.Errorf("view = %+v", view)
			}
			launches := len(r.fake.Runtimes())
			refused := func() {
				t.Helper()
				if err := r.svc.Prompt(conv.ID, "again"); !errors.Is(err, errNoSession) {
					t.Errorf("prompt: %v", err)
				}
				if err := r.svc.Start(conv.ID); !errors.Is(err, errNoSession) {
					t.Errorf("start: %v", err)
				}
				if len(r.fake.Runtimes()) != launches {
					t.Error("a conversation with no session was launched again")
				}
			}
			refused()
			r.restart()
			if err := r.svc.Resume(conv.ID); err != nil {
				t.Fatal(err)
			}
			refused()
			if err := r.svc.RemoveConversation(conv.ID); err != nil {
				t.Errorf("removing it: %v", err)
			}
		})
	}
}

// a conversation with a session whose launch fails records an error and is
// launched again, on the same session.
func TestAFailedLaunchOfASessionCanBeLaunchedAgain(t *testing.T) {
	r := newRig(t)
	conv := r.stoppedWithSession()
	r.fake.FailLaunch(errors.New("no node"))
	if err := r.svc.Prompt(conv.ID, "two"); err == nil {
		t.Fatal("expected the launch to fail")
	}
	evs := r.events(conv.ID)
	if last := evs[len(evs)-1]; last.Status != model.StatusError {
		t.Errorf("the failed launch was recorded as %q", last.Status)
	}
	r.fake.FailLaunch(nil)
	if err := r.svc.Prompt(conv.ID, "two"); err != nil {
		t.Fatal(err)
	}
	if spec := r.fake.Last().Spec; spec.NativeID != conv.NativeID {
		t.Errorf("launched %q, not the conversation's session %q", spec.NativeID, conv.NativeID)
	}
}

func TestUnknownHarnessIsInvalid(t *testing.T) {
	r := newRig(t)
	_, err := r.svc.StartConversation(r.project.ID, model.HarnessCodex, "", "one", LaunchChoice{})
	wantCode(t, err, CodeInvalid)
}

func TestRestartLosesPendingAndLeavesRunningUnsupervised(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	finishTurn(rt, "a-1", "done")
	if err := r.svc.Prompt(conv.ID, "two"); err != nil {
		t.Fatal(err)
	}
	rt.Ask(model.RequestPermission, nil)

	r.restart()
	view := r.view(conv.ID)
	if view.Runtime != model.RuntimeUnsupervised || view.State != model.RuntimeUnsupervised || len(view.Pending) != 0 {
		t.Fatalf("view after restart = %+v", view)
	}
	got := kinds(r.events(conv.ID))
	if !slices.Equal(got[len(got)-2:], []string{"request_resolved:lost", "runtime:unsupervised"}) {
		t.Errorf("events = %v", got)
	}
	if len(r.fake.Reads()) != 0 {
		t.Error("history was imported at start, while an orphaned runtime may still be writing")
	}
	wantCode(t, r.svc.Prompt(conv.ID, "three"), CodeConflict)

	// a second restart changes nothing more.
	before := len(r.events(conv.ID))
	r.restart()
	if after := len(r.events(conv.ID)); after != before {
		t.Errorf("a second restart recorded %d more events", after-before)
	}

	launches := len(r.fake.Runtimes())
	if err := r.svc.Resume(conv.ID); err != nil {
		t.Fatal(err)
	}
	if reads := r.fake.Reads(); len(reads) != 0 {
		t.Errorf("history reads = %v", reads)
	}
	if len(r.fake.Runtimes()) != launches {
		t.Error("resume started a runtime")
	}
	if view := r.view(conv.ID); view.Runtime != model.RuntimeNone {
		t.Errorf("recorded runtime after resume = %q", view.Runtime)
	}
	if err := r.svc.Prompt(conv.ID, "three"); err != nil {
		t.Fatal(err)
	}
	if spec := r.fake.Last().Spec; spec.NativeID != conv.NativeID {
		t.Errorf("resumed %q, bound to %q", spec.NativeID, conv.NativeID)
	}
}

func TestCleanShutdownLeavesNone(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	finishTurn(r.fake.Last(), "a-1", "done")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r.svc.Shutdown(ctx)
	r.restart()
	view := r.view(conv.ID)
	if view.Runtime != model.RuntimeNone || slices.ContainsFunc(r.events(conv.ID), (*model.Event).HistoryBoundary) {
		t.Errorf("view = %+v, events = %v", view, kinds(r.events(conv.ID)))
	}
}

func TestShutdownDeadlineLeavesUnfinishedRuntimeRunning(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	r.fake.Last().HoldOnClose()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	r.svc.Shutdown(ctx)
	r.restart()
	if view := r.view(conv.ID); view.Runtime != model.RuntimeUnsupervised {
		t.Errorf("recorded runtime = %q", view.Runtime)
	}
}

// a release stops the runtime and means nothing more: the log holds the stop
// and the next start as runtime lines, none of them a boundary, nothing is
// read from the session, and the turn before the release and the turn after
// it page as two turns.
func TestAReleaseAndAPromptLeaveTwoTurnsAndNoBoundary(t *testing.T) {
	r := newRig(t)
	conv := r.start("managed prompt")
	rt := r.fake.Last()
	started(rt)
	finishTurn(rt, "a-1", "same words")
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the runtime to exit", rt.Exited)
	if err := r.svc.Prompt(conv.ID, "next managed prompt"); err != nil {
		t.Fatal(err)
	}
	next := r.fake.Last()
	if next.Spec.NativeID != conv.NativeID {
		t.Fatalf("launched %q instead of %q", next.Spec.NativeID, conv.NativeID)
	}
	started(next)
	finishTurn(next, "a-2", "next managed reply")
	if reads := r.fake.Reads(); len(reads) != 0 {
		t.Fatalf("history reads = %v", reads)
	}
	evs := r.events(conv.ID)
	if slices.ContainsFunc(evs, (*model.Event).HistoryBoundary) {
		t.Errorf("a release left a boundary: %v", kinds(evs))
	}
	var statuses []string
	for _, ev := range evs {
		if ev.Kind == model.EventRuntime {
			statuses = append(statuses, ev.Status)
		}
	}
	want := []string{model.StatusStarting, model.StatusStarted, model.StatusStopping, model.StatusStopped, model.StatusStarting, model.StatusStarted}
	if !slices.Equal(statuses, want) {
		t.Errorf("runtime lines = %v, want %v", statuses, want)
	}
	latest, err := r.svc.History(conv.ID, HistoryQuery{Turns: 1})
	if err != nil {
		t.Fatal(err)
	}
	earlier, err := r.svc.History(conv.ID, HistoryQuery{Turns: 1, Before: latest.From})
	if err != nil {
		t.Fatal(err)
	}
	for i, page := range []History{earlier, latest} {
		var speech []string
		for _, ev := range page.Events {
			if ev.Kind == model.EventUser || ev.Kind == model.EventAssistant {
				speech = append(speech, ev.Text)
			}
		}
		want := [][]string{{"managed prompt", "same words"}, {"next managed prompt", "next managed reply"}}[i]
		if !slices.Equal(speech, want) {
			t.Errorf("page %d speech = %v, want %v", i, speech, want)
		}
	}
}

func TestDeltasAreStreamedNotStored(t *testing.T) {
	r := newRig(t)
	sub := r.svc.Subscribe()
	defer sub.Close()
	conv := r.start("one")
	r.fake.Last().Delta("ALP")
	before := len(r.events(conv.ID))
	var delta string
	for delta == "" {
		select {
		case update := <-sub.Updates():
			delta = update.Delta
		case <-time.After(time.Second):
			t.Fatal("no delta arrived")
		}
	}
	if delta != "ALP" || len(r.events(conv.ID)) != before {
		t.Errorf("delta %q, events %d -> %d", delta, before, len(r.events(conv.ID)))
	}
}

func TestDuplicateNativeIdentityIsRecordedOnce(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	rt.Emit(&model.Event{Kind: model.EventAssistant, Text: "x", NativeID: "a-1"})
	rt.Emit(&model.Event{Kind: model.EventAssistant, Text: "x", NativeID: "a-1"})
	if n := strings.Count(strings.Join(kinds(r.events(conv.ID)), ","), "assistant"); n != 1 {
		t.Errorf("assistant events = %d", n)
	}
}

// closeLog makes every later append fail, as a degraded event log does.
func (r *rig) closeLog(id string) {
	r.svc.mu.Lock()
	defer r.svc.mu.Unlock()
	r.svc.live[id].log.Close()
}

func TestDiscoverHidesAdoptedSessionsAndAdoptImportsWithoutRuntime(t *testing.T) {
	r := newRig(t)
	conv := r.start("managed") // binds native-1
	r.fake.SetSessions([]harness.Session{
		{NativeID: conv.NativeID, Preview: "managed"},
		{NativeID: "terminal-1", Preview: "Started in a terminal\nwith detail"},
	})
	r.fake.SetHistory("terminal-1", []*model.Event{
		{Kind: model.EventUser, Text: "Started in a terminal", NativeID: "t-1"},
		{Kind: model.EventAssistant, Text: "reply", NativeID: "t-2"},
	})
	sessions, err := r.svc.Discover(r.project.ID, model.HarnessClaude)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].NativeID != "terminal-1" {
		t.Fatalf("sessions = %+v", sessions)
	}

	launches := len(r.fake.Runtimes())
	adopted, err := r.svc.Adopt(r.project.ID, model.HarnessClaude, "terminal-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if adopted.NativeID != "terminal-1" || adopted.Name != "Started in a terminal" || adopted.Runtime != model.RuntimeNone || adopted.State != model.RuntimeNone {
		t.Errorf("adopted = %+v", adopted)
	}
	if len(r.fake.Runtimes()) != launches {
		t.Error("adoption started a runtime")
	}
	got := kinds(r.events(adopted.ID))
	if !slices.Equal(got, []string{"user", "assistant"}) {
		t.Errorf("events = %v", got)
	}

	// it is no longer offered, and cannot be adopted twice.
	if sessions, _ := r.svc.Discover(r.project.ID, model.HarnessClaude); len(sessions) != 0 {
		t.Errorf("still offered: %+v", sessions)
	}
	_, err = r.svc.Adopt(r.project.ID, model.HarnessClaude, "terminal-1", "again")
	wantCode(t, err, CodeInvalid)
	_, err = r.svc.Adopt(r.project.ID, model.HarnessClaude, "never-listed", "")
	wantCode(t, err, CodeInvalid)

	// the first prompt resumes exactly the adopted session.
	if err := r.svc.Prompt(adopted.ID, "go on"); err != nil {
		t.Fatal(err)
	}
	if spec := r.fake.Last().Spec; spec.NativeID != "terminal-1" {
		t.Errorf("resumed %q", spec.NativeID)
	}
}

func TestLeftRunningIsNeverACleanStop(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	finishTurn(rt, "a-1", "done")
	rt.HoldOnClose()
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "input to close", rt.Closed)
	rt.Exit(fmt.Errorf("%w: pids [4242]", harness.ErrLeftRunning))
	evs := r.events(conv.ID)
	last := evs[len(evs)-1]
	if last.Status != model.StatusError || !strings.Contains(last.Text, "4242") {
		t.Errorf("last event = %+v", last)
	}
}

// a release whose runtime could not list the processes it started is not a
// clean stop and not an error: it says what could not be checked, and asks
// for no attention.
func TestAReleaseThatCouldNotLookSaysSo(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	finishTurn(rt, "a-1", "done")
	rt.HoldOnClose()
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "input to close", rt.Closed)
	rt.Exit(errors.Join(nil, fmt.Errorf("%w: listing processes: no /proc", harness.ErrUnchecked)))
	evs := r.events(conv.ID)
	last := evs[len(evs)-1]
	if last.Status != model.StatusStopped || !strings.HasPrefix(last.Text, "released, but whether processes the runtime started are still running could not be checked") {
		t.Errorf("last event = %+v", last)
	}
}

func TestFirstRuntimeExitingDuringLaunchKeepsItsSession(t *testing.T) {
	r := newRig(t)
	r.fake.ExitDuringLaunch(true)
	conv, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "early", "one", LaunchChoice{})
	if err == nil {
		t.Fatal("expected the start to report the early exit")
	}
	first := r.fake.Last()
	if conv.NativeID != first.NativeID() || conv.Runtime != model.RuntimeNone {
		t.Fatalf("conversation = %+v, session %q", conv, first.NativeID())
	}
	r.fake.ExitDuringLaunch(false)
	if err := r.svc.Prompt(conv.ID, "again"); err != nil {
		t.Fatal(err)
	}
	if spec := r.fake.Last().Spec; spec.NativeID != first.NativeID() {
		t.Errorf("the next runtime launched %q, not the session the first one created (%q)", spec.NativeID, first.NativeID())
	}
}

func TestUserMessageFromTheRuntimeOpensATurn(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	finishTurn(rt, "a-1", "done")
	// a turn the runtime opened itself, which it first reports as the user
	// message it began with.
	rt.Emit(&model.Event{Kind: model.EventUser, Text: "Blue", NativeID: "u-2"})
	if !r.view(conv.ID).TurnActive {
		t.Error("the turn the answer started is not shown as active")
	}
	wantCode(t, r.svc.Prompt(conv.ID, "two"), CodeConflict)
}

// a first launch whose session cannot be saved ends the process, and the
// conversation never gets that session: the next start finds it unsupervised,
// it is refused after a resume, and the session is offered for adoption.
func TestAFirstLaunchWhoseSessionCannotBeSavedIsNeverLaunchedAgain(t *testing.T) {
	r := newRig(t)
	reasons := exits(r.svc)
	r.fake.DuringLaunch(func(rt *harnesstest.Runtime) {
		started(rt)
		r.svc.mu.Lock()
		r.svc.failUpdate = func() error { return errors.New("registry write failed") }
		r.svc.mu.Unlock()
	})
	go r.svc.StartConversation(r.project.ID, model.HarnessClaude, "unsaved", "one", LaunchChoice{})
	exited(t, reasons)
	rt := r.fake.Last()
	sentNothing(t, rt)
	r.fake.DuringLaunch(nil)
	r.crashed()
	ws := workspaceOf(t, r.svc)
	if len(ws.Conversations) != 1 || ws.Conversations[0].Runtime != model.RuntimeUnsupervised {
		t.Fatalf("after the next start: %+v", ws.Conversations)
	}
	id := ws.Conversations[0].ID
	if err := r.svc.Resume(id); err != nil {
		t.Fatal(err)
	}
	r.sessionOnlyTheLogHolds(id, rt.NativeID())
}

// a session the log shows started and the registry does not hold, as after a
// power loss between the launch and the write that binds it, is not resumed.
func TestASessionOnlyTheLogHoldsIsNotResumedAfterARestart(t *testing.T) {
	r := newRig(t)
	conv := r.stoppedWithSession()
	r.svc.mu.Lock()
	err := r.svc.reg.Update(func(d *registry.Document) error {
		for i := range d.Conversations {
			if d.Conversations[i].ID == conv.ID {
				d.Conversations[i].NativeID = ""
			}
		}
		return nil
	})
	r.svc.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	r.restart()
	r.sessionOnlyTheLogHolds(conv.ID, conv.NativeID)
}

// sessionOnlyTheLogHolds checks a conversation whose session is not in the
// registry: it shows none, reads none, is refused a prompt, and the session
// is discovered and adopted into a new conversation.
func (r *rig) sessionOnlyTheLogHolds(id, session string) {
	r.t.Helper()
	if view := r.view(id); view.NativeID != "" {
		r.t.Errorf("the view shows session %q", view.NativeID)
	}
	if len(r.fake.Reads()) != 0 {
		r.t.Errorf("adapter reads %v", r.fake.Reads())
	}
	launches := len(r.fake.Runtimes())
	if err := r.svc.Prompt(id, "two"); !errors.Is(err, errNoSession) {
		r.t.Errorf("prompt: %v", err)
	}
	if len(r.fake.Runtimes()) != launches {
		r.t.Error("the session was resumed")
	}
	r.fake.SetSessions([]harness.Session{{NativeID: session, Preview: "one"}})
	r.fake.SetHistory(session, []*model.Event{{Kind: model.EventUser, Text: "one", NativeID: "u-1"}})
	if sessions, err := r.svc.Discover(r.project.ID, model.HarnessClaude); err != nil || len(sessions) != 1 || sessions[0].NativeID != session {
		r.t.Fatalf("discovered %+v, %v", sessions, err)
	}
	adopted, err := r.svc.Adopt(r.project.ID, model.HarnessClaude, session, "")
	if err != nil || adopted.NativeID != session {
		r.t.Errorf("adopted %+v, %v", adopted, err)
	}
}

// a first launch whose binding write fails for real ends the process; after
// the next start and a resume, the conversation is refused a prompt and no
// fresh session takes the unkept one's place.
func TestUnkeptSessionIsNeverReplacedAfterRestart(t *testing.T) {
	r := newRig(t)
	dir := filepath.Dir(r.path)
	r.fake.OnLaunch(func() {
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Error(err)
		}
	})
	t.Cleanup(func() { os.Chmod(dir, 0o700) })
	reasons := exits(r.svc)
	go r.svc.StartConversation(r.project.ID, model.HarnessClaude, "unkept", "one", LaunchChoice{})
	exited(t, reasons)
	r.fake.OnLaunch(nil)
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	r.crashed()
	ws := workspaceOf(t, r.svc)
	if len(ws.Conversations) != 1 {
		t.Fatalf("conversations = %+v", ws.Conversations)
	}
	conv := ws.Conversations[0]
	if err := r.svc.Resume(conv.ID); err != nil {
		t.Fatal(err)
	}
	launches := len(r.fake.Runtimes())
	err := r.svc.Prompt(conv.ID, "two")
	wantCode(t, err, CodeConflict)
	if err == nil || !strings.Contains(err.Error(), "Adopt a session") {
		t.Errorf("refusal = %v", err)
	}
	if len(r.fake.Runtimes()) != launches {
		t.Error("a fresh session was started in place of the unkept one")
	}
}

func TestRemoveConversationRefusedWhileItHasARuntime(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	wantCode(t, r.svc.RemoveConversation(conv.ID), CodeConflict) // running

	rt.HoldOnClose()
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	if view := r.view(conv.ID); view.State != StateStopping {
		t.Fatalf("state = %q", view.State)
	}
	wantCode(t, r.svc.RemoveConversation(conv.ID), CodeConflict) // stopping

	rt.Exit(nil)
	waitFor(t, "the runtime to exit", rt.Exited)
	if err := r.svc.RemoveConversation(conv.ID); err != nil {
		t.Fatalf("removing a released conversation: %v", err)
	}
}

func TestRemoveConversationRefusedUntilSupervisionLossAcknowledged(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	started(r.fake.Last())
	r.restart()
	if view := r.view(conv.ID); view.Runtime != model.RuntimeUnsupervised {
		t.Fatalf("runtime = %q", view.Runtime)
	}
	wantCode(t, r.svc.RemoveConversation(conv.ID), CodeConflict)

	if err := r.svc.Resume(conv.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.RemoveConversation(conv.ID); err != nil {
		t.Fatalf("removing after the resume: %v", err)
	}
}

func TestRemoveConversationDropsItsLogAndFreesItsSession(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	finishTurn(rt, "a-1", "done")
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the runtime to exit", rt.Exited)
	kept := r.start("kept")
	r.fake.SetSessions([]harness.Session{{NativeID: conv.NativeID, Preview: "one"}})
	if sessions, _ := r.svc.Discover(r.project.ID, model.HarnessClaude); len(sessions) != 0 {
		t.Fatalf("a held session was offered: %+v", sessions)
	}
	logDir := filepath.Join(filepath.Dir(r.path), "conversations", conv.ID)
	if _, err := os.Stat(logDir); err != nil {
		t.Fatalf("no log before removal: %v", err)
	}

	sub := r.svc.Subscribe()
	defer sub.Close()
	if err := r.svc.RemoveConversation(conv.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case update := <-sub.Updates():
		if !update.WorkspaceChanged {
			t.Errorf("first update = %+v", update)
		}
	case <-time.After(time.Second):
		t.Error("the removal was not announced")
	}

	if _, err := os.Stat(logDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the log directory survived: %v", err)
	}
	if _, err := os.Stat(r.project.Checkout); err != nil {
		t.Errorf("the checkout was touched: %v", err)
	}
	ws := workspaceOf(t, r.svc)
	if len(ws.Projects) != 1 || len(ws.Conversations) != 1 || ws.Conversations[0].ID != kept.ID {
		t.Errorf("workspace after removal = %+v", ws)
	}
	_, err := r.svc.Events(conv.ID, 0)
	wantCode(t, err, CodeNotFound)
	wantCode(t, r.svc.RemoveConversation(conv.ID), CodeNotFound)

	// the native session is untouched and offered for adoption again.
	sessions, err := r.svc.Discover(r.project.ID, model.HarnessClaude)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].NativeID != conv.NativeID {
		t.Errorf("discovery after removal = %+v", sessions)
	}

	// a restart reads the same workspace.
	r.restart()
	if ws := workspaceOf(t, r.svc); len(ws.Conversations) != 1 || ws.Conversations[0].ID != kept.ID {
		t.Errorf("after restart = %+v", ws.Conversations)
	}
}

// a removal whose registry write fails ends the process before the log is
// deleted: the next start still has the conversation and its history.
func TestRemoveConversationKeepsItsLogWhenTheRegistryWriteFails(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the runtime to exit", rt.Exited)
	dir := filepath.Dir(r.path)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })

	reasons := exits(r.svc)
	go r.svc.RemoveConversation(conv.ID)
	exited(t, reasons)
	if _, err := os.Stat(filepath.Join(dir, "conversations", conv.ID, "events.jsonl")); err != nil {
		t.Errorf("the log went with an unconfirmed removal: %v", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	r.crashed()
	if evs := r.events(conv.ID); len(evs) == 0 {
		t.Error("the conversation lost its history")
	}
}

func TestRemoveProjectRefusedWhileAnyConversationHasARuntime(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	wantCode(t, r.svc.RemoveProject(r.project.ID), CodeConflict) // running

	rt.HoldOnClose()
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	wantCode(t, r.svc.RemoveProject(r.project.ID), CodeConflict) // stopping

	// a restart before the stop is seen settles it as unsupervised, which is
	// refused until resumed.
	r.restart()
	wantCode(t, r.svc.RemoveProject(r.project.ID), CodeConflict)
	if err := r.svc.Resume(conv.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.RemoveProject(r.project.ID); err != nil {
		t.Fatalf("removing after the resume: %v", err)
	}
}

func TestRemoveProjectDeletesItsConversationsLogs(t *testing.T) {
	r := newRig(t)
	other, err := r.svc.RegisterProject("other", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, prompt := range []string{"one", "two"} {
		conv := r.start(prompt)
		rt := r.fake.Last()
		started(rt)
		if err := r.svc.Release(conv.ID); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "the runtime to exit", rt.Exited)
		ids = append(ids, conv.ID)
	}
	kept, err := r.svc.StartConversation(other.ID, model.HarnessClaude, "", "kept", LaunchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	logs := filepath.Join(filepath.Dir(r.path), "conversations")

	if err := r.svc.RemoveProject(r.project.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if _, err := os.Stat(filepath.Join(logs, id)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the log of '%s' survived: %v", id, err)
		}
	}
	if _, err := os.Stat(filepath.Join(logs, kept.ID, "events.jsonl")); err != nil {
		t.Errorf("another project's log went with it: %v", err)
	}
	if _, err := os.Stat(r.project.Checkout); err != nil {
		t.Errorf("the checkout was touched: %v", err)
	}
	r.restart()
	if ws := workspaceOf(t, r.svc); len(ws.Projects) != 1 || len(ws.Conversations) != 1 || ws.Conversations[0].ID != kept.ID {
		t.Errorf("after restart = %+v", ws)
	}
}

// an approval for the session is accepted only where the harness offered it:
// mega never extends an approval on its own account.
func TestAllowSessionOnlyWhereOffered(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	plain := rt.Ask(model.RequestPermission, map[string]any{"tool": "Bash"})
	r.looked(conv.ID, plain, `"tool":"Bash"`)
	err := r.svc.Answer(conv.ID, plain, harness.Answer{Decision: harness.DecisionAllowSession})
	wantCode(t, err, CodeInvalid)
	if pending := r.view(conv.ID).Pending; len(pending) != 1 || pending[0].ID != plain {
		t.Fatalf("a refused answer took the request: %+v", pending)
	}
	if err := r.svc.Answer(conv.ID, plain, harness.Answer{Decision: harness.DecisionDeny}); err != nil {
		t.Fatal(err)
	}

	offered := rt.Ask(model.RequestPermission, map[string]any{"tool": "fixture/echo", "allow_session": true})
	r.looked(conv.ID, offered, `"tool":"fixture/echo"`)
	if err := r.svc.Answer(conv.ID, offered, harness.Answer{Decision: harness.DecisionAllowSession}); err != nil {
		t.Fatal(err)
	}
	deliveries := rt.Deliveries()
	if last := deliveries[len(deliveries)-1]; last.Answer.Decision != harness.DecisionAllowSession {
		t.Errorf("delivered %+v", last)
	}
}

// a question is dismissed only where the harness offers it (a pi dialog),
// and only by the operator's answer; the dismissal reaches the harness with
// no answers.
func TestDismissOnlyWhereOffered(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	plain := rt.Ask(model.RequestQuestion, map[string]any{"tool": "question", "questions": []any{map[string]any{"question": "Which?"}}})
	wantCode(t, r.svc.Answer(conv.ID, plain, harness.Answer{Decision: harness.DecisionDismiss}), CodeInvalid)
	if pending := r.view(conv.ID).Pending; len(pending) != 1 || pending[0].ID != plain {
		t.Fatalf("a refused dismissal took the request: %+v", pending)
	}
	r.looked(conv.ID, plain, `"question":"Which?"`)
	if err := r.svc.Answer(conv.ID, plain, harness.Answer{Answers: map[string]string{"Which?": "this"}}); err != nil {
		t.Fatal(err)
	}
	offered := rt.Ask(model.RequestQuestion, map[string]any{"tool": "question", "dismiss": true, "questions": []any{map[string]any{"question": "Proceed?"}}})
	if err := r.svc.Answer(conv.ID, offered, harness.Answer{Decision: harness.DecisionDismiss, Answers: map[string]string{"Proceed?": "Yes"}}); err != nil {
		t.Fatal(err)
	}
	deliveries := rt.Deliveries()
	if last := deliveries[len(deliveries)-1]; last.Answer.Decision != harness.DecisionDismiss || last.Answer.Answers != nil {
		t.Errorf("delivered %+v", last)
	}
	if pending := r.view(conv.ID).Pending; len(pending) != 0 {
		t.Errorf("still pending: %+v", pending)
	}
}

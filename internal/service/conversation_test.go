package service

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"git.hq.quigley.com/research/roundhouse/internal/harness"
	"git.hq.quigley.com/research/roundhouse/internal/harness/harnesstest"
	"git.hq.quigley.com/research/roundhouse/internal/model"
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
	return r
}

// restart closes the service as a crash would (no release) and starts another
// over the same registry and logs.
func (r *rig) restart() {
	r.t.Helper()
	r.svc.Close()
	r.svc = newService(r.t, r.path, allTrusted, r.fake)
}

func (r *rig) start(prompt string) ConversationView {
	r.t.Helper()
	conv, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "", prompt)
	if err != nil {
		r.t.Fatal(err)
	}
	return conv
}

func (r *rig) view(id string) ConversationView {
	r.t.Helper()
	return r.svc.view(id)
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
	wantCode(t, r.svc.Answer(conv.ID, id, harness.Answer{Decision: harness.DecisionAllow}), CodeNotPending)

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
	wantCode(t, r.svc.Answer(conv.ID, id, harness.Answer{Answers: map[string]string{"Which?": "Blue"}}), CodeNotPending)
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
	wantCode(t, r.svc.Answer(conv.ID, id, harness.Answer{Decision: harness.DecisionAllow}), CodeNotPending)
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
	wantCode(t, r.svc.Answer(conv.ID, old, harness.Answer{Decision: harness.DecisionAllow}), CodeNotPending)
	if len(second.Deliveries()) != 0 || len(r.view(conv.ID).Pending) != 1 {
		t.Fatal("the stale answer touched the new runtime's request")
	}
	if err := r.svc.Answer(conv.ID, fresh, harness.Answer{Decision: harness.DecisionAllow}); err != nil {
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

func TestLaunchFailureIsRecordedAndLeavesNoRuntime(t *testing.T) {
	r := newRig(t)
	r.fake.FailLaunch(errors.New("no node"))
	conv, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "named", "one")
	if err == nil {
		t.Fatal("expected the launch to fail")
	}
	if conv.Runtime != model.RuntimeNone || conv.State != model.RuntimeNone {
		t.Errorf("conversation = %+v", conv)
	}
	if got := kinds(r.events(conv.ID)); !slices.Equal(got, []string{"runtime:starting", "runtime:error"}) {
		t.Errorf("events = %v", got)
	}
	r.fake.FailLaunch(nil)
	if err := r.svc.Prompt(conv.ID, "again"); err != nil {
		t.Fatal(err)
	}
}

func TestUnknownHarnessIsInvalid(t *testing.T) {
	r := newRig(t)
	_, err := r.svc.StartConversation(r.project.ID, model.HarnessCodex, "", "one")
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
	if reads := r.fake.Reads(); !slices.Equal(reads, []string{conv.NativeID}) {
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
	if view.Runtime != model.RuntimeNone || slices.Contains(kinds(r.events(conv.ID)), "runtime:unsupervised") {
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

func TestImportIsIdempotentAndMatchesByIdentityNotText(t *testing.T) {
	r := newRig(t)
	conv := r.start("managed prompt")
	rt := r.fake.Last()
	finishTurn(rt, "a-1", "same words")
	managedPrompt := rt.Prompts()[0].UUID
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the runtime to exit", rt.Exited)

	// the native transcript after a terminal visit: the managed turn, then a
	// terminal turn whose reply repeats the managed reply's text.
	r.fake.SetHistory(conv.NativeID, []*model.Event{
		{Kind: model.EventUser, Text: "managed prompt", NativeID: managedPrompt},
		{Kind: model.EventAssistant, Text: "same words", NativeID: "a-1"},
		{Kind: model.EventUser, Text: "terminal prompt", NativeID: "t-1"},
		{Kind: model.EventAssistant, Text: "same words", NativeID: "t-2"},
	})
	if err := r.svc.Resume(conv.ID); err != nil {
		t.Fatal(err)
	}
	once := r.events(conv.ID)
	if err := r.svc.Resume(conv.ID); err != nil {
		t.Fatal(err)
	}
	twice := r.events(conv.ID)
	if got := kinds(twice[len(once):]); !slices.Equal(got, []string{"runtime:resuming", "runtime:resumed"}) {
		t.Errorf("the second import added %v", got)
	}
	if last := twice[len(twice)-1]; last.Text != "0 messages added from native history" {
		t.Errorf("resumed = %q", last.Text)
	}

	var users, sameWords int
	var native []string
	for _, ev := range twice {
		if ev.Kind == model.EventUser && ev.Text == "managed prompt" {
			users++
		}
		if ev.Text == "same words" {
			sameWords++
		}
		if ev.Origin == model.OriginNative {
			native = append(native, ev.NativeID)
		}
	}
	if users != 1 || sameWords != 2 || !slices.Equal(native, []string{"t-1", "t-2"}) {
		t.Errorf("managed prompts %d, same-text replies %d, native %v", users, sameWords, native)
	}
}

func TestPromptAfterReleaseImportsBeforeStarting(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	finishTurn(rt, "a-1", "done")
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the runtime to exit", rt.Exited)
	r.fake.SetHistory(conv.NativeID, []*model.Event{{Kind: model.EventUser, Text: "from the terminal", NativeID: "t-1"}})
	if err := r.svc.Prompt(conv.ID, "two"); err != nil {
		t.Fatal(err)
	}
	evs := r.events(conv.ID)
	var order []string
	for _, ev := range evs[len(evs)-2:] {
		order = append(order, ev.Text)
	}
	if !slices.Equal(order, []string{"from the terminal", "two"}) {
		t.Errorf("tail = %v", order)
	}
}

func TestRemoveProjectRefusedWhileRuntimeLives(t *testing.T) {
	r := newRig(t)
	r.start("one")
	wantCode(t, r.svc.RemoveProject(r.project.ID), CodeConflict)
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

func TestUnrecordedRequestIsNotAnswerable(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	r.svc.mu.Lock()
	r.svc.live[conv.ID].log.Close() // the next append fails
	r.svc.mu.Unlock()
	id := rt.Ask(model.RequestPermission, nil)
	if pending := r.view(conv.ID).Pending; len(pending) != 0 {
		t.Errorf("an unrecorded request is pending: %+v", pending)
	}
	wantCode(t, r.svc.Answer(conv.ID, id, harness.Answer{Decision: harness.DecisionAllow}), CodeNotPending)
	if len(rt.Deliveries()) != 0 {
		t.Error("an unrecorded request was answered")
	}
}

func TestUnrecordedTransitionsLeaveTheConfirmedView(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	r.svc.mu.Lock()
	r.svc.live[conv.ID].log.Close() // every later append fails
	r.svc.mu.Unlock()
	started(rt)
	rt.Emit(&model.Event{Kind: model.EventTurnEnd, Status: model.TurnSuccess})
	view := r.view(conv.ID)
	if !view.TurnActive || view.Facts != nil {
		t.Errorf("unrecorded transitions changed the view: turn active %v, facts %+v", view.TurnActive, view.Facts)
	}
}

func TestUnrecordedStopKeepsTheConfirmedViewAndRestartsUnsupervised(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	id := rt.Ask(model.RequestPermission, nil)
	sub := r.svc.Subscribe()
	defer sub.Close()
	r.svc.mu.Lock()
	r.svc.live[conv.ID].log.Close() // the abort and the stop cannot be recorded
	r.svc.mu.Unlock()
	rt.Exit(nil)

	view := r.view(conv.ID)
	if view.State != StateRunning || view.Runtime != model.RuntimeRunning || len(view.Pending) != 1 {
		t.Errorf("view after an unrecorded stop = %+v", view)
	}
	select {
	case update := <-sub.Updates():
		t.Errorf("an unrecorded stop was announced: %+v", update)
	default:
	}
	wantCode(t, r.svc.Answer(conv.ID, id, harness.Answer{Decision: harness.DecisionAllow}), CodeNotPending)
	if len(rt.Deliveries()) != 0 {
		t.Error("an answer reached a runtime that had exited")
	}

	r.restart()
	if view := r.view(conv.ID); view.Runtime != model.RuntimeUnsupervised {
		t.Errorf("after restart the conversation records %q", view.Runtime)
	}
}

// closeLog makes every later append fail, as a degraded event log does.
func (r *rig) closeLog(id string) {
	r.svc.mu.Lock()
	defer r.svc.mu.Unlock()
	r.svc.live[id].log.Close()
}

func TestUnrecordedReleaseStillStopsTheProcessOnce(t *testing.T) {
	r := newRig(t)
	conv := r.start("one") // the turn stays active, so a release interrupts
	rt := r.fake.Last()
	started(rt)
	rt.HoldOnClose()
	sub := r.svc.Subscribe()
	defer sub.Close()
	r.closeLog(conv.ID)

	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "input to close", rt.Closed)
	if view := r.view(conv.ID); view.State != StateRunning {
		t.Errorf("an unrecorded stopping changed the state to %q", view.State)
	}
	select {
	case update := <-sub.Updates():
		t.Errorf("an unrecorded stopping was announced: %+v", update)
	default:
	}

	// neither a second release nor shutdown asks the process to stop again.
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	r.svc.Shutdown(ctx)
	time.Sleep(20 * time.Millisecond)
	if n := rt.Interrupts(); n != 1 {
		t.Errorf("the process was asked to stop %d times", n)
	}
	wantCode(t, r.svc.Prompt(conv.ID, "two"), CodeUnavailable)
}

func TestUnrecordedWrongSessionStillStopsTheRuntime(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	r.closeLog(conv.ID)
	rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: "someone-else"}})
	waitFor(t, "the mismatched runtime to stop", rt.Exited)
	if view := r.view(conv.ID); view.Facts != nil || view.State == StateRunning {
		t.Errorf("the mismatched runtime was shown running: %+v", view)
	}
}

func TestUnrecordedResumeIsRefusedBeforeReading(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	finishTurn(rt, "a-1", "done")
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the runtime to exit", rt.Exited)
	r.closeLog(conv.ID)
	if err := r.svc.Resume(conv.ID); err == nil {
		t.Fatal("a resume that cannot be recorded went ahead")
	}
	if len(r.fake.Reads()) != 0 {
		t.Error("native history was read for an unrecorded resume")
	}
	if view := r.view(conv.ID); view.State != model.RuntimeNone {
		t.Errorf("state = %q", view.State)
	}
}

func TestUnrecordedLaunchFailureKeepsStartingAndClearsBusy(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	finishTurn(rt, "a-1", "done")
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the runtime to exit", rt.Exited)

	r.svc.mu.Lock()
	l := r.svc.live[conv.ID]
	r.svc.mu.Unlock()
	sub := r.svc.Subscribe()
	defer sub.Close()
	// the log breaks after starting is recorded and before the failure is: drop
	// what was announced so far, then close it.
	r.fake.OnLaunch(func() {
		for len(sub.Updates()) > 0 {
			<-sub.Updates()
		}
		l.log.Close()
	})
	r.fake.FailLaunch(errors.New("no node"))
	launches := len(r.fake.Runtimes())

	if err := r.svc.Prompt(conv.ID, "two"); err == nil {
		t.Fatal("expected the launch to fail")
	}
	view := r.view(conv.ID)
	if view.State != StateStarting {
		t.Errorf("state = %q, want the last confirmed starting", view.State)
	}
	select {
	case update := <-sub.Updates():
		t.Errorf("an unrecorded launch failure was announced: %+v", update)
	default:
	}
	r.svc.mu.Lock()
	busy, runtime := l.busy, l.runtime
	r.svc.mu.Unlock()
	if busy || runtime != nil || len(r.fake.Runtimes()) != launches {
		t.Errorf("busy %v, runtime %v, launches %d -> %d", busy, runtime, launches, len(r.fake.Runtimes()))
	}

	// the conversation stays recorded running, so a restart settles it.
	r.fake.OnLaunch(nil)
	r.restart()
	if view := r.view(conv.ID); view.Runtime != model.RuntimeUnsupervised {
		t.Errorf("after restart the conversation records %q", view.Runtime)
	}
}

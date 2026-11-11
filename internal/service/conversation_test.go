package service

import (
	"context"
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
	conv, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "", prompt, LaunchChoice{})
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
	r.fake.FailLaunch(fmt.Errorf("%w: no node", harness.ErrNotStarted))
	conv, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "named", "one", LaunchChoice{})
	if err == nil {
		t.Fatal("expected the launch to fail")
	}
	if conv.Runtime != model.RuntimeNone || conv.State != model.RuntimeNone {
		t.Errorf("conversation = %+v", conv)
	}
	if got := kinds(r.events(conv.ID)); !slices.Equal(got, []string{"runtime:starting", "runtime:not_started"}) {
		t.Errorf("events = %v", got)
	}
	r.fake.FailLaunch(nil)
	if err := r.svc.Prompt(conv.ID, "again"); err != nil {
		t.Fatal(err)
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
	r.fake.FailLaunch(fmt.Errorf("%w: no node", harness.ErrNotStarted))
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
	if !slices.Equal(got, []string{"runtime:resuming", "user", "assistant", "runtime:resumed"}) {
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
	// an answer to a question whose turn completed starts the next turn, which
	// the runtime first reports as the user message it began with.
	rt.Emit(&model.Event{Kind: model.EventUser, Text: "Blue", NativeID: "u-2"})
	if !r.view(conv.ID).TurnActive {
		t.Error("the turn the answer started is not shown as active")
	}
	wantCode(t, r.svc.Prompt(conv.ID, "two"), CodeConflict)
}

func TestUnrecordedSessionIsResumedAndBoundByTheNextLaunch(t *testing.T) {
	r := newRig(t)
	dir := filepath.Dir(r.path)
	// the registry stops taking writes while the first runtime starts, so the
	// session it creates cannot be recorded.
	r.fake.OnLaunch(func() {
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Error(err)
		}
	})
	t.Cleanup(func() { os.Chmod(dir, 0o700) })
	conv, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "unbound", "one", LaunchChoice{})
	if err == nil {
		t.Skip("directory permissions are not enforced here")
	}
	first := r.fake.Last()
	waitFor(t, "the unbindable runtime to stop", first.Exited)
	if view := r.view(conv.ID); view.NativeID != "" {
		t.Fatalf("the registry recorded %q despite the failed write", view.NativeID)
	}

	r.fake.OnLaunch(nil)
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Prompt(conv.ID, "two"); err != nil {
		t.Fatal(err)
	}
	if spec := r.fake.Last().Spec; spec.NativeID != first.NativeID() {
		t.Errorf("the next launch used %q, not the session already created (%q)", spec.NativeID, first.NativeID())
	}
	if view := r.view(conv.ID); view.NativeID != first.NativeID() {
		t.Errorf("the registry holds %q after the next launch", view.NativeID)
	}
}

func TestSessionStartedInTheLogIsResumedAfterARestart(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	finishTurn(rt, "a-1", "done")
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the runtime to exit", rt.Exited)
	// a registry that lost the binding while the log kept the started event.
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
	launches := len(r.fake.Runtimes())
	if err := r.svc.Prompt(conv.ID, "two"); err != nil {
		t.Fatal(err)
	}
	if len(r.fake.Runtimes()) != launches+1 {
		t.Fatal("no runtime launched")
	}
	if spec := r.fake.Last().Spec; spec.NativeID != rt.NativeID() {
		t.Errorf("launched %q, not the session the log shows started (%q)", spec.NativeID, rt.NativeID())
	}
	if view := r.view(conv.ID); view.NativeID != rt.NativeID() {
		t.Errorf("the registry holds %q", view.NativeID)
	}
	if reads := r.fake.Reads(); len(reads) == 0 || reads[len(reads)-1] != rt.NativeID() {
		t.Errorf("history reads = %v", reads)
	}
}

// both stores fail to keep the session a first launch created: the log
// cannot record its start and the registry cannot bind it. after a restart and
// a resume, the conversation must not start another session in its place.
func TestUnkeptSessionIsNeverReplacedAfterRestart(t *testing.T) {
	r := newRig(t)
	dir := filepath.Dir(r.path)
	svc := r.svc
	r.fake.OnLaunch(func() {
		svc.mu.Lock()
		for _, l := range svc.live {
			l.log.Close()
		}
		svc.mu.Unlock()
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Error(err)
		}
	})
	t.Cleanup(func() { os.Chmod(dir, 0o700) })
	conv, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "unkept", "one", LaunchChoice{})
	if err == nil {
		t.Skip("directory permissions are not enforced here")
	}
	first := r.fake.Last()
	waitFor(t, "the unbindable runtime to stop", first.Exited)
	r.fake.OnLaunch(nil)
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	r.restart()
	if err := r.svc.Resume(conv.ID); err != nil {
		t.Fatal(err)
	}
	launches := len(r.fake.Runtimes())
	err = r.svc.Prompt(conv.ID, "two")
	wantCode(t, err, CodeConflict)
	if err == nil || !strings.Contains(err.Error(), "Adopt a session") {
		t.Errorf("refusal = %v", err)
	}
	if len(r.fake.Runtimes()) != launches {
		t.Error("a fresh session was started in place of the unkept one")
	}
}

func TestRecordedNotStartedAllowsAFreshSession(t *testing.T) {
	r := newRig(t)
	r.fake.FailLaunch(fmt.Errorf("%w: no node", harness.ErrNotStarted))
	conv, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "retry", "one", LaunchChoice{})
	if err == nil {
		t.Fatal("expected the launch to fail")
	}
	r.fake.FailLaunch(nil)
	if err := r.svc.Prompt(conv.ID, "two"); err != nil {
		t.Fatal(err)
	}
	if spec := r.fake.Last().Spec; spec.NativeID != "" {
		t.Errorf("launched %q, want a fresh session", spec.NativeID)
	}
}

// a launch whose failure itself could not be recorded leaves a starting with
// nothing after it: the accepted closed side, refused although no session
// exists.
func TestStartingWithNothingAfterItIsRefused(t *testing.T) {
	r := newRig(t)
	svc := r.svc
	r.fake.OnLaunch(func() {
		svc.mu.Lock()
		for _, l := range svc.live {
			l.log.Close()
		}
		svc.mu.Unlock()
	})
	r.fake.FailLaunch(fmt.Errorf("%w: no node", harness.ErrNotStarted))
	conv, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "closed", "one", LaunchChoice{})
	if err == nil {
		t.Fatal("expected the launch to fail")
	}
	r.fake.OnLaunch(nil)
	r.fake.FailLaunch(nil)
	r.restart()
	if got := kinds(r.events(conv.ID)); !slices.Equal(got[:1], []string{"runtime:starting"}) || slices.Contains(got, "runtime:not_started") {
		t.Fatalf("events = %v", got)
	}
	if err := r.svc.Resume(conv.ID); err != nil {
		t.Fatal(err)
	}
	launches := len(r.fake.Runtimes())
	wantCode(t, r.svc.Prompt(conv.ID, "two"), CodeConflict)
	if len(r.fake.Runtimes()) != launches {
		t.Error("a fresh session was started without proof that none exists")
	}
}

// an unwrapped launch error leaves the session unknown, as a codex thread/start
// that got no reply may have created a thread: it is recorded as an error, and
// the conversation is refused a fresh session.
func TestUnclassifiedLaunchFailureLeavesTheSessionUnknown(t *testing.T) {
	r := newRig(t)
	r.fake.FailLaunch(errors.New("no reply to thread/start"))
	conv, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "unknown", "one", LaunchChoice{})
	if err == nil {
		t.Fatal("expected the launch to fail")
	}
	if got := kinds(r.events(conv.ID)); !slices.Equal(got, []string{"runtime:starting", "runtime:error"}) {
		t.Errorf("events = %v", got)
	}
	if view := r.view(conv.ID); view.State != model.RuntimeNone || view.Runtime != model.RuntimeNone {
		t.Errorf("view = %+v", view)
	}
	r.fake.FailLaunch(nil)
	launches := len(r.fake.Runtimes())
	wantCode(t, r.svc.Prompt(conv.ID, "again"), CodeConflict)
	if len(r.fake.Runtimes()) != launches {
		t.Error("a fresh session was started after a launch whose session is unknown")
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

func TestRemoveConversationRefusedWhileUnsupervisedOrResuming(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	started(r.fake.Last())
	r.restart()
	if view := r.view(conv.ID); view.Runtime != model.RuntimeUnsupervised {
		t.Fatalf("runtime = %q", view.Runtime)
	}
	wantCode(t, r.svc.RemoveConversation(conv.ID), CodeConflict)

	// while the resume reads native history the conversation is busy.
	r.fake.SetHistory(conv.NativeID, nil)
	reading := make(chan struct{})
	release := make(chan struct{})
	r.fake.OnHistory(func() {
		close(reading)
		<-release
	})
	done := make(chan error, 1)
	go func() { done <- r.svc.Resume(conv.ID) }()
	<-reading
	wantCode(t, r.svc.RemoveConversation(conv.ID), CodeConflict)
	close(release)
	if err := <-done; err != nil {
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
	ws := r.svc.Workspace()
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
	if ws := r.svc.Workspace(); len(ws.Conversations) != 1 || ws.Conversations[0].ID != kept.ID {
		t.Errorf("after restart = %+v", ws.Conversations)
	}
}

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

	if err := r.svc.RemoveConversation(conv.ID); err == nil {
		t.Skip("directory permissions are not enforced here")
	}
	if _, err := os.Stat(filepath.Join(dir, "conversations", conv.ID, "events.jsonl")); err != nil {
		t.Errorf("the log went with an unconfirmed removal: %v", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
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

	// the stop cannot be recorded, so the conversation stays recorded running.
	r.closeLog(conv.ID)
	rt.Exit(nil)
	waitFor(t, "the runtime to exit", rt.Exited)
	if view := r.view(conv.ID); view.Runtime != model.RuntimeRunning {
		t.Fatalf("runtime after an unrecorded stop = %q", view.Runtime)
	}
	wantCode(t, r.svc.RemoveProject(r.project.ID), CodeConflict)

	// a restart settles it as unsupervised, which is refused until resumed.
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
	if ws := r.svc.Workspace(); len(ws.Projects) != 1 || len(ws.Conversations) != 1 || ws.Conversations[0].ID != kept.ID {
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
	err := r.svc.Answer(conv.ID, plain, harness.Answer{Decision: harness.DecisionAllowSession})
	wantCode(t, err, CodeInvalid)
	if pending := r.view(conv.ID).Pending; len(pending) != 1 || pending[0].ID != plain {
		t.Fatalf("a refused answer took the request: %+v", pending)
	}
	if err := r.svc.Answer(conv.ID, plain, harness.Answer{Decision: harness.DecisionDeny}); err != nil {
		t.Fatal(err)
	}

	offered := rt.Ask(model.RequestPermission, map[string]any{"tool": "fixture/echo", "allow_session": true})
	if err := r.svc.Answer(conv.ID, offered, harness.Answer{Decision: harness.DecisionAllowSession}); err != nil {
		t.Fatal(err)
	}
	deliveries := rt.Deliveries()
	if last := deliveries[len(deliveries)-1]; last.Answer.Decision != harness.DecisionAllowSession {
		t.Errorf("delivered %+v", last)
	}
}

// a meta user message (a subagent's hand-back) arrives only through an import,
// as a native event with its own identity; importing it again adds nothing.
func TestMetaMessageImportedTwiceIsRecordedOnce(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	finishTurn(rt, "a-1", "done")
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the runtime to exit", rt.Exited)
	r.fake.SetHistory(conv.NativeID, []*model.Event{
		{Kind: model.EventAssistant, Text: "done", NativeID: "a-1"},
		{Kind: model.EventNative, Text: "meta: hand-back from web-fetch", NativeID: "meta-1"},
	})
	for i := 0; i < 2; i++ {
		if err := r.svc.Resume(conv.ID); err != nil {
			t.Fatal(err)
		}
	}
	var metas, prompts int
	for _, ev := range r.events(conv.ID) {
		if ev.NativeID == "meta-1" {
			metas++
			if ev.Kind != model.EventNative {
				t.Errorf("the hand-back was recorded as %s", ev.Kind)
			}
		}
		if ev.Kind == model.EventUser && ev.Origin == model.OriginNative {
			prompts++
		}
	}
	if metas != 1 || prompts != 0 {
		t.Errorf("hand-back recorded %d times; native prompts %d", metas, prompts)
	}
}

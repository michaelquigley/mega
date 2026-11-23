package service

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/registry"
)

// harden-requests-stops-and-restarts: each test fails without its fix.

// Release fences the runtime before it returns: nothing the runtime is asked
// to deliver from then on starts work, whatever the asynchronous interrupt
// and close have reached.
func TestReleaseFencesTheRuntimeBeforeItReturns(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	rt.HoldOnClose()
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	if rt.Fences() == 0 {
		t.Error("Release returned before fencing the runtime")
	}
}

// a request the harness settled, whose resolution could not be recorded,
// stays listed as the last confirmed state, and nothing is broadcast, as
// after any durability fault; but it is never answerable: no answer reaches
// the harness.
func TestAnUnrecordedSettlementLeavesNothingAnswerable(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	id := rt.Ask(model.RequestPermission, map[string]any{"tool": "Bash", "input": map[string]any{"command": "echo"}})
	r.closeLog(conv.ID)
	sub := r.svc.Subscribe()
	defer sub.Close()
	rt.Settle(id, model.OutcomeAborted)
	select {
	case update := <-sub.Updates():
		t.Errorf("an unrecorded settlement was broadcast: %+v", update)
	case <-time.After(100 * time.Millisecond):
	}
	view := r.view(conv.ID)
	if !slices.ContainsFunc(view.Pending, func(req model.Request) bool { return req.ID == id }) {
		t.Fatalf("the last confirmed request is gone: %+v", view.Pending)
	}
	reached := 0
	rt.OnAnswer(func() { reached++ })
	err := r.svc.Answer(conv.ID, id, harness.Answer{Decision: harness.DecisionDeny})
	wantCode(t, err, CodeNotPending)
	if err == nil || !strings.Contains(err.Error(), "could not be recorded") {
		t.Errorf("the refusal does not say why: %v", err)
	}
	if reached != 0 {
		t.Errorf("an answer to a settled request reached the harness %d times", reached)
	}
}

// an answer the harness refuses without settling the request, such as a
// choice it did not offer, records nothing: the request stays pending, and a
// fitting answer then reaches it.
func TestARejectedAnswerLeavesTheRequestPending(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	id := rt.Ask(model.RequestQuestion, map[string]any{"questions": []any{map[string]any{"question": "Which?", "options": []any{map[string]any{"label": "A"}}}}})
	rt.RejectAnswers(fmt.Errorf("%w: 'B' is not one of the choices offered", harness.ErrInvalidAnswer))
	r.looked(conv.ID, id, `"question":"Which?"`)
	wantCode(t, r.svc.Answer(conv.ID, id, harness.Answer{Answers: map[string]string{"Which?": "B"}}), CodeInvalid)
	if pending := r.view(conv.ID).Pending; len(pending) != 1 || pending[0].ID != id {
		t.Fatalf("a rejected answer settled the request: %+v", pending)
	}
	for _, ev := range r.events(conv.ID) {
		if ev.Kind == model.EventRequestResolved {
			t.Errorf("a rejected answer recorded %+v", ev.Req)
		}
	}
	rt.RejectAnswers(nil)
	r.looked(conv.ID, id, `"label":"A"`)
	if err := r.svc.Answer(conv.ID, id, harness.Answer{Answers: map[string]string{"Which?": "A"}}); err != nil {
		t.Errorf("a fitting answer after a rejected one: %v", err)
	}
}

// an answer the harness was sent and never confirmed is settled once, as
// unconfirmed, by the adapter before it returns: the operator is told so in
// the harness's words, the request is no longer pending, and a second answer
// is refused without reaching the harness.
func TestAnUnconfirmedAnswerIsSettledOnceAndSaidSo(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	id := rt.Ask(model.RequestQuestion, map[string]any{"questions": []any{map[string]any{"question": "Which?", "options": []any{map[string]any{"label": "A"}}}}})
	rt.SettleAnswers(model.OutcomeUnconfirmed, fmt.Errorf("Fake %w", harness.ErrUnconfirmed))
	r.looked(conv.ID, id, `"question":"Which?"`)
	err := r.svc.Answer(conv.ID, id, harness.Answer{Answers: map[string]string{"Which?": "A"}})
	wantCode(t, err, CodeHarnessFailed)
	if err == nil || err.Error() != "Fake did not confirm whether it received your answer" {
		t.Errorf("the operator is told %v", err)
	}
	if pending := r.view(conv.ID).Pending; len(pending) != 0 {
		t.Errorf("still pending: %+v", pending)
	}
	var outcomes []string
	for _, ev := range r.events(conv.ID) {
		if ev.Kind == model.EventRequestResolved && ev.Req.ID == id {
			outcomes = append(outcomes, ev.Req.Outcome)
		}
	}
	if !slices.Equal(outcomes, []string{model.OutcomeUnconfirmed}) {
		t.Errorf("resolutions recorded: %v", outcomes)
	}
	wantCode(t, r.svc.Answer(conv.ID, id, harness.Answer{Answers: map[string]string{"Which?": "A"}}), CodeNotPending)
	if delivered := rt.Deliveries(); len(delivered) != 0 {
		t.Errorf("delivered %+v", delivered)
	}
}

// a settlement whose record failed keeps its outcome: when the runtime's exit
// retires what is still listed, once the log takes appends again, it records
// that outcome, not aborted, so the history says what the operator was told.
func TestAnExitRecordsAnUnrecordedSettlementAsItWas(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	id := rt.Ask(model.RequestQuestion, map[string]any{"questions": []any{map[string]any{"question": "Which?", "options": []any{map[string]any{"label": "A"}}}}})
	r.closeLog(conv.ID)
	rt.Settle(id, model.OutcomeUnconfirmed)
	if pending := r.view(conv.ID).Pending; len(pending) != 1 {
		t.Fatalf("the unrecorded settlement is not listed as last confirmed: %+v", pending)
	}
	r.reopenLog(conv.ID)
	rt.Exit(nil)
	var outcomes []string
	for _, ev := range r.events(conv.ID) {
		if ev.Kind == model.EventRequestResolved && ev.Req.ID == id {
			outcomes = append(outcomes, ev.Req.Outcome)
		}
	}
	if !slices.Equal(outcomes, []string{model.OutcomeUnconfirmed}) {
		t.Errorf("resolutions recorded: %v", outcomes)
	}
}

// an answer the harness took, and settled itself, whose settlement cannot be
// recorded is not reported delivered as if all were well: the caller hears
// that it could not be recorded, and the request stays listed as last
// confirmed, taking no further answer.
func TestADeliveredAnswerWhoseSettlementIsNotRecordedSaysSo(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	id := rt.Ask(model.RequestQuestion, map[string]any{"questions": []any{map[string]any{"question": "Which?", "options": []any{map[string]any{"label": "A"}}}}})
	rt.SettleAnswers(model.OutcomeAnswered, nil)
	r.looked(conv.ID, id, `"question":"Which?"`)
	rt.OnAnswer(func() { r.closeLog(conv.ID) })
	if err := r.svc.Answer(conv.ID, id, harness.Answer{Answers: map[string]string{"Which?": "A"}}); err == nil {
		t.Error("an answer whose settlement was not recorded is reported delivered")
	}
	if pending := r.view(conv.ID).Pending; len(pending) != 1 || pending[0].ID != id {
		t.Errorf("the last confirmed request is gone: %+v", pending)
	}
	wantCode(t, r.svc.Answer(conv.ID, id, harness.Answer{Answers: map[string]string{"Which?": "A"}}), CodeNotPending)
	if delivered := rt.Deliveries(); len(delivered) != 1 {
		t.Errorf("delivered %d times", len(delivered))
	}
}

// a settlement the harness reports while an answer is on its way, and that
// could not be recorded, stands when the answer returns: it is recorded as
// it was settled, not as the answer's own result, and the request takes no
// further answer.
func TestAnAnswerKeepsASettlementThatCrossedIt(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	id := rt.Ask(model.RequestQuestion, map[string]any{"questions": []any{map[string]any{"question": "Which?", "options": []any{map[string]any{"label": "A"}}}}})
	rt.OnAnswer(func() {
		rt.OnAnswer(nil)
		r.closeLog(conv.ID)
		rt.Settle(id, model.OutcomeExpired)
		r.reopenLog(conv.ID)
	})
	r.looked(conv.ID, id, `"question":"Which?"`)
	wantCode(t, r.svc.Answer(conv.ID, id, harness.Answer{Answers: map[string]string{"Which?": "A"}}), CodeNotPending)
	var outcomes []string
	for _, ev := range r.events(conv.ID) {
		if ev.Kind == model.EventRequestResolved && ev.Req.ID == id {
			outcomes = append(outcomes, ev.Req.Outcome)
		}
	}
	if !slices.Equal(outcomes, []string{model.OutcomeExpired}) {
		t.Errorf("resolutions recorded: %v", outcomes)
	}
	if pending := r.view(conv.ID).Pending; len(pending) != 0 {
		t.Errorf("still listed: %+v", pending)
	}
}

// a resume whose recovered output cannot be recorded unread stays
// unsupervised and says so, so a retry counts it; it never clears the state
// that would have counted it.
func TestAResumeThatCannotRecordAcknowledgmentStaysUnsupervised(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	finishTurn(r.fake.Last(), "a-1", "done")
	r.readAll(conv.ID)
	r.restart()
	r.fake.SetHistory(conv.NativeID, []*model.Event{
		{Kind: model.EventAssistant, Text: "done", NativeID: "a-1"},
		{Kind: model.EventAssistant, Text: "finished unobserved", NativeID: "o-1"},
	})
	failed := false
	r.svc.failUpdate = func() error {
		if failed {
			return nil
		}
		failed = true
		return errors.New("the registry could not be written")
	}
	if err := r.svc.Resume(conv.ID); err == nil {
		t.Error("a resume that could not record the acknowledgment reported success")
	}
	if view := r.view(conv.ID); view.Runtime != model.RuntimeUnsupervised {
		t.Fatalf("after the failed resume: %q", view.Runtime)
	}
	if err := r.svc.Resume(conv.ID); err != nil {
		t.Fatal(err)
	}
	if got := r.attention(conv.ID); got != model.AttentionIdle {
		t.Errorf("after the retry: %q", got)
	}
}

// a runtime that reports another session than the one it was bound to is
// stopped, and nothing it reports from then on is recorded: no output, no
// request, never in the conversation it does not belong to.
func TestAWrongSessionRecordsNothingMore(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	rt.HoldOnClose()
	rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: "another-session"}})
	rt.Emit(&model.Event{Kind: model.EventAssistant, Text: "from another session", NativeID: "x-1"})
	rt.Ask(model.RequestPermission, map[string]any{"tool": "Bash", "input": map[string]any{"command": "echo"}})
	for _, ev := range r.events(conv.ID) {
		if ev.NativeID == "x-1" || ev.Kind == model.EventRequest {
			t.Errorf("a runtime on the wrong session recorded %s %q", ev.Kind, ev.Text)
		}
	}
	if pending := r.view(conv.ID).Pending; len(pending) != 0 {
		t.Errorf("a runtime on the wrong session raised %+v", pending)
	}
}

// a prompt the harness refuses starts no turn: the turn the service opened
// for it ends, saying why, and the next prompt is taken.
func TestARefusedPromptLeavesNoTurnOpen(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	finishTurn(rt, "a-1", "done")
	rt.RefusePrompts(fmt.Errorf("%w: busy", harness.ErrPromptRefused))
	if err := r.svc.Prompt(conv.ID, "two"); err == nil {
		t.Error("a refused prompt reported success")
	}
	if view := r.view(conv.ID); view.TurnActive || view.Attention == model.AttentionRunning {
		t.Errorf("after a refused prompt: turn %v, attention %q", view.TurnActive, view.Attention)
	}
	rt.RefusePrompts(nil)
	if err := r.svc.Prompt(conv.ID, "three"); err != nil {
		t.Errorf("the next prompt: %v", err)
	}
}

// once the service has closed, nothing writes the registry: a late
// environment resolution, or anything else, finds the write refused, so the
// file the next instance owns is never touched.
func TestNothingWritesTheRegistryAfterClose(t *testing.T) {
	r := newRig(t)
	r.svc.Close()
	err := r.svc.update(func(doc *registry.Document) error {
		doc.Projects = nil
		return nil
	})
	if err == nil {
		t.Error("the registry was written after Close")
	}
}

// an orderly shutdown's last word to every subscriber, before its stream
// closes, is that it has finished; a service closed without one, as after a
// crash or in a test, says nothing of the kind.
func TestAFinishedShutdownIsTheStreamsLastWord(t *testing.T) {
	r := newRig(t)
	sub := r.svc.Subscribe()
	r.svc.Shutdown(t.Context())
	r.svc.Close()
	var last Update
	for update := range sub.Updates() {
		last = update
	}
	if !last.ShutdownFinished {
		t.Errorf("the last update before the stream closed was %+v", last)
	}

	other := newRig(t)
	quiet := other.svc.Subscribe()
	other.svc.Close()
	for update := range quiet.Updates() {
		if update.ShutdownFinished {
			t.Error("a service closed without a shutdown said one finished")
		}
	}
}

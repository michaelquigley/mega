package service

import (
	"errors"
	"fmt"
	"slices"
	"testing"

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

// a resume whose acknowledgment cannot be written ends the process, and the
// next start finds the conversation still unsupervised, to be resumed again.
func TestAResumeThatCannotRecordAcknowledgmentStaysUnsupervised(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	finishTurn(r.fake.Last(), "a-1", "done")
	r.readAll(conv.ID)
	r.restart()
	r.svc.failUpdate = func() error { return errors.New("the registry could not be written") }
	reasons := exits(r.svc)
	go r.svc.Resume(conv.ID)
	exited(t, reasons)
	r.crashed()
	if view := r.view(conv.ID); view.Runtime != model.RuntimeUnsupervised {
		t.Fatalf("after the next start: %q", view.Runtime)
	}
	if err := r.svc.Resume(conv.ID); err != nil {
		t.Fatal(err)
	}
	if got := r.attention(conv.ID); got != model.AttentionIdle {
		t.Errorf("after the resume: %q", got)
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

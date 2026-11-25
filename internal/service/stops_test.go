package service

import (
	"errors"
	"testing"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

// the faults around stops and restarts (roadmap, loose-ends-around-stops-
// and-restarts): each test fails without its fix.

func seqOf(evs []*model.Event, nativeID string) int64 {
	for _, ev := range evs {
		if ev.NativeID == nativeID {
			return ev.Seq
		}
	}
	return 0
}

// a runtime failure outranks unread output until a new runtime starts.
func TestAFailedRuntimesOutputRemainsUnreadAfterItStartsAgain(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	rt.Emit(&model.Event{Kind: model.EventAssistant, Text: "half a reply", NativeID: "a-1"})
	rt.Exit(errors.New("the process died"))
	if got := r.attention(conv.ID); got != model.AttentionError {
		t.Fatalf("after the exit: attention %q", got)
	}
	if err := r.svc.Prompt(conv.ID, "two"); err != nil {
		t.Fatal(err)
	}
	next := r.fake.Last()
	started(next)
	next.Emit(&model.Event{Kind: model.EventNative, Text: "system/status", NativeID: "n-1"})
	next.Emit(&model.Event{Kind: model.EventTurnEnd, Status: model.TurnSuccess})
	if got := r.attention(conv.ID); got != model.AttentionUnread {
		t.Errorf("after a new runtime started: attention %q", got)
	}
	if err := r.svc.Read(conv.ID, seqOf(r.events(conv.ID), "a-1")); err != nil {
		t.Fatal(err)
	}
	if got := r.attention(conv.ID); got != model.AttentionIdle {
		t.Errorf("after reading through the half reply: attention %q", got)
	}
}

func TestAResumeAfterACrashBeforeStartedLeavesAdoptedOutputUnread(t *testing.T) {
	r := newRig(t)
	r.fake.SetSessions([]harness.Session{{NativeID: "terminal-2", Preview: "from the terminal"}})
	r.fake.SetHistory("terminal-2", []*model.Event{
		{Kind: model.EventUser, Text: "from the terminal", NativeID: "u-1"},
		{Kind: model.EventAssistant, Text: "reply in the terminal", NativeID: "u-2"},
	})
	conv, err := r.svc.Adopt(r.project.ID, model.HarnessClaude, "terminal-2", "adopted")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Start(conv.ID); err != nil {
		t.Fatal(err)
	}
	// no started report: the service dies first.
	r.restart()
	if view := r.view(conv.ID); view.Runtime != model.RuntimeUnsupervised {
		t.Fatalf("after the restart: %q", view.Runtime)
	}
	if err := r.svc.Resume(conv.ID); err != nil {
		t.Fatal(err)
	}
	if got := r.attention(conv.ID); got != model.AttentionUnread {
		t.Errorf("after resume without reading the adopted history: %q", got)
	}
}

// a request leaves the pending set readers see only once its answer is
// recorded, and is reserved against a second answer meanwhile.
func TestAnAnswerHidesNoRequest(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	id := rt.Ask(model.RequestPermission, map[string]any{"tool": "Bash", "input": map[string]any{"command": "echo"}})
	var during []model.Request
	var second error
	rt.OnAnswer(func() {
		during = r.view(conv.ID).Pending
		second = r.svc.Answer(conv.ID, id, harness.Answer{Decision: harness.DecisionDeny})
	})
	if err := r.svc.Answer(conv.ID, id, harness.Answer{Decision: harness.DecisionDeny}); err != nil {
		t.Fatal(err)
	}
	if len(during) != 1 || during[0].ID != id {
		t.Errorf("while the answer was in flight the pending set was %+v", during)
	}
	wantCode(t, second, CodeConflict)
	if len(rt.Deliveries()) != 1 || len(r.view(conv.ID).Pending) != 0 {
		t.Errorf("deliveries %+v, pending %+v", rt.Deliveries(), r.view(conv.ID).Pending)
	}
}

// an interrupt is under way while a turn or a request is outstanding. one
// pressed with no turn running, while a request is still on its way to being
// settled, as a codex answer's steer awaiting its reply after its turn ended,
// is over once that request is settled, not at the next prompt. with a turn
// running, it lasts until the turn ends.
func TestAnInterruptEndsWhenNothingIsLeftOutstanding(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	rt.HoldInterrupts()
	question := map[string]any{"questions": []any{map[string]any{"question": "Which?", "options": []any{map[string]any{"label": "A"}}}}}
	during := rt.Ask(model.RequestQuestion, question)
	if err := r.svc.Interrupt(conv.ID); err != nil {
		t.Fatal(err)
	}
	rt.Settle(during, model.OutcomeAborted)
	if view := r.view(conv.ID); !view.Interrupting || !view.TurnActive {
		t.Errorf("with the turn still running, the interrupt is over: %+v", view)
	}
	finishTurn(rt, "a-1", "done")
	after := rt.Ask(model.RequestQuestion, question)
	if err := r.svc.Interrupt(conv.ID); err != nil {
		t.Fatal(err)
	}
	if view := r.view(conv.ID); !view.Interrupting || view.TurnActive {
		t.Fatalf("the interrupt under way, with no turn: %+v", view)
	}
	rt.Settle(after, model.OutcomeUnconfirmed)
	if view := r.view(conv.ID); view.Interrupting || len(view.Pending) != 0 {
		t.Errorf("with nothing left outstanding, an interrupt is still shown under way: interrupting %v, pending %+v", view.Interrupting, view.Pending)
	}
}

// once an interrupt or a release has begun, an answer that would let work go
// on is refused as a conflict, the request still pending; deny and dismiss
// still settle it, toward the stop.
func TestNoWorkIsAnsweredAfterAStop(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	rt.HoldInterrupts()
	id := rt.Ask(model.RequestPermission, map[string]any{"tool": "Bash", "input": map[string]any{"command": "echo"}, "allow_session": true})
	if err := r.svc.Interrupt(conv.ID); err != nil {
		t.Fatal(err)
	}
	if view := r.view(conv.ID); !view.Interrupting {
		t.Errorf("an interrupt under way is not shown: %+v", view)
	}
	r.looked(conv.ID, id, `"command":"echo"`)
	for _, refused := range []harness.Answer{{Decision: harness.DecisionAllow}, {Decision: harness.DecisionAllowSession}} {
		wantCode(t, r.svc.Answer(conv.ID, id, refused), CodeConflict)
	}
	if len(rt.Deliveries()) != 0 || len(r.view(conv.ID).Pending) != 1 {
		t.Fatalf("after a refused answer: deliveries %+v, pending %+v", rt.Deliveries(), r.view(conv.ID).Pending)
	}
	if err := r.svc.Answer(conv.ID, id, harness.Answer{Decision: harness.DecisionDeny}); err != nil {
		t.Errorf("deny after an interrupt: %v", err)
	}

	// a question asked under a release that has begun: a chosen answer is
	// refused, a dismissal is not.
	other := r.start("two")
	ort := r.fake.Last()
	started(ort)
	ort.HoldInterrupts()
	ort.HoldOnClose()
	q := ort.Ask(model.RequestQuestion, map[string]any{"questions": []any{map[string]any{"question": "Which?", "options": []any{map[string]any{"label": "A"}}}}, "dismiss": true})
	if err := r.svc.Release(other.ID); err != nil {
		t.Fatal(err)
	}
	r.looked(other.ID, q, `"question":"Which?"`)
	wantCode(t, r.svc.Answer(other.ID, q, harness.Answer{Answers: map[string]string{"Which?": "A"}}), CodeConflict)
	if err := r.svc.Answer(other.ID, q, harness.Answer{Decision: harness.DecisionDismiss}); err != nil {
		t.Errorf("dismiss during a release: %v", err)
	}
}

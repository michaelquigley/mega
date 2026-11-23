package service

import (
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

// promptAt is the time of the newest prompt mega recorded for a
// conversation.
func (r *rig) promptAt(id string) time.Time {
	r.t.Helper()
	var at time.Time
	for _, ev := range r.events(id) {
		if ev.Kind == model.EventUser && ev.Origin == model.OriginManaged {
			at = ev.At
		}
	}
	return at
}

// a conversation was last instructed when mega last sent it a prompt: the
// launch's opening prompt, then every prompt after. an answer to a request,
// a start, a mode change, a prompt a harness reports itself and an imported
// prompt leave it as it was. it survives a restart, read back from the log.
func TestInstructedIsTheLastPromptSent(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	startedIn(rt, "default")
	opened := r.promptAt(conv.ID)
	if got := r.view(conv.ID).InstructedAt; opened.IsZero() || !got.Equal(opened) {
		t.Fatalf("after the launch: %v, the prompt at %v", got, opened)
	}
	finishTurn(rt, "a-1", "done")

	// an answer to a request, a mode change, and a prompt the harness reports
	// itself: none instructs.
	time.Sleep(5 * time.Millisecond)
	id := rt.Ask(model.RequestPermission, map[string]any{"tool": "Bash"})
	waitFor(t, "the request pending", func() bool { return len(r.view(conv.ID).Pending) == 1 })
	// denied: what the answer is does not matter here, and nothing is
	// approved that is not itself under test.
	if err := r.svc.Answer(conv.ID, id, harness.Answer{Decision: harness.DecisionDeny, Message: "the test only answers"}); err != nil {
		t.Fatal(err)
	}
	r.switchTo(conv.ID, "acceptEdits")
	rt.Emit(&model.Event{Kind: model.EventUser, Text: "a prompt the harness reported", NativeID: "h-1", Raw: map[string]any{"type": "user"}})
	if got := r.view(conv.ID).InstructedAt; !got.Equal(opened) {
		t.Errorf("an answer, a mode change or a harness's own prompt moved it: %v, was %v", got, opened)
	}

	// a prompt moves it.
	finishTurn(rt, "a-1b", "answered")
	time.Sleep(5 * time.Millisecond)
	if err := r.svc.Prompt(conv.ID, "two"); err != nil {
		t.Fatal(err)
	}
	second := r.promptAt(conv.ID)
	if got := r.view(conv.ID).InstructedAt; !second.After(opened) || !got.Equal(second) {
		t.Fatalf("after a second prompt: %v, the prompt at %v", got, second)
	}

	// a release, a view of a prompt typed in a terminal, and a start with
	// no prompt: none instructs.
	finishTurn(rt, "a-2", "done")
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the runtime to exit", rt.Exited)
	r.fake.SetHistory(conv.NativeID, []*model.Event{{Kind: model.EventUser, Text: "from the terminal", NativeID: "t-1"}})
	if _, err := r.svc.NativeTranscript(conv.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Start(conv.ID); err != nil {
		t.Fatal(err)
	}
	startedIn(r.fake.Last(), "default")
	waitFor(t, "the start", func() bool { return r.view(conv.ID).State == model.RuntimeRunning })
	if got := r.view(conv.ID).InstructedAt; !got.Equal(second) {
		t.Errorf("viewing the transcript or starting moved it: %v, was %v", got, second)
	}

	// a restart reads it back from the log, past the harness's own prompt
	// kept in its timeline.
	r.restart()
	if got := r.view(conv.ID).InstructedAt; !got.Equal(second) {
		t.Errorf("after a restart: %v, want %v", got, second)
	}
	// a conversation never prompted through mega has none.
	adopted, err := r.svc.Adopt(r.project.ID, model.HarnessClaude, "terminal-9", "")
	if err == nil && !r.view(adopted.ID).InstructedAt.IsZero() {
		t.Errorf("an adopted conversation: %v", r.view(adopted.ID).InstructedAt)
	}
}

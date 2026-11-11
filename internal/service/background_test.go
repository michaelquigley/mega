package service

import (
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/model"
)

func jobs(descriptions ...string) []model.BackgroundJob {
	var out []model.BackgroundJob
	for i, d := range descriptions {
		out = append(out, model.BackgroundJob{ID: d, Description: d, Kind: "local_bash", StartedAt: time.Now().Add(-time.Duration(i) * time.Minute)})
	}
	return out
}

// background work is in the workspace read while the runtime reports it,
// replaced whole by each report, and never attention: an idle conversation
// stays idle and out of needs-you, an unread one stays unread.
func TestBackgroundIsShownButNeverAttention(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	startedIn(rt, "default")
	finishTurn(rt, "a-1", "done")
	if err := r.svc.Read(conv.ID, r.view(conv.ID).LastSeq); err != nil {
		t.Fatal(err)
	}
	rt.Background(jobs("npm test", "sleep 300"))
	view := r.view(conv.ID)
	if len(view.Background) != 2 || view.Background[0].Description != "npm test" {
		t.Fatalf("background = %+v", view.Background)
	}
	if view.Attention != model.AttentionIdle || len(r.svc.Workspace().NeedsYou) != 0 {
		t.Errorf("background work changed attention: %q, needs-you %v", view.Attention, r.svc.Workspace().NeedsYou)
	}
	rt.Background(jobs("sleep 300"))
	if view := r.view(conv.ID); len(view.Background) != 1 || view.Background[0].Description != "sleep 300" {
		t.Errorf("after the next report: %+v", view.Background)
	}
	// a woken turn leaves output unread; background work does not hide it.
	rt.Emit(&model.Event{Kind: model.EventAssistant, Text: "a job finished", NativeID: "a-2"})
	rt.Emit(&model.Event{Kind: model.EventTurnEnd, Status: model.TurnSuccess})
	if view := r.view(conv.ID); view.Attention != model.AttentionUnread || len(view.Background) != 1 {
		t.Errorf("unread with background: %q, %d jobs", view.Attention, len(view.Background))
	}
	rt.Background(nil)
	if view := r.view(conv.ID); len(view.Background) != 0 {
		t.Errorf("an empty report left %+v", view.Background)
	}
	if evs := r.events(conv.ID); len(evs) == 0 {
		t.Fatal("no events")
	}
	for _, ev := range r.events(conv.ID) {
		if ev.Kind == model.EventRuntime && ev.Text == "background" {
			t.Errorf("background was recorded: %+v", ev)
		}
	}
}

// the list goes with the runtime, and a restart knows none.
func TestBackgroundClearsOnExitAndIsAbsentAfterARestart(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	startedIn(rt, "default")
	finishTurn(rt, "a-1", "done")
	rt.Background(jobs("sleep 300"))
	rt.Exit(nil)
	waitFor(t, "the exit to be recorded", func() bool { return r.view(conv.ID).State == model.RuntimeNone })
	if view := r.view(conv.ID); len(view.Background) != 0 {
		t.Errorf("after the exit: %+v", view.Background)
	}

	other := r.start("two")
	ort := r.fake.Last()
	startedIn(ort, "default")
	finishTurn(ort, "b-1", "done")
	ort.Background(jobs("sleep 300"))
	r.restart()
	view := r.view(other.ID)
	if len(view.Background) != 0 || view.State != model.RuntimeUnsupervised {
		t.Errorf("after a restart: state %q, background %+v", view.State, view.Background)
	}
	if !r.registryLacks(t, "sleep 300") {
		t.Error("the registry stores background work")
	}
}

// a codex background command recorded live after its turn, under the
// identities its history maps to, is not recorded again when the conversation
// is released and resumed: the import's tool call and result for it are the
// same native identities, already seen.
func TestBackgroundCompletionIsNotImportedAgainOnResume(t *testing.T) {
	r, codex := modelRig(t)
	conv, err := r.svc.StartConversation(r.project.ID, model.HarnessCodex, "", "one", LaunchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	rt := codex.Last()
	startedOn(rt, "gpt-a", "high")
	finishTurn(rt, "a-1", "done")
	// as the codex adapter records a command that finished after its turn.
	rt.Emit(&model.Event{Kind: model.EventToolUse, Tool: "shell", ToolUseID: "exec-1", Text: "shell sleep 30", NativeID: "exec-1", AfterTurn: true})
	rt.Emit(&model.Event{Kind: model.EventToolResult, ToolUseID: "exec-1", Text: "done", NativeID: "exec-1/result", AfterTurn: true})
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the runtime to exit", rt.Exited)
	codex.SetHistory(conv.NativeID, []*model.Event{
		{Kind: model.EventAssistant, Text: "done", NativeID: "a-1"},
		{Kind: model.EventToolUse, Tool: "shell", ToolUseID: "exec-1", Text: "shell sleep 30", NativeID: "exec-1"},
		{Kind: model.EventToolResult, ToolUseID: "exec-1", Text: "", NativeID: "exec-1/result"},
	})
	before := len(r.events(conv.ID))
	if err := r.svc.Resume(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the resume to settle", func() bool { return r.view(conv.ID).State == model.RuntimeNone })
	for _, ev := range r.events(conv.ID)[before:] {
		if ev.NativeID == "exec-1" || ev.NativeID == "exec-1/result" {
			t.Errorf("the background command was imported again: %s %q", ev.Kind, ev.Text)
		}
	}
}

// output after its turn (a codex background command finishing once its turn
// ended) opens no turn and is unread at once; read, it is read.
func TestOutputAfterItsTurnIsUnreadWithoutATurn(t *testing.T) {
	r, codex := modelRig(t)
	conv, err := r.svc.StartConversation(r.project.ID, model.HarnessCodex, "", "one", LaunchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	rt := codex.Last()
	startedOn(rt, "gpt-a", "high")
	finishTurn(rt, "a-1", "BG-STARTED")
	if err := r.svc.Read(conv.ID, r.view(conv.ID).LastSeq); err != nil {
		t.Fatal(err)
	}
	rt.Background(jobs("sleep 30"))
	if view := r.view(conv.ID); view.Attention != model.AttentionIdle {
		t.Fatalf("before the command finished: %q", view.Attention)
	}
	rt.Emit(&model.Event{Kind: model.EventToolUse, Tool: "shell", ToolUseID: "exec-1", Text: "shell sleep 30", NativeID: "exec-1", AfterTurn: true})
	rt.Emit(&model.Event{Kind: model.EventToolResult, ToolUseID: "exec-1", Text: "failed: exit 1", Status: model.TurnError, NativeID: "exec-1/result", AfterTurn: true})
	rt.Background(nil)
	view := r.view(conv.ID)
	if view.TurnActive || view.Attention != model.AttentionUnread || view.OutputSeq != view.LastSeq {
		t.Errorf("after the late result: turn %v, attention %q, output %d of %d", view.TurnActive, view.Attention, view.OutputSeq, view.LastSeq)
	}
	if ws := r.svc.Workspace(); len(ws.NeedsYou) != 1 || ws.NeedsYou[0] != conv.ID {
		t.Errorf("needs-you = %v", ws.NeedsYou)
	}
}

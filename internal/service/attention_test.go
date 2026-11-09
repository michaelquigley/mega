package service

import (
	"context"
	"slices"
	"testing"
	"time"

	"git.hq.quigley.com/research/roundhouse/internal/harness"
	"git.hq.quigley.com/research/roundhouse/internal/model"
)

func (r *rig) attention(id string) string {
	r.t.Helper()
	return r.view(id).Attention
}

func (r *rig) readAll(id string) {
	r.t.Helper()
	if err := r.svc.Read(id, r.view(id).LastSeq); err != nil {
		r.t.Fatal(err)
	}
}

func TestAttentionFollowsConfirmedFacts(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	if got := r.attention(conv.ID); got != model.AttentionRunning {
		t.Errorf("prompted: %q", got)
	}

	id := rt.Ask(model.RequestPermission, nil)
	if got := r.attention(conv.ID); got != model.AttentionRequest {
		t.Errorf("request pending: %q", got)
	}
	if err := r.svc.Answer(conv.ID, id, harness.Answer{Decision: harness.DecisionAllow}); err != nil {
		t.Fatal(err)
	}

	// output mid-turn is running, not unread.
	rt.Emit(&model.Event{Kind: model.EventAssistant, Text: "working", NativeID: "a-1"})
	if got, view := r.attention(conv.ID), r.view(conv.ID); got != model.AttentionRunning || view.OutputSeq != 0 {
		t.Errorf("mid-turn: %q, output seq %d", got, view.OutputSeq)
	}
	rt.Emit(&model.Event{Kind: model.EventTurnEnd, Status: model.TurnSuccess})
	view := r.view(conv.ID)
	if view.Attention != model.AttentionUnread || view.OutputSeq == 0 || view.OutputSeq >= view.LastSeq {
		t.Errorf("turn ended: %q, output seq %d of %d", view.Attention, view.OutputSeq, view.LastSeq)
	}

	r.readAll(conv.ID)
	if got := r.attention(conv.ID); got != model.AttentionIdle {
		t.Errorf("read: %q", got)
	}
}

func TestTurnWithoutAgentOutputIsNotUnread(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	rt.Emit(&model.Event{Kind: model.EventTurnEnd, Status: model.TurnInterrupted})
	if got := r.attention(conv.ID); got != model.AttentionIdle {
		t.Errorf("attention = %q", got)
	}
}

func TestReadThenReleaseStaysIdle(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	finishTurn(rt, "a-1", "done")
	r.readAll(conv.ID)
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the runtime to exit", rt.Exited)
	if got := r.attention(conv.ID); got != model.AttentionIdle {
		t.Errorf("after release: %q", got)
	}
}

func TestReadThenRestartStaysIdle(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	finishTurn(r.fake.Last(), "a-1", "done")
	r.readAll(conv.ID)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r.svc.Shutdown(ctx)
	r.restart()
	if got := r.attention(conv.ID); got != model.AttentionIdle {
		t.Errorf("after a clean restart: %q", got)
	}
}

func TestUnsupervisedIsItsOwnState(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	finishTurn(r.fake.Last(), "a-1", "done")
	r.readAll(conv.ID)
	r.restart() // unclean: the runtime was never seen to exit
	if got := r.attention(conv.ID); got != model.AttentionUnsupervised {
		t.Errorf("after an unclean restart: %q", got)
	}
	if err := r.svc.Resume(conv.ID); err != nil {
		t.Fatal(err)
	}
	if got := r.attention(conv.ID); got != model.AttentionIdle {
		t.Errorf("after resume: %q", got)
	}
}

func TestImportFromOutsideRoundhouseIsNotUnread(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	finishTurn(rt, "a-1", "done")
	r.readAll(conv.ID)
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the runtime to exit", rt.Exited)
	r.fake.SetHistory(conv.NativeID, []*model.Event{
		{Kind: model.EventUser, Text: "from the terminal", NativeID: "t-1"},
		{Kind: model.EventAssistant, Text: "terminal reply", NativeID: "t-2"},
	})
	if err := r.svc.Resume(conv.ID); err != nil {
		t.Fatal(err)
	}
	if got := r.attention(conv.ID); got != model.AttentionIdle {
		t.Errorf("after importing a terminal visit: %q", got)
	}
}

func TestExitDuringTurnIsErrorUntilARuntimeStarts(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	r.fake.Last().Exit(nil)
	if got := r.attention(conv.ID); got != model.AttentionError {
		t.Errorf("exited mid-turn: %q", got)
	}
	r.readAll(conv.ID)
	if got := r.attention(conv.ID); got != model.AttentionError {
		t.Errorf("reading does not clear an error: %q", got)
	}
	if err := r.svc.Prompt(conv.ID, "again"); err != nil {
		t.Fatal(err)
	}
	started(r.fake.Last())
	if got := r.attention(conv.ID); got != model.AttentionRunning {
		t.Errorf("after a new start: %q", got)
	}
}

func TestReadingNeverResolvesARequest(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	id := rt.Ask(model.RequestQuestion, nil)
	r.readAll(conv.ID)
	view := r.view(conv.ID)
	if view.Attention != model.AttentionRequest || len(view.Pending) != 1 || view.Pending[0].ID != id {
		t.Errorf("after reading: %q, pending %+v", view.Attention, view.Pending)
	}
}

func TestReadNeverMovesBackOrPastTheEnd(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	finishTurn(r.fake.Last(), "a-1", "done")
	last := r.view(conv.ID).LastSeq
	if err := r.svc.Read(conv.ID, last+100); err != nil {
		t.Fatal(err)
	}
	if got := r.view(conv.ID).ReadSeq; got != last {
		t.Errorf("read past the end: %d, last %d", got, last)
	}
	if err := r.svc.Read(conv.ID, 1); err != nil {
		t.Fatal(err)
	}
	if got := r.view(conv.ID).ReadSeq; got != last {
		t.Errorf("read moved back to %d", got)
	}
	wantCode(t, r.svc.Read(conv.ID, -1), CodeInvalid)
}

func TestNeedsYouListsRequestsFirstThenOldestFirst(t *testing.T) {
	r := newRig(t)
	older := r.start("older unread")
	finishTurn(r.fake.Last(), "a-1", "first")
	time.Sleep(2 * time.Millisecond)
	asking := r.start("asking")
	time.Sleep(2 * time.Millisecond)
	newer := r.start("newer unread")
	finishTurn(r.fake.Last(), "a-2", "second")
	time.Sleep(2 * time.Millisecond)
	r.fake.Runtimes()[1].Ask(model.RequestPermission, nil)
	quiet := r.start("read")
	finishTurn(r.fake.Last(), "a-3", "third")
	r.readAll(quiet.ID)

	got := r.svc.Workspace().NeedsYou
	want := []string{asking.ID, older.ID, newer.ID}
	if !slices.Equal(got, want) {
		t.Errorf("needs you = %v, want %v", got, want)
	}
}

// after an unclean restart the orphaned runtime may have kept working where
// nobody saw it: what resume imports is unread and lands in needs-you.
func TestResumeAfterUnsupervisedMarksImportedOutputUnread(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	finishTurn(r.fake.Last(), "a-1", "done")
	r.readAll(conv.ID)
	r.restart()
	r.fake.SetHistory(conv.NativeID, []*model.Event{
		{Kind: model.EventAssistant, Text: "done", NativeID: "a-1"},
		{Kind: model.EventAssistant, Text: "finished unobserved", NativeID: "o-1"},
	})
	if err := r.svc.Resume(conv.ID); err != nil {
		t.Fatal(err)
	}
	view := r.view(conv.ID)
	if view.Attention != model.AttentionUnread {
		t.Fatalf("after resuming an unsupervised conversation: %q", view.Attention)
	}
	if needs := r.svc.Workspace().NeedsYou; !slices.Equal(needs, []string{conv.ID}) {
		t.Errorf("needs you = %v", needs)
	}
	r.readAll(conv.ID)
	if got := r.attention(conv.ID); got != model.AttentionIdle {
		t.Errorf("after reading: %q", got)
	}
}

func TestAdoptedHistoryIsNotUnread(t *testing.T) {
	r := newRig(t)
	r.fake.SetSessions([]harness.Session{{NativeID: "terminal-1", Preview: "from the terminal"}})
	r.fake.SetHistory("terminal-1", []*model.Event{
		{Kind: model.EventUser, Text: "from the terminal", NativeID: "t-1"},
		{Kind: model.EventAssistant, Text: "reply", NativeID: "t-2"},
	})
	conv, err := r.svc.Adopt(r.project.ID, model.HarnessClaude, "terminal-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := r.attention(conv.ID); got != model.AttentionIdle {
		t.Errorf("after adoption: %q", got)
	}
}

// crashUnfinished leaves a conversation whose second turn produced output the
// service cached but whose turn never ended, then restarts uncleanly. readAfter
// reads the conversation after that output arrived, before the crash.
func crashUnfinished(t *testing.T, readAfter bool) (*rig, string) {
	t.Helper()
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	finishTurn(rt, "a-1", "done")
	r.readAll(conv.ID)
	if err := r.svc.Prompt(conv.ID, "two"); err != nil {
		t.Fatal(err)
	}
	rt.Emit(&model.Event{Kind: model.EventAssistant, Text: "unfinished", NativeID: "a-2"})
	if readAfter {
		r.readAll(conv.ID)
	}
	r.restart()
	r.fake.SetHistory(conv.NativeID, []*model.Event{
		{Kind: model.EventAssistant, Text: "done", NativeID: "a-1"},
		{Kind: model.EventAssistant, Text: "unfinished", NativeID: "a-2"},
	})
	if err := r.svc.Resume(conv.ID); err != nil {
		t.Fatal(err)
	}
	return r, conv.ID
}

func TestCachedOutputOfTheUnfinishedTurnIsUnreadAfterResume(t *testing.T) {
	r, id := crashUnfinished(t, false)
	if got := r.attention(id); got != model.AttentionUnread {
		t.Errorf("cached output the turn never finished, not read: %q", got)
	}
}

func TestCachedOutputAlreadyReadStaysReadAfterResume(t *testing.T) {
	r, id := crashUnfinished(t, true)
	if got := r.attention(id); got != model.AttentionIdle {
		t.Errorf("cached output already read: %q", got)
	}
}

func TestOutputOfATurnStillInProgressDoesNotCount(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	finishTurn(r.fake.Last(), "a-1", "done")
	r.readAll(conv.ID)
	r.restart()
	r.fake.SetHistory(conv.NativeID, []*model.Event{
		{Kind: model.EventAssistant, Text: "done", NativeID: "a-1"},
		{Kind: model.EventUser, Text: "next", NativeID: "u-2"},
		{Kind: model.EventAssistant, Text: "still going", NativeID: "o-1"},
	})
	r.fake.SetOpen(conv.NativeID, "u-2", "o-1")
	if err := r.svc.Resume(conv.ID); err != nil {
		t.Fatal(err)
	}
	if got := r.attention(conv.ID); got != model.AttentionIdle {
		t.Errorf("output of an in-progress native turn counted: %q", got)
	}
}

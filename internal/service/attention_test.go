package service

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
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

// after Close lets go of the live state, the HTTP server still answers reads
// until it stops, so the page that asked for a shutdown gets its answer. a
// read then must still be computed from the facts last confirmed: a page
// holding an empty attention broke when it went stale, and one holding idle
// for an error would be told something untrue.
func TestAttentionAfterCloseIsTheLastConfirmed(t *testing.T) {
	r := newRig(t)
	unread := r.start("one")
	finishTurn(r.fake.Last(), "a-1", "done")
	read := r.start("two")
	finishTurn(r.fake.Last(), "b-1", "done")
	r.readAll(read.ID)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r.svc.Shutdown(ctx)
	before := map[string]ConversationView{}
	for _, view := range r.svc.Workspace().Conversations {
		before[view.ID] = view
	}
	r.svc.Close()
	got := map[string]string{}
	for _, view := range r.svc.Workspace().Conversations {
		got[view.ID] = view.Attention
		if was := before[view.ID]; view.LastSeq != was.LastSeq || view.State != was.State || len(view.Pending) != len(was.Pending) {
			t.Errorf("%s after close: last %d state %q pending %d, before: %d %q %d", view.ID, view.LastSeq, view.State, len(view.Pending), was.LastSeq, was.State, len(was.Pending))
		}
	}
	if got[unread.ID] != model.AttentionUnread || got[read.ID] != model.AttentionIdle {
		t.Errorf("after close: unread %q, read %q", got[unread.ID], got[read.ID])
	}
}

func TestAnErrorStaysAnErrorAfterClose(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	finishTurn(r.fake.Last(), "a-1", "done")
	if err := r.svc.Prompt(conv.ID, "again"); err != nil {
		t.Fatal(err)
	}
	r.fake.Last().Exit(nil)
	if got := r.attention(conv.ID); got != model.AttentionError {
		t.Fatalf("exited mid-turn: %q", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r.svc.Shutdown(ctx)
	r.svc.Close()
	for _, view := range r.svc.Workspace().Conversations {
		if view.ID == conv.ID && view.Attention != model.AttentionError {
			t.Errorf("after close: %q, want error", view.Attention)
		}
	}
}

// once Close has closed the logs, a history read or a read acknowledgment is
// refused rather than opening a log again from disk.
func TestNoLogIsReopenedAfterClose(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	finishTurn(r.fake.Last(), "a-1", "done")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r.svc.Shutdown(ctx)
	last := r.view(conv.ID).LastSeq
	r.svc.Close()
	if _, err := r.svc.Events(conv.ID, 0); ErrorCode(err) != CodeUnavailable {
		t.Errorf("events after close: %v", err)
	}
	if err := r.svc.Read(conv.ID, last); ErrorCode(err) != CodeUnavailable {
		t.Errorf("read after close: %v", err)
	}
	r.svc.mu.Lock()
	reopened := len(r.svc.live)
	r.svc.mu.Unlock()
	if reopened != 0 {
		t.Errorf("%d logs reopened after close", reopened)
	}
	if view := r.view(conv.ID); view.LastSeq != last {
		t.Errorf("the view after close: last %d, want %d", view.LastSeq, last)
	}
}

// a conversation whose log could not be opened has no live state at all; its
// attention is what the registry says.
func TestRecordedAttention(t *testing.T) {
	cases := []struct {
		conv model.Conversation
		want string
	}{
		{model.Conversation{Runtime: model.RuntimeUnsupervised, OutputSeq: 5, ReadSeq: 1}, model.AttentionUnsupervised},
		{model.Conversation{OutputSeq: 5, ReadSeq: 1}, model.AttentionUnread},
		{model.Conversation{OutputSeq: 5, ReadSeq: 5}, model.AttentionIdle},
	}
	for _, c := range cases {
		if got := recordedAttention(c.conv); got != c.want {
			t.Errorf("%+v: %q, want %q", c.conv, got, c.want)
		}
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

func TestImportFromOutsideMegaIsNotUnread(t *testing.T) {
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

// native events are the harness's own bookkeeping and never count as agent
// output: a turn of init, status and lifecycle frames leaves the unread marker
// where it was, and one assistant event still moves it.
func TestNativeEventsNeverCountAsOutput(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	finishTurn(rt, "a-1", "done")
	r.readAll(conv.ID)
	before := r.view(conv.ID).OutputSeq

	if err := r.svc.Prompt(conv.ID, "two"); err != nil {
		t.Fatal(err)
	}
	for i, text := range []string{"system/init", "system/status", "system/session_state_changed"} {
		rt.Emit(&model.Event{Kind: model.EventNative, Text: text, NativeID: fmt.Sprintf("n-%d", i)})
	}
	rt.Emit(&model.Event{Kind: model.EventTurnEnd, Status: model.TurnSuccess})
	if view := r.view(conv.ID); view.OutputSeq != before || view.Attention != model.AttentionIdle {
		t.Errorf("a bookkeeping-only turn: output seq %d -> %d, attention %q", before, view.OutputSeq, view.Attention)
	}

	if err := r.svc.Prompt(conv.ID, "three"); err != nil {
		t.Fatal(err)
	}
	rt.Emit(&model.Event{Kind: model.EventNative, Text: "system/init", NativeID: "n-3"})
	rt.Emit(&model.Event{Kind: model.EventAssistant, Text: "reply", NativeID: "a-3"})
	rt.Emit(&model.Event{Kind: model.EventNative, Text: "system/status", NativeID: "n-4"})
	rt.Emit(&model.Event{Kind: model.EventTurnEnd, Status: model.TurnSuccess})
	evs := r.events(conv.ID)
	var replySeq int64
	for _, ev := range evs {
		if ev.NativeID == "a-3" {
			replySeq = ev.Seq
		}
	}
	if view := r.view(conv.ID); view.OutputSeq != replySeq || view.Attention != model.AttentionUnread {
		t.Errorf("a turn with one assistant event: output seq %d (want %d), attention %q", view.OutputSeq, replySeq, view.Attention)
	}
}

// after an unsupervised resume the marker is the newest assistant or tool
// event, never a native one recorded after it.
func TestResumeMarkerIgnoresNativeEvents(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	finishTurn(r.fake.Last(), "a-1", "done")
	r.readAll(conv.ID)
	r.restart()
	r.fake.SetHistory(conv.NativeID, []*model.Event{
		{Kind: model.EventAssistant, Text: "done", NativeID: "a-1"},
		{Kind: model.EventToolUse, Text: "Bash ls", Tool: "Bash", ToolUseID: "tu-1", NativeID: "u-1"},
		{Kind: model.EventToolResult, Text: "file", ToolUseID: "tu-1", NativeID: "r-1"},
		{Kind: model.EventNative, Text: "system/init", NativeID: "n-1"},
		{Kind: model.EventNative, Text: "system/status", NativeID: "n-2"},
	})
	if err := r.svc.Resume(conv.ID); err != nil {
		t.Fatal(err)
	}
	var resultSeq, nativeSeq int64
	for _, ev := range r.events(conv.ID) {
		switch ev.NativeID {
		case "r-1":
			resultSeq = ev.Seq
		case "n-2":
			nativeSeq = ev.Seq
		}
	}
	view := r.view(conv.ID)
	if view.OutputSeq != resultSeq || nativeSeq <= resultSeq {
		t.Errorf("marker = %d, want the tool result's %d (newest native at %d)", view.OutputSeq, resultSeq, nativeSeq)
	}
	if view.Attention != model.AttentionUnread {
		t.Errorf("attention = %q", view.Attention)
	}
}

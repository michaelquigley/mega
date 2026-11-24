package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
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
	if err := r.svc.Answer(conv.ID, id, harness.Answer{Decision: harness.DecisionDeny}); err != nil {
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

func TestLegacyImportFailureDoesNotChangeRuntimeAttention(t *testing.T) {
	for _, c := range []struct {
		name      string
		exitError error
		want      string
	}{
		{"failed import alone", nil, model.AttentionIdle},
		{"failed import after runtime failure", errors.New("the process died"), model.AttentionError},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t)
			conv := r.start("one")
			rt := r.fake.Last()
			started(rt)
			finishTurn(rt, "a-1", "done")
			r.readAll(conv.ID)
			rt.Exit(c.exitError)
			if got := r.attention(conv.ID); got != c.want {
				t.Fatalf("before importing: attention = %q, want %q", got, c.want)
			}

			// Append the rows the old service wrote for a failed import.
			r.svc.mu.Lock()
			l := r.svc.live[conv.ID]
			startErr := l.log.Append(&model.Event{Kind: model.EventRuntime, Origin: model.OriginService, Status: model.StatusResuming})
			failErr := l.log.Append(&model.Event{Kind: model.EventRuntime, Origin: model.OriginService, Status: model.StatusError, Text: "native history could not be read: session missing"})
			r.svc.mu.Unlock()
			if err := errors.Join(startErr, failErr); err != nil {
				t.Fatal(err)
			}
			before := r.events(conv.ID)

			r.restart()
			if got := r.attention(conv.ID); got != c.want {
				t.Errorf("after replay: attention = %q, want %q", got, c.want)
			}
			if after := r.events(conv.ID); !reflect.DeepEqual(before, after) {
				t.Errorf("replay changed the history: got %v, want %v", kinds(after), kinds(before))
			}
		})
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

	got := workspaceOf(t, r.svc).NeedsYou
	want := []string{asking.ID, older.ID, newer.ID}
	if !slices.Equal(got, want) {
		t.Errorf("needs you = %v, want %v", got, want)
	}
}

func TestUnwitnessedOutputDoesNotBecomeUnreadWhenSupervisionIsAcknowledged(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	finishTurn(r.fake.Last(), "a-1", "done")
	r.readAll(conv.ID)
	r.restart()
	r.fake.SetHistory(conv.NativeID, []*model.Event{{Kind: model.EventAssistant, Text: "finished unobserved", NativeID: "o-1"}})
	if err := r.svc.Resume(conv.ID); err != nil {
		t.Fatal(err)
	}
	if got := r.attention(conv.ID); got != model.AttentionIdle || len(r.fake.Reads()) != 0 {
		t.Fatalf("attention = %q, history reads = %v", got, r.fake.Reads())
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

func TestCachedOutputOfTheUnfinishedTurnStaysInHistoryWithoutNewUnread(t *testing.T) {
	r, id := crashUnfinished(t, false)
	if got := r.attention(id); got != model.AttentionIdle {
		t.Errorf("cached output of an unfinished turn changed attention: %q", got)
	}
	if !slices.ContainsFunc(r.events(id), func(ev *model.Event) bool { return ev.Text == "unfinished" }) {
		t.Error("the recorded reply disappeared")
	}
}

func TestCachedOutputAlreadyReadStaysReadAfterResume(t *testing.T) {
	r, id := crashUnfinished(t, true)
	if got := r.attention(id); got != model.AttentionIdle {
		t.Errorf("cached output already read: %q", got)
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

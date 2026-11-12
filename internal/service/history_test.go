package service

import (
	"fmt"
	"testing"

	"github.com/michaelquigley/mega/internal/harness/harnesstest"
	"github.com/michaelquigley/mega/internal/model"
)

// toolTurn plays one turn after its prompt: an init, tool calls with native
// frames between each call and its result, a request on one of them, a reply
// and the turn's end.
func toolTurn(t *testing.T, r *rig, rt *harnesstest.Runtime, convID string, turn int) {
	t.Helper()
	rt.Emit(&model.Event{Kind: model.EventNative, Text: "system/init"})
	for call := range turn%3 + 2 {
		id := fmt.Sprintf("call-%d-%d", turn, call)
		rt.Emit(&model.Event{Kind: model.EventToolUse, ToolUseID: id, Text: "Bash " + id})
		rt.Emit(&model.Event{Kind: model.EventNative, Text: "stream_event"})
		rt.Emit(&model.Event{Kind: model.EventNative, Text: "system/status"})
		rt.Emit(&model.Event{Kind: model.EventToolResult, ToolUseID: id, Text: "ok " + id})
	}
	finishTurn(rt, fmt.Sprintf("reply-%d", turn), fmt.Sprintf("reply %d", turn))
	waitFor(t, "the turn to end", func() bool { return !r.view(convID).TurnActive })
}

func TestTheTailStartsAtATurnAndKeepsEveryCallWithItsResult(t *testing.T) {
	r := newRig(t)
	conv := r.start("turn 0")
	rt := r.fake.Last()
	started(rt)
	toolTurn(t, r, rt, conv.ID, 0)
	for turn := 1; turn < 15; turn++ {
		if err := r.svc.Prompt(conv.ID, fmt.Sprintf("turn %d", turn)); err != nil {
			t.Fatal(err)
		}
		toolTurn(t, r, rt, conv.ID, turn)
	}
	// a turn in flight, with a call still waiting for its result.
	if err := r.svc.Prompt(conv.ID, "in flight"); err != nil {
		t.Fatal(err)
	}
	rt.Emit(&model.Event{Kind: model.EventToolUse, ToolUseID: "open", Text: "Bash open"})
	waitFor(t, "the open call to be recorded", func() bool { return r.events(conv.ID)[len(r.events(conv.ID))-1].ToolUseID == "open" })

	all := r.events(conv.ID)
	tail, err := r.svc.History(conv.ID, HistoryQuery{Turns: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !tail.Windowed || tail.Through != all[len(all)-1].Seq {
		t.Errorf("the tail covers through %d (windowed %t), want the newest event %d", tail.Through, tail.Windowed, all[len(all)-1].Seq)
	}
	// it starts at a turn's first event: right after a turn's end.
	if prev := all[tail.From-2]; prev.Kind != model.EventTurnEnd {
		t.Errorf("the tail starts at %d, after a %s, not after a turn's end", tail.From, prev.Kind)
	}
	var prompts []string
	calls := map[string]bool{}
	for _, ev := range tail.Events {
		switch ev.Kind {
		case model.EventNative:
			t.Errorf("a native event %d was sent", ev.Seq)
		case model.EventUser:
			prompts = append(prompts, ev.Text)
		case model.EventToolUse:
			calls[ev.ToolUseID] = true
		case model.EventToolResult:
			if !calls[ev.ToolUseID] {
				t.Errorf("result %d of %s came without its call", ev.Seq, ev.ToolUseID)
			}
		}
	}
	want := []string{"turn 5", "turn 6", "turn 7", "turn 8", "turn 9", "turn 10", "turn 11", "turn 12", "turn 13", "turn 14", "in flight"}
	if fmt.Sprint(prompts) != fmt.Sprint(want) {
		t.Errorf("the tail's prompts are %v, want the last ten turns and the one in flight", prompts)
	}
	if tail.EarlierTurns != 5 || tail.EarlierEvents() != tail.From-1 {
		t.Errorf("earlier: %d turns, %d events; want 5 turns and %d events", tail.EarlierTurns, tail.EarlierEvents(), tail.From-1)
	}

	// with native events asked for, the same stretch, whole.
	withNative, err := r.svc.History(conv.ID, HistoryQuery{Turns: 10, Native: true})
	if err != nil {
		t.Fatal(err)
	}
	if withNative.From != tail.From || len(withNative.Events) != int(tail.Through-tail.From+1) {
		t.Errorf("with native events: from %d, %d events; want from %d and every one of %d", withNative.From, len(withNative.Events), tail.From, tail.Through-tail.From+1)
	}

	// the turns before it, and everything before it.
	page, err := r.svc.History(conv.ID, HistoryQuery{Turns: 3, Before: tail.From})
	if err != nil {
		t.Fatal(err)
	}
	if got := promptsOf(page.Events); page.Through != tail.From-1 || page.EarlierTurns != 2 || fmt.Sprint(got) != "[turn 2 turn 3 turn 4]" {
		t.Errorf("the page before: through %d, %d earlier turns, prompts %v", page.Through, page.EarlierTurns, got)
	}
	rest, err := r.svc.History(conv.ID, HistoryQuery{Before: tail.From, Native: true})
	if err != nil {
		t.Fatal(err)
	}
	if rest.From != 1 || len(rest.Events) != int(tail.From-1) || rest.EarlierTurns != 0 {
		t.Errorf("all before: from %d, %d events, %d earlier turns; want everything before %d", rest.From, len(rest.Events), rest.EarlierTurns, tail.From)
	}

	// a read that returns nothing new.
	nothing, err := r.svc.History(conv.ID, HistoryQuery{Since: tail.Through})
	if err != nil {
		t.Fatal(err)
	}
	if len(nothing.Events) != 0 || nothing.From != tail.Through+1 || nothing.Through != tail.Through {
		t.Errorf("since the newest: %d events, from %d through %d", len(nothing.Events), nothing.From, nothing.Through)
	}
}

func TestReadingWhatATailCoversClearsUnread(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	toolTurn(t, r, rt, conv.ID, 0)
	// a native event after the turn: the newest event the tail covers but
	// does not send.
	rt.Emit(&model.Event{Kind: model.EventNative, Text: "system/status"})
	waitFor(t, "the native event", func() bool { return r.events(conv.ID)[len(r.events(conv.ID))-1].Kind == model.EventNative })
	if a := r.view(conv.ID).Attention; a != model.AttentionUnread {
		t.Fatalf("attention before reading is %q", a)
	}
	tail, err := r.svc.History(conv.ID, HistoryQuery{Turns: 10})
	if err != nil {
		t.Fatal(err)
	}
	if last := tail.Events[len(tail.Events)-1]; last.Seq == tail.Through {
		t.Fatalf("the newest event %d was sent; the test wants it left out", last.Seq)
	}
	if err := r.svc.Read(conv.ID, tail.Through); err != nil {
		t.Fatal(err)
	}
	if v := r.view(conv.ID); v.Attention != model.AttentionIdle || v.ReadSeq != tail.Through {
		t.Errorf("after reading through %d: attention %q, read_seq %d", tail.Through, v.Attention, v.ReadSeq)
	}
}

func promptsOf(evs []*model.Event) []string {
	var out []string
	for _, ev := range evs {
		if ev.Kind == model.EventUser {
			out = append(out, ev.Text)
		}
	}
	return out
}

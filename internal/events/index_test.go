package events

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/michaelquigley/mega/internal/model"
)

// turnLog appends turns of varying length to log, each a runtime line or two
// between turns, a prompt, tool calls with native frames between call and
// result, a reply and a turn_end, and returns where each turn starts.
func turnLog(t *testing.T, log *Log, turns int, seed int) []int64 {
	t.Helper()
	var starts []int64
	add := func(ev *model.Event) int64 {
		t.Helper()
		if ev.Origin == "" {
			ev.Origin = model.OriginManaged
		}
		if err := log.Append(ev); err != nil {
			t.Fatal(err)
		}
		return ev.Seq
	}
	for turn := range turns {
		n := seed + turn
		first := add(&model.Event{Kind: model.EventRuntime, Origin: model.OriginService, Status: model.StatusStarting})
		if n%3 == 0 {
			add(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted})
		}
		starts = append(starts, first)
		add(&model.Event{Kind: model.EventUser, Text: fmt.Sprintf("prompt %d", n)})
		for call := range n%5 + 1 {
			id := fmt.Sprintf("call-%d-%d", n, call)
			add(&model.Event{Kind: model.EventToolUse, ToolUseID: id, Text: id})
			for range call + n%4 {
				add(&model.Event{Kind: model.EventNative, Text: "frame"})
			}
			add(&model.Event{Kind: model.EventToolResult, ToolUseID: id, Text: "result " + id})
		}
		add(&model.Event{Kind: model.EventAssistant, Text: fmt.Sprintf("reply %d", n)})
		add(&model.Event{Kind: model.EventTurnEnd, Status: model.TurnSuccess})
	}
	return starts
}

func turns(log *Log, seq int64, n int) (int64, int) {
	from, before, _ := log.TurnsBefore(seq, n)
	return from, before
}

func seqs(evs []*model.Event) []int64 {
	out := []int64{}
	for _, ev := range evs {
		out = append(out, ev.Seq)
	}
	return out
}

// wantRange is what a read from from through to keeping keep must return,
// worked out from the whole log rather than the index.
func wantRange(t *testing.T, log *Log, from, to int64, keep func(*model.Event) bool) []int64 {
	t.Helper()
	all, err := log.Range(1, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := []int64{}
	for _, ev := range all {
		if ev.Seq >= from && (to <= 0 || ev.Seq <= to) && (keep == nil || keep(ev)) {
			out = append(out, ev.Seq)
		}
	}
	return out
}

func notNative(ev *model.Event) bool { return ev.Kind != model.EventNative }

func TestTheIndexSeeksAcrossManyPoints(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log := openLog(t, path)
	turnLog(t, log, 120, 0)
	last := log.LastSeq()
	if len(log.index.points) < 4 {
		t.Fatalf("the log crosses %d seek points; the test wants several", len(log.index.points))
	}
	for _, tc := range []struct{ from, to int64 }{
		{1, 0}, {2, 9}, {pointEvery - 1, pointEvery + 1}, {pointEvery, 0}, {pointEvery + 1, 3 * pointEvery},
		{3*pointEvery + 7, 3*pointEvery + 7}, {last, 0}, {last + 1, 0}, {last - 40, last - 2},
	} {
		for _, keep := range []func(*model.Event) bool{nil, notNative} {
			got, err := log.Range(tc.from, tc.to, keep)
			if err != nil {
				t.Fatal(err)
			}
			if want := wantRange(t, log, tc.from, tc.to, keep); !reflect.DeepEqual(seqs(got), want) {
				t.Errorf("range %d..%d (filtered %t): %v, want %v", tc.from, tc.to, keep != nil, seqs(got), want)
			}
		}
	}
	// a read after a given event, the read a reconnect makes.
	for _, since := range []int64{0, 1, pointEvery, 2*pointEvery + 3, last - 1, last} {
		got, err := log.Since(since)
		if err != nil {
			t.Fatal(err)
		}
		if want := wantRange(t, log, since+1, 0, nil); !reflect.DeepEqual(seqs(got), want) {
			t.Errorf("since %d: %d events, want %d", since, len(got), len(want))
		}
	}
}

func TestTheTailIsTheLastTurnsAndTheTurnInFlight(t *testing.T) {
	log := openLog(t, filepath.Join(t.TempDir(), "events.jsonl"))
	starts := turnLog(t, log, 60, 1)

	// every turn has ended: the last ten are the tail.
	if from, before := turns(log, 0, 10); from != starts[50] || before != 50 {
		t.Errorf("tail of ended turns starts at %d after %d turns, want %d after 50", from, before, starts[50])
	}
	// a page before a given event: the ten turns before the tail's first.
	if from, before := turns(log, starts[50], 10); from != starts[40] || before != 40 {
		t.Errorf("the page before %d starts at %d after %d turns, want %d after 40", starts[50], from, before, starts[40])
	}
	if from, before := turns(log, starts[5], 10); from != starts[0] || before != 0 {
		t.Errorf("a page near the start starts at %d after %d, want the first event", from, before)
	}
	if from, before := turns(log, starts[50], 0); from != 1 || before != 0 {
		t.Errorf("all before %d starts at %d after %d, want 1 after 0", starts[50], from, before)
	}
	if from, _ := turns(log, 0, 1000); from != 1 {
		t.Errorf("more turns than held starts at %d, want 1", from)
	}

	// a turn in flight is always in the tail and not one of the ten.
	for _, ev := range []*model.Event{
		{Kind: model.EventRuntime, Origin: model.OriginService, Status: model.StatusStarting},
		{Kind: model.EventUser, Origin: model.OriginManaged, Text: "in flight"},
		{Kind: model.EventToolUse, Origin: model.OriginManaged, ToolUseID: "open", Text: "open"},
	} {
		if err := log.Append(ev); err != nil {
			t.Fatal(err)
		}
	}
	if from, before := turns(log, 0, 10); from != starts[50] || before != 50 {
		t.Errorf("tail with a turn in flight starts at %d after %d, want %d after 50", from, before, starts[50])
	}
}

func TestEventsBetweenTurnsTravelWithTheNextTurn(t *testing.T) {
	log := openLog(t, filepath.Join(t.TempDir(), "events.jsonl"))
	turnLog(t, log, 1, 0)
	gap := []*model.Event{
		{Kind: model.EventRuntime, Origin: model.OriginService, Status: model.StatusStopping},
		{Kind: model.EventRuntime, Origin: model.OriginService, Status: model.StatusStopped},
		{Kind: model.EventNative, Origin: model.OriginManaged, Text: "init"},
		{Kind: model.EventUser, Origin: model.OriginManaged, Text: "next"},
		{Kind: model.EventAssistant, Origin: model.OriginManaged, Text: "reply"},
		{Kind: model.EventTurnEnd, Origin: model.OriginManaged, Status: model.TurnSuccess},
	}
	for _, ev := range gap {
		if err := log.Append(ev); err != nil {
			t.Fatal(err)
		}
	}
	if from, before := turns(log, 0, 1); from != gap[0].Seq || before != 1 {
		t.Errorf("the last turn starts at %d after %d, want the stopping line %d after 1", from, before, gap[0].Seq)
	}
}

func TestImportedPromptsStartTurnsWithoutSplittingACall(t *testing.T) {
	log := openLog(t, filepath.Join(t.TempDir(), "events.jsonl"))
	add := func(kind, text, tool string) int64 {
		t.Helper()
		ev := &model.Event{Kind: kind, Origin: model.OriginNative, Text: text, ToolUseID: tool}
		if err := log.Append(ev); err != nil {
			t.Fatal(err)
		}
		return ev.Seq
	}
	one := add(model.EventUser, "one", "")
	add(model.EventAssistant, "reply one", "")
	twoLead := add(model.EventNative, "frame between", "")
	add(model.EventUser, "two", "")
	add(model.EventToolUse, "call", "c1")
	// a prompt while the call waits for its result does not start a turn.
	add(model.EventUser, "queued", "")
	add(model.EventToolResult, "result", "c1")
	add(model.EventAssistant, "reply two", "")
	three := add(model.EventUser, "three", "")
	// a managed prompt with no turn_end before it, after a runtime that died
	// mid-turn, does not start one: managed turns end with turn_end.
	if err := log.Append(&model.Event{Kind: model.EventUser, Origin: model.OriginManaged, Text: "managed"}); err != nil {
		t.Fatal(err)
	}
	if got, want := marks(log.index.turns), []int64{one, twoLead, three}; !reflect.DeepEqual(got, want) {
		t.Errorf("turns start at %v, want %v", got, want)
	}
}

func marks(ms []mark) []int64 {
	out := []int64{}
	for _, m := range ms {
		out = append(out, m.seq)
	}
	return out
}

func TestTheIndexIsRebuiltOnOpenAndKeptByAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	turnLog(t, log, 70, 2)
	built := log.index
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := openLog(t, path)
	if !reflect.DeepEqual(reopened.index.points, built.points) || !reflect.DeepEqual(reopened.index.turns, built.turns) {
		t.Fatal("the index rebuilt on open differs from the one appends kept")
	}
	starts := turnLog(t, reopened, 30, 3)
	if from, before := turns(reopened, 0, 10); from != starts[20] || before != 90 {
		t.Errorf("after appends to a reopened log the tail starts at %d after %d, want %d after 90", from, before, starts[20])
	}
	last := reopened.LastSeq()
	got, err := reopened.Range(last-300, last-250, notNative)
	if err != nil {
		t.Fatal(err)
	}
	if want := wantRange(t, reopened, last-300, last-250, notNative); !reflect.DeepEqual(seqs(got), want) {
		t.Errorf("a read across the reopen: %v, want %v", seqs(got), want)
	}
}

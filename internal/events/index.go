package events

import (
	"sort"

	"github.com/michaelquigley/mega/internal/model"
)

// pointEvery is how many events lie between two seek points: a read seeks to
// the nearest point at or before where it starts and parses at most this many
// events it does not return.
const pointEvery = 256

// mark is where an event begins: its seq and the offset of its line.
type mark struct {
	seq    int64
	offset int64
}

// index is the log's in-memory map, built by the scan Open makes and kept as
// events are appended; it is never written. it holds a seek point every
// pointEvery events and where each turn starts.
//
// a turn starts at the log's first event and at the event after each
// turn_end, so the events between two turns (runtime lines, an init) travel
// with the turn that follows them. history imported from a harness carries no
// turn_end, so a prompt read from native history starts a turn too, once the
// turn before it holds anything and no tool call in it waits for its result.
// it then starts at the first event after that turn's last prompt, reply,
// tool call or request, for the same reason. a tool call is never split from
// its result: an imported call left without one keeps its turn open, which
// makes that turn longer, never wrong.
type index struct {
	points []mark
	turns  []mark

	// ended: the newest event is a turn_end, so the next event starts a turn.
	ended bool
	// held: the newest turn holds a prompt, reply, tool call or request.
	held bool
	// calls: the newest turn's tool calls still waiting for their results.
	calls map[string]bool
	// lead: the first event since the newest turn's last content, where a
	// turn started by an imported prompt begins. zero seq while none.
	lead mark
}

// note adds an event that begins at offset.
func (x *index) note(ev *model.Event, offset int64) {
	at := mark{seq: ev.Seq, offset: offset}
	if len(x.points) == 0 || ev.Seq-x.points[len(x.points)-1].seq >= pointEvery {
		x.points = append(x.points, at)
	}
	if x.lead.seq == 0 {
		x.lead = at
	}
	switch {
	case len(x.turns) == 0 || x.ended:
		x.startTurn(at)
	case ev.Kind == model.EventUser && ev.Origin == model.OriginNative && x.held && len(x.calls) == 0:
		x.startTurn(x.lead)
	}
	x.ended = ev.Kind == model.EventTurnEnd
	switch ev.Kind {
	case model.EventToolUse:
		if ev.ToolUseID != "" {
			if x.calls == nil {
				x.calls = make(map[string]bool)
			}
			x.calls[ev.ToolUseID] = true
		}
	case model.EventToolResult:
		delete(x.calls, ev.ToolUseID)
	case model.EventTurnEnd:
		// a call the turn ended without answering, an interrupt's, is over.
		x.calls = nil
	}
	switch ev.Kind {
	case model.EventUser, model.EventAssistant, model.EventToolUse, model.EventToolResult, model.EventRequest, model.EventRequestResolved:
		x.held = true
		x.lead = mark{}
	}
}

func (x *index) startTurn(at mark) {
	x.turns = append(x.turns, at)
	x.held = false
}

// seek returns the offset to start reading at to reach seq: the nearest seek
// point or turn start at or before it.
func (x *index) seek(seq int64) int64 {
	var offset int64
	for _, marks := range [][]mark{x.points, x.turns} {
		i := sort.Search(len(marks), func(i int) bool { return marks[i].seq > seq })
		if i > 0 && marks[i-1].offset > offset {
			offset = marks[i-1].offset
		}
	}
	return offset
}

// turnsBefore returns the start of the n-th turn before seq, and how many
// turns start before it. seq zero means the end of the log: there, a newest
// turn that has not ended (the turn in flight, or the events after a turn
// that open the next) is not counted among the n, and is always included. n
// of zero or less reaches the first event.
func (x *index) turnsBefore(seq int64, n int) (int64, int) {
	if len(x.turns) == 0 {
		return 1, 0
	}
	count := len(x.turns)
	if seq > 0 {
		count = sort.Search(len(x.turns), func(i int) bool { return x.turns[i].seq >= seq })
	} else if !x.ended {
		count--
	}
	first := 0
	if n > 0 {
		first = max(count-n, 0)
	}
	return x.turns[first].seq, first
}

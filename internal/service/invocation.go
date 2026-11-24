package service

import (
	"fmt"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

// Invocation is what one recorded action invoked, for its expanded row.
// Recorded says whether the adapter could read it from the event; an action
// whose event holds no payload it can read has Summary, the line it was
// recorded with, and nothing more.
type Invocation struct {
	harness.Invocation
	Recorded bool
	Summary  string
}

// Invocation reads what the action at seq invoked. the event is read from the
// log, from the index's nearest seek point at or before it, so a read parses
// at most one stretch between points (256 events); nothing is stored for it
// beyond the event itself. the conversation's adapter reads the payload,
// since what it holds is the harness's own protocol.
func (s *Service) Invocation(conversationID string, seq int64) (Invocation, error) {
	if err := s.lockOpen(); err != nil {
		return Invocation{}, err
	}
	conv, err := s.conversationLocked(conversationID)
	if err != nil {
		s.mu.Unlock()
		return Invocation{}, err
	}
	l, err := s.liveLocked(conversationID)
	s.mu.Unlock()
	if err != nil {
		return Invocation{}, err
	}
	evs, err := l.log.Range(seq, seq, nil)
	if err != nil {
		return Invocation{}, err
	}
	if len(evs) != 1 || evs[0].Kind != model.EventToolUse {
		return Invocation{}, newError(CodeNotFound, fmt.Sprintf("conversation '%s' has no action at %d", conversationID, seq))
	}
	ev := evs[0]
	out := Invocation{Summary: ev.Text}
	if invoker, ok := s.harnesses[conv.Harness].(harness.Invoker); ok {
		out.Invocation, out.Recorded = invoker.Invocation(ev)
	}
	if out.Tool == "" {
		out.Tool = ev.Tool
	}
	return out, nil
}

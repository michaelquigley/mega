package service

import (
	"errors"
	"fmt"
	"slices"

	"github.com/michaelquigley/df/dl"

	"git.hq.quigley.com/research/roundhouse/internal/events"
	"git.hq.quigley.com/research/roundhouse/internal/harness"
	"git.hq.quigley.com/research/roundhouse/internal/model"
	"git.hq.quigley.com/research/roundhouse/internal/registry"
)

// transient runtime states the service reports alongside the recorded ones.
const (
	StateStarting = "starting"
	StateRunning  = model.RuntimeRunning
	StateStopping = "stopping"
	StateResuming = "resuming"
)

// ConversationView is a conversation as the service currently knows it: the
// recorded identity plus what lives only in memory, such as the pending set.
type ConversationView struct {
	model.Conversation

	// State is the runtime as the service sees it now: none, starting, running,
	// stopping, resuming, or unsupervised.
	State      string
	TurnActive bool
	Facts      *model.RuntimeFacts
	Pending    []model.Request
}

// live is the in-memory side of one conversation. the pending set lives here
// and nowhere else: it dies with the runtime and is never rebuilt from the log.
//
// two kinds of state live here. state, turnActive, facts and pending are what
// browsers see, and change only once the event behind them is recorded. runtime,
// inc, busy and releasing describe the process and the operations on it, and
// follow the process whether or not the log could record anything.
type live struct {
	id         string
	log        *events.Log
	seen       map[string]bool // native ids already in the log
	runtime    harness.Runtime
	inc        *incarnation // the current runtime's identity; nil when none
	busy       bool         // a start or a resume is under way
	releasing  bool         // the current runtime has been asked to stop
	state      string       // displayed: "", starting, running, stopping or resuming
	turnActive bool
	facts      *model.RuntimeFacts
	pending    []*model.Request
}

// incarnation tells one runtime's callbacks from a later runtime's, so a late
// call from a runtime that has been replaced changes nothing.
type incarnation struct {
	exited chan struct{}
}

// liveLocked returns the conversation's live state, opening its log on first
// use. callers hold s.mu.
func (s *Service) liveLocked(conversationID string) (*live, error) {
	if l := s.live[conversationID]; l != nil {
		return l, nil
	}
	log, err := events.Open(s.logPath(conversationID))
	if err != nil {
		return nil, err
	}
	evs, err := log.Since(0)
	if err != nil {
		log.Close()
		return nil, err
	}
	l := &live{id: conversationID, log: log, seen: make(map[string]bool)}
	for _, ev := range evs {
		if ev.NativeID != "" {
			l.seen[ev.NativeID] = true
		}
	}
	s.live[conversationID] = l
	return l, nil
}

// record appends an event and announces it. an event whose native identity is
// already in the log is skipped, so a message is never recorded twice whether
// it arrives live, echoed, or through an import. callers hold s.mu.
func (s *Service) record(l *live, ev *model.Event) error {
	if ev.NativeID != "" && l.seen[ev.NativeID] {
		return nil
	}
	if ev.Origin == "" {
		ev.Origin = model.OriginService
	}
	if err := l.log.Append(ev); err != nil {
		if errors.Is(err, events.ErrUncertain) {
			return newError(CodeUnavailable, err.Error())
		}
		return fmt.Errorf("recording event for '%s': %w", l.id, err)
	}
	if ev.NativeID != "" {
		l.seen[ev.NativeID] = true
	}
	s.publish(Update{ConversationID: l.id, Event: ev})
	return nil
}

// changed tells subscribers to re-read the workspace after a change that lives
// in memory, such as runtime state or the pending set. callers hold s.mu.
func (s *Service) changed() {
	s.publish(Update{WorkspaceChanged: true})
}

// transition is the only way the displayed runtime state changes. it records
// the runtime event first, and only once that succeeds shows the new state,
// applies apply (which may be nil) and announces the change. an unrecorded
// transition leaves the last confirmed state showing; whatever the caller does
// to the process goes ahead regardless, and a restart settles what the log
// could not record. callers hold s.mu.
func (s *Service) transition(l *live, ev *model.Event, state string, apply func()) error {
	ev.Kind = model.EventRuntime
	if err := s.record(l, ev); err != nil {
		return err
	}
	l.state = state
	if apply != nil {
		apply()
	}
	s.changed()
	return nil
}

// failedLaunch records that no runtime started. like every transition it is
// shown only once recorded: if the log cannot record it, the last confirmed
// starting stays on screen, and the log's sticky refusal of later writes is
// what says a restart is needed. it reports whether the failure was recorded.
// callers hold s.mu.
func (s *Service) failedLaunch(l *live, reason error) bool {
	ev := &model.Event{Status: model.StatusError, Text: fmt.Sprintf("the runtime did not start: %v", reason)}
	if err := s.transition(l, ev, "", nil); err != nil {
		dl.Errorf("%v", err)
		return false
	}
	return true
}

// beginStopLocked asks the current runtime to stop, once: interrupt if work or
// a request is outstanding, then close input. the stopping state is shown only
// if its event is recorded, but the process is stopped either way, because a
// storage fault must never trap a running agent. callers hold s.mu.
func (s *Service) beginStopLocked(l *live, ev *model.Event) {
	if l.runtime == nil || l.releasing {
		return
	}
	l.releasing = true
	if err := s.transition(l, ev, StateStopping, nil); err != nil {
		dl.Errorf("stopping '%s' could not be recorded; stopping it anyway: %v", l.id, err)
	}
	go releaseRuntime(l.runtime, l.turnActive || len(l.pending) > 0)
}

// viewLocked joins a recorded conversation with its live state. callers hold s.mu.
func (s *Service) viewLocked(conv model.Conversation) ConversationView {
	view := ConversationView{Conversation: conv, State: conv.Runtime}
	if view.State == "" {
		view.State = model.RuntimeNone
	}
	l := s.live[conv.ID]
	if l == nil {
		return view
	}
	view.LastSeq = l.log.LastSeq()
	if l.state != "" {
		view.State = l.state
	}
	view.TurnActive = l.turnActive
	view.Facts = l.facts
	view.Pending = make([]model.Request, 0, len(l.pending))
	for _, req := range l.pending {
		view.Pending = append(view.Pending, *req)
	}
	return view
}

// takePending removes a request from the pending set. callers hold s.mu.
func (l *live) takePending(requestID string) (*model.Request, bool) {
	i := slices.IndexFunc(l.pending, func(r *model.Request) bool { return r.ID == requestID })
	if i < 0 {
		return nil, false
	}
	req := l.pending[i]
	l.pending = slices.Delete(l.pending, i, i+1)
	return req, true
}

// resolve records a request's resolution. callers hold s.mu.
func (s *Service) resolve(l *live, req *model.Request, outcome string) error {
	return s.record(l, &model.Event{
		Kind: model.EventRequestResolved,
		Text: outcome,
		Req:  &model.Request{ID: req.ID, Kind: req.Kind, Outcome: outcome},
	})
}

// resolveOrLog records a resolution where no one is waiting on the result.
// callers hold s.mu.
func (s *Service) resolveOrLog(l *live, req *model.Request, outcome string) {
	if err := s.resolve(l, req, outcome); err != nil {
		dl.Errorf("%v", err)
	}
}

// conversationLocked finds a recorded conversation. callers hold s.mu.
func (s *Service) conversationLocked(id string) (model.Conversation, error) {
	for _, conv := range s.reg.Snapshot().Conversations {
		if conv.ID == id {
			return conv, nil
		}
	}
	return model.Conversation{}, newError(CodeNotFound, fmt.Sprintf("no conversation '%s'", id))
}

// updateConversation changes one recorded conversation. callers hold s.mu.
func (s *Service) updateConversation(id string, fn func(*model.Conversation)) error {
	return s.update(func(doc *registry.Document) error {
		i := slices.IndexFunc(doc.Conversations, func(c model.Conversation) bool { return c.ID == id })
		if i < 0 {
			return newError(CodeNotFound, fmt.Sprintf("no conversation '%s'", id))
		}
		fn(&doc.Conversations[i])
		return nil
	})
}

// sink receives one runtime incarnation's output.
type sink struct {
	s   *Service
	l   *live
	inc *incarnation
}

// current reports whether this sink's runtime is still the conversation's.
// callers hold s.mu.
func (k *sink) current() bool {
	return k.l.inc == k.inc && k.s.live[k.l.id] == k.l
}

// Event records what the runtime reported. a transition the browser sees (a
// turn starting or ending, the runtime's facts) takes effect only once its
// event is recorded, so a log that cannot confirm it leaves the last confirmed
// view standing.
func (k *sink) Event(ev *model.Event) {
	s := k.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if !k.current() {
		return
	}
	l := k.l
	if ev.Kind == model.EventRuntime && ev.Status == model.StatusStarted {
		k.started(ev)
		return
	}
	if err := s.record(l, ev); err != nil {
		dl.Errorf("%v", err)
		return
	}
	switch ev.Kind {
	case model.EventAssistant, model.EventToolUse, model.EventToolResult:
		if !l.turnActive {
			l.turnActive = true
			s.changed()
		}
	case model.EventTurnEnd:
		l.turnActive = false
		s.changed()
	}
}

// started shows the runtime running once it reports the session it holds, and
// stops a runtime bound to a different session than the conversation's:
// resuming anything but the exact identity is never acceptable. callers hold
// s.mu.
func (k *sink) started(ev *model.Event) {
	s, l := k.s, k.l
	conv, err := s.conversationLocked(l.id)
	if err != nil {
		return
	}
	if ev.Facts != nil && ev.Facts.Session != "" && conv.NativeID != "" && ev.Facts.Session != conv.NativeID {
		s.beginStopLocked(l, &model.Event{
			Status: model.StatusError,
			Text:   fmt.Sprintf("the runtime reported session '%s', but this conversation is bound to '%s'; stopping it", ev.Facts.Session, conv.NativeID),
		})
		return
	}
	facts := ev.Facts
	if err := s.transition(l, ev, StateRunning, func() { l.facts = facts }); err != nil {
		dl.Errorf("%v", err)
	}
}

func (k *sink) Delta(text string) {
	s := k.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if k.current() {
		s.publish(Update{ConversationID: k.l.id, Delta: text})
	}
}

// Request makes a request answerable only once its event is recorded. one the
// log cannot record never joins the pending set: no card offers it, nothing
// answers it on the operator's behalf, and the harness keeps waiting until an
// interrupt or release aborts it.
func (k *sink) Request(kind string, payload any) string {
	s := k.s
	s.mu.Lock()
	defer s.mu.Unlock()
	req := &model.Request{ID: registry.NewID(), Kind: kind, Payload: payload}
	if !k.current() {
		return req.ID
	}
	l := k.l
	if err := s.record(l, &model.Event{Kind: model.EventRequest, Origin: model.OriginManaged, Text: kind, Req: req}); err != nil {
		dl.Errorf("request '%s' for '%s' could not be recorded and is not answerable: %v", req.ID, l.id, err)
		return req.ID
	}
	l.pending = append(l.pending, req)
	s.changed()
	return req.ID
}

func (k *sink) Resolved(requestID, outcome string) {
	s := k.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if !k.current() {
		return
	}
	// the resolution leaves the pending set only once it is recorded. one that
	// cannot be recorded stays listed, and an answer to it reaches the runtime,
	// which refuses it as no longer pending.
	l := k.l
	i := slices.IndexFunc(l.pending, func(r *model.Request) bool { return r.ID == requestID })
	if i < 0 {
		return
	}
	if err := s.resolve(l, l.pending[i], outcome); err != nil {
		dl.Errorf("%v", err)
		return
	}
	l.takePending(requestID)
	s.changed()
}

// Exited settles everything the runtime left: its pending requests are
// aborted, the runtime is recorded as stopped (or as an error when it died
// mid-turn or unasked), and the conversation records no runtime. a runtime
// released with no turn in flight has stopped as asked even if it reports a
// failure status: claude 2.1.318 exits 1 when input closes after an
// interrupted turn. the status is kept in the text.
//
// the dead process is always let go. if the stop cannot be recorded, the view
// keeps its last confirmed state and nothing is announced: requests whose abort
// was not recorded stay listed (an answer reaches no runtime and is refused as
// not pending), and the conversation stays recorded running, so a restart
// reports it unsupervised rather than claiming a stop it never confirmed.
func (k *sink) Exited(err error) {
	s := k.s
	s.mu.Lock()
	defer s.mu.Unlock()
	defer close(k.inc.exited)
	if !k.current() {
		return
	}
	l := k.l
	released := l.releasing
	l.runtime, l.inc, l.releasing = nil, nil, false
	confirmed := true
	var unrecorded []*model.Request
	for _, req := range l.pending {
		if err := s.resolve(l, req, model.OutcomeAborted); err != nil {
			dl.Errorf("%v", err)
			unrecorded = append(unrecorded, req)
			confirmed = false
		}
	}
	l.pending = unrecorded
	ev := &model.Event{Status: model.StatusStopped}
	switch {
	case l.turnActive:
		ev.Status, ev.Text = model.StatusError, "the runtime exited during a turn"
		if err != nil {
			ev.Text += fmt.Sprintf(": %v", err)
		}
	case err != nil && released:
		ev.Text = fmt.Sprintf("released; the runtime reported: %v", err)
	case err != nil:
		ev.Status, ev.Text = model.StatusError, fmt.Sprintf("the runtime exited: %v", err)
	}
	if !confirmed {
		dl.Errorf("the stop of '%s' could not be recorded; it stays recorded running", l.id)
		return
	}
	if err := s.transition(l, ev, "", func() { l.turnActive, l.facts = false, nil }); err != nil {
		dl.Errorf("the stop of '%s' could not be recorded; it stays recorded running: %v", l.id, err)
		return
	}
	if err := s.updateConversation(l.id, func(c *model.Conversation) { c.Runtime = model.RuntimeNone }); err != nil {
		dl.Errorf("recording that '%s' stopped: %v", l.id, err)
	}
}

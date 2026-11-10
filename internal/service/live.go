package service

import (
	"errors"
	"fmt"
	"slices"
	"time"

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
	// TurnStartedAt is when the turn in flight began, from memory; zero when
	// no turn is active.
	TurnStartedAt time.Time
	Facts      *model.RuntimeFacts
	Pending    []model.Request

	// Attention is computed from the facts above every time it is read, and
	// AttentionAt is when the fact behind it happened: the oldest pending
	// request, or the unread output.
	Attention   string
	AttentionAt time.Time

	// ModelNext and EffortNext are a change asked for while running that the
	// next prompt carries; empty once that turn has ended.
	ModelNext  string
	EffortNext string
}

// live is the in-memory side of one conversation. the pending set lives here
// and nowhere else: it dies with the runtime and is never rebuilt from the log.
//
// two kinds of state live here. state, turnActive, facts and pending are what
// browsers see, and change only once the event behind them is recorded. runtime,
// inc, busy and releasing describe the process and the operations on it, and
// follow the process whether or not the log could record anything.
type live struct {
	id        string
	log       *events.Log
	seen      map[string]bool // native ids already in the log
	runtime   harness.Runtime
	inc       *incarnation // the current runtime's identity; nil when none
	busy      bool         // a start or a resume is under way
	releasing bool         // the current runtime has been asked to stop
	switching bool         // a permission mode or model change is waiting on the runtime

	// nextModel and nextEffort are a change asked for while running, shown as
	// the next prompt's until a turn begun after the change ends. nextSent is
	// whether such a turn has begun.
	nextModel  string
	nextEffort string
	nextSent   bool

	// session is a native session this conversation created that the registry
	// does not hold yet: one a runtime reported, or the last one the log shows
	// started. the next launch binds it and resumes it; a fresh session starts
	// only when neither the registry nor this knows one.
	session    string
	state      string // displayed: "", starting, running, stopping or resuming
	turnActive bool
	// turnStartedAt is when the turn in flight began; zero when none is. it
	// lives only here, beside turnActive, and is never stored.
	turnStartedAt time.Time
	facts      *model.RuntimeFacts
	pending    []*model.Request
	pendingAt  map[string]time.Time // when each pending request was recorded

	// fault is whether the last runtime ended badly: an error, an exit during a
	// turn, or a launch that did not start. a runtime that starts or stops
	// cleanly clears it.
	fault bool

	// turnOutput is the newest agent output recorded in the turn in flight. it
	// becomes the conversation's OutputSeq only when the turn ends, so output
	// still being produced is running, not unread.
	turnOutput   int64
	turnOutputAt time.Time

	// outputAt is when the conversation's OutputSeq event was recorded.
	outputAt time.Time
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
	l := &live{id: conversationID, log: log, seen: make(map[string]bool), pendingAt: make(map[string]time.Time)}
	var outputSeq int64
	if conv, err := s.conversationLocked(conversationID); err == nil {
		outputSeq = conv.OutputSeq
	}
	for _, ev := range evs {
		if ev.NativeID != "" {
			l.seen[ev.NativeID] = true
		}
		if ev.Kind == model.EventRuntime && ev.Status == model.StatusStarted && ev.Facts != nil && ev.Facts.Session != "" {
			l.session = ev.Facts.Session
		}
		if ev.Kind == model.EventRuntime {
			l.noteRuntime(ev.Status)
		}
		if ev.Seq == outputSeq {
			l.outputAt = ev.At
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
	l.noteRuntime(ev.Status)
	if apply != nil {
		apply()
	}
	s.changed()
	return nil
}

// noteRuntime follows how the last runtime ended, for the error attention.
func (l *live) noteRuntime(status string) {
	switch status {
	case model.StatusStarted, model.StatusStopped:
		l.fault = false
	case model.StatusError, model.StatusNotStarted:
		l.fault = true
	}
}

// failedLaunch records a launch that failed. only a failure proven to come
// before any native session could exist is not_started: the service's own
// failures before launching, and a harness error wrapping harness.ErrNotStarted.
// any other failure is an error whose session is unknown, which keeps the
// conversation refused a fresh session. like every transition it is shown only
// once recorded: if the log cannot record it, the last confirmed starting stays
// on screen and the log's sticky refusal of later writes says a restart is
// needed. it reports whether the failure was recorded. callers hold s.mu.
func (s *Service) failedLaunch(l *live, reason error, beforeLaunch bool) bool {
	ev := &model.Event{Status: model.StatusNotStarted, Text: fmt.Sprintf("the runtime did not start: %v", reason)}
	if !beforeLaunch && !errors.Is(reason, harness.ErrNotStarted) {
		ev = &model.Event{Status: model.StatusError, Text: fmt.Sprintf("the launch failed and whether it created a session is unknown: %v", reason)}
	}
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
	if l.turnActive {
		view.TurnStartedAt = l.turnStartedAt
	}
	view.Facts = l.facts
	view.ModelNext, view.EffortNext = l.nextModel, l.nextEffort
	view.Pending = make([]model.Request, 0, len(l.pending))
	for _, req := range l.pending {
		view.Pending = append(view.Pending, *req)
	}
	view.Attention, view.AttentionAt = l.attention(conv)
	return view
}

// attention is computed from confirmed facts, in order: a pending request; an
// unsupervised runtime, uncertain and never shown as running or idle; a turn in
// flight; a runtime that ended badly; agent output from a completed turn past
// the reading position; and otherwise idle. reading never resolves a request.
// callers hold s.mu.
func (l *live) attention(conv model.Conversation) (string, time.Time) {
	if len(l.pending) > 0 {
		var oldest time.Time
		for _, req := range l.pending {
			if at := l.pendingAt[req.ID]; oldest.IsZero() || at.Before(oldest) {
				oldest = at
			}
		}
		return model.AttentionRequest, oldest
	}
	switch {
	case conv.Runtime == model.RuntimeUnsupervised:
		return model.AttentionUnsupervised, time.Time{}
	case l.turnActive:
		return model.AttentionRunning, time.Time{}
	case l.fault:
		return model.AttentionError, time.Time{}
	case conv.OutputSeq > conv.ReadSeq:
		return model.AttentionUnread, l.outputAt
	default:
		return model.AttentionIdle, time.Time{}
	}
}

// sessionOf is the native session the conversation is bound to: the one the
// registry holds, or else one it created that the registry does not hold yet.
// callers hold s.mu.
func (l *live) sessionOf(conv model.Conversation) string {
	if conv.NativeID != "" {
		return conv.NativeID
	}
	return l.session
}

// errSessionUnknown refuses to start a fresh session for a conversation whose
// earlier launch may have created one that neither the registry nor the log
// recorded.
var errSessionUnknown = newError(CodeConflict, "this conversation launched before but its session could not be recorded, so roundhouse will not start another in its place; find the session under 'Adopt a session' and adopt it into a new conversation")

// freshAllowedLocked reports whether the log proves no native session can
// exist for the conversation: no launch has started, or every launch that
// started was recorded as not_started. callers hold s.mu.
func (s *Service) freshAllowedLocked(l *live) (bool, error) {
	evs, err := l.log.Since(0)
	if err != nil {
		return false, err
	}
	open := false
	for _, ev := range evs {
		if ev.Kind != model.EventRuntime {
			continue
		}
		switch ev.Status {
		case model.StatusStarting:
			if open {
				return false, nil
			}
			open = true
		case model.StatusNotStarted:
			open = false
		default:
			if open {
				return false, nil
			}
		}
	}
	return !open, nil
}

// takePending removes a request from the pending set. callers hold s.mu.
func (l *live) takePending(requestID string) (*model.Request, bool) {
	i := slices.IndexFunc(l.pending, func(r *model.Request) bool { return r.ID == requestID })
	if i < 0 {
		return nil, false
	}
	req := l.pending[i]
	l.pending = slices.Delete(l.pending, i, i+1)
	delete(l.pendingAt, requestID)
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
	if ev.Kind == model.EventRuntime && ev.Status == model.StatusModeChanged {
		s.harnessModeChanged(l, ev)
		return
	}
	if ev.Kind == model.EventRuntime && ev.Status == model.StatusModelChanged {
		s.harnessModelChanged(l, ev)
		return
	}
	if err := s.record(l, ev); err != nil {
		dl.Errorf("%v", err)
		return
	}
	switch ev.Kind {
	case model.EventUser, model.EventAssistant, model.EventToolUse, model.EventToolResult:
		// a user message the runtime reports opens a turn too, such as an
		// answer that starts the next turn.
		if !l.turnActive {
			l.beginTurn(ev.At)
			s.changed()
		}
	case model.EventTurnEnd:
		l.turnActive, l.turnStartedAt = false, time.Time{}
		if l.nextSent {
			// a model or effort asked for has had its prompt; from here the
			// harness's report stands alone.
			l.nextModel, l.nextEffort, l.nextSent = "", "", false
		}
		s.endTurnLocked(l)
		s.changed()
	}
	// agent output counts toward unread only within a turn, and only once that
	// turn ends. native events never count: they are the harness's own
	// bookkeeping (init, status, lifecycle), and bookkeeping never creates
	// attention.
	switch ev.Kind {
	case model.EventAssistant, model.EventToolUse, model.EventToolResult:
		if l.turnActive && ev.Seq > 0 {
			l.turnOutput, l.turnOutputAt = ev.Seq, ev.At
		}
	}
}

// endTurnLocked makes the ended turn's newest agent output the conversation's
// OutputSeq, so it reads as unread. the registry is the record: memory follows
// only a confirmed write. a turn with no agent output changes nothing.
// callers hold s.mu.
func (s *Service) endTurnLocked(l *live) {
	seq, at := l.turnOutput, l.turnOutputAt
	l.turnOutput, l.turnOutputAt = 0, time.Time{}
	if seq == 0 {
		return
	}
	if err := s.markUnreadLocked(l, seq, at); err != nil {
		dl.Errorf("recording the output of '%s': %v", l.id, err)
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
	bound := l.sessionOf(conv)
	if ev.Facts != nil && ev.Facts.Session != "" && bound != "" && ev.Facts.Session != bound {
		s.beginStopLocked(l, &model.Event{
			Status: model.StatusError,
			Text:   fmt.Sprintf("the runtime reported session '%s', but this conversation is bound to '%s'; stopping it", ev.Facts.Session, bound),
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
	recorded := &model.Event{Kind: model.EventRequest, Origin: model.OriginManaged, Text: kind, Req: req}
	if err := s.record(l, recorded); err != nil {
		dl.Errorf("request '%s' for '%s' could not be recorded and is not answerable: %v", req.ID, l.id, err)
		return req.ID
	}
	l.pending = append(l.pending, req)
	l.pendingAt[req.ID] = recorded.At
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
	case errors.Is(err, harness.ErrLeftRunning):
		// never a clean stop, asked for or not: something it started may still
		// be working.
		ev.Status, ev.Text = model.StatusError, fmt.Sprintf("the runtime exited but left processes running: %v", err)
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
	if err := s.transition(l, ev, "", func() {
		l.turnActive, l.turnStartedAt, l.facts = false, time.Time{}, nil
		// a change asked of this runtime dies with it; the next launch
		// passes the stored choice.
		l.nextModel, l.nextEffort, l.nextSent = "", "", false
	}); err != nil {
		dl.Errorf("the stop of '%s' could not be recorded; it stays recorded running: %v", l.id, err)
		return
	}
	if err := s.updateConversation(l.id, func(c *model.Conversation) { c.Runtime = model.RuntimeNone }); err != nil {
		dl.Errorf("recording that '%s' stopped: %v", l.id, err)
	}
}

// beginTurn marks a turn in flight from at, the time its first event was
// recorded (now, if it was not recorded). a model or effort change asked for
// before it goes with it.
func (l *live) beginTurn(at time.Time) {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	l.turnActive, l.turnStartedAt = true, at
	l.nextSent = true
}

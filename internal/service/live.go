package service

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/michaelquigley/df/dl"

	"github.com/michaelquigley/mega/internal/events"
	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/registry"
)

// transient runtime states the service reports alongside the recorded ones.
const (
	StateStarting = "starting"
	StateRunning  = model.RuntimeRunning
	StateStopping = "stopping"
)

// ConversationView is a conversation as the service currently knows it: the
// recorded identity plus what lives only in memory, such as the pending set.
type ConversationView struct {
	model.Conversation

	// State is the runtime as the service sees it now: none, starting, running,
	// stopping, or unsupervised.
	State      string
	TurnActive bool
	// Interrupting says an interrupt has begun and the turn has not yet
	// ended: answers that would let work go on are refused meanwhile.
	Interrupting bool
	// TurnStartedAt is when the turn in flight began, from memory; zero when
	// no turn is active.
	TurnStartedAt time.Time
	// InstructedAt is when the operator last instructed the conversation,
	// the time of the newest prompt mega sent it; zero when none was. it is
	// a fact about the operator, never attention.
	InstructedAt time.Time
	// PendingAt is when each pending request was recorded, by request id: its
	// request event's own time, held in memory beside the pending set.
	PendingAt map[string]time.Time
	// Background is the work the runtime keeps running outside a turn, from
	// memory; empty when none, or when there is no runtime to ask.
	Background []model.BackgroundJob
	// MCPServers are the runtime's MCP servers, from memory, and MCPReported
	// says the runtime has reported them; both are empty with no runtime.
	MCPServers  []model.MCPServer
	MCPReported bool
	Facts       *model.RuntimeFacts
	// EnvironmentStale says the running runtime would start in another
	// environment now: its project's activation, or the fingerprint of the
	// project's latest resolution, has changed since it started. it is
	// computed on every read; the runtime keeps what it started with.
	EnvironmentStale bool
	// Environment is what the running runtime was launched with, as names
	// and fingerprints; absent when no runtime runs.
	Environment *model.EnvironmentFacts
	Pending     []model.Request

	// Attention is computed from the facts above every time it is read, and
	// AttentionAt is when the fact behind it happened: the oldest pending
	// request, or the unread output.
	Attention   string
	AttentionAt time.Time

	// TurnAsked and TurnRan are what the turn in flight asked for and, once
	// its first output has arrived, ran with; nil when no turn is in flight,
	// and TurnRan before then. a finished turn carries its own on its
	// turn_end event. TurnDiffers names where TurnRan is not what was
	// asked for, as a turn_end's record does.
	TurnAsked   *model.TurnSettings
	TurnRan     *model.TurnSettings
	TurnDiffers []string
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
	busy      bool         // a start is under way
	releasing bool         // the current runtime has been asked to stop
	// refused is a wrong-session report that arrived while the runtime was
	// still launching, with no runtime yet to stop; the launch applies it as
	// soon as Launch returns, before any prompt.
	refused   *model.Event
	switching bool // a permission mode or model change is waiting on the runtime

	state      string // displayed: "", starting, running, or stopping
	turnActive bool
	// turnStartedAt is when the turn in flight began; zero when none is. it
	// lives only here, beside turnActive, and is never stored.
	turnStartedAt time.Time
	// turnAsked is what the turn in flight asked for, and turnRan what it ran
	// with, kept at its first output (ranLocked); both go on its turn_end.
	turnAsked model.TurnSettings
	turnRan   *model.TurnSettings
	// background is the work the runtime keeps running outside a turn, as it
	// last reported it; it goes with the runtime.
	background []model.BackgroundJob
	// mcp is the runtime's MCP servers as it last reported them, and
	// mcpReported whether it has reported any yet; they go with the
	// runtime, and are never attention.
	mcp          []model.MCPServer
	mcpReported  bool
	facts        *model.RuntimeFacts
	pending      []*model.Request
	pendingAt    map[string]time.Time // when each pending request was recorded
	reserved     map[string]string    // listed requests no answer may reach: answerInFlight, or a settled outcome not yet recorded
	interrupting bool                 // an interrupt has begun, and a turn or a request is still outstanding

	// fault follows whether the last runtime ended badly or failed to launch.
	fault bool

	// turnOutput is the newest agent output recorded in the turn in flight. it
	// becomes the conversation's OutputSeq only when the turn ends, so output
	// still being produced is running, not unread.
	turnOutput   int64
	turnOutputAt time.Time

	// outputAt is when the conversation's OutputSeq event was recorded.
	outputAt time.Time

	// instructedAt is when the operator last instructed the conversation:
	// the time of the newest prompt mega sent it, a launch's opening one or
	// any after. the prompt's own event is the record; this is read from the
	// log as it is opened (instructs) and moved as send records a prompt.
	// zero when none was ever sent through mega.
	instructedAt time.Time
}

// instructs reports whether an event in the log is a prompt mega sent: a
// user event mega recorded itself, which carries no harness payload. a
// prompt a harness reported, live or imported, carries its own and does not
// count; an imported one's time is when mega imported it.
func instructs(ev *model.Event) bool {
	return ev.Kind == model.EventUser && ev.Origin == model.OriginManaged && ev.Raw == nil
}

// incarnation tells one runtime's callbacks from a later runtime's, so a late
// call from a runtime that has been replaced changes nothing.
type incarnation struct {
	exited chan struct{}
	// launched is closed once its launch has returned, or was refused at the
	// last check: a shutdown that began meanwhile waits for it.
	launched chan struct{}
	// env is the environment the runtime was launched with, as names and
	// fingerprints, recorded with its started report.
	env *model.EnvironmentFacts
	// version is what the harness's --version printed just before the
	// launch, used when its started report names none.
	version string
	// wrong latches a report of another session than the one bound: from
	// then on nothing this runtime reports is recorded or offered, only its
	// exit and its settlements.
	wrong bool
}

// liveLocked returns the conversation's live state, opening its log on first
// use. once Close has closed the logs it opens none again: a history read or
// a read acknowledgment in the moment before the HTTP server stops is refused
// as unavailable, rather than read from disk past what was confirmed. callers
// hold s.mu.
func (s *Service) liveLocked(conversationID string) (*live, error) {
	if l := s.live[conversationID]; l != nil {
		return l, nil
	}
	if s.logsClosed {
		return nil, newError(CodeUnavailable, "the service is shutting down")
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
	l := &live{id: conversationID, log: log, seen: make(map[string]bool), pendingAt: make(map[string]time.Time), reserved: make(map[string]string)}
	var outputSeq int64
	if conv, err := s.conversationLocked(conversationID); err == nil {
		outputSeq = conv.OutputSeq
	}
	var legacyImport bool
	for _, ev := range evs {
		if ev.NativeID != "" {
			l.seen[ev.NativeID] = true
		}
		if ev.Seq == outputSeq {
			l.outputAt = ev.At
		}
		if instructs(ev) {
			l.instructedAt = ev.At
		}
		if ev.Kind != model.EventRuntime {
			continue
		}
		// Old imports wrote resuming before their result. A failed import
		// stays in the timeline and leaves the runtime fault alone.
		switch ev.Status {
		case model.StatusResuming:
			legacyImport = true
		case model.StatusError:
			if legacyImport {
				legacyImport = false
				continue
			}
		case model.StatusResumed, model.StatusStarting, model.StatusStarted,
			model.StatusStopped, model.StatusUnsupervised:
			legacyImport = false
		}
		l.noteRuntime(ev.Status)
	}
	s.live[conversationID] = l
	return l, nil
}

func (l *live) hasRecorded(ev *model.Event) bool {
	return ev.NativeID != "" && l.seen[ev.NativeID]
}

// record appends an event and announces it. an event whose native identity is
// already in the log is skipped, including an echo of a snapshot message. callers hold s.mu.
func (s *Service) record(l *live, ev *model.Event) error {
	if l.hasRecorded(ev) {
		return nil
	}
	if ev.Origin == "" {
		ev.Origin = model.OriginService
	}
	if err := s.appendEvent(l.id, l.log, ev); err != nil {
		return err
	}
	if ev.NativeID != "" {
		l.seen[ev.NativeID] = true
	}
	s.publish(Update{ConversationID: l.id, Event: ev})
	return nil
}

func (s *Service) appendEvent(id string, log *events.Log, ev *model.Event) error {
	if s.failRecord != nil {
		if err := s.failRecord(ev); err != nil {
			return fmt.Errorf("recording event for '%s': %w", id, err)
		}
	}
	if err := log.Append(ev); err != nil {
		if errors.Is(err, events.ErrUncertain) {
			return newError(CodeUnavailable, err.Error())
		}
		return fmt.Errorf("recording event for '%s': %w", id, err)
	}
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

// noteRuntime follows the runtime fault for attention, in recording order.
func (l *live) noteRuntime(status string) {
	switch status {
	case model.StatusStarted, model.StatusStopped:
		l.fault = false
	case model.StatusError:
		l.fault = true
	}
}

// failedLaunch records a launch that failed, whatever failed, as one error.
// like every transition it is shown only once recorded: if the log cannot
// record it, the last confirmed starting stays on screen and the log's sticky
// refusal of later writes says a restart is needed. it reports whether the
// failure was recorded. callers hold s.mu.
func (s *Service) failedLaunch(l *live, reason error) bool {
	ev := &model.Event{Status: model.StatusError, Text: fmt.Sprintf("the launch failed: %s", recordable(reason))}
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
		// let go of at close: the facts as they stood then.
		l = s.closedLive[conv.ID]
	}
	if l == nil {
		// no live state ever: the log could not be opened. attention is what
		// the registry says, so a page never holds a conversation without one.
		view.Attention = recordedAttention(conv)
		return view
	}
	view.LastSeq = l.log.LastSeq()
	if l.state != "" {
		view.State = l.state
	}
	view.TurnActive = l.turnActive
	view.Interrupting = l.interrupting
	if l.turnActive {
		view.TurnStartedAt = l.turnStartedAt
	}
	view.InstructedAt = l.instructedAt
	// a copy, so a reader of the view can never change the service's facts.
	view.Facts = cloneFacts(l.facts)
	if l.runtime != nil && view.Facts != nil {
		view.Environment = view.Facts.Environment
		view.EnvironmentStale = s.envStaleLocked(conv, l.facts.Environment)
	}
	view.Background = slices.Clone(l.background)
	view.MCPServers, view.MCPReported = cloneMCP(l.mcp), l.mcpReported
	if l.turnActive {
		asked := l.turnAsked
		view.TurnAsked = &asked
		if l.turnRan != nil {
			ran := *l.turnRan
			view.TurnRan = &ran
			view.TurnDiffers = s.differs(conv.Harness, asked, ran)
		}
	}
	view.Pending = make([]model.Request, 0, len(l.pending))
	view.PendingAt = make(map[string]time.Time, len(l.pending))
	for _, req := range l.pending {
		// a copy, payload and all: the pending set stays the service's own.
		view.Pending = append(view.Pending, req.Clone())
		view.PendingAt[req.ID] = l.pendingAt[req.ID]
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

// recordedAttention is attention from the registry's facts alone, for a
// conversation whose live state was never opened: no request, turn or fault
// is known, so it is unsupervised, unread or idle, in attention's own order.
func recordedAttention(conv model.Conversation) string {
	switch {
	case conv.Runtime == model.RuntimeUnsupervised:
		return model.AttentionUnsupervised
	case conv.OutputSeq > conv.ReadSeq:
		return model.AttentionUnread
	default:
		return model.AttentionIdle
	}
}

// errNoSession refuses to launch a conversation the registry holds no
// session for, whatever left it without one: only its creation launches a
// new session, once.
var errNoSession = newError(CodeConflict, "this conversation has no recorded session, so mega will not launch it; remove it, and if the harness made a session, adopt it under 'Adopt a session'")

// unrecorded is the outcome the harness settled a request with, if that
// could not be recorded yet: the request stays listed, as last confirmed,
// but no answer reaches it. callers hold s.mu.
func (l *live) unrecorded(requestID string) (string, bool) {
	state := l.reserved[requestID]
	return state, state != "" && state != answerInFlight
}

// holdsRequests is whether the harness still holds any pending request: one
// it settled is no longer the harness's to settle or an interrupt's to stop.
// callers hold s.mu.
func (l *live) holdsRequests() bool {
	return slices.ContainsFunc(l.pending, func(r *model.Request) bool {
		_, settled := l.unrecorded(r.ID)
		return !settled
	})
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
	delete(l.reserved, requestID)
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

// relays is whether what this runtime reports reaches the conversation: it is
// current, and it has not reported another session than the one bound.
func (k *sink) relays() bool {
	return k.current() && !k.inc.wrong
}

// Event records what the runtime reported. a transition the browser sees (a
// turn starting or ending, the runtime's facts) takes effect only once its
// event is recorded, so a log that cannot confirm it leaves the last confirmed
// view standing.
func (k *sink) Event(ev *model.Event) {
	s := k.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if !k.relays() {
		return
	}
	l := k.l
	if l.hasRecorded(ev) {
		return
	}
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
	if ev.Kind == model.EventTurnEnd && l.turnActive {
		// the turn's own record of what it asked for and ran with.
		ev.Turn = s.turnRecordLocked(l)
	}
	if err := s.record(l, ev); err != nil {
		dl.Errorf("%v", err)
		return
	}
	switch ev.Kind {
	case model.EventUser, model.EventAssistant, model.EventToolUse, model.EventToolResult:
		// a user message the runtime reports opens a turn too. output the
		// adapter marks as after its turn opens none, since no turn is coming
		// to end it, and never settles what a turn under way runs with, since
		// that turn did not produce it.
		if !l.turnActive && !ev.AfterTurn {
			s.beginTurnLocked(l, ev.At)
			s.changed()
		}
		if ev.Kind != model.EventUser && !ev.AfterTurn && l.turnActive && l.turnRan == nil {
			// the turn's first output: what it runs with is settled.
			s.ranLocked(l)
			s.changed()
		}
	case model.EventTurnEnd:
		s.turnEndedLocked(l)
	}
	// agent output counts toward unread within a turn, once that turn ends;
	// output the adapter marks as after its turn (a codex background command
	// finishing) counts at once, its turn having ended already. native events
	// never count: they are the harness's own bookkeeping (init, status,
	// lifecycle), and bookkeeping never creates attention.
	switch ev.Kind {
	case model.EventAssistant, model.EventToolUse, model.EventToolResult:
		switch {
		case ev.AfterTurn && ev.Seq > 0 && !l.turnActive:
			// output of a turn that has already ended: unread at once, as a
			// turn's end would make it.
			if err := s.markUnreadLocked(l, ev.Seq, ev.At); err != nil {
				dl.Errorf("recording the output of '%s': %v", l.id, err)
			}
		case l.turnActive && ev.Seq > 0:
			l.turnOutput, l.turnOutputAt = ev.Seq, ev.At
		}
	}
}

// turnEndedLocked is what a recorded turn end does to memory, wherever it
// came from: the turn is over, its settings and interrupt with it, and its
// output is counted. callers hold s.mu and have recorded the turn_end.
func (s *Service) turnEndedLocked(l *live) {
	l.turnActive, l.turnStartedAt = false, time.Time{}
	l.turnAsked, l.turnRan = model.TurnSettings{}, nil
	l.interrupting = false
	s.endTurnLocked(l)
	s.changed()
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
	bound := conv.NativeID
	if ev.Facts != nil && ev.Facts.Session != "" && bound != "" && ev.Facts.Session != bound {
		k.inc.wrong = true
		refusal := &model.Event{
			Status: model.StatusError,
			Text:   fmt.Sprintf("the runtime reported session '%s', but the launch expected '%s'", ev.Facts.Session, bound),
		}
		if l.runtime == nil {
			// still launching (an adapter may report before Launch returns):
			// held until the runtime is in hand, never dropped.
			if l.refused == nil {
				l.refused = refusal
			}
			return
		}
		s.beginStopLocked(l, refusal)
		return
	}
	// the start records what it was launched with, as names and
	// fingerprints only, and where its version stands against the tested
	// range: the history's started line is marked by it.
	withEnv := model.RuntimeFacts{}
	if ev.Facts != nil {
		withEnv = *ev.Facts
	}
	if k.inc.env != nil {
		withEnv.Environment = k.inc.env
	}
	if withEnv.Version == "" {
		withEnv.Version = k.inc.version
	}
	withEnv.Standing, withEnv.VersionNote = harness.Stand(conv.Harness, withEnv.Version)
	withEnv.Tested = harness.Tested[conv.Harness].String()
	ev.Facts = cloneFacts(&withEnv)
	// the facts the service keeps are its own: the event goes to every
	// subscriber, and none of them may reach what later reads show.
	facts := cloneFacts(ev.Facts)
	// a release asked for before the runtime reported (an idle start's report
	// can take half a second) keeps the stopping it recorded: the runtime is
	// on its way out, and only its exit settles it.
	state := StateRunning
	if l.releasing || l.refused != nil {
		state = l.state
	}
	if err := s.transition(l, ev, state, func() { l.facts = facts }); err != nil {
		dl.Errorf("%v", err)
	}
}

// MCPServers replaces the runtime's list of MCP servers. like background
// jobs it is held in memory beside the runtime's facts, never recorded, and
// shown as it changes; it is not attention, so a server that failed or needs
// signing in never puts a conversation in the needs-you list. an unchanged
// list announces nothing.
func (k *sink) MCPServers(servers []model.MCPServer) {
	s := k.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if !k.relays() {
		return
	}
	if k.l.mcpReported && slices.EqualFunc(k.l.mcp, servers, sameMCPServer) {
		return
	}
	k.l.mcp, k.l.mcpReported = cloneMCP(servers), true
	s.changed()
}

func sameMCPServer(a, b model.MCPServer) bool {
	return a.Name == b.Name && a.Status == b.Status && a.Error == b.Error && slices.Equal(a.Tools, b.Tools)
}

// cloneMCP copies a list, tools and all, so neither side can change the
// other's.
func cloneMCP(servers []model.MCPServer) []model.MCPServer {
	if servers == nil {
		return nil
	}
	out := make([]model.MCPServer, len(servers))
	for i, server := range servers {
		out[i] = server
		out[i].Tools = slices.Clone(server.Tools)
	}
	return out
}

// Background replaces the runtime's list of background jobs. it is held in
// memory beside turn_active, never recorded, and shown as it changes; it is
// not attention, so it never puts a conversation in the needs-you list.
func (k *sink) Background(jobs []model.BackgroundJob) {
	s := k.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if !k.relays() {
		return
	}
	k.l.background = slices.Clone(jobs)
	s.changed()
}

func (k *sink) Delta(text string) {
	s := k.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if k.relays() {
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
	if !k.relays() {
		return req.ID
	}
	l := k.l
	// the event, which every subscriber receives, carries its own copy; the
	// pending set keeps the service's.
	published := req.Clone()
	recorded := &model.Event{Kind: model.EventRequest, Origin: model.OriginManaged, Text: kind, Req: &published}
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
	if err := s.settleLocked(l, l.pending[i], outcome); err != nil {
		dl.Errorf("%v", err)
	}
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
// was not recorded stay listed, and take no further answer, so none reaches a
// runtime that replaces this one (retireRequestsLocked), and the conversation
// stays recorded running, so a restart
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
	confirmed := s.retireRequestsLocked(l)
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
	case only(err, harness.ErrUnchecked):
		// not a clean stop, and not a leak: the processes it started could not
		// be listed, so whether any outlived it is not known, and it says so.
		said := "stopped"
		if released {
			said = "released"
		}
		ev.Text = fmt.Sprintf("%s, but %v", said, err)
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
		l.turnActive, l.turnStartedAt, l.facts, l.background = false, time.Time{}, nil, nil
		l.mcp, l.mcpReported = nil, false
		l.turnAsked, l.turnRan = model.TurnSettings{}, nil
		l.interrupting = false
	}); err != nil {
		dl.Errorf("the stop of '%s' could not be recorded; it stays recorded running: %v", l.id, err)
		return
	}
	// the turn ended with its runtime: what it had produced is counted now,
	// as a turn's end would count it, and nothing is left for a later turn.
	s.endTurnLocked(l)
	if err := s.updateConversation(l.id, func(c *model.Conversation) { c.Runtime = model.RuntimeNone }); err != nil {
		dl.Errorf("recording that '%s' stopped: %v", l.id, err)
	}
}

// retireRequestsLocked settles every request a runtime that has ended left
// pending, through settleLocked: as aborted, or as the settlement that could
// not be recorded before, so the history says what the operator was told.
// one whose settlement cannot be recorded stays listed as last recorded, and
// takes no further answer, so none reaches a runtime that replaces this one.
// it reports whether every settlement was recorded. callers hold s.mu.
func (s *Service) retireRequestsLocked(l *live) bool {
	confirmed := true
	for _, req := range slices.Clone(l.pending) {
		outcome, settled := l.unrecorded(req.ID)
		if !settled {
			outcome = model.OutcomeAborted
		}
		if err := s.settleLocked(l, req, outcome); err != nil {
			dl.Errorf("%v", err)
			confirmed = false
		}
	}
	return confirmed
}

// only is whether err is target alone: target itself, or errors joined whose
// every one is target. a runtime that could not check its processes and also
// reported something else is told by the other.
func only(err, target error) bool {
	if !errors.Is(err, target) {
		return false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, e := range joined.Unwrap() {
			if !errors.Is(e, target) {
				return false
			}
		}
	}
	return true
}

// cloneFacts copies runtime facts with nothing mutable shared: the
// environment, and its changed names, are copied too. nil stays nil.
func cloneFacts(f *model.RuntimeFacts) *model.RuntimeFacts {
	if f == nil {
		return nil
	}
	c := *f
	if f.Environment != nil {
		env := *f.Environment
		env.Changed = slices.Clone(env.Changed)
		c.Environment = &env
	}
	return &c
}

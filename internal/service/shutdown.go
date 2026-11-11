package service

import "time"

// CodeShutdownPending refuses new work while a shutdown is armed.
const CodeShutdownPending = "shutdown_pending"

// errShutdownPending is the gate's refusal of a prompt, a launch, an adoption
// or a start.
var errShutdownPending = newError(CodeShutdownPending, "mega is waiting to shut down once every agent is idle, so it starts no new work; cancel the pending shutdown to continue")

// the kinds of thing that hold the gate.
const (
	BlockerTurn       = "turn"       // a turn in flight
	BlockerRequest    = "request"    // a request waiting on the operator
	BlockerBackground = "background" // a background job still running
	BlockerSettling   = "settling"   // a runtime starting, or a conversation resuming
)

// Blocker is one conversation holding the gate, by kind.
type Blocker struct {
	ConversationID string
	ProjectID      string
	Name           string
	Turn           bool
	Requests       int
	Background     int
	// Settling is the transient state in the way ("starting", "resuming"),
	// if any: a launch or an import that has not finished yet.
	Settling string
}

// Gate is the "shut down when idle" state: whether it is armed, since when,
// and what is in the way. idle is computed from the conversations at every
// read, never stored: no turn in flight, no pending request, no background
// job and nothing starting or resuming, in any conversation. a runtime that is
// merely running and idle is not in the way; the shutdown releases it.
type Gate struct {
	Armed    bool
	ArmedAt  time.Time
	Blockers []Blocker
}

// Idle is whether nothing is in the way.
func (g Gate) Idle() bool {
	return len(g.Blockers) == 0
}

// shutdownGate is the service's gate. it is held in memory only: the restart
// it announces is what ends it, so nothing is stored.
type shutdownGate struct {
	armed     bool
	armedAt   time.Time
	requested chan struct{} // closed once a shutdown is accepted
}

// blockerOf says what, if anything, a conversation's view holds the gate with.
// busy is the live guard on a launch or a resume: a runtime may report started
// before its launch returns, so the displayed state alone can say running while
// the service does not yet hold the runtime it would have to stop.
func blockerOf(view ConversationView, busy bool) (Blocker, bool) {
	b := Blocker{
		ConversationID: view.ID,
		ProjectID:      view.ProjectID,
		Name:           view.Name,
		Turn:           view.TurnActive,
		Requests:       len(view.Pending),
		Background:     len(view.Background),
	}
	switch {
	case view.State == StateStarting || view.State == StateResuming:
		b.Settling = view.State
	case busy:
		b.Settling = StateStarting
	}
	return b, b.Turn || b.Requests > 0 || b.Background > 0 || b.Settling != ""
}

// gateLocked is the gate as of now. callers hold s.mu.
func (s *Service) gateLocked() Gate {
	gate := Gate{Armed: s.gate.armed, ArmedAt: s.gate.armedAt}
	for _, conv := range s.reg.Snapshot().Conversations {
		if b, ok := blockerOf(s.viewLocked(conv), s.busyLocked(conv.ID)); ok {
			gate.Blockers = append(gate.Blockers, b)
		}
	}
	return gate
}

// refuseNewWorkLocked is the gate's check before a prompt, a launch, an
// adoption or a start. callers hold s.mu.
func (s *Service) refuseNewWorkLocked() error {
	if s.gate.armed {
		return errShutdownPending
	}
	return nil
}

// ArmShutdown arms "shut down when idle". from now on no new work starts;
// everything in motion is left to finish, and nothing is answered, approved,
// denied, interrupted or released to get there. arming twice changes nothing.
func (s *Service) ArmShutdown() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return newError(CodeUnavailable, "the service is shutting down")
	}
	if s.gate.armed {
		return nil
	}
	s.gate.armed, s.gate.armedAt = true, time.Now()
	s.changed()
	return nil
}

// CancelShutdown lifts the gate at once. cancelling an unarmed gate changes
// nothing; once a shutdown is under way there is nothing left to cancel.
func (s *Service) CancelShutdown() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return newError(CodeUnavailable, "the service is shutting down")
	}
	if !s.gate.armed {
		return nil
	}
	s.gate.armed, s.gate.armedAt = false, time.Time{}
	s.changed()
	return nil
}

// ShutDown asks for the orderly stop a signal gives, and is refused unless
// the gate is armed and nothing is in the way at this moment. it only asks:
// the caller that owns the process (serve) waits on ShutdownRequested and
// runs the stop, so there is one shutdown path.
func (s *Service) ShutDown() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return newError(CodeUnavailable, "the service is shutting down")
	}
	if !s.gate.armed {
		return newError(CodeConflict, "shutting down needs the shutdown armed first, and every agent idle")
	}
	if gate := s.gateLocked(); !gate.Idle() {
		return newError(CodeConflict, "not every agent is idle yet; the shutdown waits until nothing is in the way")
	}
	// from here the service is shutting down: nothing new starts and the gate
	// can no longer be lifted, so the stop finds the workspace as idle as it
	// was when the operator chose it.
	s.closed = true
	close(s.gate.requested)
	return nil
}

// ShutdownRequested is closed once a shutdown from the gate is accepted.
func (s *Service) ShutdownRequested() <-chan struct{} {
	return s.gate.requested
}

// busyLocked is whether a launch or a resume of the conversation is under way.
// callers hold s.mu.
func (s *Service) busyLocked(conversationID string) bool {
	l := s.live[conversationID]
	return l != nil && l.busy
}

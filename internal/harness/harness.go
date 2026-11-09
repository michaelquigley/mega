// Package harness is the seam between the service and the native harness
// adapters. it speaks only model types: everything a harness's wire protocol
// looks like stays inside its adapter package.
package harness

import (
	"context"
	"errors"

	"git.hq.quigley.com/research/roundhouse/internal/model"
)

// ErrNotPending reports an answer to a request the runtime no longer waits on.
var ErrNotPending = errors.New("request is not pending")

// Harness launches runtimes and reads native history for one kind of agent.
type Harness interface {
	// Name is the model.Harness* value conversations record.
	Name() string

	// Launch starts a runtime in cwd. an empty nativeID starts a new native
	// session; otherwise the runtime resumes exactly that session. the runtime's
	// native identity is known when Launch returns.
	Launch(spec Spec, sink Sink) (Runtime, error)

	// History reads the native session's stored messages without a runtime.
	// every event carries a NativeID so the caller can match without text.
	History(ctx context.Context, cwd, nativeID string) ([]*model.Event, error)
}

// Spec describes a runtime to launch.
type Spec struct {
	Cwd      string
	NativeID string
}

// Sink receives a runtime's output. calls arrive in order from one goroutine
// per runtime; a sink must not call back into the runtime while handling one.
type Sink interface {
	// Event delivers a normalized event to record.
	Event(ev *model.Event)

	// Delta delivers streaming text for liveness only; it is never recorded.
	Delta(text string)

	// Request records a question or approval and returns the service's id for
	// it. the adapter keeps the native id private and maps answers through it.
	Request(kind string, payload any) string

	// Resolved reports that the harness settled a request on its own, for
	// example by aborting it on interrupt.
	Resolved(requestID, outcome string)

	// Exited reports that the runtime process ended. err is nil for a clean
	// exit. it is the last call a sink receives.
	Exited(err error)
}

// Runtime is one running harness process for one conversation. its methods
// may block on process I/O, so callers must not hold locks a Sink needs.
type Runtime interface {
	// NativeID is the native session identity this runtime is bound to.
	NativeID() string

	// Prompt starts a turn. uuid is the service-minted message identity, which
	// the harness records so a later history import recognizes the prompt.
	Prompt(uuid, text string) error

	// Answer delivers the operator's answer. it returns ErrNotPending if the
	// harness already settled the request.
	Answer(requestID string, answer Answer) error

	// Interrupt stops the active turn, aborting any pending request.
	Interrupt() error

	// Close ends input. the runtime exits once the harness is idle.
	Close() error
}

// Answer is the operator's response to a request.
type Answer struct {
	Decision string            // "allow" | "deny", for a permission
	Message  string            // why, for a deny
	Answers  map[string]string // question text to chosen label, for a question
}

// decisions an Answer carries for a permission request.
const (
	DecisionAllow = "allow"
	DecisionDeny  = "deny"
)

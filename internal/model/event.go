package model

import "time"

// event kinds.
const (
	EventRuntime         = "runtime"
	EventUser            = "user"
	EventAssistant       = "assistant"
	EventToolUse         = "tool_use"
	EventToolResult      = "tool_result"
	EventRequest         = "request"
	EventRequestResolved = "request_resolved"
	EventTurnEnd         = "turn_end"
	EventNative          = "native"
)

// event origins.
const (
	OriginManaged = "managed"
	OriginNative  = "native"
	OriginService = "service"
)

// runtime event states, carried in Event.Status. every change to a
// conversation's displayed runtime state is recorded as one of these.
const (
	StatusStarting = "starting"
	StatusStarted  = "started"
	// StatusNotStarted records a launch that failed before any process or
	// native session existed. it is the proof that a later launch may start a
	// fresh session.
	StatusNotStarted   = "not_started"
	StatusStopping     = "stopping"
	StatusStopped      = "stopped"
	StatusUnsupervised = "unsupervised"
	StatusResuming     = "resuming"
	StatusResumed      = "resumed"
	StatusError        = "error"
	// StatusModeChanged records a change of permission mode in a running
	// runtime, carried in Facts.PermissionMode: the operator's, or one the
	// harness reports, such as leaving plan mode.
	StatusModeChanged = "mode_changed"
)

// turn end statuses, carried in Event.Status.
const (
	TurnSuccess     = "success"
	TurnError       = "error"
	TurnInterrupted = "interrupted"
)

// Event is one entry in a conversation's append-only history.
type Event struct {
	Seq    int64
	At     time.Time
	Kind   string
	Origin string
	Text   string `dd:",+omitempty"`

	// NativeID is the harness's own identity for the message this event came
	// from. native history import matches on it, never on text.
	NativeID string `dd:",+omitempty"`

	// Status is the state of a runtime event or the outcome of a turn_end.
	Status string `dd:",+omitempty"`

	// Tool and ToolUseID name a tool_use and tie its tool_result to it.
	Tool      string `dd:",+omitempty"`
	ToolUseID string `dd:",+omitempty"`

	Req   *Request      `dd:",+omitempty"`
	Facts *RuntimeFacts `dd:",+omitempty"`

	// Raw is the harness-native payload, kept for fidelity and shown collapsed.
	Raw any `dd:",+opaque,+nullable,+omitempty"`
}

// RuntimeFacts are what a runtime reported about itself when it started.
type RuntimeFacts struct {
	Session        string `dd:",+omitempty"`
	Model          string `dd:",+omitempty"`
	Version        string `dd:",+omitempty"`
	PermissionMode string `dd:",+omitempty"`
	// ModeInherited is whether roundhouse passed no permission mode, so the
	// operator's native settings chose it.
	ModeInherited bool   `dd:",+omitempty"`
	Cwd           string `dd:",+omitempty"`
}

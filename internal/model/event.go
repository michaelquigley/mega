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

// Event is one entry in a conversation's append-only history.
type Event struct {
	Seq    int64
	At     time.Time
	Kind   string
	Origin string
	Text   string   `dd:",+omitempty"`
	Req    *Request `dd:",+omitempty"`

	// Raw is the harness-native payload, kept for fidelity and shown collapsed.
	Raw any `dd:",+opaque,+nullable,+omitempty"`
}

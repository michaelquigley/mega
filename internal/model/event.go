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
	// StatusModelChanged records a model or effort the harness reported as
	// changed, carried in Facts.Model and Facts.Effort.
	StatusModelChanged = "model_changed"
	// StatusChoiceFilled records the one fill of a conversation created
	// before every conversation held a concrete model, effort and mode:
	// Facts carries what was filled in, and Text where it came from.
	StatusChoiceFilled = "choice_filled"
	// StatusModeRequested records a permission mode the operator requested
	// with no runtime to switch: the request, never a report of what runs.
	StatusModeRequested = "mode_requested"
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

	// Turn is what the turn this turn_end closes asked for and ran with,
	// recorded by the service. turns recorded before it was kept have none.
	Turn *TurnRecord `dd:",+omitempty"`

	// Raw is the harness-native payload, kept for fidelity and shown collapsed.
	Raw any `dd:",+opaque,+nullable,+omitempty"`

	// AfterTurn marks agent output a runtime reports after the turn it
	// belongs to has ended, such as a codex background command finishing.
	// the service records it without opening a turn and counts it as unread
	// at once. it is a signal through the sink, never written to the log.
	AfterTurn bool `dd:"-"`
}

// RuntimeFacts are what a runtime reported about itself when it started.
type RuntimeFacts struct {
	Session        string `dd:",+omitempty"`
	Model          string `dd:",+omitempty"`
	Version        string `dd:",+omitempty"`
	PermissionMode string `dd:",+omitempty"`
	// ModeConfirmed marks a mode report that only confirmed the mode a
	// launch passed, so the history draws no row for it. nothing records it
	// now; logs written before every launch passed a mode carry it.
	ModeConfirmed bool `dd:",+omitempty"`
	// Effort is the effort level the harness reports, where it reports one
	// (codex does; claude does not).
	Effort string `dd:",+omitempty"`
	Cwd    string `dd:",+omitempty"`
	// Environment is what the runtime was launched with, as names and
	// fingerprints only: an environment carries credentials, so no value is
	// ever recorded.
	Environment *EnvironmentFacts `dd:",+omitempty"`
}

// environment sources: where a runtime's activation came from.
const (
	EnvSourceNone     = "none"     // no file: the shell's own environment
	EnvSourceFile     = "file"     // the checkout's approved .mega/environment.yaml
	EnvSourceIgnored  = "ignored"  // the operator's override ignores the file
	EnvSourceOverride = "override" // the operator's own activation
)

// EnvironmentFacts is the environment a runtime started in: where its
// activation came from and that activation's fingerprint, the names of the
// variables the activation changed against the shell's own environment, and
// the fingerprint of the whole.
type EnvironmentFacts struct {
	Source      string
	Activation  string   `dd:",+omitempty"`
	Changed     []string `dd:",+omitempty"`
	Fingerprint string
}

// Clone returns a copy of e that shares nothing mutable with it: the request,
// the facts and the raw payload are copied all the way down.
func (e *Event) Clone() *Event {
	if e == nil {
		return nil
	}
	c := *e
	if e.Req != nil {
		req := e.Req.Clone()
		c.Req = &req
	}
	if e.Facts != nil {
		facts := *e.Facts
		if e.Facts.Environment != nil {
			env := *e.Facts.Environment
			env.Changed = append([]string(nil), env.Changed...)
			facts.Environment = &env
		}
		c.Facts = &facts
	}
	c.Raw = clonePayload(e.Raw)
	return &c
}

// TurnSettings is a model, effort and permission mode: what a turn asked
// for, or what it ran with. Mode is empty for a harness with no permission
// mode (codex), and Effort for a model that takes no effort level.
type TurnSettings struct {
	Model  string `dd:",+omitempty"`
	Effort string `dd:",+omitempty"`
	Mode   string `dd:",+omitempty"`
}

// TurnRecord is what one turn asked for and ran with. Asked is the
// conversation's choice when the turn began, which mega passed. Ran is what
// the harness reported for the turn, as it stood when the turn's first output
// arrived: the model and, where the harness reports them, the effort and the
// mode. claude never reports effort, so its Ran.Effort is the effort sent.
// Differs names the fields in which they disagree: model, effort, mode.
type TurnRecord struct {
	Asked   TurnSettings
	Ran     TurnSettings
	Differs []string `dd:",+omitempty"`
}

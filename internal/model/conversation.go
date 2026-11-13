package model

const (
	HarnessClaude = "claude"
	HarnessCodex  = "codex"

	// ModeLongLived is the only runtime mode in this slice.
	ModeLongLived = "long-lived"
)

// PermissionModes are the claude permission modes an operator may choose for
// a conversation. the bypass modes are never offered.
var PermissionModes = []string{"default", "auto", "acceptEdits", "plan"}

// PermissionModeDefault is the manual mode: every approval the checkout's
// rules do not settle reaches the operator.
const PermissionModeDefault = "default"

// runtime states a conversation records, as the service last confirmed them.
const (
	// RuntimeNone: no runtime, or one the service saw exit.
	RuntimeNone = "none"
	// RuntimeRunning: a runtime was started and has not been seen to exit.
	RuntimeRunning = "running"
	// RuntimeUnsupervised: found running at service start. the runtime was not
	// seen to exit, so execution is uncertain until the operator resumes.
	RuntimeUnsupervised = "unsupervised"
)

// Conversation is a durable identity wrapped around a native harness session.
type Conversation struct {
	ID        string
	ProjectID string
	Name      string
	Label     string
	Harness   string
	NativeID  string // claude session id or codex thread id
	Mode      string
	Runtime   string // RuntimeNone, RuntimeRunning or RuntimeUnsupervised

	// PermissionMode, Model and Effort are what the operator requests for the
	// conversation, passed on every launch, and to codex on every prompt. a
	// new conversation copies them once from the harness's settings; after
	// that only the operator changes them. PermissionMode is empty for a
	// harness with no permission mode (codex), and Effort for a model that
	// takes no effort level. a conversation created before every one held
	// them may still lack one until its one fill (service.fillChoicesLocked).
	PermissionMode string `dd:",+omitempty"`
	Model          string `dd:",+omitempty"`
	Effort         string `dd:",+omitempty"`
	ReadSeq        int64  // reading position
	OutputSeq      int64  // seq of the latest agent output in a completed turn; drives unread

	// LastSeq is the newest event's seq, for replay. it is read from the event
	// log rather than stored, so appending an event never rewrites the registry.
	LastSeq int64 `dd:"-"`
}

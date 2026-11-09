package model

const (
	HarnessClaude = "claude"
	HarnessCodex  = "codex"

	// ModeLongLived is the only runtime mode in this slice.
	ModeLongLived = "long-lived"
)

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
	ReadSeq   int64  // reading position
	OutputSeq int64  // seq of the latest agent output in a completed turn; drives unread

	// LastSeq is the newest event's seq, for replay. it is read from the event
	// log rather than stored, so appending an event never rewrites the registry.
	LastSeq int64 `dd:"-"`
}

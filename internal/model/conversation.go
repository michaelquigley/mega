package model

const (
	HarnessClaude = "claude"
	HarnessCodex  = "codex"

	// ModeLongLived is the only runtime mode in this slice.
	ModeLongLived = "long-lived"
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
	ReadSeq   int64 // reading position
	LastSeq   int64 // last event seq, for replay
	OutputSeq int64 // seq of the latest agent output in a completed turn; drives unread
}

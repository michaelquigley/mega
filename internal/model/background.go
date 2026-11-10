package model

import "time"

// BackgroundJob is one piece of work a runtime keeps running after its turn
// ended: a backgrounded shell command or subagent (claude), or a command left
// running as a terminal session (codex). it is held in memory while the
// runtime runs, and never stored.
type BackgroundJob struct {
	ID          string
	Description string
	// Kind is what the harness calls it, such as local_bash or local_agent
	// (claude) or command (codex).
	Kind string
	// StartedAt is when the adapter first saw the job: neither harness
	// reports a start time.
	StartedAt time.Time
}

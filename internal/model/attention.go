package model

// attention states. attention is computed from confirmed facts every time it
// is read and never stored.
const (
	// AttentionRequest: a question or approval is pending.
	AttentionRequest = "request"
	// AttentionUnsupervised: the service restarted without seeing the runtime
	// exit, so whether it is still working is uncertain.
	AttentionUnsupervised = "unsupervised"
	// AttentionRunning: a turn is in flight.
	AttentionRunning = "running"
	// AttentionError: the last runtime ended in an error, exited during a
	// turn, or failed to launch.
	AttentionError = "error"
	// AttentionUnread: recorded agent output the operator has not read.
	AttentionUnread = "unread"
	// AttentionIdle: nothing waits on the operator.
	AttentionIdle = "idle"
)

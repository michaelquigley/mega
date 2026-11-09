package model

// request kinds.
const (
	RequestQuestion   = "question"
	RequestPermission = "permission"
)

// request outcomes. a pending request has an empty outcome.
const (
	OutcomeAnswered = "answered"
	OutcomeAborted  = "aborted"
	OutcomeLost     = "lost"
)

// Request is a question or approval from a harness. ID is minted by the service
// and unique across runtime incarnations; adapters keep the native id private.
type Request struct {
	ID      string
	Kind    string
	Payload any    `dd:",+opaque,+nullable,+omitempty"`
	Outcome string `dd:",+omitempty"`
}

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

// Clone returns a copy of r whose payload shares nothing mutable with r's:
// maps and lists, as decoded JSON holds them, are copied all the way down.
func (r Request) Clone() Request {
	c := r
	c.Payload = clonePayload(r.Payload)
	return c
}

func clonePayload(v any) any {
	switch v := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, e := range v {
			out[k] = clonePayload(e)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = clonePayload(e)
		}
		return out
	case map[string]string:
		out := make(map[string]string, len(v))
		for k, e := range v {
			out[k] = e
		}
		return out
	case []string:
		return append([]string(nil), v...)
	case []map[string]any:
		out := make([]map[string]any, len(v))
		for i, e := range v {
			out[i] = clonePayload(e).(map[string]any)
		}
		return out
	default:
		return v
	}
}

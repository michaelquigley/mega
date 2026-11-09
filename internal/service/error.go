package service

import "errors"

// error codes, stable across the API.
const (
	CodeInvalid  = "invalid"
	CodeNotFound = "not_found"
	CodeConflict = "conflict"
	// CodeUnavailable refuses a change the service cannot make durable until
	// it restarts.
	CodeUnavailable = "unavailable"
	// CodeNotPending rejects an answer to a request that is no longer pending.
	CodeNotPending = "not_pending"
)

// Error is a failure the caller can act on, as opposed to an internal fault.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string {
	return e.Message
}

func newError(code, message string) error {
	return &Error{Code: code, Message: message}
}

// ErrorCode returns the code of err, or an empty string for an internal fault.
func ErrorCode(err error) string {
	var svcErr *Error
	if errors.As(err, &svcErr) {
		return svcErr.Code
	}
	return ""
}

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
	// CodeCheckoutMissing refuses work in a project whose checkout is gone,
	// not a directory, or not readable.
	CodeCheckoutMissing = "checkout_missing"
	// CodeExecutableMissing refuses work for a harness that cannot run because
	// something it executes cannot be found from mega's environment.
	CodeExecutableMissing = "executable_missing"
	// CodeEnvironmentUnapproved refuses work in a project whose checkout has
	// a .mega/environment.yaml the operator has never approved.
	CodeEnvironmentUnapproved = "environment_unapproved"
	// CodeEnvironmentChanged refuses work in a project whose
	// .mega/environment.yaml differs from the version the operator approved.
	CodeEnvironmentChanged = "environment_changed"
	// CodeEnvironmentInvalid refuses work in a project whose
	// .mega/environment.yaml cannot be read or is not a valid file.
	CodeEnvironmentInvalid = "environment_invalid"
	// CodeEnvironmentFailed refuses work in a project whose environment could
	// not be resolved: the activation failed, exited or timed out.
	CodeEnvironmentFailed = "environment_failed"
)

// Error is a failure the caller can act on, as opposed to an internal fault.
type Error struct {
	Code    string
	Message string
	// Recorded is what the event log may hold of this failure, when it
	// differs from Message: an environment failure's message carries what
	// the activation printed, which is shown to the operator and never
	// recorded.
	Recorded string
}

// recordable is what an event may say of err.
func recordable(err error) string {
	var svcErr *Error
	if errors.As(err, &svcErr) && svcErr.Recorded != "" {
		return svcErr.Recorded
	}
	return err.Error()
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

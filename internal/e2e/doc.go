// Package e2e holds the harness round trips run by `make e2e`. they drive the
// real harnesses against the disposable fixtures with the operator's logins, so
// they sit behind the e2e build tag and never run in the ordinary gate.
package e2e

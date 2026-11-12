package service

import (
	"errors"
	"os"
	"slices"

	"github.com/michaelquigley/mega/internal/harness"
)

// Preflight is whether each harness can run from mega's own environment:
// what is missing, if anything, and the PATH mega has. it is computed on every
// workspace read and never stored, since installing a tool or restarting mega
// from another environment changes it.
type Preflight struct {
	Path    string
	Missing []harness.MissingError
}

// preflight checks every harness, in name order.
func (s *Service) preflight() Preflight {
	p := Preflight{Path: os.Getenv("PATH")}
	names := make([]string, 0, len(s.harnesses))
	for name := range s.harnesses {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		var missing *harness.MissingError
		if errors.As(s.harnesses[name].Check(), &missing) {
			p.Missing = append(p.Missing, *missing)
		}
	}
	return p
}

// missingCoded turns a missing executable into the refusal the operator
// sees: its own code and the harness's words, never an internal error.
func missingCoded(err error) error {
	var missing *harness.MissingError
	if errors.As(err, &missing) {
		return newError(CodeExecutableMissing, missing.Error())
	}
	return err
}

// harnessReady refuses work for a harness that cannot run now. the harness
// set is fixed at New, so it needs no lock.
func (s *Service) harnessReady(harnessName string) error {
	h, ok := s.harnesses[harnessName]
	if !ok {
		return nil
	}
	return missingCoded(h.Check())
}

// explain names a harness failure that a missing executable caused: an exec
// that could not start (a raw "executable file not found") is checked against
// the harness and reported as the missing executable it was. other failures
// pass through.
func (s *Service) explain(harnessName string, err error) error {
	if err == nil {
		return nil
	}
	if coded := missingCoded(err); coded != err {
		return coded
	}
	if ready := s.harnessReady(harnessName); ready != nil {
		return ready
	}
	return err
}

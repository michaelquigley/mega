package service

import (
	"errors"

	"github.com/michaelquigley/mega/internal/harness"
)

// missingCoded turns a missing executable into the refusal the operator
// sees: its own code and the harness's words, never an internal error.
func missingCoded(err error) error {
	var missing *harness.MissingError
	if errors.As(err, &missing) {
		return newError(CodeExecutableMissing, missing.Error())
	}
	return err
}

// harnessReadyLocked refuses work for a harness that cannot run in the
// project's latest resolved environment. before any resolution there is
// nothing to go on, and a start checks the environment it resolves. callers
// hold s.mu.
func (s *Service) harnessReadyLocked(projectID, harnessName string) error {
	h, ok := s.harnesses[harnessName]
	if !ok {
		return nil
	}
	rec := s.envs[projectID]
	if rec == nil || rec.resolved == nil || rec.resolved.Error != "" {
		return nil
	}
	return missingCoded(h.Check(rec.resolved.env))
}

// explain names a harness failure that a missing executable caused: an exec
// that could not start (a raw "executable file not found") is checked against
// the harness in env, the environment it ran in, and reported as the missing
// executable it was. other failures pass through.
func (s *Service) explain(harnessName string, env []string, err error) error {
	if err == nil {
		return nil
	}
	if coded := missingCoded(err); coded != err {
		return coded
	}
	if h, ok := s.harnesses[harnessName]; ok && env != nil {
		if missing := missingCoded(h.Check(env)); missing != nil {
			return missing
		}
	}
	return err
}

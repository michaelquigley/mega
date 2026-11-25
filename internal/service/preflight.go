package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/michaelquigley/df/dl"

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
// project's latest resolved environment. before any resolution of the
// activation that would run now there is nothing to go on, and a start checks
// the environment it resolves. callers hold s.mu.
func (s *Service) harnessReadyLocked(projectID, harnessName string) error {
	h, ok := s.harnesses[harnessName]
	if !ok {
		return nil
	}
	project, err := s.projectLocked(projectID)
	if err != nil {
		return nil
	}
	r := s.envStateLocked(project).Resolved
	if r == nil || r.Error != "" || r.namesRefused() {
		return nil
	}
	return missingCoded(h.Check(r.before))
}

// explain names a harness failure that a missing executable caused: an exec
// that could not start (a raw "executable file not found") is checked against
// the harness in find, the environment it located its programs in, and
// reported as the missing executable it was. other failures pass through.
func (s *Service) explain(harnessName string, find []string, err error) error {
	if err == nil {
		return nil
	}
	if coded := missingCoded(err); coded != err {
		return coded
	}
	if h, ok := s.harnesses[harnessName]; ok && find != nil {
		if missing := missingCoded(h.Check(find)); missing != nil {
			return missing
		}
	}
	var failed *harnessFailure
	if errors.As(err, &failed) {
		// the whole of it, stack and all, goes to the log; the operator gets
		// the harness's own first words.
		dl.Errorf("'%s' failed: %v", harnessName, err)
		return &Error{Code: CodeHarnessFailed, Message: fmt.Sprintf("'%s' failed: %s; mega's log has the rest", harnessName, summarize(failed.err.Error()))}
	}
	return err
}

// harnessFailure marks an error a harness itself returned, from listing its
// models, discovering or reading sessions, or starting a runtime, so explain
// can tell it from a fault of mega's own.
type harnessFailure struct {
	harness string
	err     error
}

func (f *harnessFailure) Error() string { return f.err.Error() }
func (f *harnessFailure) Unwrap() error { return f.err }

// harnessFault marks err as the harness's own failure; nil stays nil.
func harnessFault(harnessName string, err error) error {
	if err == nil {
		return nil
	}
	return &harnessFailure{harness: harnessName, err: err}
}

// summarizeLimit bounds what a harness failure shows the operator.
const summarizeLimit = 300

// summarize is a harness failure as the page may show it: its first line,
// and the error line a stack trace leads with when the first line is not
// one, cut to summarizeLimit; never the frames of a stack.
func summarize(message string) string {
	var lines []string
	for _, line := range strings.Split(message, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "at ") || strings.HasPrefix(line, "^") || strings.HasPrefix(line, "file://") || strings.HasPrefix(line, "node:") {
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return "it gave no reason"
	}
	out := lines[0]
	if !strings.Contains(out, "Error") {
		for _, line := range lines[1:] {
			if strings.Contains(line, "Error") {
				out += " " + line
				break
			}
		}
	}
	if r := []rune(out); len(r) > summarizeLimit {
		out = string(r[:summarizeLimit]) + "…"
	}
	return out
}

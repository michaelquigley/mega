package service

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"

	"github.com/michaelquigley/mega/internal/model"
)

// the reasons a project's checkout cannot be worked in.
const (
	CheckoutDoesNotExist = "does not exist"
	CheckoutNotDirectory = "not a directory"
	CheckoutNotReadable  = "not readable"
)

// checkoutProblem says why a checkout cannot be worked in now, or returns
// empty when it can. the path was validated at registration, but it can move,
// be removed, or have been set by hand, so it is checked whenever a project is
// read and before any work starts there. it reads names and modes only.
func checkoutProblem(checkout string) string {
	info, err := os.Stat(checkout)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return CheckoutDoesNotExist
	case err != nil:
		// a parent that cannot be searched: the directory cannot be reached.
		return CheckoutNotReadable
	case !info.IsDir():
		return CheckoutNotDirectory
	}
	// a harness lists the directory and runs in it, so it must be readable
	// and searchable by the service's user.
	if err := syscall.Access(checkout, 0o4|0o1); err != nil {
		return CheckoutNotReadable
	}
	return ""
}

// checkoutRefusal is the refusal of work in a project whose checkout cannot
// be worked in, naming the path and why, or nil.
func checkoutRefusal(project model.Project) error {
	problem := checkoutProblem(project.Checkout)
	if problem == "" {
		return nil
	}
	return newError(CodeCheckoutMissing, fmt.Sprintf("the checkout is missing: '%s' (%s)", project.Checkout, problem))
}

// checkoutLocked refuses work in a project whose checkout cannot be worked
// in. a directory moving away announces nothing, so a refusal is how the
// service learns of it: it announces a workspace change, and every page reads
// the notice at once instead of at its next read. callers hold s.mu.
func (s *Service) checkoutLocked(project model.Project) error {
	err := checkoutRefusal(project)
	if err != nil {
		s.changed()
	}
	return err
}

// conversationCheckoutLocked is checkoutLocked for a conversation's project.
// callers hold s.mu.
func (s *Service) conversationCheckoutLocked(conv model.Conversation) error {
	project, err := s.projectLocked(conv.ProjectID)
	if err != nil {
		return err
	}
	return s.checkoutLocked(project)
}

// Package service is the single authority for workspace state. every write goes
// through it, and it is the only writer of the registry and the event logs.
package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"git.hq.quigley.com/research/roundhouse/internal/harness"
	"git.hq.quigley.com/research/roundhouse/internal/model"
	"git.hq.quigley.com/research/roundhouse/internal/registry"
)

// UntrustedChecker reports whether a checkout should carry the trust notice.
type UntrustedChecker func(checkout string) bool

// Options are the service's collaborators.
type Options struct {
	IsUntrusted UntrustedChecker
	// InheritedMode returns the claude permission mode the operator's settings
	// name for a checkout, or empty.
	InheritedMode func(checkout string) string
	EventLogPath  func(conversationID string) string
	Harnesses     []harness.Harness
}

// Workspace is the arrangement as the service last confirmed it.
type Workspace struct {
	Projects      []model.Project
	Conversations []ConversationView

	// NeedsYou lists the conversations waiting on the operator, in the order
	// to take them: pending requests first, then unread output, oldest first.
	NeedsYou []string
}

// Service owns the registry, the event logs, the runtimes and their pending
// requests, and fans confirmed changes out to subscribers.
type Service struct {
	mu          sync.Mutex
	reg         *registry.Registry
	isUntrusted UntrustedChecker
	inherited   func(checkout string) string
	logPath     func(conversationID string) string
	harnesses   map[string]harness.Harness
	live        map[string]*live
	subs        map[*Subscription]struct{}
	closed      bool
	catalogs    catalogs // model lists, per harness, for the service's life
}

// New returns a service over reg. it recovers from the previous run before
// returning: requests that were pending are resolved lost, and conversations
// whose runtime was not seen to exit become unsupervised.
func New(reg *registry.Registry, opts Options) (*Service, error) {
	s := &Service{
		reg:         reg,
		isUntrusted: opts.IsUntrusted,
		inherited:   opts.InheritedMode,
		logPath:     opts.EventLogPath,
		harnesses:   make(map[string]harness.Harness),
		live:        make(map[string]*live),
		subs:        make(map[*Subscription]struct{}),
	}
	if s.isUntrusted == nil {
		s.isUntrusted = func(string) bool { return false }
	}
	if s.inherited == nil {
		s.inherited = func(string) string { return "" }
	}
	for _, h := range opts.Harnesses {
		s.harnesses[h.Name()] = h
	}
	if err := s.recover(); err != nil {
		s.closeLogs()
		return nil, err
	}
	return s, nil
}

// Workspace returns the current arrangement. trust notices are recomputed on
// each read, so a terminal visit clears the notice without a restart.
func (s *Service) Workspace() Workspace {
	s.mu.Lock()
	doc := s.reg.Snapshot()
	conversations := make([]ConversationView, 0, len(doc.Conversations))
	for _, conv := range doc.Conversations {
		conversations = append(conversations, s.viewLocked(conv))
	}
	s.mu.Unlock()
	for i := range doc.Projects {
		s.applyTrust(&doc.Projects[i])
	}
	return Workspace{Projects: doc.Projects, Conversations: conversations, NeedsYou: needsYou(conversations)}
}

// RegisterProject adds an existing checkout to the workspace. an empty name
// falls back to the checkout's directory name.
func (s *Service) RegisterProject(name, checkout string) (model.Project, error) {
	checkout, err := validateCheckout(checkout)
	if err != nil {
		return model.Project{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = filepath.Base(checkout)
	}
	project := model.Project{ID: registry.NewID(), Name: name, Checkout: checkout}

	s.mu.Lock()
	defer s.mu.Unlock()
	err = s.update(func(doc *registry.Document) error {
		for _, existing := range doc.Projects {
			if existing.Checkout == checkout {
				return newError(CodeConflict, fmt.Sprintf("checkout '%s' is already registered as '%s'", checkout, existing.Name))
			}
		}
		doc.Projects = append(doc.Projects, project)
		return nil
	})
	if err != nil {
		return model.Project{}, err
	}
	s.applyTrust(&project)
	return project, nil
}

// RenameProject changes a project's display name.
func (s *Service) RenameProject(id, name string) (model.Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.Project{}, newError(CodeInvalid, "name is required")
	}
	var project model.Project
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.update(func(doc *registry.Document) error {
		i := slices.IndexFunc(doc.Projects, func(p model.Project) bool { return p.ID == id })
		if i < 0 {
			return newError(CodeNotFound, fmt.Sprintf("no project '%s'", id))
		}
		doc.Projects[i].Name = name
		project = doc.Projects[i]
		return nil
	})
	if err != nil {
		return model.Project{}, err
	}
	s.applyTrust(&project)
	return project, nil
}

// RemoveProject takes a project and its conversations out of the workspace,
// and deletes those conversations' event logs. it never touches the checkout
// or the native sessions, which discovery lists again if the checkout is
// registered again. it refuses while any of its conversations has a runtime in
// any state, as removing that conversation alone would. the project and its
// conversations leave the registry in one confirmed write, and only then are
// the logs deleted, in the same order and with the same failure handling as a
// single conversation's removal.
func (s *Service) RemoveProject(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.projectLocked(id); err != nil {
		return err
	}
	var removed []string
	for _, conv := range s.reg.Snapshot().Conversations {
		if conv.ProjectID != id {
			continue
		}
		if reason := s.busyReasonLocked(conv); reason != "" {
			return newError(CodeConflict, fmt.Sprintf("conversation '%s' %s and its project", conv.Name, reason))
		}
		removed = append(removed, conv.ID)
	}
	err := s.update(func(doc *registry.Document) error {
		i := slices.IndexFunc(doc.Projects, func(p model.Project) bool { return p.ID == id })
		if i < 0 {
			return newError(CodeNotFound, fmt.Sprintf("no project '%s'", id))
		}
		doc.Projects = slices.Delete(doc.Projects, i, i+1)
		doc.Conversations = slices.DeleteFunc(doc.Conversations, func(c model.Conversation) bool {
			return c.ProjectID == id
		})
		return nil
	})
	if err != nil {
		return err
	}
	s.dropLogsLocked(removed...)
	return nil
}

// Subscribe starts delivery of updates. after the service closes, the returned
// subscription's channel is already closed.
func (s *Service) Subscribe() *Subscription {
	sub := &Subscription{svc: s, updates: make(chan Update, updateBuffer)}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		close(sub.updates)
		return sub
	}
	s.subs[sub] = struct{}{}
	return sub
}

// update writes through the registry and announces the confirmed change.
// callers hold s.mu. a registry whose durability is uncertain refuses the write
// as unavailable rather than as an internal fault, since it has already logged
// the condition once. nothing was confirmed, so nothing is announced.
func (s *Service) update(fn func(*registry.Document) error) error {
	err := s.reg.Update(fn)
	if errors.Is(err, registry.ErrUncertain) {
		return newError(CodeUnavailable, err.Error())
	}
	if err != nil {
		return err
	}
	s.publish(Update{WorkspaceChanged: true})
	return nil
}

// publish delivers to every subscriber without blocking. callers hold s.mu, so
// subscribers see updates in the order they were confirmed.
func (s *Service) publish(update Update) {
	for sub := range s.subs {
		select {
		case sub.updates <- update:
		default:
			s.drop(sub)
		}
	}
}

// drop removes and closes a subscription. callers hold s.mu.
func (s *Service) drop(sub *Subscription) {
	if _, ok := s.subs[sub]; !ok {
		return
	}
	delete(s.subs, sub)
	close(sub.updates)
}

// applyTrust fills what is read from the operator's claude configuration each
// time a project is read: the trust notice and the inherited permission mode.
func (s *Service) applyTrust(project *model.Project) {
	project.Untrusted = s.isUntrusted(project.Checkout)
	project.InheritedPermissionMode = s.inherited(project.Checkout)
}

func validateCheckout(checkout string) (string, error) {
	checkout = strings.TrimSpace(checkout)
	if checkout == "" {
		return "", newError(CodeInvalid, "checkout is required")
	}
	if !filepath.IsAbs(checkout) {
		return "", newError(CodeInvalid, fmt.Sprintf("checkout '%s' must be an absolute path", checkout))
	}
	checkout = filepath.Clean(checkout)
	info, err := os.Stat(checkout)
	if err != nil {
		return "", newError(CodeInvalid, fmt.Sprintf("checkout '%s' is not accessible here: %v", checkout, err))
	}
	if !info.IsDir() {
		return "", newError(CodeInvalid, fmt.Sprintf("checkout '%s' is not a directory", checkout))
	}
	return checkout, nil
}

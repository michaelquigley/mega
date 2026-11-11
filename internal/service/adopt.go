package service

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/michaelquigley/df/dl"

	"git.hq.quigley.com/research/roundhouse/internal/harness"
	"git.hq.quigley.com/research/roundhouse/internal/model"
	"git.hq.quigley.com/research/roundhouse/internal/registry"
)

const discoverTimeout = time.Minute

// Discover lists the native sessions stored for a project's checkout that no
// conversation holds yet, for the operator to choose from.
func (s *Service) Discover(projectID, harnessName string) ([]harness.Session, error) {
	s.mu.Lock()
	project, err := s.projectLocked(projectID)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	h, ok := s.harnesses[harnessName]
	if !ok {
		s.mu.Unlock()
		return nil, newError(CodeInvalid, fmt.Sprintf("harness '%s' is not available", harnessName))
	}
	held := s.heldLocked(harnessName)
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), discoverTimeout)
	defer cancel()
	sessions, err := h.Discover(ctx, project.Checkout)
	if err != nil {
		return nil, fmt.Errorf("discovering sessions for '%s': %w", projectID, err)
	}
	return slices.DeleteFunc(sessions, func(session harness.Session) bool { return held[session.NativeID] }), nil
}

// heldLocked is the set of native ids some conversation is bound to. callers
// hold s.mu.
func (s *Service) heldLocked(harnessName string) map[string]bool {
	held := make(map[string]bool)
	for _, conv := range s.reg.Snapshot().Conversations {
		if conv.Harness != harnessName {
			continue
		}
		session := conv.NativeID
		if l := s.live[conv.ID]; l != nil {
			session = l.sessionOf(conv)
		}
		if session != "" {
			held[session] = true
		}
	}
	return held
}

// Adopt binds a native session the operator chose from discovery to a new
// conversation and imports its history. it starts no runtime; the next prompt
// resumes the session by its id. the session must be one discovery lists for
// the checkout and no conversation may hold it already.
func (s *Service) Adopt(projectID, harnessName, nativeID, name string) (ConversationView, error) {
	nativeID = strings.TrimSpace(nativeID)
	if nativeID == "" {
		return ConversationView{}, newError(CodeInvalid, "a native session id is required")
	}
	sessions, err := s.Discover(projectID, harnessName)
	if err != nil {
		return ConversationView{}, err
	}
	i := slices.IndexFunc(sessions, func(session harness.Session) bool { return session.NativeID == nativeID })
	if i < 0 {
		return ConversationView{}, newError(CodeInvalid, fmt.Sprintf("session '%s' is not an unadopted '%s' session of this project", nativeID, harnessName))
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = defaultName(sessions[i].Preview)
	}
	if name == "" {
		name = harnessName + " session"
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ConversationView{}, newError(CodeUnavailable, "the service is shutting down")
	}
	if err := s.refuseNewWorkLocked(); err != nil {
		s.mu.Unlock()
		return ConversationView{}, err
	}
	if s.heldLocked(harnessName)[nativeID] {
		s.mu.Unlock()
		return ConversationView{}, newError(CodeConflict, fmt.Sprintf("session '%s' was adopted meanwhile", nativeID))
	}
	conv := model.Conversation{
		ID:        registry.NewID(),
		ProjectID: projectID,
		Name:      name,
		Harness:   harnessName,
		NativeID:  nativeID,
		Mode:      model.ModeLongLived,
		Runtime:   model.RuntimeNone,
	}
	if err := s.update(func(doc *registry.Document) error {
		doc.Conversations = append(doc.Conversations, conv)
		return nil
	}); err != nil {
		s.mu.Unlock()
		return ConversationView{}, err
	}
	l, err := s.liveLocked(conv.ID)
	if err != nil {
		s.mu.Unlock()
		return ConversationView{}, err
	}
	if err := s.transition(l, &model.Event{Status: model.StatusResuming, Text: fmt.Sprintf("adopting session '%s'", nativeID)}, StateResuming, nil); err != nil {
		s.mu.Unlock()
		return s.view(conv.ID), err
	}
	l.busy = true
	s.mu.Unlock()

	result, importErr := s.importHistory(conv, l)

	s.mu.Lock()
	l.busy = false
	if importErr != nil {
		if err := s.transition(l, &model.Event{Status: model.StatusError, Text: fmt.Sprintf("native history could not be read: %v", importErr)}, "", nil); err != nil {
			dl.Errorf("%v", err)
		}
		s.mu.Unlock()
		return s.view(conv.ID), importErr
	}
	// adopting is the operator's choice of a session they know, so what it
	// brings in is not unread.
	err = s.transition(l, &model.Event{Status: model.StatusResumed, Text: fmt.Sprintf("%d messages added from native history", result.added)}, "", nil)
	s.mu.Unlock()
	return s.view(conv.ID), err
}

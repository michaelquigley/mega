package service

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/michaelquigley/mega/internal/events"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/registry"
)

const discoverTimeout = time.Minute

// Discover lists the native sessions stored for a project's checkout that no
// conversation holds yet, for the operator to choose from.
func (s *Service) Discover(projectID, harnessName string) ([]harness.Session, error) {
	found, err := s.DiscoverHeld(projectID, harnessName)
	return found.Sessions, err
}

// Discovered is what discovery found in a checkout for one harness: the
// sessions no conversation holds, and how many more it found that a
// conversation already holds, which are left out.
type Discovered struct {
	Sessions []harness.Session
	Held     int
}

// DiscoverHeld is Discover, with the count of the sessions it left out
// because a conversation already holds them.
func (s *Service) DiscoverHeld(projectID, harnessName string) (Discovered, error) {
	s.mu.Lock()
	project, err := s.projectLocked(projectID)
	if err != nil {
		s.mu.Unlock()
		return Discovered{}, err
	}
	h, ok := s.harnesses[harnessName]
	if !ok {
		s.mu.Unlock()
		return Discovered{}, newError(CodeInvalid, fmt.Sprintf("harness '%s' is not available", harnessName))
	}
	held := s.heldLocked(harnessName)
	s.mu.Unlock()
	// a checkout that cannot be worked in has nothing to offer for adoption;
	// the project's own notice says why.
	if checkoutProblem(project.Checkout) != "" {
		return Discovered{Sessions: []harness.Session{}}, nil
	}
	// discovery runs the harness in the project's environment, which must be
	// one mega may run.
	place, err := s.placeFor(projectID)
	if err != nil {
		return Discovered{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), discoverTimeout)
	defer cancel()
	sessions, err := h.Discover(ctx, place)
	if err != nil {
		return Discovered{}, s.explain(harnessName, place.Find, fmt.Errorf("discovering sessions for '%s': %w", projectID, harnessFault(harnessName, err)))
	}
	before := len(sessions)
	sessions = slices.DeleteFunc(sessions, func(session harness.Session) bool { return held[session.NativeID] })
	return Discovered{Sessions: sessions, Held: before - len(sessions)}, nil
}

// Look reads how a session in the project's checkout began and how it left
// off, for the operator choosing which to adopt: the harness's adapter reads
// its own store, as discovery and an import do, in the project's
// environment. it changes nothing: nothing is imported, no conversation or
// registry entry is made, no attention moves, and nothing of what it read is
// logged. what it returns is bounded (harness.LookOf) and goes only to the
// caller.
func (s *Service) Look(projectID, harnessName, nativeID string) (harness.Look, error) {
	if strings.TrimSpace(nativeID) == "" {
		return harness.Look{}, newError(CodeInvalid, "a session id is required")
	}
	s.mu.Lock()
	project, err := s.projectLocked(projectID)
	if err != nil {
		s.mu.Unlock()
		return harness.Look{}, err
	}
	h, ok := s.harnesses[harnessName]
	s.mu.Unlock()
	if !ok {
		return harness.Look{}, newError(CodeInvalid, fmt.Sprintf("harness '%s' is not available", harnessName))
	}
	looker, ok := h.(harness.Looker)
	if !ok {
		return harness.Look{}, newError(CodeInvalid, fmt.Sprintf("harness '%s' cannot show a session before it is adopted", harnessName))
	}
	if problem := checkoutProblem(project.Checkout); problem != "" {
		return harness.Look{}, newError(CodeConflict, problem)
	}
	place, err := s.placeFor(projectID)
	if err != nil {
		return harness.Look{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), discoverTimeout)
	defer cancel()
	look, err := looker.Look(ctx, place, nativeID)
	if err != nil {
		return harness.Look{}, s.explain(harnessName, place.Find, fmt.Errorf("reading session '%s' for '%s': %w", nativeID, projectID, harnessFault(harnessName, err)))
	}
	return look, nil
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
	s.mu.Lock()
	project, err := s.projectLocked(projectID)
	if err == nil {
		err = s.checkoutLocked(project)
	}
	s.mu.Unlock()
	if err != nil {
		return ConversationView{}, err
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
	// an adopted conversation's request is copied once from the harness's
	// settings, as a new conversation's is. claude settings that name no
	// effort leave it to be chosen on the control bar before the first
	// launch (errChooseEffort).
	copyPlace, facts, err := s.placeWithFacts(projectID)
	if err != nil {
		return ConversationView{}, err
	}
	copyCtx, cancelCopy := context.WithTimeout(context.Background(), modelsTimeout)
	asked, _, err := s.copyFor(copyCtx, harnessName, copyPlace, model.TurnSettings{})
	cancelCopy()
	if err != nil {
		return ConversationView{}, err
	}

	snapshot, err := s.readNativeHistory(projectID, harnessName, nativeID, copyPlace, facts)
	if err != nil {
		return ConversationView{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ConversationView{}, newError(CodeUnavailable, "the service is shutting down")
	}
	if err := s.refuseNewWorkLocked(); err != nil {
		return ConversationView{}, err
	}
	if _, err := s.projectLocked(projectID); err != nil {
		return ConversationView{}, err
	}
	if s.heldLocked(harnessName)[nativeID] {
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

		PermissionMode: asked.Mode,
		Model:          asked.Model,
		Effort:         asked.Effort,
	}
	if err := s.writeSnapshot(&conv, snapshot); err != nil {
		return ConversationView{}, err
	}
	if _, err := s.liveLocked(conv.ID); err != nil {
		return ConversationView{}, err
	}
	if err := s.update(func(doc *registry.Document) error {
		doc.Conversations = append(doc.Conversations, conv)
		return nil
	}); err != nil {
		// The rename may have survived even if its directory sync failed.
		// Keep the complete log so either result is safe after restart.
		s.live[conv.ID].log.Close()
		delete(s.live, conv.ID)
		return ConversationView{}, err
	}
	return s.viewLocked(conv), nil
}

// writeSnapshot completes the log before the conversation is published.
func (s *Service) writeSnapshot(conv *model.Conversation, snapshot []*model.Event) error {
	log, err := events.Open(s.logPath(conv.ID))
	if err != nil {
		return err
	}
	defer log.Close()
	seen := make(map[string]bool)
	for _, ev := range snapshot {
		if ev.NativeID == "" || seen[ev.NativeID] {
			continue
		}
		ev = ev.Clone()
		ev.Origin = model.OriginNative
		if err := s.appendEvent(conv.ID, log, ev); err != nil {
			return err
		}
		seen[ev.NativeID] = true
	}
	conv.ReadSeq = log.LastSeq()
	return nil
}

// NativeTranscript reads speech from the exact bound session on demand.
func (s *Service) NativeTranscript(conversationID string) ([]*model.Event, error) {
	s.mu.Lock()
	conv, err := s.conversationLocked(conversationID)
	if err == nil {
		conv = s.viewLocked(conv).Conversation
		err = s.conversationCheckoutLocked(conv)
	}
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if conv.NativeID == "" {
		return nil, errSessionUnknown
	}
	place, facts, err := s.placeWithFacts(conv.ProjectID)
	if err != nil {
		return nil, err
	}
	evs, err := s.readNativeHistory(conv.ProjectID, conv.Harness, conv.NativeID, place, facts)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(evs, func(ev *model.Event) bool {
		return ev.Kind != model.EventUser && ev.Kind != model.EventAssistant
	}), nil
}

func (s *Service) readNativeHistory(projectID, harnessName, nativeID string, place harness.Place, facts *model.EnvironmentFacts) ([]*model.Event, error) {
	s.mu.Lock()
	err := s.harnessGateLocked(projectID, facts)
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	h, ok := s.harnesses[harnessName]
	if !ok {
		return nil, newError(CodeInvalid, fmt.Sprintf("harness '%s' is not available", harnessName))
	}
	ctx, cancel := context.WithTimeout(context.Background(), historyTimeout)
	defer cancel()
	history, err := h.History(ctx, place, nativeID)
	if err != nil {
		return nil, s.explain(harnessName, place.Find, fmt.Errorf("reading native transcript: %w", harnessFault(harnessName, err)))
	}
	return history.Events, nil
}

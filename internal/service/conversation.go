package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/michaelquigley/df/dl"

	"git.hq.quigley.com/research/roundhouse/internal/harness"
	"git.hq.quigley.com/research/roundhouse/internal/model"
	"git.hq.quigley.com/research/roundhouse/internal/registry"
)

const (
	historyTimeout = 2 * time.Minute
	maxNameRunes   = 48
	denyMessage    = "the operator denied this in roundhouse"
)

// StartConversation launches a new conversation in a project and sends its
// opening prompt.
func (s *Service) StartConversation(projectID, harnessName, name, prompt string) (ConversationView, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return ConversationView{}, newError(CodeInvalid, "an opening prompt is required")
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ConversationView{}, newError(CodeUnavailable, "the service is shutting down")
	}
	if _, ok := s.harnesses[harnessName]; !ok {
		s.mu.Unlock()
		return ConversationView{}, newError(CodeInvalid, fmt.Sprintf("harness '%s' is not available", harnessName))
	}
	if _, err := s.projectLocked(projectID); err != nil {
		s.mu.Unlock()
		return ConversationView{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = defaultName(prompt)
	}
	conv := model.Conversation{
		ID:        registry.NewID(),
		ProjectID: projectID,
		Name:      name,
		Harness:   harnessName,
		Mode:      model.ModeLongLived,
		Runtime:   model.RuntimeNone,
	}
	err := s.update(func(doc *registry.Document) error {
		doc.Conversations = append(doc.Conversations, conv)
		return nil
	})
	if err != nil {
		s.mu.Unlock()
		return ConversationView{}, err
	}
	l, err := s.liveLocked(conv.ID)
	if err != nil {
		s.mu.Unlock()
		return ConversationView{}, err
	}
	if err := s.beginStartLocked(l); err != nil {
		s.mu.Unlock()
		return s.view(conv.ID), err
	}
	err = s.startRuntimeLocked(conv.ID, l)
	s.mu.Unlock()
	if err != nil {
		return s.view(conv.ID), err
	}
	if err := s.send(conv.ID, prompt); err != nil {
		return s.view(conv.ID), err
	}
	return s.view(conv.ID), nil
}

// Prompt sends the operator's text to a conversation, starting a runtime that
// resumes its exact native session if none is running.
func (s *Service) Prompt(conversationID, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return newError(CodeInvalid, "the prompt is empty")
	}
	s.mu.Lock()
	conv, l, err := s.idleCheckLocked(conversationID)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	if l.turnActive {
		s.mu.Unlock()
		return newError(CodeConflict, "a turn is in progress; wait for it or interrupt it")
	}
	if l.runtime != nil {
		s.mu.Unlock()
		return s.send(conversationID, text)
	}
	// no runtime: bring the cache up to date with anything written natively
	// since the last one stopped, then resume by exact id.
	if err := s.beginStartLocked(l); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()

	if conv.NativeID != "" {
		if _, err := s.importHistory(conv, l); err != nil {
			s.mu.Lock()
			l.busy = false
			if recordErr := s.transition(l, &model.Event{Status: model.StatusError, Text: fmt.Sprintf("native history could not be read, so no runtime was started: %v", err)}, "", nil); recordErr != nil {
				dl.Errorf("%v", recordErr)
			}
			s.mu.Unlock()
			return err
		}
	}
	s.mu.Lock()
	err = s.startRuntimeLocked(conversationID, l)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return s.send(conversationID, text)
}

// idleCheckLocked finds a conversation that can take an operator action that
// needs its runtime settled. callers hold s.mu.
func (s *Service) idleCheckLocked(conversationID string) (model.Conversation, *live, error) {
	if s.closed {
		return model.Conversation{}, nil, newError(CodeUnavailable, "the service is shutting down")
	}
	conv, err := s.conversationLocked(conversationID)
	if err != nil {
		return conv, nil, err
	}
	if conv.Runtime == model.RuntimeUnsupervised {
		return conv, nil, newError(CodeConflict, "supervision was lost when the service restarted and execution is uncertain; resume the conversation first")
	}
	l, err := s.liveLocked(conversationID)
	if err != nil {
		return conv, nil, err
	}
	switch {
	case l.busy:
		return conv, nil, newError(CodeConflict, "the runtime is starting or the conversation is resuming")
	case l.releasing:
		return conv, nil, newError(CodeConflict, "the runtime is stopping")
	}
	return conv, l, nil
}

// beginStartLocked shows a start under way and holds off other starts and
// resumes until it ends. callers hold s.mu.
func (s *Service) beginStartLocked(l *live) error {
	if err := s.transition(l, &model.Event{Status: model.StatusStarting}, StateStarting, nil); err != nil {
		return err
	}
	l.busy = true
	return nil
}

// startRuntimeLocked launches a runtime for the conversation, after
// beginStartLocked. the conversation records running before the process
// starts, so a crash from here on leaves it unsupervised rather than silently
// stopped. it shows starting until the runtime reports the session it holds.
// callers hold s.mu.
func (s *Service) startRuntimeLocked(conversationID string, l *live) error {
	defer func() { l.busy = false }()
	conv, err := s.conversationLocked(conversationID)
	if err != nil {
		s.failedLaunch(l, err)
		return err
	}
	project, err := s.projectLocked(conv.ProjectID)
	if err != nil {
		s.failedLaunch(l, err)
		return err
	}
	h, ok := s.harnesses[conv.Harness]
	if !ok {
		err := newError(CodeInvalid, fmt.Sprintf("harness '%s' is not available", conv.Harness))
		s.failedLaunch(l, err)
		return err
	}
	if err := s.updateConversation(conversationID, func(c *model.Conversation) { c.Runtime = model.RuntimeRunning }); err != nil {
		s.failedLaunch(l, err)
		return err
	}
	inc := &incarnation{exited: make(chan struct{})}
	l.inc = inc
	rt, err := h.Launch(harness.Spec{Cwd: project.Checkout, NativeID: conv.NativeID}, &sink{s: s, l: l, inc: inc})
	if err != nil {
		l.inc = nil
		// like an unrecorded stop, an unrecorded failure leaves the conversation
		// recorded running, so a restart settles it rather than this run
		// claiming what its log could not confirm.
		if s.failedLaunch(l, err) {
			if updateErr := s.updateConversation(conversationID, func(c *model.Conversation) { c.Runtime = model.RuntimeNone }); updateErr != nil {
				dl.Errorf("recording that '%s' did not start: %v", conversationID, updateErr)
			}
		}
		return fmt.Errorf("starting runtime for '%s': %w", conversationID, err)
	}
	l.runtime = rt
	if conv.NativeID == "" {
		if err := s.updateConversation(conversationID, func(c *model.Conversation) { c.NativeID = rt.NativeID() }); err != nil {
			// the session exists but the conversation cannot record it; stop it
			// rather than run a session nothing can resume.
			s.beginStopLocked(l, &model.Event{Status: model.StatusError, Text: fmt.Sprintf("the session could not be recorded; stopping it: %v", err)})
			return err
		}
	}
	return nil
}

// send records the prompt with a service-minted message identity and hands it
// to the runtime, which passes the identity to the harness so a later import
// recognizes the prompt instead of recording it twice.
func (s *Service) send(conversationID, text string) error {
	s.mu.Lock()
	l := s.live[conversationID]
	if l == nil || l.runtime == nil {
		s.mu.Unlock()
		return newError(CodeConflict, "the conversation has no runtime")
	}
	uuid := registry.NewUUID()
	if err := s.record(l, &model.Event{Kind: model.EventUser, Origin: model.OriginManaged, Text: text, NativeID: uuid}); err != nil {
		s.mu.Unlock()
		return err
	}
	l.turnActive = true
	s.changed()
	rt := l.runtime
	s.mu.Unlock()
	if err := rt.Prompt(uuid, text); err != nil {
		return fmt.Errorf("sending prompt to '%s': %w", conversationID, err)
	}
	return nil
}

// Answer delivers the operator's answer to a pending request. the request
// leaves the pending set before the answer is delivered, so a second answer,
// from this browser or another, is rejected as not_pending.
func (s *Service) Answer(conversationID, requestID string, answer harness.Answer) error {
	s.mu.Lock()
	if _, err := s.conversationLocked(conversationID); err != nil {
		s.mu.Unlock()
		return err
	}
	l := s.live[conversationID]
	var req *model.Request
	if l != nil {
		i := slices.IndexFunc(l.pending, func(r *model.Request) bool { return r.ID == requestID })
		if i >= 0 {
			req = l.pending[i]
		}
	}
	if req == nil {
		s.mu.Unlock()
		return newError(CodeNotPending, fmt.Sprintf("request '%s' is not pending", requestID))
	}
	if err := validateAnswer(req.Kind, &answer); err != nil {
		s.mu.Unlock()
		return err
	}
	l.takePending(requestID)
	s.changed()
	rt := l.runtime
	s.mu.Unlock()

	err := harness.ErrNotPending
	if rt != nil {
		err = rt.Answer(requestID, answer)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case err == nil:
		return s.resolve(l, req, model.OutcomeAnswered)
	case errors.Is(err, harness.ErrNotPending):
		// the harness settled it first, for example on an interrupt.
		s.resolveOrLog(l, req, model.OutcomeAborted)
		return newError(CodeNotPending, fmt.Sprintf("request '%s' was settled before the answer arrived", requestID))
	default:
		s.resolveOrLog(l, req, model.OutcomeAborted)
		return fmt.Errorf("delivering answer to '%s': %w", conversationID, err)
	}
}

func validateAnswer(kind string, answer *harness.Answer) error {
	switch kind {
	case model.RequestPermission:
		switch answer.Decision {
		case harness.DecisionAllow:
			answer.Message, answer.Answers = "", nil
		case harness.DecisionDeny:
			answer.Answers = nil
			if strings.TrimSpace(answer.Message) == "" {
				answer.Message = denyMessage
			}
		default:
			return newError(CodeInvalid, "a permission answer needs a decision of 'allow' or 'deny'")
		}
	case model.RequestQuestion:
		if len(answer.Answers) == 0 {
			return newError(CodeInvalid, "a question answer needs at least one answer")
		}
		answer.Decision, answer.Message = "", ""
	}
	return nil
}

// Interrupt stops the conversation's active turn.
func (s *Service) Interrupt(conversationID string) error {
	s.mu.Lock()
	if _, err := s.conversationLocked(conversationID); err != nil {
		s.mu.Unlock()
		return err
	}
	l := s.live[conversationID]
	if l == nil || l.runtime == nil {
		s.mu.Unlock()
		return newError(CodeConflict, "the conversation has no runtime")
	}
	rt := l.runtime
	s.mu.Unlock()
	return rt.Interrupt()
}

// Release stops the conversation's runtime for a terminal handoff: interrupt
// if work or a request is outstanding, then close input. the conversation
// reports stopping until the runtime is seen to exit.
func (s *Service) Release(conversationID string) error {
	s.mu.Lock()
	if _, err := s.conversationLocked(conversationID); err != nil {
		s.mu.Unlock()
		return err
	}
	defer s.mu.Unlock()
	l := s.live[conversationID]
	if l == nil || l.runtime == nil {
		return newError(CodeConflict, "the conversation has no runtime to release")
	}
	s.beginStopLocked(l, &model.Event{Status: model.StatusStopping})
	return nil
}

// releaseRuntime interrupts before closing when anything is outstanding:
// closing input with a request pending keeps claude alive for minutes.
func releaseRuntime(rt harness.Runtime, active bool) {
	if active {
		if err := rt.Interrupt(); err != nil {
			dl.Warnf("interrupting before release: %v", err)
		}
	}
	if err := rt.Close(); err != nil {
		dl.Warnf("closing runtime input: %v", err)
	}
}

// Resume brings a stopped or unsupervised conversation's cache up to date with
// its native history and clears unsupervised. it starts no runtime; the next
// prompt does.
func (s *Service) Resume(conversationID string) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return newError(CodeUnavailable, "the service is shutting down")
	}
	conv, err := s.conversationLocked(conversationID)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	l, err := s.liveLocked(conversationID)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	if l.runtime != nil || l.busy {
		s.mu.Unlock()
		return newError(CodeConflict, "the conversation has a runtime or is starting; release it before resuming")
	}
	if err := s.transition(l, &model.Event{Status: model.StatusResuming}, StateResuming, nil); err != nil {
		s.mu.Unlock()
		return err
	}
	l.busy = true
	s.mu.Unlock()

	var added int
	var importErr error
	if conv.NativeID != "" {
		added, importErr = s.importHistory(conv, l)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	l.busy = false
	if importErr != nil {
		if err := s.transition(l, &model.Event{Status: model.StatusError, Text: fmt.Sprintf("native history could not be read: %v", importErr)}, "", nil); err != nil {
			dl.Errorf("%v", err)
		}
		return importErr
	}
	if err := s.transition(l, &model.Event{Status: model.StatusResumed, Text: fmt.Sprintf("%d messages added from native history", added)}, "", nil); err != nil {
		return err
	}
	if conv.Runtime == model.RuntimeUnsupervised {
		return s.updateConversation(conversationID, func(c *model.Conversation) { c.Runtime = model.RuntimeNone })
	}
	return nil
}

// importHistory appends the native messages the cache lacks, matched by native
// identity, as native-origin events, and reports how many it added. it never
// rewrites what the log holds.
func (s *Service) importHistory(conv model.Conversation, l *live) (int, error) {
	s.mu.Lock()
	h, ok := s.harnesses[conv.Harness]
	project, projectErr := s.projectLocked(conv.ProjectID)
	s.mu.Unlock()
	if !ok {
		return 0, newError(CodeInvalid, fmt.Sprintf("harness '%s' is not available", conv.Harness))
	}
	if projectErr != nil {
		return 0, projectErr
	}
	ctx, cancel := context.WithTimeout(context.Background(), historyTimeout)
	defer cancel()
	evs, err := h.History(ctx, project.Checkout, conv.NativeID)
	if err != nil {
		return 0, fmt.Errorf("importing history of '%s': %w", conv.ID, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	added := 0
	for _, ev := range evs {
		if ev.NativeID == "" || l.seen[ev.NativeID] {
			continue
		}
		ev.Origin = model.OriginNative
		if err := s.record(l, ev); err != nil {
			return added, err
		}
		added++
	}
	return added, nil
}

// Events returns the conversation's recorded events after seq.
func (s *Service) Events(conversationID string, since int64) ([]*model.Event, error) {
	s.mu.Lock()
	if _, err := s.conversationLocked(conversationID); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	l, err := s.liveLocked(conversationID)
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return l.log.Since(since)
}

// UpdateConversation changes a conversation's name and label.
func (s *Service) UpdateConversation(conversationID, name, label string) (ConversationView, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ConversationView{}, newError(CodeInvalid, "name is required")
	}
	s.mu.Lock()
	err := s.updateConversation(conversationID, func(c *model.Conversation) {
		c.Name, c.Label = name, strings.TrimSpace(label)
	})
	s.mu.Unlock()
	if err != nil {
		return ConversationView{}, err
	}
	return s.view(conversationID), nil
}

// view returns one conversation's current view.
func (s *Service) view(conversationID string) ConversationView {
	s.mu.Lock()
	defer s.mu.Unlock()
	conv, err := s.conversationLocked(conversationID)
	if err != nil {
		return ConversationView{}
	}
	return s.viewLocked(conv)
}

// projectLocked finds a project. callers hold s.mu.
func (s *Service) projectLocked(id string) (model.Project, error) {
	for _, project := range s.reg.Snapshot().Projects {
		if project.ID == id {
			return project, nil
		}
	}
	return model.Project{}, newError(CodeNotFound, fmt.Sprintf("no project '%s'", id))
}

// defaultName is the first line of the opening prompt, shortened.
func defaultName(prompt string) string {
	line, _, _ := strings.Cut(prompt, "\n")
	line = strings.TrimSpace(line)
	if utf8.RuneCountInString(line) <= maxNameRunes {
		return line
	}
	runes := []rune(line)
	return strings.TrimSpace(string(runes[:maxNameRunes])) + "…"
}

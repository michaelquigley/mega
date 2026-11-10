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

	"git.hq.quigley.com/research/roundhouse/internal/events"
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
func (s *Service) StartConversation(projectID, harnessName, name, prompt string, choice LaunchChoice) (ConversationView, error) {
	permissionMode := choice.PermissionMode
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return ConversationView{}, newError(CodeInvalid, "an opening prompt is required")
	}
	if err := checkPermissionMode(harnessName, permissionMode); err != nil {
		return ConversationView{}, err
	}
	if err := s.checkModelChoice(harnessName, choice.Model, choice.Effort); err != nil {
		return ConversationView{}, err
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

		PermissionMode: permissionMode,
		Model:          choice.Model,
		Effort:         choice.Effort,
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
	s.mu.Unlock()
	err = s.startRuntime(conv.ID, l)
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
	// since the last one stopped, then resume by exact id. with no session
	// known, a fresh one starts only on the log's proof that none exists; this
	// is checked before anything is recorded, so a refusal never counts as proof.
	if l.sessionOf(conv) == "" {
		fresh, err := s.freshAllowedLocked(l)
		if err != nil {
			s.mu.Unlock()
			return err
		}
		if !fresh {
			s.mu.Unlock()
			return errSessionUnknown
		}
	}
	if err := s.beginStartLocked(l); err != nil {
		s.mu.Unlock()
		return err
	}
	conv.NativeID = l.sessionOf(conv)
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
	if err := s.startRuntime(conversationID, l); err != nil {
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

// startRuntime launches a runtime for the conversation, after
// beginStartLocked, and clears busy when it is done. the conversation records
// running before the process starts, so a crash from here on leaves it
// unsupervised rather than silently stopped. it shows starting until the
// runtime reports the session it holds. the launch itself runs without s.mu,
// because a harness may report to its sink before Launch returns; busy keeps
// other starts, resumes and prompts away meanwhile. callers do not hold s.mu.
func (s *Service) startRuntime(conversationID string, l *live) error {
	s.mu.Lock()
	h, spec, inc, err := s.prepareLaunchLocked(conversationID, l)
	if err != nil {
		l.busy = false
		s.failedLaunch(l, err, true)
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()

	rt, err := h.Launch(spec, &sink{s: s, l: l, inc: inc})

	s.mu.Lock()
	defer s.mu.Unlock()
	l.busy = false
	if err != nil {
		if l.inc == inc {
			l.inc = nil
		}
		// like an unrecorded stop, an unrecorded failure leaves the conversation
		// recorded running, so a restart settles it rather than this run
		// claiming what its log could not confirm.
		if s.failedLaunch(l, err, false) {
			if updateErr := s.updateConversation(conversationID, func(c *model.Conversation) { c.Runtime = model.RuntimeNone }); updateErr != nil {
				dl.Errorf("recording that '%s' did not start: %v", conversationID, updateErr)
			}
		}
		return fmt.Errorf("starting runtime for '%s': %w", conversationID, err)
	}
	// a new session belongs to this conversation from the moment it exists:
	// hold it until the registry does, so no later launch starts another.
	bindErr := error(nil)
	if spec.NativeID == "" && rt.NativeID() != "" {
		l.session = rt.NativeID()
		bindErr = s.updateConversation(conversationID, func(c *model.Conversation) { c.NativeID = rt.NativeID() })
		if bindErr == nil {
			l.session = ""
		} else {
			dl.Errorf("recording the session of '%s'; holding it for the next launch: %v", conversationID, bindErr)
		}
	}
	if l.inc != inc {
		// it exited while starting, and its exit has been settled already.
		return fmt.Errorf("the runtime for '%s' exited as it started", conversationID)
	}
	l.runtime = rt
	if bindErr != nil {
		// the conversation cannot record its session yet; stop the runtime
		// rather than run one the registry does not know.
		s.beginStopLocked(l, &model.Event{Status: model.StatusError, Text: fmt.Sprintf("the session could not be recorded; stopping it: %v", bindErr)})
		return bindErr
	}
	if s.closed {
		s.beginStopLocked(l, &model.Event{Status: model.StatusStopping})
		return newError(CodeUnavailable, "the service is shutting down")
	}
	return nil
}

// prepareLaunchLocked checks a conversation can launch, records it running and
// opens its incarnation. callers hold s.mu.
func (s *Service) prepareLaunchLocked(conversationID string, l *live) (harness.Harness, harness.Spec, *incarnation, error) {
	conv, err := s.conversationLocked(conversationID)
	if err != nil {
		return nil, harness.Spec{}, nil, err
	}
	project, err := s.projectLocked(conv.ProjectID)
	if err != nil {
		return nil, harness.Spec{}, nil, err
	}
	h, ok := s.harnesses[conv.Harness]
	if !ok {
		return nil, harness.Spec{}, nil, newError(CodeInvalid, fmt.Sprintf("harness '%s' is not available", conv.Harness))
	}
	// a session this conversation created but the registry does not hold yet is
	// bound in the same write that records the runtime running, and resumed. if
	// that write fails, the launch is refused, so no fresh session is started
	// in its place.
	session := l.sessionOf(conv)
	if err := s.updateConversation(conversationID, func(c *model.Conversation) {
		c.Runtime = model.RuntimeRunning
		c.NativeID = session
	}); err != nil {
		return nil, harness.Spec{}, nil, err
	}
	l.session = ""
	inc := &incarnation{exited: make(chan struct{})}
	l.inc = inc
	// the recorded choice goes with every launch, so a release and a resume
	// keep it; a terminal visit never changes it.
	return h, harness.Spec{Cwd: project.Checkout, NativeID: session, PermissionMode: conv.PermissionMode, Model: conv.Model, Effort: conv.Effort}, inc, nil
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
	l.nextSent = true
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
	if err := validateAnswer(req, &answer); err != nil {
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

func validateAnswer(req *model.Request, answer *harness.Answer) error {
	switch req.Kind {
	case model.RequestPermission:
		switch answer.Decision {
		case harness.DecisionAllow:
			answer.Message, answer.Answers = "", nil
		case harness.DecisionAllowSession:
			// only where the harness itself offered it: roundhouse never extends
			// an approval on its own account.
			if payload, _ := req.Payload.(map[string]any); payload["allow_session"] != true {
				return newError(CodeInvalid, "this request offers no approval for the session; answer 'allow' or 'deny'")
			}
			answer.Message, answer.Answers = "", nil
		case harness.DecisionDeny:
			answer.Answers = nil
			if strings.TrimSpace(answer.Message) == "" {
				answer.Message = denyMessage
			}
		default:
			return newError(CodeInvalid, "a permission answer needs a decision of 'allow', 'deny', or 'allow_session' where offered")
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

	var result imported
	var importErr error
	s.mu.Lock()
	conv.NativeID = l.sessionOf(conv)
	s.mu.Unlock()
	if conv.NativeID != "" {
		result, importErr = s.importHistory(conv, l)
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
	// after an unclean restart the runtime may have kept working where nobody
	// could see it, and the turn it was in never ended where the service could
	// see it either. the marker moves to the newest finished agent output in the
	// whole log, cached or imported; the reading position decides whether it is
	// unread. after a release the operator was the one in the terminal, so the
	// import changes nothing.
	if conv.Runtime == model.RuntimeUnsupervised {
		seq, at, err := s.newestFinishedOutputLocked(l, result.open)
		if err == nil && seq > 0 {
			err = s.markUnreadLocked(l, seq, at)
		}
		if err != nil {
			dl.Errorf("marking the output found on resuming '%s' unread: %v", conversationID, err)
		}
	}
	if err := s.transition(l, &model.Event{Status: model.StatusResumed, Text: fmt.Sprintf("%d messages added from native history", result.added)}, "", nil); err != nil {
		return err
	}
	if conv.Runtime == model.RuntimeUnsupervised {
		return s.updateConversation(conversationID, func(c *model.Conversation) { c.Runtime = model.RuntimeNone })
	}
	return nil
}

// imported reports what an import added, and which native events belong to a
// turn still in progress.
type imported struct {
	added int
	open  map[string]bool
}

// importHistory appends the native messages the cache lacks, matched by native
// identity, as native-origin events, and reports what it added. it never
// rewrites what the log holds, and never moves the unread marker itself.
func (s *Service) importHistory(conv model.Conversation, l *live) (imported, error) {
	s.mu.Lock()
	h, ok := s.harnesses[conv.Harness]
	project, projectErr := s.projectLocked(conv.ProjectID)
	s.mu.Unlock()
	if !ok {
		return imported{}, newError(CodeInvalid, fmt.Sprintf("harness '%s' is not available", conv.Harness))
	}
	if projectErr != nil {
		return imported{}, projectErr
	}
	ctx, cancel := context.WithTimeout(context.Background(), historyTimeout)
	defer cancel()
	history, err := h.History(ctx, project.Checkout, conv.NativeID)
	if err != nil {
		return imported{}, fmt.Errorf("importing history of '%s': %w", conv.ID, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	result := imported{open: history.Open}
	for _, ev := range history.Events {
		if ev.NativeID == "" || l.seen[ev.NativeID] {
			continue
		}
		ev.Origin = model.OriginNative
		if err := s.record(l, ev); err != nil {
			return result, err
		}
		result.added++
	}
	return result, nil
}

// newestFinishedOutputLocked finds the newest agent output (assistant, tool use
// or tool result; never a native event) anywhere in the log, cached or
// imported, leaving out events of a turn still in progress. callers hold s.mu.
func (s *Service) newestFinishedOutputLocked(l *live, open map[string]bool) (int64, time.Time, error) {
	evs, err := l.log.Since(0)
	if err != nil {
		return 0, time.Time{}, err
	}
	for i := len(evs) - 1; i >= 0; i-- {
		ev := evs[i]
		switch ev.Kind {
		case model.EventAssistant, model.EventToolUse, model.EventToolResult:
			if ev.NativeID != "" && open[ev.NativeID] {
				continue
			}
			return ev.Seq, ev.At, nil
		}
	}
	return 0, time.Time{}, nil
}

// markUnreadLocked advances the unread marker to seq, as a confirmed registry
// write that memory follows. callers hold s.mu.
func (s *Service) markUnreadLocked(l *live, seq int64, at time.Time) error {
	err := s.updateConversation(l.id, func(c *model.Conversation) {
		if seq > c.OutputSeq {
			c.OutputSeq = seq
		}
	})
	if err != nil {
		return err
	}
	if conv, err := s.conversationLocked(l.id); err == nil && conv.OutputSeq == seq {
		l.outputAt = at
	}
	return nil
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

// RemoveConversation takes one conversation out of the workspace and deletes
// its event log, the conversation's own history and reading state. it never
// touches the checkout or the native session, which discovery lists again
// afterwards. it refuses while the conversation has a runtime in any state
// (see busyReasonLocked).
//
// the registry write comes first and memory follows it. the log directory is
// deleted only once the registry no longer names the conversation, so a crash
// between the two leaves an orphan directory, never a registered conversation
// without its log (see dropLogsLocked).
func (s *Service) RemoveConversation(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	conv, err := s.conversationLocked(id)
	if err != nil {
		return err
	}
	if reason := s.busyReasonLocked(conv); reason != "" {
		return newError(CodeConflict, "the conversation "+reason)
	}
	err = s.update(func(doc *registry.Document) error {
		i := slices.IndexFunc(doc.Conversations, func(c model.Conversation) bool { return c.ID == id })
		if i < 0 {
			return newError(CodeNotFound, fmt.Sprintf("no conversation '%s'", id))
		}
		doc.Conversations = slices.Delete(doc.Conversations, i, i+1)
		return nil
	})
	if err != nil {
		return err
	}
	s.dropLogsLocked(id)
	return nil
}

// busyReasonLocked says why a conversation cannot be removed, or returns "" if
// it can. a conversation with a runtime in any state is refused: starting,
// running, stopping, resuming, or unsupervised, since an orphan from before a
// restart may still be working. a stop the log could not record leaves the
// conversation recorded running, and that is refused too until a restart
// settles it. the reason reads after the conversation's name. callers hold s.mu.
func (s *Service) busyReasonLocked(conv model.Conversation) string {
	l := s.live[conv.ID]
	switch {
	case conv.Runtime == model.RuntimeUnsupervised:
		return "lost supervision when the service restarted and its execution is uncertain; resume it before removing it"
	case l != nil && l.busy:
		return "is starting a runtime or resuming; wait for it to settle before removing it"
	case conv.Runtime == model.RuntimeRunning || (l != nil && (l.runtime != nil || l.releasing)):
		return "has a runtime; release it before removing it"
	}
	return ""
}

// dropLogsLocked lets go of removed conversations once the registry write that
// removed them is confirmed: their open logs are closed, their live state is
// dropped, and their log directories are deleted. a directory that cannot be
// deleted is logged and left behind, the same orphan a crash between the write
// and the delete leaves; the removal itself stands. callers hold s.mu.
func (s *Service) dropLogsLocked(ids ...string) {
	for _, id := range ids {
		if l := s.live[id]; l != nil {
			l.log.Close()
			delete(s.live, id)
		}
		if err := events.Remove(s.logPath(id)); err != nil {
			dl.Errorf("removed conversation '%s' from the workspace; its log directory is left behind: %v", id, err)
		}
	}
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

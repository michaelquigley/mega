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

	"github.com/michaelquigley/mega/internal/events"
	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/registry"
)

const (
	historyTimeout = 2 * time.Minute
	maxNameRunes   = 48
	denyMessage    = "the operator denied this in mega"
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
	if err := s.refuseNewWorkLocked(); err != nil {
		s.mu.Unlock()
		return ConversationView{}, err
	}
	if _, ok := s.harnesses[harnessName]; !ok {
		s.mu.Unlock()
		return ConversationView{}, newError(CodeInvalid, fmt.Sprintf("harness '%s' is not available", harnessName))
	}
	project, err := s.projectLocked(projectID)
	if err != nil {
		s.mu.Unlock()
		return ConversationView{}, err
	}
	if err := s.checkoutLocked(project); err != nil {
		s.mu.Unlock()
		return ConversationView{}, err
	}
	if err := s.envReadyLocked(project); err != nil {
		s.mu.Unlock()
		return ConversationView{}, err
	}
	if err := s.harnessReadyLocked(projectID, harnessName); err != nil {
		s.mu.Unlock()
		return ConversationView{}, err
	}
	s.mu.Unlock()
	// the project's environment is resolved before the conversation exists,
	// so an activation that fails leaves nothing behind but the refusal.
	place, facts, err := s.resolveProject(projectID)
	if err != nil {
		return ConversationView{}, err
	}
	if err := missingCoded(s.harnesses[harnessName].Check(place.Find)); err != nil {
		return ConversationView{}, err
	}
	// what the operator left unchosen is copied once from the harness's
	// settings, and passed from then on; later changes to the settings do not
	// reach this conversation.
	copyCtx, cancelCopy := context.WithTimeout(context.Background(), modelsTimeout)
	asked, needEffort, err := s.copyFor(copyCtx, harnessName, place, model.TurnSettings{Model: choice.Model, Effort: choice.Effort, Mode: permissionMode})
	cancelCopy()
	if err != nil {
		return ConversationView{}, err
	}
	if needEffort {
		return ConversationView{}, errChooseEffort
	}
	// the copy has fetched the model list, against which the request is
	// checked again.
	if err := s.checkModelChoice(harnessName, asked.Model, asked.Effort); err != nil {
		return ConversationView{}, err
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

		PermissionMode: asked.Mode,
		Model:          asked.Model,
		Effort:         asked.Effort,
	}
	err = s.update(func(doc *registry.Document) error {
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
	err = s.startRuntime(conv.ID, l, false, place, facts)
	if err != nil {
		return s.view(conv.ID), s.explain(harnessName, place.Find, err)
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
	if err := s.refuseNewWorkLocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	if err := s.conversationCheckoutLocked(conv); err != nil {
		s.mu.Unlock()
		return err
	}
	if err := s.conversationEnvReadyLocked(conv); err != nil {
		s.mu.Unlock()
		return err
	}
	if err := s.harnessReadyLocked(conv.ProjectID, conv.Harness); err != nil {
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
	// Without a runtime, resume by exact id. With no known session, a
	// fresh one starts only on the log’s proof that none exists. This
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
	s.mu.Unlock()

	place, facts, err := s.resolveForStart(conv, l)
	if err != nil {
		return err
	}
	if err := s.startRuntime(conversationID, l, false, place, facts); err != nil {
		return s.explain(conv.Harness, place.Find, err)
	}
	return s.send(conversationID, text)
}

// Start starts a runtime for a stopped conversation and sends nothing: the
// operator's gesture to have it running, its MCP servers connected and its
// settings taking changes at once. like a prompt to a stopped conversation, it
// resumes the exact session. a conversation with no session is refused, since
// only its opening prompt may start one; so is an unsupervised one, which
// Resume settles first.
func (s *Service) Start(conversationID string) error {
	s.mu.Lock()
	conv, l, err := s.idleCheckLocked(conversationID)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	if err := s.refuseNewWorkLocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	if err := s.conversationCheckoutLocked(conv); err != nil {
		s.mu.Unlock()
		return err
	}
	if err := s.conversationEnvReadyLocked(conv); err != nil {
		s.mu.Unlock()
		return err
	}
	if err := s.harnessReadyLocked(conv.ProjectID, conv.Harness); err != nil {
		s.mu.Unlock()
		return err
	}
	if l.runtime != nil {
		s.mu.Unlock()
		return newError(CodeConflict, "the conversation already has a runtime")
	}
	conv.NativeID = l.sessionOf(conv)
	if conv.NativeID == "" {
		// a launch whose session went unrecorded is refused as a prompt would
		// refuse it; otherwise there is no session yet.
		fresh, err := s.freshAllowedLocked(l)
		s.mu.Unlock()
		switch {
		case err != nil:
			return err
		case !fresh:
			return errSessionUnknown
		}
		return newError(CodeConflict, "the conversation has no session yet; its opening prompt starts one")
	}
	if err := s.beginStartLocked(l); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()

	place, facts, err := s.resolveForStart(conv, l)
	if err != nil {
		return err
	}
	return s.explain(conv.Harness, place.Find, s.startRuntime(conversationID, l, true, place, facts))
}

// conversationEnvReadyLocked refuses work in a conversation whose project's
// environment may not run. callers hold s.mu.
func (s *Service) conversationEnvReadyLocked(conv model.Conversation) error {
	project, err := s.projectLocked(conv.ProjectID)
	if err != nil {
		return err
	}
	return s.envReadyLocked(project)
}

// resolveForStart resolves the project's environment for a runtime start,
// after beginStartLocked, and checks the harness can run in it. a failure ends
// the start, recorded as not started (no process was), with what may be
// recorded of it and never the activation's output. callers do not hold s.mu.
func (s *Service) resolveForStart(conv model.Conversation, l *live) (harness.Place, *model.EnvironmentFacts, error) {
	place, facts, err := s.resolveProject(conv.ProjectID)
	if err == nil {
		err = missingCoded(s.harnesses[conv.Harness].Check(place.Find))
	}
	if err != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		l.busy = false
		s.failedLaunch(l, err, true)
		return harness.Place{}, nil, err
	}
	return place, facts, nil
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
		return conv, nil, newError(CodeConflict, "the runtime is starting")
	case l.releasing:
		return conv, nil, newError(CodeConflict, "the runtime is stopping")
	}
	return conv, l, nil
}

// beginStartLocked shows a start under way and holds off other starts until it ends. callers hold s.mu.
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
// other starts, resumes and prompts away meanwhile. idle is a start with no
// prompt following. callers do not hold s.mu.
func (s *Service) startRuntime(conversationID string, l *live, idle bool, place harness.Place, facts *model.EnvironmentFacts) error {
	// a conversation from before every one held a concrete request is filled
	// once, before anything is passed.
	if err := s.fillAtLaunch(conversationID, place); err != nil {
		s.mu.Lock()
		l.busy = false
		s.failedLaunch(l, err, true)
		s.mu.Unlock()
		return err
	}
	s.mu.Lock()
	conv, err := s.conversationLocked(conversationID)
	if err != nil {
		l.busy = false
		s.mu.Unlock()
		return err
	}
	h, spec, inc, err := s.prepareLaunchLocked(conv, l, facts)
	spec.Idle = idle
	// the project's resolved environment, in full, and nothing of mega's own;
	// what it was is recorded with the runtime's start, as names and
	// fingerprints.
	spec.Env, spec.Find = place.Env, place.Find
	if inc != nil {
		inc.env = facts
	}
	if err != nil {
		l.busy = false
		s.failedLaunch(l, err, true)
		s.mu.Unlock()
		return err
	}
	// a shutdown waits for a launch that has begun, until it has returned.
	defer close(inc.launched)
	s.mu.Unlock()

	// the version that will run, read just before the launch, bounded by
	// harness.VersionTimeout; one that cannot be read leaves the launch to
	// proceed and its started line to say unknown.
	if v, ok := h.(harness.Versioner); ok && inc != nil {
		version, verr := v.Version(context.Background(), place.Find)
		if verr != nil {
			dl.Debugf("reading the version of '%s' before launching '%s': %v", h.Name(), conversationID, verr)
		}
		inc.version = version
		if _, note := harness.Stand(h.Name(), version); note != "" {
			dl.Infof("launching '%s': %s", conversationID, note)
		}
		s.versionSeen(h.Name(), version)
	}
	// the last check, immediately before the harness starts: nothing reading
	// the version may have begun a shutdown or changed the environment file.
	s.mu.Lock()
	if err := s.harnessGateLocked(conv.ProjectID, facts); err != nil {
		l.busy = false
		if l.inc == inc {
			l.inc = nil
		}
		if s.failedLaunch(l, err, true) {
			if updateErr := s.updateConversation(conversationID, func(c *model.Conversation) { c.Runtime = model.RuntimeNone }); updateErr != nil {
				dl.Errorf("recording that '%s' did not start: %v", conversationID, updateErr)
			}
		}
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	k := &sink{s: s, l: l, inc: inc}
	rt, err := h.Launch(spec, k)
	var nativeID string
	if err == nil && spec.NativeID == "" {
		nativeID = rt.NativeID()
	}
	// the harness's own failure: what is recorded and shown is its summary.
	err = harnessFault(h.Name(), err)

	s.mu.Lock()
	defer s.mu.Unlock()
	l.busy = false
	refused := l.refused
	l.refused = nil
	held, earlyExit := inc.early, inc.earlyExit
	inc.early, inc.earlyExit = nil, nil
	// a new session belongs to this conversation from the moment it exists:
	// hold it until the registry does, so no later launch starts another.
	bindErr := error(nil)
	if err == nil && held != nil {
		refused = k.confirmFirstStart(nativeID, held)
		if refused == nil && nativeID != "" {
			bindErr = s.updateConversation(conversationID, func(c *model.Conversation) { c.NativeID = nativeID })
			if bindErr == nil {
				l.session, l.sessionRecorded = "", false
			} else {
				dl.Errorf("recording the session of '%s'; holding it for the next launch: %v", conversationID, bindErr)
			}
		}
	}
	if earlyExit != nil {
		// record the refusal separately from the exit the adapter observed.
		if refused != nil && k.current() {
			if err := s.transition(l, refused, l.state, nil); err != nil {
				dl.Errorf("recording the first launch refusal for '%s': %v", conversationID, err)
			}
		}
		earlyExit()
	}
	if err != nil {
		if l.inc == inc {
			l.inc = nil
			// a request raised while launching died with the launch.
			s.retireRequestsLocked(l)
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
	if l.inc != inc {
		if refused != nil {
			return newError(CodeConflict, refused.Text)
		}
		// it exited while starting, and its exit has been settled already.
		return fmt.Errorf("the runtime for '%s' exited as it started", conversationID)
	}
	l.runtime = rt
	if refused != nil {
		// it reported another session than the one it was launched for while
		// launching: recorded and stopped now, and no prompt follows.
		s.beginStopLocked(l, refused)
		return newError(CodeConflict, refused.Text)
	}
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
	// a turn's comparison resolves model aliases through the list.
	if conv, err := s.conversationLocked(conversationID); err == nil {
		go s.prefetchCatalog(conv.Harness, place)
	}
	return nil
}

// prepareLaunchLocked checks a conversation can launch, records it running and
// opens its incarnation. callers hold s.mu.
func (s *Service) prepareLaunchLocked(conv model.Conversation, l *live, env *model.EnvironmentFacts) (harness.Harness, harness.Spec, *incarnation, error) {
	project, err := s.projectLocked(conv.ProjectID)
	if err != nil {
		return nil, harness.Spec{}, nil, err
	}
	// the environment was resolved since the file was checked: the launch is committed only while the project's
	// environment may still run and is still the activation that was resolved.
	// the same check runs again immediately before the harness starts.
	if err := s.harnessGateLocked(conv.ProjectID, env); err != nil {
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
	if err := s.updateConversation(conv.ID, func(c *model.Conversation) {
		c.Runtime = model.RuntimeRunning
		c.NativeID = session
	}); err != nil {
		return nil, harness.Spec{}, nil, err
	}
	l.session, l.sessionRecorded = "", false
	inc := &incarnation{exited: make(chan struct{}), launched: make(chan struct{})}
	if session == "" {
		inc.early = []launchCallback{}
	}
	l.inc = inc
	// the recorded request goes with every launch, so a release and a resume
	// keep it; a terminal visit never changes it.
	return h, harness.Spec{Cwd: project.Checkout, NativeID: session, PermissionMode: conv.PermissionMode, Model: conv.Model, Effort: conv.Effort}, inc, nil
}

// harnessGateLocked rechecks the approved activation just before a harness
// launch or transcript read. Callers hold s.mu.
func (s *Service) harnessGateLocked(projectID string, env *model.EnvironmentFacts) error {
	if s.closed {
		return newError(CodeUnavailable, "the service is shutting down")
	}
	project, err := s.projectLocked(projectID)
	if err != nil {
		return err
	}
	return s.envUnchangedLocked(project, env)
}

// send records the prompt with a service-minted message identity and hands it
// to the runtime, which can pass it to the harness to identify live echoes.
func (s *Service) send(conversationID, text string) error {
	s.mu.Lock()
	l := s.live[conversationID]
	if l == nil || l.runtime == nil {
		s.mu.Unlock()
		return newError(CodeConflict, "the conversation has no runtime")
	}
	if l.inc.wrong || l.releasing {
		s.mu.Unlock()
		return newError(CodeConflict, "the runtime is stopping")
	}
	uuid := registry.NewUUID()
	ev := &model.Event{Kind: model.EventUser, Origin: model.OriginManaged, Text: text, NativeID: uuid}
	if err := s.record(l, ev); err != nil {
		s.mu.Unlock()
		return err
	}
	s.beginTurnLocked(l, ev.At)
	l.instructedAt = ev.At
	s.changed()
	rt := l.runtime
	s.mu.Unlock()
	if err := rt.Prompt(uuid, text); err != nil {
		if errors.Is(err, harness.ErrPromptRefused) {
			// refused, no turn started: the turn opened for this prompt ends
			// here, saying why, as a turn's end would, so the next prompt is
			// taken. a prompt whose fate is unknown keeps its turn.
			s.mu.Lock()
			if s.live[conversationID] == l && l.turnActive && l.turnStartedAt.Equal(ev.At) {
				end := &model.Event{Kind: model.EventTurnEnd, Status: model.TurnError, Origin: model.OriginManaged, Text: fmt.Sprintf("the harness refused the prompt: %v", err)}
				end.Turn = s.turnRecordLocked(l)
				if recordErr := s.record(l, end); recordErr != nil {
					dl.Errorf("%v", recordErr)
				} else {
					s.turnEndedLocked(l)
				}
			}
			s.mu.Unlock()
		}
		return fmt.Errorf("sending prompt to '%s': %w", conversationID, err)
	}
	return nil
}

// Answer delivers the operator's answer to a pending request. the answer
// reserves the request while it is delivered: the request stays in the
// pending set every reader sees, and a second answer meanwhile, from this
// browser or another, is refused as conflict. it leaves the set only once its
// resolution is recorded; a later answer gets not_pending. once an interrupt
// or a release has begun, an answer that would let work go on is refused as
// conflict, and deny and dismiss are not.
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
	if l.reserved[requestID] == answerInFlight {
		s.mu.Unlock()
		return newError(CodeConflict, fmt.Sprintf("request '%s' is already being answered", requestID))
	}
	if _, settled := l.unrecorded(requestID); settled {
		// answered, or settled by the harness, and still listed only because
		// that could not be recorded: the harness holds it no more.
		s.mu.Unlock()
		return newError(CodeNotPending, fmt.Sprintf("request '%s' was settled, but that could not be recorded, so it is shown as last recorded and takes no further answer", requestID))
	}
	if err := validateAnswer(req, &answer); err != nil {
		s.mu.Unlock()
		return err
	}
	// once an interrupt or a release has begun, an answer that would let work
	// go on is refused: the request is still pending, and deny or dismiss,
	// which only settle it toward the stop, are not.
	if (l.interrupting || l.releasing) && harness.CarriesWork(answer) {
		s.mu.Unlock()
		return newError(CodeConflict, "the turn is stopping, so this answer would let work go on after it; Deny or Dismiss can still be pressed")
	}
	// the request stays listed until its resolution is recorded, reserved
	// against a second answer meanwhile.
	l.reserved[requestID] = answerInFlight
	rt := l.runtime
	s.mu.Unlock()

	err := harness.ErrNotPending
	if rt != nil {
		err = rt.Answer(requestID, answer)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// the request is settled once: as the harness settled it while the answer
	// was on its way, if it did and that could not be recorded, else by the
	// answer's result.
	listed := slices.ContainsFunc(l.pending, func(r *model.Request) bool { return r.ID == requestID })
	outcome, settled := l.unrecorded(requestID)
	if !settled {
		delete(l.reserved, requestID)
		if errors.Is(err, harness.ErrInvalidAnswer) {
			// refused without settling: the request is still pending, and a
			// fitting answer or a dismissal still reaches it.
			return newError(CodeInvalid, err.Error())
		}
		switch {
		case err == nil:
			outcome = model.OutcomeAnswered
		case errors.Is(err, harness.ErrUnconfirmed):
			outcome = model.OutcomeUnconfirmed
		default:
			outcome = model.OutcomeAborted
		}
	}
	var recordErr error
	if listed {
		recordErr = s.settleLocked(l, req, outcome)
	}
	switch {
	case err == nil:
		// taken: one whose settlement cannot be recorded says so.
		return recordErr
	case recordErr != nil:
		dl.Errorf("%v", recordErr)
	}
	switch {
	case errors.Is(err, harness.ErrUnconfirmed):
		// sent, and never confirmed: the operator is told in its words.
		return newError(CodeHarnessFailed, err.Error())
	case listed && !settled && !errors.Is(err, harness.ErrNotPending):
		return fmt.Errorf("delivering answer to '%s': %w", conversationID, err)
	}
	// the harness settled it first, for example on an interrupt, or the
	// runtime exited.
	return newError(CodeNotPending, fmt.Sprintf("request '%s' was settled before the answer arrived", requestID))
}

// answerInFlight reserves a request whose answer is on its way to the
// harness. otherwise a reservation is the outcome of a settlement that
// could not be recorded yet (live.unrecorded).
const answerInFlight = "in flight"

// settleLocked is the one move of a request out of the pending set, for an
// answer and for the harness's own settlement alike: it records the
// resolution and only then takes the request out. one whose resolution
// cannot be recorded stays listed, as the last confirmed state, and nothing
// is broadcast, as after any durability fault; but the harness holds it no
// more, so it is never answerable again: an answer gets not_pending without
// reaching the harness, and its outcome is kept for the runtime's retirement
// to record. an interrupt under way with no turn running ends once the
// harness holds no request: nothing is left for it to stop. callers hold s.mu.
func (s *Service) settleLocked(l *live, req *model.Request, outcome string) error {
	if err := s.resolve(l, req, outcome); err != nil {
		l.reserved[req.ID] = outcome
		return err
	}
	l.takePending(req.ID)
	if !l.turnActive && !l.holdsRequests() {
		l.interrupting = false
	}
	s.changed()
	return nil
}

func validateAnswer(req *model.Request, answer *harness.Answer) error {
	switch req.Kind {
	case model.RequestPermission:
		switch answer.Decision {
		case harness.DecisionAllow:
			answer.Message, answer.Answers = "", nil
		case harness.DecisionAllowSession:
			// only where the harness itself offered it: mega never extends
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
		if answer.Decision == harness.DecisionDismiss {
			// pi's own cancel, where the harness offers it: the operator's
			// gesture, as Escape is in pi's terminal. mega never sends it.
			if payload, _ := req.Payload.(map[string]any); payload["dismiss"] != true {
				return newError(CodeInvalid, "this question cannot be dismissed; answer it")
			}
			answer.Message, answer.Answers = "", nil
			return nil
		}
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
	// an interrupt with a turn, or a request the harness still holds, to stop
	// is under way while either is outstanding: until the turn ends, the
	// harness holds no request with no turn running, or the runtime exits.
	if l.turnActive || l.holdsRequests() {
		l.interrupting = true
		s.changed()
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
	l := s.live[conversationID]
	if l == nil || l.runtime == nil {
		s.mu.Unlock()
		return newError(CodeConflict, "the conversation has no runtime to release")
	}
	rt := l.runtime
	l.handoff = true
	s.beginStopLocked(l, &model.Event{Status: model.StatusStopping})
	s.mu.Unlock()
	// the stop is fenced before Release returns: nothing the runtime is asked
	// to deliver from now on starts work, whatever the asynchronous interrupt
	// and close have reached. outside s.mu, since an adapter may report to
	// the service while it holds its own lock.
	rt.Stopping()
	return nil
}

// releaseRuntime interrupts before closing when anything is outstanding:
// closing input with a request pending keeps claude alive for minutes.
func releaseRuntime(rt harness.Runtime, active bool) {
	// every stop fences first; Release has fenced already, before returning.
	rt.Stopping()
	if active {
		if err := rt.Interrupt(); err != nil {
			dl.Warnf("interrupting before release: %v", err)
		}
	}
	if err := rt.Close(); err != nil {
		dl.Warnf("closing runtime input: %v", err)
	}
}

// Resume acknowledges lost supervision without talking to the harness.
func (s *Service) Resume(conversationID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return newError(CodeUnavailable, "the service is shutting down")
	}
	conv, err := s.conversationLocked(conversationID)
	if err != nil {
		return err
	}
	if conv.Runtime == model.RuntimeUnsupervised {
		return s.updateConversation(conversationID, func(c *model.Conversation) { c.Runtime = model.RuntimeNone })
	}
	if s.busyReasonLocked(conv) != "" {
		return newError(CodeConflict, "the conversation has a runtime or is starting; release it before resuming")
	}
	return nil
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

// HistoryQuery names the part of a conversation's history a read wants.
type HistoryQuery struct {
	// Turns, when positive, reads that many turns. with Before zero they are
	// the newest, plus the turn in flight; otherwise the ones before Before.
	Turns int
	// Before, when positive, reads history before that seq: Turns of it, or
	// all of it when Turns is zero.
	Before int64
	// Since reads every event after it, when Turns and Before are both zero.
	Since int64
	// Native includes native events, which are otherwise left out.
	Native bool
}

// History is a stretch of a conversation's history, From through Through.
// native events are left out unless asked for, but the stretch still covers
// them: a reader holding it has been given everything up to Through.
type History struct {
	Events  []*model.Event
	From    int64
	Through int64
	// Windowed: the read was by turns, so Earlier says what lies before From.
	Windowed bool
	// EarlierTurns is how many turns start before From.
	EarlierTurns int
}

// EarlierEvents is how many events lie before From, native ones included.
func (h History) EarlierEvents() int64 { return max(h.From-1, 0) }

// History reads part of a conversation's history: the newest turns, the turns
// before an event, or everything after one, through the log's index, so a read
// parses about what it returns. a windowed read starts at a turn's first
// event, so a tool call never arrives without its result's turn or a reply
// without its prompt.
func (s *Service) History(conversationID string, q HistoryQuery) (History, error) {
	s.mu.Lock()
	if _, err := s.conversationLocked(conversationID); err != nil {
		s.mu.Unlock()
		return History{}, err
	}
	l, err := s.liveLocked(conversationID)
	s.mu.Unlock()
	if err != nil {
		return History{}, err
	}
	var h History
	switch {
	case q.Turns > 0 || q.Before > 0:
		h.Windowed = true
		h.From, h.EarlierTurns, h.Through = l.log.TurnsBefore(q.Before, q.Turns)
		if q.Before > 0 {
			h.Through = min(q.Before-1, h.Through)
		}
	default:
		h.From, h.Through = q.Since+1, l.log.LastSeq()
	}
	if h.Through < h.From {
		return h, nil
	}
	var keep func(*model.Event) bool
	if !q.Native {
		keep = func(ev *model.Event) bool { return ev.Kind != model.EventNative }
	}
	h.Events, err = l.log.Range(h.From, h.Through, keep)
	return h, err
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
// running, stopping, or unsupervised, since an orphan from before a
// restart may still be working. a stop the log could not record leaves the
// conversation recorded running, and that is refused too until a restart
// settles it. the reason reads after the conversation's name. callers hold s.mu.
func (s *Service) busyReasonLocked(conv model.Conversation) string {
	l := s.live[conv.ID]
	switch {
	case conv.Runtime == model.RuntimeUnsupervised:
		return "lost supervision when the service restarted and its execution is uncertain; resume it before removing it"
	case l != nil && l.busy:
		return "is starting a runtime; wait for it to settle before removing it"
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

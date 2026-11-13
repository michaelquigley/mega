package service

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/michaelquigley/df/dl"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

// checkPermissionMode validates an operator's choice of permission mode for a
// conversation on harnessName. empty is mega's default. only claude has
// modes, and only model.PermissionModes are offered: never a bypass mode.
func checkPermissionMode(harnessName, mode string) error {
	if mode == "" {
		return nil
	}
	if harnessName != model.HarnessClaude {
		return newError(CodeInvalid, fmt.Sprintf("'%s' conversations have no permission mode; the harness's approval policy comes from its own configuration", harnessName))
	}
	if !slices.Contains(model.PermissionModes, mode) {
		offered := make([]string, len(model.PermissionModes))
		for i, m := range model.PermissionModes {
			offered[i] = "'" + m + "'"
		}
		return newError(CodeInvalid, fmt.Sprintf("permission mode '%s' is not offered; choose %s", mode, strings.Join(offered, ", ")))
	}
	return nil
}

// outcomes of a permission mode switch, as the harness answered it.
const (
	ModeConfirmed   = "confirmed"
	ModeRefused     = "refused"
	ModeUnconfirmed = "unconfirmed"
	// ModeStored: there was no runtime to ask, so only the choice was
	// written, for the next launch to pass.
	ModeStored = "stored"
)

// ModeOutcome is the harness's answer to a switch, with what it said.
type ModeOutcome struct {
	Outcome string
	Message string
}

// SetPermissionMode records the mode the operator requests for a claude
// conversation and, if a runtime is running, asks it to switch. the request
// is intent: it is recorded as a mode_requested event and written on the
// conversation first (requestModeLocked), as the mode the next launch
// passes, whether or not a runtime runs. that event is the operator's
// gesture, never a report: the live mode comes only from what the runtime
// reports, recorded by harnessModeChanged in the order claude reported it,
// so there is exactly one writer of it.
//
// with no runtime the outcome is stored. an unsupervised conversation, or one
// starting, stopping or resuming, is refused. with a runtime the outcome is
// the harness's answer. confirmed: the runtime has already reported the new
// mode. refused: the request goes back to what it was before, recorded the
// same way. unconfirmed: the request stands and the live mode is whatever
// claude last reported, which a turn's line shows when it differs. a pending
// request is never touched.
func (s *Service) SetPermissionMode(conversationID, mode string) (ModeOutcome, error) {
	s.mu.Lock()
	conv, err := s.conversationLocked(conversationID)
	if err != nil {
		s.mu.Unlock()
		return ModeOutcome{}, err
	}
	if mode == "" {
		s.mu.Unlock()
		return ModeOutcome{}, newError(CodeInvalid, "a permission mode is required")
	}
	if err := checkPermissionMode(conv.Harness, mode); err != nil {
		s.mu.Unlock()
		return ModeOutcome{}, err
	}
	if conv.Runtime == model.RuntimeUnsupervised {
		s.mu.Unlock()
		return ModeOutcome{}, newError(CodeConflict, "supervision was lost when the service restarted and execution is uncertain; resume the conversation first")
	}
	l := s.live[conversationID]
	switch {
	case l != nil && l.busy:
		s.mu.Unlock()
		return ModeOutcome{}, newError(CodeConflict, "the runtime is starting or the conversation is resuming")
	case l != nil && l.releasing:
		s.mu.Unlock()
		return ModeOutcome{}, newError(CodeConflict, "the runtime is stopping")
	case l != nil && l.switching:
		s.mu.Unlock()
		return ModeOutcome{}, newError(CodeConflict, "a permission mode change is already under way")
	case l == nil || l.runtime == nil:
		// no runtime: the request is all there is to change, and no harness is
		// asked. the operator's gesture is recorded as a mode_requested event,
		// a request and never a report of what runs; the next launch's init is
		// the report. a request the log cannot record is put back.
		err := s.requestModeLocked(conversationID, conv.PermissionMode, mode, "the next launch passes it")
		s.mu.Unlock()
		if err != nil {
			return ModeOutcome{}, err
		}
		return ModeOutcome{Outcome: ModeStored}, nil
	case l.facts == nil:
		s.mu.Unlock()
		return ModeOutcome{}, newError(CodeConflict, "the runtime is still starting")
	}
	// the request is recorded and written before claude is asked, as with no
	// runtime; claude's confirmation is its own mode_changed report.
	if err := s.requestModeLocked(conversationID, conv.PermissionMode, mode, "asked of Claude"); err != nil {
		s.mu.Unlock()
		return ModeOutcome{}, err
	}
	rt := l.runtime
	l.switching = true
	s.mu.Unlock()

	// the harness call runs without s.mu: a confirmed switch is reported to
	// the sink, which takes s.mu, before the call returns.
	switchErr := rt.SetPermissionMode(mode)

	s.mu.Lock()
	defer s.mu.Unlock()
	l.switching = false
	switch {
	case switchErr == nil:
		return ModeOutcome{Outcome: ModeConfirmed}, nil
	case errors.Is(switchErr, harness.ErrModeUnconfirmed):
		return ModeOutcome{Outcome: ModeUnconfirmed, Message: "claude did not confirm the switch in time; each turn's line shows the mode it ran in"}, nil
	}
	// refused: the request goes back to what it was before, recorded as the
	// request was.
	if err := s.requestModeLocked(conversationID, mode, conv.PermissionMode, fmt.Sprintf("Claude refused %s, so the request goes back", mode)); err != nil {
		dl.Errorf("putting back the requested permission mode of '%s': %v", conversationID, err)
	}
	return ModeOutcome{Outcome: ModeRefused, Message: switchErr.Error()}, nil
}

// requestModeLocked records and writes a requested mode, whether or not a
// runtime runs, and when a refusal puts one back: a service-origin
// mode_requested event first, then the registry, so the request is never
// stored unrecorded. the event is a request, never a report of what runs.
// interrupted between the two, the next start applies the request from its
// event (settleModeRequestLocked). a registry write that fails is recorded
// too, by a second event putting the request back, so the log's last word is
// again what the registry holds. callers hold s.mu, across both writes.
func (s *Service) requestModeLocked(conversationID, before, mode, text string) error {
	l, err := s.liveLocked(conversationID)
	if err != nil {
		return err
	}
	requested := func(mode, text string) error {
		return s.record(l, &model.Event{
			Kind:   model.EventRuntime,
			Status: model.StatusModeRequested,
			Origin: model.OriginService,
			Text:   text,
			Facts:  &model.RuntimeFacts{PermissionMode: mode},
		})
	}
	if err := requested(mode, text); err != nil {
		return err
	}
	if err := s.updateConversation(conversationID, func(c *model.Conversation) { c.PermissionMode = mode }); err != nil {
		if backErr := requested(before, "the request could not be stored, so it stays as it was"); backErr != nil {
			return fmt.Errorf("the request was recorded but could not be stored; the next start applies it: %w", err)
		}
		return err
	}
	return nil
}

// settleModeRequestLocked finishes a mode request a crash interrupted
// between its event and its registry write. requestModeLocked holds s.mu across
// both, so nothing else reaches the log in between: such a request is the
// log's last event, a mode_requested naming another mode than the registry
// holds. it runs at start-up, before anything else is recorded. callers hold
// s.mu.
func (s *Service) settleModeRequestLocked(conv model.Conversation, l *live) {
	if conv.Harness != model.HarnessClaude {
		return
	}
	tail, err := l.log.Tail(1)
	if err != nil || len(tail) == 0 {
		return
	}
	last := tail[0]
	if last.Kind != model.EventRuntime || last.Status != model.StatusModeRequested || last.Facts == nil {
		return
	}
	mode := last.Facts.PermissionMode
	if mode == "" || mode == conv.PermissionMode || !slices.Contains(model.PermissionModes, mode) {
		return
	}
	if err := s.updateConversation(conv.ID, func(c *model.Conversation) { c.PermissionMode = mode }); err != nil {
		dl.Errorf("applying the recorded mode request of '%s': %v", conv.ID, err)
		return
	}
	dl.Infof("applied the mode request '%s' recorded for '%s' before the last stop", mode, conv.ID)
}

// harnessModeChanged records a permission mode the runtime reported, in the
// order it reported them: a switch claude confirmed, early or late, or a
// change claude made itself, such as leaving plan mode. it is the one writer
// of the live mode, and like every transition it shows only once recorded; a
// log that cannot record it keeps the last confirmed mode on screen. the
// recorded choice is left alone. callers hold s.mu.
func (s *Service) harnessModeChanged(l *live, ev *model.Event) {
	mode := ""
	if ev.Facts != nil {
		mode = ev.Facts.PermissionMode
	}
	if err := s.transition(l, ev, l.state, func() { l.setMode(mode) }); err != nil {
		dl.Errorf("%v", err)
	}
}

// setMode replaces the shown facts with a copy carrying mode, as the harness
// reported or confirmed it. the facts the started event delivered are never
// mutated, since that event may still be on its way to subscribers.
func (l *live) setMode(mode string) {
	if l.facts == nil {
		return
	}
	facts := *l.facts
	facts.PermissionMode = mode
	l.facts = &facts
}

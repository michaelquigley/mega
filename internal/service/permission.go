package service

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/michaelquigley/df/dl"

	"git.hq.quigley.com/research/roundhouse/internal/harness"
	"git.hq.quigley.com/research/roundhouse/internal/model"
)

// checkPermissionMode validates an operator's choice of permission mode for a
// conversation on harnessName. empty is roundhouse's default. only claude has
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

// SetPermissionMode records the mode the operator chose for a claude
// conversation and, if a runtime is running, asks it to switch. the choice is
// intent: it is written on the conversation first, as the mode the next
// launch passes. the service records no mode change of its own. the live mode
// comes only from what the runtime reports, recorded by harnessModeChanged in
// the order claude reported it, so there is exactly one writer of it.
//
// with no runtime the outcome is stored: only the choice was written. an
// unsupervised conversation, or one starting, stopping or resuming, is
// refused. with a runtime the outcome is the harness's answer. confirmed: the
// runtime has already reported the new mode. refused: the choice goes back to
// what it was before, possibly no choice, in one registry write. unconfirmed:
// the choice stands and the live mode is whatever claude last reported, which
// the header shows apart from the choice when they differ. a pending request
// is never touched.
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
		// no runtime: the choice is all there is to change. no harness call
		// and no runtime event; the next launch's init is the report.
		err := s.updateConversation(conversationID, func(c *model.Conversation) { c.PermissionMode = mode })
		s.mu.Unlock()
		if err != nil {
			return ModeOutcome{}, err
		}
		return ModeOutcome{Outcome: ModeStored}, nil
	case l.facts == nil:
		s.mu.Unlock()
		return ModeOutcome{}, newError(CodeConflict, "the runtime is still starting")
	}
	if err := s.updateConversation(conversationID, func(c *model.Conversation) { c.PermissionMode = mode }); err != nil {
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
		return ModeOutcome{Outcome: ModeUnconfirmed, Message: "claude did not confirm the switch in time; the header shows the mode claude last reported"}, nil
	}
	// refused: the choice goes back to what it was before this request, which
	// may be no choice at all (roundhouse's default, inheriting the settings).
	back := conv.PermissionMode
	if err := s.updateConversation(conversationID, func(c *model.Conversation) { c.PermissionMode = back }); err != nil {
		dl.Errorf("putting back the recorded permission mode of '%s': %v", conversationID, err)
	}
	return ModeOutcome{Outcome: ModeRefused, Message: switchErr.Error()}, nil
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
	inherited := ev.Facts != nil && ev.Facts.ModeInherited
	if err := s.transition(l, ev, l.state, func() { l.setMode(mode, inherited) }); err != nil {
		dl.Errorf("%v", err)
	}
}

// setMode replaces the shown facts with a copy carrying mode, as the harness
// reported or confirmed it, so no longer only requested. the facts the started
// event delivered are never mutated, since that event may still be on its way
// to subscribers. a changed mode is no longer the one the operator's settings
// chose; a first report of the inherited mode says it still is.
func (l *live) setMode(mode string, inherited bool) {
	if l.facts == nil {
		return
	}
	facts := *l.facts
	facts.PermissionMode = mode
	facts.ModeInherited = inherited
	facts.ModeRequested = false
	l.facts = &facts
}

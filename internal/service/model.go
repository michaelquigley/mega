package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sync"
	"time"

	"github.com/michaelquigley/df/dl"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

// LaunchChoice is what the operator chose at launch: the permission mode
// (claude only), the model and the effort. empty fields inherit.
type LaunchChoice struct {
	PermissionMode string
	Model          string
	Effort         string
}

// outcomes of a model or effort change, as the harness answered it, beside
// the permission-mode outcomes.
const (
	// ModelNextPrompt: the change is stored and goes with the next prompt
	// (codex takes overrides per turn).
	ModelNextPrompt = "next_prompt"
)

// modelsTimeout bounds asking a harness for its models or its settings.
const modelsTimeout = 30 * time.Second

// Models is what a harness offers for a checkout, and what a launch with no
// choice gets there.
type Models struct {
	Version         string
	Models          []harness.ModelOption
	InheritedModel  string
	InheritedEffort string
}

// catalogs caches each harness's model list, with the version it came from,
// for the service's life.
type catalogs struct {
	mu   sync.Mutex
	byID map[string]harness.Catalog
}

func (c *catalogs) get(name string) (harness.Catalog, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	catalog, ok := c.byID[name]
	return catalog, ok
}

func (c *catalogs) put(name string, catalog harness.Catalog) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byID == nil {
		c.byID = map[string]harness.Catalog{}
	}
	c.byID[name] = catalog
}

// Models lists a harness's models for a project's checkout, from the cache
// after the first ask, and reads afresh what inherit means there. listing
// models starts no conversation.
func (s *Service) Models(projectID, harnessName string) (Models, error) {
	s.mu.Lock()
	project, err := s.projectLocked(projectID)
	h, ok := s.harnesses[harnessName]
	s.mu.Unlock()
	if err != nil {
		return Models{}, err
	}
	if !ok {
		return Models{}, newError(CodeInvalid, fmt.Sprintf("harness '%s' is not available", harnessName))
	}
	ctx, cancel := context.WithTimeout(context.Background(), modelsTimeout)
	defer cancel()
	catalog, cached := s.catalogs.get(harnessName)
	if !cached {
		catalog, err = h.Models(ctx, project.Checkout)
		if err != nil {
			return Models{}, s.explain(harnessName, fmt.Errorf("listing '%s' models: %w", harnessName, err))
		}
		s.catalogs.put(harnessName, catalog)
	}
	inheritedModel, inheritedEffort, err := h.Inherited(ctx, project.Checkout)
	if err != nil {
		// what inherit means is a courtesy; the list stands without it.
		dl.Warnf("reading what '%s' inherits in '%s': %v", harnessName, project.Checkout, err)
	}
	return Models{Version: catalog.Version, Models: catalog.Models, InheritedModel: inheritedModel, InheritedEffort: inheritedEffort}, nil
}

// safeChoice is what a model or effort may look like when no catalog is at
// hand to check it against: it is passed to a harness's command line.
var safeChoice = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/\[\]-]*$`)

// checkModelChoice validates a model and effort against the harness's
// catalog, when one is cached, and their form otherwise. empty inherits.
func (s *Service) checkModelChoice(harnessName, chosenModel, effort string) error {
	for _, value := range []string{chosenModel, effort} {
		if value != "" && !safeChoice.MatchString(value) {
			return newError(CodeInvalid, fmt.Sprintf("'%s' is not a model or effort level", value))
		}
	}
	catalog, ok := s.catalogs.get(harnessName)
	if !ok {
		return nil
	}
	if chosenModel != "" {
		i := slices.IndexFunc(catalog.Models, func(m harness.ModelOption) bool { return m.ID == chosenModel })
		if i < 0 {
			return newError(CodeInvalid, fmt.Sprintf("model '%s' is not offered by '%s'", chosenModel, harnessName))
		}
		if effort != "" && !slices.Contains(catalog.Models[i].Efforts, effort) {
			return newError(CodeInvalid, fmt.Sprintf("model '%s' does not offer effort '%s'", chosenModel, effort))
		}
		return nil
	}
	if effort != "" && !slices.ContainsFunc(catalog.Models, func(m harness.ModelOption) bool { return slices.Contains(m.Efforts, effort) }) {
		return newError(CodeInvalid, fmt.Sprintf("effort '%s' is not offered by '%s'", effort, harnessName))
	}
	return nil
}

// offersEffort is whether a model takes an effort level: false only when the
// cached catalog lists it with none, as claude's haiku.
func (s *Service) offersEffort(harnessName, chosenModel string) bool {
	catalog, ok := s.catalogs.get(harnessName)
	if !ok {
		return true
	}
	i := slices.IndexFunc(catalog.Models, func(m harness.ModelOption) bool { return m.ID == chosenModel })
	return i < 0 || len(catalog.Models[i].Efforts) > 0
}

// SetModel records the model and effort the operator chose for a
// conversation and, if a runtime is running, asks it to change. the choice is
// intent: written first, as what every later launch passes (and, for codex,
// every prompt). with no runtime the outcome is stored. while one runs, inherit
// cannot be chosen, since neither harness returns to the operator's settings
// mid-session (a model with no effort levels leaves the effort unchosen); the
// harness's answer is the outcome: confirmed (claude applies
// it from the next response), next_prompt (codex takes it with the next
// turn), refused (the choice goes back), or unconfirmed (the choice stands).
// the live model and effort are never written here: the harness reports them.
func (s *Service) SetModel(conversationID, chosenModel, effort string) (ModeOutcome, error) {
	s.mu.Lock()
	conv, err := s.conversationLocked(conversationID)
	if err != nil {
		s.mu.Unlock()
		return ModeOutcome{}, err
	}
	if err := s.checkModelChoice(conv.Harness, chosenModel, effort); err != nil {
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
		return ModeOutcome{}, newError(CodeConflict, "a change is already under way")
	case l == nil || l.runtime == nil:
		err := s.updateConversation(conversationID, func(c *model.Conversation) { c.Model, c.Effort = chosenModel, effort })
		s.mu.Unlock()
		if err != nil {
			return ModeOutcome{}, err
		}
		return ModeOutcome{Outcome: ModeStored}, nil
	case l.facts == nil:
		s.mu.Unlock()
		return ModeOutcome{}, newError(CodeConflict, "the runtime is still starting")
	}
	// codex can return to inherit while running: its adapter sends the
	// config's current values on every turn, so inherit takes effect at the
	// next prompt like any other change. claude cannot: its reset calls go to
	// claude code's built-in defaults, not the operator's settings.
	inheritRunning := conv.Harness == model.HarnessCodex
	if !inheritRunning && ((chosenModel == "" && conv.Model != "") || (effort == "" && conv.Effort != "" && s.offersEffort(conv.Harness, chosenModel))) {
		s.mu.Unlock()
		return ModeOutcome{}, newError(CodeInvalid, "inherit can be chosen only while no runtime runs; pick a model and effort, or release first")
	}
	if err := s.updateConversation(conversationID, func(c *model.Conversation) { c.Model, c.Effort = chosenModel, effort }); err != nil {
		s.mu.Unlock()
		return ModeOutcome{}, err
	}
	rt := l.runtime
	l.switching = true
	s.mu.Unlock()

	// the harness call runs without s.mu: claude's reply arrives on the
	// runtime's reader, which may first deliver an event to the sink.
	deferred, switchErr := rt.SetModel(chosenModel, effort)

	s.mu.Lock()
	defer s.mu.Unlock()
	l.switching = false
	switch {
	case switchErr == nil:
		// until a turn begun from here ends, the header shows what was asked
		// for as the next prompt's; the harness's report then stands alone.
		if chosenModel != conv.Model {
			l.nextModel = pendingSetting{value: chosenModel, set: true}
		}
		if effort != conv.Effort {
			l.nextEffort = pendingSetting{value: effort, set: true}
		}
		l.nextSent = false
		s.changed()
		if deferred {
			return ModeOutcome{Outcome: ModelNextPrompt}, nil
		}
		return ModeOutcome{Outcome: ModeConfirmed}, nil
	case errors.Is(switchErr, harness.ErrSettingUnconfirmed):
		return ModeOutcome{Outcome: ModeUnconfirmed, Message: "the harness did not confirm the change in time; the header shows the model it last reported"}, nil
	case errors.Is(switchErr, harness.ErrEffortRefused):
		// the model changed and stays chosen; only the effort goes back to the
		// last one accepted.
		if err := s.updateConversation(conversationID, func(c *model.Conversation) { c.Effort = conv.Effort }); err != nil {
			dl.Errorf("putting back the recorded effort of '%s': %v", conversationID, err)
		}
		if chosenModel != conv.Model {
			l.nextModel, l.nextSent = pendingSetting{value: chosenModel, set: true}, false
		}
		s.changed()
		return ModeOutcome{Outcome: ModeRefused, Message: switchErr.Error()}, nil
	}
	if err := s.updateConversation(conversationID, func(c *model.Conversation) { c.Model, c.Effort = conv.Model, conv.Effort }); err != nil {
		dl.Errorf("putting back the recorded model of '%s': %v", conversationID, err)
	}
	return ModeOutcome{Outcome: ModeRefused, Message: switchErr.Error()}, nil
}

// harnessModelChanged records a model or effort the runtime reported, in the
// order it reported them: the one writer of the live model and effort. it
// shows once recorded, like every transition. callers hold s.mu.
func (s *Service) harnessModelChanged(l *live, ev *model.Event) {
	if ev.Facts == nil {
		return
	}
	reported := *ev.Facts
	err := s.transition(l, ev, l.state, func() {
		if l.facts == nil {
			return
		}
		facts := *l.facts
		if reported.Model != "" {
			facts.Model = reported.Model
		}
		if reported.Effort != "" {
			facts.Effort = reported.Effort
		}
		l.facts = &facts
	})
	if err != nil {
		dl.Errorf("%v", err)
	}
}

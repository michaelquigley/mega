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
// (claude only), the model and the effort. a field left empty is copied once
// from the harness's settings (copyFor).
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

// Models is what a harness offers for a checkout, and what a new
// conversation there copies from the harness's settings. Copy.Effort is
// empty, with NeedEffort set, when the settings give the copied model no
// effort level and the list names no default: the operator chooses one.
type Models struct {
	Version    string
	Models     []harness.ModelOption
	Copy       model.TurnSettings
	NeedEffort bool
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
// after the first ask, and reads afresh what a new conversation there would
// copy from the harness's settings. listing models starts no conversation.
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
	// the harness runs in the project's environment, which must be one mega
	// may run.
	place, err := s.placeFor(projectID)
	if err != nil {
		return Models{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), modelsTimeout)
	defer cancel()
	catalog, err := s.catalogFor(ctx, harnessName, h, place)
	if err != nil {
		return Models{}, err
	}
	copied, needEffort, err := s.copyFor(ctx, harnessName, place, model.TurnSettings{})
	if err != nil {
		// what a new conversation copies is a courtesy; the list stands
		// without it.
		dl.Warnf("reading what a new '%s' conversation copies in '%s': %v", harnessName, project.Checkout, err)
	}
	return Models{Version: catalog.Version, Models: catalog.Models, Copy: copied, NeedEffort: needEffort}, nil
}

// safeChoice is what a model or effort may look like when no catalog is at
// hand to check it against: it is passed to a harness's command line.
var safeChoice = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/\[\]-]*$`)

// checkModelChoice validates a model and effort against the harness's
// catalog, when one is cached, and their form otherwise. a full id the list
// holds only as an alias's resolved model, as a model filled in from what
// claude reported, is checked against that alias's row (catalogRow). an
// empty field is one still to be copied (copyFor), or an effort for a model
// that takes none.
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
		row, ok := catalogRow(catalog, chosenModel)
		if !ok {
			return newError(CodeInvalid, fmt.Sprintf("model '%s' is not offered by '%s'", chosenModel, harnessName))
		}
		if effort != "" && !slices.Contains(row.Efforts, effort) {
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
	row, ok := catalogRow(catalog, chosenModel)
	return !ok || len(row.Efforts) > 0
}

// SetModel records the model and effort the operator requests for a
// conversation and, if a runtime is running, asks it to change. the request
// is written first, as what every later launch passes (and, for codex, every
// prompt). with no runtime the outcome is stored. both are always concrete:
// a model, and an effort wherever the model takes one. the harness's answer
// is the outcome: confirmed (claude applies it from the next response),
// next_prompt (codex takes it with the next turn), refused (the request goes
// back), or unconfirmed (the request stands). what a turn ran with is the
// harness's report, recorded with the turn, never written here.
func (s *Service) SetModel(conversationID, chosenModel, effort string) (ModeOutcome, error) {
	s.mu.Lock()
	conv, err := s.conversationLocked(conversationID)
	if err != nil {
		s.mu.Unlock()
		return ModeOutcome{}, err
	}
	if chosenModel == "" {
		s.mu.Unlock()
		return ModeOutcome{}, newError(CodeInvalid, "a model is required")
	}
	if effort == "" && s.offersEffort(conv.Harness, chosenModel) {
		s.mu.Unlock()
		return ModeOutcome{}, newError(CodeInvalid, fmt.Sprintf("model '%s' takes an effort level; choose one", chosenModel))
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
		s.changed()
		if deferred {
			return ModeOutcome{Outcome: ModelNextPrompt}, nil
		}
		return ModeOutcome{Outcome: ModeConfirmed}, nil
	case errors.Is(switchErr, harness.ErrSettingUnconfirmed):
		return ModeOutcome{Outcome: ModeUnconfirmed, Message: "the harness did not confirm the change in time; each turn's line shows what it ran with"}, nil
	case errors.Is(switchErr, harness.ErrEffortRefused):
		// the model changed and stays requested; only the effort goes back to
		// the last one accepted.
		if err := s.updateConversation(conversationID, func(c *model.Conversation) { c.Effort = conv.Effort }); err != nil {
			dl.Errorf("putting back the recorded effort of '%s': %v", conversationID, err)
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

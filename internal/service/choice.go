package service

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/michaelquigley/df/dl"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

// what a conversation requests: its model, effort and (claude) permission
// mode. a new conversation's are concrete from creation, copied once from the
// harness's settings (copyFor) where the operator chose none; after that only
// the operator changes them. conversations created before that held none may
// lack one: the start-up fill (fillChoicesLocked) takes what their log last
// reported, and the fill at launch (fillAtLaunch) takes the rest from the
// settings, each recorded once as a choice_filled event.

// fill sources, as the choice_filled event names them.
const (
	filledFromReport   = "filled in from what the harness last reported"
	filledFromSettings = "filled in from the harness's settings"
)

// errChooseEffort refuses a claude launch the settings give no effort level:
// claude's model list names no default effort and claude never reports one,
// so any level mega picked would be a guess (Michael's call, 2026-11-13).
var errChooseEffort = newError(CodeInvalid, "the Claude settings name no effort level, so choose one for this conversation")

// catalogFor is the harness's model list, from the cache after the first
// ask. callers do not hold s.mu.
func (s *Service) catalogFor(ctx context.Context, harnessName string, h harness.Harness, place harness.Place) (harness.Catalog, error) {
	if catalog, ok := s.catalogs.get(harnessName); ok {
		return catalog, nil
	}
	catalog, err := h.Models(ctx, place)
	if err != nil {
		return harness.Catalog{}, s.explain(harnessName, place.Find, fmt.Errorf("listing '%s' models: %w", harnessName, err))
	}
	s.catalogs.put(harnessName, catalog)
	return catalog, nil
}

// copyFor completes have, a request with some fields chosen, from the
// harness's settings and its model list, as a new conversation copies them:
//   - the model: have's; else the settings' when the list holds it as a row;
//     else the list's default row; else the settings' as it is, when no list
//     could be had.
//   - the effort: have's; else the settings' when the model offers it; else
//     the model's default level where the list names one (codex); none for a
//     model that takes none. a model that takes levels and has none to copy
//     reports needEffort: the operator must choose (errChooseEffort).
//   - the mode (claude): have's; else the settings' when mega offers it; else
//     default. a mode mega never passes (bypassPermissions, dontAsk) is never
//     copied (permissions-never-weakened).
//
// it reads only: the settings files and the model list. callers do not hold
// s.mu.
func (s *Service) copyFor(ctx context.Context, harnessName string, place harness.Place, have model.TurnSettings) (out model.TurnSettings, needEffort bool, err error) {
	h, ok := s.harnesses[harnessName]
	if !ok {
		return model.TurnSettings{}, false, newError(CodeInvalid, fmt.Sprintf("harness '%s' is not available", harnessName))
	}
	settings, err := h.Settings(ctx, place)
	if err != nil {
		// settings that cannot be read leave the list's defaults.
		dl.Warnf("reading the '%s' settings in '%s': %v", harnessName, place.Cwd, err)
		settings = model.TurnSettings{}
	}
	catalog, catalogErr := s.catalogFor(ctx, harnessName, h, place)
	out = have
	row := func(id string) (harness.ModelOption, bool) {
		i := slices.IndexFunc(catalog.Models, func(m harness.ModelOption) bool { return m.ID == id })
		if i < 0 {
			return harness.ModelOption{}, false
		}
		return catalog.Models[i], true
	}
	if out.Model == "" {
		if _, ok := row(settings.Model); ok && settings.Model != "" {
			out.Model = settings.Model
		} else if i := slices.IndexFunc(catalog.Models, func(m harness.ModelOption) bool { return m.IsDefault }); i >= 0 {
			out.Model = catalog.Models[i].ID
		} else if len(catalog.Models) > 0 {
			out.Model = catalog.Models[0].ID
		} else if settings.Model != "" && safeChoice.MatchString(settings.Model) {
			out.Model = settings.Model
		} else if catalogErr != nil {
			return model.TurnSettings{}, false, catalogErr
		} else {
			return model.TurnSettings{}, false, newError(CodeInvalid, fmt.Sprintf("'%s' offers no model to copy", harnessName))
		}
	}
	if out.Effort == "" {
		chosen, known := catalogRow(catalog, out.Model)
		switch {
		case known && len(chosen.Efforts) == 0:
			// a model that takes no effort level has none to copy.
		case known && settings.Effort != "" && slices.Contains(chosen.Efforts, settings.Effort):
			out.Effort = settings.Effort
		case known && chosen.DefaultEffort != "" && slices.Contains(chosen.Efforts, chosen.DefaultEffort):
			out.Effort = chosen.DefaultEffort
		case !known && settings.Effort != "" && safeChoice.MatchString(settings.Effort):
			out.Effort = settings.Effort
		default:
			needEffort = true
		}
	}
	if harnessName == model.HarnessClaude {
		if out.Mode == "" {
			out.Mode = model.PermissionModeDefault
			if slices.Contains(model.PermissionModes, settings.Mode) {
				out.Mode = settings.Mode
			}
		}
	} else {
		out.Mode = ""
	}
	return out, needEffort, nil
}

// missingLocked names what a conversation's request lacks: a model; an effort
// for a model that takes one (as far as the cached list says); a mode for
// claude. callers hold s.mu.
func (s *Service) missingLocked(conv model.Conversation) bool {
	if conv.Model == "" {
		return true
	}
	if conv.Effort == "" && s.offersEffort(conv.Harness, conv.Model) {
		return true
	}
	return conv.Harness == model.HarnessClaude && conv.PermissionMode == ""
}

// fillAtLaunch fills, before a launch, what a conversation created before
// every conversation held a concrete request still lacks: from its log's
// own earlier fill, else from the harness's settings, as a new conversation
// copies them. it is recorded once, as a choice_filled event and the
// registry write. a claude conversation whose settings give no effort is
// refused (errChooseEffort): the operator chooses one on the control bar.
// callers do not hold s.mu.
func (s *Service) fillAtLaunch(conversationID string, place harness.Place) error {
	s.mu.Lock()
	conv, err := s.conversationLocked(conversationID)
	if err != nil || !s.missingLocked(conv) {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), modelsTimeout)
	defer cancel()
	have := model.TurnSettings{Model: conv.Model, Effort: conv.Effort, Mode: conv.PermissionMode}
	filled, needEffort, err := s.copyFor(ctx, conv.Harness, place, have)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.applyFillLocked(conversationID, filled, filledFromSettings); err != nil {
		return err
	}
	if needEffort {
		return errChooseEffort
	}
	return nil
}

// applyFillLocked writes the fields a conversation still lacks from filled,
// with a choice_filled event recording what was filled and from where,
// before the registry write: interrupted between the two, the next fill
// finds the event and applies it again without recording another. a field
// the operator has set meanwhile is kept. callers hold s.mu.
func (s *Service) applyFillLocked(conversationID string, filled model.TurnSettings, source string) error {
	conv, err := s.conversationLocked(conversationID)
	if err != nil {
		return err
	}
	fill := model.TurnSettings{}
	if conv.Model == "" {
		fill.Model = filled.Model
	}
	if conv.Effort == "" {
		fill.Effort = filled.Effort
	}
	if conv.PermissionMode == "" && conv.Harness == model.HarnessClaude {
		fill.Mode = filled.Mode
	}
	if fill == (model.TurnSettings{}) {
		return nil
	}
	l, err := s.liveLocked(conversationID)
	if err != nil {
		return err
	}
	ev := &model.Event{
		Kind:   model.EventRuntime,
		Status: model.StatusChoiceFilled,
		Origin: model.OriginService,
		Text:   source,
		Facts:  &model.RuntimeFacts{Model: fill.Model, Effort: fill.Effort, PermissionMode: fill.Mode},
	}
	if err := s.record(l, ev); err != nil {
		return fmt.Errorf("recording the fill of '%s': %w", conversationID, err)
	}
	return s.updateConversation(conversationID, func(c *model.Conversation) {
		if c.Model == "" {
			c.Model = fill.Model
		}
		if c.Effort == "" {
			c.Effort = fill.Effort
		}
		if c.PermissionMode == "" {
			c.PermissionMode = fill.Mode
		}
	})
}

// fillChoicesLocked is the start-up fill: every conversation that lacks a
// model, an effort (codex) or a mode (claude) takes what its log last
// reported, in the order the harness reported it. a log that already holds
// a fill applies that fill and records nothing more. it reads only the log,
// so it runs before any harness can be asked; what the log cannot give (a
// conversation that never reported, or claude's effort, which claude never
// reports) is filled at the conversation's next launch. a second run finds
// nothing to fill. callers hold s.mu.
func (s *Service) fillChoicesLocked() {
	for _, conv := range s.reg.Snapshot().Conversations {
		lacks := conv.Model == "" || (conv.Harness == model.HarnessClaude && conv.PermissionMode == "") || (conv.Harness != model.HarnessClaude && conv.Effort == "")
		if !lacks {
			continue
		}
		l, err := s.liveLocked(conv.ID)
		if err != nil {
			dl.Warnf("filling '%s': its log cannot be read: %v", conv.ID, err)
			continue
		}
		evs, err := l.log.Since(0)
		if err != nil {
			dl.Warnf("filling '%s': reading its log: %v", conv.ID, err)
			continue
		}
		reported, earlier := lastReported(conv.Harness, evs)
		if earlier != nil {
			// a fill recorded before an interruption: applied again, recorded once.
			if err := s.updateConversation(conv.ID, func(c *model.Conversation) {
				if c.Model == "" {
					c.Model = earlier.Model
				}
				if c.Effort == "" {
					c.Effort = earlier.Effort
				}
				if c.PermissionMode == "" {
					c.PermissionMode = earlier.Mode
				}
			}); err != nil {
				dl.Errorf("filling '%s' from its recorded fill: %v", conv.ID, err)
			}
			continue
		}
		if reported == (model.TurnSettings{}) {
			continue
		}
		if err := s.applyFillLocked(conv.ID, reported, filledFromReport); err != nil {
			dl.Errorf("filling '%s' from its log: %v", conv.ID, err)
		}
	}
}

// lastReported is what a log says the harness last ran with: the model, the
// effort (codex reports it; claude never does) and the mode (claude), from
// the runtime's started, model_changed and mode_changed events in order. a
// reported mode mega never passes is taken as default. earlier is the
// newest choice_filled the log holds, if any.
func lastReported(harnessName string, evs []*model.Event) (reported model.TurnSettings, earlier *model.TurnSettings) {
	for _, ev := range evs {
		if ev.Kind != model.EventRuntime || ev.Facts == nil {
			continue
		}
		f := ev.Facts
		switch ev.Status {
		case model.StatusChoiceFilled:
			earlier = &model.TurnSettings{Model: f.Model, Effort: f.Effort, Mode: f.PermissionMode}
		case model.StatusStarted, model.StatusModelChanged, model.StatusModeChanged:
			if f.Model != "" && ev.Status != model.StatusModeChanged {
				reported.Model = f.Model
			}
			if f.Effort != "" && harnessName != model.HarnessClaude {
				reported.Effort = f.Effort
			}
			if f.PermissionMode != "" && harnessName == model.HarnessClaude && ev.Status != model.StatusModelChanged {
				reported.Mode = f.PermissionMode
			}
		}
	}
	if reported.Mode != "" && !slices.Contains(model.PermissionModes, reported.Mode) {
		reported.Mode = model.PermissionModeDefault
	}
	return reported, earlier
}

// beginTurnLocked marks a turn in flight from at, the time its first event
// was recorded (now, if it was not recorded), and keeps what it asks for:
// the conversation's request as it stands now, which every launch and, for
// codex, every prompt passes. callers hold s.mu.
func (s *Service) beginTurnLocked(l *live, at time.Time) {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	l.turnActive, l.turnStartedAt = true, at
	l.turnAsked, l.turnRan = model.TurnSettings{}, nil
	if conv, err := s.conversationLocked(l.id); err == nil {
		l.turnAsked = model.TurnSettings{Model: conv.Model, Effort: conv.Effort, Mode: conv.PermissionMode}
	}
}

// ranLocked keeps what the turn in flight ran with, once: the harness's
// reports as they stand when its first output arrives, by which time the
// turn's own report (claude's init, codex's settings update) has been
// recorded. a mode switched later in the turn draws its own row and leaves
// this alone. claude never reports effort, so its effort is the one sent;
// codex has no permission mode. callers hold s.mu.
func (s *Service) ranLocked(l *live) {
	if !l.turnActive || l.turnRan != nil || l.facts == nil {
		return
	}
	conv, err := s.conversationLocked(l.id)
	if err != nil {
		return
	}
	ran := model.TurnSettings{Model: l.facts.Model, Effort: l.facts.Effort, Mode: l.facts.PermissionMode}
	if conv.Harness == model.HarnessClaude {
		ran.Effort = l.turnAsked.Effort
	} else {
		ran.Mode = ""
	}
	l.turnRan = &ran
}

// turnRecordLocked is what the turn ending now asked for and ran with, for
// its turn_end event, or nil when the harness reported nothing to say what it
// ran with. callers hold s.mu.
func (s *Service) turnRecordLocked(l *live) *model.TurnRecord {
	s.ranLocked(l)
	if l.turnRan == nil {
		return nil
	}
	conv, err := s.conversationLocked(l.id)
	if err != nil {
		return nil
	}
	record := &model.TurnRecord{Asked: l.turnAsked, Ran: *l.turnRan}
	record.Differs = s.differs(conv.Harness, record.Asked, record.Ran)
	return record
}

// differs names where a turn ran with other than it asked for: the model,
// compared through the model list (claude asks by alias and reports the
// model it resolves to), the effort, and the mode. a field either side lacks
// is not compared, and a model is not called different without a list to
// say so.
func (s *Service) differs(harnessName string, asked, ran model.TurnSettings) []string {
	var out []string
	if asked.Model != "" && ran.Model != "" && !s.sameModel(harnessName, asked.Model, ran.Model) {
		out = append(out, "model")
	}
	if asked.Effort != "" && ran.Effort != "" && asked.Effort != ran.Effort {
		out = append(out, "effort")
	}
	if asked.Mode != "" && ran.Mode != "" && asked.Mode != ran.Mode {
		out = append(out, "mode")
	}
	return out
}

// catalogRow is the list's row for a model a request names: the row with
// that id, else the row of an alias that resolves to it, as a model filled
// in from what claude reported is a full id. the default row, which moves
// with claude's versions, is the last resort.
func catalogRow(catalog harness.Catalog, id string) (harness.ModelOption, bool) {
	if id == "" {
		return harness.ModelOption{}, false
	}
	for _, match := range []func(harness.ModelOption) bool{
		func(m harness.ModelOption) bool { return m.ID == id },
		func(m harness.ModelOption) bool { return m.Resolved == id && !m.IsDefault },
		func(m harness.ModelOption) bool { return m.Resolved == id },
	} {
		if i := slices.IndexFunc(catalog.Models, match); i >= 0 {
			return catalog.Models[i], true
		}
	}
	return harness.ModelOption{}, false
}

// sameModel is whether a model asked for and a model reported are the same,
// as the harness's model list resolves them.
func (s *Service) sameModel(harnessName, asked, ran string) bool {
	if asked == ran {
		return true
	}
	catalog, ok := s.catalogs.get(harnessName)
	if !ok {
		return true
	}
	resolve := func(id string) string {
		for _, m := range catalog.Models {
			if m.ID == id && m.Resolved != "" {
				return m.Resolved
			}
		}
		return id
	}
	return resolve(asked) == resolve(ran)
}

// prefetchCatalog lists a harness's models in the background once a runtime
// has started, so a turn's comparison has the list to resolve aliases with.
// callers do not hold s.mu.
func (s *Service) prefetchCatalog(harnessName string, place harness.Place) {
	if _, ok := s.catalogs.get(harnessName); ok {
		return
	}
	h, ok := s.harnesses[harnessName]
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), modelsTimeout)
	defer cancel()
	if _, err := s.catalogFor(ctx, harnessName, h, place); err != nil {
		dl.Debugf("listing '%s' models after a start: %v", harnessName, err)
	}
}

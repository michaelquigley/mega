package service

import (
	"context"
	"slices"

	"github.com/michaelquigley/df/dl"

	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/registry"
)

// recover settles what the previous run left. nothing is replayed: a request
// that was pending died with its runtime and is resolved lost, and a
// conversation still recorded running was never seen to exit, so it becomes
// unsupervised until the operator resumes it. native history is not imported
// here, because an orphaned runtime may still be writing.
func (s *Service) recover() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var orphaned []string
	for _, conv := range s.reg.Snapshot().Conversations {
		l, err := s.liveLocked(conv.ID)
		if err != nil {
			return err
		}
		if err := s.loseUnresolvedLocked(l); err != nil {
			return err
		}
		if conv.Runtime == model.RuntimeRunning {
			if err := s.record(l, &model.Event{
				Kind:   model.EventRuntime,
				Status: model.StatusUnsupervised,
				Text:   "the service restarted without seeing this runtime exit; supervision is lost and execution is uncertain",
			}); err != nil {
				return err
			}
			orphaned = append(orphaned, conv.ID)
		}
	}
	if len(orphaned) == 0 {
		return nil
	}
	return s.update(func(doc *registry.Document) error {
		for i := range doc.Conversations {
			if slices.Contains(orphaned, doc.Conversations[i].ID) {
				doc.Conversations[i].Runtime = model.RuntimeUnsupervised
			}
		}
		return nil
	})
}

// loseUnresolvedLocked resolves every request the log shows without a
// resolution. this reads history only to close it out; the pending set itself
// is never rebuilt from the log. callers hold s.mu.
func (s *Service) loseUnresolvedLocked(l *live) error {
	evs, err := l.log.Since(0)
	if err != nil {
		return err
	}
	var open []*model.Request
	for _, ev := range evs {
		if ev.Req == nil {
			continue
		}
		switch ev.Kind {
		case model.EventRequest:
			open = append(open, ev.Req)
		case model.EventRequestResolved:
			open = slices.DeleteFunc(open, func(r *model.Request) bool { return r.ID == ev.Req.ID })
		}
	}
	for _, req := range open {
		if err := s.record(l, &model.Event{
			Kind: model.EventRequestResolved,
			Text: model.OutcomeLost,
			Req:  &model.Request{ID: req.ID, Kind: req.Kind, Outcome: model.OutcomeLost},
		}); err != nil {
			return err
		}
	}
	return nil
}

// Shutdown releases every runtime the way a release does and waits, until ctx
// ends, for each to be seen to exit; each that exits records none. any still
// running at the deadline stays recorded running, so the next start reports
// it unsupervised instead of claiming it stopped.
func (s *Service) Shutdown(ctx context.Context) {
	s.mu.Lock()
	s.closed = true
	type stopping struct {
		id     string
		exited chan struct{}
	}
	var waits []stopping
	for _, l := range s.live {
		if l.runtime == nil {
			continue
		}
		waits = append(waits, stopping{id: l.id, exited: l.inc.exited})
		s.beginStopLocked(l, &model.Event{Status: model.StatusStopping})
	}
	s.mu.Unlock()

	for _, w := range waits {
		select {
		case <-w.exited:
		case <-ctx.Done():
			dl.Warnf("conversation '%s' did not stop before shutdown; it will be unsupervised at the next start", w.id)
		}
	}
}

// Close ends every subscription and closes the event logs. runtimes still
// alive after Shutdown are left to finish on their own.
func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for sub := range s.subs {
		s.drop(sub)
	}
	s.closeLogsLocked()
	s.stopWatchLocked()
}

func (s *Service) closeLogs() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeLogsLocked()
}

// closeLogsLocked closes every log and forgets the live state, so a late
// runtime callback finds no current incarnation and records nothing. callers
// hold s.mu.
func (s *Service) closeLogsLocked() {
	for id, l := range s.live {
		if err := l.log.Close(); err != nil {
			dl.Warnf("closing event log of '%s': %v", id, err)
		}
		delete(s.live, id)
	}
}

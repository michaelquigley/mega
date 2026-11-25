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
		s.settleModeRequestLocked(conv, l)
		evs, err := l.log.Since(0)
		if err != nil {
			return err
		}
		s.loseUnresolvedLocked(l, evs)
		if conv.Runtime == model.RuntimeRunning {
			if !lostSinceStart(evs) {
				s.record(l, &model.Event{
					Kind:   model.EventRuntime,
					Status: model.StatusUnsupervised,
					Text:   "the service restarted without seeing this runtime exit; supervision is lost and execution is uncertain",
				})
			}
			orphaned = append(orphaned, conv.ID)
		}
	}
	if len(orphaned) == 0 {
		return nil
	}
	s.update(func(doc *registry.Document) error {
		for i := range doc.Conversations {
			c := &doc.Conversations[i]
			if slices.Contains(orphaned, c.ID) {
				c.Runtime = model.RuntimeUnsupervised
			}
		}
		return nil
	})
	return nil
}

// lostSinceStart covers a restart between writing an unsupervised line and
// saving the registry state that goes with it, so a further restart adds no
// second line.
func lostSinceStart(evs []*model.Event) bool {
	for i := len(evs) - 1; i >= 0; i-- {
		ev := evs[i]
		if ev.HistoryBoundary() {
			return true
		}
		if ev.Kind == model.EventRuntime && ev.Status == model.StatusStarting {
			break
		}
	}
	return false
}

// loseUnresolvedLocked resolves every request the log shows without a
// resolution. this reads history only to close it out; the pending set itself
// is never rebuilt from the log. callers hold s.mu.
func (s *Service) loseUnresolvedLocked(l *live, evs []*model.Event) {
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
		s.resolve(l, req, model.OutcomeLost)
	}
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
	type launching struct {
		l   *live
		inc *incarnation
	}
	var waits []stopping
	var launches []launching
	for _, l := range s.live {
		if l.runtime == nil {
			// a launch past its preparation: it starts no harness from here,
			// or one it started is stopped as its launch returns; either way
			// the shutdown waits for it.
			if l.busy && l.inc != nil {
				launches = append(launches, launching{l: l, inc: l.inc})
			}
			continue
		}
		waits = append(waits, stopping{id: l.id, exited: l.inc.exited})
		s.beginStopLocked(l, &model.Event{Status: model.StatusStopping})
	}
	s.mu.Unlock()

	for _, w := range launches {
		select {
		case <-w.inc.launched:
		case <-ctx.Done():
			dl.Warnf("a launch of '%s' did not return before shutdown; it will be unsupervised at the next start", w.l.id)
			continue
		}
		s.mu.Lock()
		running := w.l.inc == w.inc && w.l.runtime != nil
		s.mu.Unlock()
		if running {
			waits = append(waits, stopping{id: w.l.id, exited: w.inc.exited})
		}
	}
	for _, w := range waits {
		select {
		case <-w.exited:
		case <-ctx.Done():
			dl.Warnf("conversation '%s' did not stop before shutdown; it will be unsupervised at the next start", w.id)
		}
	}
	s.mu.Lock()
	s.shutdownDone = true
	s.mu.Unlock()
}

// Close ends every subscription and closes the event logs. runtimes still
// alive after Shutdown are left to finish on their own.
func (s *Service) Close() {
	// a configuration snapshot under way finishes first, and none starts
	// after: nothing is written in the data directory once the service has
	// let it go.
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.snapshotting.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shutdownDone {
		// an orderly shutdown's last word, before every stream closes: a page
		// that hears it knows mega was shut down, and one that lost contact
		// first knows only that it was asked to.
		s.publish(Update{ShutdownFinished: true})
	}
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

// closeLogsLocked closes every log and lets go of the live state, so a late
// runtime callback finds no current incarnation and records nothing, and a
// command that finishes late changes a record no read can reach: from here
// every read is refused. callers hold s.mu.
func (s *Service) closeLogsLocked() {
	s.logsClosed = true
	for id, l := range s.live {
		if err := l.log.Close(); err != nil {
			dl.Warnf("closing event log of '%s': %v", id, err)
		}
		delete(s.live, id)
	}
}

package service

import (
	"context"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

// once Close has let the record go, every read is refused as unavailable,
// in the moment before the HTTP server stops: nothing is kept to answer one
// from, and no command's reply is built either. a read during the
// shutdown's release, before Close, is still answered.
func TestEveryReadIsRefusedAfterClose(t *testing.T) {
	r := newRig(t)
	conv, _ := r.atRest("one")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r.svc.Shutdown(ctx)
	if _, err := r.svc.Workspace(); err != nil {
		t.Fatalf("a workspace read after the shutdown, before Close: %v", err)
	}
	r.svc.Close()
	p, c := r.project.ID, conv.ID
	for name, read := range map[string]func() error{
		"workspace":        func() error { _, err := r.svc.Workspace(); return err },
		"harnesses":        func() error { _, err := r.svc.Harnesses(); return err },
		"snapshots":        func() error { _, err := r.svc.Snapshots(); return err },
		"folders":          func() error { _, err := r.svc.Folders(""); return err },
		"discover":         func() error { _, err := r.svc.DiscoverHeld(p, model.HarnessClaude); return err },
		"look":             func() error { _, err := r.svc.Look(p, model.HarnessClaude, "terminal-1"); return err },
		"models":           func() error { _, err := r.svc.Models(p, model.HarnessClaude); return err },
		"paths":            func() error { _, err := r.svc.Paths(p, ""); return err },
		"outside paths":    func() error { _, _, err := r.svc.OutsidePaths(p, "~/"); return err },
		"approved version": func() error { _, err := r.svc.ApprovedVersion(p); return err },
		"history":          func() error { _, err := r.svc.History(c, HistoryQuery{Turns: 10}); return err },
		"invocation":       func() error { _, err := r.svc.Invocation(c, 1); return err },
		"mcp":              func() error { _, err := r.svc.MCP(c); return err },
		"unknown history":  func() error { _, err := r.svc.History("no-such-conversation", HistoryQuery{}); return err },
		// a command whose write landed just before Close builds its reply here.
		"a command's reply": func() error { _, err := r.svc.view(c); return err },
	} {
		if err := read(); ErrorCode(err) != CodeUnavailable {
			t.Errorf("%s after close: %v, want unavailable", name, err)
		}
	}
}

// a command that finishes after Close changes nothing a read can show: an
// answer that returns after the shutdown's wait has run out takes its
// request out of a record no read reaches, and writes nothing, so the next
// start finds the request pending and resolves it lost.
func TestALateAnswerChangesNothingAReadCanShow(t *testing.T) {
	r := newRig(t)
	conv, rt := r.atRest("one")
	asked := rt.Ask(model.RequestQuestion, map[string]any{"questions": []any{map[string]any{"question": "Which?", "options": []any{map[string]any{"label": "A"}}}}})
	waitFor(t, "the request", func() bool { return len(r.view(conv.ID).Pending) == 1 })
	// the runtime outlives the shutdown's wait, and its question outlives the
	// release's interrupt, as pi's dialog does.
	rt.HoldOnClose()
	rt.HoldInterrupts()
	inFlight, returns := make(chan struct{}), make(chan struct{})
	rt.OnAnswer(func() {
		close(inFlight)
		<-returns
	})
	answered := make(chan error, 1)
	go func() {
		answered <- r.svc.Answer(conv.ID, asked, harness.Answer{Answers: map[string]string{"Which?": "A"}})
	}()
	<-inFlight

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	r.svc.Shutdown(ctx)
	r.svc.Close()
	close(returns)
	if err := <-answered; err != nil {
		t.Fatalf("the late answer: %v", err)
	}
	if _, err := r.svc.Workspace(); ErrorCode(err) != CodeUnavailable {
		t.Errorf("a workspace read after the late answer: %v, want unavailable", err)
	}
	if _, err := r.svc.History(conv.ID, HistoryQuery{}); ErrorCode(err) != CodeUnavailable {
		t.Errorf("a history read after the late answer: %v, want unavailable", err)
	}

	r.crashed()
	var outcomes []string
	for _, ev := range r.events(conv.ID) {
		if ev.Kind == model.EventRequestResolved && ev.Req.ID == asked {
			outcomes = append(outcomes, ev.Req.Outcome)
		}
	}
	if len(outcomes) != 1 || outcomes[0] != model.OutcomeLost {
		t.Errorf("the request ended %v, want lost", outcomes)
	}
}

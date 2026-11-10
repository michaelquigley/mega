package service

import (
	"os"
	"strings"
	"testing"
	"time"

	"git.hq.quigley.com/research/roundhouse/internal/model"
)

// turn_started_at is memory beside turn_active: set when a turn starts (from
// a prompt, or a turn the runtime opens), cleared when it ends or the runtime
// goes, and absent after a restart.
func TestTurnStartedAtFollowsTheTurnInFlight(t *testing.T) {
	r := newRig(t)
	before := time.Now().Add(-time.Second)
	conv := r.start("one")
	rt := r.fake.Last()
	startedIn(rt, "default")
	view := r.view(conv.ID)
	if !view.TurnActive || view.TurnStartedAt.Before(before) || view.TurnStartedAt.After(time.Now()) {
		t.Fatalf("after the prompt: active %v, started at %v", view.TurnActive, view.TurnStartedAt)
	}
	first := view.TurnStartedAt
	finishTurn(rt, "a-1", "done")
	if view := r.view(conv.ID); view.TurnActive || !view.TurnStartedAt.IsZero() {
		t.Errorf("after the turn ended: active %v, started at %v", view.TurnActive, view.TurnStartedAt)
	}

	// a turn the runtime opens itself, such as an answer starting the next.
	rt.Emit(&model.Event{Kind: model.EventAssistant, Text: "more", NativeID: "a-2"})
	if view := r.view(conv.ID); !view.TurnActive || view.TurnStartedAt.IsZero() || view.TurnStartedAt.Before(first) {
		t.Errorf("a runtime-opened turn: active %v, started at %v", view.TurnActive, view.TurnStartedAt)
	}

	// a turn in flight when the service goes is not one after a restart.
	r.restart()
	if view := r.view(conv.ID); view.TurnActive || !view.TurnStartedAt.IsZero() {
		t.Errorf("after a restart: active %v, started at %v", view.TurnActive, view.TurnStartedAt)
	}
	if !r.registryLacks(t, "started_at") {
		t.Error("the registry stores when the turn started")
	}
}

// a runtime that exits mid-turn takes its turn with it.
func TestTurnStartedAtClearsWhenTheRuntimeExits(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	startedIn(rt, "default")
	rt.Exit(nil)
	waitFor(t, "the exit to be recorded", func() bool { return r.view(conv.ID).State == model.RuntimeNone })
	if view := r.view(conv.ID); !view.TurnStartedAt.IsZero() {
		t.Errorf("after the exit: started at %v", view.TurnStartedAt)
	}
}

// registryLacks reports whether the registry file never mentions word.
func (r *rig) registryLacks(t *testing.T, word string) bool {
	t.Helper()
	data, err := os.ReadFile(r.path)
	if err != nil {
		t.Fatal(err)
	}
	return !strings.Contains(strings.ToLower(string(data)), word)
}

package service

import (
	"errors"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/michaelquigley/mega/internal/environ"
	"github.com/michaelquigley/mega/internal/harness/pi"
	"github.com/michaelquigley/mega/internal/model"
)

func TestRestartKeepsOneGapWhenTheExitWasRecordedButTheRegistryStillSaysRunning(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	finishTurn(rt, "reply", "done")
	r.svc.failUpdate = func() error { return errors.New("registry write interrupted") }
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the runtime to exit", rt.Exited)
	if r.view(conv.ID).Runtime != model.RuntimeRunning {
		t.Fatal("did not leave a stale running registry entry")
	}
	r.restart()
	if view := r.view(conv.ID); view.Runtime != model.RuntimeUnsupervised {
		t.Fatalf("recovered runtime = %s", view.Runtime)
	}
	gaps := 0
	for _, ev := range r.events(conv.ID) {
		if ev.HistoryBoundary() {
			gaps++
		}
	}
	if gaps != 1 {
		t.Fatalf("timeline contains %d gaps", gaps)
	}
	wantCode(t, r.svc.Start(conv.ID), CodeConflict)
	if err := r.svc.Resume(conv.ID); err != nil {
		t.Fatal(err)
	}
	if view := r.view(conv.ID); view.Attention != model.AttentionUnread {
		t.Fatalf("the unread reply was lost: %+v", view)
	}
}

func TestReleaseWithoutAConfirmedExitLeavesItsGapAtSupervisionLoss(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	started(rt)
	rt.HoldOnClose()
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "release to close input", rt.Closed)
	if slices.ContainsFunc(r.events(conv.ID), (*model.Event).HistoryBoundary) {
		t.Fatal("recorded a handoff before exit")
	}
	r.restart()
	evs := r.events(conv.ID)
	if last := evs[len(evs)-1]; last.Status != model.StatusUnsupervised || !last.Gap {
		t.Fatalf("last event = %+v", last)
	}
	r.restart()
	if got := r.events(conv.ID); len(got) != len(evs) {
		t.Fatal("a further restart added another gap")
	}
	rt.Exit(nil)
}

func TestPiRecordsBothLiveRepliesWithoutFileEntryIDs(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not on PATH")
	}
	binary, err := filepath.Abs("../harness/pi/fake/pi")
	if err != nil {
		t.Fatal(err)
	}
	svc := newService(t, filepath.Join(t.TempDir(), "registry.yaml"), allTrusted, pi.New(pi.Config{Binary: binary}))
	resolver := svc.resolver.(*environ.Resolver)
	resolver.Base = append(resolver.Base, "PATH="+filepath.Dir(node)+":/usr/bin:/bin")
	project, err := svc.RegisterProject("pi fixture", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	conv, err := svc.StartConversation(project.ID, model.HarnessPi, "pi fixture", "twins", LaunchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "both replies to end their turn", func() bool { return !svc.view(conv.ID).TurnActive })
	evs, err := svc.Events(conv.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var replies []string
	var result string
	for _, ev := range evs {
		switch ev.Kind {
		case model.EventAssistant:
			replies = append(replies, ev.Text)
			if ev.NativeID != "" {
				t.Errorf("invented entry id %q", ev.NativeID)
			}
		case model.EventTurnEnd:
			result = ev.Status
		}
	}
	if !slices.Equal(replies, []string{"first", "second"}) || result != model.TurnSuccess {
		t.Fatalf("replies = %v, turn = %q", replies, result)
	}
	if err := svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "pi to exit", func() bool { return svc.view(conv.ID).Runtime == model.RuntimeNone })
}

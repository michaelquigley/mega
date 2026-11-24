package service

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/registry"
)

func adoptionRig(t *testing.T) *rig {
	t.Helper()
	r := newRig(t)
	r.fake.SetSessions([]harness.Session{{NativeID: "terminal", Preview: "terminal question"}})
	r.fake.SetHistory("terminal", []*model.Event{
		{Kind: model.EventUser, Text: "terminal question", NativeID: "term-user"},
		{Kind: model.EventAssistant, Text: "terminal reply", NativeID: "term-reply"},
		{Kind: model.EventNative, Text: "meta: hand-back", NativeID: "term-meta"},
	})
	return r
}

func (r *rig) adoptTerminal() (ConversationView, error) {
	return r.svc.Adopt(r.project.ID, model.HarnessClaude, "terminal", "")
}

func (r *rig) terminalIsDiscoverable() {
	r.t.Helper()
	sessions, err := r.svc.Discover(r.project.ID, model.HarnessClaude)
	if err != nil || len(sessions) != 1 || sessions[0].NativeID != "terminal" {
		r.t.Fatalf("discoverable sessions = %+v, %v", sessions, err)
	}
}

func (r *rig) completeSnapshot(id string) {
	r.t.Helper()
	var ids []string
	for _, ev := range r.events(id) {
		if ev.Origin == model.OriginNative {
			ids = append(ids, ev.NativeID)
		}
	}
	if !slices.Equal(ids, []string{"term-user", "term-reply", "term-meta"}) {
		r.t.Fatalf("snapshot ids = %v", ids)
	}
}

func TestAdoptionReadCanBeRetriedAndTheSnapshotSuppressesLiveEchoes(t *testing.T) {
	r := adoptionRig(t)
	r.fake.FailHistory(errors.New("transcript unavailable"))
	if _, err := r.adoptTerminal(); err == nil {
		t.Fatal("adoption succeeded without a snapshot")
	}
	if len(workspaceOf(t, r.svc).Conversations) != 0 {
		t.Fatal("an incomplete adoption was published")
	}
	r.terminalIsDiscoverable()
	r.fake.FailHistory(nil)
	sub := r.svc.Subscribe()
	defer sub.Close()
	conv, err := r.adoptTerminal()
	if err != nil {
		t.Fatal(err)
	}
	r.completeSnapshot(conv.ID)
	if conv.ReadSeq != conv.LastSeq || conv.Attention != model.AttentionIdle || len(workspaceOf(t, r.svc).Conversations) != 1 {
		t.Fatalf("adopted conversation = %+v", conv)
	}
	select {
	case update := <-sub.Updates():
		if !update.WorkspaceChanged || update.Event != nil {
			t.Fatalf("adoption published %+v", update)
		}
	default:
		t.Fatal("adoption did not announce the workspace change")
	}
	if len(sub.Updates()) != 0 {
		t.Fatal("adoption announced more than one update")
	}
	if err := r.svc.Start(conv.ID); err != nil {
		t.Fatal(err)
	}
	rt := r.fake.Last()
	started(rt)
	rt.Emit(&model.Event{Kind: model.EventAssistant, Text: "terminal reply", NativeID: "term-reply"})
	if view := r.view(conv.ID); view.TurnActive || view.Attention != model.AttentionIdle {
		t.Fatalf("snapshot echo changed the runtime or attention: %+v", view)
	}
	if err := r.svc.Prompt(conv.ID, "managed follow-up"); err != nil {
		t.Fatal(err)
	}
	finishTurn(rt, "managed-reply", "managed reply")
	repeated := 0
	for _, ev := range r.events(conv.ID) {
		if ev.NativeID == "term-reply" {
			repeated++
		}
	}
	if repeated != 1 || !slices.Equal(r.fake.Reads(), []string{"terminal", "terminal"}) {
		t.Fatalf("snapshot reply appeared %d times, reads = %v", repeated, r.fake.Reads())
	}
}

func TestAdoptionRechecksTheOriginalProjectAfterReading(t *testing.T) {
	r := adoptionRig(t)
	reading, release := make(chan struct{}), make(chan struct{})
	r.fake.OnHistory(func() {
		close(reading)
		<-release
	})
	done := make(chan error, 1)
	go func() {
		_, err := r.adoptTerminal()
		done <- err
	}()
	<-reading
	err := r.svc.RemoveProject(r.project.ID)
	close(release)
	adoptErr := <-done
	r.fake.OnHistory(nil)
	if err != nil {
		t.Fatal(err)
	}
	wantCode(t, adoptErr, CodeNotFound)
	project, err := r.svc.RegisterProject("registered again", r.project.Checkout)
	if err != nil {
		t.Fatal(err)
	}
	r.project = project
	r.terminalIsDiscoverable()
	if len(workspaceOf(t, r.svc).Conversations) != 0 {
		t.Fatal("adoption published into a removed project")
	}
}

func TestAdoptionInterruptedDuringSnapshotWriteCanBeRetriedAfterRestart(t *testing.T) {
	r := adoptionRig(t)
	r.svc.failRecord = func(ev *model.Event) error {
		if ev.NativeID == "term-reply" {
			return errors.New("snapshot interrupted")
		}
		return nil
	}
	reasons := exits(r.svc)
	go r.adoptTerminal()
	exited(t, reasons)
	r.crashed()
	if len(workspaceOf(t, r.svc).Conversations) != 0 {
		t.Fatal("partial snapshot was published")
	}
	r.terminalIsDiscoverable()
	conv, err := r.adoptTerminal()
	if err != nil {
		t.Fatal(err)
	}
	r.completeSnapshot(conv.ID)
	if len(workspaceOf(t, r.svc).Conversations) != 1 {
		t.Fatal("more than one conversation visible")
	}
}

func TestAnAdoptionWhoseRegistryRenameSurvivesHasItsCompleteSnapshot(t *testing.T) {
	r := adoptionRig(t)
	var id string
	r.svc.failUpdate = func() error {
		logs, err := os.ReadDir(filepath.Join(filepath.Dir(r.path), "conversations"))
		if err != nil || len(logs) != 1 {
			t.Fatalf("registry was saved before the snapshot: %v %v", logs, err)
		}
		id = logs[0].Name()
		if _, err := os.Stat(r.svc.logPath(id)); err != nil {
			t.Fatal(err)
		}
		reg, err := registry.Load(r.path)
		if err != nil {
			t.Fatal(err)
		}
		// Model the post-rename failure: the on-disk binding survives, and
		// the write fails.
		if err := reg.Update(func(doc *registry.Document) error {
			doc.Conversations = append(doc.Conversations, model.Conversation{
				ID: id, ProjectID: r.project.ID, NativeID: "terminal", Harness: model.HarnessClaude, Runtime: model.RuntimeNone, ReadSeq: 3,
			})
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return errors.New("the directory sync after the rename failed")
	}
	reasons := exits(r.svc)
	go r.adoptTerminal()
	exited(t, reasons)
	r.crashed()
	r.completeSnapshot(id)
	if conv := r.view(id); conv.NativeID != "terminal" || conv.ReadSeq != conv.LastSeq {
		t.Fatalf("recovered adoption = %+v", conv)
	}
}

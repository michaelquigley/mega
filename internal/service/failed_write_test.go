package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/harness/harnesstest"
	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/registry"
	"github.com/michaelquigley/mega/internal/snapshot"
)

// a-failed-write-stops-mega: the first write to the record that fails ends
// the process, as a crash would leave things, and the next start is the start
// after a crash.

// sentNothing fails the test if the runtime was asked anything on the way
// out: no fence, no interrupt, no close.
func sentNothing(t *testing.T, rt *harnesstest.Runtime) {
	t.Helper()
	if rt.Fences() != 0 || rt.Interrupts() != 0 || rt.Closed() {
		t.Errorf("the runtime was sent a stop: fences %d, interrupts %d, closed %v", rt.Fences(), rt.Interrupts(), rt.Closed())
	}
}

// an idle conversation with a runtime, and its runtime.
func (r *rig) atRest(prompt string) (ConversationView, *harnesstest.Runtime) {
	r.t.Helper()
	conv := r.start(prompt)
	rt := r.fake.Last()
	started(rt)
	finishTurn(rt, "a-"+conv.ID, "done")
	return conv, rt
}

// a failed write to the registry, injected or real, and to a log, injected
// or real, each end the process where they are met, and nothing stops a
// runtime on the way out. a failed sync is a failed write to the service: the
// stores return both alike (TestAFailedDirectorySyncIsReturned,
// TestAFailedSyncIsReturnedAndNotConfirmed).
func TestAFailedWriteEndsTheProcessWithoutTheOrderlyStop(t *testing.T) {
	for _, tc := range []struct {
		name  string
		fail  func(r *rig, conv ConversationView)
		write func(r *rig, conv ConversationView, rt *harnesstest.Runtime)
	}{
		{"an injected registry write", func(r *rig, _ ConversationView) {
			r.svc.failUpdate = func() error { return errors.New("injected registry failure") }
		}, func(r *rig, conv ConversationView, _ *harnesstest.Runtime) {
			r.svc.UpdateConversation(conv.ID, "renamed", "")
		}},
		{"a real registry write", func(r *rig, _ ConversationView) {
			dir := filepath.Dir(r.path)
			if err := os.Chmod(dir, 0o500); err != nil {
				r.t.Fatal(err)
			}
			r.t.Cleanup(func() { os.Chmod(dir, 0o700) })
		}, func(r *rig, conv ConversationView, _ *harnesstest.Runtime) {
			r.svc.UpdateConversation(conv.ID, "renamed", "")
		}},
		{"an injected log write", func(r *rig, _ ConversationView) {
			r.svc.failRecord = func(*model.Event) error { return errors.New("injected log failure") }
		}, func(_ *rig, _ ConversationView, rt *harnesstest.Runtime) {
			rt.Emit(&model.Event{Kind: model.EventAssistant, Text: "late", NativeID: "late-1", AfterTurn: true})
		}},
		{"a real log write", func(r *rig, conv ConversationView) {
			r.closeLog(conv.ID)
		}, func(_ *rig, _ ConversationView, rt *harnesstest.Runtime) {
			rt.Emit(&model.Event{Kind: model.EventAssistant, Text: "late", NativeID: "late-1", AfterTurn: true})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			conv, rt := r.atRest("one")
			_, otherRT := r.atRest("two")
			reasons := exits(r.svc)
			tc.fail(r, conv)
			go tc.write(r, conv, rt)
			if reason := exited(t, reasons); reason == nil {
				t.Error("the exit gave no reason")
			}
			sentNothing(t, rt)
			sentNothing(t, otherRT)
			if rt.Exited() || otherRT.Exited() {
				t.Error("a runtime was let go")
			}
		})
	}
}

// the next start is the start after a crash: a conversation that was running
// is unsupervised, and a request that was pending is lost.
func TestTheStartAfterAFailedWriteSettlesWhatItLeft(t *testing.T) {
	r := newRig(t)
	conv, rt := r.atRest("one")
	asked := rt.Ask(model.RequestPermission, map[string]any{"tool": "Bash", "input": map[string]any{"command": "echo"}})
	r.closeLog(conv.ID)
	reasons := exits(r.svc)
	go rt.Settle(asked, model.OutcomeAborted)
	exited(t, reasons)
	sentNothing(t, rt)

	r.crashed()
	view := r.view(conv.ID)
	if view.Runtime != model.RuntimeUnsupervised || view.Attention != model.AttentionUnsupervised {
		t.Errorf("after the next start: runtime %q, attention %q", view.Runtime, view.Attention)
	}
	var outcomes []string
	for _, ev := range r.events(conv.ID) {
		if ev.Kind == model.EventRequestResolved && ev.Req.ID == asked {
			outcomes = append(outcomes, ev.Req.Outcome)
		}
	}
	if len(outcomes) != 1 || outcomes[0] != model.OutcomeLost {
		t.Errorf("the pending request ended %v, want lost", outcomes)
	}
	if len(view.Pending) != 0 {
		t.Errorf("still pending: %+v", view.Pending)
	}
}

// a snapshot of the configuration is best effort: one that cannot be written
// stops nothing.
func TestAFailedSnapshotStopsNothing(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	config := t.TempDir()
	if err := os.WriteFile(filepath.Join(config, "settings.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	fake := harnesstest.New(model.HarnessClaude)
	fake.SetVersion("2.1.326", nil)
	fake.SetConfiguration(func([]string) []harness.ConfigRoot {
		return []harness.ConfigRoot{{Harness: model.HarnessClaude, Label: "config", Dir: config, Include: []string{"settings.json"}}}
	})
	reg, err := registry.Load(filepath.Join(dir, "registry.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := New(reg, Options{
		IsUntrusted:  allTrusted,
		EventLogPath: func(id string) string { return filepath.Join(dir, "conversations", id, "events.jsonl") },
		Harnesses:    []harness.Harness{fake},
		Resolver:     testResolver(t),
		Snapshots:    snapshot.NewStore(snapshot.Options{Dir: filepath.Join(blocked, "snapshots"), Keep: 5, KeepAtVersion: 5}),
		Exit:         func(reason error) { t.Errorf("a failed snapshot ended the process: %v", reason) },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	svc.background.Wait()
	project, err := svc.RegisterProject("fixture", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc.background.Wait()
	if _, err := svc.StartConversation(project.ID, model.HarnessClaude, "", "hello", LaunchChoice{}); err != nil {
		t.Fatal(err)
	}
	svc.background.Wait()
	if list, err := svc.snapshots.List(); err == nil && len(list) != 0 {
		t.Fatalf("a snapshot was written: %+v", list)
	}
}

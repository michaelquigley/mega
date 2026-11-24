package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/harness/harnesstest"
	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/registry"
	"github.com/michaelquigley/mega/internal/snapshot"
)

// the service snapshots the configuration in the background after the
// start-up check, and again when a launch's version read meets a version no
// snapshot has recorded; a start with nothing changed takes none. the
// configuration is made up, in a scratch directory.
func TestTheServiceSnapshotsTheConfiguration(t *testing.T) {
	config := t.TempDir()
	if err := os.WriteFile(filepath.Join(config, "settings.json"), []byte(`{"effortLevel":"medium"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	fake := harnesstest.New(model.HarnessClaude)
	fake.SetVersion("2.1.325", nil)
	var seenEnv []string
	fake.SetConfiguration(func(env []string) []harness.ConfigRoot {
		seenEnv = env
		return []harness.ConfigRoot{{Harness: model.HarnessClaude, Label: "config", Dir: config, Include: []string{"settings.json"}}}
	})
	dir := t.TempDir()
	store := snapshot.NewStore(snapshot.Options{Dir: filepath.Join(dir, "snapshots"), Keep: 5, KeepAtVersion: 5})
	start := func() *Service {
		reg, err := registry.Load(filepath.Join(dir, "registry.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		svc, err := New(reg, Options{
			IsUntrusted:  allTrusted,
			EventLogPath: func(id string) string { return filepath.Join(dir, "conversations", id, "events.jsonl") },
			Harnesses:    []harness.Harness{fake},
			Resolver:     testResolver(t),
			Snapshots:    store,
			Exit:         func(reason error) { panic(reason) },
		})
		if err != nil {
			t.Fatal(err)
		}
		svc.background.Wait()
		return svc
	}
	svc := start()
	list, err := store.List()
	if err != nil || len(list) != 1 || list[0].Reason != snapshot.ReasonStartUp || list[0].Versions[model.HarnessClaude] != "2.1.325" {
		t.Fatalf("after start-up: %+v %v", list, err)
	}
	if seenEnv == nil {
		t.Error("the configuration was not looked for in the start-up check's environment")
	}
	if sum := svc.Snapshots(); sum == nil || sum.Count != 1 || sum.Newest.Name != list[0].Name || sum.Dir != store.Dir() {
		t.Errorf("summary = %+v", sum)
	}

	// a launch whose version read meets a new version takes one.
	project, err := svc.RegisterProject("fixture", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc.background.Wait()
	fake.SetVersion("2.1.326", nil)
	if _, err := svc.StartConversation(project.ID, model.HarnessClaude, "", "hello", LaunchChoice{}); err != nil {
		t.Fatal(err)
	}
	svc.background.Wait()
	list, _ = store.List()
	if len(list) != 2 || list[0].Reason != snapshot.ReasonVersion || list[0].Versions[model.HarnessClaude] != "2.1.326" {
		t.Fatalf("after a new version: %+v", list)
	}
	// a second launch on the same version takes none.
	if _, err := svc.StartConversation(project.ID, model.HarnessClaude, "", "again", LaunchChoice{}); err != nil {
		t.Fatal(err)
	}
	svc.background.Wait()
	if list, _ = store.List(); len(list) != 2 {
		t.Errorf("a version already recorded took another: %d", len(list))
	}
	svc.Close()

	// a start with nothing changed takes none.
	svc = start()
	defer svc.Close()
	if list, _ = store.List(); len(list) != 2 {
		t.Errorf("an unchanged start took one: %d", len(list))
	}
}

// with no store, the service takes no snapshot and reports none.
func TestNoStoreTakesNone(t *testing.T) {
	svc := tempService(t)
	if svc.Snapshots() != nil {
		t.Error("a service with no store reports snapshots")
	}
}

package registry

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michaelquigley/mega/internal/durable"
	"github.com/michaelquigley/mega/internal/model"
)

func TestMissingFileIsEmptyAndNotCreated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "registry.yaml")
	reg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if doc := reg.Snapshot(); len(doc.Projects) != 0 || len(doc.Conversations) != 0 {
		t.Fatalf("expected empty, got %+v", doc)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("load created the file: %v", err)
	}
}

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "registry.yaml")
	reg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	project := model.Project{ID: "p1", Name: "fixture", Checkout: "/tmp/fixture"}
	conversation := model.Conversation{
		ID: "c1", ProjectID: "p1", Name: "planning", Label: "plan", Harness: model.HarnessClaude,
		NativeID: "native-1", Mode: model.ModeLongLived, Runtime: model.RuntimeRunning, ReadSeq: 3, OutputSeq: 7,
	}
	err = reg.Update(func(d *Document) error {
		d.Projects = append(d.Projects, project)
		d.Conversations = append(d.Conversations, conversation)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	again, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	doc := again.Snapshot()
	if len(doc.Projects) != 1 || doc.Projects[0] != project {
		t.Errorf("projects = %+v", doc.Projects)
	}
	if len(doc.Conversations) != 1 || doc.Conversations[0] != conversation {
		t.Errorf("conversations = %+v", doc.Conversations)
	}
}

func TestLastSeqIsNotPersisted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.yaml")
	reg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	err = reg.Update(func(d *Document) error {
		d.Conversations = append(d.Conversations, model.Conversation{ID: "c1", LastSeq: 9})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "last_seq") {
		t.Errorf("last_seq reached the registry:\n%s", body)
	}
}

func TestComputedTrustNoticeIsNotPersisted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.yaml")
	reg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	err = reg.Update(func(d *Document) error {
		d.Projects = append(d.Projects, model.Project{ID: "p1", Name: "n", Checkout: "/c", Untrusted: true})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "untrusted") {
		t.Errorf("computed field reached the file:\n%s", body)
	}
}

func TestFailedUpdateLeavesStateAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.yaml")
	reg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	err = reg.Update(func(d *Document) error {
		d.Projects = append(d.Projects, model.Project{ID: "p1"})
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if len(reg.Snapshot().Projects) != 0 {
		t.Error("memory advanced past a failed update")
	}
}

func TestWriteFailureLeavesMemoryAndFileAlone(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	path := filepath.Join(dir, "registry.yaml")
	reg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	add := func(id string) error {
		return reg.Update(func(d *Document) error {
			d.Projects = append(d.Projects, model.Project{ID: id, Name: id, Checkout: "/c"})
			return nil
		})
	}
	if err := add("p1"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// a read-only directory makes the write fail.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })
	if err := add("p2"); err == nil {
		t.Skip("directory permissions are not enforced here")
	}
	if got := reg.Snapshot().Projects; len(got) != 1 {
		t.Errorf("memory advanced past a failed write: %+v", got)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("file changed on a failed write")
	}

	// a failure before the rename changed nothing, so a retry is honest.
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := add("p2"); err != nil {
		t.Fatalf("retry after a failure before the rename was refused: %v", err)
	}
}

func TestUpdateLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	reg, err := Load(filepath.Join(dir, "registry.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := reg.Update(func(*Document) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "registry.yaml" {
		t.Errorf("directory holds %v", entries)
	}
}

func TestSnapshotIsACopy(t *testing.T) {
	reg, err := Load(filepath.Join(t.TempDir(), "registry.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	err = reg.Update(func(d *Document) error {
		d.Projects = append(d.Projects, model.Project{ID: "p1", Name: "a"})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	reg.Snapshot().Projects[0].Name = "changed"
	if reg.Snapshot().Projects[0].Name != "a" {
		t.Error("snapshot aliases registry state")
	}
}

func TestRefusesUnknownVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.yaml")
	if err := os.WriteFile(path, []byte("version: 99\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected an error for an unknown version")
	}
}

func TestNewIDIsDirectorySafeAndDistinct(t *testing.T) {
	a, b := NewID(), NewID()
	if a == b {
		t.Fatal("ids collided")
	}
	if strings.ContainsAny(a, "/. ") || len(a) != 16 {
		t.Errorf("id = %q", a)
	}
}

func TestFailedDirectorySyncAfterRenameIsSticky(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.yaml")
	reg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	addProject := func(id string) error {
		return reg.Update(func(d *Document) error {
			d.Projects = append(d.Projects, model.Project{ID: id, Name: id, Checkout: "/tmp/" + id})
			return nil
		})
	}

	ioErr := errors.New("input/output error")
	syncDir = func(string) error { return ioErr }
	t.Cleanup(func() { syncDir = durable.SyncDir })

	err = addProject("p1")
	if !errors.Is(err, ErrUncertain) || !errors.Is(err, ioErr) {
		t.Fatalf("expected ErrUncertain wrapping the sync error, got %v", err)
	}
	// the rename landed but was never confirmed, so memory keeps the last
	// confirmed document while the file may already hold the change.
	if doc := reg.Snapshot(); len(doc.Projects) != 0 {
		t.Fatalf("memory advanced past an unconfirmed write: %+v", doc)
	}
	onDisk, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(onDisk.Snapshot().Projects) != 1 {
		t.Fatalf("file does not hold the renamed document")
	}

	// a working sync does not lift the refusal; only a restart does.
	syncDir = durable.SyncDir
	if err := addProject("p2"); !errors.Is(err, ErrUncertain) {
		t.Fatalf("later update was not refused: %v", err)
	}
	if doc := reg.Snapshot(); len(doc.Projects) != 0 {
		t.Errorf("refused update changed memory: %+v", doc)
	}
	again, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Snapshot().Projects) != 1 {
		t.Errorf("refused update changed the file")
	}
}

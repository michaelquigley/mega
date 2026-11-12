package service

import (
	"github.com/michaelquigley/mega/internal/environ"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/registry"
)

func allTrusted(string) bool { return false }

func newService(t *testing.T, registryPath string, checker UntrustedChecker, harnesses ...harness.Harness) *Service {
	t.Helper()
	reg, err := registry.Load(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	logs := filepath.Join(filepath.Dir(registryPath), "conversations")
	svc, err := New(reg, Options{
		IsUntrusted:  checker,
		EventLogPath: func(id string) string { return filepath.Join(logs, id, "events.jsonl") },
		Harnesses:    harnesses,
		Resolver:     testResolver(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Close)
	return svc
}

func tempService(t *testing.T) *Service {
	t.Helper()
	return newService(t, filepath.Join(t.TempDir(), "registry.yaml"), allTrusted)
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	if got := ErrorCode(err); got != code {
		t.Errorf("code = %q, want %q (err = %v)", got, code, err)
	}
}

func TestRegisterValidatesCheckout(t *testing.T) {
	svc := tempService(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "a-file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, checkout := range []string{"", "   ", "relative/path", filepath.Join(dir, "absent"), file} {
		_, err := svc.RegisterProject("x", checkout)
		wantCode(t, err, CodeInvalid)
	}
	if got := svc.Workspace().Projects; len(got) != 0 {
		t.Errorf("a rejected registration landed: %+v", got)
	}
}

func TestRegisterDefaultsNameAndCleansPath(t *testing.T) {
	svc := tempService(t)
	dir := filepath.Join(t.TempDir(), "my-checkout")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	project, err := svc.RegisterProject(" ", dir+"/./")
	if err != nil {
		t.Fatal(err)
	}
	if project.Name != "my-checkout" || project.Checkout != dir || project.ID == "" {
		t.Errorf("project = %+v", project)
	}
}

func TestDuplicateCheckoutConflicts(t *testing.T) {
	svc := tempService(t)
	dir := t.TempDir()
	if _, err := svc.RegisterProject("first", dir); err != nil {
		t.Fatal(err)
	}
	_, err := svc.RegisterProject("second", dir+"/")
	wantCode(t, err, CodeConflict)
	if got := svc.Workspace().Projects; len(got) != 1 || got[0].Name != "first" {
		t.Errorf("projects = %+v", got)
	}
}

func TestRegistrationSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.yaml")
	dir := t.TempDir()
	first := newService(t, path, allTrusted)
	project, err := first.RegisterProject("fixture", dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.RenameProject(project.ID, "renamed"); err != nil {
		t.Fatal(err)
	}

	second := newService(t, path, allTrusted)
	got := second.Workspace().Projects
	if len(got) != 1 || got[0].ID != project.ID || got[0].Name != "renamed" || got[0].Checkout != dir {
		t.Errorf("projects after restart = %+v", got)
	}
}

func TestRenameRules(t *testing.T) {
	svc := tempService(t)
	project, err := svc.RegisterProject("a", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.RenameProject(project.ID, "  ")
	wantCode(t, err, CodeInvalid)
	_, err = svc.RenameProject("absent", "b")
	wantCode(t, err, CodeNotFound)
	renamed, err := svc.RenameProject(project.ID, " b ")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "b" {
		t.Errorf("name = %q", renamed.Name)
	}
}

func TestRemoveLeavesCheckoutAndOtherProjectsAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.yaml")
	reg, err := registry.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := New(reg, Options{
		EventLogPath: func(id string) string { return filepath.Join(filepath.Dir(path), "conversations", id, "events.jsonl") },
		Resolver:     testResolver(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Close)
	keepDir, dropDir := t.TempDir(), t.TempDir()
	marker := filepath.Join(dropDir, "keep-me")
	if err := os.WriteFile(marker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	keep, err := svc.RegisterProject("keep", keepDir)
	if err != nil {
		t.Fatal(err)
	}
	drop, err := svc.RegisterProject("drop", dropDir)
	if err != nil {
		t.Fatal(err)
	}
	err = reg.Update(func(d *registry.Document) error {
		d.Conversations = append(d.Conversations,
			model.Conversation{ID: "c-keep", ProjectID: keep.ID},
			model.Conversation{ID: "c-drop", ProjectID: drop.ID})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.RemoveProject(drop.ID); err != nil {
		t.Fatal(err)
	}
	wantCode(t, svc.RemoveProject(drop.ID), CodeNotFound)

	ws := svc.Workspace()
	if len(ws.Projects) != 1 || ws.Projects[0].ID != keep.ID {
		t.Errorf("projects = %+v", ws.Projects)
	}
	if len(ws.Conversations) != 1 || ws.Conversations[0].ID != "c-keep" {
		t.Errorf("conversations = %+v", ws.Conversations)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("checkout was touched: %v", err)
	}
}

func TestTrustNoticeIsRecomputedOnRead(t *testing.T) {
	untrusted := true
	svc := newService(t, filepath.Join(t.TempDir(), "registry.yaml"), func(string) bool { return untrusted })
	project, err := svc.RegisterProject("a", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !project.Untrusted {
		t.Error("registration did not carry the notice")
	}
	if !svc.Workspace().Projects[0].Untrusted {
		t.Error("workspace did not carry the notice")
	}
	untrusted = false
	if svc.Workspace().Projects[0].Untrusted {
		t.Error("notice outlived the terminal visit")
	}
}

func TestSubscribersSeeConfirmedChangesOnly(t *testing.T) {
	svc := tempService(t)
	sub := svc.Subscribe()
	defer sub.Close()

	if _, err := svc.RegisterProject("bad", "relative"); err == nil {
		t.Fatal("expected a validation error")
	}
	select {
	case update := <-sub.Updates():
		t.Fatalf("rejected write published %+v", update)
	default:
	}

	if _, err := svc.RegisterProject("good", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	select {
	case update := <-sub.Updates():
		if !update.WorkspaceChanged {
			t.Errorf("update = %+v", update)
		}
	default:
		t.Fatal("no update for a confirmed change")
	}
}

func TestSlowSubscriberIsDroppedWithoutBlockingOthers(t *testing.T) {
	svc := tempService(t)
	slow := svc.Subscribe()
	defer slow.Close()

	project, err := svc.RegisterProject("a", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for range updateBuffer + 1 {
		if _, err := svc.RenameProject(project.ID, "b"); err != nil {
			t.Fatal(err)
		}
	}

	drained := 0
	for range slow.Updates() {
		drained++
	}
	if drained != updateBuffer {
		t.Errorf("drained %d, want %d", drained, updateBuffer)
	}

	fresh := svc.Subscribe()
	defer fresh.Close()
	if _, err := svc.RenameProject(project.ID, "c"); err != nil {
		t.Fatal(err)
	}
	if update := <-fresh.Updates(); !update.WorkspaceChanged {
		t.Errorf("update = %+v", update)
	}
}

func TestCloseIsSafeInEveryOrder(t *testing.T) {
	svc := tempService(t)
	sub := svc.Subscribe()
	sub.Close()
	sub.Close()

	open := svc.Subscribe()
	svc.Close()
	if _, ok := <-open.Updates(); ok {
		t.Error("channel still open after service close")
	}
	open.Close()

	late := svc.Subscribe()
	if _, ok := <-late.Updates(); ok {
		t.Error("subscribing after close delivered a live channel")
	}
	late.Close()
}

// testResolver resolves through bash from a base with a temporary HOME, so no
// start-up file of the operator's is read and nothing of the test's own
// environment but a system PATH reaches what it resolves.
func testResolver(t *testing.T) *environ.Resolver {
	t.Helper()
	shell := "/bin/bash"
	if _, err := os.Stat(shell); err != nil {
		t.Skip("bash is not installed")
	}
	return &environ.Resolver{Shell: shell, Base: []string{"HOME=" + t.TempDir(), "PATH=/usr/local/bin:/usr/bin:/bin"}, Timeout: 5 * time.Second}
}

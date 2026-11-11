package service

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

func (r *rig) projectView() model.Project {
	r.t.Helper()
	for _, p := range r.svc.Workspace().Projects {
		if p.ID == r.project.ID {
			return p
		}
	}
	r.t.Fatal("project gone")
	return model.Project{}
}

func wantMissing(t *testing.T, err error, checkout string) {
	t.Helper()
	wantCode(t, err, CodeCheckoutMissing)
	if err == nil || !strings.Contains(err.Error(), "the checkout is missing: '"+checkout+"'") {
		t.Errorf("message = %v", err)
	}
}

// a checkout moved away is reported on every read and refuses new work, with
// reads, rename, star and removal still working; moved back, it all lifts.
func TestAMissingCheckoutIsReportedAndRefusesWork(t *testing.T) {
	r := newRig(t)
	checkout := r.project.Checkout
	write(t, checkout, "a.txt", "a")
	running := r.start("one")
	rt := r.fake.Last()
	started(rt)
	finishTurn(rt, "a-1", "done")
	stopped := r.start("two")
	stoppedRT := r.fake.Last()
	started(stoppedRT)
	finishTurn(stoppedRT, "b-1", "done")
	if err := r.svc.Release(stopped.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the release", stoppedRT.Exited)
	r.fake.SetSessions([]harness.Session{{NativeID: "terminal-1", Preview: "from a terminal"}})
	if p := r.projectView(); p.CheckoutMissing != "" {
		t.Fatalf("present checkout reported missing: %q", p.CheckoutMissing)
	}

	moved := checkout + ".moved"
	if err := os.Rename(checkout, moved); err != nil {
		t.Fatal(err)
	}
	if p := r.projectView(); p.CheckoutMissing != CheckoutDoesNotExist {
		t.Fatalf("moved away: %q", p.CheckoutMissing)
	}
	launched := len(r.fake.Runtimes())
	sub := r.svc.Subscribe()
	defer sub.Close()
	_, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "", "new", LaunchChoice{})
	wantMissing(t, err, checkout)
	_, err = r.svc.Adopt(r.project.ID, model.HarnessClaude, "terminal-1", "")
	wantMissing(t, err, checkout)
	wantMissing(t, r.svc.Prompt(running.ID, "more"), checkout)
	wantMissing(t, r.svc.Prompt(stopped.ID, "more"), checkout)
	wantMissing(t, r.svc.Start(stopped.ID), checkout)
	// the refusal announces a change, so pages read the notice at once.
	expectChanged(t, sub)
	if len(r.fake.Runtimes()) != launched || len(rt.Prompts()) != 1 {
		t.Error("work reached a harness in a missing checkout")
	}
	if paths, err := r.svc.Paths(r.project.ID, ""); err != nil || len(paths) != 0 {
		t.Errorf("paths: %v %v", paths, err)
	}
	if sessions, err := r.svc.Discover(r.project.ID, model.HarnessClaude); err != nil || sessions == nil || len(sessions) != 0 {
		t.Errorf("discover: %v %v", sessions, err)
	}
	// reads, rename and star still work.
	if _, err := r.svc.Events(running.ID, 0); err != nil {
		t.Errorf("read: %v", err)
	}
	if _, err := r.svc.RenameProject(r.project.ID, "renamed"); err != nil {
		t.Errorf("rename: %v", err)
	}
	starred := true
	if _, err := r.svc.UpdateProject(r.project.ID, ProjectChange{Starred: &starred}); err != nil {
		t.Errorf("star: %v", err)
	}

	// a file in its place: not a directory.
	if err := os.WriteFile(checkout, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if p := r.projectView(); p.CheckoutMissing != CheckoutNotDirectory {
		t.Errorf("a file in its place: %q", p.CheckoutMissing)
	}
	if err := os.Remove(checkout); err != nil {
		t.Fatal(err)
	}

	// moved back: the flag and the refusals lift.
	if err := os.Rename(moved, checkout); err != nil {
		t.Fatal(err)
	}
	if p := r.projectView(); p.CheckoutMissing != "" {
		t.Fatalf("moved back: %q", p.CheckoutMissing)
	}
	if err := r.svc.Prompt(running.ID, "more"); err != nil {
		t.Errorf("prompt once back: %v", err)
	}
	if paths, err := r.svc.Paths(r.project.ID, "a"); err != nil || !slices.Contains(paths, "a.txt") {
		t.Errorf("paths once back: %v %v", paths, err)
	}
}

// a checkout the service's user cannot read is reported as not readable, and
// a project whose checkout is missing can still be removed.
func TestAnUnreadableCheckoutIsReported(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads every directory")
	}
	r := newRig(t)
	checkout := r.project.Checkout
	if err := os.Chmod(checkout, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(checkout, 0o755) })
	if p := r.projectView(); p.CheckoutMissing != CheckoutNotReadable {
		t.Errorf("unreadable: %q", p.CheckoutMissing)
	}
	_, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "", "new", LaunchChoice{})
	wantMissing(t, err, checkout)

	gone := filepath.Join(t.TempDir(), "gone")
	other, err := r.svc.RegisterProject("other", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(other.Checkout, gone); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.RemoveProject(other.ID); err != nil {
		t.Errorf("removing a project whose checkout is missing: %v", err)
	}
}

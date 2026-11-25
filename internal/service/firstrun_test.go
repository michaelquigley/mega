package service

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/michaelquigley/mega/internal/environ"
	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/harness/harnesstest"
	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/registry"
)

// countingResolver resolves with the test resolver, counting the
// resolutions of each directory.
type countingResolver struct {
	inner Resolver

	mu    sync.Mutex
	calls map[string]int
}

func (c *countingResolver) Resolve(ctx context.Context, dir, activate string) (*environ.Resolution, error) {
	c.mu.Lock()
	c.calls[dir]++
	c.mu.Unlock()
	return c.inner.Resolve(ctx, dir, activate)
}

func (c *countingResolver) count(dir string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls[dir]
}

func countingService(t *testing.T, path string, resolver *countingResolver, harnesses ...harness.Harness) *Service {
	t.Helper()
	reg, err := registry.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	logs := filepath.Join(filepath.Dir(path), "conversations")
	svc, err := New(reg, Options{
		IsUntrusted:  allTrusted,
		EventLogPath: func(id string) string { return filepath.Join(logs, id, "events.jsonl") },
		Harnesses:    harnesses,
		Resolver:     resolver,
		Exit:         func(reason error) { panic(reason) },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Close)
	return svc
}

// nothing resolves a project's environment in the background: not a
// registration, not reading the workspace however often, not a change of the
// activation that would run, and not a start of mega over approved projects.
// the first thing that needs the environment resolves it, once, and the
// project's state then shows the result. an unapproved file shows its refusal
// with nothing run.
func TestNothingResolvesAProjectUntilSomethingNeedsIt(t *testing.T) {
	resolver := &countingResolver{inner: testResolver(t), calls: map[string]int{}}
	fake := harnesstest.New(model.HarnessClaude)
	path := filepath.Join(t.TempDir(), "registry.yaml")
	svc := countingService(t, path, resolver, fake)
	plain, approved, unapproved := t.TempDir(), t.TempDir(), t.TempDir()
	writeEnvFile(t, approved, "export MEGA_APPROVED=1")
	writeEnvFile(t, unapproved, "export MEGA_NEVER=1")
	p, err := svc.RegisterProject("plain", plain)
	if err != nil {
		t.Fatal(err)
	}
	a, err := svc.RegisterProject("approved", approved)
	if err != nil {
		t.Fatal(err)
	}
	u, err := svc.RegisterProject("unapproved", unapproved)
	if err != nil {
		t.Fatal(err)
	}
	// an approval resolves at once, to record the names it changes.
	if _, err := svc.ApproveEnvironment(a.ID, workspaceOf(t, svc).Environments[a.ID].File.Digest); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetEnvironmentOverride(p.ID, &model.EnvOverride{Activate: "export MEGA_OVERRIDE=1"}); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		workspaceOf(t, svc)
	}
	svc.background.Wait()
	counts := func(when string, wantApproved int) {
		t.Helper()
		if n := resolver.count(plain); n != 0 {
			t.Errorf("%s: the untouched project resolved %d times", when, n)
		}
		if n := resolver.count(approved); n != wantApproved {
			t.Errorf("%s: the approved project resolved %d times, want %d", when, n, wantApproved)
		}
		if n := resolver.count(unapproved); n != 0 {
			t.Errorf("%s: an unapproved file was resolved %d times", when, n)
		}
	}
	counts("after registering and reading", 1)
	if st := workspaceOf(t, svc).Environments[p.ID]; st.Resolved != nil {
		t.Errorf("the untouched project has a resolution: %+v", st.Resolved)
	}

	svc.Close()
	again := countingService(t, path, resolver, fake)
	for range 5 {
		workspaceOf(t, again)
	}
	again.background.Wait()
	counts("after a restart", 1)
	ws := workspaceOf(t, again)
	for _, id := range []string{p.ID, a.ID, u.ID} {
		if st := ws.Environments[id]; st.Resolved != nil {
			t.Errorf("after a restart, %s has a resolution: %+v", id, st.Resolved)
		}
	}
	if st := ws.Environments[u.ID]; st.Status != EnvUnapproved || st.Refusal == nil || st.Refusal.Code != CodeEnvironmentUnapproved {
		t.Errorf("the unapproved file shows status %q, refusal %v", st.Status, st.Refusal)
	}

	if _, err := again.StartConversation(a.ID, model.HarnessClaude, "", "hello", LaunchChoice{}); err != nil {
		t.Fatal(err)
	}
	counts("after a start", 2)
	if st := workspaceOf(t, again).Environments[a.ID]; st.Resolved == nil || st.Resolved.Error != "" || !slices.Equal(st.Resolved.Changed, []string{"MEGA_APPROVED"}) {
		t.Errorf("after a start the project shows %+v", st.Resolved)
	}
	again.background.Wait()
	counts("after reading the started project", 2)
}

// the start-up check says, per harness, whether it can run in the
// operator's shell environment and why not, and re-checks on every read.
func TestHarnessStatusOnTheWorkspace(t *testing.T) {
	claude := harnesstest.New(model.HarnessClaude)
	codex := harnesstest.New(model.HarnessCodex)
	codex.SetMissing(&harness.MissingError{Harness: model.HarnessCodex, Executable: "codex", Reason: "'codex' was not found on the project's PATH"})
	svc := newService(t, filepath.Join(t.TempDir(), "registry.yaml"), allTrusted, claude, codex)
	got := workspaceOf(t, svc).Harnesses
	if len(got) != 2 || got[0].Name != model.HarnessClaude || !got[0].Found || got[1].Name != model.HarnessCodex || got[1].Found {
		t.Fatalf("harnesses = %+v", got)
	}
	var missing *harness.MissingError
	if !errors.As(codex.Check(nil), &missing) || got[1].Reason != "'codex' was not found on your shell's PATH" {
		t.Errorf("codex reason = %q", got[1].Reason)
	}
	codex.SetMissing(nil)
	if got := workspaceOf(t, svc).Harnesses; !got[1].Found {
		t.Errorf("an install is not seen until a restart: %+v", got[1])
	}
}

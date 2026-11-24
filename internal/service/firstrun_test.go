package service

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/environ"
	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/harness/harnesstest"
	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/registry"
)

// countingResolver resolves with the test resolver, counting the
// resolutions of each directory and the most that ran at once, and holding
// each for hold.
type countingResolver struct {
	inner Resolver
	hold  time.Duration

	mu      sync.Mutex
	calls   map[string]int
	running int
	most    int
}

func (c *countingResolver) Resolve(ctx context.Context, dir, activate string) (*environ.Resolution, error) {
	c.mu.Lock()
	c.calls[dir]++
	c.running++
	c.most = max(c.most, c.running)
	c.mu.Unlock()
	time.Sleep(c.hold)
	defer func() {
		c.mu.Lock()
		c.running--
		c.mu.Unlock()
	}()
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

// registering a project resolves its environment in the background, so what
// each harness is missing there is known before anything is opened; a
// restart resolves it again by itself, and an unapproved file is never run.
func TestRegistrationAndStartUpResolveInTheBackground(t *testing.T) {
	resolver := &countingResolver{inner: testResolver(t), calls: map[string]int{}}
	fake := harnesstest.New(model.HarnessClaude)
	fake.SetMissing(&harness.MissingError{Harness: model.HarnessClaude, Executable: "/x/claude", Reason: "'/x/claude' does not exist"})
	path := filepath.Join(t.TempDir(), "registry.yaml")
	svc := countingService(t, path, resolver, fake)
	plain, unapproved := t.TempDir(), t.TempDir()
	writeEnvFile(t, unapproved, "export MEGA_NEVER=1")
	p, err := svc.RegisterProject("plain", plain)
	if err != nil {
		t.Fatal(err)
	}
	u, err := svc.RegisterProject("unapproved", unapproved)
	if err != nil {
		t.Fatal(err)
	}
	svc.background.Wait()
	svc.mu.Lock()
	project, _ := svc.projectLocked(p.ID)
	st := svc.envStateLocked(project)
	svc.mu.Unlock()
	if st.Resolved == nil || len(st.Missing) != 1 || st.Missing[0].Executable != "/x/claude" {
		t.Fatalf("after registering: resolved %v, missing %+v", st.Resolved, st.Missing)
	}
	if n := resolver.count(plain); n != 1 {
		t.Errorf("the registered project resolved %d times", n)
	}

	svc.Close()
	again := countingService(t, path, resolver, fake)
	again.background.Wait()
	again.mu.Lock()
	project, _ = again.projectLocked(p.ID)
	st = again.envStateLocked(project)
	unapprovedProject, _ := again.projectLocked(u.ID)
	ust := again.envStateLocked(unapprovedProject)
	again.mu.Unlock()
	if st.Resolved == nil || len(st.Missing) != 1 {
		t.Errorf("after a restart: resolved %v, missing %+v", st.Resolved, st.Missing)
	}
	if n := resolver.count(plain); n != 2 {
		t.Errorf("the project resolved %d times across a start-up", n)
	}
	if n := resolver.count(unapproved); n != 0 || ust.Resolved != nil || ust.Status != EnvUnapproved {
		t.Errorf("an unapproved file was resolved %d times (status %q)", n, ust.Status)
	}
}

// many projects resolve at most resolveAtOnce at a time, and the service
// answers meanwhile.
func TestStartUpResolutionIsBounded(t *testing.T) {
	resolver := &countingResolver{inner: testResolver(t), calls: map[string]int{}}
	path := filepath.Join(t.TempDir(), "registry.yaml")
	svc := countingService(t, path, resolver)
	for range 12 {
		if _, err := svc.RegisterProject("", t.TempDir()); err != nil {
			t.Fatal(err)
		}
	}
	svc.background.Wait()
	svc.Close()
	resolver.mu.Lock()
	resolver.most, resolver.hold = 0, 50*time.Millisecond
	resolver.mu.Unlock()
	start := time.Now()
	again := countingService(t, path, resolver)
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("start-up waited %s for the resolutions", took)
	}
	if got := len(again.Workspace().Projects); got != 12 {
		t.Errorf("the workspace read %d projects while resolving", got)
	}
	again.background.Wait()
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	if resolver.most > resolveAtOnce {
		t.Errorf("%d resolutions ran at once", resolver.most)
	}
}

// the start-up check says, per harness, whether it can run in the
// operator's shell environment and why not, and re-checks on every read.
func TestHarnessStatusOnTheWorkspace(t *testing.T) {
	claude := harnesstest.New(model.HarnessClaude)
	codex := harnesstest.New(model.HarnessCodex)
	codex.SetMissing(&harness.MissingError{Harness: model.HarnessCodex, Executable: "codex", Reason: "'codex' was not found on the project's PATH"})
	svc := newService(t, filepath.Join(t.TempDir(), "registry.yaml"), allTrusted, claude, codex)
	got := svc.Workspace().Harnesses
	if len(got) != 2 || got[0].Name != model.HarnessClaude || !got[0].Found || got[1].Name != model.HarnessCodex || got[1].Found {
		t.Fatalf("harnesses = %+v", got)
	}
	var missing *harness.MissingError
	if !errors.As(codex.Check(nil), &missing) || got[1].Reason != "'codex' was not found on your shell's PATH" {
		t.Errorf("codex reason = %q", got[1].Reason)
	}
	codex.SetMissing(nil)
	if got := svc.Workspace().Harnesses; !got[1].Found {
		t.Errorf("an install is not seen until a restart: %+v", got[1])
	}
}

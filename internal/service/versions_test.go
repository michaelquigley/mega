package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/harness/harnesstest"
	"github.com/michaelquigley/mega/internal/model"
)

func startedFacts(t *testing.T, r *rig, id string) *model.RuntimeFacts {
	t.Helper()
	for _, ev := range r.events(id) {
		if ev.Kind == model.EventRuntime && ev.Status == model.StatusStarted {
			return ev.Facts
		}
	}
	t.Fatalf("no started event for %s", id)
	return nil
}

// a runtime's start records where its version stands against the tested
// range, for the history's started line: a tested version and a newer one,
// the normal state, with no sentence; an older and an unreadable one with
// it; nothing is refused for its version.
func TestTheStartRecordsWhereTheVersionStands(t *testing.T) {
	r := newRig(t)
	tested := harness.Tested[model.HarnessClaude]
	for _, c := range []struct{ version, standing, says string }{
		{"2.1.320", harness.StandingTested, ""},
		{"2.1.999", harness.StandingNewer, ""},
		{"2.1.100", harness.StandingOlder, "expects Claude Code '" + tested.Low + "' or later"},
		{"fake", harness.StandingUnknown, "could not be read"},
	} {
		conv := r.start("one")
		rt := r.fake.Last()
		rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: rt.NativeID(), Version: c.version}})
		f := startedFacts(t, r, conv.ID)
		if f.Standing != c.standing || f.Tested != tested.String() || (c.says == "") != (f.VersionNote == "") || !strings.Contains(f.VersionNote, c.says) {
			t.Errorf("%s: standing %q, tested %q, note %q", c.version, f.Standing, f.Tested, f.VersionNote)
		}
		if view := r.view(conv.ID); view.Facts.Standing != c.standing {
			t.Errorf("%s: the view's facts stand %q", c.version, view.Facts.Standing)
		}
	}
}

// the version read just before a launch stands in when the harness's own
// start reports none; a read that fails leaves the launch to proceed.
func TestTheVersionReadBeforeALaunchFillsAnEmptyReport(t *testing.T) {
	r := newRig(t)
	r.fake.SetVersion("2.1.100", nil)
	conv := r.start("one")
	rt := r.fake.Last()
	rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: rt.NativeID()}})
	if f := startedFacts(t, r, conv.ID); f.Version != "2.1.100" || f.Standing != harness.StandingOlder {
		t.Errorf("filled %q, standing %q", f.Version, f.Standing)
	}
	r.fake.SetVersion("", errors.New("'claude --version' did not answer within 2s"))
	conv = r.start("two")
	rt = r.fake.Last()
	if rt.Spec.Cwd == "" {
		t.Fatal("the launch did not proceed")
	}
	rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: rt.NativeID()}})
	if f := startedFacts(t, r, conv.ID); f.Version != "" || f.Standing != harness.StandingUnknown {
		t.Errorf("an unreadable version: %q, standing %q", f.Version, f.Standing)
	}
}

// the start-up check reads each harness's version, and the workspace's
// report says where it stands; one that cannot be read is unknown, and the
// harness can still run.
func TestTheStartUpCheckReadsVersions(t *testing.T) {
	claude := harnesstest.New(model.HarnessClaude)
	claude.SetVersion("2.1.999", nil)
	codex := harnesstest.New(model.HarnessCodex)
	codex.SetVersion("", errors.New("printed no version"))
	svc := newService(t, filepath.Join(t.TempDir(), "registry.yaml"), allTrusted, claude, codex)
	got := workspaceOf(t, svc).Harnesses
	if len(got) != 2 {
		t.Fatalf("harnesses = %+v", got)
	}
	if c := got[0]; !c.Found || c.Version != "2.1.999" || c.Standing != harness.StandingNewer || c.Tested != harness.Tested[model.HarnessClaude].String() || c.Note != "" {
		t.Errorf("claude = %+v", c)
	}
	if x := got[1]; !x.Found || x.Version != "" || x.Standing != harness.StandingUnknown || !strings.Contains(x.Note, "could not be read") {
		t.Errorf("codex = %+v", x)
	}
	if h, err := svc.Harnesses(); err != nil || len(h) != 2 || h[0].Version != "2.1.999" {
		t.Errorf("Harnesses() = %+v %v", h, err)
	}
}

// a harness updated behind the same path is read again without a restart,
// and the workspace announces it: a link moved to a new build, as Claude
// Code's updater does, and a program replaced in place with its modification
// time kept, as an npm update does.
func TestAnUpdateBehindTheSamePathIsReadAgain(t *testing.T) {
	dir := t.TempDir()
	builds := filepath.Join(dir, "versions")
	if err := os.MkdirAll(builds, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"2.1.320", "2.1.330"} {
		if err := os.WriteFile(filepath.Join(builds, v), []byte("build "+v), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(dir, "claude")
	if err := os.Symlink(filepath.Join(builds, "2.1.320"), link); err != nil {
		t.Fatal(err)
	}
	program := filepath.Join(dir, "codex")
	if err := os.WriteFile(program, []byte("codex one"), 0o755); err != nil {
		t.Fatal(err)
	}
	kept := time.Date(1985, 10, 26, 8, 15, 0, 0, time.UTC)
	if err := os.Chtimes(program, kept, kept); err != nil {
		t.Fatal(err)
	}
	claude := harnesstest.New(model.HarnessClaude)
	claude.SetProgram(link)
	claude.SetVersion("2.1.320", nil)
	codex := harnesstest.New(model.HarnessCodex)
	codex.SetProgram(program)
	codex.SetVersion("0.166.0", nil)
	svc := newService(t, filepath.Join(t.TempDir(), "registry.yaml"), allTrusted, claude, codex)
	versions := func() (string, string) {
		got, err := svc.Harnesses()
		if err != nil || len(got) != 2 {
			t.Fatalf("harnesses = %+v %v", got, err)
		}
		return got[0].Version, got[1].Version
	}
	if c, x := versions(); c != "2.1.320" || x != "0.166.0" {
		t.Fatalf("at start: claude %q, codex %q", c, x)
	}
	// nothing changed: the read is kept, however often it is asked for.
	claude.SetVersion("2.1.999", nil)
	if c, _ := versions(); c != "2.1.320" {
		t.Fatalf("a read with nothing changed: claude %q", c)
	}
	sub := svc.Subscribe()
	defer sub.Close()

	claude.SetVersion("2.1.330", nil)
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(builds, "2.1.330"), link); err != nil {
		t.Fatal(err)
	}
	codex.SetVersion("0.167.0", nil)
	next := program + ".new"
	if err := os.WriteFile(next, []byte("codex two"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(next, kept, kept); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(next, program); err != nil {
		t.Fatal(err)
	}
	// the file watch looks within envWatchEvery and announces the new read,
	// with no workspace read to prompt it.
	deadline := time.After(3 * envWatchEvery)
	for {
		select {
		case <-sub.Updates():
		case <-deadline:
			c, x := versions()
			t.Fatalf("after the update: claude %q, codex %q", c, x)
		}
		svc.mu.Lock()
		c, x := svc.harnessVersions[model.HarnessClaude].version, svc.harnessVersions[model.HarnessCodex].version
		svc.mu.Unlock()
		if c == "2.1.330" && x == "0.167.0" {
			break
		}
	}
	if c, x := versions(); c != "2.1.330" || x != "0.167.0" {
		t.Errorf("after the update: claude %q, codex %q", c, x)
	}
}

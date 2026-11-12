package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/environ"
	"github.com/michaelquigley/mega/internal/harness/harnesstest"
	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/registry"
)

// a marker value distinctive enough that finding it anywhere on disk means a
// value was recorded.
const markerOne, markerTwo = "marker-value-7f3a-one", "marker-value-7f3a-two"

// writeEnvFile writes a checkout's .mega/environment.yaml.
func writeEnvFile(t *testing.T, checkout, activate string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(checkout, ".mega"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "activate: |\n"
	for line := range strings.SplitSeq(activate, "\n") {
		body += "  " + line + "\n"
	}
	if err := os.WriteFile(filepath.Join(checkout, environ.FilePath), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (r *rig) envState(projectID string) EnvState {
	r.t.Helper()
	return r.svc.Workspace().Environments[projectID]
}

// approve approves the file as the project page shows it.
func (r *rig) approve(projectID string) {
	r.t.Helper()
	if _, err := r.svc.ApproveEnvironment(projectID, r.envState(projectID).File.Digest); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) startIn(projectID, prompt string) (ConversationView, error) {
	r.t.Helper()
	return r.svc.StartConversation(projectID, model.HarnessClaude, "", prompt, LaunchChoice{})
}

// noValueOnDisk fails if a value appears in the registry or any event log.
func noValueOnDisk(t *testing.T, r *rig, values ...string) {
	t.Helper()
	root := filepath.Dir(r.path)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, v := range values {
			if strings.Contains(string(data), v) {
				t.Errorf("'%s' holds the value %q", path, v)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestEachProjectLaunchesInItsOwnEnvironment(t *testing.T) {
	t.Setenv("MEGA_ENV_LEAK", "mega's own")
	r := newRig(t)
	one := r.project
	two, err := r.svc.RegisterProject("two", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeEnvFile(t, one.Checkout, "export MEGA_MARKER="+markerOne)
	writeEnvFile(t, two.Checkout, "export MEGA_MARKER="+markerTwo)
	r.approve(one.ID)
	r.approve(two.ID)

	launch := func(projectID, want string) *harnesstest.Runtime {
		t.Helper()
		if _, err := r.startIn(projectID, "hello"); err != nil {
			t.Fatal(err)
		}
		rt := r.fake.Last()
		if got, _ := environ.Value(rt.Spec.Env, "MEGA_MARKER"); got != want {
			t.Errorf("launched with marker %q, want %q", got, want)
		}
		if _, ok := environ.Value(rt.Spec.Env, "MEGA_ENV_LEAK"); ok {
			t.Error("mega's own environment reached the launch")
		}
		return rt
	}
	rtOne := launch(one.ID, markerOne)
	rtTwo := launch(two.ID, markerTwo)
	started(rtOne)
	started(rtTwo)

	// the start records names and fingerprints, and no value anywhere.
	var facts *model.EnvironmentFacts
	for _, ev := range r.events(r.svc.Workspace().Conversations[0].ID) {
		if ev.Kind == model.EventRuntime && ev.Status == model.StatusStarted {
			facts = ev.Facts.Environment
		}
	}
	if facts == nil || facts.Source != model.EnvSourceFile || !slices.Equal(facts.Changed, []string{"MEGA_MARKER"}) || !strings.HasPrefix(facts.Fingerprint, "sha256:") || facts.Activation == "" {
		t.Fatalf("the start event's environment: %+v", facts)
	}
	noValueOnDisk(t, r, markerOne, markerTwo)

	// the helpers run in the project's environment too.
	if _, err := r.svc.Discover(two.ID, model.HarnessClaude); err != nil {
		t.Fatal(err)
	}
	places := r.fake.Places()
	if got, _ := environ.Value(places[len(places)-1].Env, "MEGA_MARKER"); got != markerTwo || places[len(places)-1].Cwd != two.Checkout {
		t.Errorf("discovery ran with marker %q in %q", got, places[len(places)-1].Cwd)
	}
	if _, err := r.svc.Models(one.ID, model.HarnessClaude); err != nil {
		t.Fatal(err)
	}
	places = r.fake.Places()
	if got, _ := environ.Value(places[len(places)-1].Env, "MEGA_MARKER"); got != markerOne {
		t.Errorf("the model list ran with marker %q", got)
	}
}

func TestNoFileIsTheShellEnvironmentAndNothingMore(t *testing.T) {
	r := newRig(t)
	if st := r.envState(r.project.ID); st.Source != model.EnvSourceNone || st.Status != EnvReady || st.Refusal != nil {
		t.Fatalf("no file: %+v", st)
	}
	r.start("hello")
	rt := r.fake.Last()
	if _, ok := environ.Value(rt.Spec.Env, "MEGA_MARKER"); ok {
		t.Error("a marker with no activation")
	}
	if home, _ := environ.Value(rt.Spec.Env, "HOME"); home == "" || home == os.Getenv("HOME") {
		t.Errorf("the launch's HOME is %q, not the base's", home)
	}
	started(rt)
	facts := r.view(r.svc.Workspace().Conversations[0].ID).Facts.Environment
	if facts == nil || facts.Source != model.EnvSourceNone || len(facts.Changed) != 0 || facts.Activation != "" {
		t.Errorf("a start with no file: %+v", facts)
	}
}

func TestAFailedActivationRefusesTheLaunch(t *testing.T) {
	r := newRig(t)
	conv := r.start("before")
	rt := r.fake.Last()
	started(rt)
	finishTurn(rt, "a-1", "done")
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the release", rt.Exited)
	launched := len(r.fake.Runtimes())

	writeEnvFile(t, r.project.Checkout, "echo 'devprj: secret-ish-output-91c2' >&2\nfalse")
	r.approve(r.project.ID)
	for name, attempt := range map[string]func() error{
		"launch": func() error { _, err := r.startIn(r.project.ID, "new"); return err },
		"prompt": func() error { return r.svc.Prompt(conv.ID, "more") },
		"start":  func() error { return r.svc.Start(conv.ID) },
	} {
		err := attempt()
		wantCode(t, err, CodeEnvironmentFailed)
		if err == nil || !strings.Contains(err.Error(), "secret-ish-output-91c2") || !strings.Contains(err.Error(), environ.FilePath) {
			t.Errorf("%s: the refusal should carry the activation's message and name the file: %v", name, err)
		}
	}
	if len(r.fake.Runtimes()) != launched || len(r.svc.Workspace().Conversations) != 1 {
		t.Error("a failed activation launched something, or left a conversation behind")
	}
	// the log says the start did not happen, never what the activation printed.
	evs := r.events(conv.ID)
	if last := evs[len(evs)-1]; last.Status != model.StatusNotStarted || !strings.Contains(last.Text, "could not be resolved") {
		t.Errorf("the failed start was recorded as %s: %q", last.Status, last.Text)
	}
	noValueOnDisk(t, r, "secret-ish-output-91c2")
	if st := r.envState(r.project.ID); st.Resolved == nil || st.Resolved.Error == "" || strings.Contains(st.Resolved.Error, "secret-ish") {
		t.Errorf("the project's last resolution: %+v", st.Resolved)
	}
	// reads, rename, star and removal keep working.
	if _, err := r.svc.History(conv.ID, HistoryQuery{Turns: 10}); err != nil {
		t.Errorf("reading history: %v", err)
	}
	starred := true
	if _, err := r.svc.UpdateProject(r.project.ID, ProjectChange{Starred: &starred}); err != nil {
		t.Errorf("starring: %v", err)
	}
	if _, err := r.svc.UpdateConversation(conv.ID, "renamed", ""); err != nil {
		t.Errorf("renaming: %v", err)
	}
	if err := r.svc.RemoveConversation(conv.ID); err != nil {
		t.Errorf("removing: %v", err)
	}
}

func TestATimedOutActivationRefusesTheLaunch(t *testing.T) {
	r := newRig(t)
	r.svc.resolver.(*environ.Resolver).Timeout = 300 * time.Millisecond
	writeEnvFile(t, r.project.Checkout, "sleep 20 & wait")
	r.approve(r.project.ID)
	start := time.Now()
	_, err := r.startIn(r.project.ID, "hello")
	wantCode(t, err, CodeEnvironmentFailed)
	if err == nil || !strings.Contains(err.Error(), "did not finish") || time.Since(start) > 5*time.Second {
		t.Errorf("a slow activation: %v after %s", err, time.Since(start))
	}
}

func TestAChangedActivationShowsTheRuntimeAsOlder(t *testing.T) {
	r := newRig(t)
	writeEnvFile(t, r.project.Checkout, "export MEGA_MARKER="+markerOne)
	r.approve(r.project.ID)
	conv, err := r.startIn(r.project.ID, "hello")
	if err != nil {
		t.Fatal(err)
	}
	started(r.fake.Last())
	if r.view(conv.ID).EnvironmentStale {
		t.Fatal("a runtime that just started is shown as older")
	}
	// the operator's own activation now differs from the one it started in.
	if _, err := r.svc.SetEnvironmentOverride(r.project.ID, &model.EnvOverride{Activate: "export MEGA_MARKER=" + markerTwo}); err != nil {
		t.Fatal(err)
	}
	if !r.view(conv.ID).EnvironmentStale {
		t.Error("a changed activation does not show the running conversation as older")
	}
	// the same activation again is the same environment.
	if _, err := r.svc.SetEnvironmentOverride(r.project.ID, nil); err != nil {
		t.Fatal(err)
	}
	if r.view(conv.ID).EnvironmentStale {
		t.Error("the original activation still shows it as older")
	}
}

func TestAnUnapprovedFileIsNeverRun(t *testing.T) {
	r := newRig(t)
	ran := filepath.Join(t.TempDir(), "ran")
	writeEnvFile(t, r.project.Checkout, "touch "+ran+"\nexport MEGA_MARKER="+markerOne)

	st := r.envState(r.project.ID)
	if st.Status != EnvUnapproved || st.Refusal == nil || st.Refusal.Code != CodeEnvironmentUnapproved || !strings.Contains(st.Refusal.Message, environ.FilePath) {
		t.Fatalf("unapproved: %+v", st)
	}
	_, err := r.startIn(r.project.ID, "hello")
	wantCode(t, err, CodeEnvironmentUnapproved)
	_, err = r.svc.Discover(r.project.ID, model.HarnessClaude)
	wantCode(t, err, CodeEnvironmentUnapproved)
	_, err = r.svc.Models(r.project.ID, model.HarnessClaude)
	wantCode(t, err, CodeEnvironmentUnapproved)
	_, err = r.svc.Paths(r.project.ID, "")
	wantCode(t, err, CodeEnvironmentUnapproved)
	r.svc.Workspace() // a read never resolves an unapproved file either
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(ran); err == nil {
		t.Fatal("an unapproved activation ran")
	}

	// approving at a fingerprint the operator was not shown is refused.
	if _, err := r.svc.ApproveEnvironment(r.project.ID, environ.Digest([]byte("a version never shown"))); ErrorCode(err) != CodeConflict {
		t.Errorf("approving another version: %v", err)
	}
	r.approve(r.project.ID)
	conv, err := r.startIn(r.project.ID, "hello")
	if err != nil {
		t.Fatalf("an approved file: %v", err)
	}
	if _, err := os.Stat(ran); err != nil {
		t.Error("the approved activation did not run")
	}
	rt := r.fake.Last()
	started(rt)
	finishTurn(rt, "a-1", "done")
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the release", rt.Exited)

	// changed after approval: refused, and the approved version is not
	// stored, so outside git it cannot be shown, only named.
	writeEnvFile(t, r.project.Checkout, "export MEGA_MARKER="+markerTwo)
	st = r.envState(r.project.ID)
	if st.Status != EnvChanged || st.Approved == nil || st.Approved.Digest == st.File.Digest {
		t.Fatalf("changed: status %s, approved %+v", st.Status, st.Approved)
	}
	if v, err := r.svc.ApprovedVersion(r.project.ID); err != nil || v.Available || v.Digest != st.Approved.Digest {
		t.Errorf("the approved version outside git: %+v %v", v, err)
	}
	wantCode(t, r.svc.Start(conv.ID), CodeEnvironmentChanged)
	wantCode(t, r.svc.Prompt(conv.ID, "more"), CodeEnvironmentChanged)
	wantCode(t, r.svc.Resume(conv.ID), CodeEnvironmentChanged)

	// approving the new version lifts it.
	r.approve(r.project.ID)
	if err := r.svc.Start(conv.ID); err != nil {
		t.Fatalf("after approving again: %v", err)
	}
	if got, _ := environ.Value(r.fake.Last().Spec.Env, "MEGA_MARKER"); got != markerTwo {
		t.Errorf("the new version's marker is %q", got)
	}
}

func TestAnOverrideIgnoresTheFile(t *testing.T) {
	r := newRig(t)
	ran := filepath.Join(t.TempDir(), "ran")
	writeEnvFile(t, r.project.Checkout, "touch "+ran+"\nexport MEGA_MARKER="+markerOne)
	if _, err := r.svc.SetEnvironmentOverride(r.project.ID, &model.EnvOverride{Ignore: true}); err != nil {
		t.Fatal(err)
	}
	if st := r.envState(r.project.ID); st.Source != model.EnvSourceIgnored || st.Refusal != nil {
		t.Fatalf("ignored: %+v", st)
	}
	r.start("hello")
	if _, ok := environ.Value(r.fake.Last().Spec.Env, "MEGA_MARKER"); ok {
		t.Error("an ignored file's activation applied")
	}
	if _, err := os.Stat(ran); err == nil {
		t.Error("an ignored file ran")
	}
	for _, bad := range []*model.EnvOverride{{Ignore: true, Activate: "x"}, {}} {
		if _, err := r.svc.SetEnvironmentOverride(r.project.ID, bad); ErrorCode(err) != CodeInvalid {
			t.Errorf("override %+v: %v", bad, err)
		}
	}
}

func TestAMalformedFileIsReportedAndRefused(t *testing.T) {
	r := newRig(t)
	if err := os.MkdirAll(filepath.Join(r.project.Checkout, ".mega"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.project.Checkout, environ.FilePath), []byte("activate: [devprj]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := r.envState(r.project.ID)
	if st.Status != EnvMalformed || st.Refusal == nil || st.Refusal.Code != CodeEnvironmentInvalid || !strings.Contains(st.Refusal.Message, environ.FilePath) {
		t.Fatalf("malformed: %+v", st)
	}
	_, err := r.startIn(r.project.ID, "hello")
	wantCode(t, err, CodeEnvironmentInvalid)
	_, err = r.svc.ApproveEnvironment(r.project.ID, st.File.Digest)
	wantCode(t, err, CodeEnvironmentInvalid)
}

func TestApprovalAndOverrideSurviveARestartAndAnOldRegistryApprovesNothing(t *testing.T) {
	r := newRig(t)
	writeEnvFile(t, r.project.Checkout, "export MEGA_MARKER="+markerOne)
	r.approve(r.project.ID)
	if _, err := r.svc.SetEnvironmentOverride(r.project.ID, &model.EnvOverride{Activate: "export MEGA_MARKER=" + markerTwo}); err != nil {
		t.Fatal(err)
	}
	r.restart()
	st := r.envState(r.project.ID)
	if st.Approved == nil || st.Approved.Digest != st.File.Digest || st.Override == nil || st.Source != model.EnvSourceOverride {
		t.Fatalf("after a restart: %+v", st)
	}

	// a registry written before approvals: no approval, no override, and the
	// version is unchanged.
	old := filepath.Join(t.TempDir(), "registry.yaml")
	checkout := t.TempDir()
	writeEnvFile(t, checkout, "export MEGA_MARKER="+markerOne)
	body := "version: 1\nprojects:\n  - id: p1\n    name: old\n    checkout: " + checkout + "\nconversations: []\n"
	if err := os.WriteFile(old, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	reg, err := registry.Load(old)
	if err != nil {
		t.Fatal(err)
	}
	if doc := reg.Snapshot(); doc.Projects[0].EnvApproved != nil || doc.Projects[0].EnvOverride != nil || doc.Version != 1 {
		t.Errorf("an old registry: %+v", doc.Projects[0])
	}
	svc := newService(t, old, allTrusted)
	if st := svc.Workspace().Environments["p1"]; st.Status != EnvUnapproved {
		t.Errorf("an old registry's project reads %s", st.Status)
	}
}

func TestTheApprovedVersionIsFoundInGitHistory(t *testing.T) {
	r := newRig(t)
	checkout := r.project.Checkout
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = checkout
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git("init", "-q")
	writeEnvFile(t, checkout, "devprj")
	approved := r.envState(r.project.ID).File.Content
	git("add", environ.FilePath)
	git("commit", "-q", "-m", "environment")
	r.approve(r.project.ID)
	writeEnvFile(t, checkout, "devprj\nexport EXTRA=1")
	st := r.envState(r.project.ID)
	if st.Status != EnvChanged {
		t.Fatalf("changed: %s", st.Status)
	}
	v, err := r.svc.ApprovedVersion(r.project.ID)
	if err != nil || !v.Available || v.Content != approved || !(strings.HasPrefix(v.Found, "commit ") || v.Found == "the git index") {
		t.Errorf("the approved version from git: %+v %v", v, err)
	}
	if v.Content == st.File.Content {
		t.Error("the approved version is the current one")
	}
}

// an edit to the file on disk, by a terminal or an agent, is announced
// without any other change, so the project page and the headers follow it.
func TestAnEditToTheFileIsAnnounced(t *testing.T) {
	r := newRig(t)
	sub := r.svc.Subscribe()
	time.Sleep(envWatchEvery + 500*time.Millisecond) // the watch's first look
	for len(sub.Updates()) > 0 {
		<-sub.Updates()
	}
	writeEnvFile(t, r.project.Checkout, "export MEGA_MARKER="+markerOne)
	select {
	case u := <-sub.Updates():
		if !u.WorkspaceChanged {
			t.Errorf("the announcement: %+v", u)
		}
	case <-time.After(3 * envWatchEvery):
		t.Fatal("an edit to the file was not announced")
	}
}

// the override the caller passed, and the project returned, are the caller's
// own: changing either changes no workspace state.
func TestTheOverrideIsTheServicesOwn(t *testing.T) {
	r := newRig(t)
	passed := &model.EnvOverride{Activate: "export MEGA_MARKER=" + markerOne}
	returned, err := r.svc.SetEnvironmentOverride(r.project.ID, passed)
	if err != nil {
		t.Fatal(err)
	}
	passed.Activate = "changed by the caller"
	returned.EnvOverride.Activate = "changed through the result"
	if st := r.envState(r.project.ID); st.Override == nil || st.Override.Activate != "export MEGA_MARKER="+markerOne {
		t.Errorf("the override after the caller changed its copies: %+v", st.Override)
	}
	st := r.envState(r.project.ID)
	st.Override.Activate = "changed through a read"
	if again := r.envState(r.project.ID); again.Override.Activate != "export MEGA_MARKER="+markerOne {
		t.Errorf("changing a read changed the override: %+v", again.Override)
	}
}

// what a read returns is a copy: changing a conversation's environment facts
// or a project's resolution in a read changes nothing the service holds.
func TestReadsShareNoEnvironmentState(t *testing.T) {
	r := newRig(t)
	writeEnvFile(t, r.project.Checkout, "export MEGA_MARKER="+markerOne)
	r.approve(r.project.ID)
	conv, err := r.startIn(r.project.ID, "hello")
	if err != nil {
		t.Fatal(err)
	}
	started(r.fake.Last())
	view := r.view(conv.ID)
	if view.Environment == nil || view.Facts.Environment == nil {
		t.Fatal("no environment on the view")
	}
	fingerprint := view.Environment.Fingerprint
	view.Environment.Fingerprint = "sha256:changed-by-a-reader"
	view.Environment.Changed[0] = "CHANGED_BY_A_READER"
	again := r.view(conv.ID)
	if again.Environment.Fingerprint != fingerprint || again.Environment.Changed[0] != "MEGA_MARKER" || again.EnvironmentStale {
		t.Errorf("changing a view changed the service's facts: %+v, stale %t", again.Environment, again.EnvironmentStale)
	}
	values, err := r.svc.ResolveEnvironment(r.project.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	values.Changed[0] = "CHANGED_BY_A_CALLER"
	if again := r.envState(r.project.ID); again.Resolved.Changed[0] != "MEGA_MARKER" {
		t.Errorf("changing a resolution's answer changed the project's resolution: %+v", again.Resolved)
	}
	st := r.envState(r.project.ID)
	st.Resolved.Fingerprint = "sha256:changed-by-a-reader"
	st.Resolved.Changed[0] = "CHANGED_BY_A_READER"
	if again := r.envState(r.project.ID); again.Resolved.Fingerprint == "sha256:changed-by-a-reader" || again.Resolved.Changed[0] != "MEGA_MARKER" {
		t.Errorf("changing a read changed the project's resolution: %+v", again.Resolved)
	}
}

// a subscriber's started event is its own: changing its facts changes nothing
// the service shows afterwards.
func TestAStartedEventSharesNothingWithTheService(t *testing.T) {
	r := newRig(t)
	writeEnvFile(t, r.project.Checkout, "export MEGA_MARKER="+markerOne)
	r.approve(r.project.ID)
	sub := r.svc.Subscribe()
	conv, err := r.startIn(r.project.ID, "hello")
	if err != nil {
		t.Fatal(err)
	}
	started(r.fake.Last())
	var startedEvent *model.Event
	deadline := time.After(5 * time.Second)
	for startedEvent == nil {
		select {
		case u := <-sub.Updates():
			if u.Event != nil && u.Event.Status == model.StatusStarted {
				startedEvent = u.Event
			}
		case <-deadline:
			t.Fatal("no started event reached the subscriber")
		}
	}
	if startedEvent.Facts == nil || startedEvent.Facts.Environment == nil {
		t.Fatalf("the started event carries no environment: %+v", startedEvent.Facts)
	}
	fingerprint := startedEvent.Facts.Environment.Fingerprint
	startedEvent.Facts.Environment.Fingerprint = "sha256:changed-by-a-subscriber"
	startedEvent.Facts.Environment.Changed[0] = "CHANGED_BY_A_SUBSCRIBER"
	startedEvent.Facts.Model = "changed by a subscriber"
	view := r.view(conv.ID)
	if view.Environment.Fingerprint != fingerprint || view.Environment.Changed[0] != "MEGA_MARKER" || view.Facts.Model == "changed by a subscriber" || view.EnvironmentStale {
		t.Errorf("changing a subscriber's event changed the service: %+v %+v", view.Environment, view.Facts)
	}
}

// a file that changes while its approved activation is being resolved
// refuses the launch: nothing starts on an environment whose file is no
// longer the one approved.
func TestAFileChangedDuringResolutionRefusesTheLaunch(t *testing.T) {
	r := newRig(t)
	writeEnvFile(t, r.project.Checkout, "sleep 0.5\nexport MEGA_MARKER="+markerOne)
	r.approve(r.project.ID)
	go func() {
		time.Sleep(150 * time.Millisecond)
		writeEnvFile(t, r.project.Checkout, "export MEGA_MARKER="+markerTwo)
	}()
	launched := len(r.fake.Runtimes())
	_, err := r.startIn(r.project.ID, "hello")
	wantCode(t, err, CodeEnvironmentChanged)
	if len(r.fake.Runtimes()) != launched {
		t.Error("a runtime launched on a file that changed while it resolved")
	}
}

// a pending request is the service's own: changing the copy a read returns,
// or the one a subscriber receives, changes nothing in the pending set.
func TestPendingRequestsShareNothing(t *testing.T) {
	r := newRig(t)
	sub := r.svc.Subscribe()
	conv := r.start("hello")
	rt := r.fake.Last()
	started(rt)
	rt.Ask(model.RequestPermission, map[string]any{"tool": "Bash", "input": map[string]any{"command": "sleep 3"}})
	var published *model.Request
	deadline := time.After(5 * time.Second)
	for published == nil {
		select {
		case u := <-sub.Updates():
			if u.Event != nil && u.Event.Kind == model.EventRequest {
				published = u.Event.Req
			}
		case <-deadline:
			t.Fatal("no request reached the subscriber")
		}
	}
	waitFor(t, "the request to be pending", func() bool { return len(r.view(conv.ID).Pending) == 1 })
	read := r.view(conv.ID).Pending[0]
	read.Payload.(map[string]any)["input"].(map[string]any)["command"] = "rm -rf by a reader"
	published.Payload.(map[string]any)["tool"] = "changed by a subscriber"
	published.Kind = "changed"
	held := r.view(conv.ID).Pending[0]
	payload := held.Payload.(map[string]any)
	if held.Kind != model.RequestPermission || payload["tool"] != "Bash" || payload["input"].(map[string]any)["command"] != "sleep 3" {
		t.Errorf("changing copies changed the pending request: %+v", held)
	}
}

// a subscriber's event is a copy: changing its kind, seq or time changes
// nothing the service goes on to do with the event it recorded.
func TestAPublishedEventIsACopy(t *testing.T) {
	r := newRig(t)
	sub := r.svc.Subscribe()
	conv := r.start("hello")
	rt := r.fake.Last()
	started(rt)
	go func() {
		for u := range sub.Updates() {
			if u.Event != nil {
				u.Event.Kind, u.Event.Seq, u.Event.At = "changed", 9999, time.Time{}
			}
		}
	}()
	finishTurn(rt, "a-1", "done")
	waitFor(t, "the turn to end", func() bool { return !r.view(conv.ID).TurnActive })
	v := r.view(conv.ID)
	if v.OutputSeq == 9999 || v.OutputSeq == 0 || v.Attention != model.AttentionUnread {
		t.Errorf("a subscriber's changes reached the service: output %d, attention %s", v.OutputSeq, v.Attention)
	}
}

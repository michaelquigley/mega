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
	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/harness/claude"
	"github.com/michaelquigley/mega/internal/model"
)

// an approved activation that puts a fake git and a fake node first on PATH
// changes what an agent's commands see, but not which program mega runs: the
// shim runs on the node found before the activation, and path search on the
// git found there, while the shim's process still gets the activated PATH.
func TestAnActivationCannotChooseMegasPrograms(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	realNode, _ := filepath.EvalSymlinks(node)
	work := t.TempDir()
	checkout := filepath.Join(work, "checkout")
	planted := filepath.Join(checkout, "planted")
	ran := filepath.Join(work, "a-planted-program-ran")
	record := filepath.Join(work, "shim-record")
	for _, name := range []string{"node", "git"} {
		if err := os.MkdirAll(planted, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(planted, name), []byte("#!/bin/sh\necho "+name+" >> "+ran+"\nexit 1\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	gitIn := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = checkout
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	gitIn("init", "-q")
	if err := os.WriteFile(filepath.Join(checkout, ".gitignore"), []byte("ignored.txt\nplanted/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"tracked.txt", "ignored.txt"} {
		if err := os.WriteFile(filepath.Join(checkout, f), []byte(f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitIn("add", ".gitignore", "tracked.txt")
	gitIn("commit", "-q", "-m", "fixture")

	// a stand-in shim: it records which node runs it and the first PATH entry
	// its process sees, then exits.
	shim := filepath.Join(work, "shim")
	if err := os.MkdirAll(filepath.Join(shim, "node_modules", "@anthropic-ai", "claude-agent-sdk"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shim, "shim.mjs"), []byte("import { writeFileSync } from 'node:fs'\nwriteFileSync(process.env.MEGA_SHIM_RECORD, process.execPath + '\\n' + process.env.PATH.split(':')[0] + '\\n')\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := newService(t, filepath.Join(work, "data", "registry.yaml"), allTrusted, claude.New(claude.Config{Node: "node", Shim: shim, Executable: "/bin/true"}))
	// the base has real node and git; the activation puts the planted ones
	// first.
	svc.resolver = &environ.Resolver{Shell: "/bin/bash", Base: []string{"HOME=" + t.TempDir(), "PATH=" + filepath.Dir(node) + ":/usr/local/bin:/usr/bin:/bin"}, Timeout: 5 * time.Second, SkipProfile: true}
	project, err := svc.RegisterProject("planted", checkout)
	if err != nil {
		t.Fatal(err)
	}
	writeEnvFile(t, checkout, "export PATH=$PWD/planted:$PATH\nexport MEGA_SHIM_RECORD="+record)
	if _, err := svc.ApproveEnvironment(project.ID, svc.Workspace().Environments[project.ID].File.Digest); err != nil {
		t.Fatal(err)
	}

	// the launch runs the stand-in shim, which exits at once; whatever the
	// launch makes of that, the record says which node ran.
	_, _ = svc.StartConversation(project.ID, model.HarnessClaude, "", "hello", LaunchChoice{})
	waitFor(t, "the shim to record", func() bool { _, err := os.Stat(record); return err == nil })
	data, _ := os.ReadFile(record)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	ranNode, _ := filepath.EvalSymlinks(lines[0])
	if ranNode != realNode {
		t.Errorf("the shim ran on %q, not the node found before the activation (%q)", lines[0], realNode)
	}
	if len(lines) < 2 || lines[1] != planted {
		t.Errorf("the shim's process saw PATH starting %q; the activation's %q should still reach it", lines[1:], planted)
	}

	// path search runs the git found before the activation: the listing
	// leaves out what git ignores, which a walk would not.
	paths, err := svc.Paths(project.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(paths, "tracked.txt") || slices.Contains(paths, "ignored.txt") {
		t.Errorf("path search did not list through git: %v", paths)
	}
	// the preflight checks the same programs, found the same way.
	if st := svc.Workspace().Environments[project.ID]; len(st.Missing) != 0 {
		t.Errorf("the preflight: %+v", st.Missing)
	}
	if got, err := os.ReadFile(ran); err == nil {
		t.Errorf("a planted program ran: %s", got)
	}
}

// Program follows a script's #!/usr/bin/env interpreter into find, not the
// PATH the process will see.
func TestProgramLocatesAScriptsInterpreter(t *testing.T) {
	dir := t.TempDir()
	tools := filepath.Join(dir, "tools")
	interp := filepath.Join(dir, "interp")
	for _, d := range []string{tools, interp} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(tools, "wrapper"), []byte("#!/usr/bin/env -S fakeinterp --flag\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// a stand-in for a compiled interpreter: no #! line, so the chain ends.
	if err := os.WriteFile(filepath.Join(interp, "fakeinterp"), []byte("\x7fELF stand-in\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	argv, err := harness.Program("wrapper", []string{"PATH=" + tools + ":" + interp})
	want := []string{filepath.Join(interp, "fakeinterp"), "--flag", filepath.Join(tools, "wrapper")}
	if err != nil || !slices.Equal(argv, want) {
		t.Errorf("program %v (%v), want %v", argv, err, want)
	}
	if _, err := harness.Program("wrapper", []string{"PATH=" + tools}); err == nil || !strings.Contains(err.Error(), "fakeinterp") {
		t.Errorf("a missing interpreter: %v", err)
	}

	// env -S is followed into find too, never left for env to look up on the
	// activated PATH, where a planted interpreter sits first.
	planted := filepath.Join(dir, "planted")
	if err := os.MkdirAll(planted, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(planted, "fakeinterp"), []byte("\x7fELF planted\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tools, "split"), []byte("#!/usr/bin/env -S fakeinterp --flag\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	argv, err = harness.Program("split", []string{"PATH=" + tools + ":" + interp})
	want = []string{filepath.Join(interp, "fakeinterp"), "--flag", filepath.Join(tools, "split")}
	if err != nil || !slices.Equal(argv, want) {
		t.Errorf("env -S: %v (%v), want %v", argv, err, want)
	}
	// a chain is followed all the way: an interpreter that is itself an env
	// script, and an absolute interpreter that is one, resolve through find.
	if err := os.WriteFile(filepath.Join(interp, "chained"), []byte("#!/usr/bin/env fakeinterp\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tools, "outer"), []byte("#!/usr/bin/env chained\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	argv, err = harness.Program("outer", []string{"PATH=" + tools + ":" + interp})
	want = []string{filepath.Join(interp, "fakeinterp"), filepath.Join(interp, "chained"), filepath.Join(tools, "outer")}
	if err != nil || !slices.Equal(argv, want) {
		t.Errorf("an env chain: %v (%v), want %v", argv, err, want)
	}
	if err := os.WriteFile(filepath.Join(tools, "absolute"), []byte("#!"+filepath.Join(interp, "chained")+" -x\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	argv, err = harness.Program("absolute", []string{"PATH=" + tools + ":" + interp})
	want = []string{filepath.Join(interp, "fakeinterp"), filepath.Join(interp, "chained"), "-x", filepath.Join(tools, "absolute")}
	if err != nil || !slices.Equal(argv, want) {
		t.Errorf("an absolute interpreter that is an env script: %v (%v), want %v", argv, err, want)
	}
	// a loop is bounded, not followed forever.
	if err := os.WriteFile(filepath.Join(tools, "loop"), []byte("#!/usr/bin/env loop\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if argv, err := harness.Program("loop", []string{"PATH=" + tools}); err == nil {
		t.Errorf("a loop ran as %v", argv)
	}

	// any other env form is refused rather than run unresolved, env naming
	// env above all, which would look the next program up on the activated
	// PATH.
	for name, line := range map[string]string{
		"ignores":   "#!/usr/bin/env -i fakeinterp\n",
		"assigns":   "#!/usr/bin/env FOO=1 fakeinterp\n",
		"bare":      "#!/usr/bin/env\n",
		"nestedenv": "#!/usr/bin/env -S env fakeinterp\n",
		"twowords":  "#!/usr/bin/env fakeinterp --flag\n",
		"relative":  "#!bin/fakeinterp\n",
	} {
		if err := os.WriteFile(filepath.Join(tools, name), []byte(line), 0o755); err != nil {
			t.Fatal(err)
		}
		if argv, err := harness.Program(name, []string{"PATH=" + tools + ":" + interp}); err == nil {
			t.Errorf("%s: an env form mega does not follow ran as %v", name, argv)
		}
	}
}

// the approval records the names its activation changes; a later resolution
// that changes another set is refused, with the difference, until the
// operator approves again.
func TestTheApprovalHoldsTheActivationToItsNames(t *testing.T) {
	r := newRig(t)
	sourced := filepath.Join(r.project.Checkout, "envrc")
	writeSourced := func(body string) {
		t.Helper()
		if err := os.WriteFile(sourced, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeSourced("export MEGA_MARKER=" + markerOne + "\n")
	writeEnvFile(t, r.project.Checkout, ". ./envrc")
	r.approve(r.project.ID)
	st := r.envState(r.project.ID)
	if st.Approved == nil || !st.Approved.NamesKnown || !slices.Equal(st.Approved.Changed, []string{"MEGA_MARKER"}) {
		t.Fatalf("the approval's names: %+v", st.Approved)
	}
	if _, err := r.startIn(r.project.ID, "hello"); err != nil {
		t.Fatalf("a launch with the approved names: %v", err)
	}

	// the sourced file, which the approval does not cover, now adds a name
	// and drops one.
	writeSourced("export GIT_SSH_COMMAND=" + markerTwo + "\n")
	launched := len(r.fake.Runtimes())
	_, err := r.startIn(r.project.ID, "hello")
	wantCode(t, err, CodeEnvironmentNamesChanged)
	if err == nil || !strings.Contains(err.Error(), "GIT_SSH_COMMAND") || !strings.Contains(err.Error(), "MEGA_MARKER") {
		t.Errorf("the refusal should name what was added and removed: %v", err)
	}
	if len(r.fake.Runtimes()) != launched {
		t.Error("a runtime launched with other names")
	}
	st = r.envState(r.project.ID)
	if st.Status != EnvNamesChanged || st.Refusal == nil || !slices.Equal(st.Resolved.Added, []string{"GIT_SSH_COMMAND"}) || !slices.Equal(st.Resolved.Removed, []string{"MEGA_MARKER"}) {
		t.Errorf("the project's state: %s %+v", st.Status, st.Resolved)
	}
	// helpers wait on it too, and names are all that is kept.
	_, err = r.svc.Models(r.project.ID, model.HarnessClaude)
	wantCode(t, err, CodeEnvironmentNamesChanged)
	noValueOnDisk(t, r, markerOne, markerTwo)

	// putting the sourced file back and resolving again lifts it: the
	// refusal was about that resolution, not the approved text.
	writeSourced("export MEGA_MARKER=" + markerOne + "\n")
	if _, err := r.svc.ResolveEnvironment(r.project.ID, false); err != nil {
		t.Fatalf("resolving with the approved names again: %v", err)
	}
	if st := r.envState(r.project.ID); st.Refusal != nil {
		t.Fatalf("the refusal outlived a resolution with the approved names: %+v", st.Refusal)
	}
	writeSourced("export GIT_SSH_COMMAND=" + markerTwo + "\n")
	_, err = r.startIn(r.project.ID, "hello")
	wantCode(t, err, CodeEnvironmentNamesChanged)

	// approving again, the same text, takes the new names.
	r.approve(r.project.ID)
	st = r.envState(r.project.ID)
	if st.Refusal != nil || !slices.Equal(st.Approved.Changed, []string{"GIT_SSH_COMMAND"}) {
		t.Fatalf("after approving again: %s %+v", st.Status, st.Approved)
	}
	if _, err := r.startIn(r.project.ID, "hello"); err != nil {
		t.Errorf("a launch after approving again: %v", err)
	}
}

// an approval stored in the short form reads as not approved, and says why;
// approving takes the full digest only.
func TestAShortFormApprovalNeedsApprovingAgain(t *testing.T) {
	data := t.TempDir()
	checkout := t.TempDir()
	writeEnvFile(t, checkout, "export MEGA_MARKER="+markerOne)
	short := environ.ReadFile(checkout).Fingerprint
	body := "version: 1\nprojects:\n  - id: p1\n    name: old\n    checkout: " + checkout + "\n    env_approved:\n      fingerprint: " + short + "\n      at: 2026-11-12T10:00:00Z\nconversations: []\n"
	if err := os.WriteFile(filepath.Join(data, "registry.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := newService(t, filepath.Join(data, "registry.yaml"), allTrusted)
	st := svc.Workspace().Environments["p1"]
	if st.Status != EnvUnapproved || st.Refusal == nil || !strings.Contains(st.Refusal.Message, "short fingerprint") {
		t.Fatalf("a short-form approval: %s %+v", st.Status, st.Refusal)
	}
	if _, err := svc.ApproveEnvironment("p1", short); ErrorCode(err) != CodeInvalid {
		t.Errorf("approving at the short form: %v", err)
	}
	if _, err := svc.ApproveEnvironment("p1", st.File.Digest); err != nil {
		t.Fatal(err)
	}
	if st := svc.Workspace().Environments["p1"]; st.Status != EnvReady || st.Approved.Digest != st.File.Digest {
		t.Errorf("approved again: %s %+v", st.Status, st.Approved)
	}
}

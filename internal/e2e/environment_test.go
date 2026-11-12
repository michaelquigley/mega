//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/harness/codex"
	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/registry"
	"github.com/michaelquigley/mega/internal/service"
)

// envProbe prints only what the check compares: the Go path, the first PATH
// entry, and where the Go path's bin sits on PATH. never the whole
// environment, which carries credentials.
const envProbe = `#!/bin/sh
echo "gopath=${GOPATH:-unset}"
echo "path1=${PATH%%:*}"
n=0; at=0; IFS=:
for entry in $PATH; do n=$((n+1)); if [ "$entry" = "${GOPATH:-/nonexistent}/bin" ] && [ $at = 0 ]; then at=$n; fi; done
echo "gobin_at=$at"
`

const envPrompt = "Run ./envprobe.sh exactly once with your shell tool, then reply with its output verbatim. Nothing else."

// placeFile writes a file into a fixture for the test's life and puts back
// whatever was there before.
func placeFile(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	old, err := os.ReadFile(path)
	existed := err == nil
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.WriteFile(path, old, mode)
		} else {
			_ = os.Remove(path)
		}
	})
}

// probeTurn runs the probe in one conversation and returns what it printed.
// nothing is approved on the operator's behalf: the claude fixture's own
// narrow allow rule lets exactly the probe run, and codex runs it under the
// operator's own policy and sandbox. a request that arrives anyway fails the
// test rather than being answered.
func probeTurn(t *testing.T, svc *service.Service, projectID, harnessName string, choice service.LaunchChoice) map[string]string {
	t.Helper()
	conv, err := svc.StartConversation(projectID, harnessName, "environment", envPrompt, choice)
	if err != nil {
		t.Fatal(err)
	}
	s := &session{t: t, svc: svc, id: conv.ID}
	deadline := time.Now().Add(turnTimeout)
	for {
		v := s.view()
		if !v.TurnActive && v.State == service.StateRunning {
			break
		}
		if len(v.Pending) > 0 {
			t.Fatalf("%s asked for approval, which this test does not give: %+v", harnessName, v.Pending)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out; view = %+v", v)
		}
		time.Sleep(250 * time.Millisecond)
	}
	evs, err := svc.Events(conv.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, ev := range evs {
		if ev.Kind != model.EventToolResult || !strings.Contains(ev.Text, "gopath=") {
			continue
		}
		for _, line := range strings.Split(ev.Text, "\n") {
			if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
				got[k] = v
			}
		}
	}
	var facts *model.EnvironmentFacts
	for _, ev := range evs {
		if ev.Kind == model.EventRuntime && ev.Status == model.StatusStarted && ev.Facts != nil && ev.Facts.Environment != nil {
			facts = ev.Facts.Environment
		}
	}
	if facts == nil || !slices.Contains(facts.Changed, "GOPATH") || !slices.Contains(facts.Changed, "PATH") || facts.Source != model.EnvSourceFile {
		t.Errorf("%s: the start's environment: %+v", harnessName, facts)
	}
	if err := svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	s.waitFor("the release", func(v service.ConversationView) bool { return v.State == model.RuntimeNone })
	return got
}

// TestEachProjectRunsInItsOwnEnvironment gives each fixture a scratch .devprj
// with its own Go path, and a .mega/environment.yaml naming devprj, approved
// through the service in this test's own scratch workspace. a Claude turn and
// a Codex turn then print their Go path and where its bin sits on PATH: each
// matches its own project, and neither matches the other's or the Go path
// this test (mega) runs in. it needs devprj from the tools repository on the
// interactive shell's PATH, and costs one turn of each harness.
func TestEachProjectRunsInItsOwnEnvironment(t *testing.T) {
	claudeFixture := fixture(t, "MEGA_E2E_CLAUDE_FIXTURE")
	codexFixture := fixture(t, "MEGA_E2E_CODEX_FIXTURE")
	gopaths := t.TempDir()
	for _, fx := range []string{claudeFixture, codexFixture} {
		name := filepath.Base(fx)
		// registered first so it runs last, once the files placed below are
		// put back; it removes .mega only if it is then empty.
		t.Cleanup(func() { _ = os.Remove(filepath.Join(fx, ".mega")) })
		placeFile(t, filepath.Join(fx, ".devprj"), "name: "+name+"\nset:\n  GOPATH: "+filepath.Join(gopaths, name)+"\nprepend:\n  PATH:\n    - "+filepath.Join(gopaths, name, "bin")+"\n", 0o644)
		placeFile(t, filepath.Join(fx, ".mega", "environment.yaml"), "activate: devprj\n", 0o644)
		placeFile(t, filepath.Join(fx, "envprobe.sh"), envProbe, 0o755)
		if fx == claudeFixture {
			// the fixture's own narrow allow rule for exactly the probe, in its
			// local settings, as the capability probes declare theirs.
			placeFile(t, filepath.Join(fx, ".claude", "settings.local.json"), "{\"permissions\": {\"allow\": [\"Bash(./envprobe.sh)\"]}}\n", 0o644)
		}
	}

	data := t.TempDir()
	reg, err := registry.Load(filepath.Join(data, "registry.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := service.New(reg, service.Options{
		EventLogPath: func(id string) string { return filepath.Join(data, "conversations", id, "events.jsonl") },
		Harnesses:    []harness.Harness{newClaudeHarness(t), codex.New(codex.Config{Binary: env("MEGA_E2E_CODEX", "codex")})},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Close)
	register := func(name, checkout string) model.Project {
		t.Helper()
		p, err := svc.RegisterProject(name, checkout)
		if err != nil {
			t.Fatal(err)
		}
		// approved in this scratch workspace, at the fingerprint it shows.
		if _, err := svc.ApproveEnvironment(p.ID, svc.Workspace().Environments[p.ID].File.Digest); err != nil {
			t.Fatal(err)
		}
		return p
	}
	claudeProject := register("claude-rt1", claudeFixture)
	codexProject := register("codex-rt1", codexFixture)

	claudeGot := probeTurn(t, svc, claudeProject.ID, model.HarnessClaude, service.LaunchChoice{Model: env("MEGA_E2E_CLAUDE_MODEL", "haiku")})
	codexGot := probeTurn(t, svc, codexProject.ID, model.HarnessCodex, service.LaunchChoice{})
	t.Logf("claude: %v", claudeGot)
	t.Logf("codex: %v", codexGot)

	own := map[string]string{
		"claude": filepath.Join(gopaths, filepath.Base(claudeFixture)),
		"codex":  filepath.Join(gopaths, filepath.Base(codexFixture)),
	}
	megas := os.Getenv("GOPATH")
	for name, got := range map[string]map[string]string{"claude": claudeGot, "codex": codexGot} {
		other := own["codex"]
		if name == "codex" {
			other = own["claude"]
		}
		if got["gopath"] != own[name] {
			t.Errorf("%s ran with Go path %q, want its own %q", name, got["gopath"], own[name])
		}
		if got["gopath"] == other || (megas != "" && got["gopath"] == megas) {
			t.Errorf("%s ran with another's Go path %q", name, got["gopath"])
		}
		at, _ := strconv.Atoi(got["gobin_at"])
		if at == 0 {
			t.Errorf("%s: its own bin is not on PATH", name)
		}
		if got["path1"] == filepath.Join(other, "bin") || (megas != "" && got["path1"] == filepath.Join(megas, "bin")) {
			t.Errorf("%s: the first PATH entry %q is another's bin", name, got["path1"])
		}
	}
	// claude keeps the launch's PATH order: its own bin comes first. codex
	// runs each command in a login shell, whose profile and codex's own
	// helper directory come first (capability report, 2026-11-12).
	if claudeGot["path1"] != filepath.Join(own["claude"], "bin") {
		t.Errorf("claude's first PATH entry is %q, want its own bin", claudeGot["path1"])
	}
}

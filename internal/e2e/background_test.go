//go:build e2e

package e2e

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/harness/codex"
	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/service"
)

// background jobs: work a runtime keeps running after its turn ends, which
// the adapter reports whole (Sink.Background) and the page shows beside the
// timeline. each round trip starts one job that ends by itself, sees it
// reported, and sees the list empty again (capability report, 2026-11-10).

// turnEnds counts the conversation's turn ends.
func (s *session) turnEnds() int {
	s.t.Helper()
	evs, err := s.svc.Events(s.id, 0)
	if err != nil {
		s.t.Fatal(err)
	}
	n := 0
	for _, ev := range evs {
		if ev.Kind == model.EventTurnEnd {
			n++
		}
	}
	return n
}

// bgProbe is the command the Claude background round trip backgrounds: it
// holds for ten seconds and ends by itself.
const bgProbe = "#!/bin/sh\nsleep 10\necho BG-PROBE-DONE\n"

// TestClaudeBackgroundJob backgrounds ./bgprobe.sh, which a narrow allow rule
// of the test's own lets run: for its life the test places the script in the
// fixture with a .claude/settings.local.json allowing exactly
// Bash(./bgprobe.sh), as the environment round trip does, and puts back what
// those paths held. it answers nothing: a request fails the test unanswered.
// the job is reported while it runs and the list empties once it ends.
// claude then starts a turn of its own for the job's completion (2.1.318),
// which is waited for so that it ends before the shutdown, and is said, not
// required.
func TestClaudeBackgroundJob(t *testing.T) {
	fixture := fixture(t, "MEGA_E2E_CLAUDE_FIXTURE")
	placeFile(t, filepath.Join(fixture, "bgprobe.sh"), bgProbe, 0o755)
	placeFile(t, filepath.Join(fixture, ".claude", "settings.local.json"), "{\"permissions\": {\"allow\": [\"Bash(./bgprobe.sh)\"]}}\n", 0o644)
	svc := newE2EService(t, mustNewClaudeHarness(t))
	project, err := svc.RegisterProject("claude-rt1", fixture)
	if err != nil {
		t.Fatal(err)
	}
	conv, err := svc.StartConversation(project.ID, model.HarnessClaude, "e2e background", "Using the Bash tool with run_in_background set to true, run exactly `./bgprobe.sh`. Do not wait for it. Reply BG-STARTED and stop. Nothing else.", service.LaunchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{t: t, svc: svc, id: conv.ID}
	view := s.waitFor("the job to be reported", func(v service.ConversationView) bool { return len(v.Background) > 0 || len(v.Pending) > 0 })
	if len(view.Pending) > 0 {
		t.Fatalf("claude asked for approval, which this test does not give: %+v", view.Pending)
	}
	t.Logf("%d background jobs reported", len(view.Background))
	s.waitFor("the job to end", func(v service.ConversationView) bool { return len(v.Background) == 0 })
	// the first turn, and the one the completion may wake.
	deadline := time.Now().Add(2 * time.Minute)
	for s.turnEnds() < 2 && time.Now().Before(deadline) {
		time.Sleep(250 * time.Millisecond)
	}
	s.waitFor("no turn to be open", func(v service.ConversationView) bool { return !v.TurnActive })
	if len(s.view().Pending) > 0 {
		t.Fatalf("claude asked for approval, which this test does not give: %+v", s.view().Pending)
	}
	t.Logf("turns ended: %d (the second is the one the job's completion woke)", s.turnEnds())
	shutdown(t, svc, s)
}

// TestCodexBackgroundJob runs a command that outlives its turn, as a
// terminal session codex keeps. no approval is expected: it runs inside the
// checkout under the operator's own sandbox, and a request fails the test
// unanswered. the command is reported while it runs past its turn; when it
// ends, its result is recorded, the list empties, and no turn opens.
func TestCodexBackgroundJob(t *testing.T) {
	fixture := fixture(t, "MEGA_E2E_CODEX_FIXTURE")
	svc := newE2EService(t, codex.New(codex.Config{Binary: env("MEGA_E2E_CODEX", "codex")}))
	project, err := svc.RegisterProject("codex-rt1", fixture)
	if err != nil {
		t.Fatal(err)
	}
	conv, err := svc.StartConversation(project.ID, model.HarnessCodex, "e2e background", "Run this exact command directly, with no nohup, no & and no redirection: `sleep 20 && echo BG-CODEX-DONE`. Use a short yield time (about one second) so the tool returns while the command is still running as a terminal session, and do not poll or wait for it. Then reply BG-STARTED immediately. Nothing else.", service.LaunchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{t: t, svc: svc, id: conv.ID}
	view := s.waitFor("the turn to end with the job running", func(v service.ConversationView) bool {
		return len(v.Pending) > 0 || (v.State == service.StateRunning && s.settled(v))
	})
	if len(view.Pending) > 0 {
		t.Fatalf("codex asked for approval, which this test does not give: %+v", view.Pending)
	}
	if len(view.Background) == 0 {
		t.Fatalf("the turn ended with no background job reported; the reply was %q", s.lastAssistant())
	}
	t.Logf("%d background jobs reported after the turn", len(view.Background))
	ended := s.turnEnds()
	s.waitFor("the job to end", func(v service.ConversationView) bool { return len(v.Background) == 0 })
	// the late result arrives outside any turn and opens none.
	time.Sleep(3 * time.Second)
	if v := s.view(); v.TurnActive || s.turnEnds() != ended {
		t.Errorf("the job's end opened a turn: turn %v, turn ends %d, were %d", v.TurnActive, s.turnEnds(), ended)
	}
	evs, err := svc.Events(s.id, 0)
	if err != nil {
		t.Fatal(err)
	}
	var result bool
	for _, ev := range evs {
		result = result || (ev.Kind == model.EventToolResult && strings.Contains(ev.Text, "BG-CODEX-DONE"))
	}
	if !result {
		t.Error("the job's result was not recorded")
	}
	shutdown(t, svc, s)
}

//go:build e2e

package e2e

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michaelquigley/mega/internal/harness/codex"
	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/service"
)

// an operator's Deny that reaches the harness: the request is asserted, Deny
// is sent through the HTTP API a browser uses, the tool does not run, and the
// turn ends as the harness ends it, whatever that is. each prompt asks the
// model not to try again, and any request after the denied one fails the
// test unanswered.

// denied waits for the turn the denied request belonged to to end, failing
// unanswered on any request that follows it, and returns how it ended.
func (s *session) denied(after int64) string {
	s.t.Helper()
	view := s.waitFor("the turn after the deny", func(v service.ConversationView) bool {
		return len(v.Pending) > 0 || (v.State == service.StateRunning && s.settled(v))
	})
	if len(view.Pending) > 0 {
		s.t.Fatalf("another request followed the deny: %+v; it was not answered", view.Pending)
	}
	evs, err := s.svc.Events(s.id, after)
	if err != nil {
		s.t.Fatal(err)
	}
	for _, ev := range evs {
		if ev.Seq > after && ev.Kind == model.EventTurnEnd {
			return ev.Status
		}
	}
	s.t.Fatal("no turn ended after the deny")
	return ""
}

// TestClaudeDeny: the fixture's ask rule for Bash(sleep *) raises the
// approval; Deny is sent, and the file the command would make is never made.
func TestClaudeDeny(t *testing.T) {
	fixture := fixture(t, "MEGA_E2E_CLAUDE_FIXTURE")
	check := filepath.Join(fixture, "claude-deny-check.txt")
	_ = os.Remove(check)
	t.Cleanup(func() { _ = os.Remove(check) })
	svc := newE2EService(t, mustNewClaudeHarness(t))
	project, err := svc.RegisterProject("claude-rt1", fixture)
	if err != nil {
		t.Fatal(err)
	}
	const command = "sleep 1 && touch claude-deny-check.txt"
	conv, err := svc.StartConversation(project.ID, model.HarnessClaude, "e2e deny", "Using the Bash tool, run exactly `"+command+"` and then reply RAN-3f1a. If the command is not approved, run nothing else and reply DENIED-3f1a. Nothing else.", service.LaunchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{t: t, svc: svc, id: conv.ID}
	approval := s.expectApproval(approvalWant{tool: "Bash", command: command})
	asked := s.seq()
	if code := s.answer(approval.ID, "deny"); code != "" {
		t.Fatalf("the deny was refused: %s", code)
	}
	ended := s.denied(asked)
	if _, err := os.Stat(check); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the denied command ran: %v", err)
	}
	t.Logf("after the deny the turn ended %s; the reply names DENIED-3f1a: %v", ended, strings.Contains(s.lastAssistant(), "DENIED-3f1a"))
	shutdown(t, svc, s)
}

// TestCodexDeny: a write outside the checkout raises the approval under the
// operator's own policy and sandbox; Deny is sent, and nothing is written.
func TestCodexDeny(t *testing.T) {
	fixture := fixture(t, "MEGA_E2E_CODEX_FIXTURE")
	check := filepath.Join(filepath.Dir(fixture), "codex-rt1-deny.txt")
	_ = os.Remove(check)
	t.Cleanup(func() { _ = os.Remove(check) })
	svc := newE2EService(t, codex.New(codex.Config{Binary: env("MEGA_E2E_CODEX", "codex")}))
	project, err := svc.RegisterProject("codex-rt1", fixture)
	if err != nil {
		t.Fatal(err)
	}
	const command = "echo DENY-4d1e > ../codex-rt1-deny.txt"
	conv, err := svc.StartConversation(project.ID, model.HarnessCodex, "e2e deny", "Run exactly this shell command, and no other: "+command+" (it writes in the parent of the working directory). Then reply with DENY-4d1e. If the command is not approved, run nothing else and reply DENIED-4d1e. Nothing else.", service.LaunchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{t: t, svc: svc, id: conv.ID}
	approval := s.expectApproval(approvalWant{tool: "shell", command: command})
	asked := s.seq()
	if code := s.answer(approval.ID, "deny"); code != "" {
		t.Fatalf("the deny was refused: %s", code)
	}
	ended := s.denied(asked)
	if _, err := os.Stat(check); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the denied command ran: %v", err)
	}
	t.Logf("after the deny the turn ended %s; the reply names DENIED-4d1e: %v", ended, strings.Contains(s.lastAssistant(), "DENIED-4d1e"))
	shutdown(t, svc, s)
}

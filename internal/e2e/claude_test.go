//go:build e2e

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.hq.quigley.com/research/roundhouse/internal/harness"
	"git.hq.quigley.com/research/roundhouse/internal/harness/claude"
	"git.hq.quigley.com/research/roundhouse/internal/model"
	"git.hq.quigley.com/research/roundhouse/internal/registry"
	"git.hq.quigley.com/research/roundhouse/internal/service"
)

const (
	defaultFixture = "/home/michael/Repos/q/research/roundhouse-fixtures/claude-rt1"
	turnTimeout    = 4 * time.Minute

	markerPrompt   = "Reply with the project instruction marker token from your project instructions, followed by the word ALPHA-1a2b. Nothing else."
	questionPrompt = "Use the AskUserQuestion tool to ask me a single question: which color do you prefer? Offer exactly two options, Red and Blue. After I answer, reply with my chosen color followed by GAMMA-5e6f. Nothing else."
	approvalPrompt = "Using the Bash tool, run exactly `sleep 3` and then reply DELTA-7a8b. Nothing else."
	returnPrompt   = "List every marker token you have seen in this conversation so far, in order, one per line, then append SDK-RETURN-c3d4 on its own line. Nothing else."
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type session struct {
	t   *testing.T
	svc *service.Service
	id  string
}

func (s *session) view() service.ConversationView {
	for _, conv := range s.svc.Workspace().Conversations {
		if conv.ID == s.id {
			return conv
		}
	}
	s.t.Fatalf("conversation %s is gone", s.id)
	return service.ConversationView{}
}

func (s *session) waitFor(what string, cond func(service.ConversationView) bool) service.ConversationView {
	s.t.Helper()
	deadline := time.Now().Add(turnTimeout)
	for {
		view := s.view()
		if cond(view) {
			return view
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("timed out waiting for %s; view = %+v", what, view)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func (s *session) idle() {
	s.t.Helper()
	s.waitFor("the turn to end", func(v service.ConversationView) bool { return !v.TurnActive && v.State == service.StateRunning })
}

func (s *session) lastAssistant() string {
	s.t.Helper()
	evs, err := s.svc.Events(s.id, 0)
	if err != nil {
		s.t.Fatal(err)
	}
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Kind == model.EventAssistant {
			return evs[i].Text
		}
	}
	return ""
}

func (s *session) expectReply(contains ...string) {
	s.t.Helper()
	reply := s.lastAssistant()
	for _, want := range contains {
		if !strings.Contains(reply, want) {
			s.t.Errorf("reply %q lacks %q", reply, want)
		}
	}
}

func (s *session) prompt(text string) {
	s.t.Helper()
	if err := s.svc.Prompt(s.id, text); err != nil {
		s.t.Fatal(err)
	}
}

// TestClaudeRoundTrip is C1 to C4 and C6 from the capability report through the
// service, without the terminal visit (C5), which the operator does by hand.
func TestClaudeRoundTrip(t *testing.T) {
	fixture := env("ROUNDHOUSE_E2E_CLAUDE_FIXTURE", defaultFixture)
	shim, err := filepath.Abs(filepath.Join("..", "..", "shim", "claude"))
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	h := claude.New(claude.Config{
		Node:       env("ROUNDHOUSE_E2E_NODE", "node"),
		Shim:       shim,
		Executable: env("ROUNDHOUSE_E2E_CLAUDE", filepath.Join(home, ".local", "share", "claude", "versions", "2.1.318")),
	})
	svc := newE2EService(t, h)
	project, err := svc.RegisterProject("claude-rt1", fixture)
	if err != nil {
		t.Fatal(err)
	}

	// C1: launch with the marker turn; project instructions load.
	conv, err := svc.StartConversation(project.ID, model.HarnessClaude, "e2e", markerPrompt)
	if err != nil {
		t.Fatal(err)
	}
	s := &session{t: t, svc: svc, id: conv.ID}
	s.idle()
	s.expectReply("ROUNDHOUSE-MARKER-7f3a", "ALPHA-1a2b")
	if facts := s.view().Facts; facts == nil || facts.PermissionMode != "default" || facts.Session != conv.NativeID {
		t.Errorf("runtime facts = %+v, bound to %s", facts, conv.NativeID)
	}

	// C2: AskUserQuestion reaches the operator and the answer returns.
	s.prompt(questionPrompt)
	view := s.waitFor("the question", func(v service.ConversationView) bool { return len(v.Pending) > 0 })
	question := view.Pending[0]
	if question.Kind != model.RequestQuestion {
		t.Fatalf("expected a question, got %+v", question)
	}
	if err := svc.Answer(s.id, question.ID, harness.Answer{Answers: map[string]string{"Which color do you prefer?": "Blue"}}); err != nil {
		t.Fatal(err)
	}
	s.idle()
	s.expectReply("Blue", "GAMMA-5e6f")

	// C3: a real permission request waits for a delayed approval.
	s.prompt(approvalPrompt)
	view = s.waitFor("the approval", func(v service.ConversationView) bool { return len(v.Pending) > 0 })
	approval := view.Pending[0]
	if approval.Kind != model.RequestPermission {
		t.Fatalf("expected a permission request, got %+v", approval)
	}
	time.Sleep(4 * time.Second)
	if s.view().TurnActive == false {
		t.Error("the turn ended while the approval was pending")
	}
	if err := svc.Answer(s.id, approval.ID, harness.Answer{Decision: harness.DecisionAllow}); err != nil {
		t.Fatal(err)
	}
	s.idle()
	s.expectReply("DELTA-7a8b")
	if err := svc.Answer(s.id, approval.ID, harness.Answer{Decision: harness.DecisionAllow}); service.ErrorCode(err) != service.CodeNotPending {
		t.Errorf("late answer: %v", err)
	}

	// C4: release, and the runtime is seen to exit.
	if err := svc.Release(s.id); err != nil {
		t.Fatal(err)
	}
	s.waitFor("the runtime to exit", func(v service.ConversationView) bool { return v.State == model.RuntimeNone })

	// import after a release with no terminal visit adds nothing: every managed
	// message, including the prompts the service minted, matches by identity.
	before, err := svc.Events(s.id, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Resume(s.id); err != nil {
		t.Fatal(err)
	}
	after, err := svc.Events(s.id, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range after[len(before):] {
		if ev.Kind == model.EventRuntime {
			continue
		}
		t.Errorf("import added a managed message again: %s %s %q", ev.Kind, ev.NativeID, ev.Text)
	}
	if last := after[len(after)-1]; last.Status != model.StatusResumed || last.Text != "0 messages added from native history" {
		t.Errorf("resume ended with %s %q", last.Status, last.Text)
	}
	recorded := make(map[string]bool)
	for _, ev := range after {
		recorded[ev.NativeID] = true
	}
	history, err := h.History(context.Background(), fixture, conv.NativeID)
	if err != nil {
		t.Fatal(err)
	}
	var prompts int
	for _, ev := range history {
		if !recorded[ev.NativeID] {
			t.Errorf("native message %s %s %q is not in the log", ev.Kind, ev.NativeID, ev.Text)
		}
		if ev.Kind == model.EventUser {
			prompts++
		}
	}
	if len(history) == 0 || prompts != 3 {
		t.Errorf("history has %d events and %d prompts, want the 3 managed prompts", len(history), prompts)
	}

	// C6: the next prompt resumes the exact session and remembers everything.
	s.prompt(returnPrompt)
	s.idle()
	s.expectReply("ROUNDHOUSE-MARKER-7f3a", "ALPHA-1a2b", "GAMMA-5e6f", "DELTA-7a8b", "SDK-RETURN-c3d4")
	if facts := s.view().Facts; facts == nil || facts.Session != conv.NativeID {
		t.Errorf("resumed session = %+v, bound to %s", facts, conv.NativeID)
	}

	shutdown(t, svc, s)
	t.Logf("session %s", conv.NativeID)
}

// newE2EService is a service over h with a throwaway data directory.
func newE2EService(t *testing.T, h harness.Harness) *service.Service {
	t.Helper()
	data := t.TempDir()
	reg, err := registry.Load(filepath.Join(data, "registry.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := service.New(reg, service.Options{
		EventLogPath: func(id string) string { return filepath.Join(data, "conversations", id, "events.jsonl") },
		Harnesses:    []harness.Harness{h},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Close)
	return svc
}

// shutdown releases every runtime the way serve does and checks each was seen
// to stop.
func shutdown(t *testing.T, svc *service.Service, s *session) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	svc.Shutdown(ctx)
	if v := s.view(); v.Runtime != model.RuntimeNone {
		t.Errorf("after shutdown the conversation records %q", v.Runtime)
	}
}

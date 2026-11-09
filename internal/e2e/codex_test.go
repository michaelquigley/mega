//go:build e2e

package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"git.hq.quigley.com/research/roundhouse/internal/harness"
	"git.hq.quigley.com/research/roundhouse/internal/harness/codex"
	"git.hq.quigley.com/research/roundhouse/internal/model"
	"git.hq.quigley.com/research/roundhouse/internal/service"
)

const defaultCodexFixture = "/home/michael/Repos/q/research/roundhouse-fixtures/codex-rt1"

// TestCodexDiscoveryAndHistory reads the fixture's stored threads without a
// model turn: discovery through thread/list, and history through thread/read.
func TestCodexDiscoveryAndHistory(t *testing.T) {
	fixture := env("ROUNDHOUSE_E2E_CODEX_FIXTURE", defaultCodexFixture)
	h := codex.New(codex.Config{Binary: env("ROUNDHOUSE_E2E_CODEX", "codex")})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	sessions, err := h.Discover(ctx, fixture)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) == 0 {
		t.Fatal("no threads discovered in the fixture")
	}
	for _, s := range sessions[:min(3, len(sessions))] {
		t.Logf("thread %s updated %s: %.60q", s.NativeID, s.UpdatedAt.Format(time.RFC3339), s.Preview)
	}
	history, err := h.History(ctx, fixture, sessions[0].NativeID)
	if err != nil {
		t.Fatal(err)
	}
	evs := history.Events
	counts := map[string]int{}
	seen := map[string]bool{}
	for _, ev := range evs {
		counts[ev.Kind]++
		if ev.NativeID == "" || seen[ev.NativeID] || ev.Origin != model.OriginNative {
			t.Errorf("history event %s has native id %q (seen %v), origin %q", ev.Kind, ev.NativeID, seen[ev.NativeID], ev.Origin)
		}
		seen[ev.NativeID] = true
	}
	t.Logf("history of %s: %v", sessions[0].NativeID, counts)
	if counts[model.EventUser] == 0 || counts[model.EventAssistant] == 0 {
		t.Errorf("history lacks user or assistant messages: %v", counts)
	}
}

const (
	codexMarkerPrompt = "Reply with the project instruction marker token from your project instructions (AGENTS.md), followed by the word ALPHA-1a2b. Nothing else."
	// the fixture is trusted in the operator's codex config, which gives it a
	// workspace-write sandbox, so only a write outside the checkout needs approval.
	codexApprovalPrompt = "Using a shell command, create the file ../codex-rt1-approval.txt (in the parent of the working directory) containing the single line GAMMA-5e6f. Then reply with GAMMA-5e6f. Nothing else."
	codexQuestionPrompt = "Before doing anything else, ask me a question using your user-input question mechanism: which color do I prefer, Red or Blue? Wait for my answer. Then reply with my chosen color followed by STEER-GAMMA-5e6f. Nothing else."
	codexReturnPrompt   = "List every marker token you have seen in this conversation so far, in order, one per line, then append RESUME-SDK-c3d4 on its own line. Nothing else."
)

// TestCodexRoundTrip is X1 to X4 and an X7-style resumed turn through the
// service, without the terminal visit (X5), which the operator does by hand.
func TestCodexRoundTrip(t *testing.T) {
	fixture := env("ROUNDHOUSE_E2E_CODEX_FIXTURE", defaultCodexFixture)
	h := codex.New(codex.Config{Binary: env("ROUNDHOUSE_E2E_CODEX", "codex")})
	svc := newE2EService(t, h)
	project, err := svc.RegisterProject("codex-rt1", fixture)
	if err != nil {
		t.Fatal(err)
	}

	// X1: a new thread; AGENTS.md loads; the operator's approval policy and
	// sandbox are inherited.
	conv, err := svc.StartConversation(project.ID, model.HarnessCodex, "e2e", codexMarkerPrompt)
	if err != nil {
		t.Fatal(err)
	}
	s := &session{t: t, svc: svc, id: conv.ID}
	s.idle()
	s.expectReply("ROUNDHOUSE-MARKER-7f3a", "ALPHA-1a2b")
	facts := s.view().Facts
	if facts == nil || facts.Session != conv.NativeID || !strings.Contains(facts.PermissionMode, "on-request") {
		t.Errorf("runtime facts = %+v, bound to %s", facts, conv.NativeID)
	}

	// X2: a genuine approval waits for a delayed answer.
	s.prompt(codexApprovalPrompt)
	view := s.waitFor("the approval", func(v service.ConversationView) bool { return len(v.Pending) > 0 })
	approval := view.Pending[0]
	if approval.Kind != model.RequestPermission {
		t.Fatalf("expected a permission request, got %+v", approval)
	}
	time.Sleep(4 * time.Second)
	if !s.view().TurnActive {
		t.Error("the turn ended while the approval was pending")
	}
	if err := svc.Answer(s.id, approval.ID, harness.Answer{Decision: harness.DecisionAllow}); err != nil {
		t.Fatal(err)
	}
	s.idle()
	s.expectReply("GAMMA-5e6f")

	// X3: the default-mode async question is a request, answered into its turn.
	s.prompt(codexQuestionPrompt)
	view = s.waitFor("the question", func(v service.ConversationView) bool { return len(v.Pending) > 0 })
	question := view.Pending[0]
	payload, _ := question.Payload.(map[string]any)
	questions, _ := payload["questions"].([]any)
	if question.Kind != model.RequestQuestion || len(questions) == 0 {
		t.Fatalf("expected a question, got %+v", question)
	}
	title, _ := questions[0].(map[string]any)["question"].(string)
	if err := svc.Answer(s.id, question.ID, harness.Answer{Answers: map[string]string{title: "Blue"}}); err != nil {
		t.Fatal(err)
	}
	s.idle()
	s.expectReply("Blue", "STEER-GAMMA-5e6f")

	// release, seen to exit with nothing left running.
	if err := svc.Release(s.id); err != nil {
		t.Fatal(err)
	}
	s.waitFor("the runtime to exit", func(v service.ConversationView) bool { return v.State == model.RuntimeNone })
	evs, err := svc.Events(s.id, 0)
	if err != nil {
		t.Fatal(err)
	}
	if last := evs[len(evs)-1]; last.Status != model.StatusStopped {
		t.Errorf("release ended with %s %q", last.Status, last.Text)
	}

	// X4: import after a release with no terminal visit adds nothing; every
	// managed message, including the prompts the service minted, matches.
	if err := svc.Resume(s.id); err != nil {
		t.Fatal(err)
	}
	after, err := svc.Events(s.id, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range after[len(evs):] {
		if ev.Kind != model.EventRuntime {
			t.Errorf("import added a managed message again: %s %s %q", ev.Kind, ev.NativeID, ev.Text)
		}
	}
	recorded := make(map[string]bool)
	for _, ev := range after {
		recorded[ev.NativeID] = true
	}
	history, err := h.History(context.Background(), fixture, conv.NativeID)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range history.Events {
		if !recorded[ev.NativeID] {
			t.Errorf("native message %s %s %q is not in the log", ev.Kind, ev.NativeID, ev.Text)
		}
	}

	// X7-style: the next prompt resumes the exact thread and remembers.
	s.prompt(codexReturnPrompt)
	s.idle()
	s.expectReply("ROUNDHOUSE-MARKER-7f3a", "ALPHA-1a2b", "GAMMA-5e6f", "RESUME-SDK-c3d4")
	if facts := s.view().Facts; facts == nil || facts.Session != conv.NativeID {
		t.Errorf("resumed thread = %+v, bound to %s", facts, conv.NativeID)
	}
	shutdown(t, svc, s)
	t.Logf("thread %s", conv.NativeID)
}

// TestCodexReleaseDuringCommand releases while a sandboxed command runs. the
// capability report saw the sandbox child outlive the app-server after an
// interrupt; the release must report stopped only if the app-server's process
// group emptied, and otherwise name what is still running.
func TestCodexReleaseDuringCommand(t *testing.T) {
	fixture := env("ROUNDHOUSE_E2E_CODEX_FIXTURE", defaultCodexFixture)
	h := codex.New(codex.Config{Binary: env("ROUNDHOUSE_E2E_CODEX", "codex")})
	svc := newE2EService(t, h)
	project, err := svc.RegisterProject("codex-rt1", fixture)
	if err != nil {
		t.Fatal(err)
	}
	conv, err := svc.StartConversation(project.ID, model.HarnessCodex, "e2e release", `Run exactly python3 -c "import time; time.sleep(40); print(1)" in the working directory and then reply SLEPT. Nothing else.`)
	if err != nil {
		t.Fatal(err)
	}
	s := &session{t: t, svc: svc, id: conv.ID}
	// the command is recorded only when it completes; give the model time to
	// start it, then release while it runs.
	time.Sleep(15 * time.Second)
	if !s.view().TurnActive {
		t.Fatal("the turn ended before the release")
	}
	if err := svc.Release(s.id); err != nil {
		t.Fatal(err)
	}
	s.waitFor("the runtime to exit", func(v service.ConversationView) bool { return v.State == model.RuntimeNone })
	evs, err := svc.Events(s.id, 0)
	if err != nil {
		t.Fatal(err)
	}
	last := evs[len(evs)-1]
	t.Logf("release ended with %s %q", last.Status, last.Text)
	if last.Status != model.StatusStopped && !strings.Contains(last.Text, "left processes running") {
		t.Errorf("unexpected end: %+v", last)
	}
}

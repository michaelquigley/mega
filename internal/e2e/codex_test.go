//go:build e2e

package e2e

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/harness/codex"
	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/service"
)

const (
	codexMarkerPrompt = "Reply with the project instruction marker token from your project instructions (AGENTS.md), followed by the word ALPHA-1a2b. Nothing else."
	// the fixture is trusted in the operator's codex config, which gives it a
	// workspace-write sandbox, so only a write outside the checkout needs approval.
	codexApprovalPrompt = "Run exactly this shell command, and no other: echo GAMMA-5e6f > ../codex-rt1-approval.txt (it writes in the parent of the working directory). Then reply with GAMMA-5e6f. Nothing else."
	// the one command the approval round trip expects to be asked about.
	codexApprovalCommand = "echo GAMMA-5e6f > ../codex-rt1-approval.txt"
	// the command the interrupted approval asks about, never answered.
	codexInterruptCommand = "echo EPSILON-2b9c > ../codex-rt1-interrupt.txt"
	codexInterruptPrompt  = "Run exactly this shell command, and no other: " + codexInterruptCommand + " (it writes in the parent of the working directory). Then reply with EPSILON-2b9c. Nothing else."
	codexQuestionPrompt   = "Before doing anything else, ask me a question using your user-input question mechanism: which color do I prefer, Red or Blue? Wait for my answer. Then reply with my chosen color followed by STEER-GAMMA-5e6f. Nothing else."
	codexReturnPrompt     = "List every marker token you have seen in this conversation so far, in order, one per line, then append RESUME-SDK-c3d4 on its own line. Nothing else."
)

// TestCodexRoundTrip is X1 to X4 and an X7-style resumed turn through the
// service, without the terminal visit (X5), which the operator does by hand,
// with an interrupt while a second approval waits, and the thread then
// discovered and adopted again, its history read: everything it reads is
// the thread it made, never one another run left in the fixture.
//
// the approval round trip is what X2 tests: a genuine command approval
// reaches the operator, the turn waits on it, and the command runs only after
// the answer. the test plays the operator for exactly that request (shell,
// `echo GAMMA-5e6f > ../codex-rt1-approval.txt`), asserted before it answers,
// through the HTTP API a browser uses; anything else fails the test
// unanswered.
func TestCodexRoundTrip(t *testing.T) {
	fixture := fixture(t, "MEGA_E2E_CODEX_FIXTURE")
	h := codex.New(codex.Config{Binary: env("MEGA_E2E_CODEX", "codex")})
	svc := newE2EService(t, h)
	project, err := svc.RegisterProject("codex-rt1", fixture)
	if err != nil {
		t.Fatal(err)
	}

	// what the approved and the interrupted commands write, beside the
	// checkout.
	approvalFile := filepath.Join(filepath.Dir(fixture), "codex-rt1-approval.txt")
	interruptFile := filepath.Join(filepath.Dir(fixture), "codex-rt1-interrupt.txt")
	for _, f := range []string{approvalFile, interruptFile} {
		_ = os.Remove(f)
		t.Cleanup(func() { _ = os.Remove(f) })
	}

	// X1: a new thread; AGENTS.md loads; the operator's approval policy and
	// sandbox are inherited. the service holds no other conversation, so
	// every delta is this one's.
	count := deltas(svc, "")
	conv, err := svc.StartConversation(project.ID, model.HarnessCodex, "e2e", codexMarkerPrompt, service.LaunchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{t: t, svc: svc, id: conv.ID}
	s.idle()
	s.expectDeltas(count, "the marker turn")
	s.expectReply("ROUNDHOUSE-MARKER-7f3a", "ALPHA-1a2b")
	facts := s.view().Facts
	if facts == nil || facts.Session != conv.NativeID || !strings.Contains(facts.PermissionMode, "on-request") {
		t.Errorf("runtime facts = %+v, bound to %s", facts, conv.NativeID)
	}
	s.mcpReported()

	// X2: a genuine approval waits for a delayed answer, and nothing is
	// written until it comes.
	s.prompt(codexApprovalPrompt)
	approval := s.expectApproval(approvalWant{tool: "shell", command: codexApprovalCommand})
	time.Sleep(4 * time.Second)
	if !s.view().TurnActive {
		t.Error("the turn ended while the approval was pending")
	}
	if _, err := os.Stat(approvalFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the command wrote before Allow: %v", err)
	}
	if code := s.answer(approval.ID, "allow"); code != "" {
		t.Fatalf("the answer was refused: %s", code)
	}
	s.idle()
	s.expectReply("GAMMA-5e6f")
	if written, err := os.ReadFile(approvalFile); err != nil || !strings.Contains(string(written), "GAMMA-5e6f") {
		t.Errorf("the approved command wrote %q (%v)", written, err)
	}

	// an interrupt with a second approval pending, never answered: the turn
	// ends interrupted, the request aborted, and nothing is written.
	asked := s.seq()
	s.prompt(codexInterruptPrompt)
	second := s.expectApproval(approvalWant{tool: "shell", command: codexInterruptCommand})
	if err := svc.Interrupt(s.id); err != nil {
		t.Fatal(err)
	}
	s.waitFor("the interrupted turn to end", func(v service.ConversationView) bool { return !v.TurnActive && len(v.Pending) == 0 })
	s.endedInterrupted(asked, second.ID)
	time.Sleep(2 * time.Second)
	if _, err := os.Stat(interruptFile); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the interrupted command wrote: %v", err)
	}

	// X3: the default-mode async question is a request, answered into its turn.
	s.prompt(codexQuestionPrompt)
	view := s.waitFor("the question", func(v service.ConversationView) bool { return len(v.Pending) > 0 })
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

	// X4: every message in the thread's own history is in the log.
	recorded := make(map[string]bool)
	for _, ev := range evs {
		recorded[ev.NativeID] = true
	}
	history, err := h.History(context.Background(), harness.Place{Cwd: fixture, Env: os.Environ(), Find: os.Environ()}, conv.NativeID)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range history.Events {
		if !recorded[ev.NativeID] {
			t.Errorf("native message %s %s %q is not in the log", ev.Kind, ev.NativeID, ev.Text)
		}
	}

	// X7-style: the next prompt resumes the exact thread and remembers, and
	// imports nothing.
	before := s.seq()
	s.prompt(codexReturnPrompt)
	s.idle()
	s.expectReply("ROUNDHOUSE-MARKER-7f3a", "ALPHA-1a2b", "GAMMA-5e6f", "RESUME-SDK-c3d4")
	if facts := s.view().Facts; facts == nil || facts.Session != conv.NativeID {
		t.Errorf("resumed thread = %+v, bound to %s", facts, conv.NativeID)
	}
	s.importedSince("the exact-id resume", before)
	s.noDifferences()
	s.oneVersion()

	// released and removed, the thread is discovered, by its own id among
	// every thread in the checkout, and adopted again, its history imported.
	if err := svc.Release(s.id); err != nil {
		t.Fatal(err)
	}
	s.waitFor("the runtime to exit again", func(v service.ConversationView) bool { return v.State == model.RuntimeNone })
	messages := s.replaceConversationByAdopting(project.ID, model.HarnessCodex, conv.NativeID)
	counts := map[string]int{}
	for _, ev := range messages {
		counts[ev.Kind]++
	}
	if counts[model.EventUser] == 0 || counts[model.EventAssistant] == 0 {
		t.Errorf("the adoption's snapshot lacks prompts or replies: %v", counts)
	}
	shutdown(t, svc, s)
	t.Logf("thread %s", conv.NativeID)
}

// TestCodexReleaseDuringCommand releases while a sandboxed command runs. the
// capability report saw the sandbox child outlive the app-server after an
// interrupt; the release must report stopped only if the app-server's process
// group emptied, and otherwise name what is still running.
func TestCodexReleaseDuringCommand(t *testing.T) {
	fixture := fixture(t, "MEGA_E2E_CODEX_FIXTURE")
	h := codex.New(codex.Config{Binary: env("MEGA_E2E_CODEX", "codex")})
	svc := newE2EService(t, h)
	project, err := svc.RegisterProject("codex-rt1", fixture)
	if err != nil {
		t.Fatal(err)
	}
	conv, err := svc.StartConversation(project.ID, model.HarnessCodex, "e2e release", `Run exactly python3 -c "import time; time.sleep(40); print(1)" in the working directory and then reply SLEPT. Nothing else.`, service.LaunchChoice{})
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

// TestCodexMCPApproval calls the fixture's MCP echo tool (mcp/fixture-server.mjs,
// named in the fixture's .codex/config.toml). codex asks through an MCP
// elicitation; it must arrive as a permission card naming fixture/echo with
// its parameter, offering to allow it for the session, the tool must not run
// while the card waits, and it must run once allowed for the session, which
// takes Allow's place. the fixture server logs each call to
// mcp/mcp-calls.log.
//
// the approval round trip is what it tests. the test plays the operator for
// exactly that request (fixture/echo, server fixture, tool echo, text
// MCP-E2E-7c1d), asserted before it answers, through the HTTP API a
// browser uses; anything else fails the test unanswered.
func TestCodexMCPApproval(t *testing.T) {
	fixture := fixture(t, "MEGA_E2E_CODEX_FIXTURE")
	calls := filepath.Join(fixture, "mcp", "mcp-calls.log")
	_ = os.Remove(calls)
	t.Cleanup(func() { _ = os.Remove(calls) })
	h := codex.New(codex.Config{Binary: env("MEGA_E2E_CODEX", "codex")})
	svc := newE2EService(t, h)
	project, err := svc.RegisterProject("codex-rt1", fixture)
	if err != nil {
		t.Fatal(err)
	}
	conv, err := svc.StartConversation(project.ID, model.HarnessCodex, "e2e mcp", "Call the echo tool from the fixture MCP server with the text MCP-E2E-7c1d, then reply with exactly what the tool returned. Nothing else.", service.LaunchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{t: t, svc: svc, id: conv.ID}
	approval := s.expectApproval(approvalWant{tool: "fixture/echo", mcpServer: "fixture", mcpTool: "echo"})
	payload, _ := approval.Payload.(map[string]any)
	if payload["allow_session"] != true {
		t.Fatalf("expected the MCP approval to offer allow for the session, got %+v; it was not answered", approval)
	}
	if input, _ := payload["input"].(map[string]any); input["text"] != "MCP-E2E-7c1d" {
		t.Fatalf("expected the echo of MCP-E2E-7c1d, got input %v; it was not answered", payload["input"])
	}
	time.Sleep(3 * time.Second)
	if _, err := os.Stat(calls); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the tool ran before it was allowed: %v", err)
	}
	// allowed for the session, which the request offers, in place of Allow.
	if code := s.answer(approval.ID, harness.DecisionAllowSession); code != "" {
		t.Fatalf("the answer was refused: %s", code)
	}
	s.idle()
	s.expectReply("echo: MCP-E2E-7c1d")
	logged, err := os.ReadFile(calls)
	if err != nil || !strings.Contains(string(logged), "MCP-E2E-7c1d") {
		t.Errorf("the fixture server logged %q (%v)", logged, err)
	}
	shutdown(t, svc, s)
	t.Logf("thread %s", s.view().NativeID)
}

// TestCodexModelAndEffort lists codex's models and what a new conversation
// copies, then launches one marker turn with an explicit model and effort:
// codex reports the model on thread/start and both with
// thread/settings/updated as the turn starts, and the runtime's facts are
// that report.
func TestCodexModelAndEffort(t *testing.T) {
	fixture := fixture(t, "MEGA_E2E_CODEX_FIXTURE")
	chosen := env("MEGA_E2E_CODEX_MODEL", "gpt-6-luna")
	h := codex.New(codex.Config{Binary: env("MEGA_E2E_CODEX", "codex")})
	readsSettings(t, h, fixture)
	svc := newE2EService(t, h)
	project, err := svc.RegisterProject("codex-rt1", fixture)
	if err != nil {
		t.Fatal(err)
	}
	list, err := svc.Models(project.ID, model.HarnessCodex)
	if err != nil {
		t.Fatal(err)
	}
	listed := false
	for _, m := range list.Models {
		listed = listed || m.ID == chosen
	}
	t.Logf("codex %s lists %d models; a new conversation copies %q at %q", list.Version, len(list.Models), list.Copy.Model, list.Copy.Effort)
	if !listed {
		t.Fatalf("%s is not listed", chosen)
	}
	if list.Copy.Effort == "low" {
		t.Fatal("the operator's config already runs low; the effort override would not show")
	}
	conv, err := svc.StartConversation(project.ID, model.HarnessCodex, "e2e model", codexMarkerPrompt, service.LaunchChoice{Model: chosen, Effort: "low"})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{t: t, svc: svc, id: conv.ID}
	s.idle()
	s.expectReply("ROUNDHOUSE-MARKER-7f3a", "ALPHA-1a2b")
	view := s.view()
	if view.Facts == nil || view.Facts.Model != chosen || view.Facts.Effort != "low" {
		t.Errorf("codex reported %+v, want %s at low", view.Facts, chosen)
	}
	reported := 0
	evs, err := svc.Events(s.id, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range evs {
		if ev.Status == model.StatusModelChanged {
			reported++
		}
	}
	if reported != 1 {
		t.Errorf("model changes recorded = %d, want the one report as the turn started", reported)
	}
	shutdown(t, svc, s)
	t.Logf("thread %s ran %s at %s", view.NativeID, view.Facts.Model, view.Facts.Effort)
}

// TestCodexStartWithoutPrompt starts a released conversation with no prompt:
// thread/resume holds the exact thread idle and reports what codex runs; the
// next prompt is answered in the same thread.
func TestCodexStartWithoutPrompt(t *testing.T) {
	fixture := fixture(t, "MEGA_E2E_CODEX_FIXTURE")
	h := codex.New(codex.Config{Binary: env("MEGA_E2E_CODEX", "codex")})
	svc := newE2EService(t, h)
	project, err := svc.RegisterProject("codex-rt1", fixture)
	if err != nil {
		t.Fatal(err)
	}
	conv, err := svc.StartConversation(project.ID, model.HarnessCodex, "e2e start", codexMarkerPrompt, service.LaunchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{t: t, svc: svc, id: conv.ID}
	s.idle()
	view := startedSession(t, svc, s)
	if view.Facts.Version == "" || view.Facts.Model == "" || view.Facts.Model != view.Model {
		t.Errorf("idle facts = %+v", view.Facts)
	}
	s.prompt("Reply with the word BETA-START-9e1f. Do not run any commands. Nothing else.")
	s.idle()
	s.expectReply("BETA-START-9e1f")
	if v := s.view(); v.NativeID != view.NativeID || v.Facts.Session != view.NativeID {
		t.Errorf("after the prompt: thread %q, facts %+v", v.NativeID, v.Facts)
	}
	shutdown(t, svc, s)
	t.Logf("thread %s started idle on %s %s", view.NativeID, view.Facts.Version, view.Facts.Model)
}

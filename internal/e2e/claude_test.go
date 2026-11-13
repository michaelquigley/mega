//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"github.com/michaelquigley/df/dd"
	"github.com/michaelquigley/mega/internal/server"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/harness/claude"
	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/registry"
	"github.com/michaelquigley/mega/internal/service"
)

const (
	turnTimeout = 4 * time.Minute

	markerPrompt   = "Reply with the project instruction marker token from your project instructions, followed by the word ALPHA-1a2b. Nothing else."
	questionPrompt = "Use the AskUserQuestion tool to ask me a single question: which color do you prefer? Offer exactly two options, Red and Blue. After I answer, reply with my chosen color followed by GAMMA-5e6f. Nothing else."
	approvalPrompt = "Using the Bash tool, run exactly `sleep 3` and then reply DELTA-7a8b. Nothing else."
	returnPrompt   = "List every marker token you have seen in this conversation so far, in order, one per line, then append SDK-RETURN-c3d4 on its own line. Nothing else."
)

// fixture is the disposable checkout a round trip runs in, named by the
// environment variable key; with none set, the test is skipped, since it would
// otherwise run against whatever directory it found.
func fixture(t *testing.T, key string) string {
	t.Helper()
	dir := os.Getenv(key)
	if dir == "" {
		t.Skipf("%s is not set: the round trip needs a disposable fixture checkout (see internal/e2e/doc.go)", key)
	}
	return dir
}

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
	api *httptest.Server // the service's own HTTP handler, for answers
}

// answerBody is what a browser posts to answer a request.
type answerBody struct {
	RequestID string
	Decision  string
}

type errorReply struct {
	Code    string
	Message string
}

// answer answers a request the way a browser does: POST
// /api/conversations/{id}/answer to the service's own HTTP handler, never a
// call into the service or the harness. it returns the error code the API
// answered with, or empty for an accepted answer.
func (s *session) answer(requestID, decision string) string {
	s.t.Helper()
	if s.api == nil {
		s.api = httptest.NewServer(server.New(s.svc, server.About{Version: "e2e", Developer: true}).Handler())
		s.t.Cleanup(s.api.Close)
	}
	body, err := dd.UnbindJSON(&answerBody{RequestID: requestID, Decision: decision})
	if err != nil {
		s.t.Fatal(err)
	}
	resp, err := http.Post(s.api.URL+"/api/conversations/"+s.id+"/answer", "application/json", bytes.NewReader(body))
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusAccepted {
		return ""
	}
	data, _ := io.ReadAll(resp.Body)
	reply := &errorReply{}
	if err := dd.BindJSON(reply, data); err != nil || reply.Code == "" {
		s.t.Fatalf("answering %s: %d %s", requestID, resp.StatusCode, data)
	}
	return reply.Code
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

// approvalWant is the one approval an approval round trip means to answer:
// the tool, and the exact command, or the MCP server and tool.
type approvalWant struct {
	tool      string
	command   string
	mcpServer string
	mcpTool   string
}

// unwrapShell takes the command codex reports out of the login shell it runs
// it in ("/bin/bash -lc '...'"), so the test can compare the command itself.
func unwrapShell(command string) string {
	if inner, ok := strings.CutPrefix(command, "/bin/bash -lc '"); ok && strings.HasSuffix(inner, "'") {
		return strings.TrimSuffix(inner, "'")
	}
	return command
}

// expectApproval waits for the conversation's pending requests and asserts
// they are exactly one permission request for want, before anything answers
// it. any other request fails the test and is left unanswered: a test plays
// the operator only for the approval it is about.
func (s *session) expectApproval(want approvalWant) model.Request {
	s.t.Helper()
	view := s.waitFor("the approval", func(v service.ConversationView) bool { return len(v.Pending) > 0 })
	if len(view.Pending) != 1 {
		s.t.Fatalf("expected exactly one pending request, got %+v; none was answered", view.Pending)
	}
	req := view.Pending[0]
	payload, _ := req.Payload.(map[string]any)
	input, _ := payload["input"].(map[string]any)
	command, _ := input["command"].(string)
	switch {
	case req.Kind != model.RequestPermission:
		s.t.Fatalf("expected a permission request, got %+v; it was not answered", req)
	case payload["tool"] != want.tool:
		s.t.Fatalf("expected an approval for %q, got %v; it was not answered", want.tool, payload["tool"])
	case want.command != "" && unwrapShell(command) != want.command:
		s.t.Fatalf("expected an approval for exactly %q, got %q; it was not answered", want.command, command)
	case want.mcpServer != "" && (payload["mcp_server"] != want.mcpServer || payload["mcp_tool"] != want.mcpTool):
		s.t.Fatalf("expected an approval for %s/%s, got %v/%v; it was not answered", want.mcpServer, want.mcpTool, payload["mcp_server"], payload["mcp_tool"])
	}
	return req
}

func (s *session) prompt(text string) {
	s.t.Helper()
	if err := s.svc.Prompt(s.id, text); err != nil {
		s.t.Fatal(err)
	}
}

// TestClaudeRoundTrip is C1 to C4 and C6 from the capability report through the
// service, without the terminal visit (C5), which the operator does by hand.
//
// the approval round trip is what C3 tests: a real permission request reaches
// the operator, the turn waits on it, and the command runs only after the
// answer. the test plays the operator for exactly that request (Bash, `sleep
// 3`), asserted before it answers, through the HTTP API a
// browser uses; anything else fails the test unanswered.
func TestClaudeRoundTrip(t *testing.T) {
	fixture := fixture(t, "MEGA_E2E_CLAUDE_FIXTURE")
	h := newClaudeHarness(t)
	svc := newE2EService(t, h)
	project, err := svc.RegisterProject("claude-rt1", fixture)
	if err != nil {
		t.Fatal(err)
	}

	// C1: launch with the marker turn; project instructions load.
	conv, err := svc.StartConversation(project.ID, model.HarnessClaude, "e2e", markerPrompt, service.LaunchChoice{})
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
	approval := s.expectApproval(approvalWant{tool: "Bash", command: "sleep 3"})
	time.Sleep(4 * time.Second)
	if s.view().TurnActive == false {
		t.Error("the turn ended while the approval was pending")
	}
	if code := s.answer(approval.ID, "allow"); code != "" {
		t.Fatalf("the answer was refused: %s", code)
	}
	s.idle()
	s.expectReply("DELTA-7a8b")
	if code := s.answer(approval.ID, "allow"); code != service.CodeNotPending {
		t.Errorf("late answer: %q", code)
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
	history, err := h.History(context.Background(), harness.Place{Cwd: fixture, Env: os.Environ(), Find: os.Environ()}, conv.NativeID)
	if err != nil {
		t.Fatal(err)
	}
	var prompts int
	for _, ev := range history.Events {
		if !recorded[ev.NativeID] {
			t.Errorf("native message %s %s %q is not in the log", ev.Kind, ev.NativeID, ev.Text)
		}
		if ev.Kind == model.EventUser {
			prompts++
		}
	}
	if len(history.Events) == 0 || prompts != 3 {
		t.Errorf("history has %d events and %d prompts, want the 3 managed prompts", len(history.Events), prompts)
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

// newClaudeHarness runs the real shim and the pinned claude code.
func newClaudeHarness(t *testing.T) *claude.Harness {
	t.Helper()
	shim, err := filepath.Abs(filepath.Join("..", "..", "shim", "claude"))
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	return claude.New(claude.Config{
		Node:       env("MEGA_E2E_NODE", "node"),
		Shim:       shim,
		Executable: env("MEGA_E2E_CLAUDE", filepath.Join(home, ".local", "share", "claude", "versions", "2.1.318")),
	})
}

// TestClaudeAutoMode launches in the auto mode the operator chose and runs the
// fixture's sleep turn, which its project ask rule keeps out of the
// classifier's hands: the residue must still arrive as a request, raised in
// auto mode, and wait for the operator's answer.
//
// the approval round trip is what it tests: in auto mode, the residue the
// classifier leaves still reaches the operator as a request. the test plays
// the operator for exactly that request (Bash, `sleep 3`, raised in auto
// mode), asserted before it answers, through the HTTP API a
// browser uses; anything else fails the test unanswered.
func TestClaudeAutoMode(t *testing.T) {
	fixture := fixture(t, "MEGA_E2E_CLAUDE_FIXTURE")
	svc := newE2EService(t, newClaudeHarness(t))
	project, err := svc.RegisterProject("claude-rt1", fixture)
	if err != nil {
		t.Fatal(err)
	}
	conv, err := svc.StartConversation(project.ID, model.HarnessClaude, "e2e auto", approvalPrompt, service.LaunchChoice{PermissionMode: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{t: t, svc: svc, id: conv.ID}
	approval := s.expectApproval(approvalWant{tool: "Bash", command: "sleep 3"})
	if facts := s.view().Facts; facts == nil || facts.PermissionMode != "auto" {
		t.Errorf("runtime facts = %+v", facts)
	}
	if payload, _ := approval.Payload.(map[string]any); payload["permission_mode"] != "auto" {
		t.Fatalf("expected the request raised in auto mode, got %+v; it was not answered", approval)
	}
	if code := s.answer(approval.ID, "allow"); code != "" {
		t.Fatalf("the answer was refused: %s", code)
	}
	s.idle()
	s.expectReply("DELTA-7a8b")
	shutdown(t, svc, s)
	t.Logf("session %s", s.view().NativeID)
}

// TestClaudeModeSwitch switches a default-mode conversation to auto while its
// sleep turn waits on an approval, mid-turn. claude must confirm the switch,
// the live mode must follow from its report, the pending request must survive
// and be answerable, and the next turn's system/init, as claude re-sends it,
// must report the same mode the header shows. its subject is the switch, not
// the approval, so it answers nothing: once the request is seen to survive the
// switch, the turn is interrupted, which settles the request without a
// decision and runs nothing. the next turn's report is checked through the
// normalized facts and events, never claude's own frames.
func TestClaudeModeSwitch(t *testing.T) {
	fixture := fixture(t, "MEGA_E2E_CLAUDE_FIXTURE")
	svc := newE2EService(t, newClaudeHarness(t))
	project, err := svc.RegisterProject("claude-rt1", fixture)
	if err != nil {
		t.Fatal(err)
	}
	conv, err := svc.StartConversation(project.ID, model.HarnessClaude, "e2e switch", approvalPrompt, service.LaunchChoice{PermissionMode: "default"})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{t: t, svc: svc, id: conv.ID}
	approval := s.expectApproval(approvalWant{tool: "Bash", command: "sleep 3"})
	view := s.view()
	if view.Facts == nil || view.Facts.PermissionMode != "default" {
		t.Fatalf("started in %+v", view.Facts)
	}

	outcome, err := svc.SetPermissionMode(s.id, "auto")
	if err != nil || outcome.Outcome != service.ModeConfirmed {
		t.Fatalf("switch: %+v, %v", outcome, err)
	}
	view = s.view()
	if view.Facts.PermissionMode != "auto" || view.PermissionMode != "auto" {
		t.Errorf("after the switch: live %q, choice %q", view.Facts.PermissionMode, view.PermissionMode)
	}
	if len(view.Pending) != 1 || view.Pending[0].ID != approval.ID {
		t.Fatalf("the pending request did not survive the switch: %+v", view.Pending)
	}
	if err := svc.Interrupt(s.id); err != nil {
		t.Fatal(err)
	}
	s.idle()
	if len(s.view().Pending) != 0 {
		t.Errorf("the interrupted turn's request is still pending: %+v", s.view().Pending)
	}

	// the next turn's init is claude's own report of the mode. the adapter
	// turns a report of any other mode into a mode_changed event, so the
	// normalized facts tell it: the header still says auto, and no change away
	// from auto was recorded after the switch.
	s.prompt(markerPrompt)
	s.idle()
	s.expectReply("ALPHA-1a2b")
	evs, err := svc.Events(s.id, 0)
	if err != nil {
		t.Fatal(err)
	}
	if mode := s.view().Facts.PermissionMode; mode != "auto" {
		t.Errorf("after the next turn the header shows %q, want auto", mode)
	}
	var changes []string
	for _, ev := range evs {
		if ev.Kind == model.EventRuntime && ev.Status == model.StatusModeChanged {
			changes = append(changes, ev.Facts.PermissionMode+": "+ev.Text)
			if ev.Facts.PermissionMode != "auto" {
				t.Errorf("a change away from auto was recorded after the switch: %s %q", ev.Facts.PermissionMode, ev.Text)
			}
		}
	}
	if len(changes) == 0 {
		t.Error("the switch to auto was not recorded")
	}
	t.Logf("mode changes recorded: %v", changes)
	shutdown(t, svc, s)
	t.Logf("session %s", s.view().NativeID)
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

// TestClaudeModelAndEffort lists claude's models through an idle session, then
// launches one marker turn with an explicit model and effort: claude's own
// report, the next system/init, names the chosen model resolved. effort is
// never reported, so only the request is checked.
func TestClaudeModelAndEffort(t *testing.T) {
	fixture := fixture(t, "MEGA_E2E_CLAUDE_FIXTURE")
	chosen := env("MEGA_E2E_CLAUDE_MODEL", "sonnet")
	svc := newE2EService(t, newClaudeHarness(t))
	project, err := svc.RegisterProject("claude-rt1", fixture)
	if err != nil {
		t.Fatal(err)
	}
	list, err := svc.Models(project.ID, model.HarnessClaude)
	if err != nil {
		t.Fatal(err)
	}
	var resolved string
	for _, m := range list.Models {
		if m.ID == chosen {
			resolved = m.Resolved
		}
	}
	t.Logf("claude %s lists %d models; %s resolves to %q; a new conversation copies %q at %q in %q", list.Version, len(list.Models), chosen, resolved, list.Copy.Model, list.Copy.Effort, list.Copy.Mode)
	if resolved == "" {
		t.Fatalf("%s is not listed", chosen)
	}
	conv, err := svc.StartConversation(project.ID, model.HarnessClaude, "e2e model", markerPrompt, service.LaunchChoice{Model: chosen, Effort: "low"})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{t: t, svc: svc, id: conv.ID}
	s.idle()
	s.expectReply("ROUNDHOUSE-MARKER-7f3a", "ALPHA-1a2b")
	view := s.view()
	if view.Facts == nil || view.Facts.Model != resolved {
		t.Errorf("claude reported %+v, want %s", view.Facts, resolved)
	}
	if view.Model != chosen || view.Effort != "low" {
		t.Errorf("stored choice %q %q", view.Model, view.Effort)
	}
	shutdown(t, svc, s)
	t.Logf("session %s ran %s", view.NativeID, view.Facts.Model)
}

// startedSession releases a conversation that has had its first turn, starts
// it again with no prompt, and checks the start resumed the same session and
// opened no turn.
func startedSession(t *testing.T, svc *service.Service, s *session) service.ConversationView {
	t.Helper()
	before := s.view()
	if err := svc.Release(s.id); err != nil {
		t.Fatal(err)
	}
	s.waitFor("the release", func(v service.ConversationView) bool {
		return v.Runtime == model.RuntimeNone && v.State == model.RuntimeNone
	})
	evs, err := svc.Events(s.id, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(s.id); err != nil {
		t.Fatal(err)
	}
	view := s.waitFor("the start", func(v service.ConversationView) bool { return v.State == service.StateRunning && v.Facts != nil })
	// claude's idle report carries no session until claude sends one; codex
	// reports it with thread/resume. either way it is the session resumed.
	if view.TurnActive || view.NativeID != before.NativeID || (view.Facts.Session != "" && view.Facts.Session != before.NativeID) {
		t.Fatalf("started: turn %v, session %q (facts %q), want %q", view.TurnActive, view.NativeID, view.Facts.Session, before.NativeID)
	}
	// a few idle seconds: nothing is sent, so no message, tool call or turn
	// arrives. the harness's own bookkeeping may (claude-rt1's SessionStart
	// hook reports as native events).
	time.Sleep(5 * time.Second)
	after, err := svc.Events(s.id, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range after[len(evs):] {
		switch ev.Kind {
		case model.EventUser, model.EventAssistant, model.EventToolUse, model.EventToolResult, model.EventTurnEnd:
			t.Errorf("an idle start recorded %s %q", ev.Kind, ev.Text)
		}
	}
	if v := s.view(); v.TurnActive || v.Attention == model.AttentionRunning {
		t.Errorf("idle after a start: turn %v, attention %q", v.TurnActive, v.Attention)
	}
	return view
}

// TestClaudeStartWithoutPrompt starts a released conversation with no
// prompt: the runtime resumes the exact session and holds idle, reporting its
// version and model, in the mode the launch passed, which is the
// conversation's request; a mode switch is confirmed at once; the next
// prompt is answered in the same session.
func TestClaudeStartWithoutPrompt(t *testing.T) {
	fixture := fixture(t, "MEGA_E2E_CLAUDE_FIXTURE")
	svc := newE2EService(t, newClaudeHarness(t))
	project, err := svc.RegisterProject("claude-rt1", fixture)
	if err != nil {
		t.Fatal(err)
	}
	conv, err := svc.StartConversation(project.ID, model.HarnessClaude, "e2e start", markerPrompt, service.LaunchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{t: t, svc: svc, id: conv.ID}
	s.idle()
	view := startedSession(t, svc, s)
	if view.Facts.Version == "" || view.Facts.Model == "" || view.PermissionMode == "" || view.Facts.PermissionMode != view.PermissionMode {
		t.Errorf("idle facts = %+v", view.Facts)
	}
	outcome, err := svc.SetPermissionMode(s.id, "acceptEdits")
	if err != nil || outcome.Outcome != service.ModeConfirmed {
		t.Fatalf("idle mode switch: %+v, %v", outcome, err)
	}
	if f := s.view().Facts; f.PermissionMode != "acceptEdits" {
		t.Errorf("after the switch: %+v", f)
	}
	s.prompt("Reply with the word BETA-START-9e1f. Nothing else.")
	s.idle()
	s.expectReply("BETA-START-9e1f")
	if v := s.view(); v.NativeID != view.NativeID || v.State != service.StateRunning || v.Facts.PermissionMode != "acceptEdits" {
		t.Errorf("after the prompt: session %q, facts %+v", v.NativeID, v.Facts)
	}
	shutdown(t, svc, s)
	t.Logf("session %s started idle on %s %s", view.NativeID, view.Facts.Version, view.Facts.Model)
}

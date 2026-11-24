//go:build e2e

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/environ"
	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/harness/pi"
	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/service"
)

// the pi round trips run on the operator's own pi default, whatever pi's
// settings name (Michael's is qwen3.8-27b, served from his host eleven): mega
// changes nothing in pi's configuration to reach it. a local model is slower
// and less exact than a hosted one, so they assert on markers and protocol
// events, never on phrasing, and give it the same generous turn time.
//
// the fixture's extension asks before any bash command containing "sleep",
// with a select, "Approve bash command: <command>" (Allow, Block), and gives
// the model an ask_user tool, a plain question. pi marks neither as an
// approval, so both are questions here. the tests play the operator only for
// the dialog each is about, after asserting what it is.
const (
	piMarkerPrompt   = "Reply with the project instruction marker token from your project instructions, followed by the word ALPHA-1a2b. Nothing else."
	piQuestionPrompt = "Use the ask_user tool to ask me a single question: which color do you prefer? Options: Red, Blue. Then reply with my chosen color followed by GAMMA-5e6f. Nothing else."
	piGatedPrompt    = "Using the bash tool, run exactly `sleep 3` and then reply DELTA-7a8b. Nothing else."
	piGatedTitle     = "Approve bash command: sleep 3"
	piReturnPrompt   = "List every marker token you have seen in this conversation so far, in order, one per line, then append RPC-RETURN-c3d4 on its own line. Nothing else."
)

func newPiHarness() *pi.Harness {
	return pi.New(pi.Config{Binary: env("MEGA_E2E_PI", "pi")})
}

// skipUnanswered skips when the first turn failed because the model's server
// did not answer: pi persists that as a response that failed.
func (s *session) skipUnanswered() {
	s.t.Helper()
	evs, err := s.svc.Events(s.id, 0)
	if err != nil {
		s.t.Fatal(err)
	}
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Kind == model.EventTurnEnd {
			if evs[i].Status == model.TurnError {
				s.t.Skipf("pi's default model did not answer (%q); is its server up?", evs[i].Text)
			}
			return
		}
	}
}

// expectDialog asserts the conversation waits on exactly one pi dialog, a
// question offering Dismiss and never a session-wide allow, titled title
// when title is given, before anything answers it.
func (s *session) expectDialog(title string) model.Request {
	s.t.Helper()
	view := s.waitFor("the dialog", func(v service.ConversationView) bool { return len(v.Pending) > 0 })
	if len(view.Pending) != 1 {
		s.t.Fatalf("expected exactly one pending dialog, got %+v; none was answered", view.Pending)
	}
	req := view.Pending[0]
	payload, _ := req.Payload.(map[string]any)
	questions, _ := payload["questions"].([]any)
	asked := ""
	if len(questions) == 1 {
		q, _ := questions[0].(map[string]any)
		asked, _ = q["question"].(string)
	}
	switch {
	case req.Kind != model.RequestQuestion:
		s.t.Fatalf("a pi dialog is a question, got %+v; it was not answered", req)
	case payload["dismiss"] != true || payload["allow_session"] != nil:
		s.t.Fatalf("a pi dialog offers Dismiss and no session-wide allow, got %v; it was not answered", payload)
	case title != "" && asked != title:
		s.t.Fatalf("expected the dialog %q, got %q; it was not answered", title, asked)
	}
	return req
}

func dialogTitle(req model.Request) string {
	payload, _ := req.Payload.(map[string]any)
	questions, _ := payload["questions"].([]any)
	q, _ := questions[0].(map[string]any)
	title, _ := q["question"].(string)
	return title
}

// TestPiRoundTrip: a conversation copies pi's own model and thinking level,
// a marker turn, the extension's plain question answered, the extension's
// gate on a command answered after a delay with the turn waiting, a release,
// an exact-id launch that remembers, and the session adopted again.
func TestPiRoundTrip(t *testing.T) {
	fixture := fixture(t, "MEGA_E2E_PI_FIXTURE")
	h := newPiHarness()
	place := harness.Place{Cwd: fixture, Env: os.Environ(), Find: os.Environ()}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	settings, err := h.Settings(ctx, place)
	if err != nil || settings.Model == "" {
		t.Fatalf("pi's settings name no model: %+v, %v", settings, err)
	}
	t.Logf("pi's own default: %s, thinking %q", settings.Model, settings.Effort)
	svc := newE2EService(t, h)
	project, err := svc.RegisterProject("pi-rt1", fixture)
	if err != nil {
		t.Fatal(err)
	}
	conv, err := svc.StartConversation(project.ID, model.HarnessPi, "e2e", piMarkerPrompt, service.LaunchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	if conv.Model != settings.Model || conv.Effort != settings.Effort || conv.PermissionMode != "" {
		t.Errorf("the conversation copied %q %q %q, pi's settings name %q %q", conv.Model, conv.Effort, conv.PermissionMode, settings.Model, settings.Effort)
	}
	s := &session{t: t, svc: svc, id: conv.ID}
	s.idle()
	s.skipUnanswered()
	s.expectReply("ROUNDHOUSE-MARKER-7f3a", "ALPHA-1a2b")
	if facts := s.view().Facts; facts == nil || facts.Session != conv.NativeID || facts.Model != settings.Model || facts.PermissionMode != "" {
		t.Errorf("runtime facts = %+v, bound to %s", facts, conv.NativeID)
	}

	// the extension's plain question, a select the model words.
	s.prompt(piQuestionPrompt)
	question := s.expectDialog("")
	if code := s.post(&answerBody{RequestID: question.ID, Answers: map[string]string{dialogTitle(question): "Blue"}}); code != "" {
		t.Fatalf("the answer was refused: %s", code)
	}
	s.idle()
	s.expectReply("Blue", "GAMMA-5e6f")

	// the extension's gate on a command: the turn waits on it.
	s.prompt(piGatedPrompt)
	gate := s.expectDialog(piGatedTitle)
	time.Sleep(4 * time.Second)
	if !s.view().TurnActive {
		t.Error("the turn ended while the dialog was pending")
	}
	if code := s.post(&answerBody{RequestID: gate.ID, Answers: map[string]string{piGatedTitle: "Allow"}}); code != "" {
		t.Fatalf("the answer was refused: %s", code)
	}
	s.idle()
	s.expectReply("DELTA-7a8b")
	if code := s.post(&answerBody{RequestID: gate.ID, Decision: "dismiss"}); code != service.CodeNotPending {
		t.Errorf("late answer: %q", code)
	}

	// a release, seen to exit.
	if err := svc.Release(s.id); err != nil {
		t.Fatal(err)
	}
	s.waitFor("the runtime to exit", func(v service.ConversationView) bool { return v.State == model.RuntimeNone })

	// the exact-id resume remembers the whole conversation.
	s.prompt(piReturnPrompt)
	s.idle()
	s.expectReply("ROUNDHOUSE-MARKER-7f3a", "ALPHA-1a2b", "GAMMA-5e6f", "DELTA-7a8b", "RPC-RETURN-c3d4")
	if facts := s.view().Facts; facts == nil || facts.Session != conv.NativeID {
		t.Errorf("resumed session = %+v, bound to %s", facts, conv.NativeID)
	}

	// released, the session adopted again brings its speech in.
	if err := svc.Release(s.id); err != nil {
		t.Fatal(err)
	}
	s.waitFor("the runtime to exit again", func(v service.ConversationView) bool { return v.State == model.RuntimeNone })
	if messages := s.replaceConversationByAdopting(project.ID, model.HarnessPi, conv.NativeID); len(messages) == 0 {
		t.Fatal("the adoption's snapshot has no prompts or replies")
	}
	shutdown(t, svc, s)
	t.Logf("session %s", conv.NativeID)
}

// TestPiInterruptWaitsOnADialog: an interrupt with the extension's gate
// pending says what it waits on, and the turn ends only once the operator
// dismisses the dialog; the command never runs. mega never answers the
// dialog to let the interrupt through.
func TestPiInterruptWaitsOnADialog(t *testing.T) {
	fixture := fixture(t, "MEGA_E2E_PI_FIXTURE")
	svc := newE2EService(t, newPiHarness())
	project, err := svc.RegisterProject("pi-rt1", fixture)
	if err != nil {
		t.Fatal(err)
	}
	conv, err := svc.StartConversation(project.ID, model.HarnessPi, "e2e interrupt", piMarkerPrompt, service.LaunchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{t: t, svc: svc, id: conv.ID}
	s.idle()
	s.skipUnanswered()
	s.prompt(piGatedPrompt)
	gate := s.expectDialog(piGatedTitle)
	if err := svc.Interrupt(s.id); err != nil {
		t.Fatal(err)
	}
	s.waitFor("the interrupt's note", func(service.ConversationView) bool {
		evs, _ := svc.Events(s.id, 0)
		return slices.ContainsFunc(evs, func(ev *model.Event) bool {
			return ev.Status == model.StatusWaiting && strings.Contains(ev.Text, piGatedTitle)
		})
	})
	time.Sleep(3 * time.Second)
	if v := s.view(); !v.TurnActive || len(v.Pending) != 1 {
		t.Fatalf("the interrupt went through with the dialog pending: %+v", v)
	}
	if code := s.post(&answerBody{RequestID: gate.ID, Decision: harness.DecisionDismiss}); code != "" {
		t.Fatalf("the dismissal was refused: %s", code)
	}
	s.idle()
	evs, err := svc.Events(s.id, 0)
	if err != nil {
		t.Fatal(err)
	}
	if last := evs[len(evs)-1]; last.Kind != model.EventTurnEnd || last.Status != model.TurnInterrupted {
		t.Errorf("the turn ended %s %s", last.Kind, last.Status)
	}
	shutdown(t, svc, s)
}

// TestPiReleaseWithADialogPending: a release with the extension's gate
// pending ends pi's process, since a pending dialog holds pi open through
// its input closing; the dialog is recorded as never answered, the command
// it gated never runs, and the session resumes by its exact id.
func TestPiReleaseWithADialogPending(t *testing.T) {
	fixture := fixture(t, "MEGA_E2E_PI_FIXTURE")
	check := filepath.Join(fixture, "pi-release-check.txt")
	_ = os.Remove(check)
	t.Cleanup(func() { _ = os.Remove(check) })
	svc := newE2EService(t, newPiHarness())
	project, err := svc.RegisterProject("pi-rt1", fixture)
	if err != nil {
		t.Fatal(err)
	}
	conv, err := svc.StartConversation(project.ID, model.HarnessPi, "e2e release", piMarkerPrompt, service.LaunchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{t: t, svc: svc, id: conv.ID}
	s.idle()
	s.skipUnanswered()
	s.prompt("Using the bash tool, run exactly `touch pi-release-check.txt; sleep 2` and then reply EPSILON-9f0a. Nothing else.")
	gate := s.expectDialog("Approve bash command: touch pi-release-check.txt; sleep 2")
	begun := time.Now()
	if err := svc.Release(s.id); err != nil {
		t.Fatal(err)
	}
	s.waitFor("pi to be ended", func(v service.ConversationView) bool { return v.State == model.RuntimeNone })
	t.Logf("pi was ended %s after the release", time.Since(begun).Round(time.Millisecond))
	if running := processesNaming(conv.NativeID); len(running) > 0 {
		t.Errorf("pi is still running: pids %v", running)
	}
	if _, err := os.Stat(check); err == nil {
		t.Error("the command the dialog gated ran")
	}
	evs, err := svc.Events(s.id, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(evs, func(ev *model.Event) bool {
		return ev.Kind == model.EventRequestResolved && ev.Req != nil && ev.Req.ID == gate.ID && ev.Req.Outcome == model.OutcomeUnanswered
	}) {
		t.Error("the dialog is not recorded as never answered")
	}
	// the session resumes, by its exact id.
	s.prompt("Reply with the project instruction marker token followed by ZETA-1b2c. Nothing else.")
	s.idle()
	s.expectReply("ROUNDHOUSE-MARKER-7f3a", "ZETA-1b2c")
	if facts := s.view().Facts; facts == nil || facts.Session != conv.NativeID {
		t.Errorf("resumed session = %+v, bound to %s", facts, conv.NativeID)
	}
	shutdown(t, svc, s)
}

// processesNaming lists the live processes whose command line holds s, such
// as a pi started with --session-id s.
func processesNaming(s string) []int {
	entries, _ := os.ReadDir("/proc")
	var out []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		cmdline, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err == nil && strings.Contains(string(cmdline), s) {
			out = append(out, pid)
		}
	}
	return out
}

// TestPiDiscoveryAndAdoption finds a session of its own and adopts it: a copy
// of a recorded session (the pi adapter's testdata/p1-and-p4.jsonl), planted
// by the adapter under a fresh id in a session root of this test's own, which
// the service's resolver points pi at. it depends on no other run's sessions
// and on no model, so it passes with a reply-less session newest in the
// fixture's folder, and with pi's model server down.
func TestPiDiscoveryAndAdoption(t *testing.T) {
	fixture := fixture(t, "MEGA_E2E_PI_FIXTURE")
	root := t.TempDir()
	svc := newE2EServiceResolving(t, newPiHarness(), piSessionRoot{root: root, base: service.DefaultResolver(os.Environ())})
	id, err := pi.PlantSession([]string{piSessionDir + "=" + root}, fixture, filepath.Join("..", "harness", "pi", "testdata", "p1-and-p4.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	project, err := svc.RegisterProject("pi-rt1", fixture)
	if err != nil {
		t.Fatal(err)
	}
	found, err := svc.Discover(project.ID, model.HarnessPi)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(found, func(s harness.Session) bool { return s.NativeID == id })
	if i < 0 {
		t.Fatalf("discovery did not find the planted session %s among %d", id, len(found))
	}
	t.Logf("%d sessions; planted %s: %.60q", len(found), id, found[i].Preview)
	conv, err := svc.Adopt(project.ID, model.HarnessPi, id, "adopted")
	if err != nil {
		t.Fatal(err)
	}
	evs, err := svc.Events(conv.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, ev := range evs {
		counts[ev.Kind]++
		if ev.Kind != model.EventRuntime && ev.NativeID == "" {
			t.Errorf("an imported %s has no identity", ev.Kind)
		}
	}
	t.Logf("adopted %s: %v", id, counts)
	if counts[model.EventUser] == 0 || counts[model.EventAssistant] == 0 {
		t.Errorf("the import lacks prompts or replies: %v", counts)
	}
}

// piSessionDir is the variable that moves pi's session store (pi 1.5.2).
const piSessionDir = "PI_CODING_AGENT_SESSION_DIR"

// piSessionRoot resolves a project's environment as mega does, then points
// pi's session store at root, leaving the rest of the environment as it
// resolved.
type piSessionRoot struct {
	root string
	base service.Resolver
}

func (r piSessionRoot) Resolve(ctx context.Context, dir, activate string) (*environ.Resolution, error) {
	res, err := r.base.Resolve(ctx, dir, activate)
	if err != nil {
		return nil, err
	}
	res.Env = append(slices.DeleteFunc(res.Env, func(kv string) bool { return strings.HasPrefix(kv, piSessionDir+"=") }), piSessionDir+"="+r.root)
	return res, nil
}

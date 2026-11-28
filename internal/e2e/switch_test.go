//go:build e2e

package e2e

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/harness/codex"
	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/service"
)

// the idle model switch: a runtime started with no prompt has its model
// switched while no turn is open, then takes a prompt. claude code 2.1.327
// answered such a switch with a user message marked as a replay, outside any
// turn, which mega once took for a turn that never ended (capability report,
// 2026-11-18). each harness's round trip asserts the conversation is idle
// after the switch, takes a prompt, and that the turn ran on the new model.

// idleSwitch switches an idle conversation's model to another the harness
// lists, prefer if it is one, and checks the conversation stays idle: over a
// few seconds no turn opens, it is not running, and no message is recorded; a
// harness's own echo of the switch may arrive only as a native event. strict,
// it switches to prefer or skips, saying why, never to another model. it
// returns the model switched to, the id it runs as, and the native events the
// switch recorded.
func idleSwitch(t *testing.T, svc *service.Service, s *session, projectID, harnessName, prefer, wantOutcome string, strict bool) (chosen, runsAs string, natives []string) {
	t.Helper()
	view := s.view()
	if view.TurnActive || view.Facts == nil {
		t.Fatalf("not idle before the switch: turn %v, facts %+v", view.TurnActive, view.Facts)
	}
	list, err := svc.Models(projectID, harnessName)
	if err != nil {
		t.Fatal(err)
	}
	current := view.Facts.Model
	var pick *harness.ModelOption
	for i := range list.Models {
		m := &list.Models[i]
		runs := m.Resolved
		if runs == "" {
			runs = m.ID
		}
		if m.ID == "default" || m.ID == view.Model || m.ID == current || runs == current {
			continue
		}
		if pick == nil || m.ID == prefer {
			pick = m
		}
	}
	if pick == nil {
		t.Skipf("%s lists no model besides the one running, %s", harnessName, current)
	}
	if strict && pick.ID != prefer {
		t.Skipf("%s cannot switch to %s: it is not listed, or is the one running, %s", harnessName, prefer, current)
	}
	effort := ""
	if len(pick.Efforts) > 0 {
		effort = pick.DefaultEffort
		if slices.Contains(pick.Efforts, "low") {
			effort = "low"
		}
		if effort == "" {
			effort = pick.Efforts[0]
		}
	}
	before, err := svc.Events(s.id, 0)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := svc.SetModel(s.id, pick.ID, effort)
	if err != nil || outcome.Outcome != wantOutcome {
		t.Fatalf("switching %s to %s at %q while idle: %+v, %v; want %s", current, pick.ID, effort, outcome, err, wantOutcome)
	}
	// long enough for an echo, and for a turn it might open to show.
	time.Sleep(5 * time.Second)
	after, err := svc.Events(s.id, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range after[len(before):] {
		switch ev.Kind {
		case model.EventUser, model.EventAssistant, model.EventToolUse, model.EventToolResult, model.EventTurnEnd:
			t.Errorf("an idle model switch recorded %s %q", ev.Kind, ev.Text)
		case model.EventNative:
			natives = append(natives, ev.Text)
		}
	}
	if v := s.view(); v.TurnActive || v.Attention == model.AttentionRunning {
		t.Errorf("after an idle model switch: turn %v, attention %q", v.TurnActive, v.Attention)
	}
	runsAs = pick.Resolved
	if runsAs == "" {
		runsAs = pick.ID
	}
	t.Logf("switched %s to %s (runs as %s) at %q while idle: %s; native events %q", current, pick.ID, runsAs, effort, outcome.Outcome, natives)
	return pick.ID, runsAs, natives
}

// ranOn checks the conversation's newest turn asked for chosen and ran on
// runsAs, by its own record.
func ranOn(t *testing.T, svc *service.Service, s *session, chosen, runsAs string) {
	t.Helper()
	evs, err := svc.Events(s.id, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Kind != model.EventTurnEnd {
			continue
		}
		turn := evs[i].Turn
		if turn == nil || turn.Asked.Model != chosen || turn.Ran.Model != runsAs {
			t.Errorf("the turn after the switch recorded %+v, want asked %s, ran %s", turn, chosen, runsAs)
		}
		if f := s.view().Facts; f == nil || f.Model != runsAs {
			t.Errorf("the runtime reports %+v, want %s", f, runsAs)
		}
		return
	}
	t.Error("no turn ended after the switch")
}

// TestClaudeModelSwitchWhileIdle starts a released conversation with no
// prompt, switches its model while idle, which claude confirms at once, and
// checks it stays idle, takes a prompt, and that the turn ran on the new
// model. then the session is adopted again, and its prompts are exactly the
// two the test sent, so claude's local /model command is not one of them.
func TestClaudeModelSwitchWhileIdle(t *testing.T) {
	fixture := fixture(t, "MEGA_E2E_CLAUDE_FIXTURE")
	svc := newE2EService(t, mustNewClaudeHarness(t))
	project, err := svc.RegisterProject("claude-rt1", fixture)
	if err != nil {
		t.Fatal(err)
	}
	conv, err := svc.StartConversation(project.ID, model.HarnessClaude, "e2e idle switch", markerPrompt, service.LaunchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{t: t, svc: svc, id: conv.ID}
	s.idle()
	view := startedSession(t, svc, s)
	chosen, runsAs, natives := idleSwitch(t, svc, s, project.ID, model.HarnessClaude, env("MEGA_E2E_CLAUDE_MODEL", "sonnet"), service.ModeConfirmed, false)
	t.Logf("claude %s echoed the switch as a replay: %v", view.Facts.Version, slices.Contains(natives, "user/replay"))
	s.prompt(switchPrompt)
	s.idle()
	s.expectReply("BETA-SWITCH-4c2d")
	ranOn(t, svc, s, chosen, runsAs)

	// released, the session holds what claude wrote for the switch, its
	// local /model command among it. adopted again, the snapshot's prompts
	// are exactly the two this test sent, so the command is not shown as a
	// prompt, and the reply after the switch is speech. what else the
	// assistant's speech holds is not checked.
	if err := svc.Release(s.id); err != nil {
		t.Fatal(err)
	}
	s.waitFor("the runtime to exit", func(v service.ConversationView) bool { return v.State == model.RuntimeNone })
	var prompts []string
	var speech strings.Builder
	for _, ev := range s.replaceConversationByAdopting(project.ID, model.HarnessClaude, conv.NativeID) {
		switch ev.Kind {
		case model.EventUser:
			prompts = append(prompts, ev.Text)
		case model.EventAssistant:
			speech.WriteString(ev.Text + "\n")
		}
	}
	if sent := []string{markerPrompt, switchPrompt}; !slices.Equal(prompts, sent) {
		t.Errorf("the snapshot's prompts are %d, want exactly the %d this test sent", len(prompts), len(sent))
	}
	if !strings.Contains(speech.String(), "BETA-SWITCH-4c2d") {
		t.Error("the snapshot's assistant speech lacks the reply after the switch")
	}
	shutdown(t, svc, s)
}

// switchPrompt is the prompt after an idle model switch.
const switchPrompt = "Reply with the word BETA-SWITCH-4c2d. Nothing else."

// TestCodexModelSwitchWhileIdle starts a released conversation with no
// prompt and switches its model while idle. codex takes a model choice with
// the next prompt, so the switch is stored for it (next_prompt); the
// conversation stays idle, takes a prompt, and the turn runs on the new
// model.
func TestCodexModelSwitchWhileIdle(t *testing.T) {
	fixture := fixture(t, "MEGA_E2E_CODEX_FIXTURE")
	svc := newE2EService(t, codex.New(codex.Config{Binary: env("MEGA_E2E_CODEX", "codex")}))
	project, err := svc.RegisterProject("codex-rt1", fixture)
	if err != nil {
		t.Fatal(err)
	}
	conv, err := svc.StartConversation(project.ID, model.HarnessCodex, "e2e idle switch", codexMarkerPrompt, service.LaunchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{t: t, svc: svc, id: conv.ID}
	s.idle()
	startedSession(t, svc, s)
	chosen, runsAs, _ := idleSwitch(t, svc, s, project.ID, model.HarnessCodex, env("MEGA_E2E_CODEX_MODEL", "gpt-6-luna"), service.ModelNextPrompt, false)
	s.prompt("Reply with the word BETA-SWITCH-4c2d. Do not run any commands. Nothing else.")
	s.idle()
	s.expectReply("BETA-SWITCH-4c2d")
	ranOn(t, svc, s, chosen, runsAs)
	shutdown(t, svc, s)
}

// TestPiModelSwitchWhileIdle starts a released conversation with no prompt
// and switches its model while idle, which pi applies at once. the first
// turn runs on pi's own default; the switch goes to MEGA_E2E_PI_MODEL, by
// default openai-codex/gpt-6-sol, a hosted model, since the default is the
// operator's local one and an idle switch needs another. it goes there or
// nowhere: where pi does not list it, the test skips, so no other provider
// is ever used.
func TestPiModelSwitchWhileIdle(t *testing.T) {
	fixture := fixture(t, "MEGA_E2E_PI_FIXTURE")
	svc := newE2EService(t, newPiHarness())
	project, err := svc.RegisterProject("pi-rt1", fixture)
	if err != nil {
		t.Fatal(err)
	}
	conv, err := svc.StartConversation(project.ID, model.HarnessPi, "e2e idle switch", piMarkerPrompt, service.LaunchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{t: t, svc: svc, id: conv.ID}
	s.idle()
	s.skipUnanswered()
	startedSession(t, svc, s)
	chosen, runsAs, _ := idleSwitch(t, svc, s, project.ID, model.HarnessPi, env("MEGA_E2E_PI_MODEL", "openai-codex/gpt-6-sol"), service.ModeConfirmed, true)
	s.prompt("Reply with the word BETA-SWITCH-4c2d. Nothing else.")
	s.idle()
	s.expectReply("BETA-SWITCH-4c2d")
	ranOn(t, svc, s, chosen, runsAs)
	shutdown(t, svc, s)
}

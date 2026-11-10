package service

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"git.hq.quigley.com/research/roundhouse/internal/harness"
	"git.hq.quigley.com/research/roundhouse/internal/harness/harnesstest"
	"git.hq.quigley.com/research/roundhouse/internal/model"
)

func startedIn(rt *harnesstest.Runtime, mode string) {
	rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: rt.NativeID(), PermissionMode: mode}})
}

func modeEvents(evs []*model.Event) []string {
	var modes []string
	for _, ev := range evs {
		if ev.Kind == model.EventRuntime && ev.Status == model.StatusModeChanged {
			modes = append(modes, ev.Facts.PermissionMode)
		}
	}
	return modes
}

func TestChosenModeIsRecordedAndPassedOnEveryLaunch(t *testing.T) {
	r := newRig(t)
	conv, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "", "one", "auto")
	if err != nil {
		t.Fatal(err)
	}
	first := r.fake.Last()
	if first.Spec.PermissionMode != "auto" || r.view(conv.ID).PermissionMode != "auto" {
		t.Fatalf("spec %q, recorded %q", first.Spec.PermissionMode, r.view(conv.ID).PermissionMode)
	}
	startedIn(first, "auto")
	finishTurn(first, "a-1", "done")
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the runtime to exit", first.Exited)
	if err := r.svc.Prompt(conv.ID, "two"); err != nil {
		t.Fatal(err)
	}
	if spec := r.fake.Last().Spec; spec.PermissionMode != "auto" {
		t.Errorf("the next launch passed %q", spec.PermissionMode)
	}

	// no choice is roundhouse's default, resolved by the adapter at launch.
	plain := r.start("three")
	if spec := r.fake.Last().Spec; spec.PermissionMode != "" || r.view(plain.ID).PermissionMode != "" {
		t.Errorf("no choice: spec %q, recorded %q", spec.PermissionMode, r.view(plain.ID).PermissionMode)
	}
}

func TestOnlyOfferedModesAreAccepted(t *testing.T) {
	r := newRig(t)
	for _, mode := range []string{"bypassPermissions", "dontAsk", "Auto", "yolo"} {
		_, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "", "one", mode)
		wantCode(t, err, CodeInvalid)
	}
	_, err := r.svc.StartConversation(r.project.ID, model.HarnessCodex, "", "one", "auto")
	wantCode(t, err, CodeInvalid)
	if n := len(r.svc.Workspace().Conversations); n != 0 {
		t.Errorf("refused launches left %d conversations", n)
	}

	conv := r.start("one")
	startedIn(r.fake.Last(), "default")
	_, err = r.svc.SetPermissionMode(conv.ID, "bypassPermissions")
	wantCode(t, err, CodeInvalid)
	_, err = r.svc.SetPermissionMode(conv.ID, "")
	wantCode(t, err, CodeInvalid)
	if modes := r.fake.Last().Modes(); len(modes) != 0 {
		t.Errorf("a refused mode reached the runtime: %v", modes)
	}
}

// switchTo asks for a mode and fails the test on an error; it returns the outcome.
func (r *rig) switchTo(id, mode string) string {
	r.t.Helper()
	outcome, err := r.svc.SetPermissionMode(id, mode)
	if err != nil {
		r.t.Fatal(err)
	}
	return outcome.Outcome
}

func TestSwitchWritesTheChoiceAndShowsOnlyWhatTheRuntimeReports(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	startedIn(rt, "default")
	id := rt.Ask(model.RequestPermission, map[string]any{"tool": "Bash"})

	// while the harness switches, the choice is written but the live mode has
	// not moved and no mode change is recorded.
	var during ConversationView
	var duringEvents []string
	rt.OnMode(func() {
		during = r.view(conv.ID)
		duringEvents = modeEvents(r.events(conv.ID))
	})
	if outcome := r.switchTo(conv.ID, "auto"); outcome != ModeConfirmed {
		t.Fatalf("outcome = %q", outcome)
	}
	if during.PermissionMode != "auto" || during.Facts.PermissionMode != "default" || len(duringEvents) != 0 {
		t.Errorf("during the switch: choice %q, live %q, mode events %v", during.PermissionMode, during.Facts.PermissionMode, duringEvents)
	}

	// the live mode is the runtime's report, recorded once, with no event of
	// the service's own.
	view := r.view(conv.ID)
	if view.Facts.PermissionMode != "auto" || view.PermissionMode != "auto" || view.Facts.ModeInherited {
		t.Errorf("after: live %q, choice %q, inherited %v", view.Facts.PermissionMode, view.PermissionMode, view.Facts.ModeInherited)
	}
	if got := modeEvents(r.events(conv.ID)); !slices.Equal(got, []string{"auto"}) {
		t.Errorf("mode events = %v", got)
	}

	// the request raised before the switch is still pending, and answerable.
	if len(view.Pending) != 1 || view.Pending[0].ID != id {
		t.Fatalf("pending after the switch = %+v", view.Pending)
	}
	if err := r.svc.Answer(conv.ID, id, harness.Answer{Decision: harness.DecisionAllow}); err != nil {
		t.Errorf("answering after the switch: %v", err)
	}
}

// the reviewer's round 4 regression: a newer report arriving before the switch
// returns is never overwritten by the switch.
func TestNewerReportBeforeTheSwitchReturnsIsNeverOverwritten(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	startedIn(rt, "default")
	rt.AfterMode(func() {
		rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusModeChanged, Text: "reported by claude", Facts: &model.RuntimeFacts{PermissionMode: "plan"}})
	})
	if outcome := r.switchTo(conv.ID, "auto"); outcome != ModeConfirmed {
		t.Fatalf("outcome = %q", outcome)
	}
	view := r.view(conv.ID)
	if view.Facts.PermissionMode != "plan" {
		t.Errorf("live mode = %q, want the newer report's plan", view.Facts.PermissionMode)
	}
	if view.PermissionMode != "auto" {
		t.Errorf("choice = %q, want the operator's auto", view.PermissionMode)
	}
	if got := modeEvents(r.events(conv.ID)); !slices.Equal(got, []string{"auto", "plan"}) {
		t.Errorf("mode events = %v, in claude's order", got)
	}
}

func TestRefusedSwitchRestoresThePreviousChoice(t *testing.T) {
	r := newRig(t)
	conv, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "", "one", "plan")
	if err != nil {
		t.Fatal(err)
	}
	rt := r.fake.Last()
	startedIn(rt, "plan")
	rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusModeChanged, Text: "reported by claude", Facts: &model.RuntimeFacts{PermissionMode: "default"}})
	rt.FailMode(errors.New("claude refused permission mode 'auto': not available"))
	outcome, err := r.svc.SetPermissionMode(conv.ID, "auto")
	if err != nil || outcome.Outcome != ModeRefused || !strings.Contains(outcome.Message, "not available") {
		t.Fatalf("outcome = %+v, err %v", outcome, err)
	}
	view := r.view(conv.ID)
	if view.PermissionMode != "plan" || view.Facts.PermissionMode != "default" {
		t.Errorf("after a refusal: choice %q (want the previous plan), live %q", view.PermissionMode, view.Facts.PermissionMode)
	}
	if got := modeEvents(r.events(conv.ID)); !slices.Equal(got, []string{"default"}) {
		t.Errorf("mode events = %v", got)
	}
}

// a conversation with no choice keeps none after a refusal: it goes on
// inheriting, rather than being pinned to the mode it happened to run in.
func TestRefusedSwitchRestoresNoChoice(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	startedIn(rt, "default")
	rt.FailMode(errors.New("not available"))
	if outcome, err := r.svc.SetPermissionMode(conv.ID, "auto"); err != nil || outcome.Outcome != ModeRefused {
		t.Fatalf("outcome = %+v, err %v", outcome, err)
	}
	if view := r.view(conv.ID); view.PermissionMode != "" {
		t.Errorf("choice after a refusal = %q, want none", view.PermissionMode)
	}
}

func TestUnconfirmedSwitchLeavesTheLiveModeAsReported(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	startedIn(rt, "default")
	rt.FailMode(fmt.Errorf("%w: no reply", harness.ErrModeUnconfirmed))
	outcome, err := r.svc.SetPermissionMode(conv.ID, "auto")
	if err != nil || outcome.Outcome != ModeUnconfirmed {
		t.Fatalf("outcome = %+v, err %v", outcome, err)
	}
	if view := r.view(conv.ID); view.PermissionMode != "auto" || view.Facts.PermissionMode != "default" || len(modeEvents(r.events(conv.ID))) != 0 {
		t.Errorf("after an unconfirmed switch: choice %q, live %q", view.PermissionMode, view.Facts.PermissionMode)
	}
	// claude confirms later; its report is what moves the live mode.
	rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusModeChanged, Text: "confirmed by claude; applies from the next tool call", Facts: &model.RuntimeFacts{PermissionMode: "auto"}})
	if view := r.view(conv.ID); view.Facts.PermissionMode != "auto" {
		t.Errorf("after the late confirmation: live %q", view.Facts.PermissionMode)
	}
}

// a report the log cannot record leaves the last confirmed mode on screen, as
// every unrecorded transition does; nothing switches the harness back.
func TestUnrecordedReportKeepsTheLastConfirmedMode(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	startedIn(rt, "default")
	r.closeLog(conv.ID)
	if outcome := r.switchTo(conv.ID, "auto"); outcome != ModeConfirmed {
		t.Fatalf("outcome = %q", outcome)
	}
	if view := r.view(conv.ID); view.Facts.PermissionMode != "default" {
		t.Errorf("an unrecorded report showed %q", view.Facts.PermissionMode)
	}
	if !slices.Equal(rt.Modes(), []string{"auto"}) {
		t.Errorf("runtime switches = %v; nothing switches it back", rt.Modes())
	}
}

func TestOneSwitchAtATime(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	startedIn(rt, "default")
	var second error
	rt.OnMode(func() { _, second = r.svc.SetPermissionMode(conv.ID, "plan") })
	if outcome := r.switchTo(conv.ID, "auto"); outcome != ModeConfirmed {
		t.Fatalf("outcome = %q", outcome)
	}
	wantCode(t, second, CodeConflict)
	if !slices.Equal(rt.Modes(), []string{"auto"}) {
		t.Errorf("runtime switches = %v", rt.Modes())
	}
}

// with no runtime, a choice is stored for the next launch: one registry write,
// no harness call and no runtime event. the next launch passes it.
func TestChoiceWithoutARuntimeIsStoredForTheNextLaunch(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	startedIn(rt, "default")
	finishTurn(rt, "a-1", "done")
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the runtime to exit", rt.Exited)
	before := len(r.events(conv.ID))
	sub := r.svc.Subscribe()
	defer sub.Close()

	if outcome := r.switchTo(conv.ID, "auto"); outcome != ModeStored {
		t.Fatalf("outcome = %q", outcome)
	}
	view := r.view(conv.ID)
	if view.PermissionMode != "auto" || view.Facts != nil {
		t.Errorf("choice %q, live facts %+v", view.PermissionMode, view.Facts)
	}
	if evs := r.events(conv.ID); len(evs) != before {
		t.Errorf("storing a choice recorded %d events", len(evs)-before)
	}
	if modes := rt.Modes(); len(modes) != 0 {
		t.Errorf("a stopped runtime was asked to switch: %v", modes)
	}
	var announced bool
	for len(sub.Updates()) > 0 {
		if update := <-sub.Updates(); update.WorkspaceChanged {
			announced = true
		}
	}
	if !announced {
		t.Error("the stored choice was not announced")
	}

	if err := r.svc.Prompt(conv.ID, "two"); err != nil {
		t.Fatal(err)
	}
	if spec := r.fake.Last().Spec; spec.PermissionMode != "auto" {
		t.Errorf("the next launch passed %q", spec.PermissionMode)
	}
}

// an adopted conversation has never run here; its choice can still be set
// ahead of its first prompt.
func TestChoiceCanBeSetBeforeAnAdoptedConversationsFirstLaunch(t *testing.T) {
	r := newRig(t)
	r.fake.SetSessions([]harness.Session{{NativeID: "terminal-1", Preview: "from the terminal"}})
	conv, err := r.svc.Adopt(r.project.ID, model.HarnessClaude, "terminal-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if outcome := r.switchTo(conv.ID, "plan"); outcome != ModeStored {
		t.Fatalf("outcome = %q", outcome)
	}
	if err := r.svc.Prompt(conv.ID, "go on"); err != nil {
		t.Fatal(err)
	}
	if spec := r.fake.Last().Spec; spec.PermissionMode != "plan" || spec.NativeID != "terminal-1" {
		t.Errorf("first launch: %+v", spec)
	}
}

func TestChoiceIsRefusedWhileUnsupervised(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	startedIn(r.fake.Last(), "default")
	r.restart()
	_, err := r.svc.SetPermissionMode(conv.ID, "auto")
	wantCode(t, err, CodeConflict)
	if view := r.view(conv.ID); view.PermissionMode != "" {
		t.Errorf("choice written while unsupervised: %q", view.PermissionMode)
	}
	_, err = r.svc.SetPermissionMode("nope", "auto")
	wantCode(t, err, CodeNotFound)
}

func TestHarnessReportedModeShowsButLeavesTheChoice(t *testing.T) {
	r := newRig(t)
	conv, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "", "one", "plan")
	if err != nil {
		t.Fatal(err)
	}
	rt := r.fake.Last()
	startedIn(rt, "plan")
	rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusModeChanged, Facts: &model.RuntimeFacts{PermissionMode: "default"}, Text: "claude reports permission mode 'default'"})
	view := r.view(conv.ID)
	if view.Facts.PermissionMode != "default" || view.PermissionMode != "plan" {
		t.Errorf("shown %q, recorded %q", view.Facts.PermissionMode, view.PermissionMode)
	}
	if got := modeEvents(r.events(conv.ID)); !slices.Equal(got, []string{"default"}) {
		t.Errorf("mode events = %v", got)
	}
}

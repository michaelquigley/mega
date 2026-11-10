package service

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"git.hq.quigley.com/research/roundhouse/internal/harness"
	"git.hq.quigley.com/research/roundhouse/internal/harness/harnesstest"
	"git.hq.quigley.com/research/roundhouse/internal/model"
	"git.hq.quigley.com/research/roundhouse/internal/registry"
)

// modelRig is a rig with a codex harness beside claude, whose runtimes take a
// model change with the next prompt, as codex does.
func modelRig(t *testing.T) (*rig, *harnesstest.Harness) {
	t.Helper()
	r := &rig{t: t, path: t.TempDir() + "/registry.yaml", fake: harnesstest.New(model.HarnessClaude)}
	codex := harnesstest.New(model.HarnessCodex)
	codex.DeferModelChanges(true)
	r.svc = newService(t, r.path, allTrusted, r.fake, codex)
	project, err := r.svc.RegisterProject("fixture", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r.project = project
	return r, codex
}

func startedOn(rt *harnesstest.Runtime, runningModel, effort string) {
	rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: rt.NativeID(), Model: runningModel, Effort: effort}})
}

func reportModel(rt *harnesstest.Runtime, runningModel, effort string) {
	rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusModelChanged, Origin: model.OriginManaged, Facts: &model.RuntimeFacts{Model: runningModel, Effort: effort}})
}

func (r *rig) setModel(id, chosenModel, effort string) string {
	r.t.Helper()
	outcome, err := r.svc.SetModel(id, chosenModel, effort)
	if err != nil {
		r.t.Fatal(err)
	}
	return outcome.Outcome
}

// the choice is intent: on the conversation's record before the harness is
// asked, and passed again on every later launch.
func TestModelChoiceIsStoredBeforeItIsPassed(t *testing.T) {
	r, _ := modelRig(t)
	var recorded []model.Conversation
	r.fake.OnLaunch(func() {
		reg, err := registry.Load(r.path)
		if err != nil {
			t.Error(err)
			return
		}
		recorded = reg.Snapshot().Conversations
	})
	conv, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "", "one", LaunchChoice{Model: "sonnet", Effort: "low"})
	if err != nil {
		t.Fatal(err)
	}
	if len(recorded) != 1 || recorded[0].Model != "sonnet" || recorded[0].Effort != "low" {
		t.Fatalf("recorded at launch: %+v", recorded)
	}
	first := r.fake.Last()
	if first.Spec.Model != "sonnet" || first.Spec.Effort != "low" {
		t.Fatalf("spec = %+v", first.Spec)
	}
	startedOn(first, "claude-sonnet-5-5", "")
	finishTurn(first, "a-1", "done")
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the runtime to exit", first.Exited)
	if err := r.svc.Prompt(conv.ID, "two"); err != nil {
		t.Fatal(err)
	}
	if spec := r.fake.Last().Spec; spec.Model != "sonnet" || spec.Effort != "low" {
		t.Errorf("the next launch passed %q %q", spec.Model, spec.Effort)
	}

	r.start("three")
	if spec := r.fake.Last().Spec; spec.Model != "" || spec.Effort != "" {
		t.Errorf("no choice passed %q %q", spec.Model, spec.Effort)
	}
}

// with no runtime a change is only stored, and the next launch passes it.
func TestModelChangeWithNoRuntimeIsStored(t *testing.T) {
	r, _ := modelRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	startedOn(rt, "claude-opus-5-5", "")
	finishTurn(rt, "a-1", "done")
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the runtime to exit", rt.Exited)
	launches := len(r.fake.Runtimes())
	if outcome := r.setModel(conv.ID, "haiku", ""); outcome != ModeStored {
		t.Fatalf("outcome = %q", outcome)
	}
	if n := len(r.fake.Runtimes()); n != launches {
		t.Errorf("storing a choice launched a runtime")
	}
	if view := r.view(conv.ID); view.Model != "haiku" || view.ModelNext != "" {
		t.Errorf("stored %q, next %q", view.Model, view.ModelNext)
	}
	// inherit can be chosen with no runtime.
	if outcome := r.setModel(conv.ID, "", ""); outcome != ModeStored || r.view(conv.ID).Model != "" {
		t.Errorf("inherit: %q, stored %q", outcome, r.view(conv.ID).Model)
	}
}

// a running claude conversation changes at once; the live model is what
// claude then reports, never the choice.
func TestRunningClaudeChangeIsConfirmedAndShownOnlyAsReported(t *testing.T) {
	r, _ := modelRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	startedOn(rt, "claude-opus-5-5", "")
	var during ConversationView
	rt.OnMode(func() { during = r.view(conv.ID) })
	if outcome := r.setModel(conv.ID, "sonnet", "low"); outcome != ModeConfirmed {
		t.Fatalf("outcome = %q", outcome)
	}
	if during.Model != "sonnet" || during.Effort != "low" {
		t.Errorf("choice during the change: %q %q", during.Model, during.Effort)
	}
	if got := rt.ModelChanges(); !slices.Equal(got, []string{"sonnet/low"}) {
		t.Errorf("changes passed: %v", got)
	}
	view := r.view(conv.ID)
	if view.Facts.Model != "claude-opus-5-5" || view.ModelNext != "sonnet" || view.EffortNext != "low" {
		t.Errorf("before claude reports: live %q, next %q %q", view.Facts.Model, view.ModelNext, view.EffortNext)
	}
	// the turn under way when the change was asked for ends; the change is
	// still the next prompt's.
	finishTurn(rt, "a-1", "done")
	if view := r.view(conv.ID); view.ModelNext != "sonnet" {
		t.Errorf("the change was dropped as the earlier turn ended: next %q", view.ModelNext)
	}
	if err := r.svc.Prompt(conv.ID, "two"); err != nil {
		t.Fatal(err)
	}
	reportModel(rt, "claude-sonnet-5-5", "")
	finishTurn(rt, "a-2", "done")
	view = r.view(conv.ID)
	if view.Facts.Model != "claude-sonnet-5-5" || view.ModelNext != "" || view.EffortNext != "" {
		t.Errorf("after the turn: live %q, next %q %q", view.Facts.Model, view.ModelNext, view.EffortNext)
	}
}

// a running codex conversation stores the change for its next turn: the
// outcome says so, the header shows it as the next prompt's, and the live
// model and effort move only when codex reports them as that turn starts.
func TestRunningCodexChangeAppliesOnTheNextTurnOnly(t *testing.T) {
	r, codex := modelRig(t)
	conv, err := r.svc.StartConversation(r.project.ID, model.HarnessCodex, "", "one", LaunchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	rt := codex.Last()
	startedOn(rt, "gpt-a", "high")
	if outcome := r.setModel(conv.ID, "gpt-b", "low"); outcome != ModelNextPrompt {
		t.Fatalf("outcome = %q", outcome)
	}
	view := r.view(conv.ID)
	if view.Model != "gpt-b" || view.Effort != "low" || view.Facts.Model != "gpt-a" || view.Facts.Effort != "high" {
		t.Errorf("stored %q %q, live %q %q", view.Model, view.Effort, view.Facts.Model, view.Facts.Effort)
	}
	if view.ModelNext != "gpt-b" || view.EffortNext != "low" {
		t.Errorf("next prompt %q %q", view.ModelNext, view.EffortNext)
	}
	// the turn still running ends with the old settings and still shows the
	// change as pending.
	finishTurn(rt, "a-1", "done")
	if view := r.view(conv.ID); view.Facts.Model != "gpt-a" || view.ModelNext != "gpt-b" {
		t.Errorf("as the earlier turn ended: live %q, next %q", view.Facts.Model, view.ModelNext)
	}
	if err := r.svc.Prompt(conv.ID, "two"); err != nil {
		t.Fatal(err)
	}
	reportModel(rt, "gpt-b", "low")
	finishTurn(rt, "a-2", "done")
	view = r.view(conv.ID)
	if view.Facts.Model != "gpt-b" || view.Facts.Effort != "low" || view.ModelNext != "" {
		t.Errorf("after the next turn: live %q %q, next %q", view.Facts.Model, view.Facts.Effort, view.ModelNext)
	}
	var reported []string
	for _, ev := range r.events(conv.ID) {
		if ev.Status == model.StatusModelChanged {
			reported = append(reported, ev.Facts.Model+"/"+ev.Facts.Effort)
		}
	}
	if !slices.Equal(reported, []string{"gpt-b/low"}) {
		t.Errorf("model changes recorded: %v", reported)
	}
}

// a running codex conversation can return to inherit: stored, deferred to
// the next prompt like any codex change, and shown as the next prompt's
// inherit until a turn begun after it ends.
func TestRunningCodexCanReturnToInherit(t *testing.T) {
	r, codex := modelRig(t)
	conv, err := r.svc.StartConversation(r.project.ID, model.HarnessCodex, "", "one", LaunchChoice{Model: "gpt-b", Effort: "low"})
	if err != nil {
		t.Fatal(err)
	}
	rt := codex.Last()
	startedOn(rt, "gpt-b", "low")
	finishTurn(rt, "a-1", "done")
	if outcome := r.setModel(conv.ID, "", ""); outcome != ModelNextPrompt {
		t.Fatalf("outcome = %q", outcome)
	}
	if got := rt.ModelChanges(); !slices.Equal(got, []string{"/"}) {
		t.Errorf("passed to the runtime: %v", got)
	}
	view := r.view(conv.ID)
	if view.Model != "" || view.Effort != "" || !view.ModelNextInherit || !view.EffortNextInherit || view.ModelNext != "" {
		t.Errorf("after inherit: stored %q %q, next inherit %v %v (%q)", view.Model, view.Effort, view.ModelNextInherit, view.EffortNextInherit, view.ModelNext)
	}
	if view.Facts.Model != "gpt-b" {
		t.Errorf("the live model moved before codex reported: %q", view.Facts.Model)
	}
	if err := r.svc.Prompt(conv.ID, "two"); err != nil {
		t.Fatal(err)
	}
	reportModel(rt, "gpt-a", "medium")
	finishTurn(rt, "a-2", "done")
	view = r.view(conv.ID)
	if view.ModelNextInherit || view.EffortNextInherit || view.Facts.Model != "gpt-a" {
		t.Errorf("after the next turn: next inherit %v %v, live %q", view.ModelNextInherit, view.EffortNextInherit, view.Facts.Model)
	}
	// claude stays refused.
	claudeConv, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "", "one", LaunchChoice{Model: "opus"})
	if err != nil {
		t.Fatal(err)
	}
	startedOn(r.fake.Last(), "claude-opus-5-5", "")
	_, err = r.svc.SetModel(claudeConv.ID, "", "")
	wantCode(t, err, CodeInvalid)
}

func TestRefusedModelChangePutsTheChoiceBack(t *testing.T) {
	r, _ := modelRig(t)
	conv, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "", "one", LaunchChoice{Model: "opus"})
	if err != nil {
		t.Fatal(err)
	}
	rt := r.fake.Last()
	startedOn(rt, "claude-opus-5-5", "")
	rt.FailMode(errors.New("not available"))
	outcome, err := r.svc.SetModel(conv.ID, "sonnet", "")
	if err != nil || outcome.Outcome != ModeRefused || outcome.Message != "not available" {
		t.Fatalf("outcome = %+v, %v", outcome, err)
	}
	if view := r.view(conv.ID); view.Model != "opus" || view.ModelNext != "" {
		t.Errorf("after refusal: stored %q, next %q", view.Model, view.ModelNext)
	}

	rt.FailMode(fmt.Errorf("%w: no reply", harness.ErrSettingUnconfirmed))
	if outcome := r.setModel(conv.ID, "sonnet", ""); outcome != ModeUnconfirmed {
		t.Fatalf("unconfirmed outcome = %q", outcome)
	}
	if view := r.view(conv.ID); view.Model != "sonnet" {
		t.Errorf("an unconfirmed choice did not stand: %q", view.Model)
	}

	// claude accepted the model and refused the effort: the model stays
	// chosen and shows as the next prompt's; the effort goes back.
	rt.FailMode(fmt.Errorf("%w: claude refused effort 'max': not available", harness.ErrEffortRefused))
	outcome, err = r.svc.SetModel(conv.ID, "haiku", "max")
	if err != nil || outcome.Outcome != ModeRefused {
		t.Fatalf("effort refused: %+v, %v", outcome, err)
	}
	if view := r.view(conv.ID); view.Model != "haiku" || view.Effort != "" || view.ModelNext != "haiku" || view.EffortNext != "" {
		t.Errorf("after the effort was refused: stored %q %q, next %q %q", view.Model, view.Effort, view.ModelNext, view.EffortNext)
	}
	rt.FailMode(nil)
	if outcome := r.setModel(conv.ID, "sonnet", ""); outcome != ModeConfirmed {
		t.Fatalf("back to sonnet: %q", outcome)
	}

	// inherit cannot be chosen while a runtime runs.
	rt.FailMode(nil)
	_, err = r.svc.SetModel(conv.ID, "", "")
	wantCode(t, err, CodeInvalid)
	if view := r.view(conv.ID); view.Model != "sonnet" {
		t.Errorf("a refused inherit moved the choice to %q", view.Model)
	}
}

func TestModelListIsCachedAndChoicesAreChecked(t *testing.T) {
	r, _ := modelRig(t)
	r.fake.SetCatalog(harness.Catalog{Version: "2.1.318", Models: []harness.ModelOption{
		{ID: "default", IsDefault: true, Efforts: []string{"low", "high"}},
		{ID: "haiku"},
	}})
	r.fake.SetInherited("opus", "high")
	for range 2 {
		list, err := r.svc.Models(r.project.ID, model.HarnessClaude)
		if err != nil {
			t.Fatal(err)
		}
		if list.Version != "2.1.318" || len(list.Models) != 2 || list.InheritedModel != "opus" || list.InheritedEffort != "high" {
			t.Errorf("list = %+v", list)
		}
	}
	if n := r.fake.ModelLists(); n != 1 {
		t.Errorf("the harness was asked for its models %d times", n)
	}
	r.fake.SetInherited("sonnet", "")
	if list, _ := r.svc.Models(r.project.ID, model.HarnessClaude); list.InheritedModel != "sonnet" {
		t.Errorf("what inherit means was cached: %q", list.InheritedModel)
	}
	_, err := r.svc.Models("nonesuch", model.HarnessClaude)
	wantCode(t, err, CodeNotFound)

	for _, choice := range []LaunchChoice{{Model: "nonesuch"}, {Model: "haiku", Effort: "low"}, {Effort: "max"}, {Model: "--bad"}} {
		_, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "", "one", choice)
		wantCode(t, err, CodeInvalid)
	}
	if n := len(r.svc.Workspace().Conversations); n != 0 {
		t.Errorf("refused launches left %d conversations", n)
	}
	conv, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "", "one", LaunchChoice{Model: "default", Effort: "high"})
	if err != nil {
		t.Fatalf("an offered choice: %v", err)
	}
	startedOn(r.fake.Last(), "claude-opus-5-5", "")
	// while running, a model with no effort levels leaves the effort unchosen;
	// one with levels needs one.
	_, err = r.svc.SetModel(conv.ID, "default", "")
	wantCode(t, err, CodeInvalid)
	if outcome := r.setModel(conv.ID, "haiku", ""); outcome != ModeConfirmed {
		t.Errorf("haiku: %q", outcome)
	}
	if view := r.view(conv.ID); view.Model != "haiku" || view.Effort != "" {
		t.Errorf("stored %q %q", view.Model, view.Effort)
	}
}

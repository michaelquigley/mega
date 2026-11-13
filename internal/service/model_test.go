package service

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/harness/harnesstest"
	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/registry"
)

// claudeCatalog is a claude model list as claude 2.1.318 shapes it: a
// default row, aliases that resolve to full ids, no default effort, and one
// model that takes no effort level.
func claudeCatalog() harness.Catalog {
	efforts := []string{"low", "medium", "high"}
	return harness.Catalog{Version: "2.1.318", Models: []harness.ModelOption{
		{ID: "default", Resolved: "claude-opus-5-5", DisplayName: "Default (recommended)", Efforts: efforts, IsDefault: true},
		{ID: "opus", Resolved: "claude-opus-5-5", DisplayName: "Opus 5.5", Efforts: efforts},
		{ID: "sonnet", Resolved: "claude-sonnet-5-5", DisplayName: "Sonnet 5.5", Efforts: efforts},
		{ID: "haiku", Resolved: "claude-haiku-4-5", DisplayName: "Haiku 4.5"},
	}}
}

// codexCatalog is a codex model list: a default model, and a default effort
// for each.
func codexCatalog() harness.Catalog {
	efforts := []string{"low", "medium", "high"}
	return harness.Catalog{Version: "0.166.0", Models: []harness.ModelOption{
		{ID: "gpt-a", Efforts: efforts, DefaultEffort: "medium", IsDefault: true},
		{ID: "gpt-b", Efforts: efforts, DefaultEffort: "medium"},
	}}
}

// modelRig is a rig with a codex harness beside claude, whose runtimes take a
// model change with the next prompt, as codex does, each offering its own
// list.
func modelRig(t *testing.T) (*rig, *harnesstest.Harness) {
	t.Helper()
	r := &rig{t: t, path: t.TempDir() + "/registry.yaml", fake: harnesstest.New(model.HarnessClaude)}
	r.fake.SetCatalog(claudeCatalog())
	codex := harnesstest.New(model.HarnessCodex)
	codex.SetCatalog(codexCatalog())
	codex.SetSettings(model.TurnSettings{})
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

// turnRecords is each recorded turn_end's record, nil where it has none.
func (r *rig) turnRecords(id string) []*model.TurnRecord {
	r.t.Helper()
	var out []*model.TurnRecord
	for _, ev := range r.events(id) {
		if ev.Kind == model.EventTurnEnd {
			out = append(out, ev.Turn)
		}
	}
	return out
}

// the request is intent: on the conversation's record before the harness is
// asked, and passed again on every later launch. what the operator leaves
// unchosen is copied from the settings, so every launch passes all three.
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
	if len(recorded) != 1 || recorded[0].Model != "sonnet" || recorded[0].Effort != "low" || recorded[0].PermissionMode != model.PermissionModeDefault {
		t.Fatalf("recorded at launch: %+v", recorded)
	}
	first := r.fake.Last()
	if first.Spec.Model != "sonnet" || first.Spec.Effort != "low" || first.Spec.PermissionMode != model.PermissionModeDefault {
		t.Fatalf("spec = %q %q %q", first.Spec.Model, first.Spec.Effort, first.Spec.PermissionMode)
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
	if spec := r.fake.Last().Spec; spec.Model != "default" || spec.Effort != harnesstest.DefaultEffort || spec.PermissionMode != model.PermissionModeDefault {
		t.Errorf("no choice passed %q %q %q", spec.Model, spec.Effort, spec.PermissionMode)
	}
}

// with no runtime a change is only stored, and the next launch passes it. a
// model is always required, and an effort for a model that takes one.
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
	if view := r.view(conv.ID); view.Model != "haiku" || view.Effort != "" {
		t.Errorf("stored %q %q", view.Model, view.Effort)
	}
	_, err := r.svc.SetModel(conv.ID, "", "")
	wantCode(t, err, CodeInvalid)
	_, err = r.svc.SetModel(conv.ID, "sonnet", "")
	wantCode(t, err, CodeInvalid)
	if view := r.view(conv.ID); view.Model != "haiku" {
		t.Errorf("a refused change moved the request to %q", view.Model)
	}
}

// a running claude conversation changes at once; the live model is what
// claude then reports, never the request. the turn that ran with it records
// both, and an alias and the full id it resolves to are the same model.
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
		t.Errorf("request during the change: %q %q", during.Model, during.Effort)
	}
	if got := rt.ModelChanges(); !slices.Equal(got, []string{"sonnet/low"}) {
		t.Errorf("changes passed: %v", got)
	}
	if view := r.view(conv.ID); view.Facts.Model != "claude-opus-5-5" {
		t.Errorf("before claude reports: live %q", view.Facts.Model)
	}
	finishTurn(rt, "a-1", "done")
	if err := r.svc.Prompt(conv.ID, "two"); err != nil {
		t.Fatal(err)
	}
	reportModel(rt, "claude-sonnet-5-5", "")
	finishTurn(rt, "a-2", "done")
	if view := r.view(conv.ID); view.Facts.Model != "claude-sonnet-5-5" {
		t.Errorf("after the turn: live %q", view.Facts.Model)
	}
	records := r.turnRecords(conv.ID)
	if len(records) != 2 || records[1] == nil {
		t.Fatalf("records = %v", records)
	}
	// the first turn asked for the copy and ran on what it resolves to.
	if first := records[0]; first == nil || first.Asked.Model != "default" || first.Ran.Model != "claude-opus-5-5" || len(first.Differs) != 0 {
		t.Errorf("first turn = %+v", first)
	}
	// claude never reports effort: a turn ran with the one sent.
	want := model.TurnRecord{Asked: model.TurnSettings{Model: "sonnet", Effort: "low", Mode: model.PermissionModeDefault}, Ran: model.TurnSettings{Model: "claude-sonnet-5-5", Effort: "low"}}
	if got := *records[1]; got.Asked != want.Asked || got.Ran != want.Ran || len(got.Differs) != 0 {
		t.Errorf("second turn = %+v", got)
	}
}

// a running codex conversation stores the change for its next turn: the
// outcome says so, and the live model and effort move only when codex
// reports them as that turn starts. the turn under way when it was asked
// for keeps what it asked for.
func TestRunningCodexChangeAppliesOnTheNextTurnOnly(t *testing.T) {
	r, codex := modelRig(t)
	conv, err := r.svc.StartConversation(r.project.ID, model.HarnessCodex, "", "one", LaunchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	rt := codex.Last()
	if rt.Spec.Model != "gpt-a" || rt.Spec.Effort != "medium" || rt.Spec.PermissionMode != "" {
		t.Fatalf("the copy passed %q %q %q", rt.Spec.Model, rt.Spec.Effort, rt.Spec.PermissionMode)
	}
	startedOn(rt, "gpt-a", "medium")
	if outcome := r.setModel(conv.ID, "gpt-b", "low"); outcome != ModelNextPrompt {
		t.Fatalf("outcome = %q", outcome)
	}
	view := r.view(conv.ID)
	if view.Model != "gpt-b" || view.Effort != "low" || view.Facts.Model != "gpt-a" || view.Facts.Effort != "medium" {
		t.Errorf("stored %q %q, live %q %q", view.Model, view.Effort, view.Facts.Model, view.Facts.Effort)
	}
	finishTurn(rt, "a-1", "done")
	if err := r.svc.Prompt(conv.ID, "two"); err != nil {
		t.Fatal(err)
	}
	reportModel(rt, "gpt-b", "low")
	finishTurn(rt, "a-2", "done")
	view = r.view(conv.ID)
	if view.Facts.Model != "gpt-b" || view.Facts.Effort != "low" {
		t.Errorf("after the next turn: live %q %q", view.Facts.Model, view.Facts.Effort)
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
	records := r.turnRecords(conv.ID)
	if len(records) != 2 || records[0] == nil || records[1] == nil {
		t.Fatalf("records = %v", records)
	}
	if got := records[0]; got.Asked != (model.TurnSettings{Model: "gpt-a", Effort: "medium"}) || got.Ran != got.Asked || len(got.Differs) != 0 {
		t.Errorf("the turn under way kept %+v", got)
	}
	if got := records[1]; got.Asked != (model.TurnSettings{Model: "gpt-b", Effort: "low"}) || got.Ran != got.Asked {
		t.Errorf("the next turn = %+v", got)
	}
}

// a turn that ran with other than it asked for says where: here codex ran
// another model and effort than the request, as a config pin or a harness
// fallback would make it.
func TestATurnRecordsWhereItRanWithOtherThanAsked(t *testing.T) {
	r, codex := modelRig(t)
	conv, err := r.svc.StartConversation(r.project.ID, model.HarnessCodex, "", "one", LaunchChoice{Model: "gpt-b", Effort: "high"})
	if err != nil {
		t.Fatal(err)
	}
	rt := codex.Last()
	startedOn(rt, "gpt-a", "low")
	// mid-turn, the view says what the turn asked for and what it runs with.
	rt.Emit(&model.Event{Kind: model.EventAssistant, Text: "working", NativeID: "a-0"})
	view := r.view(conv.ID)
	if view.TurnAsked == nil || view.TurnRan == nil || view.TurnAsked.Model != "gpt-b" || view.TurnRan.Model != "gpt-a" || !slices.Equal(view.TurnDiffers, []string{"model", "effort"}) {
		t.Fatalf("in flight: asked %+v, ran %+v", view.TurnAsked, view.TurnRan)
	}
	finishTurn(rt, "a-1", "done")
	records := r.turnRecords(conv.ID)
	if len(records) != 1 || records[0] == nil || !slices.Equal(records[0].Differs, []string{"model", "effort"}) {
		t.Fatalf("records = %+v", records)
	}
	if view := r.view(conv.ID); view.TurnAsked != nil || view.TurnRan != nil {
		t.Errorf("after the turn: asked %+v, ran %+v", view.TurnAsked, view.TurnRan)
	}
}

// a turn the harness reported nothing for carries no record: nothing says
// what it ran with.
func TestATurnWithNoReportCarriesNoRecord(t *testing.T) {
	r, _ := modelRig(t)
	conv := r.start("one")
	finishTurn(r.fake.Last(), "a-1", "done")
	if records := r.turnRecords(conv.ID); len(records) != 1 || records[0] != nil {
		t.Errorf("records = %+v", records)
	}
}

// a model is always required, for codex as for claude, and an effort for a
// model that takes one.
func TestAModelIsAlwaysRequired(t *testing.T) {
	r, codex := modelRig(t)
	conv, err := r.svc.StartConversation(r.project.ID, model.HarnessCodex, "", "one", LaunchChoice{Model: "gpt-b", Effort: "low"})
	if err != nil {
		t.Fatal(err)
	}
	rt := codex.Last()
	startedOn(rt, "gpt-b", "low")
	finishTurn(rt, "a-1", "done")
	for _, choice := range [][2]string{{"", ""}, {"", "low"}, {"gpt-a", ""}} {
		_, err := r.svc.SetModel(conv.ID, choice[0], choice[1])
		wantCode(t, err, CodeInvalid)
	}
	if got := rt.ModelChanges(); len(got) != 0 {
		t.Errorf("passed to the runtime: %v", got)
	}
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
	outcome, err := r.svc.SetModel(conv.ID, "sonnet", "low")
	if err != nil || outcome.Outcome != ModeRefused || outcome.Message != "not available" {
		t.Fatalf("outcome = %+v, %v", outcome, err)
	}
	if view := r.view(conv.ID); view.Model != "opus" || view.Effort != harnesstest.DefaultEffort {
		t.Errorf("after refusal: stored %q %q", view.Model, view.Effort)
	}

	rt.FailMode(fmt.Errorf("%w: no reply", harness.ErrSettingUnconfirmed))
	if outcome := r.setModel(conv.ID, "sonnet", "low"); outcome != ModeUnconfirmed {
		t.Fatalf("unconfirmed outcome = %q", outcome)
	}
	if view := r.view(conv.ID); view.Model != "sonnet" {
		t.Errorf("an unconfirmed request did not stand: %q", view.Model)
	}

	// claude accepted the model and refused the effort: the model stays
	// requested; the effort goes back to the last one accepted.
	rt.FailMode(fmt.Errorf("%w: claude refused effort 'high': not available", harness.ErrEffortRefused))
	outcome, err = r.svc.SetModel(conv.ID, "opus", "high")
	if err != nil || outcome.Outcome != ModeRefused {
		t.Fatalf("effort refused: %+v, %v", outcome, err)
	}
	if view := r.view(conv.ID); view.Model != "opus" || view.Effort != "low" {
		t.Errorf("after the effort was refused: stored %q %q", view.Model, view.Effort)
	}
	rt.FailMode(nil)
	if outcome := r.setModel(conv.ID, "sonnet", "low"); outcome != ModeConfirmed {
		t.Fatalf("back to sonnet: %q", outcome)
	}
}

func TestModelListIsCachedAndChoicesAreChecked(t *testing.T) {
	r, _ := modelRig(t)
	r.fake.SetCatalog(harness.Catalog{Version: "2.1.318", Models: []harness.ModelOption{
		{ID: "default", IsDefault: true, Efforts: []string{"low", "high"}},
		{ID: "haiku"},
	}})
	r.fake.SetSettings(model.TurnSettings{Model: "haiku", Mode: "plan"})
	for range 2 {
		list, err := r.svc.Models(r.project.ID, model.HarnessClaude)
		if err != nil {
			t.Fatal(err)
		}
		if list.Version != "2.1.318" || len(list.Models) != 2 || list.Copy != (model.TurnSettings{Model: "haiku", Mode: "plan"}) || list.NeedEffort {
			t.Errorf("list = %+v", list)
		}
	}
	if n := r.fake.ModelLists(); n != 1 {
		t.Errorf("the harness was asked for its models %d times", n)
	}
	// the settings are read afresh; the list is not.
	r.fake.SetSettings(model.TurnSettings{Model: "default", Effort: "high"})
	if list, _ := r.svc.Models(r.project.ID, model.HarnessClaude); list.Copy.Model != "default" || list.Copy.Effort != "high" {
		t.Errorf("the settings were cached: %+v", list.Copy)
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
	// while running, a model with no effort levels takes none; one with
	// levels needs one.
	_, err = r.svc.SetModel(conv.ID, "default", "")
	wantCode(t, err, CodeInvalid)
	if outcome := r.setModel(conv.ID, "haiku", ""); outcome != ModeConfirmed {
		t.Errorf("haiku: %q", outcome)
	}
	if view := r.view(conv.ID); view.Model != "haiku" || view.Effort != "" {
		t.Errorf("stored %q %q", view.Model, view.Effort)
	}
}

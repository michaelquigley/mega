package service

import (
	"slices"
	"testing"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/harness/harnesstest"
	"github.com/michaelquigley/mega/internal/model"
)

// restartWith closes the service and starts another over the same registry
// and logs, with both fakes, as Michael's next start would.
func (r *rig) restartWith(codex *harnesstest.Harness) {
	r.t.Helper()
	r.svc.Close()
	r.svc = newService(r.t, r.path, allTrusted, r.fake, codex)
}

// stopped runs a conversation's turn to its end, reporting what the harness
// runs, and releases it, so it rests with no runtime.
func (r *rig) stopped(id string, rt *harnesstest.Runtime, runningModel, effort, mode string) {
	r.t.Helper()
	rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: rt.NativeID(), Model: runningModel, Effort: effort, PermissionMode: mode}})
	finishTurn(rt, "a-"+id, "done")
	if err := r.svc.Release(id); err != nil {
		r.t.Fatal(err)
	}
	waitFor(r.t, "the runtime to exit", rt.Exited)
}

// unset makes a conversation as one recorded before every conversation held
// a concrete request: no model, effort or mode chosen.
func (r *rig) unset(id string) {
	r.t.Helper()
	r.svc.mu.Lock()
	defer r.svc.mu.Unlock()
	if err := r.svc.updateConversation(id, func(c *model.Conversation) { c.Model, c.Effort, c.PermissionMode = "", "", "" }); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) request(id string) model.TurnSettings {
	r.t.Helper()
	view := r.view(id)
	return model.TurnSettings{Model: view.Model, Effort: view.Effort, Mode: view.PermissionMode}
}

func (r *rig) fills(id string) []*model.Event {
	r.t.Helper()
	var out []*model.Event
	for _, ev := range r.events(id) {
		if ev.Kind == model.EventRuntime && ev.Status == model.StatusChoiceFilled {
			out = append(out, ev)
		}
	}
	return out
}

// a new conversation's request is copied once from the settings: a later
// change to the settings reaches only conversations created after it.
func TestNewConversationCopiesTheSettingsOnce(t *testing.T) {
	r, _ := modelRig(t)
	r.fake.SetSettings(model.TurnSettings{Model: "sonnet", Effort: "high", Mode: "acceptEdits"})
	conv := r.start("one")
	want := model.TurnSettings{Model: "sonnet", Effort: "high", Mode: "acceptEdits"}
	if got := r.request(conv.ID); got != want {
		t.Fatalf("copied %+v", got)
	}
	if spec := r.fake.Last().Spec; spec.Model != want.Model || spec.Effort != want.Effort || spec.PermissionMode != want.Mode {
		t.Errorf("passed %q %q %q", spec.Model, spec.Effort, spec.PermissionMode)
	}
	r.stopped(conv.ID, r.fake.Last(), "claude-sonnet-5-5", "", "acceptEdits")

	r.fake.SetSettings(model.TurnSettings{Model: "opus", Effort: "low", Mode: "plan"})
	if err := r.svc.Start(conv.ID); err != nil {
		t.Fatal(err)
	}
	if spec := r.fake.Last().Spec; spec.Model != "sonnet" || spec.Effort != "high" || spec.PermissionMode != "acceptEdits" {
		t.Errorf("a later settings change reached the conversation: %q %q %q", spec.Model, spec.Effort, spec.PermissionMode)
	}
	if got := r.request(r.start("two").ID); got != (model.TurnSettings{Model: "opus", Effort: "low", Mode: "plan"}) {
		t.Errorf("a conversation created after the change copied %+v", got)
	}
	if fills := r.fills(conv.ID); len(fills) != 0 {
		t.Errorf("a new conversation recorded a fill: %v", fills)
	}
}

// the copy rules, for each harness: a settings value mega offers is copied;
// one it does not offer takes the list's default; claude's effort has no
// default to take; a mode mega never passes becomes default.
func TestCopyRules(t *testing.T) {
	r, codex := modelRig(t)
	place := harness.Place{Cwd: r.project.Checkout}
	cases := []struct {
		name     string
		harness  string
		settings model.TurnSettings
		have     model.TurnSettings
		want     model.TurnSettings
		need     bool
	}{
		{"claude, all offered", model.HarnessClaude, model.TurnSettings{Model: "sonnet", Effort: "high", Mode: "plan"}, model.TurnSettings{}, model.TurnSettings{Model: "sonnet", Effort: "high", Mode: "plan"}, false},
		{"claude, nothing named", model.HarnessClaude, model.TurnSettings{}, model.TurnSettings{}, model.TurnSettings{Model: "default", Mode: "default"}, true},
		{"claude, a full id no row holds", model.HarnessClaude, model.TurnSettings{Model: "claude-opus-5-5", Effort: "low"}, model.TurnSettings{}, model.TurnSettings{Model: "default", Effort: "low", Mode: "default"}, false},
		{"claude, an alias the list lacks", model.HarnessClaude, model.TurnSettings{Model: "fable", Effort: "low"}, model.TurnSettings{}, model.TurnSettings{Model: "default", Effort: "low", Mode: "default"}, false},
		{"claude, an effort the model lacks", model.HarnessClaude, model.TurnSettings{Model: "sonnet", Effort: "max"}, model.TurnSettings{}, model.TurnSettings{Model: "sonnet", Mode: "default"}, true},
		{"claude, a model with no efforts", model.HarnessClaude, model.TurnSettings{Model: "haiku", Effort: "high"}, model.TurnSettings{}, model.TurnSettings{Model: "haiku", Mode: "default"}, false},
		{"claude, bypassPermissions", model.HarnessClaude, model.TurnSettings{Effort: "low", Mode: "bypassPermissions"}, model.TurnSettings{}, model.TurnSettings{Model: "default", Effort: "low", Mode: "default"}, false},
		{"claude, dontAsk", model.HarnessClaude, model.TurnSettings{Effort: "low", Mode: "dontAsk"}, model.TurnSettings{}, model.TurnSettings{Model: "default", Effort: "low", Mode: "default"}, false},
		{"claude, the operator's choice kept", model.HarnessClaude, model.TurnSettings{Model: "sonnet", Effort: "high", Mode: "plan"}, model.TurnSettings{Model: "opus", Mode: "auto"}, model.TurnSettings{Model: "opus", Effort: "high", Mode: "auto"}, false},
		{"codex, nothing named", model.HarnessCodex, model.TurnSettings{}, model.TurnSettings{}, model.TurnSettings{Model: "gpt-a", Effort: "medium"}, false},
		{"codex, all offered", model.HarnessCodex, model.TurnSettings{Model: "gpt-b", Effort: "high"}, model.TurnSettings{}, model.TurnSettings{Model: "gpt-b", Effort: "high"}, false},
		{"codex, a model the list lacks", model.HarnessCodex, model.TurnSettings{Model: "gpt-z", Effort: "xhigh"}, model.TurnSettings{}, model.TurnSettings{Model: "gpt-a", Effort: "medium"}, false},
	}
	for _, tc := range cases {
		fake := r.fake
		if tc.harness == model.HarnessCodex {
			fake = codex
		}
		fake.SetSettings(tc.settings)
		got, need, err := r.svc.copyFor(t.Context(), tc.harness, place, tc.have)
		if err != nil || got != tc.want || need != tc.need {
			t.Errorf("%s: %+v need %v (%v), want %+v need %v", tc.name, got, need, err, tc.want, tc.need)
		}
	}
}

// a mode mega never offers, named in the settings, is never passed: the new
// conversation runs in the manual mode.
func TestUnofferedSettingsModeBecomesDefault(t *testing.T) {
	for _, mode := range []string{"bypassPermissions", "dontAsk"} {
		r, _ := modelRig(t)
		r.fake.SetSettings(model.TurnSettings{Effort: "low", Mode: mode})
		conv := r.start("one")
		if spec := r.fake.Last().Spec; spec.PermissionMode != model.PermissionModeDefault || r.view(conv.ID).PermissionMode != model.PermissionModeDefault {
			t.Errorf("%s: passed %q, recorded %q", mode, spec.PermissionMode, r.view(conv.ID).PermissionMode)
		}
		if list, err := r.svc.Models(r.project.ID, model.HarnessClaude); err != nil || list.Copy.Mode != model.PermissionModeDefault {
			t.Errorf("%s: the list's copy %+v (%v)", mode, list.Copy, err)
		}
	}
}

// claude settings that name no effort give a model that takes one nothing to
// copy: a launch that chooses none is refused and leaves nothing behind, and
// the list says the effort must be chosen.
func TestClaudeLaunchWithNoEffortToCopyIsRefused(t *testing.T) {
	r, _ := modelRig(t)
	r.fake.SetSettings(model.TurnSettings{Model: "sonnet"})
	list, err := r.svc.Models(r.project.ID, model.HarnessClaude)
	if err != nil || !list.NeedEffort || list.Copy.Model != "sonnet" || list.Copy.Effort != "" {
		t.Fatalf("list copy %+v need %v (%v)", list.Copy, list.NeedEffort, err)
	}
	_, err = r.svc.StartConversation(r.project.ID, model.HarnessClaude, "", "one", LaunchChoice{})
	wantCode(t, err, CodeInvalid)
	if n := len(r.svc.Workspace().Conversations); n != 0 {
		t.Errorf("the refusal left %d conversations", n)
	}
	if n := len(r.fake.Runtimes()); n != 0 {
		t.Errorf("the refusal launched %d runtimes", n)
	}
	conv, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "", "one", LaunchChoice{Effort: "low"})
	if err != nil {
		t.Fatal(err)
	}
	if got := r.request(conv.ID); got != (model.TurnSettings{Model: "sonnet", Effort: "low", Mode: "default"}) {
		t.Errorf("with an effort chosen: %+v", got)
	}
	// a model that takes no effort needs none.
	if _, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "", "one", LaunchChoice{Model: "haiku"}); err != nil {
		t.Errorf("haiku: %v", err)
	}
}

// the start-up fill: a conversation recorded with no request takes what its
// log last reported, recorded once as a choice_filled event; a second start
// finds nothing to fill and records nothing.
func TestStartUpFillTakesWhatTheLogReportedOnce(t *testing.T) {
	r, codex := modelRig(t)
	codexConv, err := r.svc.StartConversation(r.project.ID, model.HarnessCodex, "", "one", LaunchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	crt := codex.Last()
	crt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: crt.NativeID(), Model: "gpt-a", Effort: "medium"}})
	reportModel(crt, "gpt-b", "high")
	finishTurn(crt, "a-1", "done")
	if err := r.svc.Release(codexConv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the codex runtime to exit", crt.Exited)

	claudeConv := r.start("two")
	r.stopped(claudeConv.ID, r.fake.Last(), "claude-opus-5-5", "", "plan")
	// a later mode report, and one mega never passes, is taken as default.
	chosen := r.start("three")
	rt := r.fake.Last()
	rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: rt.NativeID(), Model: "claude-sonnet-5-5", PermissionMode: "plan"}})
	rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusModeChanged, Facts: &model.RuntimeFacts{PermissionMode: "bypassPermissions"}})
	finishTurn(rt, "a-3", "done")
	if err := r.svc.Release(chosen.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the runtime to exit", rt.Exited)
	// one with a request already, which the fill leaves alone.
	kept := r.start("four")
	r.stopped(kept.ID, r.fake.Last(), "claude-opus-5-5", "", "default")
	keptBefore := r.request(kept.ID)

	for _, id := range []string{codexConv.ID, claudeConv.ID, chosen.ID} {
		r.unset(id)
	}
	r.restartWith(codex)

	want := map[string]model.TurnSettings{
		codexConv.ID:  {Model: "gpt-b", Effort: "high"},
		claudeConv.ID: {Model: "claude-opus-5-5", Mode: "plan"},
		chosen.ID:     {Model: "claude-sonnet-5-5", Mode: "default"},
	}
	for id, w := range want {
		if got := r.request(id); got != w {
			t.Errorf("%s filled %+v, want %+v", id, got, w)
		}
		fills := r.fills(id)
		if len(fills) != 1 || fills[0].Origin != model.OriginService || fills[0].Text != filledFromReport {
			t.Errorf("%s fills = %+v", id, fills)
		}
	}
	if got := r.request(kept.ID); got != keptBefore || len(r.fills(kept.ID)) != 0 {
		t.Errorf("a conversation with a request was filled: %+v, %d fills", got, len(r.fills(kept.ID)))
	}

	// the second start changes nothing.
	before := map[string]int{}
	for _, id := range []string{codexConv.ID, claudeConv.ID, chosen.ID, kept.ID} {
		before[id] = len(r.events(id))
	}
	r.restartWith(codex)
	for id, n := range before {
		if got := len(r.events(id)); got != n {
			t.Errorf("%s: the second start recorded %d events", id, got-n)
		}
	}
	for id, w := range want {
		if got := r.request(id); got != w {
			t.Errorf("%s after the second start: %+v", id, got)
		}
	}

	// claude never reports effort: the filled claude conversation is given
	// one at its next launch, from the settings, as the same kind of record.
	if err := r.svc.Start(claudeConv.ID); err != nil {
		t.Fatal(err)
	}
	if spec := r.fake.Last().Spec; spec.Model != "claude-opus-5-5" || spec.Effort != harnesstest.DefaultEffort || spec.PermissionMode != "plan" {
		t.Errorf("the launch passed %q %q %q", spec.Model, spec.Effort, spec.PermissionMode)
	}
	fills := r.fills(claudeConv.ID)
	if len(fills) != 2 || fills[1].Text != filledFromSettings || fills[1].Facts.Effort != harnesstest.DefaultEffort || fills[1].Facts.Model != "" {
		t.Errorf("fills at launch = %+v", fills)
	}
}

// a fill interrupted between its record and the registry write is applied
// at the next start from the record, and recorded no second time.
func TestInterruptedFillIsAppliedOnceFromItsRecord(t *testing.T) {
	r, codex := modelRig(t)
	conv, err := r.svc.StartConversation(r.project.ID, model.HarnessCodex, "", "one", LaunchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	rt := codex.Last()
	r.stopped(conv.ID, rt, "gpt-a", "medium", "")
	r.unset(conv.ID)
	// the record written, then the service gone before the registry write.
	r.svc.mu.Lock()
	l, err := r.svc.liveLocked(conv.ID)
	if err == nil {
		err = r.svc.record(l, &model.Event{Kind: model.EventRuntime, Status: model.StatusChoiceFilled, Origin: model.OriginService, Text: filledFromReport, Facts: &model.RuntimeFacts{Model: "gpt-b", Effort: "low"}})
	}
	r.svc.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	r.restartWith(codex)
	if got := r.request(conv.ID); got != (model.TurnSettings{Model: "gpt-b", Effort: "low"}) {
		t.Errorf("applied %+v, want the recorded fill", got)
	}
	if fills := r.fills(conv.ID); len(fills) != 1 {
		t.Errorf("fills = %d", len(fills))
	}
}

// a conversation whose log reported nothing is filled at its next launch
// from the settings, before anything is passed; claude settings with no
// effort refuse the launch until one is chosen, and the rest is filled.
func TestConversationWithNoReportIsFilledAtLaunch(t *testing.T) {
	r, codex := modelRig(t)
	conv, err := r.svc.Adopt(r.project.ID, model.HarnessClaude, "terminal-1", "")
	if err == nil {
		t.Fatal("adopted a session the fake never offered")
	}
	r.fake.SetSessions([]harness.Session{{NativeID: "terminal-1", Preview: "from the terminal"}})
	conv, err = r.svc.Adopt(r.project.ID, model.HarnessClaude, "terminal-1", "")
	if err != nil {
		t.Fatal(err)
	}
	r.unset(conv.ID)
	r.restartWith(codex)
	if got := r.request(conv.ID); got != (model.TurnSettings{}) || len(r.fills(conv.ID)) != 0 {
		t.Fatalf("the start-up fill had nothing to take, and took %+v", got)
	}
	r.fake.SetSettings(model.TurnSettings{Model: "sonnet", Mode: "auto"})
	err = r.svc.Start(conv.ID)
	wantCode(t, err, CodeInvalid)
	if n := len(r.fake.Runtimes()); n != 0 {
		t.Errorf("a refused launch started %d runtimes", n)
	}
	if got := r.request(conv.ID); got != (model.TurnSettings{Model: "sonnet", Mode: "auto"}) {
		t.Errorf("filled before the refusal: %+v", got)
	}
	if outcome := r.setModel(conv.ID, "sonnet", "high"); outcome != ModeStored {
		t.Fatalf("outcome = %q", outcome)
	}
	if err := r.svc.Start(conv.ID); err != nil {
		t.Fatal(err)
	}
	if spec := r.fake.Last().Spec; spec.Model != "sonnet" || spec.Effort != "high" || spec.PermissionMode != "auto" || spec.NativeID != "terminal-1" {
		t.Errorf("passed %q %q %q %q", spec.Model, spec.Effort, spec.PermissionMode, spec.NativeID)
	}
	fills := r.fills(conv.ID)
	if len(fills) != 1 || fills[0].Text != filledFromSettings {
		t.Errorf("fills = %+v", fills)
	}
}

// an adopted conversation copies its request at adoption, as a new one does.
func TestAdoptedConversationCopiesTheSettings(t *testing.T) {
	r, _ := modelRig(t)
	r.fake.SetSettings(model.TurnSettings{Model: "opus", Effort: "high", Mode: "plan"})
	r.fake.SetSessions([]harness.Session{{NativeID: "terminal-1", Preview: "from the terminal"}})
	conv, err := r.svc.Adopt(r.project.ID, model.HarnessClaude, "terminal-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := r.request(conv.ID); got != (model.TurnSettings{Model: "opus", Effort: "high", Mode: "plan"}) {
		t.Errorf("adopted with %+v", got)
	}
	if !slices.Equal(kinds(r.fills(conv.ID)), nil) {
		t.Errorf("adoption recorded a fill")
	}
}

// a claude conversation filled in from its log holds the full id claude
// reported, which the list holds only as an alias's resolved model: it takes
// that alias's effort levels, so an effort can be chosen for it on the bar.
func TestAFilledFullIdTakesItsAliasEfforts(t *testing.T) {
	r, codex := modelRig(t)
	conv := r.start("one")
	r.stopped(conv.ID, r.fake.Last(), "claude-sonnet-5-5", "", "default")
	r.unset(conv.ID)
	r.restartWith(codex)
	if got := r.request(conv.ID); got != (model.TurnSettings{Model: "claude-sonnet-5-5", Mode: "default"}) {
		t.Fatalf("filled %+v", got)
	}
	r.fake.SetSettings(model.TurnSettings{})
	wantCode(t, r.svc.Start(conv.ID), CodeInvalid)
	_, err := r.svc.SetModel(conv.ID, "claude-sonnet-5-5", "max")
	wantCode(t, err, CodeInvalid)
	if outcome := r.setModel(conv.ID, "claude-sonnet-5-5", "high"); outcome != ModeStored {
		t.Fatalf("outcome = %q", outcome)
	}
	if err := r.svc.Start(conv.ID); err != nil {
		t.Fatal(err)
	}
	if spec := r.fake.Last().Spec; spec.Model != "claude-sonnet-5-5" || spec.Effort != "high" {
		t.Errorf("passed %q %q", spec.Model, spec.Effort)
	}
}

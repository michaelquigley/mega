package service

import (
	"bytes"
	"errors"
	"os"
	"slices"
	"strings"
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
	if n := len(workspaceOf(t, r.svc).Conversations); n != 0 {
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
}

// a request that names the full id claude reports, which the list holds
// only as an alias's resolved model, is checked against that alias's row:
// its levels are the alias's, a launch with none is refused, and one of them
// can be chosen on the bar and is passed as it is.
func TestAFullIdIsCheckedAgainstItsAliasRow(t *testing.T) {
	r, _ := modelRig(t)
	conv := r.start("one")
	r.stopped(conv.ID, r.fake.Last(), "claude-sonnet-5-5", "", "default")
	r.svc.mu.Lock()
	err := r.svc.updateConversation(conv.ID, func(c *model.Conversation) { c.Model, c.Effort = "claude-sonnet-5-5", "" })
	r.svc.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Start(conv.ID); !errors.Is(err, errChooseEffort) {
		t.Fatalf("a full id whose alias takes levels launched with none: %v", err)
	}
	_, err = r.svc.SetModel(conv.ID, "claude-sonnet-5-5", "max")
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

// launches is how many runtimes a fake has launched.
func launches(h *harnesstest.Harness) int { return len(h.Runtimes()) }

// wantRefused is a launch refused with want before anything was passed, and
// recorded as any refused launch is: an error row, and the request as it was.
func (r *rig) wantRefused(id string, h *harnesstest.Harness, want error) {
	r.t.Helper()
	before, request, n := launches(h), r.request(id), len(r.events(id))
	if err := r.svc.Start(id); !errors.Is(err, want) {
		r.t.Fatalf("Start = %v, want %v", err, want)
	}
	if got := launches(h); got != before {
		r.t.Errorf("a refused launch started %d runtimes", got-before)
	}
	if got := r.request(id); got != request {
		r.t.Errorf("a refused launch changed the request to %+v", got)
	}
	evs := r.events(id)[n:]
	if i := slices.IndexFunc(evs, func(ev *model.Event) bool {
		return ev.Kind == model.EventRuntime && ev.Status == model.StatusError && strings.Contains(ev.Text, want.Error())
	}); i < 0 {
		r.t.Errorf("the refusal was not recorded: %+v", evs)
	}
}

// a conversation whose registry entry lacks a model, though its log says
// what the harness ran, is not filled at start-up: nothing is written to its
// log or the registry. a launch is refused with what to choose, and it
// launches once the operator has chosen it on the bar.
func TestAnIncompleteRequestIsNotFilled(t *testing.T) {
	r, codex := modelRig(t)
	conv := r.start("one")
	r.stopped(conv.ID, r.fake.Last(), "claude-opus-5-5", "", "plan")
	x, err := r.svc.StartConversation(r.project.ID, model.HarnessCodex, "", "two", LaunchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	r.stopped(x.ID, codex.Last(), "gpt-b", "high", "")
	r.unset(conv.ID)
	r.unset(x.ID)
	logged := map[string]int{conv.ID: len(r.events(conv.ID)), x.ID: len(r.events(x.ID))}
	r.svc.Close()
	registry, err := os.ReadFile(r.path)
	if err != nil {
		t.Fatal(err)
	}
	r.svc = newService(t, r.path, allTrusted, r.fake, codex)
	if after, err := os.ReadFile(r.path); err != nil || !bytes.Equal(after, registry) {
		t.Errorf("the start wrote the registry (%v)", err)
	}
	for id, n := range logged {
		if got := r.request(id); got != (model.TurnSettings{}) {
			t.Errorf("%s was filled with %+v", id, got)
		}
		if got := len(r.events(id)); got != n {
			t.Errorf("%s: the start recorded %d events", id, got-n)
		}
	}

	r.wantRefused(conv.ID, r.fake, errChooseModel)
	if outcome := r.setModel(conv.ID, "opus", "low"); outcome != ModeStored {
		t.Fatalf("outcome = %q", outcome)
	}
	r.wantRefused(conv.ID, r.fake, errChooseMode)
	if outcome := r.switchTo(conv.ID, "plan"); outcome != ModeStored {
		t.Fatalf("outcome = %q", outcome)
	}
	if err := r.svc.Start(conv.ID); err != nil {
		t.Fatal(err)
	}
	if spec := r.fake.Last().Spec; spec.Model != "opus" || spec.Effort != "low" || spec.PermissionMode != "plan" {
		t.Errorf("passed %q %q %q", spec.Model, spec.Effort, spec.PermissionMode)
	}

	r.wantRefused(x.ID, codex, errChooseModel)
	if outcome := r.setModel(x.ID, "gpt-a", "low"); outcome != ModeStored {
		t.Fatalf("outcome = %q", outcome)
	}
	if err := r.svc.Start(x.ID); err != nil {
		t.Fatal(err)
	}
	if spec := codex.Last().Spec; spec.Model != "gpt-a" || spec.Effort != "low" {
		t.Errorf("passed %q %q", spec.Model, spec.Effort)
	}
}

// a claude conversation adopted while its settings named no effort is
// refused at launch, as a new one would be. the settings are not read again:
// naming an effort there later changes nothing, and it launches once the
// operator chooses one on the bar.
func TestAClaudeConversationAdoptedWithNoEffortIsRefused(t *testing.T) {
	r, _ := modelRig(t)
	r.fake.SetSettings(model.TurnSettings{Model: "sonnet", Mode: "auto"})
	r.fake.SetSessions([]harness.Session{{NativeID: "terminal-1", Preview: "from the terminal"}})
	conv, err := r.svc.Adopt(r.project.ID, model.HarnessClaude, "terminal-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := r.request(conv.ID); got != (model.TurnSettings{Model: "sonnet", Mode: "auto"}) {
		t.Fatalf("adopted with %+v", got)
	}
	r.wantRefused(conv.ID, r.fake, errChooseEffort)
	r.fake.SetSettings(model.TurnSettings{Model: "sonnet", Effort: "high", Mode: "auto"})
	r.wantRefused(conv.ID, r.fake, errChooseEffort)
	if outcome := r.setModel(conv.ID, "sonnet", "low"); outcome != ModeStored {
		t.Fatalf("outcome = %q", outcome)
	}
	if err := r.svc.Start(conv.ID); err != nil {
		t.Fatal(err)
	}
	if spec := r.fake.Last().Spec; spec.Model != "sonnet" || spec.Effort != "low" || spec.PermissionMode != "auto" || spec.NativeID != "terminal-1" {
		t.Errorf("passed %q %q %q %q", spec.Model, spec.Effort, spec.PermissionMode, spec.NativeID)
	}
}

// after a restart no model list is held. a conversation on a model that
// takes no effort reads the list at its launch and launches; one that holds
// an effort launches without reading it.
func TestALaunchReadsTheListOnlyForARequestWithNoEffort(t *testing.T) {
	r, codex := modelRig(t)
	haiku, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "", "one", LaunchChoice{Model: "haiku"})
	if err != nil {
		t.Fatal(err)
	}
	r.stopped(haiku.ID, r.fake.Last(), "claude-haiku-4-5", "", "default")
	sonnet, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "", "two", LaunchChoice{Model: "sonnet", Effort: "low"})
	if err != nil {
		t.Fatal(err)
	}
	r.stopped(sonnet.ID, r.fake.Last(), "claude-sonnet-5-5", "", "default")
	r.restartWith(codex)

	lists := r.fake.ModelLists()
	if err := r.svc.Start(sonnet.ID); err != nil {
		t.Fatal(err)
	}
	if got := r.fake.ModelLists(); got != lists {
		t.Errorf("a request with an effort read the list %d times at its launch", got-lists)
	}
	if err := r.svc.Start(haiku.ID); err != nil {
		t.Fatal(err)
	}
	if got := r.fake.ModelLists(); got != lists+1 {
		t.Errorf("a request with no effort read the list %d times at its launch", got-lists)
	}
	if spec := r.fake.Last().Spec; spec.Model != "haiku" || spec.Effort != "" {
		t.Errorf("passed %q %q", spec.Model, spec.Effort)
	}
}

// a model list that has since given a conversation's model levels refuses
// its launch until the operator chooses one; the settings' effort is not
// copied. claude's refusal is errChooseEffort, the others' errChooseLevel.
func TestAListThatGivesTheModelLevelsRefusesTheLaunch(t *testing.T) {
	r, codex := modelRig(t)
	plain := codexCatalog()
	plain.Models = append(plain.Models, harness.ModelOption{ID: "gpt-plain"})
	codex.SetCatalog(plain)
	x, err := r.svc.StartConversation(r.project.ID, model.HarnessCodex, "", "one", LaunchChoice{Model: "gpt-plain"})
	if err != nil {
		t.Fatal(err)
	}
	r.stopped(x.ID, codex.Last(), "gpt-plain", "", "")
	haiku, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "", "two", LaunchChoice{Model: "haiku"})
	if err != nil {
		t.Fatal(err)
	}
	r.stopped(haiku.ID, r.fake.Last(), "claude-haiku-4-5", "", "default")

	levels := []string{"low", "high"}
	for i := range plain.Models {
		if plain.Models[i].ID == "gpt-plain" {
			plain.Models[i].Efforts = levels
		}
	}
	codex.SetCatalog(plain)
	codex.SetSettings(model.TurnSettings{Effort: "high"})
	claude := claudeCatalog()
	for i := range claude.Models {
		if claude.Models[i].ID == "haiku" {
			claude.Models[i].Efforts = levels
		}
	}
	r.fake.SetCatalog(claude)
	r.restartWith(codex)

	r.wantRefused(x.ID, codex, errChooseLevel)
	r.wantRefused(haiku.ID, r.fake, errChooseEffort)
	if outcome := r.setModel(haiku.ID, "haiku", "low"); outcome != ModeStored {
		t.Fatalf("outcome = %q", outcome)
	}
	if err := r.svc.Start(haiku.ID); err != nil {
		t.Fatal(err)
	}
}

// a launch whose request holds no effort, when the model list cannot be
// read, is refused: mega cannot tell whether the model takes one, and the
// settings' effort is not copied. launching again reads the list again. a
// request that holds an effort launches while the list cannot be read.
func TestAnUnreadListRefusesALaunchWithNoEffort(t *testing.T) {
	r, codex := modelRig(t)
	haiku, err := r.svc.StartConversation(r.project.ID, model.HarnessClaude, "", "one", LaunchChoice{Model: "haiku"})
	if err != nil {
		t.Fatal(err)
	}
	r.stopped(haiku.ID, r.fake.Last(), "claude-haiku-4-5", "", "default")
	sonnet := r.start("two")
	r.stopped(sonnet.ID, r.fake.Last(), "claude-sonnet-5-5", "", "default")
	r.restartWith(codex)

	r.fake.FailModels(errors.New("the model list failed"))
	r.wantRefused(haiku.ID, r.fake, errModelsUnread)
	if err := r.svc.Start(sonnet.ID); err != nil {
		t.Fatalf("a request with an effort: %v", err)
	}
	r.fake.FailModels(nil)
	lists := r.fake.ModelLists()
	if err := r.svc.Start(haiku.ID); err != nil {
		t.Fatal(err)
	}
	if got := r.fake.ModelLists(); got != lists+1 {
		t.Errorf("the launch after a failed list read it %d times", got-lists)
	}
	if spec := r.fake.Last().Spec; spec.Model != "haiku" || spec.Effort != "" {
		t.Errorf("passed %q %q", spec.Model, spec.Effort)
	}
}

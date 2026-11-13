package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

func TestLaunchPassesTheChosenModelAndReportsWhatCodexRuns(t *testing.T) {
	h := fakeHarness(t, "start")
	for _, tc := range []struct {
		spec harness.Spec
		want string
	}{
		{harness.Spec{Env: os.Environ(), Find: os.Environ(), Cwd: t.TempDir(), Model: "gpt-b", Effort: "low"}, "gpt-b"},
		{harness.Spec{Env: os.Environ(), Find: os.Environ(), Cwd: t.TempDir(), NativeID: "t-1", Model: "gpt-b"}, "gpt-b"},
	} {
		sink := &recordingSink{}
		rt, err := h.Launch(tc.spec, sink)
		if err != nil {
			t.Fatal(err)
		}
		started := sink.events[0]
		if started.Status != model.StatusStarted || started.Facts.Model != tc.want || started.Facts.Effort != "high" {
			t.Errorf("model %q, native id %q: started facts %+v", tc.spec.Model, tc.spec.NativeID, started.Facts)
		}
		r := rt.(*runtime)
		r.mu.Lock()
		chosenModel, chosenEffort := r.model, r.effort
		r.mu.Unlock()
		if chosenModel != tc.spec.Model || chosenEffort != tc.spec.Effort {
			t.Errorf("model %q, native id %q: kept for turns %q %q", tc.spec.Model, tc.spec.NativeID, chosenModel, chosenEffort)
		}
		_ = rt.Close()
	}
}

// codex takes a model and effort per turn: the choice goes with every
// turn/start, and a change is stored for the next one and sends nothing now.
func TestEveryTurnStartCarriesTheStoredChoice(t *testing.T) {
	r, _, w := testRuntime()
	prompt := func(text string) map[string]any {
		t.Helper()
		result := make(chan error, 1)
		go func() { result <- r.Prompt("svc-"+text, text) }()
		frame := r.reply(t, w, "turn/start", `{"turn":{"id":"turn-`+text+`","status":"inProgress","items":[]}}`)
		if err := <-result; err != nil {
			t.Fatal(err)
		}
		r.feed(`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-` + text + `","status":"completed","items":[]}}}`)
		return frame["params"].(map[string]any)
	}
	params := prompt("one")
	if params["model"] != "gpt-test" || params["effort"] != "medium" {
		t.Errorf("turn one carried %v %v", params["model"], params["effort"])
	}
	deferred, err := r.SetModel("gpt-b", "low")
	if err != nil || !deferred {
		t.Fatalf("change: deferred %v, %v", deferred, err)
	}
	w.quiet(t)
	for _, text := range []string{"two", "three"} {
		params = prompt(text)
		if params["model"] != "gpt-b" || params["effort"] != "low" {
			t.Errorf("turn %s carried %v %v", text, params["model"], params["effort"])
		}
	}
	// the service sends the whole choice; a change of effort alone carries
	// the model as it was.
	if _, err := r.SetModel("gpt-b", "medium"); err != nil {
		t.Fatal(err)
	}
	if params = prompt("four"); params["model"] != "gpt-b" || params["effort"] != "medium" {
		t.Errorf("effort change carried %v %v", params["model"], params["effort"])
	}
}

func TestSettingsUpdateIsTheLiveModelAndEffort(t *testing.T) {
	r, sink, _ := testRuntime()
	r.liveModel, r.liveEffort = "gpt-a", "high"
	update := func(m, e string) {
		r.feed(`{"method":"thread/settings/updated","params":{"threadId":"thread-1","threadSettings":{"model":"` + m + `","effort":"` + e + `"}}}`)
	}
	update("gpt-a", "high")
	update("gpt-a", "low")
	update("gpt-b", "low")
	var changes []string
	for _, ev := range sink.events {
		if ev.Kind == model.EventRuntime && ev.Status == model.StatusModelChanged {
			changes = append(changes, ev.Facts.Model+"/"+ev.Facts.Effort)
		}
	}
	if strings.Join(changes, ",") != "gpt-a/low,gpt-b/low" {
		t.Errorf("changes reported = %v", changes)
	}
}

func TestModelsListsWhatCodexOffersAndSettingsReadsItsConfig(t *testing.T) {
	h := fakeHarness(t, "models")
	catalog, err := h.Models(t.Context(), harness.Place{Cwd: t.TempDir(), Env: os.Environ(), Find: os.Environ()})
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Version != "0.166.0" || len(catalog.Models) != 2 {
		t.Fatalf("catalog = %+v", catalog)
	}
	a, b := catalog.Models[0], catalog.Models[1]
	if a.ID != "gpt-a" || !a.IsDefault || a.DefaultEffort != "medium" || strings.Join(a.Efforts, ",") != "low,medium" {
		t.Errorf("a = %+v", a)
	}
	if b.ID != "gpt-b" || strings.Join(b.Efforts, ",") != "low" {
		t.Errorf("b = %+v", b)
	}
	settings, err := h.Settings(t.Context(), harness.Place{Cwd: t.TempDir(), Env: os.Environ(), Find: os.Environ()})
	if err != nil || settings != (model.TurnSettings{Model: "gpt-b", Effort: "low"}) {
		t.Errorf("settings %+v %v", settings, err)
	}
}

// every launch and every turn passes the conversation's request as it
// stands: the model on thread/start and thread/resume, and the model and
// effort on each turn/start.
func TestEveryLaunchAndTurnPassesTheRequestWithNoConfigRead(t *testing.T) {
	h := fakeHarness(t, "start")
	frames := filepath.Join(t.TempDir(), "frames")
	t.Setenv(fakeFrames, frames)
	cwd := t.TempDir()
	run := func(spec harness.Spec) {
		t.Helper()
		rt, err := h.Launch(spec, &recordingSink{})
		if err != nil {
			t.Fatal(err)
		}
		if err := rt.Prompt("svc-1", "hello"); err != nil {
			t.Fatal(err)
		}
		_ = rt.Close()
	}
	sent := func(method string) []map[string]any {
		t.Helper()
		data, err := os.ReadFile(frames)
		if err != nil {
			t.Fatal(err)
		}
		var params []map[string]any
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			frame := map[string]any{}
			if err := json.Unmarshal([]byte(line), &frame); err != nil {
				t.Fatal(err)
			}
			if frame["method"] == method {
				params = append(params, frame["params"].(map[string]any))
			}
		}
		return params
	}

	run(harness.Spec{Env: os.Environ(), Find: os.Environ(), Cwd: cwd, Model: "gpt-a", Effort: "high"})
	if starts := sent("thread/start"); len(starts) != 1 || starts[0]["model"] != "gpt-a" {
		t.Fatalf("thread/start = %v", starts)
	}
	if turns := sent("turn/start"); len(turns) != 1 || turns[0]["model"] != "gpt-a" || turns[0]["effort"] != "high" {
		t.Fatalf("the overriding turn = %v", turns)
	}

	run(harness.Spec{Env: os.Environ(), Find: os.Environ(), Cwd: cwd, NativeID: "t-1", Model: "gpt-b", Effort: "low"})
	if resumes := sent("thread/resume"); len(resumes) != 1 || resumes[0]["model"] != "gpt-b" {
		t.Errorf("thread/resume = %v", resumes)
	}
	if turns := sent("turn/start"); len(turns) != 2 || turns[1]["model"] != "gpt-b" || turns[1]["effort"] != "low" {
		t.Errorf("the resumed turn = %v", turns[len(turns)-1])
	}
	// the request is all that is passed: the config is never read to fill a
	// launch or a turn.
	if reads := sent("config/read"); len(reads) != 0 {
		t.Errorf("config/read sent %d times", len(reads))
	}
}

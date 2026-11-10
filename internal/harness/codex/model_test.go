package codex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.hq.quigley.com/research/roundhouse/internal/harness"
	"git.hq.quigley.com/research/roundhouse/internal/model"
)

func TestLaunchPassesTheChosenModelAndReportsWhatCodexRuns(t *testing.T) {
	h := fakeHarness(t, "start")
	for _, tc := range []struct {
		spec harness.Spec
		want string
	}{
		{harness.Spec{Cwd: t.TempDir(), Model: "gpt-b", Effort: "low"}, "gpt-b"},
		{harness.Spec{Cwd: t.TempDir()}, "gpt-b"}, // inherit sends the config's model
		{harness.Spec{Cwd: t.TempDir(), NativeID: "t-1", Model: "gpt-b"}, "gpt-b"},
	} {
		sink := &recordingSink{}
		rt, err := h.Launch(tc.spec, sink)
		if err != nil {
			t.Fatal(err)
		}
		started := sink.events[0]
		if started.Status != model.StatusStarted || started.Facts.Model != tc.want || started.Facts.Effort != "high" {
			t.Errorf("%+v: started facts %+v", tc.spec, started.Facts)
		}
		r := rt.(*runtime)
		r.mu.Lock()
		chosenModel, chosenEffort := r.model, r.effort
		r.mu.Unlock()
		if chosenModel != tc.spec.Model || chosenEffort != tc.spec.Effort {
			t.Errorf("%+v: kept for turns %q %q", tc.spec, chosenModel, chosenEffort)
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

func TestModelsListsWhatCodexOffersAndInheritedReadsItsConfig(t *testing.T) {
	h := fakeHarness(t, "models")
	catalog, err := h.Models(t.Context(), t.TempDir())
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
	m, e, err := h.Inherited(t.Context(), t.TempDir())
	if err != nil || m != "gpt-b" || e != "low" {
		t.Errorf("inherited %q %q %v", m, e, err)
	}
}

// inherit is sent as the config's current values, read afresh for every
// turn: a change to the config shows at the next turn, a chosen field stays
// chosen, and a config that cannot be read sends nothing for the inherited
// fields.
func TestInheritSendsTheConfigReadAfreshForEveryTurn(t *testing.T) {
	r, _, w := testRuntime()
	r.model, r.effort = "", ""
	prompt := func(text, config string) map[string]any {
		t.Helper()
		result := make(chan error, 1)
		go func() { result <- r.Prompt("svc-"+text, text) }()
		read := w.sent(t)
		if read["method"] != "config/read" || read["params"].(map[string]any)["cwd"] != "/f" {
			t.Fatalf("sent %v before the turn", read)
		}
		r.feed(fmt.Sprintf(`{"id":%v,%s}`, read["id"], config))
		frame := r.reply(t, w, "turn/start", `{"turn":{"id":"turn-`+text+`","status":"inProgress","items":[]}}`)
		if err := <-result; err != nil {
			t.Fatal(err)
		}
		r.feed(`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-` + text + `","status":"completed","items":[]}}}`)
		return frame["params"].(map[string]any)
	}
	if p := prompt("one", `"result":{"config":{"model":"gpt-b","model_reasoning_effort":"low"}}`); p["model"] != "gpt-b" || p["effort"] != "low" {
		t.Errorf("turn one carried %v %v", p["model"], p["effort"])
	}
	if p := prompt("two", `"result":{"config":{"model":"gpt-a","model_reasoning_effort":"high"}}`); p["model"] != "gpt-a" || p["effort"] != "high" {
		t.Errorf("after the config changed, turn two carried %v %v", p["model"], p["effort"])
	}
	r.model = "gpt-c"
	if p := prompt("three", `"result":{"config":{"model":"gpt-a","model_reasoning_effort":"high"}}`); p["model"] != "gpt-c" || p["effort"] != "high" {
		t.Errorf("a chosen model with inherited effort carried %v %v", p["model"], p["effort"])
	}
	r.model = ""
	p := prompt("four", `"error":{"code":-32000,"message":"no config"}`)
	for _, key := range []string{"model", "effort"} {
		if _, ok := p[key]; ok {
			t.Errorf("with the config unreadable, %s was sent: %v", key, p[key])
		}
	}
}

// a thread that ran an explicit override and is set back to inherit returns
// to the config: the resume carries the config's model and the next turn its
// model and effort, where passing nothing would keep the override
// (capability report, 2026-11-10).
func TestOverriddenThreadSetBackToInheritSendsTheConfig(t *testing.T) {
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

	run(harness.Spec{Cwd: cwd, Model: "gpt-a", Effort: "high"})
	if starts := sent("thread/start"); len(starts) != 1 || starts[0]["model"] != "gpt-a" {
		t.Fatalf("thread/start = %v", starts)
	}
	if turns := sent("turn/start"); len(turns) != 1 || turns[0]["model"] != "gpt-a" || turns[0]["effort"] != "high" {
		t.Fatalf("the overriding turn = %v", turns)
	}

	run(harness.Spec{Cwd: cwd, NativeID: "t-1"})
	if resumes := sent("thread/resume"); len(resumes) != 1 || resumes[0]["model"] != "gpt-b" {
		t.Errorf("thread/resume set back to inherit = %v", resumes)
	}
	if turns := sent("turn/start"); len(turns) != 2 || turns[1]["model"] != "gpt-b" || turns[1]["effort"] != "low" {
		t.Errorf("the next turn set back to inherit = %v", turns[len(turns)-1])
	}
}

// a running thread set back to inherit sends the config's values from its
// next turn, as an inherit launch would: an empty field is inherit, not "as
// before".
func TestRunningThreadSetBackToInheritSendsTheConfigNextTurn(t *testing.T) {
	r, _, w := testRuntime()
	if _, err := r.SetModel("", ""); err != nil {
		t.Fatal(err)
	}
	w.quiet(t)
	result := make(chan error, 1)
	go func() { result <- r.Prompt("svc-1", "next") }()
	read := w.sent(t)
	if read["method"] != "config/read" {
		t.Fatalf("sent %v before the turn", read["method"])
	}
	r.feed(fmt.Sprintf(`{"id":%v,"result":{"config":{"model":"gpt-a","model_reasoning_effort":"high"}}}`, read["id"]))
	frame := r.reply(t, w, "turn/start", `{"turn":{"id":"turn-1","status":"inProgress","items":[]}}`)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if p := frame["params"].(map[string]any); p["model"] != "gpt-a" || p["effort"] != "high" {
		t.Errorf("the turn after inherit carried %v %v", p["model"], p["effort"])
	}
}

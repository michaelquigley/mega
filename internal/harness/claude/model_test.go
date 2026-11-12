package claude

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

func TestLaunchPassesTheChosenModelAndEffortOnlyWhenChosen(t *testing.T) {
	h, cwd := settingsHarness(t)
	args := launchArgsFor(t, h, harness.Spec{Env: os.Environ(), Cwd: cwd, Model: "sonnet", Effort: "low"})
	if got, ok := argValue(args, "--model"); !ok || got != "sonnet" {
		t.Errorf("--model %q (%v) in %v", got, ok, args)
	}
	if got, ok := argValue(args, "--effort"); !ok || got != "low" {
		t.Errorf("--effort %q (%v) in %v", got, ok, args)
	}
	args = launchArgsFor(t, h, harness.Spec{Env: os.Environ(), Cwd: cwd})
	for _, name := range []string{"--model", "--effort"} {
		if got, ok := argValue(args, name); ok {
			t.Errorf("unchosen, yet passed %s %q", name, got)
		}
	}
}

// settingWith changes the model and effort on another goroutine and plays the
// shim's replies, in order, as each frame is sent.
func settingWith(t *testing.T, r *runtime, chosenModel, effort string, replies ...string) (bool, error) {
	t.Helper()
	type result struct {
		deferred bool
		err      error
	}
	done := make(chan result, 1)
	go func() {
		deferred, err := r.SetModel(chosenModel, effort)
		done <- result{deferred, err}
	}()
	for _, reply := range replies {
		deadline := time.Now().Add(5 * time.Second)
		for {
			r.mu.Lock()
			waiting := len(r.modes) > 0
			r.mu.Unlock()
			if waiting {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("no frame was sent for %s", reply)
			}
			time.Sleep(time.Millisecond)
		}
		r.handle([]byte(reply))
	}
	res := <-done
	return res.deferred, res.err
}

func TestModelAndEffortFramesWaitForTheShimsReplies(t *testing.T) {
	r, _, stdin := testRuntime()
	deferred, err := settingWith(t, r, "sonnet", "low",
		`{"type":"model_set","id":"model-1","model":"sonnet"}`,
		`{"type":"effort_set","id":"effort-2","effort":"low"}`)
	if err != nil || deferred {
		t.Fatalf("change: deferred %v, %v", deferred, err)
	}
	sent := stdin.String()
	if !strings.Contains(sent, `{"id":"model-1","model":"sonnet","type":"set_model"}`) || !strings.Contains(sent, `{"effort":"low","id":"effort-2","type":"set_effort"}`) {
		t.Errorf("frames sent: %s", sent)
	}
	if r.reqModel != "sonnet" || r.reqEffort != "low" {
		t.Errorf("after confirmation: %q %q", r.reqModel, r.reqEffort)
	}

	// only what differs is sent.
	stdin.Reset()
	if _, err := settingWith(t, r, "sonnet", "high", `{"type":"effort_set","id":"effort-3","effort":"high"}`); err != nil {
		t.Fatal(err)
	}
	if sent := stdin.String(); strings.Contains(sent, "set_model") || !strings.Contains(sent, `"effort":"high"`) {
		t.Errorf("an unchanged model was sent again: %s", sent)
	}

	_, err = settingWith(t, r, "nonesuch", "", `{"type":"model_set","id":"model-4","model":"nonesuch","error":"not available"}`)
	if err == nil || !strings.Contains(err.Error(), "not available") {
		t.Errorf("refusal: %v", err)
	}
	if r.reqModel != "sonnet" {
		t.Errorf("a refused model moved the request to %q", r.reqModel)
	}

	r.gone = true
	if _, err := r.SetModel("opus", ""); !errors.Is(err, errExited) {
		t.Errorf("change after exit: %v", err)
	}
}

func TestUnansweredModelChangeIsUnconfirmed(t *testing.T) {
	r, _, _ := testRuntime()
	saved := modeTimeout
	modeTimeout = 20 * time.Millisecond
	defer func() { modeTimeout = saved }()
	if _, err := r.SetModel("sonnet", ""); !errors.Is(err, harness.ErrSettingUnconfirmed) {
		t.Fatalf("timed out: %v", err)
	}
	r.handle([]byte(`{"type":"model_set","id":"model-1","model":"sonnet"}`))
	if r.reqModel != "sonnet" {
		t.Errorf("a late confirmation was not recorded: %q", r.reqModel)
	}
}

// the live model is claude's own report: the first init's model is the
// started fact, and a later init with another model is a model change.
func TestInitReportsTheModelAndItsChanges(t *testing.T) {
	r, sink, _ := testRuntime()
	init := func(reported string) {
		r.handle([]byte(`{"type":"event","msg":{"type":"system","subtype":"init","session_id":"edfe4ade-9f59-4db3-89bc-75fcc8b52c44","model":"` + reported + `","permissionMode":"default"}}`))
	}
	init("claude-opus-5-5")
	if started := sink.events[0]; started.Status != model.StatusStarted || started.Facts.Model != "claude-opus-5-5" {
		t.Fatalf("started = %+v", started.Facts)
	}
	init("claude-opus-5-5")
	init("claude-sonnet-5-5")
	var changes []string
	for _, ev := range sink.events {
		if ev.Kind == model.EventRuntime && ev.Status == model.StatusModelChanged {
			changes = append(changes, ev.Facts.Model)
		}
	}
	if strings.Join(changes, ",") != "claude-sonnet-5-5" {
		t.Errorf("model changes reported = %v", changes)
	}
}

func TestInheritedReadsTheFirstLayerNamingEach(t *testing.T) {
	h, cwd := settingsHarness(t)
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if m, e, err := h.Inherited(t.Context(), harness.Place{Cwd: cwd, Env: os.Environ()}); err != nil || m != "" || e != "" {
		t.Fatalf("no settings: %q %q %v", m, e, err)
	}
	write(filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), "settings.json"), `{"model":"opus","effortLevel":"high"}`)
	write(filepath.Join(cwd, ".claude", "settings.json"), `{"model":"sonnet"}`)
	if m, e, _ := h.Inherited(t.Context(), harness.Place{Cwd: cwd, Env: os.Environ()}); m != "sonnet" || e != "high" {
		t.Errorf("project model over user effort: %q %q", m, e)
	}
}

// the models command runs the shim with the checkout and executable, and its
// one line is the catalog.
func TestModelsReadsTheShimsCatalog(t *testing.T) {
	h, cwd := settingsHarness(t)
	shim := t.TempDir()
	if err := os.WriteFile(filepath.Join(shim, shimScript), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(shim, "node_modules", "@anthropic-ai", "claude-agent-sdk"), 0o700); err != nil {
		t.Fatal(err)
	}
	node := filepath.Join(t.TempDir(), "node")
	line := `{"version":"2.1.318","models":[{"value":"default","resolvedModel":"claude-opus-5-5","displayName":"Default","supportedEffortLevels":["low","high"]},{"value":"haiku","resolvedModel":"claude-haiku-4-5-20251001","displayName":"Haiku"}]}`
	script := "#!/bin/sh\ncase \"$*\" in *\"models --cwd " + cwd + " --exec /x\"*) printf '%s\\n' '" + line + "' ;; *) exit 3 ;; esac\n"
	if err := os.WriteFile(node, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	h.cfg.Node, h.cfg.Shim, h.cfg.Executable = node, shim, "/x"
	catalog, err := h.Models(t.Context(), harness.Place{Cwd: cwd, Env: os.Environ()})
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Version != "2.1.318" || len(catalog.Models) != 2 {
		t.Fatalf("catalog = %+v", catalog)
	}
	first, second := catalog.Models[0], catalog.Models[1]
	if first.ID != "default" || !first.IsDefault || first.Resolved != "claude-opus-5-5" || strings.Join(first.Efforts, ",") != "low,high" {
		t.Errorf("first = %+v", first)
	}
	if second.ID != "haiku" || second.IsDefault || len(second.Efforts) != 0 {
		t.Errorf("second = %+v", second)
	}
}

// an effort refused after the model was accepted says so: the model changed
// and the effort did not.
func TestEffortRefusedAfterTheModelChangedSaysSo(t *testing.T) {
	r, _, _ := testRuntime()
	r.reqModel, r.reqEffort = "opus", "high"
	_, err := settingWith(t, r, "sonnet", "low",
		`{"type":"model_set","id":"model-1","model":"sonnet"}`,
		`{"type":"effort_set","id":"effort-2","effort":"low","error":"not available"}`)
	if !errors.Is(err, harness.ErrEffortRefused) || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("refusal: %v", err)
	}
	if r.reqModel != "sonnet" || r.reqEffort != "high" {
		t.Errorf("after: %q %q", r.reqModel, r.reqEffort)
	}
	// an effort refused alone is a plain refusal.
	_, err = settingWith(t, r, "sonnet", "low", `{"type":"effort_set","id":"effort-3","effort":"low","error":"not available"}`)
	if err == nil || errors.Is(err, harness.ErrEffortRefused) {
		t.Errorf("effort alone: %v", err)
	}
}

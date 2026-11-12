package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michaelquigley/mega/internal/model"
)

func TestTheEnvironmentOnTheWire(t *testing.T) {
	f := start(t)
	status, project := f.do(t, http.MethodPost, "/api/projects", createBody("p", f.checkout))
	if status != http.StatusCreated {
		t.Fatalf("create project: %d %v", status, project)
	}
	pid := project["id"].(string)
	if err := os.MkdirAll(filepath.Join(f.checkout, ".mega"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.checkout, ".mega", "environment.yaml"), []byte("activate: export MEGA_MARKER=wire-value-5d1e\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := func() map[string]any {
		t.Helper()
		_, workspace := f.do(t, http.MethodGet, "/api/workspace", "")
		return workspace["projects"].([]any)[0].(map[string]any)["environment"].(map[string]any)
	}
	e := env()
	file := e["file"].(map[string]any)
	if e["status"] != "unapproved" || e["source"] != "file" || e["refusal"].(map[string]any)["code"] != "environment_unapproved" || file["present"] != true || !strings.Contains(file["content"].(string), "MEGA_MARKER") {
		t.Fatalf("unapproved: %v", e)
	}
	launch := func() (int, map[string]any) {
		return f.do(t, http.MethodPost, "/api/projects/"+pid+"/conversations", jsonBody(map[string]string{"harness": "claude", "prompt": "hello"}))
	}
	if status, body := launch(); status != http.StatusConflict || body["code"] != "environment_unapproved" {
		t.Errorf("launch while unapproved: %d %v", status, body)
	}

	// approving another version is refused; the shown one is accepted.
	base := "/api/projects/" + pid + "/environment"
	if status, body := f.do(t, http.MethodPost, base+"/approve", jsonBody(map[string]string{"digest": "sha256:" + strings.Repeat("0", 64)})); status != http.StatusConflict {
		t.Errorf("approving another version: %d %v", status, body)
	}
	if status, body := f.do(t, http.MethodPost, base+"/approve", jsonBody(map[string]string{"digest": file["digest"].(string)})); status != http.StatusOK {
		t.Fatalf("approve: %d %v", status, body)
	}
	if e := env(); e["status"] != "ready" || e["approved"].(map[string]any)["digest"] != file["digest"] {
		t.Errorf("approved: %v", e)
	}
	// approving resolved at once and recorded the names it changes.
	if a := env()["approved"].(map[string]any); a["names_known"] != true || fmt.Sprint(a["changed"]) != "[MEGA_MARKER]" || !strings.HasPrefix(a["fingerprint"].(string), "sha256:") || len(a["fingerprint"].(string)) != len("sha256:")+16 {
		t.Errorf("the approval's names and short form: %v", a)
	}
	if status, body := f.do(t, http.MethodGet, base+"/approved", ""); status != http.StatusOK || body["available"] != false {
		t.Errorf("the approved version outside git: %d %v", status, body)
	}

	// a live resolution: names and a fingerprint, and values only when asked.
	status, res := f.do(t, http.MethodPost, base+"/resolve", "")
	if status != http.StatusOK || fmt.Sprint(res["changed"]) != "[MEGA_MARKER]" || res["values"] != nil || !strings.HasPrefix(res["fingerprint"].(string), "sha256:") {
		t.Errorf("resolve: %d %v", status, res)
	}
	status, res = f.do(t, http.MethodPost, base+"/resolve?values=1", "")
	if status != http.StatusOK || res["values"].(map[string]any)["MEGA_MARKER"] != "wire-value-5d1e" {
		t.Errorf("resolve with values: %d %v", status, res)
	}
	// the resolution's names are on the workspace read; its values are not.
	if e := env(); fmt.Sprint(e["resolved"].(map[string]any)["changed"]) != "[MEGA_MARKER]" || strings.Contains(fmt.Sprint(e["resolved"]), "wire-value-5d1e") {
		t.Errorf("the workspace's resolution: %v", e["resolved"])
	}

	// a launch records names and a fingerprint, shown on the conversation.
	status, conv := launch()
	if status != http.StatusCreated {
		t.Fatalf("launch: %d %v", status, conv)
	}
	rt := f.fake.Last()
	rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: rt.NativeID()}})
	c := f.conversation(t, conv["id"].(string))
	runtimeEnv, _ := c["environment"].(map[string]any)
	if runtimeEnv == nil || runtimeEnv["source"] != "file" || fmt.Sprint(runtimeEnv["changed"]) != "[MEGA_MARKER]" || c["environment_stale"] != nil {
		t.Errorf("the conversation's environment: %v", c)
	}

	// an override, then clearing it.
	if status, body := f.do(t, http.MethodPut, base+"/override", jsonBody(map[string]any{"ignore": true})); status != http.StatusOK {
		t.Fatalf("override: %d %v", status, body)
	}
	if e := env(); e["source"] != "ignored" || e["override"].(map[string]any)["ignore"] != true {
		t.Errorf("ignored: %v", e)
	}
	if c := f.conversation(t, conv["id"].(string)); c["environment_stale"] != true {
		t.Errorf("an override does not show the running conversation as older: %v", c["environment_stale"])
	}
	if status, body := f.do(t, http.MethodPut, base+"/override", jsonBody(map[string]any{"ignore": true, "activate": "x"})); status != http.StatusBadRequest {
		t.Errorf("an override that does both: %d %v", status, body)
	}
	if status, body := f.do(t, http.MethodDelete, base+"/override", ""); status != http.StatusOK {
		t.Fatalf("clear override: %d %v", status, body)
	}
	if e := env(); e["source"] != "file" || e["override"] != nil {
		t.Errorf("cleared: %v", e)
	}

	// a changed file refuses, with its own code.
	if err := os.WriteFile(filepath.Join(f.checkout, ".mega", "environment.yaml"), []byte("activate: export MEGA_MARKER=other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if status, body := launch(); status != http.StatusConflict || body["code"] != "environment_changed" || !strings.Contains(body["message"].(string), ".mega/environment.yaml") {
		t.Errorf("launch after a change: %d %v", status, body)
	}
}

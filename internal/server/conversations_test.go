package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/harness/harnesstest"
	"github.com/michaelquigley/mega/internal/model"
)

func jsonBody(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}

// startConversation registers the fixture checkout and launches a conversation
// through the API, returning its id and the fake runtime behind it.
func (f *fixture) startConversation(t *testing.T) (string, *harnesstest.Runtime) {
	t.Helper()
	status, project := f.do(t, http.MethodPost, "/api/projects", createBody("p", f.checkout))
	if status != http.StatusCreated {
		t.Fatalf("create project: %d %v", status, project)
	}
	path := fmt.Sprintf("/api/projects/%s/conversations", project["id"])
	status, conv := f.do(t, http.MethodPost, path, jsonBody(map[string]string{"harness": "claude", "prompt": "Reply with ALPHA"}))
	if status != http.StatusCreated {
		t.Fatalf("start conversation: %d %v", status, conv)
	}
	// starting until the runtime reports the session it holds.
	if conv["state"] != "starting" || conv["turn_active"] != true || conv["name"] != "Reply with ALPHA" {
		t.Errorf("conversation = %v", conv)
	}
	return conv["id"].(string), f.fake.Last()
}

func (f *fixture) events(t *testing.T, id string, since int) []map[string]any {
	t.Helper()
	status, body := f.do(t, http.MethodGet, fmt.Sprintf("/api/conversations/%s/events?since=%d", id, since), "")
	if status != http.StatusOK {
		t.Fatalf("events: %d %v", status, body)
	}
	var out []map[string]any
	for _, ev := range body["events"].([]any) {
		out = append(out, ev.(map[string]any))
	}
	return out
}

func (f *fixture) conversation(t *testing.T, id string) map[string]any {
	t.Helper()
	_, workspace := f.do(t, http.MethodGet, "/api/workspace", "")
	for _, conv := range workspace["conversations"].([]any) {
		if c := conv.(map[string]any); c["id"] == id {
			return c
		}
	}
	t.Fatalf("no conversation %s", id)
	return nil
}

func pendingIDs(conv map[string]any) []string {
	var ids []string
	for _, req := range conv["pending"].([]any) {
		ids = append(ids, req.(map[string]any)["id"].(string))
	}
	return ids
}

// a browser that drops and reconnects while the service and runtime live
// replays what it missed from its last seq, sees exactly the requests still
// pending, sees a resolved one only as history, and resubmits nothing.
func TestBrowserReconnectReplaysAndShowsCurrentPendingSet(t *testing.T) {
	f := start(t)
	id, rt := f.startConversation(t)
	first := dialStream(t, f)
	lastSeen := len(f.events(t, id, 0))
	first.Close()

	rt.Emit(&model.Event{Kind: model.EventAssistant, Text: "working", NativeID: "a-1"})
	stillPending := rt.Ask(model.RequestQuestion, map[string]any{"questions": []any{map[string]any{"question": "Which?"}}})
	resolved := rt.Ask(model.RequestPermission, map[string]any{"tool": "Bash"})
	if status, body := f.do(t, http.MethodPost, "/api/conversations/"+id+"/answer", jsonBody(map[string]string{"request_id": resolved, "decision": "allow"})); status != http.StatusAccepted {
		t.Fatalf("answer: %d %v", status, body)
	}

	dialStream(t, f)
	missed := f.events(t, id, lastSeen)
	var got []string
	for _, ev := range missed {
		got = append(got, ev["kind"].(string))
	}
	if !slices.Equal(got, []string{"assistant", "request", "request", "request_resolved"}) {
		t.Errorf("replayed kinds = %v", got)
	}
	if ids := pendingIDs(f.conversation(t, id)); !slices.Equal(ids, []string{stillPending}) {
		t.Errorf("pending = %v, want only %s", ids, stillPending)
	}
	if status, body := f.do(t, http.MethodPost, "/api/conversations/"+id+"/answer", jsonBody(map[string]string{"request_id": resolved, "decision": "allow"})); status != http.StatusConflict || body["code"] != "not_pending" {
		t.Errorf("late answer: %d %v", status, body)
	}
	if len(rt.Prompts()) != 1 || len(rt.Deliveries()) != 1 {
		t.Errorf("resubmitted: %d prompts, %d deliveries", len(rt.Prompts()), len(rt.Deliveries()))
	}
}

func TestStreamCarriesEventsAndDeltas(t *testing.T) {
	f := start(t)
	id, rt := f.startConversation(t)
	conn := dialStream(t, f)
	rt.Delta("AL")
	rt.Emit(&model.Event{Kind: model.EventAssistant, Text: "ALPHA", NativeID: "a-1"})
	var sawDelta, sawEvent bool
	for !(sawDelta && sawEvent) {
		var frame map[string]any
		if err := conn.ReadJSON(&frame); err != nil {
			t.Fatal(err)
		}
		if frame["conversation_id"] != nil && frame["conversation_id"] != id {
			t.Fatalf("frame for another conversation: %v", frame)
		}
		if frame["delta"] == "AL" {
			sawDelta = true
		}
		if ev, ok := frame["event"].(map[string]any); ok && ev["text"] == "ALPHA" {
			sawEvent = true
		}
	}
}

func TestConversationCommandStatuses(t *testing.T) {
	f := start(t)
	id, rt := f.startConversation(t)
	base := "/api/conversations/" + id
	cases := []struct {
		name, method, path, body string
		status                   int
		code                     string
	}{
		{"prompt mid-turn", http.MethodPost, base + "/prompt", `{"text":"two"}`, http.StatusConflict, "conflict"},
		{"empty prompt", http.MethodPost, base + "/prompt", `{"text":"  "}`, http.StatusBadRequest, "invalid"},
		{"unknown request", http.MethodPost, base + "/answer", `{"request_id":"nope","decision":"allow"}`, http.StatusConflict, "not_pending"},
		{"bad since", http.MethodGet, base + "/events?since=-1", "", http.StatusBadRequest, "invalid"},
		{"unknown conversation", http.MethodPost, "/api/conversations/nope/interrupt", "", http.StatusNotFound, "not_found"},
		{"resume while running", http.MethodPost, base + "/resume", "", http.StatusConflict, "conflict"},
		{"rename without name", http.MethodPatch, base, `{"name":""}`, http.StatusBadRequest, "invalid"},
	}
	for _, c := range cases {
		status, body := f.do(t, c.method, c.path, c.body)
		if status != c.status || body["code"] != c.code {
			t.Errorf("%s: %d %v", c.name, status, body)
		}
	}

	if status, body := f.do(t, http.MethodPatch, base, `{"name":"planning","label":"plan"}`); status != http.StatusOK || body["name"] != "planning" || body["label"] != "plan" {
		t.Errorf("rename: %d %v", status, body)
	}
	if status, _ := f.do(t, http.MethodPost, base+"/interrupt", ""); status != http.StatusAccepted || rt.Interrupts() != 1 {
		t.Errorf("interrupt: %d, interrupts %d", status, rt.Interrupts())
	}
	if status, _ := f.do(t, http.MethodPost, base+"/release", ""); status != http.StatusAccepted {
		t.Errorf("release: %d", status)
	}
}

func TestDiscoverAndAdopt(t *testing.T) {
	f := start(t)
	status, project := f.do(t, http.MethodPost, "/api/projects", createBody("p", f.checkout))
	if status != http.StatusCreated {
		t.Fatalf("create project: %d %v", status, project)
	}
	f.fake.SetSessions([]harness.Session{{NativeID: "terminal-1", Preview: "From the terminal", UpdatedAt: time.Unix(1794000000, 0).UTC()}})
	f.fake.SetHistory("terminal-1", []*model.Event{{Kind: model.EventUser, Text: "From the terminal", NativeID: "t-1"}})
	base := fmt.Sprintf("/api/projects/%s", project["id"])

	status, body := f.do(t, http.MethodGet, base+"/discover?harness=claude", "")
	sessions, _ := body["sessions"].([]any)
	if status != http.StatusOK || len(sessions) != 1 || sessions[0].(map[string]any)["native_id"] != "terminal-1" || sessions[0].(map[string]any)["preview"] != "From the terminal" {
		t.Fatalf("discover: %d %v", status, body)
	}
	if status, body := f.do(t, http.MethodGet, base+"/discover?harness=pi", ""); status != http.StatusBadRequest {
		t.Errorf("unknown harness: %d %v", status, body)
	}

	status, conv := f.do(t, http.MethodPost, base+"/adopt", jsonBody(map[string]string{"harness": "claude", "native_id": "terminal-1"}))
	if status != http.StatusCreated || conv["native_id"] != "terminal-1" || conv["state"] != "none" || conv["name"] != "From the terminal" {
		t.Fatalf("adopt: %d %v", status, conv)
	}
	if status, body := f.do(t, http.MethodPost, base+"/adopt", jsonBody(map[string]string{"harness": "claude", "native_id": "terminal-1"})); status != http.StatusBadRequest {
		t.Errorf("second adoption: %d %v", status, body)
	}
	if evs := f.events(t, conv["id"].(string), 0); len(evs) != 3 || evs[1]["origin"] != "native" {
		t.Errorf("events = %v", evs)
	}
}

func TestAttentionAndReadOnTheWire(t *testing.T) {
	f := start(t)
	id, rt := f.startConversation(t)
	rt.Emit(&model.Event{Kind: model.EventAssistant, Text: "done", NativeID: "a-1"})
	rt.Emit(&model.Event{Kind: model.EventTurnEnd, Status: model.TurnSuccess})
	_, workspace := f.do(t, http.MethodGet, "/api/workspace", "")
	if conv := f.conversation(t, id); conv["attention"] != "unread" {
		t.Errorf("attention = %v", conv["attention"])
	}
	if needs, _ := workspace["needs_you"].([]any); len(needs) != 1 || needs[0] != id {
		t.Errorf("needs_you = %v", workspace["needs_you"])
	}
	last := f.conversation(t, id)["last_seq"]
	if status, body := f.do(t, http.MethodPost, "/api/conversations/"+id+"/read", jsonBody(map[string]any{"seq": last})); status != http.StatusAccepted {
		t.Fatalf("read: %d %v", status, body)
	}
	_, workspace = f.do(t, http.MethodGet, "/api/workspace", "")
	if conv := f.conversation(t, id); conv["attention"] != "idle" || conv["read_seq"] != last {
		t.Errorf("after read: %v", conv)
	}
	if needs, _ := workspace["needs_you"].([]any); needs == nil || len(needs) != 0 {
		t.Errorf("needs_you after read = %v", workspace["needs_you"])
	}
	if status, _ := f.do(t, http.MethodPost, "/api/conversations/"+id+"/read", `{"seq":-1}`); status != http.StatusBadRequest {
		t.Errorf("negative read: %d", status)
	}
}

func TestRemoveConversationOnTheWire(t *testing.T) {
	f := start(t)
	id, rt := f.startConversation(t)
	rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: rt.NativeID()}})
	path := "/api/conversations/" + id
	if status, body := f.do(t, http.MethodDelete, path, ""); status != http.StatusConflict || body["code"] != "conflict" {
		t.Errorf("remove with a runtime: %d %v", status, body)
	}
	if status, _ := f.do(t, http.MethodPost, path+"/release", ""); status != http.StatusAccepted {
		t.Fatalf("release: %d", status)
	}
	deadline := time.Now().Add(2 * time.Second)
	for f.conversation(t, id)["state"] != "none" {
		if time.Now().After(deadline) {
			t.Fatal("the runtime never stopped")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if status, body := f.do(t, http.MethodDelete, path, ""); status != http.StatusNoContent {
		t.Fatalf("remove: %d %v", status, body)
	}
	_, workspace := f.do(t, http.MethodGet, "/api/workspace", "")
	if convs, _ := workspace["conversations"].([]any); len(convs) != 0 {
		t.Errorf("conversations after removal = %v", convs)
	}
	if projects, _ := workspace["projects"].([]any); len(projects) != 1 {
		t.Errorf("projects after removal = %v", projects)
	}
	if status, body := f.do(t, http.MethodDelete, path, ""); status != http.StatusNotFound || body["code"] != "not_found" {
		t.Errorf("second removal: %d %v", status, body)
	}
}

func TestPermissionModeOnTheWire(t *testing.T) {
	f := start(t)
	status, project := f.do(t, http.MethodPost, "/api/projects", createBody("p", f.checkout))
	if status != http.StatusCreated || project["inherited_permission_mode"] != "" {
		t.Fatalf("create project: %d %v", status, project)
	}
	path := fmt.Sprintf("/api/projects/%s/conversations", project["id"])
	if status, body := f.do(t, http.MethodPost, path, jsonBody(map[string]string{"harness": "claude", "prompt": "one", "permission_mode": "bypassPermissions"})); status != http.StatusBadRequest {
		t.Errorf("bypass at launch: %d %v", status, body)
	}
	status, conv := f.do(t, http.MethodPost, path, jsonBody(map[string]string{"harness": "claude", "prompt": "one", "permission_mode": "plan"}))
	if status != http.StatusCreated || conv["permission_mode_choice"] != "plan" || f.fake.Last().Spec.PermissionMode != "plan" {
		t.Fatalf("launch in plan: %d %v", status, conv)
	}
	id := conv["id"].(string)
	rt := f.fake.Last()
	rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: rt.NativeID(), PermissionMode: "plan"}})

	modePath := "/api/conversations/" + id + "/permission-mode"
	if status, body := f.do(t, http.MethodPost, modePath, `{"mode":"dontAsk"}`); status != http.StatusBadRequest || body["code"] != "invalid" {
		t.Errorf("dontAsk: %d %v", status, body)
	}
	if status, body := f.do(t, http.MethodPost, modePath, `{"mode":"auto"}`); status != http.StatusOK || body["outcome"] != "confirmed" {
		t.Fatalf("switch: %d %v", status, body)
	}
	got := f.conversation(t, id)
	if got["permission_mode"] != "auto" || got["permission_mode_choice"] != "auto" || got["permission_mode_inherited"] != false {
		t.Errorf("after the switch: %v", got)
	}
}

func TestModelAndEffortOnTheWire(t *testing.T) {
	f := start(t)
	f.fake.SetCatalog(harness.Catalog{Version: "2.1.318", Models: []harness.ModelOption{
		{ID: "default", Resolved: "claude-opus-5-5", DisplayName: "Default", IsDefault: true, Efforts: []string{"low", "high"}},
		{ID: "haiku", Resolved: "claude-haiku-4-5"},
	}})
	f.fake.SetInherited("sonnet", "")
	_, project := f.do(t, http.MethodPost, "/api/projects", createBody("p", f.checkout))
	pid := project["id"].(string)

	status, list := f.do(t, http.MethodGet, "/api/projects/"+pid+"/models?harness=claude", "")
	if status != http.StatusOK || list["version"] != "2.1.318" {
		t.Fatalf("models: %d %v", status, list)
	}
	models, _ := list["models"].([]any)
	if len(models) != 2 {
		t.Fatalf("models = %v", list["models"])
	}
	first, second := models[0].(map[string]any), models[1].(map[string]any)
	if first["id"] != "default" || first["resolved"] != "claude-opus-5-5" || first["is_default"] != true || fmt.Sprint(first["efforts"]) != "[low high]" {
		t.Errorf("first = %v", first)
	}
	if efforts, ok := second["efforts"].([]any); !ok || len(efforts) != 0 {
		t.Errorf("a model with no effort levels lists %v", second["efforts"])
	}
	if inherited, _ := list["inherited"].(map[string]any); inherited["model"] != "sonnet" {
		t.Errorf("inherited = %v", list["inherited"])
	}
	if status, body := f.do(t, http.MethodGet, "/api/projects/"+pid+"/models?harness=pi", ""); status != http.StatusBadRequest {
		t.Errorf("an unknown harness: %d %v", status, body)
	}

	path := "/api/projects/" + pid + "/conversations"
	if status, body := f.do(t, http.MethodPost, path, jsonBody(map[string]string{"harness": "claude", "prompt": "one", "model": "haiku", "effort": "high"})); status != http.StatusBadRequest {
		t.Errorf("an effort the model lacks: %d %v", status, body)
	}
	status, conv := f.do(t, http.MethodPost, path, jsonBody(map[string]string{"harness": "claude", "prompt": "one", "model": "default", "effort": "low"}))
	if status != http.StatusCreated || conv["model_choice"] != "default" || conv["effort_choice"] != "low" {
		t.Fatalf("launch: %d %v", status, conv)
	}
	if spec := f.fake.Last().Spec; spec.Model != "default" || spec.Effort != "low" {
		t.Errorf("spec = %+v", spec)
	}
	id := conv["id"].(string)
	rt := f.fake.Last()
	rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: rt.NativeID(), Model: "claude-opus-5-5"}})

	modelPath := "/api/conversations/" + id + "/model"
	if status, body := f.do(t, http.MethodPost, modelPath, `{"model":"default","effort":"high"}`); status != http.StatusOK || body["outcome"] != "confirmed" {
		t.Fatalf("change: %d %v", status, body)
	}
	got := f.conversation(t, id)
	if got["model"] != "claude-opus-5-5" || got["effort_choice"] != "high" || got["effort_next"] != "high" {
		t.Errorf("after the change: %v", got)
	}
	if _, ok := got["model_next"]; ok {
		t.Errorf("an unchanged model is pending: %v", got["model_next"])
	}
}

func TestTurnStartedAtOnTheWire(t *testing.T) {
	f := start(t)
	_, project := f.do(t, http.MethodPost, "/api/projects", createBody("p", f.checkout))
	_, conv := f.do(t, http.MethodPost, "/api/projects/"+project["id"].(string)+"/conversations", jsonBody(map[string]string{"harness": "claude", "prompt": "one"}))
	id := conv["id"].(string)
	rt := f.fake.Last()
	rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: rt.NativeID()}})
	got := f.conversation(t, id)
	at, _ := got["turn_started_at"].(string)
	if parsed, err := time.Parse(time.RFC3339Nano, at); err != nil || time.Since(parsed) > time.Minute {
		t.Errorf("during the turn: turn_started_at %v (%v)", got["turn_started_at"], err)
	}
	// a pending request carries the time it was recorded, after the turn's start.
	rt.Ask(model.RequestPermission, map[string]any{"tool": "Bash"})
	got = f.conversation(t, id)
	pending, _ := got["pending"].([]any)
	if len(pending) != 1 {
		t.Fatalf("pending = %v", got["pending"])
	}
	raisedRaw, _ := pending[0].(map[string]any)["at"].(string)
	raised, err := time.Parse(time.RFC3339Nano, raisedRaw)
	started, _ := time.Parse(time.RFC3339Nano, at)
	if err != nil || raised.Before(started) {
		t.Errorf("pending at %q (%v), turn started %q", raisedRaw, err, at)
	}
	rt.Settle(pending[0].(map[string]any)["id"].(string), model.OutcomeAborted)
	rt.Emit(&model.Event{Kind: model.EventTurnEnd, Status: model.TurnSuccess})
	if got := f.conversation(t, id); got["turn_started_at"] != nil {
		t.Errorf("after the turn: turn_started_at %v", got["turn_started_at"])
	}
}

func TestBackgroundOnTheWire(t *testing.T) {
	f := start(t)
	_, project := f.do(t, http.MethodPost, "/api/projects", createBody("p", f.checkout))
	_, conv := f.do(t, http.MethodPost, "/api/projects/"+project["id"].(string)+"/conversations", jsonBody(map[string]string{"harness": "claude", "prompt": "one"}))
	id := conv["id"].(string)
	rt := f.fake.Last()
	rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: rt.NativeID()}})
	if got := f.conversation(t, id); got["background"] != nil {
		t.Fatalf("no jobs, yet background = %v", got["background"])
	}
	started := time.Now().UTC().Truncate(time.Second)
	rt.Background([]model.BackgroundJob{{ID: "b1", Description: "npm test", Kind: "local_bash", StartedAt: started}})
	got := f.conversation(t, id)
	list, _ := got["background"].([]any)
	if len(list) != 1 {
		t.Fatalf("background = %v", got["background"])
	}
	job := list[0].(map[string]any)
	at, err := time.Parse(time.RFC3339Nano, job["started_at"].(string))
	if job["id"] != "b1" || job["description"] != "npm test" || job["kind"] != "local_bash" || err != nil || !at.Equal(started) {
		t.Errorf("job = %v (%v)", job, err)
	}
	rt.Background(nil)
	if got := f.conversation(t, id); got["background"] != nil {
		t.Errorf("after the list emptied: %v", got["background"])
	}
}

// start answers 202 and launches idle on the conversation's session; a second
// start, and one for an unknown conversation, are refused. the requested mode
// reaches the wire.
func TestStartOnTheWire(t *testing.T) {
	f := start(t)
	id, rt := f.startConversation(t)
	rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: rt.NativeID()}})
	path := "/api/conversations/" + id
	if status, _ := f.do(t, http.MethodPost, path+"/release", ""); status != http.StatusAccepted {
		t.Fatalf("release: %d", status)
	}
	deadline := time.Now().Add(2 * time.Second)
	for f.conversation(t, id)["state"] != "none" {
		if time.Now().After(deadline) {
			t.Fatal("the runtime never stopped")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if status, body := f.do(t, http.MethodPost, path+"/start", ""); status != http.StatusAccepted {
		t.Fatalf("start: %d %v", status, body)
	}
	next := f.fake.Last()
	if next == rt || !next.Spec.Idle || next.Spec.NativeID != rt.NativeID() || len(next.Prompts()) != 0 {
		t.Fatalf("launched %+v with %d prompts", next.Spec, len(next.Prompts()))
	}
	next.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: rt.NativeID(), PermissionMode: "default", ModeRequested: true}})
	conv := f.conversation(t, id)
	if conv["state"] != "running" || conv["turn_active"] != false || conv["permission_mode"] != "default" || conv["permission_mode_requested"] != true {
		t.Errorf("after start: %v", conv)
	}
	if status, body := f.do(t, http.MethodPost, path+"/start", ""); status != http.StatusConflict || body["code"] != "conflict" {
		t.Errorf("second start: %d %v", status, body)
	}
	if status, _ := f.do(t, http.MethodPost, "/api/conversations/nope/start", ""); status != http.StatusNotFound {
		t.Errorf("unknown conversation: %d", status)
	}
}

// the gate's endpoints: arm, cancel and shut down, with their refusal codes,
// the gate on the workspace read, and the loopback guard in front.
func TestShutdownGateOnTheWire(t *testing.T) {
	f := start(t)
	id, rt := f.startConversation(t)
	rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: rt.NativeID()}})

	if status, body := f.do(t, http.MethodPost, "/api/shutdown", ""); status != http.StatusConflict || body["code"] != "conflict" {
		t.Errorf("shut down unarmed: %d %v", status, body)
	}
	foreign, err := http.NewRequest(http.MethodPost, f.url+"/api/shutdown/arm", nil)
	if err != nil {
		t.Fatal(err)
	}
	foreign.Header.Set("Origin", "http://evil.example")
	if resp, err := http.DefaultClient.Do(foreign); err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("a foreign origin arming: %v %v", resp.StatusCode, err)
	}
	if status, _ := f.do(t, http.MethodPost, "/api/shutdown/arm", ""); status != http.StatusAccepted {
		t.Fatalf("arm: %d", status)
	}
	_, workspace := f.do(t, http.MethodGet, "/api/workspace", "")
	gate, _ := workspace["shutdown"].(map[string]any)
	blockers, _ := gate["blockers"].([]any)
	if gate["armed"] != true || gate["idle"] != false || gate["armed_at"] == nil || len(blockers) != 1 {
		t.Fatalf("gate = %v", gate)
	}
	if b := blockers[0].(map[string]any); b["conversation_id"] != id || b["turn"] != true || b["requests"] != float64(0) {
		t.Errorf("blocker = %v", b)
	}
	if status, body := f.do(t, http.MethodPost, "/api/conversations/"+id+"/prompt", jsonBody(map[string]string{"text": "more"})); status != http.StatusConflict || body["code"] != "shutdown_pending" {
		t.Errorf("prompt while armed: %d %v", status, body)
	}
	if status, body := f.do(t, http.MethodPost, "/api/shutdown", ""); status != http.StatusConflict || body["code"] != "conflict" {
		t.Errorf("shut down while a turn runs: %d %v", status, body)
	}
	if status, _ := f.do(t, http.MethodPost, "/api/shutdown/cancel", ""); status != http.StatusAccepted {
		t.Fatalf("cancel: %d", status)
	}
	_, workspace = f.do(t, http.MethodGet, "/api/workspace", "")
	if gate := workspace["shutdown"].(map[string]any); gate["armed"] != false {
		t.Errorf("cancelled gate = %v", gate)
	}

	rt.Emit(&model.Event{Kind: model.EventAssistant, Text: "done", NativeID: "a-1"})
	rt.Emit(&model.Event{Kind: model.EventTurnEnd, Status: model.TurnSuccess})
	if status, _ := f.do(t, http.MethodPost, "/api/shutdown/arm", ""); status != http.StatusAccepted {
		t.Fatalf("arm again: %d", status)
	}
	_, workspace = f.do(t, http.MethodGet, "/api/workspace", "")
	if gate := workspace["shutdown"].(map[string]any); gate["idle"] != true {
		t.Fatalf("gate at idle = %v", gate)
	}
	if status, body := f.do(t, http.MethodPost, "/api/shutdown", ""); status != http.StatusAccepted {
		t.Fatalf("shut down: %d %v", status, body)
	}
	select {
	case <-f.svc.ShutdownRequested():
	case <-time.After(time.Second):
		t.Fatal("the shutdown was not requested")
	}
	if status, body := f.do(t, http.MethodPost, "/api/shutdown/cancel", ""); status != http.StatusServiceUnavailable || body["code"] != "unavailable" {
		t.Errorf("cancel after the shutdown was accepted: %d %v", status, body)
	}
}

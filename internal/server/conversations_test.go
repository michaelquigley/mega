package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"git.hq.quigley.com/research/roundhouse/internal/harness"
	"git.hq.quigley.com/research/roundhouse/internal/harness/harnesstest"
	"git.hq.quigley.com/research/roundhouse/internal/model"
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

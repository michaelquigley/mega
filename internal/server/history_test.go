package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/model"
)

func TestHistoryWindowsOnTheWire(t *testing.T) {
	f := start(t)
	id, rt := f.startConversation(t)
	rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: rt.NativeID()}})
	rt.Emit(&model.Event{Kind: model.EventNative, Text: "system/init"})
	rt.Emit(&model.Event{Kind: model.EventAssistant, Text: "ALPHA"})
	rt.Emit(&model.Event{Kind: model.EventTurnEnd, Status: model.TurnSuccess})
	waitUntil(t, func() bool { return f.conversation(t, id)["turn_active"] == false })
	if status, body := f.do(t, http.MethodPost, "/api/conversations/"+id+"/prompt", `{"text":"two"}`); status != http.StatusAccepted {
		t.Fatalf("prompt: %d %v", status, body)
	}
	rt.Emit(&model.Event{Kind: model.EventNative, Text: "system/init"})
	rt.Emit(&model.Event{Kind: model.EventAssistant, Text: "BETA"})
	rt.Emit(&model.Event{Kind: model.EventTurnEnd, Status: model.TurnSuccess})
	rt.Emit(&model.Event{Kind: model.EventNative, Text: "system/status"})
	waitUntil(t, func() bool { evs := f.events(t, id, 0); return evs[len(evs)-1]["text"] == "system/status" })
	all := f.events(t, id, 0)
	last := int64(all[len(all)-1]["seq"].(float64))

	get := func(query string) map[string]any {
		t.Helper()
		status, body := f.do(t, http.MethodGet, fmt.Sprintf("/api/conversations/%s/events?%s", id, query), "")
		if status != http.StatusOK {
			t.Fatalf("%s: %d %v", query, status, body)
		}
		return body
	}
	texts := func(body map[string]any) []string {
		var out []string
		for _, ev := range body["events"].([]any) {
			e := ev.(map[string]any)
			text, _ := e["text"].(string)
			out = append(out, fmt.Sprintf("%s:%s", e["kind"], text))
		}
		return out
	}
	// the tail: the last turn, with the native status after it covered but not sent.
	tail := get("turns=1")
	from := int64(tail["from"].(float64))
	earlier, _ := tail["earlier"].(map[string]any)
	if int64(tail["through"].(float64)) != last || earlier == nil || earlier["turns"] != float64(1) || earlier["events"] != float64(from-1) {
		t.Errorf("tail: from %v through %v earlier %v; want through %d and one turn before", tail["from"], tail["through"], tail["earlier"], last)
	}
	if got := fmt.Sprint(texts(tail)); got != "[user:two assistant:BETA turn_end:]" {
		t.Errorf("tail events: %s", got)
	}
	if got := len(get("turns=1&native=1")["events"].([]any)); got != int(last-from+1) {
		t.Errorf("tail with native events: %d, want %d", got, last-from+1)
	}
	// the turn before it, and a read after the newest.
	page := get(fmt.Sprintf("before=%d&turns=5", from))
	if page["from"] != float64(1) || int64(page["through"].(float64)) != from-1 || page["earlier"].(map[string]any)["turns"] != float64(0) {
		t.Errorf("page before: %v", page)
	}
	nothing := get(fmt.Sprintf("since=%d", last))
	if len(nothing["events"].([]any)) != 0 || nothing["earlier"] != nil || int64(nothing["through"].(float64)) != last {
		t.Errorf("since the newest: %v", nothing)
	}
}

func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestSlowRequestsAreReported(t *testing.T) {
	var reports []slowReport
	handler := slowRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("slow") != "" {
			time.Sleep(30 * time.Millisecond)
		}
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("twelve bytes"))
	}), 10*time.Millisecond, func(r slowReport) { reports = append(reports, r) })
	for _, path := range []string{"/api/fast", "/api/conversations/c/events?turns=10&slow=1", "/api/stream?slow=1"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	if len(reports) != 1 {
		t.Fatalf("reports: %+v; want only the slow read, not the fast one or the stream", reports)
	}
	if r := reports[0]; r.Path != "/api/conversations/c/events?turns=10&slow=1" || r.Bytes != 12 || r.Status != http.StatusTeapot || r.Took < 30*time.Millisecond {
		t.Errorf("report: %+v", r)
	}
}

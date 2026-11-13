package server

import (
	"net/http"
	"testing"

	"github.com/michaelquigley/mega/internal/model"
)

// the workspace read counts the servers by status; the conversation's own
// read gives them in full; neither has anything once the runtime is gone.
func TestMCPOnTheWire(t *testing.T) {
	f := start(t)
	_, project := f.do(t, http.MethodPost, "/api/projects", createBody("p", f.checkout))
	_, conv := f.do(t, http.MethodPost, "/api/projects/"+project["id"].(string)+"/conversations", jsonBody(map[string]string{"harness": "claude", "prompt": "one"}))
	id := conv["id"].(string)
	rt := f.fake.Last()
	rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: rt.NativeID()}})
	if got := f.conversation(t, id); got["mcp"] != nil {
		t.Fatalf("before a report, mcp = %v", got["mcp"])
	}
	status, read := f.do(t, http.MethodGet, "/api/conversations/"+id+"/mcp", "")
	if status != http.StatusOK || read["running"] != true || read["reported"] != false || len(read["servers"].([]any)) != 0 {
		t.Fatalf("before a report: %d %v", status, read)
	}
	rt.MCPServers([]model.MCPServer{
		{Name: "docs", Status: model.MCPNeedsSignIn},
		{Name: "fixture", Status: model.MCPConnected, Tools: []model.MCPTool{{Name: "echo", Description: "Echo the text back."}, {Name: "pick_color"}}},
		{Name: "broken", Status: model.MCPFailed, Error: "no such file"},
	})
	summary, _ := f.conversation(t, id)["mcp"].(map[string]any)
	want := map[string]float64{"servers": 3, "connected": 1, "connecting": 0, "failed": 1, "needs_sign_in": 1, "disabled": 0}
	for key, n := range want {
		if summary[key] != n {
			t.Errorf("summary %s = %v, want %v (%v)", key, summary[key], n, summary)
		}
	}
	_, read = f.do(t, http.MethodGet, "/api/conversations/"+id+"/mcp", "")
	servers, _ := read["servers"].([]any)
	if read["reported"] != true || len(servers) != 3 {
		t.Fatalf("read = %v", read)
	}
	docs, fixture, broken := servers[0].(map[string]any), servers[1].(map[string]any), servers[2].(map[string]any)
	if docs["status"] != "needs_sign_in" || docs["error"] != nil || len(docs["tools"].([]any)) != 0 {
		t.Errorf("docs = %v", docs)
	}
	tools := fixture["tools"].([]any)
	if fixture["status"] != "connected" || len(tools) != 2 || tools[0].(map[string]any)["description"] != "Echo the text back." || tools[1].(map[string]any)["description"] != nil {
		t.Errorf("fixture = %v", fixture)
	}
	if broken["status"] != "failed" || broken["error"] != "no such file" {
		t.Errorf("broken = %v", broken)
	}
	rt.Exit(nil)
	waitUntil(t, func() bool { return f.conversation(t, id)["state"] == model.RuntimeNone })
	if got := f.conversation(t, id); got["mcp"] != nil {
		t.Errorf("after the exit, mcp = %v", got["mcp"])
	}
	_, read = f.do(t, http.MethodGet, "/api/conversations/"+id+"/mcp", "")
	if read["running"] != false || read["reported"] != false || len(read["servers"].([]any)) != 0 {
		t.Errorf("after the exit: %v", read)
	}
	if status, _ := f.do(t, http.MethodGet, "/api/conversations/nope/mcp", ""); status != http.StatusNotFound {
		t.Errorf("an unknown conversation: %d", status)
	}
}

package codex

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"git.hq.quigley.com/research/roundhouse/internal/harness"
	"git.hq.quigley.com/research/roundhouse/internal/model"
)

// frames as codex-cli 0.166.0 sends them (capability report, 2026-11-09).
const (
	mcpCallStarted = `{"method":"item/started","params":{"threadId":"thread-1","turnId":"turn-1","item":{"type":"mcpToolCall","id":"exec-1","server":"fixture","tool":"echo","status":"inProgress","arguments":{"text":"MCP-ALPHA"}}}}`
	mcpApproval    = `{"id":0,"method":"mcpServer/elicitation/request","params":{"threadId":"thread-1","turnId":"turn-1","serverName":"fixture","mode":"form","_meta":{"codex_approval_kind":"mcp_tool_call","persist":["session","always"],"tool_description":"Echo the given text back, and log the call.","tool_params":{"text":"MCP-ALPHA"},"tool_params_display":[{"name":"text","value":"MCP-ALPHA","display_name":"text"}]},"message":"Allow the fixture MCP server to run tool \"echo\"?","requestedSchema":{"type":"object","properties":{}}}}`
	mcpForm        = `{"id":2,"method":"mcpServer/elicitation/request","params":{"threadId":"thread-1","turnId":"turn-1","serverName":"fixture","mode":"form","_meta":null,"message":"Which color do you prefer?","requestedSchema":{"type":"object","properties":{"color":{"type":"string","title":"Color","enum":["Red","Blue"]},"note":{"type":"string","title":"Note","description":"Anything to add"},"count":{"type":"integer"},"sure":{"type":"boolean"}},"required":["color"]}}}`
)

func TestMCPToolApprovalIsAPermissionRequest(t *testing.T) {
	r, sink, w := testRuntime()
	r.feed(mcpCallStarted)
	r.feed(mcpApproval)
	if strings.Join(sink.requests, ",") != "permission:svc-1" {
		t.Fatalf("requests = %v", sink.requests)
	}
	payload := sink.payloads[0]
	if payload["tool"] != "fixture/echo" || payload["mcp_server"] != "fixture" || payload["mcp_tool"] != "echo" {
		t.Errorf("payload names %v %v %v", payload["tool"], payload["mcp_server"], payload["mcp_tool"])
	}
	if payload["description"] != "Echo the given text back, and log the call." || payload["allow_session"] != true {
		t.Errorf("payload = %v", payload)
	}
	if input, _ := payload["input"].(map[string]any); input["text"] != "MCP-ALPHA" {
		t.Errorf("input = %v", payload["input"])
	}
	if params, _ := payload["parameters"].([]any); len(params) != 1 || params[0].(map[string]any)["value"] != "MCP-ALPHA" {
		t.Errorf("parameters = %v", payload["parameters"])
	}
	if strings.Contains(strings.ToLower(strings.Join(mapKeys(payload), ",")), "always") {
		t.Error("the card offers always")
	}
	w.quiet(t)
}

func mapKeys(m map[string]any) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func TestMCPApprovalAnswers(t *testing.T) {
	cases := []struct {
		decision string
		want     map[string]any
	}{
		{harness.DecisionAllow, map[string]any{"action": "accept"}},
		{harness.DecisionAllowSession, map[string]any{"action": "accept", "_meta": map[string]any{"persist": "session"}}},
		{harness.DecisionDeny, map[string]any{"action": "decline"}},
	}
	for _, c := range cases {
		r, _, w := testRuntime()
		r.feed(mcpCallStarted)
		r.feed(mcpApproval)
		if err := r.Answer("svc-1", harness.Answer{Decision: c.decision, Message: "not now"}); err != nil {
			t.Fatalf("%s: %v", c.decision, err)
		}
		frame := w.sent(t)
		if frame["id"] != float64(0) || !reflect.DeepEqual(frame["result"], c.want) {
			t.Errorf("%s: response = %v, want result %v", c.decision, frame, c.want)
		}
		if err := r.Answer("svc-1", harness.Answer{Decision: harness.DecisionAllow}); !errors.Is(err, harness.ErrNotPending) {
			t.Errorf("%s: a late answer: %v", c.decision, err)
		}
		w.quiet(t)
	}
}

// only "session" is ever sent back as persistence, and only when codex offered
// it; "always" would write the operator's codex config and is never sent.
func TestSessionIsSentOnlyWhereOffered(t *testing.T) {
	r, sink, w := testRuntime()
	r.feed(strings.Replace(mcpApproval, `"persist":["session","always"],`, `"persist":["always"],`, 1))
	if _, offered := sink.payloads[0]["allow_session"]; offered {
		t.Fatalf("payload offers the session: %v", sink.payloads[0])
	}
	if err := r.Answer("svc-1", harness.Answer{Decision: harness.DecisionAllowSession}); err != nil {
		t.Fatal(err)
	}
	if frame := w.sent(t); !reflect.DeepEqual(frame["result"], map[string]any{"action": "accept"}) {
		t.Errorf("response = %v", frame)
	}
}

func TestMCPApprovalIsAbortedWhenCodexResolvesIt(t *testing.T) {
	r, sink, w := testRuntime()
	r.feed(mcpCallStarted)
	r.feed(mcpApproval)
	// what an interrupt brings: the turn ends and codex resolves the request.
	r.feed(`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"interrupted","items":[]}}}`)
	r.feed(`{"method":"serverRequest/resolved","params":{"threadId":"thread-1","requestId":0}}`)
	if strings.Join(sink.resolved, ",") != "svc-1:aborted" {
		t.Errorf("resolved = %v", sink.resolved)
	}
	if err := r.Answer("svc-1", harness.Answer{Decision: harness.DecisionAllow}); !errors.Is(err, harness.ErrNotPending) {
		t.Errorf("answer after the abort: %v", err)
	}
	w.quiet(t)
}

func TestToolNameFallsBackToTheMessage(t *testing.T) {
	r, sink, _ := testRuntime()
	r.feed(mcpApproval) // no call in flight to name it
	if sink.payloads[0]["tool"] != "fixture/echo" {
		t.Errorf("tool = %v", sink.payloads[0]["tool"])
	}
	// a finished call no longer names the tool of a later approval.
	r2, sink2, _ := testRuntime()
	r2.feed(mcpCallStarted)
	r2.feed(`{"method":"item/completed","params":{"threadId":"thread-1","turnId":"turn-1","item":{"type":"mcpToolCall","id":"exec-1","server":"fixture","tool":"echo","status":"completed"}}}`)
	r2.feed(strings.Replace(mcpApproval, `run tool \"echo\"`, `run tool \"pick_color\"`, 1))
	if sink2.payloads[0]["tool"] != "fixture/pick_color" {
		t.Errorf("tool after the earlier call finished = %v", sink2.payloads[0]["tool"])
	}
}

func TestTypedFormIsAQuestionAnsweredAsContent(t *testing.T) {
	r, sink, w := testRuntime()
	r.feed(mcpForm)
	if strings.Join(sink.requests, ",") != "question:svc-1" {
		t.Fatalf("requests = %v", sink.requests)
	}
	payload := sink.payloads[0]
	fields, _ := payload["fields"].([]any)
	var names []string
	for _, f := range fields {
		names = append(names, f.(map[string]any)["name"].(string))
	}
	if payload["message"] != "Which color do you prefer?" || strings.Join(names, ",") != "color,count,note,sure" {
		t.Fatalf("payload = %v (fields %v)", payload, names)
	}
	color := fields[0].(map[string]any)
	if color["type"] != "enum" || color["required"] != true || len(color["options"].([]map[string]any)) != 2 {
		t.Errorf("color field = %v", color)
	}
	if err := r.Answer("svc-1", harness.Answer{Answers: map[string]string{"color": "Blue", "count": "3", "sure": "true", "note": ""}}); err != nil {
		t.Fatal(err)
	}
	frame := w.sent(t)
	want := map[string]any{"action": "accept", "content": map[string]any{"color": "Blue", "count": float64(3), "sure": true}}
	if frame["id"] != float64(2) || !reflect.DeepEqual(frame["result"], want) {
		t.Errorf("response = %v, want result %v", frame, want)
	}
}

func TestUnanswerableElicitationsAreRecordedNotAnswered(t *testing.T) {
	frames := map[string]string{
		"url":         `{"id":5,"method":"mcpServer/elicitation/request","params":{"threadId":"thread-1","serverName":"auth","mode":"url","elicitationId":"e-1","url":"https://example.com/login","message":"Sign in to continue"}}`,
		"free schema": `{"id":6,"method":"mcpServer/elicitation/request","params":{"threadId":"thread-1","serverName":"x","mode":"openai/form","message":"Fill this in","requestedSchema":{"anything":true}}}`,
		"multi":       `{"id":7,"method":"mcpServer/elicitation/request","params":{"threadId":"thread-1","serverName":"x","mode":"form","message":"Pick some","requestedSchema":{"type":"object","properties":{"tags":{"type":"array","items":{"type":"string","enum":["a","b"]}}}}}}`,
		"no fields":   `{"id":8,"method":"mcpServer/elicitation/request","params":{"threadId":"thread-1","serverName":"x","mode":"form","_meta":null,"message":"Proceed?","requestedSchema":{"type":"object","properties":{}}}}`,
	}
	for name, frame := range frames {
		r, sink, w := testRuntime()
		r.feed(frame)
		if len(sink.requests) != 0 {
			t.Errorf("%s: became a request: %v", name, sink.requests)
		}
		if len(sink.events) != 1 || sink.events[0].Kind != model.EventNative || !strings.Contains(sink.events[0].Text, "roundhouse cannot answer") || !strings.Contains(sink.events[0].Text, "interrupt the turn") {
			t.Errorf("%s: recorded %v", name, sink.events)
		}
		w.quiet(t)
	}
}

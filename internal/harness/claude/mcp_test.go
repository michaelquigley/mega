package claude

import (
	"bufio"
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/model"
)

// asks is every mcp_status frame the runtime has sent the shim.
func asks(r *runtime, stdin *nopCloser) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var ids []string
	for _, line := range strings.Split(strings.TrimSpace(stdin.String()), "\n") {
		if strings.Contains(line, `"type":"mcp_status"`) {
			ids = append(ids, line)
		}
	}
	return ids
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func recordedAnswers(t *testing.T) [][]byte {
	t.Helper()
	data, err := os.ReadFile("testdata/mcp-status.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var lines [][]byte
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for scanner.Scan() {
		lines = append(lines, append([]byte(nil), scanner.Bytes()...))
	}
	return lines
}

// recorded from claude code 2.1.318 (probes/claude/mcp-status.mjs): an idle
// launch asks as it is ready; a server still pending is reported connecting
// and asked about again; the next answer is reported whole, in mega's words,
// with tools by name and no description, as claude gives none.
func TestMCPServersAreAskedForAndReportedWhole(t *testing.T) {
	saved := mcpSettle
	mcpSettle = 10 * time.Millisecond
	t.Cleanup(func() { mcpSettle = saved })
	r, sink, stdin := testRuntime()
	r.handle([]byte(`{"type":"ready","version":"2.1.318","model":"claude-opus-5-5"}`))
	if got := asks(r, stdin); len(got) != 1 || !strings.Contains(got[0], `"id":"mcp-1"`) {
		t.Fatalf("asks after ready: %v", got)
	}
	answers := recordedAnswers(t)
	r.handle(answers[0])
	if len(sink.mcp) != 1 || len(sink.mcp[0]) != 1 || sink.mcp[0][0].Name != "fixture" || sink.mcp[0][0].Status != model.MCPConnecting || len(sink.mcp[0][0].Tools) != 0 {
		t.Fatalf("first report: %+v", sink.mcp)
	}
	waitFor(t, "a second ask while the server connects", func() bool { return len(asks(r, stdin)) == 2 })
	r.handle(answers[1])
	if len(sink.mcp) != 2 {
		t.Fatalf("reports: %d", len(sink.mcp))
	}
	got := sink.mcp[1]
	var names []string
	for _, s := range got {
		names = append(names, s.Name+":"+s.Status)
	}
	if strings.Join(names, " ") != "broken:failed claude.ai Connector:needs_sign_in fixture:connected" {
		t.Fatalf("servers: %v", names)
	}
	broken, connector, fixture := got[0], got[1], got[2]
	if broken.Error == "" || len(broken.Tools) != 0 {
		t.Errorf("broken: %+v", broken)
	}
	if connector.Error != "" || len(connector.Tools) != 0 {
		t.Errorf("connector: %+v", connector)
	}
	if len(fixture.Tools) != 2 || fixture.Tools[0] != (model.MCPTool{Name: "echo"}) || fixture.Tools[1] != (model.MCPTool{Name: "pick_color"}) {
		t.Errorf("fixture tools: %+v", fixture.Tools)
	}
	// nothing connecting: no further ask.
	time.Sleep(5 * mcpSettle)
	if n := len(asks(r, stdin)); n != 2 {
		t.Errorf("asks once settled: %d", n)
	}
}

// a turn's init lists the servers' statuses: the same as the last answer asks
// nothing; a different one asks again.
func TestInitWithOtherStatusesAsksAgain(t *testing.T) {
	r, _, stdin := testRuntime()
	r.started = true
	r.mcpStatuses = map[string]string{"fixture": "connected", "broken": "failed"}
	init := func(fixture string) {
		r.handle([]byte(`{"type":"event","msg":{"type":"system","subtype":"init","session_id":"edfe4ade-9f59-4db3-89bc-75fcc8b52c44","model":"claude-opus-5-5","permissionMode":"default","tools":[],"mcp_servers":[{"name":"fixture","status":"` + fixture + `","source":"dynamic"},{"name":"broken","status":"failed","source":"dynamic"}]}}`))
	}
	init("connected")
	if n := len(asks(r, stdin)); n != 0 {
		t.Fatalf("asks after an init that agrees: %d", n)
	}
	init("failed")
	if n := len(asks(r, stdin)); n != 1 {
		t.Fatalf("asks after an init that differs: %d", n)
	}
}

// one ask is in flight at a time; one wanted meanwhile follows the answer.
func TestOneAskAtATime(t *testing.T) {
	r, sink, stdin := testRuntime()
	r.askMCP()
	r.askMCP()
	if n := len(asks(r, stdin)); n != 1 {
		t.Fatalf("asks in flight: %d", n)
	}
	r.handle([]byte(`{"type":"mcp_status","id":"mcp-1","servers":[]}`))
	if n := len(asks(r, stdin)); n != 2 {
		t.Fatalf("asks after the answer: %d", n)
	}
	if len(sink.mcp) != 1 || len(sink.mcp[0]) != 0 {
		t.Errorf("an empty answer is reported as no servers: %+v", sink.mcp)
	}
	// an error leaves the last report standing.
	r.handle([]byte(`{"type":"mcp_status","id":"mcp-2","error":"no session yet"}`))
	if len(sink.mcp) != 1 {
		t.Errorf("an error was reported as a list: %+v", sink.mcp)
	}
}

// a status claude may add later is shown as failed and named, not guessed;
// a description is its first line.
func TestUnknownStatusAndDescriptions(t *testing.T) {
	servers, statuses := claudeMCP([]any{
		map[string]any{"name": "later", "status": "hibernating"},
		map[string]any{"name": "docs", "status": "connected", "tools": []any{map[string]any{"name": "search", "description": "\n  Search the docs.\nMore detail."}}},
	})
	if servers[0].Name != "docs" || servers[0].Tools[0].Description != "Search the docs." {
		t.Errorf("docs: %+v", servers[0])
	}
	if servers[1].Status != model.MCPFailed || servers[1].Error != "claude reported the status 'hibernating'" || statuses["later"] != "hibernating" {
		t.Errorf("later: %+v %v", servers[1], statuses)
	}
}

package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

func (s *recordingSink) mcpReports() [][]model.MCPServer {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([][]model.MCPServer(nil), s.mcp...)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// listsSent counts the mcpServerStatus/list requests the fake app-server got,
// and checks each asked with the thread's id.
func listsSent(t *testing.T, frames string) int {
	t.Helper()
	data, err := os.ReadFile(frames)
	if err != nil {
		return 0
	}
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		frame := map[string]any{}
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			t.Fatal(err)
		}
		if frame["method"] == "mcpServerStatus/list" {
			n++
			if params, _ := frame["params"].(map[string]any); params["threadId"] != "t-1" {
				t.Errorf("a list without the thread's id: %s", line)
			}
		}
	}
	return n
}

// the launch lists the thread's servers once the thread is known, with the
// thread's id; a turn's start lists them again. each list is reported whole,
// in mega's words (recorded from codex-cli 0.166.0, probes/codex/mcp-status.mjs).
func TestLaunchListsTheThreadsServers(t *testing.T) {
	h := fakeHarness(t, "mcp")
	list, err := filepath.Abs("testdata/mcp-status-list.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(fakeMCPList, list)
	frames := filepath.Join(t.TempDir(), "frames")
	t.Setenv(fakeFrames, frames)
	sink := &recordingSink{}
	rt, err := h.Launch(harness.Spec{Env: os.Environ(), Find: os.Environ(), Cwd: t.TempDir()}, sink)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	waitFor(t, "the first list", func() bool { return len(sink.mcpReports()) >= 1 })
	got := sink.mcpReports()[0]
	if len(got) != 2 {
		t.Fatalf("servers: %+v", got)
	}
	broken, fixture := got[0], got[1]
	if broken.Name != "broken" || broken.Status != model.MCPFailed || broken.Error != "MCP startup failed: No such file or directory (os error 2)" || len(broken.Tools) != 0 {
		t.Errorf("broken: %+v", broken)
	}
	want := []model.MCPTool{{Name: "echo", Description: "Echo the given text back, and log the call."}, {Name: "pick_color", Description: "Ask the user to pick a color through an elicitation form."}}
	if fixture.Name != "fixture" || fixture.Status != model.MCPConnected || fixture.Error != "" || len(fixture.Tools) != 2 || fixture.Tools[0] != want[0] || fixture.Tools[1] != want[1] {
		t.Errorf("fixture: %+v", fixture)
	}
	// the startup notifications all came within the first list, or asked
	// for one more: no more than two lists so far.
	waitFor(t, "the lists to settle", func() bool { n := listsSent(t, frames); return n >= 1 && n <= 2 })
	before := listsSent(t, frames)
	if err := rt.Prompt("u-1", "hello"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "a list at the turn's start", func() bool { return listsSent(t, frames) > before })
}

// what each runtime status becomes, and the cases codex gives no runtime
// status for.
func TestCodexStatuses(t *testing.T) {
	list := []*mcpServerStatus{
		{Name: "a", RuntimeStatus: "starting"},
		{Name: "b", RuntimeStatus: "authenticationRequired", AuthStatus: "notLoggedIn"},
		{Name: "c", RuntimeStatus: "disabled"},
		{Name: "d", RuntimeStatus: "cancelled"},
		{Name: "e", AuthStatus: "notLoggedIn"},
		{Name: "f", ToolsError: "listing failed"},
		{Name: "g"},
		{Name: "h", RuntimeStatus: "sleeping"},
		{Name: "i", RuntimeStatus: "failed", Tools: map[string]any{"t": map[string]any{"name": "t", "description": "kept from before"}}},
		{Name: "j", RuntimeStatus: "connected", Tools: map[string]any{"z": map[string]any{"name": "z", "title": "Zed tool"}, "y": map[string]any{"name": "y", "description": "Why.\nMore."}}},
	}
	var got []string
	for _, s := range codexMCP(list) {
		got = append(got, s.Name+":"+s.Status)
	}
	if strings.Join(got, " ") != "a:connecting b:needs_sign_in c:disabled d:failed e:needs_sign_in f:failed g:connecting h:failed i:failed j:connected" {
		t.Errorf("statuses: %v", got)
	}
	servers := codexMCP(list)
	if servers[7].Error != "codex reported the status 'sleeping'" {
		t.Errorf("an unknown status: %+v", servers[7])
	}
	if len(servers[8].Tools) != 0 {
		t.Errorf("a failed server's tools are shown: %+v", servers[8])
	}
	if tools := servers[9].Tools; len(tools) != 2 || tools[0] != (model.MCPTool{Name: "y", Description: "Why."}) || tools[1] != (model.MCPTool{Name: "z", Description: "Zed tool"}) {
		t.Errorf("tools: %+v", tools)
	}
}

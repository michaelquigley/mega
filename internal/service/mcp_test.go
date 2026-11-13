package service

import (
	"testing"

	"github.com/michaelquigley/mega/internal/model"
)

func mcpServers(statuses ...string) []model.MCPServer {
	var out []model.MCPServer
	for i, status := range statuses {
		server := model.MCPServer{Name: string(rune('a' + i)), Status: status}
		if status == model.MCPConnected {
			server.Tools = []model.MCPTool{{Name: "echo", Description: "Echo the text back."}}
		}
		if status == model.MCPFailed {
			server.Error = "no such file"
		}
		out = append(out, server)
	}
	return out
}

// the servers are in the view once the runtime reports them, replaced whole
// by each report, announced only when they change, and never attention: a
// failed server or one that needs signing in leaves an idle conversation
// idle and out of needs-you, and is not recorded.
func TestMCPServersAreShownButNeverAttention(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	startedIn(rt, "default")
	finishTurn(rt, "a-1", "done")
	if err := r.svc.Read(conv.ID, r.view(conv.ID).LastSeq); err != nil {
		t.Fatal(err)
	}
	if view := r.view(conv.ID); view.MCPReported || len(view.MCPServers) != 0 {
		t.Fatalf("before any report: %+v", view.MCPServers)
	}
	sub := r.svc.Subscribe()
	defer sub.Close()
	rt.MCPServers(mcpServers(model.MCPConnected, model.MCPFailed, model.MCPNeedsSignIn))
	expectChanged(t, sub)
	view := r.view(conv.ID)
	if !view.MCPReported || len(view.MCPServers) != 3 || view.MCPServers[0].Tools[0].Name != "echo" || view.MCPServers[1].Error != "no such file" {
		t.Fatalf("servers = %+v", view.MCPServers)
	}
	if view.Attention != model.AttentionIdle || len(r.svc.Workspace().NeedsYou) != 0 {
		t.Errorf("MCP servers changed attention: %q, needs-you %v", view.Attention, r.svc.Workspace().NeedsYou)
	}
	// the same list again announces nothing; a changed one replaces it.
	rt.MCPServers(mcpServers(model.MCPConnected, model.MCPFailed, model.MCPNeedsSignIn))
	rt.MCPServers(mcpServers(model.MCPConnected, model.MCPConnected))
	expectChanged(t, sub)
	select {
	case update := <-sub.Updates():
		t.Errorf("an unchanged list was announced: %+v", update)
	default:
	}
	if view := r.view(conv.ID); len(view.MCPServers) != 2 || view.MCPServers[1].Status != model.MCPConnected {
		t.Errorf("after the change: %+v", view.MCPServers)
	}
	// an empty report is no servers, still reported.
	rt.MCPServers(nil)
	if view := r.view(conv.ID); !view.MCPReported || len(view.MCPServers) != 0 {
		t.Errorf("an empty report: reported %v, %+v", view.MCPReported, view.MCPServers)
	}
	for _, ev := range r.events(conv.ID) {
		if ev.Text == "no such file" {
			t.Errorf("a server was recorded: %+v", ev)
		}
	}
	if !r.registryLacks(t, "no such file") {
		t.Error("the registry stores MCP servers")
	}
}

// the servers go with the runtime: an exit clears them, and the
// conversation's own read then says it is not running.
func TestMCPServersClearOnExit(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	startedIn(rt, "default")
	finishTurn(rt, "a-1", "done")
	rt.MCPServers(mcpServers(model.MCPConnected))
	read, err := r.svc.MCP(conv.ID)
	if err != nil || !read.Running || !read.Reported || len(read.Servers) != 1 {
		t.Fatalf("while running: %+v, %v", read, err)
	}
	rt.Exit(nil)
	waitFor(t, "the exit to be recorded", func() bool { return r.view(conv.ID).State == model.RuntimeNone })
	read, err = r.svc.MCP(conv.ID)
	if err != nil || read.Running || read.Reported || len(read.Servers) != 0 {
		t.Errorf("after the exit: %+v, %v", read, err)
	}
	if _, err := r.svc.MCP("nope"); ErrorCode(err) != CodeNotFound {
		t.Errorf("an unknown conversation: %v", err)
	}
}

// a list the caller keeps is its own: changing it afterwards changes nothing
// the service holds, and nor does changing a read.
func TestMCPServersAreCopied(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	startedIn(rt, "default")
	list := mcpServers(model.MCPConnected)
	rt.MCPServers(list)
	list[0].Tools[0].Name = "changed"
	read, _ := r.svc.MCP(conv.ID)
	read.Servers[0].Tools[0].Description = "changed"
	if view := r.view(conv.ID); view.MCPServers[0].Tools[0] != (model.MCPTool{Name: "echo", Description: "Echo the text back."}) {
		t.Errorf("held list = %+v", view.MCPServers[0].Tools)
	}
}

// a resume only imports history, with no runtime: while it does, the read
// says nothing is running.
func TestMCPIsNotRunningWhileAResumeImports(t *testing.T) {
	r := newRig(t)
	conv := r.start("one")
	rt := r.fake.Last()
	startedIn(rt, "default")
	finishTurn(rt, "a-1", "done")
	rt.MCPServers(mcpServers(model.MCPConnected))
	if err := r.svc.Release(conv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the release", func() bool { return r.view(conv.ID).State == model.RuntimeNone })
	r.fake.SetHistory(r.view(conv.ID).NativeID, nil)
	reading := make(chan struct{})
	release := make(chan struct{})
	r.fake.OnHistory(func() {
		close(reading)
		<-release
	})
	done := make(chan error, 1)
	go func() { done <- r.svc.Resume(conv.ID) }()
	<-reading
	read, err := r.svc.MCP(conv.ID)
	if err != nil || r.view(conv.ID).State != StateResuming || read.Running || read.Reported || len(read.Servers) != 0 {
		t.Errorf("while resuming: state %q, %+v, %v", r.view(conv.ID).State, read, err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

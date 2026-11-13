package codex

import (
	"context"
	"fmt"

	"github.com/michaelquigley/df/dl"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

// mcpServerStatus/list, asked with the thread's id, gives the servers that
// thread runs: each with its runtime status, auth status, tools and any
// error listing them. with no thread it gives only the user-level servers
// and no runtime status (capability report, 2026-11-12).
type mcpStatusParams struct {
	ThreadID string `dd:"threadId"`
	Cursor   string `dd:"cursor,+omitempty"`
}

type mcpStatusPage struct {
	Data       []*mcpServerStatus `dd:"data,+nullable"`
	NextCursor string             `dd:"nextCursor,+nullable"`
}

type mcpServerStatus struct {
	Name          string `dd:"name"`
	AuthStatus    string `dd:"authStatus,+nullable"`
	RuntimeStatus string `dd:"runtimeStatus,+nullable"`
	// Tools is an object keyed by tool name; each tool has a name, a title
	// and a description among its fields.
	Tools      map[string]any `dd:"tools,+opaque,+nullable"`
	ToolsError string         `dd:"toolsError,+nullable"`
}

// mcpPages bounds how many pages one list reads.
const mcpPages = 20

// refreshMCP lists the thread's servers and reports them whole. codex says
// when a server starts or fails to (mcpServer/startupStatus/updated), but not
// when one dies later, so a list is also asked at each turn's start. one list
// runs at a time, off the reader, since it waits on a reply the reader
// delivers; any wanted meanwhile follows it once.
func (r *runtime) refreshMCP() {
	r.mu.Lock()
	if r.threadID == "" {
		// before the thread is known; the launch lists once it is.
		r.mu.Unlock()
		return
	}
	if r.mcpBusy {
		r.mcpAgain = true
		r.mu.Unlock()
		return
	}
	r.mcpBusy = true
	thread := r.threadID
	r.mu.Unlock()
	go r.listMCP(thread)
}

func (r *runtime) listMCP(thread string) {
	for {
		servers, err := r.mcpList(thread)
		r.mu.Lock()
		again := r.mcpAgain
		r.mcpAgain = false
		if !again {
			r.mcpBusy = false
		}
		r.mu.Unlock()
		if err != nil {
			dl.Warnf("listing the MCP servers of codex thread '%s': %v", thread, err)
		} else {
			r.reportMCP(servers)
		}
		if !again {
			return
		}
	}
}

// reportMCP hands the list to the sink, unless the runtime's exit has been
// reported: nothing follows Exited.
func (r *runtime) reportMCP(servers []model.MCPServer) {
	r.mcpReport.Lock()
	defer r.mcpReport.Unlock()
	if r.mcpExited {
		return
	}
	r.sink.MCPServers(servers)
}

// mcpEnd closes the reports, before the exit is reported.
func (r *runtime) mcpEnd() {
	r.mcpReport.Lock()
	defer r.mcpReport.Unlock()
	r.mcpExited = true
}

func (r *runtime) mcpList(thread string) ([]model.MCPServer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	var all []*mcpServerStatus
	params := &mcpStatusParams{ThreadID: thread}
	for range mcpPages {
		page := &mcpStatusPage{}
		if err := r.conn.call(ctx, "mcpServerStatus/list", params, page); err != nil {
			return nil, err
		}
		all = append(all, page.Data...)
		if page.NextCursor == "" {
			break
		}
		params.Cursor = page.NextCursor
	}
	return codexMCP(all), nil
}

// codexStatuses maps codex's runtime statuses to mega's.
var codexStatuses = map[string]string{
	"connected":              model.MCPConnected,
	"notStarted":             model.MCPConnecting,
	"starting":               model.MCPConnecting,
	"authenticationRequired": model.MCPNeedsSignIn,
	"failed":                 model.MCPFailed,
	"cancelled":              model.MCPFailed,
	"disabled":               model.MCPDisabled,
}

// codexMCP reads a list: each server's status, its error, and, while it is
// connected, its tools with the first line of each description (or the
// title, where there is no description). a server whose runtime status is
// unavailable needs signing in if codex says it is not logged in, has failed
// if listing its tools did, and is otherwise still connecting.
func codexMCP(list []*mcpServerStatus) []model.MCPServer {
	servers := make([]model.MCPServer, 0, len(list))
	for _, st := range list {
		if st == nil || st.Name == "" {
			continue
		}
		server := model.MCPServer{Name: st.Name, Error: st.ToolsError}
		status, known := codexStatuses[st.RuntimeStatus]
		switch {
		case known:
			server.Status = status
		case st.RuntimeStatus != "":
			server.Status = model.MCPFailed
			if server.Error == "" {
				server.Error = fmt.Sprintf("codex reported the status '%s'", st.RuntimeStatus)
			}
		case st.AuthStatus == "notLoggedIn":
			server.Status = model.MCPNeedsSignIn
		case st.ToolsError != "":
			server.Status = model.MCPFailed
		default:
			server.Status = model.MCPConnecting
		}
		if server.Status == model.MCPConnected {
			for key, value := range st.Tools {
				tool, _ := value.(map[string]any)
				name, _ := tool["name"].(string)
				if name == "" {
					name = key
				}
				description, _ := tool["description"].(string)
				if description == "" {
					description, _ = tool["title"].(string)
				}
				server.Tools = append(server.Tools, model.MCPTool{Name: name, Description: harness.FirstLine(description)})
			}
		}
		servers = append(servers, server)
	}
	harness.SortMCP(servers)
	return servers
}

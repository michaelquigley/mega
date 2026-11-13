package server

import (
	"net/http"

	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/service"
)

// mcpSummaryView is a conversation's MCP servers on the workspace read: how
// many, and how many stand in each status. it is absent until the runtime
// has reported them, and with no runtime.
type mcpSummaryView struct {
	Servers     int
	Connected   int
	Connecting  int
	Failed      int
	NeedsSignIn int
	Disabled    int
}

func newMCPSummary(conv service.ConversationView) *mcpSummaryView {
	if !conv.MCPReported {
		return nil
	}
	summary := &mcpSummaryView{Servers: len(conv.MCPServers)}
	for _, server := range conv.MCPServers {
		switch server.Status {
		case model.MCPConnected:
			summary.Connected++
		case model.MCPConnecting:
			summary.Connecting++
		case model.MCPFailed:
			summary.Failed++
		case model.MCPNeedsSignIn:
			summary.NeedsSignIn++
		case model.MCPDisabled:
			summary.Disabled++
		}
	}
	return summary
}

// conversationMCPView is a conversation's MCP servers on their own read.
type conversationMCPView struct {
	Running  bool
	Reported bool
	Servers  []*mcpServerView
}

type mcpServerView struct {
	Name   string
	Status string
	Error  string `dd:",+omitempty"`
	Tools  []*mcpToolView
}

type mcpToolView struct {
	Name        string
	Description string `dd:",+omitempty"`
}

// getMCP answers a conversation's MCP servers in full, from memory. it is
// read only: nothing here reconnects, enables or disables a server.
func (s *Server) getMCP(w http.ResponseWriter, r *http.Request) {
	read, err := s.svc.MCP(r.PathValue("id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	view := &conversationMCPView{Running: read.Running, Reported: read.Reported, Servers: make([]*mcpServerView, 0, len(read.Servers))}
	for _, server := range read.Servers {
		sv := &mcpServerView{Name: server.Name, Status: server.Status, Error: server.Error, Tools: make([]*mcpToolView, 0, len(server.Tools))}
		for _, tool := range server.Tools {
			sv.Tools = append(sv.Tools, &mcpToolView{Name: tool.Name, Description: tool.Description})
		}
		view.Servers = append(view.Servers, sv)
	}
	writeJSON(w, r, http.StatusOK, view)
}

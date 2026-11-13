package service

import "github.com/michaelquigley/mega/internal/model"

// ConversationMCP is a conversation's MCP servers, for its own read: whether
// a runtime is there to ask, whether it has reported its servers yet, and
// the servers in full.
type ConversationMCP struct {
	Running  bool
	Reported bool
	Servers  []model.MCPServer
}

// MCP reads a conversation's MCP servers from memory. a runtime is there to
// ask while it starts, runs or stops; with none, and while a resume only
// imports history, there is nothing to ask, and nothing is kept from an
// earlier runtime.
func (s *Service) MCP(conversationID string) (ConversationMCP, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	conv, err := s.conversationLocked(conversationID)
	if err != nil {
		return ConversationMCP{}, err
	}
	view := s.viewLocked(conv)
	running := view.State == StateStarting || view.State == StateRunning || view.State == StateStopping
	return ConversationMCP{Running: running, Reported: view.MCPReported, Servers: view.MCPServers}, nil
}

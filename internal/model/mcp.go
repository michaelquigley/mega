package model

// the status of an MCP server, as every adapter reports it: one small set,
// whatever words the harness uses.
const (
	MCPConnected   = "connected"
	MCPConnecting  = "connecting"
	MCPFailed      = "failed"
	MCPNeedsSignIn = "needs_sign_in"
	MCPDisabled    = "disabled"
)

// MCPServer is one MCP server of a running conversation, as its harness
// reports it: plain data, held in memory while the runtime runs and never
// stored. it is not attention.
type MCPServer struct {
	Name   string
	Status string
	// Error is the harness's own reason, when it gives one.
	Error string
	// Tools are the server's tools, by name; empty unless it is connected.
	Tools []MCPTool
}

// MCPTool is one tool of an MCP server: its name and the first line of its
// description, which is empty where the harness gives none.
type MCPTool struct {
	Name        string
	Description string
}

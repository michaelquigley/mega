// Package codex is the Codex adapter: a private `codex app-server` child per
// runtime, driven over stdio JSON-RPC. the types here are the subset of the
// 0.166.0 app-server protocol the adapter uses, written by hand from the schema
// under schema/; everything protocol-shaped stays inside this package.
package codex

// rpcFrame is one line from the app-server: a response (id and result or
// error), a server request (id and method), or a notification (method only).
// the server omits "jsonrpc".
type rpcFrame struct {
	ID     any            `dd:",+opaque,+nullable"`
	Method string         `dd:",+nullable"`
	Params map[string]any `dd:",+opaque,+nullable"`
	Result any            `dd:",+opaque,+nullable"`
	Error  *rpcError      `dd:",+nullable"`
}

type rpcError struct {
	Code    int
	Message string
}

// outgoing frames. ids the service sends are integers; a response echoes the
// server's own request id, whatever its type.
type rpcRequest struct {
	JSONRPC string `dd:"jsonrpc"`
	ID      int64
	Method  string
	Params  any `dd:",+opaque"`
}

type rpcNotification struct {
	JSONRPC string `dd:"jsonrpc"`
	Method  string
	Params  any `dd:",+opaque"`
}

type rpcResponse struct {
	JSONRPC string `dd:"jsonrpc"`
	ID      any    `dd:",+opaque"`
	Result  any    `dd:",+opaque"`
}

type initializeParams struct {
	ClientInfo   clientInfo         `dd:"clientInfo"`
	Capabilities clientCapabilities `dd:"capabilities"`
}

type clientInfo struct {
	Name    string `dd:"name"`
	Title   string `dd:"title"`
	Version string `dd:"version"`
}

type clientCapabilities struct {
	ExperimentalAPI bool `dd:"experimentalApi"`
}

// threadStartParams carries no approvalPolicy and no sandbox: the operator's
// own codex configuration decides both.
type threadStartParams struct {
	Cwd   string `dd:"cwd"`
	Model string `dd:"model,+omitempty"`
}

type threadResumeParams struct {
	ThreadID string `dd:"threadId"`
	Cwd      string `dd:"cwd"`
	Model    string `dd:"model,+omitempty"`
}

// threadStarted is the shared shape of the thread/start and thread/resume
// responses.
type threadStarted struct {
	Thread             thread
	Model              string         `dd:"model,+nullable"`
	ReasoningEffort    string         `dd:"reasoningEffort,+nullable"`
	ApprovalPolicy     any            `dd:"approvalPolicy,+opaque,+nullable"`
	Sandbox            map[string]any `dd:"sandbox,+opaque,+nullable"`
	InstructionSources []string       `dd:"instructionSources,+nullable"`
	Cwd                string         `dd:"cwd,+nullable"`
}

type thread struct {
	ID         string `dd:"id"`
	Preview    string `dd:"preview,+nullable"`
	Cwd        string `dd:"cwd,+nullable"`
	CliVersion string `dd:"cliVersion,+nullable"`
	CreatedAt  int64  `dd:"createdAt,+nullable"`
	UpdatedAt  int64  `dd:"updatedAt,+nullable"`
	Turns      []turn `dd:"turns,+nullable"`
}

type turn struct {
	ID     string `dd:"id"`
	Status string `dd:"status,+nullable"`
	Items  any    `dd:"items,+opaque,+nullable"`
}

type threadListParams struct {
	Cwd   string `dd:"cwd"`
	Limit int    `dd:"limit"`
}

type threadList struct {
	Data []thread `dd:"data,+nullable"`
}

type threadReadParams struct {
	ThreadID     string `dd:"threadId"`
	IncludeTurns bool   `dd:"includeTurns"`
}

type threadRead struct {
	Thread thread `dd:"thread"`
}

type textInput struct {
	Type         string `dd:"type"`
	Text         string `dd:"text"`
	TextElements []any  `dd:"text_elements"`
}

func text(s string) []textInput {
	return []textInput{{Type: "text", Text: s, TextElements: []any{}}}
}

type turnStartParams struct {
	ThreadID            string      `dd:"threadId"`
	Input               []textInput `dd:"input"`
	ClientUserMessageID string      `dd:"clientUserMessageId"`
	// the operator's choice, sent on every turn: an override lasts for this
	// turn and later ones, so re-sending the stored choice keeps it.
	Model  string `dd:"model,+omitempty"`
	Effort string `dd:"effort,+omitempty"`
}

type modelListParams struct {
	IncludeHidden bool `dd:"includeHidden"`
}

type modelList struct {
	Data []codexModel `dd:"data,+nullable"`
}

type codexModel struct {
	ID                        string `dd:"id"`
	DisplayName               string `dd:"displayName,+nullable"`
	Description               string `dd:"description,+nullable"`
	IsDefault                 bool   `dd:"isDefault,+nullable"`
	Hidden                    bool   `dd:"hidden,+nullable"`
	DefaultReasoningEffort    string `dd:"defaultReasoningEffort,+nullable"`
	SupportedReasoningEfforts any    `dd:"supportedReasoningEfforts,+opaque,+nullable"` // a list of strings or of {reasoningEffort}
}

type configReadParams struct {
	Cwd           string `dd:"cwd"`
	IncludeLayers bool   `dd:"includeLayers"`
}

type configRead struct {
	Config map[string]any `dd:"config,+opaque,+nullable"`
}

type settingsUpdated struct {
	ThreadID       string         `dd:"threadId"`
	ThreadSettings map[string]any `dd:"threadSettings,+opaque,+nullable"`
}

type initializeResult struct {
	UserAgent string `dd:"userAgent,+nullable"`
}

type turnSteerParams struct {
	ThreadID            string      `dd:"threadId"`
	ExpectedTurnID      string      `dd:"expectedTurnId"`
	Input               []textInput `dd:"input"`
	ClientUserMessageID string      `dd:"clientUserMessageId"`
}

type turnInterruptParams struct {
	ThreadID string `dd:"threadId"`
	TurnID   string `dd:"turnId"`
}

type turnResponse struct {
	Turn turn `dd:"turn"`
}

// notification params.
type turnNotification struct {
	ThreadID string `dd:"threadId"`
	Turn     turn   `dd:"turn"`
}

type itemNotification struct {
	ThreadID string         `dd:"threadId"`
	TurnID   string         `dd:"turnId"`
	Item     map[string]any `dd:"item,+opaque,+nullable"`
}

type deltaNotification struct {
	Delta string `dd:"delta"`
}

type resolvedNotification struct {
	RequestID any `dd:"requestId,+opaque,+nullable"`
}

// item is the subset of a ThreadItem the mapping reads.
type item struct {
	Type             string          `dd:"type"`
	ID               string          `dd:"id"`
	ClientID         string          `dd:"clientId,+nullable"`
	Content          any             `dd:"content,+opaque,+nullable"`
	Text             string          `dd:"text,+nullable"`
	Questions        []asyncQuestion `dd:"questions,+nullable"`
	Command          string          `dd:"command,+nullable"`
	Status           string          `dd:"status,+nullable"`
	ExitCode         *int            `dd:"exitCode,+nullable"`
	AggregatedOutput string          `dd:"aggregatedOutput,+nullable"`
	Changes          []fileChange    `dd:"changes,+nullable"`
	Server           string          `dd:"server,+nullable"`
	Tool             string          `dd:"tool,+nullable"`
	Query            string          `dd:"query,+nullable"`
	Error            map[string]any  `dd:"error,+opaque,+nullable"`
	Result           map[string]any  `dd:"result,+opaque,+nullable"`
	Arguments        any             `dd:"arguments,+opaque,+nullable"`
}

// asyncQuestion is the default-mode question an agentMessage carries when the
// model cannot raise a blocking one (capability report, 0.166.0).
type asyncQuestion struct {
	Title   string   `dd:"title"`
	Options []string `dd:"options,+nullable"`
}

type fileChange struct {
	Path string `dd:"path"`
	Diff string `dd:"diff,+nullable"`
}

// server request params.
type approvalParams struct {
	TurnID      string         `dd:"turnId"`
	ItemID      string         `dd:"itemId"`
	Command     string         `dd:"command,+nullable"`
	Cwd         string         `dd:"cwd,+nullable"`
	Reason      string         `dd:"reason,+nullable"`
	GrantRoot   string         `dd:"grantRoot,+nullable"`
	Permissions map[string]any `dd:"permissions,+opaque,+nullable"`
}

type userInputParams struct {
	TurnID    string              `dd:"turnId"`
	Questions []userInputQuestion `dd:"questions,+nullable"`
}

type userInputQuestion struct {
	ID       string            `dd:"id"`
	Header   string            `dd:"header,+nullable"`
	Question string            `dd:"question"`
	Options  []userInputOption `dd:"options,+nullable"`
}

type userInputOption struct {
	Label       string `dd:"label"`
	Description string `dd:"description,+nullable"`
}

// responses to server requests.
type decisionResult struct {
	Decision string `dd:"decision"`
}

type permissionsResult struct {
	Permissions map[string]any `dd:"permissions,+opaque"`
	Scope       string         `dd:"scope"`
}

type userInputResult struct {
	Answers map[string]userInputAnswer `dd:"answers"`
}

type userInputAnswer struct {
	Answers []string `dd:"answers"`
}

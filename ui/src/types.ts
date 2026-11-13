// wire types shared with the service's JSON API. keep in step with internal/server.

export interface Project {
  id: string
  name: string
  checkout: string
  // starred projects form their own group at the top of the tree.
  starred: boolean
  untrusted: boolean
  // checkout_missing: why the checkout cannot be worked in now ("does not
  // exist", "not a directory", "not readable"), computed on every read;
  // absent when it can.
  checkout_missing?: string
  // the project's environment as it stands now; it comes with the workspace
  // read.
  environment?: ProjectEnvironment
}

// ProjectEnvironment is a project's shell environment: the repository's
// .mega/environment.yaml as read now, the operator's approval (a fingerprint
// and when) and override, where the activation comes from, whether work may
// start, the latest resolution (names and a fingerprint, never values), and
// what each harness cannot find in it.
export interface ProjectEnvironment {
  file: {
    path: string
    present: boolean
    content?: string
    // the full sha256 an approval binds, and its short form for display.
    digest?: string
    fingerprint?: string
    modified_at?: string
    malformed?: string
  }
  // the approval: its full digest (absent for one stored in the old short
  // form, which no longer counts), its short fingerprint, when, and the names
  // the activation changed then, once known.
  approved?: { digest?: string; fingerprint: string; at: string; changed: string[]; names_known: boolean }
  override?: { ignore: boolean; activate?: string }
  // none (no file), file, ignored (the override ignores the file), override.
  source: 'none' | 'file' | 'ignored' | 'override'
  status: 'ready' | 'unapproved' | 'changed' | 'malformed' | 'names_changed'
  refusal?: { code: string; message: string }
  // the latest resolution; added and removed are the names it changed beyond,
  // and short of, the approval's, when they differ.
  resolved?: { at: string; source: string; changed: string[]; fingerprint?: string; error?: string; added?: string[]; removed?: string[] }
  missing: { harness: string; executable: string; reason: string }[]
}

// a running runtime's launch environment: names and fingerprints only.
export interface RuntimeEnvironment {
  source: string
  activation?: string
  changed: string[]
  fingerprint: string
}

// what a conversation asks of the operator, computed by the service.
export type Attention = 'request' | 'unsupervised' | 'running' | 'error' | 'unread' | 'idle'

// the runtime as the service sees it now.
export type ConversationState = 'none' | 'starting' | 'running' | 'stopping' | 'resuming' | 'unsupervised'

export interface PendingRequest {
  id: string
  kind: 'permission' | 'question'
  payload: RequestPayload | null
  // when the request was recorded, which is when its wait began.
  at?: string
}

// plain data the adapter builds for the operator; harness-specific but not protocol.
export interface RequestPayload {
  tool?: string
  input?: Record<string, unknown>
  questions?: Question[]
  title?: string
  display_name?: string
  description?: string
  decision_reason?: string
  blocked_path?: string
  // the mode the request was raised in.
  permission_mode?: string
  // an MCP tool approval: the server and tool, the parameters as the harness
  // displays them, and whether the harness offers approval for the session.
  mcp_server?: string
  mcp_tool?: string
  parameters?: { name: string; value: unknown; display_name?: string }[]
  allow_session?: boolean
  // a typed form an MCP server asked for: its message and fields.
  message?: string
  fields?: FormField[]
}

export interface FormField {
  name: string
  type: 'string' | 'number' | 'integer' | 'boolean' | 'enum'
  title?: string
  description?: string
  required: boolean
  options?: { value: string; label: string }[]
  default?: unknown
}

export interface Question {
  question: string
  header?: string
  options?: { label: string; description?: string }[]
  multiSelect?: boolean
}

// one piece of work a runtime keeps running outside a turn: a backgrounded
// command or subagent (claude), or a command left running as a terminal
// session (codex). started_at is when mega first saw it.
export interface BackgroundJob {
  id: string
  description: string
  kind: string
  started_at?: string
}

export interface Conversation {
  id: string
  project_id: string
  name: string
  label: string
  harness: string
  native_id: string
  mode: string
  state: ConversationState
  turn_active: boolean
  // when the turn in flight began, from the service's memory; absent when no
  // turn is in flight.
  turn_started_at?: string
  attention: Attention
  read_seq: number
  last_seq: number
  output_seq: number
  model?: string
  version?: string
  // the running mode, as the runtime last reported or mega last confirmed
  // it.
  permission_mode?: string
  // the request: what every launch (and, for codex, every prompt) passes.
  // concrete from creation; effort_choice is absent only for a model that
  // takes no effort level (or, rarely, one still to be chosen), and
  // permission_mode_choice for codex.
  permission_mode_choice?: string
  model_choice?: string
  effort_choice?: string
  // the effort the harness reported, where it reports one (codex). claude
  // never does, so its effort is only ever what was requested.
  effort?: string
  // what the turn in flight asked for and, from its first output, ran with,
  // and where the two differ. a finished turn carries its own on its
  // turn_end event.
  turn_asked?: TurnSettings
  turn_ran?: TurnSettings
  turn_differs?: string[]
  pending: PendingRequest[]
  // the runtime's background jobs; absent when none, or when there is no
  // runtime to ask (unknown, for an unsupervised conversation).
  background?: BackgroundJob[]
  // what the running runtime was launched with; absent with no runtime.
  environment?: RuntimeEnvironment
  // the runtime would start in another environment now.
  environment_stale?: boolean
  // the runtime's MCP servers, counted by status; absent until it has
  // reported them, and with no runtime. the servers are on their own read.
  mcp?: McpSummary
}

export interface McpSummary {
  servers: number
  connected: number
  connecting: number
  failed: number
  needs_sign_in: number
  disabled: number
}

export type McpStatus = 'connected' | 'connecting' | 'failed' | 'needs_sign_in' | 'disabled'

// ConversationMcp is a conversation's MCP servers in full
// (GET /api/conversations/{id}/mcp): read only, and from memory.
export interface ConversationMcp {
  running: boolean
  reported: boolean
  servers: McpServer[]
}

export interface McpServer {
  name: string
  status: McpStatus
  error?: string
  tools: { name: string; description?: string }[]
}

// one model a harness offers, and the effort levels it takes (none for a
// model without effort levels).
export interface ModelOption {
  id: string
  resolved?: string
  display_name?: string
  description?: string
  efforts: string[]
  default_effort?: string
  is_default: boolean
}

// what a harness offers for a checkout, and what a new conversation there
// copies from the harness's settings. need_effort: the copied model takes an
// effort level and the settings name none, so the operator chooses one.
export interface Models {
  version?: string
  models: ModelOption[]
  copy: { model?: string; effort?: string; mode?: string; need_effort?: boolean }
}

// a turn's model, effort and (claude) permission mode.
export interface TurnSettings {
  model?: string
  effort?: string
  mode?: string
}

// what a turn asked for and what the harness reported it ran with; differs
// names the fields where they are not the same.
export interface TurnRecord {
  asked: TurnSettings
  ran: TurnSettings
  differs?: string[]
}

// About is what the service says of its own build (GET /api/version): what
// 'mega version' prints, where it logs, the Claude Agent SDK beside the shim,
// and the identity of the interface it serves. it is not workspace state.
export interface About {
  version: string
  developer: boolean
  commit?: string
  built?: string
  branch?: string
  go: string
  target: string
  log_file: string
  logs_to_file: boolean
  claude_sdk: string
  interface: string
}

export interface Workspace {
  projects: Project[]
  conversations: Conversation[]
  // the conversations waiting on the operator, in the order to take them.
  needs_you: string[]
  // the "shut down when idle" gate and what is in its way.
  shutdown?: ShutdownGate
}

// Blocker is one conversation holding the shutdown gate, by kind.
export interface Blocker {
  conversation_id: string
  project_id: string
  name: string
  turn: boolean
  requests: number
  background: number
  // settling: a runtime starting, or a conversation resuming.
  settling?: string
}

// ShutdownGate is held by the service in memory: armed, since when, whether
// nothing is in the way, and what is.
export interface ShutdownGate {
  armed: boolean
  armed_at?: string
  idle: boolean
  blockers: Blocker[]
}

export type EventKind =
  | 'runtime'
  | 'user'
  | 'assistant'
  | 'tool_use'
  | 'tool_result'
  | 'request'
  | 'request_resolved'
  | 'turn_end'
  | 'native'

// History is a stretch of a conversation's history, from through through.
// native events come only when asked for, but the stretch covers them.
// earlier is present for a read by turns: what lies before from.
export interface History {
  events: ConversationEvent[]
  from: number
  through: number
  earlier?: { turns: number; events: number }
}

export interface ConversationEvent {
  seq: number
  at: string
  kind: EventKind
  origin: 'managed' | 'native' | 'service'
  text?: string
  native_id?: string
  status?: string
  tool?: string
  tool_use_id?: string
  req?: { id: string; kind: string; payload?: RequestPayload | null; outcome?: string }
  // mode_confirmed: a mode report, in older logs, that only confirmed the
  // mode a start with no prompt requested; recorded, never drawn.
  facts?: { session?: string; model?: string; effort?: string; version?: string; permission_mode?: string; mode_confirmed?: boolean; cwd?: string }
  // turn: on a turn_end, what the turn asked for and ran with; absent on
  // turns recorded before mega kept it, and where the harness said nothing.
  turn?: TurnRecord
  raw?: unknown
}

export interface StreamFrame {
  heartbeat?: string
  workspace_changed?: boolean
  conversation_id?: string
  event?: ConversationEvent
  delta?: string
}

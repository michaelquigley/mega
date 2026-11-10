// wire types shared with the service's JSON API. keep in step with internal/server.

export interface Project {
  id: string
  name: string
  checkout: string
  // starred projects form their own group at the top of the tree.
  starred: boolean
  untrusted: boolean
  // the claude mode the operator's settings name for the checkout, which a
  // launch with no choice inherits; empty when none does.
  inherited_permission_mode: string
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
  // the running mode, as the runtime last reported or roundhouse last
  // confirmed it, and whether the operator's settings chose it.
  permission_mode?: string
  permission_mode_inherited: boolean
  // the recorded choice every launch passes; empty is roundhouse's default.
  permission_mode_choice: string
  // the recorded model and effort every launch (and, for codex, every prompt)
  // passes; absent inherits the operator's settings.
  model_choice?: string
  effort_choice?: string
  // the effort the harness reported, where it reports one (codex). claude
  // never does, so its effort is only ever what was requested.
  effort?: string
  // a change asked for while running that the next prompt carries.
  model_next?: string
  effort_next?: string
  pending: PendingRequest[]
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

// what a harness offers for a checkout, and what a launch with no choice gets
// there, where the harness's settings say.
export interface Models {
  version?: string
  models: ModelOption[]
  inherited: { model?: string; effort?: string }
}

export interface Workspace {
  projects: Project[]
  conversations: Conversation[]
  // the conversations waiting on the operator, in the order to take them.
  needs_you: string[]
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
  facts?: { session?: string; model?: string; effort?: string; version?: string; permission_mode?: string; cwd?: string }
  raw?: unknown
}

export interface StreamFrame {
  heartbeat?: string
  workspace_changed?: boolean
  conversation_id?: string
  event?: ConversationEvent
  delta?: string
}

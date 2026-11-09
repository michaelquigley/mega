// wire types shared with the service's JSON API. keep in step with internal/server.

export interface Project {
  id: string
  name: string
  checkout: string
  untrusted: boolean
}

// the runtime as the service sees it now.
export type ConversationState = 'none' | 'starting' | 'running' | 'stopping' | 'resuming' | 'unsupervised'

export interface PendingRequest {
  id: string
  kind: 'permission' | 'question'
  payload: RequestPayload | null
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
  read_seq: number
  last_seq: number
  output_seq: number
  model?: string
  version?: string
  permission_mode?: string
  pending: PendingRequest[]
}

export interface Workspace {
  projects: Project[]
  conversations: Conversation[]
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
  facts?: { session?: string; model?: string; version?: string; permission_mode?: string; cwd?: string }
  raw?: unknown
}

export interface StreamFrame {
  heartbeat?: string
  workspace_changed?: boolean
  conversation_id?: string
  event?: ConversationEvent
  delta?: string
}

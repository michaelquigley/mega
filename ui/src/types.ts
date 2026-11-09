// wire types shared with the service's JSON API. keep in step with internal/server.

export interface Project {
  id: string
  name: string
  checkout: string
  untrusted: boolean
}

export interface Conversation {
  id: string
  project_id: string
  name: string
  label: string
  harness: string
  native_id: string
  mode: string
  read_seq: number
  last_seq: number
  output_seq: number
}

export interface Workspace {
  projects: Project[]
  conversations: Conversation[]
}

export interface StreamFrame {
  heartbeat?: string
  workspace_changed?: boolean
}

import type { Conversation, ConversationEvent, Models, Project, Workspace } from './types'

export class ApiError extends Error {
  readonly code: string

  constructor(code: string, message: string) {
    super(message)
    this.code = code
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const response = await fetch(path, {
    method,
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const text = await response.text()
  if (response.ok && text === '') {
    return undefined as T
  }
  let parsed: unknown
  try {
    parsed = JSON.parse(text)
  } catch {
    throw new ApiError('unreadable', text || `request failed with status ${response.status}`)
  }
  if (!response.ok) {
    const failure = parsed as { code?: string; message?: string }
    throw new ApiError(failure.code ?? 'unknown', failure.message ?? `request failed with status ${response.status}`)
  }
  return parsed as T
}

export const getWorkspace = () => request<Workspace>('GET', '/api/workspace')

export const createProject = (name: string, checkout: string) =>
  request<Project>('POST', '/api/projects', { name, checkout })

export const renameProject = (id: string, name: string) =>
  request<Project>('PATCH', `/api/projects/${encodeURIComponent(id)}`, { name })

export const removeProject = (id: string) =>
  request<void>('DELETE', `/api/projects/${encodeURIComponent(id)}`)

const conversationPath = (id: string) => `/api/conversations/${encodeURIComponent(id)}`

// what the operator chose at launch; an empty field inherits.
export interface LaunchChoice {
  permissionMode: string
  model: string
  effort: string
}

export const startConversation = (projectId: string, harness: string, name: string, prompt: string, choice: LaunchChoice) =>
  request<Conversation>('POST', `/api/projects/${encodeURIComponent(projectId)}/conversations`, {
    harness,
    name,
    prompt,
    permission_mode: choice.permissionMode,
    model: choice.model,
    effort: choice.effort,
  })

export const getModels = (projectId: string, harness: string) =>
  request<Models>('GET', `/api/projects/${encodeURIComponent(projectId)}/models?harness=${encodeURIComponent(harness)}`)

// the harness's answer to a switch. only a confirmed switch moves the live mode,
// and only once the runtime's report of it is recorded. next_prompt is a model
// or effort change codex takes with the next prompt.
export interface ModeOutcome {
  outcome: 'confirmed' | 'refused' | 'unconfirmed' | 'stored' | 'next_prompt'
  message?: string
}

export const setModel = (id: string, model: string, effort: string) =>
  request<ModeOutcome>('POST', `${conversationPath(id)}/model`, { model, effort })

export const setPermissionMode = (id: string, mode: string) =>
  request<ModeOutcome>('POST', `${conversationPath(id)}/permission-mode`, { mode })

export const updateConversation = (id: string, name: string, label: string) =>
  request<Conversation>('PATCH', conversationPath(id), { name, label })

export const removeConversation = (id: string) => request<void>('DELETE', conversationPath(id))

export const getEvents = (id: string, since: number) =>
  request<{ events: ConversationEvent[] }>('GET', `${conversationPath(id)}/events?since=${since}`)

export const sendPrompt = (id: string, text: string) => request<void>('POST', `${conversationPath(id)}/prompt`, { text })

export interface Answer {
  decision?: 'allow' | 'deny' | 'allow_session'
  message?: string
  answers?: Record<string, string>
}

export const answerRequest = (id: string, requestId: string, answer: Answer) =>
  request<void>('POST', `${conversationPath(id)}/answer`, { request_id: requestId, ...answer })

export const interruptConversation = (id: string) => request<void>('POST', `${conversationPath(id)}/interrupt`)

export const releaseConversation = (id: string) => request<void>('POST', `${conversationPath(id)}/release`)

export const resumeConversation = (id: string) => request<void>('POST', `${conversationPath(id)}/resume`)

export interface DiscoveredSession {
  native_id: string
  preview: string
  created_at?: string
  updated_at?: string
}

export const discoverSessions = (projectId: string, harness: string) =>
  request<{ sessions: DiscoveredSession[] }>(
    'GET',
    `/api/projects/${encodeURIComponent(projectId)}/discover?harness=${encodeURIComponent(harness)}`,
  )

export const adoptSession = (projectId: string, harness: string, nativeId: string, name: string) =>
  request<Conversation>('POST', `/api/projects/${encodeURIComponent(projectId)}/adopt`, {
    harness,
    native_id: nativeId,
    name,
  })

export const readConversation = (id: string, seq: number) =>
  request<void>('POST', `${conversationPath(id)}/read`, { seq })

import type { Conversation, History, Models, Project, Workspace } from './types'

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

// starring is workspace state: the service writes it, and every browser learns
// of it through the stream's workspace_changed.
export const starProject = (id: string, starred: boolean) =>
  request<Project>('PATCH', `/api/projects/${encodeURIComponent(id)}`, { starred })

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

// paths in a project's checkout matching q, best first, at most twenty;
// directories end in a slash. names only: the service reads no file.
export const getPaths = (projectId: string, q: string) =>
  request<{ paths: string[] }>('GET', `/api/projects/${encodeURIComponent(projectId)}/paths?q=${encodeURIComponent(q)}`).then(
    (answer) => answer.paths,
  )

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

// HistoryRead names a part of a conversation's history: the newest turns
// (turns alone), the turns before a seq (before, with turns), everything
// before a seq (before alone), or everything after one (since).
export interface HistoryRead {
  turns?: number
  before?: number
  since?: number
  native: boolean
}

// historyPath is the request a history read makes, which the page also shows
// while it waits.
export function historyPath(id: string, read: HistoryRead): string {
  const query = new URLSearchParams()
  if (read.since !== undefined) {
    query.set('since', String(read.since))
  }
  if (read.before !== undefined) {
    query.set('before', String(read.before))
  }
  if (read.turns !== undefined) {
    query.set('turns', String(read.turns))
  }
  if (read.native) {
    query.set('native', '1')
  }
  return `${conversationPath(id)}/events?${query.toString()}`
}

export const getHistory = (id: string, read: HistoryRead) => request<History>('GET', historyPath(id, read))

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

// startRuntime starts a stopped conversation's runtime on its session, sending
// nothing.
export const startRuntime = (id: string) => request<void>('POST', `${conversationPath(id)}/start`)

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

// the "shut down when idle" gate. arm and cancel are announced to every page;
// shut down is refused unless armed and idle, and then stops the service as a
// signal does.
export const armShutdown = () => request<void>('POST', '/api/shutdown/arm')
export const cancelShutdown = () => request<void>('POST', '/api/shutdown/cancel')
export const shutDown = () => request<void>('POST', '/api/shutdown')

// the operator approves the file at the fingerprint the page showed; a file
// that has changed since is refused.
export const approveEnvironment = (projectId: string, fingerprint: string) =>
  request<Project>('POST', `/api/projects/${encodeURIComponent(projectId)}/environment/approve`, { fingerprint })

// the operator's local override: ignore the repository's file, or run another
// activation in its place.
export const setEnvironmentOverride = (projectId: string, override: { ignore: boolean; activate?: string }) =>
  request<Project>('PUT', `/api/projects/${encodeURIComponent(projectId)}/environment/override`, override)

export const clearEnvironmentOverride = (projectId: string) =>
  request<Project>('DELETE', `/api/projects/${encodeURIComponent(projectId)}/environment/override`)

// a live resolution: names and a fingerprint, and the changed variables'
// values only when asked for. the values live in the asking page's memory.
export interface Resolved {
  at: string
  source: string
  changed: string[]
  fingerprint: string
  values?: Record<string, string>
}

export const resolveEnvironment = (projectId: string, values: boolean) =>
  request<Resolved>('POST', `/api/projects/${encodeURIComponent(projectId)}/environment/resolve${values ? '?values=1' : ''}`)

// the approved version, found in the checkout's git history by its
// fingerprint; nothing but the fingerprint is stored.
export interface ApprovedVersion {
  fingerprint: string
  at: string
  available: boolean
  content?: string
  found?: string
}

export const getApprovedVersion = (projectId: string) =>
  request<ApprovedVersion>('GET', `/api/projects/${encodeURIComponent(projectId)}/environment/approved`)

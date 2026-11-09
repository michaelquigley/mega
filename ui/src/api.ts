import type { Conversation, ConversationEvent, Project, Workspace } from './types'

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

export const startConversation = (projectId: string, harness: string, name: string, prompt: string) =>
  request<Conversation>('POST', `/api/projects/${encodeURIComponent(projectId)}/conversations`, { harness, name, prompt })

export const updateConversation = (id: string, name: string, label: string) =>
  request<Conversation>('PATCH', conversationPath(id), { name, label })

export const getEvents = (id: string, since: number) =>
  request<{ events: ConversationEvent[] }>('GET', `${conversationPath(id)}/events?since=${since}`)

export const sendPrompt = (id: string, text: string) => request<void>('POST', `${conversationPath(id)}/prompt`, { text })

export interface Answer {
  decision?: 'allow' | 'deny'
  message?: string
  answers?: Record<string, string>
}

export const answerRequest = (id: string, requestId: string, answer: Answer) =>
  request<void>('POST', `${conversationPath(id)}/answer`, { request_id: requestId, ...answer })

export const interruptConversation = (id: string) => request<void>('POST', `${conversationPath(id)}/interrupt`)

export const releaseConversation = (id: string) => request<void>('POST', `${conversationPath(id)}/release`)

export const resumeConversation = (id: string) => request<void>('POST', `${conversationPath(id)}/resume`)

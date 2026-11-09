import type { Project, Workspace } from './types'

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
  if (response.status === 204) {
    return undefined as T
  }
  const text = await response.text()
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

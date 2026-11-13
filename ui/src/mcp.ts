import { harnesses } from './harnesses'
import type { Conversation, McpStatus } from './types'

// the words for each MCP server status.
export const mcpStatusLabels: Record<McpStatus, string> = {
  connected: 'Connected',
  connecting: 'Connecting',
  failed: 'Failed',
  needs_sign_in: 'Needs sign-in',
  disabled: 'Disabled',
}

// mcpRunning is whether there is a runtime to ask for the servers: while it
// starts, runs or stops. a resume only imports history, with no runtime.
export const mcpRunning = (conversation: Conversation) => ['starting', 'running', 'stopping'].includes(conversation.state)

// mcpSummaryText is the header's summary: how many servers, and how many are
// anything but connected.
export function mcpSummaryText(conversation: Conversation): string {
  if (!mcpRunning(conversation)) {
    return 'MCP servers: not running'
  }
  const s = conversation.mcp
  if (!s) {
    return 'MCP servers: not reported yet'
  }
  if (s.servers === 0) {
    return 'No MCP servers'
  }
  const parts = [`${s.servers} MCP ${s.servers === 1 ? 'server' : 'servers'}`]
  if (s.needs_sign_in > 0) {
    parts.push(`${s.needs_sign_in} ${s.needs_sign_in === 1 ? 'needs' : 'need'} sign-in`)
  }
  if (s.failed > 0) {
    parts.push(`${s.failed} failed`)
  }
  if (s.connecting > 0) {
    parts.push(`${s.connecting} connecting`)
  }
  if (s.disabled > 0) {
    parts.push(`${s.disabled} disabled`)
  }
  return parts.join(', ')
}

// mcpNotice is whether any server is not connected, which the summary shows
// in the notice colour.
export const mcpNotice = (conversation: Conversation) => mcpRunning(conversation) && !!conversation.mcp && conversation.mcp.connected < conversation.mcp.servers

export const harnessName = (value: string) => harnesses.find((h) => h.value === value)?.label ?? value

// signInHint says where a server that needs signing in is signed in to: the
// harness's own terminal. mega starts no sign-in.
export function signInHint(harness: string, server: string): string {
  if (harness === 'codex') {
    return `Sign in from a terminal with codex mcp login ${server}. mega does not sign in for you.`
  }
  if (harness === 'claude') {
    return 'Sign in from Claude Code in a terminal: run claude in this checkout and use /mcp. mega does not sign in for you.'
  }
  return `Sign in from ${harnessName(harness)} in a terminal. mega does not sign in for you.`
}

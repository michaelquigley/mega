import { useEffect, useState } from 'react'
import { getMcp } from '../api'
import { harnessName, mcpNotice, mcpRunning, mcpStatusLabels, mcpSummaryText, signInHint } from '../mcp'
import type { Conversation, ConversationMcp } from '../types'
import { Dialog } from './Dialog'

// McpSummary is the conversation's MCP servers in one line of its header, in
// the notice colour while any is not connected. a press opens the list. it
// is read only, and never attention.
export function McpSummary({ conversation }: { conversation: Conversation }) {
  const [open, setOpen] = useState(false)
  const text = mcpSummaryText(conversation)
  const label = `${text}: show the servers and their tools`
  return (
    <>
      <button type="button" className={`mcp-summary${mcpNotice(conversation) ? ' notice' : ''}`} title={label} aria-label={label} onClick={() => setOpen(true)}>
        {text}
      </button>
      {open && <McpPanel conversation={conversation} onClose={() => setOpen(false)} />}
    </>
  )
}

// McpPanel lists the servers, each with its status and its error, and its
// tools on expansion; a server that needs signing in says where that is
// done. it reads the list when it opens and again whenever the summary
// changes while it is open.
function McpPanel({ conversation, onClose }: { conversation: Conversation; onClose: () => void }) {
  const [read, setRead] = useState<ConversationMcp | null>(null)
  const [error, setError] = useState<string | null>(null)
  const changed = `${conversation.state}:${JSON.stringify(conversation.mcp ?? null)}`
  useEffect(() => {
    let current = true
    getMcp(conversation.id).then(
      (next) => {
        if (current) {
          setRead(next)
          setError(null)
        }
      },
      (failure: unknown) => {
        if (current) {
          setError(failure instanceof Error ? failure.message : String(failure))
        }
      },
    )
    return () => {
      current = false
    }
  }, [conversation.id, changed])
  const harness = harnessName(conversation.harness)
  let body
  if (error) {
    body = <p className="error">The servers could not be read: {error}</p>
  } else if (!read) {
    body = <p className="muted">Reading the servers.</p>
  } else if (!read.running || !mcpRunning(conversation)) {
    body = <p>This conversation is not running, so there are no servers to ask. Start it from the input panel to see them.</p>
  } else if (!read.reported) {
    body = <p className="muted">Waiting for {harness} to report its servers.</p>
  } else if (read.servers.length === 0) {
    body = <p>{harness} has no MCP servers in this conversation.</p>
  } else {
    body = (
      <ul className="mcp-servers">
        {read.servers.map((server) => (
          <li key={server.name} className={`mcp-server mcp-${server.status}`}>
            <details>
              <summary>
                <span className="mcp-name">{server.name}</span>
                <span className="mcp-status">{mcpStatusLabels[server.status] ?? server.status}</span>
                <span className="muted mcp-count">
                  {server.tools.length} {server.tools.length === 1 ? 'tool' : 'tools'}
                </span>
              </summary>
              {server.tools.length === 0 ? (
                <p className="muted">No tools{server.status === 'connected' ? '' : ' while it is not connected'}.</p>
              ) : (
                <ul className="mcp-tools">
                  {server.tools.map((tool) => (
                    <li key={tool.name}>
                      <code>{tool.name}</code>
                      {tool.description && <span className="muted"> {tool.description}</span>}
                    </li>
                  ))}
                </ul>
              )}
            </details>
            {server.error && <p className="error">{server.error}</p>}
            {server.status === 'needs_sign_in' && <p className="notice">{signInHint(conversation.harness, server.name)}</p>}
          </li>
        ))}
      </ul>
    )
  }
  return (
    <Dialog title="MCP servers" onClose={onClose}>
      {body}
      <p className="muted">Read only: reconnecting, enabling and disabling servers are done in {harness} itself.</p>
    </Dialog>
  )
}

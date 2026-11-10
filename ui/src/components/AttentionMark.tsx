import { elapsedSince, shortElapsed, useNow } from '../clock'
import type { Attention, Conversation } from '../types'

const titles: Record<Attention, string> = {
  request: 'Waiting for your answer',
  unsupervised: 'Supervision lost; execution uncertain',
  running: 'Working',
  error: 'The runtime ended in an error',
  unread: 'Unread output',
  idle: 'Nothing waiting',
}

// Mark is the one shape vocabulary for attention: a square for a request, a
// filled red dot for supervision lost, a red ring for an error, a filled dot
// for unread, a rotating arc for working, a grey ring for idle. a stale mark
// is only last known: dimmed, and never moving.
function Mark({ attention, title, stale }: { attention: Attention; title: string; stale: boolean }) {
  const shown = stale ? `Last known: ${title.charAt(0).toLowerCase()}${title.slice(1)}` : title
  return <span className={`mark mark-${attention}${stale ? ' mark-stale' : ''}`} title={shown} aria-label={shown} role="img" />
}

// AttentionMark shows what a conversation asks of the operator, as the
// service computes it.
export function AttentionMark({ conversation, stale }: { conversation: Conversation; stale: boolean }) {
  const attention = conversation.attention
  let title = titles[attention]
  if (attention === 'request' && conversation.pending.length > 1) {
    title = `${conversation.pending.length} requests are waiting for your answer`
  }
  return <Mark attention={attention} title={title} stale={stale} />
}

// the project mark's order: what needs the operator outranks working, so the
// mark answers "does this project need me, and if not, is it busy".
const projectOrder: Attention[] = ['request', 'unsupervised', 'error', 'unread', 'running', 'idle']

const projectTitles: Record<Attention, (n: number) => string> = {
  request: (n) => `${n} waiting for your answer`,
  unsupervised: (n) => `${n} with supervision lost`,
  error: (n) => `${n} ended in an error`,
  unread: (n) => `${n} unread`,
  running: (n) => `${n} working`,
  idle: (n) => `${n} idle`,
}

// ProjectMark is one mark for a project: the most urgent attention among its
// conversations, derived from their confirmed attention, titled with how
// many share it.
export function ProjectMark({ conversations, stale }: { conversations: Conversation[]; stale: boolean }) {
  const attention = projectOrder.find((state) => conversations.some((c) => c.attention === state)) ?? 'idle'
  const count = conversations.filter((c) => c.attention === attention).length
  const title = conversations.length === 0 ? 'No conversations' : projectTitles[attention](count)
  return <Mark attention={attention} title={title} stale={stale} />
}

// Elapsed is how long a conversation's turn in flight has run, beside its
// name; nothing when no turn is in flight.
export function Elapsed({ conversation }: { conversation: Conversation }) {
  const clock = useNow()
  const elapsed = elapsedSince(conversation.turn_started_at, clock)
  if (elapsed === null) {
    return null
  }
  return (
    <span className="elapsed" title="How long the current turn has run">
      {shortElapsed(elapsed)}
    </span>
  )
}

// Spinner is the small working arc, for something live in flight.
export function Spinner({ label }: { label: string }) {
  return <span className="spinner" role="img" aria-label={label} title={label} />
}

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
// for unread, a rotating arc for working (muted when the work is only in the
// background), a small grey dot for idle. a stale mark is only last known:
// dimmed, and never moving.
function Mark({ attention, title, stale, quiet = false }: { attention: Attention; title: string; stale: boolean; quiet?: boolean }) {
  const shown = stale ? `Last known: ${title.charAt(0).toLowerCase()}${title.slice(1)}` : title
  return <span className={`mark mark-${attention}${quiet ? ' mark-background' : ''}${stale ? ' mark-stale' : ''}`} title={shown} aria-label={shown} role="img" />
}

const jobs = (n: number) => `${n} background ${n === 1 ? 'job' : 'jobs'} running`

// backgroundOnly is a conversation with nothing else to show but background
// work: its attention is idle, and the harness keeps jobs running.
const backgroundOnly = (c: Conversation) => c.attention === 'idle' && (c.background?.length ?? 0) > 0

// AttentionMark shows what a conversation asks of the operator, as the
// service computes it.
export function AttentionMark({ conversation, stale }: { conversation: Conversation; stale: boolean }) {
  const attention = conversation.attention
  if (backgroundOnly(conversation)) {
    // otherwise idle, but not doing nothing: the working arc, muted.
    return <Mark attention="running" quiet title={jobs(conversation.background!.length)} stale={stale} />
  }
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
  // background work counts as running for the project's order.
  const effective = (c: Conversation): Attention => (backgroundOnly(c) ? 'running' : c.attention)
  const attention = projectOrder.find((state) => conversations.some((c) => effective(c) === state)) ?? 'idle'
  const count = conversations.filter((c) => effective(c) === attention).length
  const title = conversations.length === 0 ? 'No conversations' : projectTitles[attention](count)
  return <Mark attention={attention} title={title} stale={stale} />
}

// Elapsed is a time beside a conversation's name: on one waiting for the
// operator, how long its oldest pending request has waited; otherwise how
// long its turn in flight has run; nothing when neither applies.
export function Elapsed({ conversation }: { conversation: Conversation }) {
  const clock = useNow()
  // the oldest by parsed time: the service's timestamps trim trailing zeros,
  // so their text does not sort.
  let raised: string | undefined
  for (const request of conversation.pending) {
    if (request.at && (!raised || Date.parse(request.at) < Date.parse(raised))) {
      raised = request.at
    }
  }
  const waiting = conversation.pending.length > 0
  const elapsed = waiting ? elapsedSince(raised, clock) : elapsedSince(conversation.turn_started_at, clock)
  if (elapsed === null) {
    return null
  }
  return (
    <span className="elapsed" title={waiting ? 'How long the request has waited for you' : 'How long the current turn has run'}>
      {shortElapsed(elapsed)}
    </span>
  )
}

// Spinner is the small working arc, for something live in flight.
export function Spinner({ label }: { label: string }) {
  return <span className="spinner" role="img" aria-label={label} title={label} />
}

// BackgroundLabel is "2 bg" after a conversation's name, so background work
// shows whatever its mark says; nothing when the mark is already the muted
// arc for it, or when there is none.
export function BackgroundLabel({ conversation }: { conversation: Conversation }) {
  const n = conversation.background?.length ?? 0
  if (n === 0 || backgroundOnly(conversation)) {
    return null
  }
  return (
    <span className="bg-label" title={jobs(n)} aria-label={jobs(n)}>
      {n} bg
    </span>
  )
}

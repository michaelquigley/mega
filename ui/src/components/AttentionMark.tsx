import type { Attention, Conversation } from '../types'

const titles: Record<Attention, string> = {
  request: 'Waiting for your answer',
  unsupervised: 'Supervision lost; execution uncertain',
  running: 'Working',
  error: 'The runtime ended in an error',
  unread: 'Unread output',
  idle: 'Nothing waiting',
}

// AttentionMark shows what a conversation asks of the operator, as the
// service computes it. while the stream is down it is only the last known
// attention, and says so.
export function AttentionMark({ conversation, stale }: { conversation: Conversation; stale: boolean }) {
  const attention = conversation.attention
  let title = titles[attention]
  if (attention === 'request' && conversation.pending.length > 1) {
    title = `${conversation.pending.length} requests are waiting for your answer`
  }
  if (stale) {
    title = `Last known: ${title.toLowerCase()}`
  }
  return <span className={`mark mark-${attention}${stale ? ' mark-stale' : ''}`} title={title} aria-label={title} />
}

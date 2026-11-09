import type { Conversation } from '../types'

// RuntimeMark shows a conversation's runtime as the service reports it. it is
// not attention: unread marks and the needs-you list come later. while the
// stream is down it is only the last known state, and says so.
export function RuntimeMark({ conversation, stale }: { conversation: Conversation; stale: boolean }) {
  const { state, turn_active: active, pending } = conversation
  let mark = 'mark-none'
  let title = 'No runtime'
  if (pending.length > 0) {
    mark = 'mark-request'
    title = pending.length === 1 ? 'A request is pending' : `${pending.length} requests are pending`
  } else if (state === 'unsupervised') {
    mark = 'mark-unsupervised'
    title = 'Supervision lost; execution uncertain'
  } else if (active) {
    mark = 'mark-active'
    title = 'Working'
  } else if (state === 'running') {
    mark = 'mark-running'
    title = 'Runtime idle'
  } else if (state !== 'none') {
    mark = 'mark-transition'
    title = `Runtime ${state}`
  }
  if (stale) {
    title = `Last known: ${title.toLowerCase()}`
  }
  return <span className={`mark ${mark}${stale ? ' mark-stale' : ''}`} title={title} aria-label={title} />
}

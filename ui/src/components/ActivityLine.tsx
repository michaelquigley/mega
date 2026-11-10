import { elapsedSince, longElapsed, useNow } from '../clock'
import { toolInFlight, type ConversationLog } from '../log'
import type { Conversation } from '../types'
import { Spinner } from './AttentionMark'

const transitions: Partial<Record<Conversation['state'], string>> = {
  starting: 'Starting',
  stopping: 'Stopping',
  resuming: 'Resuming',
}

// ActivityLine says, just above the reply box, what the conversation is doing
// now: a spinner, what is happening and for how long while a turn runs; the
// transitional states with the spinner; a pending request in the notice
// colour and without one, since nothing is moving until the operator acts.
// it is absent when nothing is happening. while the page shows only
// last-known state it shows that, labelled, and nothing moves.
export function ActivityLine({ conversation, log, stale }: { conversation: Conversation; log: ConversationLog | null; stale: boolean }) {
  const clock = useNow()
  const { state, pending, turn_active: active } = conversation
  let tone = 'live'
  let moving = true
  let parts: string[]
  let elapsed: string | null = null
  if (state === 'unsupervised') {
    tone = 'alert'
    moving = false
    parts = ['Supervision lost', 'execution uncertain']
  } else if (pending.length > 0) {
    tone = 'notice'
    moving = false
    const approvals = pending.every((request) => request.kind === 'permission')
    const answers = pending.every((request) => request.kind === 'question')
    parts = [approvals ? 'Waiting for your approval' : answers ? 'Waiting for your answer' : 'Waiting for your answers']
  } else if (transitions[state]) {
    parts = [transitions[state]]
  } else if (active) {
    const tool = toolInFlight(log?.events ?? [])
    if (log?.delta) {
      parts = ['Writing']
    } else if (tool?.text) {
      parts = ['Working', tool.text]
    } else {
      parts = ['Working']
    }
    const since = elapsedSince(conversation.turn_started_at, clock)
    elapsed = since === null ? null : longElapsed(since)
  } else {
    return null
  }
  const text = parts.join(' · ')
  // the elapsed time ticks every second, so it stays out of what a screen
  // reader announces.
  return (
    <div className={`activity activity-${tone}${stale ? ' activity-stale' : ''}`}>
      {moving && !stale && <Spinner label="In progress" />}
      <span className="activity-text">
        <span role="status">{stale ? `Last known: ${text}` : text}</span>
        {elapsed && <span aria-hidden="true"> · {elapsed}</span>}
      </span>
    </div>
  )
}

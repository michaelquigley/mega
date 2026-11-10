import { describeActivity, transitions } from '../activity'
import { elapsedSince, longElapsed, useNow } from '../clock'
import type { ConversationLog } from '../log'
import type { Conversation } from '../types'
import { Spinner } from './AttentionMark'

// ActivityLine says, just above the reply box, what the conversation is doing
// now: a spinner, what is happening and for how long while a turn runs; the
// transitional states with the spinner; a pending request in the notice
// colour and without one, since nothing is moving until the operator acts.
// it is absent when nothing is happening. while the page shows only
// last-known state it shows that, labelled, and nothing moves.
export function ActivityLine({ conversation, log, stale }: { conversation: Conversation; log: ConversationLog | null; stale: boolean }) {
  const clock = useNow()
  const activity = describeActivity(conversation, log, stale)
  if (!activity) {
    return null
  }
  let elapsed: string | null = null
  if (conversation.turn_active && conversation.pending.length === 0 && !transitions[conversation.state] && conversation.state !== 'unsupervised') {
    const since = elapsedSince(conversation.turn_started_at, clock)
    elapsed = since === null ? null : longElapsed(since)
  }
  // the elapsed time ticks every second, so it stays out of what a screen
  // reader announces.
  return (
    <div className={`activity activity-${activity.tone}${stale ? ' activity-stale' : ''}`}>
      {activity.moving && <Spinner label="In progress" />}
      <span className="activity-text">
        <span role="status">{activity.text}</span>
        {elapsed && <span aria-hidden="true"> · {elapsed}</span>}
      </span>
    </div>
  )
}

import { toolInFlight, type ConversationLog } from './log'
import type { Conversation } from './types'

export const transitions: Partial<Record<Conversation['state'], string>> = {
  starting: 'Starting',
  stopping: 'Stopping',
  resuming: 'Resuming',
}

export interface Activity {
  // live (the accent colour), background (muted: only background work),
  // notice (waiting for the operator) or alert.
  tone: 'live' | 'background' | 'notice' | 'alert'
  // whether something is moving, so a spinner may show.
  moving: boolean
  // what is happening, without the time; prefixed "Last known: " when stale.
  text: string
  // when the time shown began: the turn's start, or the oldest background
  // job's; and how it is written (long: 1m 12s; short: 4m).
  since?: string
  format?: 'long' | 'short'
  // after the time: the background jobs running beside a turn or a request.
  suffix?: string
}

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`

// describeActivity says what the conversation is doing now, or null when
// nothing is happening. the activity line shows it, and the reply box's
// placeholder uses the same words whenever a prompt cannot be sent, so the
// two never disagree.
export function describeActivity(conversation: Conversation, log: ConversationLog | null, stale: boolean): Activity | null {
  const { state, pending, turn_active: active } = conversation
  const background = conversation.background ?? []
  let activity: Activity
  if (state === 'unsupervised') {
    // with no runtime to ask, the background list is unknown, not empty.
    activity = { tone: 'alert', moving: false, text: 'Supervision lost · execution uncertain · background jobs unknown' }
  } else if (pending.length > 0) {
    const approvals = pending.every((request) => request.kind === 'permission')
    const answers = pending.every((request) => request.kind === 'question')
    const text = approvals ? 'Waiting for your approval' : answers ? 'Waiting for your answer' : 'Waiting for your answers'
    activity = { tone: 'notice', moving: false, text }
  } else if (transitions[state]) {
    activity = { tone: 'live', moving: true, text: transitions[state] }
  } else if (active) {
    const tool = toolInFlight(log?.events ?? [])
    let text = 'Working'
    if (log?.delta) {
      text = 'Writing'
    } else if (tool?.text) {
      text = `Working · ${tool.text}`
    }
    activity = { tone: 'live', moving: true, text, since: conversation.turn_started_at, format: 'long' }
  } else if (background.length > 0) {
    // no turn in flight, but the harness keeps work running: shown quietly.
    const oldest = background
      .map((job) => job.started_at)
      .filter((at): at is string => !!at)
      .sort((a, b) => Date.parse(a) - Date.parse(b))[0]
    const what = background.map((job) => job.description).join(', ')
    activity = { tone: 'background', moving: true, text: `${plural(background.length, 'background job', 'background jobs')} · ${what}`, since: oldest, format: 'short' }
  } else {
    return null
  }
  if (background.length > 0 && activity.tone !== 'background' && state !== 'unsupervised') {
    activity = { ...activity, suffix: `${background.length} in background` }
  }
  if (stale) {
    activity = { ...activity, moving: false, text: `Last known: ${activity.text}` }
  }
  return activity
}

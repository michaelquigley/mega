import { toolInFlight, type ConversationLog } from './log'
import type { Conversation } from './types'

export const transitions: Partial<Record<Conversation['state'], string>> = {
  starting: 'Starting',
  stopping: 'Stopping',
  resuming: 'Resuming',
}

export interface Activity {
  // live (the accent colour), notice (waiting for the operator) or alert.
  tone: 'live' | 'notice' | 'alert'
  // whether something is moving, so a spinner may show.
  moving: boolean
  // what is happening, without the time; prefixed "Last known: " when stale.
  text: string
}

// describeActivity says what the conversation is doing now, or null when
// nothing is happening. the activity line shows it, and the reply box's
// placeholder uses the same words whenever a prompt cannot be sent, so the
// two never disagree.
export function describeActivity(conversation: Conversation, log: ConversationLog | null, stale: boolean): Activity | null {
  const { state, pending, turn_active: active } = conversation
  let activity: Activity
  if (state === 'unsupervised') {
    activity = { tone: 'alert', moving: false, text: 'Supervision lost · execution uncertain' }
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
    activity = { tone: 'live', moving: true, text }
  } else {
    return null
  }
  if (stale) {
    activity = { ...activity, moving: false, text: `Last known: ${activity.text}` }
  }
  return activity
}

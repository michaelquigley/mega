import type { ConversationEvent } from './types'

// ConversationLog is the selected conversation's confirmed events plus the
// streaming text of the turn in flight, which is never stored.
export interface ConversationLog {
  id: string
  events: ConversationEvent[]
  delta: string
}

// merge folds events into a log by seq, so a replay and the live stream can
// arrive in any order without duplicating or losing anything.
export function merge(log: ConversationLog | null, id: string, events: ConversationEvent[]): ConversationLog {
  const base = log && log.id === id ? log : { id, events: [], delta: '' }
  const bySeq = new Map(base.events.map((event) => [event.seq, event]))
  let delta = base.delta
  for (const event of events) {
    bySeq.set(event.seq, event)
    if (event.kind === 'assistant' || event.kind === 'turn_end') {
      delta = ''
    }
  }
  return { id, events: [...bySeq.values()].sort((a, b) => a.seq - b.seq), delta }
}

// loadedThrough is the last seq of the unbroken run of events from seq 1, the
// part of the history known to be loaded. seqs in a log start at 1 with no gaps.
export function loadedThrough(events: ConversationEvent[]): number {
  let through = 0
  for (const event of events) {
    if (event.seq !== through + 1) {
      break
    }
    through = event.seq
  }
  return through
}

// toolInFlight is the newest tool call of the turn in flight that has no
// result yet: after the last turn end or runtime event, no matching
// tool_result. the caller says whether a turn is in flight at all.
export function toolInFlight(events: ConversationEvent[]): ConversationEvent | undefined {
  const results = new Set(events.filter((e) => e.kind === 'tool_result' && e.tool_use_id).map((e) => e.tool_use_id))
  for (let i = events.length - 1; i >= 0; i--) {
    const event = events[i]
    if (event.kind === 'turn_end' || event.kind === 'runtime') {
      return undefined
    }
    if (event.kind === 'tool_use' && !(event.tool_use_id && results.has(event.tool_use_id))) {
      return event
    }
  }
  return undefined
}

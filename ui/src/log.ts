import type { ConversationEvent, History } from './types'

// ConversationLog is the stretch of the selected conversation's history the
// page holds, plus the streaming text of the turn in flight, which is never
// stored. the stretch starts at from, a turn's first event, and is known
// unbroken through through: every event in between has been given to the page,
// whether it holds it or left it out (a native event while they are hidden).
// earlier says what lies before from. until the opening read lands, loaded is
// false and only events from the stream are held.
export interface ConversationLog {
  id: string
  events: ConversationEvent[]
  delta: string
  native: boolean
  loaded: boolean
  from: number
  through: number
  earlier: { turns: number; events: number } | null
  // seqs given past a gap after through, waiting for the gap to be filled.
  ahead: number[]
}

export const emptyLog = (id: string, native: boolean): ConversationLog => ({
  id,
  events: [],
  delta: '',
  native,
  loaded: false,
  from: 0,
  through: 0,
  earlier: null,
  ahead: [],
})

const isNative = (event: ConversationEvent) => event.kind === 'native'

// mergeEvents folds events in by seq, so a read and the live stream can arrive
// in any order without duplicating or losing anything.
function mergeEvents(held: ConversationEvent[], events: ConversationEvent[], delta: string) {
  const bySeq = new Map(held.map((event) => [event.seq, event]))
  for (const event of events) {
    bySeq.set(event.seq, event)
    if (event.kind === 'assistant' || event.kind === 'turn_end') {
      delta = ''
    }
  }
  return { events: [...bySeq.values()].sort((a, b) => a.seq - b.seq), delta }
}

// advance moves through over every seq already given past it, so a gap once
// filled joins what came after it.
function advance(log: ConversationLog, through: number, given: number[]): ConversationLog {
  const known = new Set([...log.ahead, ...given, ...log.events.map((event) => event.seq)])
  while (known.has(through + 1)) {
    through++
  }
  return { ...log, through, ahead: [...known].filter((seq) => seq > through + 1).sort((a, b) => a - b) }
}

// opened applies the opening read, the newest turns: the stretch now starts
// at its from, and the stream's events that arrived meanwhile stay.
export function opened(log: ConversationLog, read: History): ConversationLog {
  const { events, delta } = mergeEvents(
    log.events.filter((event) => event.seq >= read.from),
    read.events,
    log.delta,
  )
  const next = { ...log, events, delta, loaded: true, from: read.from, earlier: read.earlier ?? null }
  return advance(next, Math.max(read.through, read.from - 1), [])
}

// earlier puts a read of the turns before the stretch above it.
export function earlier(log: ConversationLog, read: History): ConversationLog {
  const { events } = mergeEvents(log.events, read.events, log.delta)
  return { ...log, events, from: read.from, earlier: read.earlier ?? null }
}

// caughtUp applies a read of everything after through, the read a reconnect
// makes for what the page missed.
export function caughtUp(log: ConversationLog, read: History): ConversationLog {
  const { events, delta } = mergeEvents(log.events, read.events, log.delta)
  return advance({ ...log, events, delta }, Math.max(log.through, read.through), [])
}

// received applies one event from the stream. a native event while they are
// hidden is dropped, but still counts as given, so it never reads as a gap.
export function received(log: ConversationLog, event: ConversationEvent): ConversationLog {
  if (isNative(event) && !log.native) {
    return log.loaded ? advance(log, log.through, [event.seq]) : { ...log, ahead: [...log.ahead, event.seq] }
  }
  const { events, delta } = mergeEvents(log.events, [event], log.delta)
  const next = { ...log, events, delta }
  return next.loaded ? advance(next, next.through, []) : next
}

// withNative applies a read of the whole stretch with native events, made when
// the operator asks to see them.
export function withNative(log: ConversationLog, read: History): ConversationLog {
  const { events, delta } = mergeEvents(log.events, read.events, log.delta)
  return advance({ ...log, events, delta, native: true }, Math.max(log.through, read.through), [])
}

// withoutNative drops the native events the page holds; nothing is fetched.
export function withoutNative(log: ConversationLog): ConversationLog {
  return { ...log, native: false, events: log.events.filter((event) => !isNative(event)) }
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

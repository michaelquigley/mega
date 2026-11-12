import { useSyncExternalStore } from 'react'

// one timer for the page: every elapsed label reads the same clock, which
// ticks once a second while anything shows one. while the page shows only
// last-known state the clock stands still at the moment it went stale, so no
// label counts on as if it were live.
let now = Date.now()
let frozenAt: number | null = null
let timer: number | undefined
const listeners = new Set<() => void>()

function notify() {
  for (const listener of listeners) {
    listener()
  }
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  if (timer === undefined) {
    // the clock may have stood idle: catch up now, and tell the first reader.
    now = Date.now()
    queueMicrotask(notify)
    timer = window.setInterval(() => {
      now = Date.now()
      notify()
    }, 1000)
  }
  return () => {
    listeners.delete(listener)
    if (listeners.size === 0) {
      window.clearInterval(timer)
      timer = undefined
    }
  }
}

const read = () => frozenAt ?? now

// useNow is the page's clock, in milliseconds.
export function useNow(): number {
  return useSyncExternalStore(subscribe, read)
}

const readWall = () => now

// useWallNow is the same clock, but it never stands still: it times what the
// page is waiting for, which matters most while the page is stale.
export function useWallNow(): number {
  return useSyncExternalStore(subscribe, readWall)
}

// freezeClock stops the clock while stale and restarts it when live again.
export function freezeClock(stale: boolean) {
  const next = stale ? (frozenAt ?? Date.now()) : null
  if (next !== frozenAt) {
    frozenAt = next
    now = Date.now()
    notify()
  }
}

// elapsedSince is the time since an RFC 3339 timestamp, never negative.
export function elapsedSince(at: string | undefined, clock: number): number | null {
  if (!at) {
    return null
  }
  const start = Date.parse(at)
  return Number.isNaN(start) ? null : Math.max(0, clock - start)
}

// shortElapsed is the label beside a name: 45s, 4m, 1h 12m.
export function shortElapsed(ms: number): string {
  const seconds = Math.floor(ms / 1000)
  if (seconds < 60) {
    return `${seconds}s`
  }
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) {
    return `${minutes}m`
  }
  return `${Math.floor(minutes / 60)}h ${minutes % 60}m`
}

// longElapsed is the activity line's: 8s, 1m 12s, 1h 12m.
export function longElapsed(ms: number): string {
  const seconds = Math.floor(ms / 1000)
  if (seconds < 60) {
    return `${seconds}s`
  }
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) {
    return `${minutes}m ${seconds % 60}s`
  }
  return `${Math.floor(minutes / 60)}h ${minutes % 60}m`
}

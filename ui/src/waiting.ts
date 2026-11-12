import { longElapsed } from './clock'

// Waiting is something the page is waiting on, so a slow load can say what
// it is: the request (or the stream) it asked for, since when, how many
// attempts it has made, and the last error, if any.
export interface Waiting {
  what: string
  since: number
  attempts: number
  error: string | null
}

// waitingLabel says it in one line: what, for how long, which attempt, and
// what went wrong last.
export function waitingLabel(waiting: Waiting, now: number): string {
  const parts = [waiting.what, longElapsed(Math.max(0, now - waiting.since))]
  if (waiting.attempts > 1) {
    parts.push(`attempt ${waiting.attempts}`)
  }
  if (waiting.error) {
    parts.push(`last error: ${waiting.error}`)
  }
  return parts.join(' · ')
}

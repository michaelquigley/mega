import { useEffect, useState } from 'react'
import type { StreamFrame } from './types'
import type { Waiting } from './waiting'

const HEARTBEAT_TIMEOUT_MS = 50_000
const RECONNECT_MIN_MS = 500
const RECONNECT_MAX_MS = 8_000

export type StreamState = 'connecting' | 'connected' | 'disconnected'

// Knowledge is what the page knows of the workspace: nothing yet, while it is
// connecting, once nothing has answered (unreachable), or once the service
// answered a read with an error (unreadable); the service's current state; or
// only the last state it read (stale).
export type Knowledge = 'connecting' | 'unreachable' | 'unreadable' | 'current' | 'stale'

// failing: the page has never read the workspace, and is retrying.
export const failing = (knowledge: Knowledge) => knowledge === 'unreachable' || knowledge === 'unreadable'

// the connection, as the panel's footer and the rail's dot say it.
export const connectionLabel = (knowledge: Knowledge, streamState: StreamState) =>
  knowledge === 'unreachable' ? 'Unreachable, retrying' : knowledge === 'unreadable' ? 'Read failed, retrying' : streamLabels[streamState]

// serviceAddress is where the page reaches the service: the address it was
// served from.
export const serviceAddress = () => window.location.host

export const streamLabels: Record<StreamState, string> = {
  connecting: 'Connecting',
  connected: 'Live',
  disconnected: 'Reconnecting',
}

// useStream keeps one WebSocket to the service. onOpen fires on every connection,
// because whatever happened while disconnected can only be recovered by re-reading
// confirmed state; the service never replays on its own. onClose fires when a
// connection drops, from which point what the page shows is only last known.
// waiting says, while there is no connection, what the page is waiting for:
// the stream's address, since when, how many attempts, and how the last one
// ended.
export function useStream(
  onOpen: () => void,
  onFrame: (frame: StreamFrame) => void,
  onClose: () => void,
): { state: StreamState; waiting: Waiting | null } {
  const [state, setState] = useState<StreamState>('connecting')
  const [waiting, setWaiting] = useState<Waiting | null>(null)

  useEffect(() => {
    let stopped = false
    let socket: WebSocket | null = null
    let reconnectTimer: number | undefined
    let watchdog: number | undefined
    let backoff = RECONNECT_MIN_MS
    let attempts = 0
    let since = Date.now()
    let lastError: string | null = null

    const armWatchdog = () => {
      window.clearTimeout(watchdog)
      watchdog = window.setTimeout(() => socket?.close(), HEARTBEAT_TIMEOUT_MS)
    }

    const connect = () => {
      const scheme = window.location.protocol === 'https:' ? 'wss' : 'ws'
      const address = `${scheme}://${window.location.host}/api/stream`
      const current = new WebSocket(address)
      socket = current
      attempts++
      setWaiting({ what: `the stream at ${address}`, since, attempts, error: lastError })
      current.onopen = () => {
        backoff = RECONNECT_MIN_MS
        attempts = 0
        lastError = null
        armWatchdog()
        setState('connected')
        setWaiting(null)
        onOpen()
      }
      current.onmessage = (message) => {
        armWatchdog()
        onFrame(JSON.parse(message.data as string) as StreamFrame)
      }
      current.onclose = (closed) => {
        window.clearTimeout(watchdog)
        if (stopped) {
          return
        }
        if (attempts === 0) {
          // a connection that was up has dropped: the wait starts now.
          since = Date.now()
        }
        lastError = `the connection closed (code ${closed.code}${closed.reason ? `, ${closed.reason}` : ''})`
        setWaiting({ what: `the stream at ${current.url}`, since, attempts: Math.max(attempts, 1), error: lastError })
        onClose()
        setState('disconnected')
        reconnectTimer = window.setTimeout(connect, backoff)
        backoff = Math.min(backoff * 2, RECONNECT_MAX_MS)
      }
    }

    connect()
    return () => {
      stopped = true
      window.clearTimeout(reconnectTimer)
      window.clearTimeout(watchdog)
      socket?.close()
    }
  }, [onOpen, onFrame, onClose])

  return { state, waiting }
}

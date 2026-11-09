import { useEffect, useState } from 'react'
import type { StreamFrame } from './types'

const HEARTBEAT_TIMEOUT_MS = 50_000
const RECONNECT_MIN_MS = 500
const RECONNECT_MAX_MS = 8_000

export type StreamState = 'connecting' | 'connected' | 'disconnected'

// useStream keeps one WebSocket to the service. onOpen fires on every connection,
// because whatever happened while disconnected can only be recovered by re-reading
// confirmed state; the service never replays on its own.
export function useStream(onOpen: () => void, onFrame: (frame: StreamFrame) => void): StreamState {
  const [state, setState] = useState<StreamState>('connecting')

  useEffect(() => {
    let stopped = false
    let socket: WebSocket | null = null
    let reconnectTimer: number | undefined
    let watchdog: number | undefined
    let backoff = RECONNECT_MIN_MS

    const armWatchdog = () => {
      window.clearTimeout(watchdog)
      watchdog = window.setTimeout(() => socket?.close(), HEARTBEAT_TIMEOUT_MS)
    }

    const connect = () => {
      const scheme = window.location.protocol === 'https:' ? 'wss' : 'ws'
      const current = new WebSocket(`${scheme}://${window.location.host}/api/stream`)
      socket = current
      current.onopen = () => {
        backoff = RECONNECT_MIN_MS
        armWatchdog()
        setState('connected')
        onOpen()
      }
      current.onmessage = (message) => {
        armWatchdog()
        onFrame(JSON.parse(message.data as string) as StreamFrame)
      }
      current.onclose = () => {
        window.clearTimeout(watchdog)
        if (stopped) {
          return
        }
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
  }, [onOpen, onFrame])

  return state
}

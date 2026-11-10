import { useState, type KeyboardEvent, type PointerEvent } from 'react'
import { clampWidth, readCollapsed, readWidth, saveCollapsed, saveWidth } from '../prefs'
import type { Selection } from '../selection'
import type { StreamState } from '../stream'
import type { Conversation, Project } from '../types'
import { Rail } from './Rail'
import { Sidebar } from './Sidebar'

interface SidePanelProps {
  projects: Project[]
  conversations: Conversation[]
  needsYou: string[]
  selection: Selection | null
  streamState: StreamState
  stale: boolean
  loaded: boolean
  onRegister: () => void
}

// SidePanel is the side panel at the width this browser remembers, resized by
// a handle on its right edge, or collapsed to the rail. width and collapse are
// preferences of this browser, not workspace state.
export function SidePanel(props: SidePanelProps) {
  const [width, setWidth] = useState(readWidth)
  const [collapsed, setCollapsed] = useState(readCollapsed)

  const collapse = (next: boolean) => {
    setCollapsed(next)
    saveCollapsed(next)
  }

  const onDragStart = (event: PointerEvent<HTMLDivElement>) => {
    event.preventDefault()
    const startX = event.clientX
    const startWidth = width
    const target = event.currentTarget
    target.setPointerCapture(event.pointerId)
    let latest = startWidth
    const move = (e: globalThis.PointerEvent) => {
      latest = clampWidth(startWidth + e.clientX - startX)
      setWidth(latest)
    }
    const up = () => {
      target.removeEventListener('pointermove', move)
      target.removeEventListener('pointerup', up)
      saveWidth(latest)
    }
    target.addEventListener('pointermove', move)
    target.addEventListener('pointerup', up)
  }
  const onDragKey = (event: KeyboardEvent<HTMLDivElement>) => {
    const step = event.key === 'ArrowRight' ? 16 : event.key === 'ArrowLeft' ? -16 : 0
    if (step) {
      event.preventDefault()
      const next = clampWidth(width + step)
      setWidth(next)
      saveWidth(next)
    }
  }

  if (collapsed) {
    return (
      <div className="side collapsed">
        <Rail {...props} onExpand={() => collapse(false)} />
      </div>
    )
  }
  return (
    <div className="side" style={{ width }}>
      <Sidebar {...props} onCollapse={() => collapse(true)} />
      <div
        className="side-handle"
        role="separator"
        aria-orientation="vertical"
        aria-label="Resize the side panel"
        title="Drag to resize the side panel"
        aria-valuenow={width}
        aria-valuemin={200}
        aria-valuemax={480}
        tabIndex={0}
        onPointerDown={onDragStart}
        onKeyDown={onDragKey}
      />
    </div>
  )
}

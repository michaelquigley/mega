import { useEffect, useState, type KeyboardEvent, type PointerEvent } from 'react'
import { clampWidth, readCollapsed, readWidth, saveCollapsed, saveWidth } from '../prefs'
import type { Selection } from '../selection'
import type { Knowledge, StreamState } from '../stream'
import type { About, Conversation, Project, ShutdownGate } from '../types'
import type { Waiting } from '../waiting'
import { AboutPanel } from './About'
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
  knowledge: Knowledge
  // what the page is waiting for while it has no connection or no workspace.
  waiting: Waiting | null
  onRegister: () => void
  // the shutdown gate, the page's own shutdown, and the gate's controls.
  gate: ShutdownGate | undefined
  shutDownAt: Date | null
  gateBusy: boolean
  onArm: () => void
  onCancelShutdown: () => void
  // what the service says of its build, and whether it now serves another
  // interface than this page's.
  about: About | null
  newer: boolean
}

// panelShortcut shows and hides the side panel, as its controls' titles say.
export const panelShortcut = "Ctrl+'"

// isPanelToggle is ctrl+', by physical key so it holds across layouts, from
// anywhere, the reply box included. it needs ctrl, so it never takes a
// keystroke that would type; alt chords are not used, since the operator's
// browser takes them before the page sees them.
function isPanelToggle(event: globalThis.KeyboardEvent): boolean {
  return event.ctrlKey && !event.altKey && !event.metaKey && event.code === 'Quote'
}

// SidePanel is the side panel at the width this browser remembers, resized by
// a handle on its right edge, or collapsed to the rail, by its controls or
// ctrl+'. width and collapse are preferences of this browser, not workspace
// state.
export function SidePanel(props: SidePanelProps) {
  const [width, setWidth] = useState(readWidth)
  const [collapsed, setCollapsed] = useState(readCollapsed)
  const [showingAbout, setShowingAbout] = useState(false)
  const aboutPanel = showingAbout && props.about && <AboutPanel about={props.about} newer={props.newer} onClose={() => setShowingAbout(false)} />

  const collapse = (next: boolean) => {
    setCollapsed(next)
    saveCollapsed(next)
  }

  useEffect(() => {
    const onKey = (event: globalThis.KeyboardEvent) => {
      if (!isPanelToggle(event)) {
        return
      }
      event.preventDefault()
      setCollapsed(!collapsed)
      saveCollapsed(!collapsed)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [collapsed])

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
        <Rail {...props} onExpand={() => collapse(false)} onAbout={() => setShowingAbout(true)} />
        {aboutPanel}
      </div>
    )
  }
  return (
    <div className="side" style={{ width }}>
      <Sidebar {...props} onCollapse={() => collapse(true)} onAbout={() => setShowingAbout(true)} />
      {aboutPanel}
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

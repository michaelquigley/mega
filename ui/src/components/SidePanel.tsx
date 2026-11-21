import { useEffect, useRef, useState, type KeyboardEvent, type PointerEvent } from 'react'
import { clampWidth, folds, readCollapsed, readWidth, saveCollapsed, saveWidth } from '../prefs'
import { collapseAll, expandAll, panelFocus, panelToggle } from '../keys'
import { dialogOpen } from '../modal'
import { focusPanel } from '../panel'
import type { Selection } from '../selection'
import type { Knowledge, StreamState } from '../stream'
import type { About, Conversation, HarnessStatus, Project, ShutdownGate } from '../types'
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
  // a project row's adopt a session and new conversation, opening the
  // project page's own dialogs, and what holds them, as it holds the page's.
  onLaunch: (projectId: string) => void
  onAdopt: (projectId: string) => void
  heldBy: string | null
  // the shutdown gate, the page's own shutdown, and the gate's controls.
  gate: ShutdownGate | undefined
  shutDownAt: Date | null
  // the page heard mega say its shutdown finished, before contact was lost.
  shutDownHeard: boolean
  gateBusy: boolean
  onArm: () => void
  onCancelShutdown: () => void
  // what the service says of its build, and whether it now serves another
  // interface than this page's.
  about: About | null
  newer: boolean
  // the harnesses as the workspace last said them, which follows an update
  // without the panel reading the service again.
  harnesses: HarnessStatus[] | undefined
}

// SidePanel is the side panel at the width this browser remembers, resized by
// a handle on its right edge, or collapsed to the rail, by its controls or
// its key (keys.ts), from anywhere, the reply box included. width and
// collapse are preferences of this browser, not workspace state.
export function SidePanel(props: SidePanelProps) {
  const [width, setWidth] = useState(readWidth)
  const [collapsed, setCollapsed] = useState(readCollapsed)
  const [showingAbout, setShowingAbout] = useState(false)
  const aboutPanel = showingAbout && props.about && <AboutPanel about={props.about} harnesses={props.harnesses} newer={props.newer} onClose={() => setShowingAbout(false)} />

  const collapse = (next: boolean) => {
    setCollapsed(next)
    saveCollapsed(next)
  }

  useEffect(() => {
    const onKey = (event: globalThis.KeyboardEvent) => {
      if (!panelToggle.matches(event)) {
        return
      }
      event.preventDefault()
      setCollapsed(!collapsed)
      saveCollapsed(!collapsed)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [collapsed])

  // the panel's focus key puts the focus on its projects, from anywhere but
  // a dialog, first opening the panel from the rail. the focus moves once
  // the panel is shown, which a count of presses asks for.
  const [focusWanted, setFocusWanted] = useState(0)
  useEffect(() => {
    const onKey = (event: globalThis.KeyboardEvent) => {
      if (!panelFocus.matches(event)) {
        return
      }
      event.preventDefault()
      if (dialogOpen()) {
        return
      }
      if (collapsed) {
        setCollapsed(false)
        saveCollapsed(false)
      }
      setFocusWanted((count) => count + 1)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [collapsed])
  useEffect(() => {
    if (focusWanted > 0) {
      focusPanel()
    }
  }, [focusWanted])

  // expand all and collapse all by their keys, from anywhere, the rail
  // included: a choice for every project, as the controls in the projects'
  // heading make.
  const projectsRef = useRef(props.projects)
  useEffect(() => {
    projectsRef.current = props.projects
  })
  useEffect(() => {
    const onKey = (event: globalThis.KeyboardEvent) => {
      const choice = expandAll.matches(event) ? 'unfolded' : collapseAll.matches(event) ? 'folded' : null
      if (!choice) {
        return
      }
      event.preventDefault()
      folds.setAll(projectsRef.current.map((project) => project.id), choice)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

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

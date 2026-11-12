import { useCallback, useEffect, useRef, useState } from 'react'
import { ApiError, armShutdown, cancelShutdown, getEvents, getWorkspace, readConversation, shutDown } from './api'
import { freezeClock } from './clock'
import { parseHash, select, type Selection } from './selection'
import { serviceAddress, useStream, type Knowledge } from './stream'
import { loadedThrough, merge, type ConversationLog } from './log'
import type { StreamFrame, Workspace } from './types'
import { AdoptDialog } from './components/AdoptDialog'
import { ConversationView } from './components/ConversationView'
import { LaunchDialog } from './components/LaunchDialog'
import { ProjectDetail } from './components/ProjectDetail'
import { RegisterProjectDialog } from './components/RegisterProjectDialog'
import { SidePanel } from './components/SidePanel'
import { ShutdownBanner } from './components/Gate'
import { pendingReason, shutDownReason } from './gate'
import { missingReason } from './checkout'
import { showTab, tabState } from './favicon'
import { productName } from './brand'

export function App() {
  const [workspace, setWorkspace] = useState<Workspace | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  // answered: the last failed workspace read was answered by the service with
  // an error, rather than reaching nothing.
  const [answered, setAnswered] = useState(false)
  // shutDownAt: when this page's shutdown was accepted. it stands until a read
  // finds the gate unarmed, which only a new service can say: the gate is
  // memory, and a restart is what clears it.
  const [shutDownAt, setShutDownAt] = useState<Date | null>(null)
  const [gateBusy, setGateBusy] = useState(false)
  const [gateError, setGateError] = useState<string | null>(null)
  const [selection, setSelection] = useState<Selection | null>(parseHash)
  const [registering, setRegistering] = useState(false)
  const [launching, setLaunching] = useState<string | null>(null)
  const [adopting, setAdopting] = useState<string | null>(null)
  const [log, setLog] = useState<ConversationLog | null>(null)

  const selectedConversationId = selection?.kind === 'conversation' ? selection.id : null
  const selectedRef = useRef(selectedConversationId)
  const logRef = useRef(log)
  useEffect(() => {
    selectedRef.current = selectedConversationId
    logRef.current = log
  })

  useEffect(() => {
    const onHash = () => setSelection(parseHash())
    window.addEventListener('hashchange', onHash)
    return () => window.removeEventListener('hashchange', onHash)
  }, [])

  // synced is true once the workspace has been re-read on the current
  // connection; until then runtime state on the page is only last known.
  const [synced, setSynced] = useState(false)

  // connection numbers each stream connection, so only a refresh sent on the
  // current one can mark the page up to date: one still out when its
  // connection dropped lands on a page that has moved on, and changes nothing.
  const connection = useRef(0)

  // refreshes are numbered and coalesced: at most one workspace fetch is in
  // flight, changes announced meanwhile get one follow-up fetch, and only the
  // newest response sent is applied. an older response can never land after a
  // newer one and bring back a request the service no longer lists as pending.
  const refreshing = useRef<{ current: Promise<boolean> | null; again: boolean; sent: number }>({
    current: null,
    again: false,
    sent: 0,
  })

  const refresh = useCallback((): Promise<boolean> => {
    const state = refreshing.current
    if (state.current) {
      state.again = true
      return state.current
    }
    const run = async () => {
      let ok = false
      do {
        state.again = false
        const number = ++state.sent
        const sentOn = connection.current
        try {
          const fetched = await getWorkspace()
          if (number === state.sent) {
            setWorkspace(fetched)
            setLoadError(null)
            if (fetched.shutdown && !fetched.shutdown.armed) {
              setShutDownAt(null)
            }
            ok = true
            if (sentOn === connection.current) {
              setSynced(true)
            }
          }
        } catch (error) {
          if (number === state.sent) {
            // what the page shows is now only last known, until a refresh on
            // the current connection succeeds.
            setLoadError(error instanceof Error ? error.message : String(error))
            setAnswered(error instanceof ApiError)
            setSynced(false)
            ok = false
          }
        }
      } while (state.again)
      state.current = null
      return ok
    }
    state.current = run()
    return state.current
  }, [])

  const loadEvents = useCallback(async (id: string, since: number) => {
    try {
      const { events } = await getEvents(id, since)
      setLog((current) => merge(current, id, events))
    } catch (error) {
      setLoadError(error instanceof Error ? error.message : String(error))
    }
  }, [])

  useEffect(() => {
    const id = selectedConversationId
    if (!id) {
      return
    }
    let cancelled = false
    getEvents(id, 0)
      .then(({ events }) => {
        if (!cancelled) {
          setLog((current) => merge(current, id, events))
        }
      })
      .catch((error: unknown) => {
        if (!cancelled) {
          setLoadError(error instanceof Error ? error.message : String(error))
        }
      })
    return () => {
      cancelled = true
    }
  }, [selectedConversationId])

  // every connection re-reads confirmed state; the service never replays on
  // its own, so whatever was missed comes from the workspace and the log.
  const onOpen = useCallback(() => {
    connection.current++
    setSynced(false)
    void refresh()
    const id = selectedRef.current
    if (id) {
      const current = logRef.current
      const last = current && current.id === id ? (current.events.at(-1)?.seq ?? 0) : 0
      void loadEvents(id, last)
    }
  }, [refresh, loadEvents])

  const onFrame = useCallback(
    (frame: StreamFrame) => {
      if (frame.workspace_changed) {
        void refresh()
      }
      if (!frame.conversation_id || frame.conversation_id !== selectedRef.current) {
        return
      }
      const id = frame.conversation_id
      if (frame.event) {
        const event = frame.event
        setLog((current) => merge(current, id, [event]))
      } else if (frame.delta) {
        const delta = frame.delta
        setLog((current) => (current && current.id === id ? { ...current, delta: current.delta + delta } : current))
      }
    },
    [refresh],
  )

  const onClose = useCallback(() => {
    connection.current++
    setSynced(false)
  }, [])
  const streamState = useStream(onOpen, onFrame, onClose)
  const stale = streamState !== 'connected' || !synced

  // a workspace read that fails while the stream is up is tried again, with
  // backoff, until one succeeds: nothing else would ask again until the
  // service announced a change, and a page that has never loaded would wait
  // on that forever.
  const [retryIn, setRetryIn] = useState(0)
  useEffect(() => {
    if (streamState !== 'connected' || synced || loadError === null) {
      return
    }
    const delay = Math.min(1000 * 2 ** retryIn, 8000)
    const timer = window.setTimeout(() => {
      void refresh().then((ok) => setRetryIn((n) => (ok ? 0 : n + 1)))
    }, delay)
    return () => window.clearTimeout(timer)
  }, [streamState, synced, loadError, retryIn, refresh])

  // what the page knows: nothing yet (connecting; unreachable once the stream
  // or a read has reached nothing; unreadable once the service answered a read
  // with an error), the service's current state, or only the last state it
  // read (stale). an empty workspace is shown only when a successful read
  // returned one.
  let knowledge: Knowledge
  if (workspace === null) {
    if (streamState === 'disconnected') {
      knowledge = 'unreachable'
    } else if (loadError !== null) {
      knowledge = answered ? 'unreadable' : 'unreachable'
    } else {
      knowledge = 'connecting'
    }
  } else {
    knowledge = stale ? 'stale' : 'current'
  }
  // elapsed labels stand still while the page shows only last-known state.
  useEffect(() => freezeClock(stale), [stale])

  // the shutdown gate. while armed, new work waits; at idle the page says so
  // in its title and with a notification, once, so nobody has to watch.
  const gate = workspace?.shutdown
  const heldBy = shutDownAt !== null ? shutDownReason : gate?.armed ? pendingReason : null
  const gateAction = async (action: () => Promise<void>, asking = false): Promise<boolean> => {
    if (asking && 'Notification' in window && Notification.permission === 'default') {
      // asked on the press that arms, the one gesture the page gets.
      void Notification.requestPermission().catch(() => undefined)
    }
    setGateError(null)
    setGateBusy(true)
    try {
      await action()
      return true
    } catch (failure) {
      setGateError(failure instanceof Error ? failure.message : String(failure))
      return false
    } finally {
      setGateBusy(false)
    }
  }
  // ready only on a current read: a page that has lost the service knows only
  // what it last read, and says so instead.
  const ready = !!gate?.armed && gate.idle && shutDownAt === null && !stale
  const wasReady = useRef<boolean | null>(null)
  // the tab: the needs-you mark and count while anything waits on the
  // operator, live only; the shutdown gate's title wins while it applies.
  const waitingCount = workspace?.needs_you.length ?? 0
  useEffect(() => {
    showTab(tabState(waitingCount, ready, stale || workspace === null))
  }, [waitingCount, ready, stale, workspace])
  useEffect(() => {
    if (workspace === null) {
      return
    }
    // a page that opens on an idle gate is already showing it; only the
    // change to idle is announced.
    if (wasReady.current === false && ready && 'Notification' in window && Notification.permission === 'granted') {
      try {
        new Notification(productName, { body: 'Every agent is idle. Shut down from the banner when you are ready.', tag: 'mega-shutdown' })
      } catch {
        // a browser that refuses notifications still has the title and banner.
      }
    }
    wasReady.current = ready
  }, [ready, workspace])

  const projects = workspace?.projects ?? []
  const conversations = workspace?.conversations ?? []
  const needsYou = workspace?.needs_you ?? []

  // next-waiting walks the needs-you list from the selected conversation; it
  // only moves the selection, so a reply in progress keeps its place. ctrl+/
  // works anywhere; a bare n works only while focus is outside anything that
  // takes text, so typing an n never moves the selection.
  const needsYouRef = useRef(needsYou)
  useEffect(() => {
    needsYouRef.current = needsYou
  })
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (!isNextWaiting(event)) {
        return
      }
      event.preventDefault()
      const list = needsYouRef.current
      if (list.length === 0) {
        return
      }
      const at = list.indexOf(selectedRef.current ?? '')
      select({ kind: 'conversation', id: list[(at + 1) % list.length] })
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  // opening a conversation acknowledges what is on screen: once its history is
  // loaded and current, the reading position moves to its last event. this
  // never touches a pending request, which stays until it is answered.
  const [visible, setVisible] = useState(() => document.visibilityState === 'visible')
  useEffect(() => {
    const onVisibility = () => setVisible(document.visibilityState === 'visible')
    document.addEventListener('visibilitychange', onVisibility)
    return () => document.removeEventListener('visibilitychange', onVisibility)
  }, [])
  const posted = useRef<{ id: string; seq: number } | null>(null)
  const openConversation = conversations.find((c) => c.id === selectedConversationId)
  const openReadSeq = openConversation?.read_seq ?? 0
  // only the unbroken run of events from the first one counts as on screen: a
  // live event that arrived while history was still loading proves nothing
  // about the events before it.
  const loggedSeq = log && log.id === selectedConversationId ? loadedThrough(log.events) : 0
  useEffect(() => {
    const id = selectedConversationId
    if (!id || !visible || stale || loggedSeq <= openReadSeq) {
      return
    }
    if (posted.current && posted.current.id === id && posted.current.seq >= loggedSeq) {
      return
    }
    posted.current = { id, seq: loggedSeq }
    readConversation(id, loggedSeq).catch(() => {
      posted.current = null
    })
  }, [selectedConversationId, loggedSeq, openReadSeq, visible, stale])
  // a selected conversation that leaves the workspace, removed here or from
  // another browser, hands the selection to its project. the project is taken
  // from the last workspace that listed the conversation.
  const lastProjectOf = useRef<{ id: string; project: string } | null>(null)
  useEffect(() => {
    const id = selectedConversationId
    if (!id || !workspace) {
      return
    }
    const listed = workspace.conversations.find((c) => c.id === id)
    if (listed) {
      lastProjectOf.current = { id, project: listed.project_id }
      return
    }
    const last = lastProjectOf.current
    if (last && last.id === id) {
      lastProjectOf.current = null
      if (workspace.projects.some((p) => p.id === last.project)) {
        select({ kind: 'project', id: last.project })
      }
    }
  }, [selectedConversationId, workspace])
  const selectedProject = selection?.kind === 'project' ? projects.find((p) => p.id === selection.id) : undefined
  const selectedConversation = selectedConversationId
    ? conversations.find((c) => c.id === selectedConversationId)
    : undefined
  const conversationProject = selectedConversation
    ? projects.find((p) => p.id === selectedConversation.project_id)
    : undefined

  let main
  if (selectedConversation && conversationProject) {
    main = (
      <ConversationView
        key={selectedConversation.id}
        conversation={selectedConversation}
        project={conversationProject}
        log={log && log.id === selectedConversation.id ? log : null}
        stale={stale}
        onRemoved={() => select({ kind: 'project', id: selectedConversation.project_id })}
        heldBy={heldBy ?? missingReason(conversationProject)}
      />
    )
  } else if (selectedProject) {
    main = (
      <ProjectDetail
        key={`${selectedProject.id}:${selectedProject.name}`}
        project={selectedProject}
        conversations={conversations.filter((c) => c.project_id === selectedProject.id)}
        stale={stale}
        onLaunch={() => setLaunching(selectedProject.id)}
        onAdopt={() => setAdopting(selectedProject.id)}
        heldBy={heldBy ?? missingReason(selectedProject)}
      />
    )
  } else if (workspace === null) {
    main = <Unknown knowledge={knowledge} error={loadError} />
  } else {
    main = (
      <p className="placeholder">
        {projects.length === 0 ? 'Register a project from an existing checkout to begin.' : 'Select a project or a conversation.'}
      </p>
    )
  }

  return (
    <div className="app">
      <SidePanel
        projects={projects}
        conversations={conversations}
        needsYou={needsYou}
        selection={selection}
        streamState={streamState}
        stale={stale}
        loaded={workspace !== null}
        knowledge={knowledge}
        onRegister={() => setRegistering(true)}
        gate={gate}
        shutDownAt={shutDownAt}
        gateBusy={gateBusy}
        onArm={() => void gateAction(armShutdown, true)}
        onCancelShutdown={() => void gateAction(cancelShutdown)}
      />
      <main className="main">
        <ShutdownBanner
          gate={gate}
          shutDownAt={shutDownAt}
          connected={streamState === 'connected'}
          stale={stale}
          busy={gateBusy}
          error={gateError}
          onShutDown={() =>
            void gateAction(shutDown).then((ok) => {
              if (ok) {
                setShutDownAt(new Date())
              }
            })
          }
          onCancel={() => void gateAction(cancelShutdown)}
        />
        {loadError && workspace !== null && shutDownAt === null && <p className="error">The workspace could not be read: {loadError}</p>}
        {main}
      </main>
      {registering && (
        <RegisterProjectDialog
          onClose={() => setRegistering(false)}
          onRegistered={(project) => {
            setRegistering(false)
            select({ kind: 'project', id: project.id })
          }}
        />
      )}
      {adopting && (
        <AdoptDialog
          projectId={adopting}
          onClose={() => setAdopting(null)}
          onAdopted={(conversation) => {
            setAdopting(null)
            select({ kind: 'conversation', id: conversation.id })
          }}
        />
      )}
      {launching && (
        <LaunchDialog
          projectId={launching}
          inheritedMode={projects.find((p) => p.id === launching)?.inherited_permission_mode ?? ''}
          unavailable={Object.fromEntries((workspace?.preflight?.missing ?? []).map((m) => [m.harness, m.reason]))}
          path={workspace?.preflight?.path ?? ''}
          onClose={() => setLaunching(null)}
          onLaunched={(conversation) => {
            setLaunching(null)
            select({ kind: 'conversation', id: conversation.id })
          }}
        />
      )}
    </div>
  )
}

// the next-waiting shortcut: ctrl+/ (by physical key, so it holds across
// layouts), or a bare n while focus is not in a text field.
function isNextWaiting(event: KeyboardEvent): boolean {
  if (event.ctrlKey && event.code === 'Slash') {
    return true
  }
  if (event.key !== 'n' || event.ctrlKey || event.altKey || event.metaKey || event.isComposing) {
    return false
  }
  return !takesText(event.target)
}

function takesText(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) {
    return false
  }
  return (
    target.isContentEditable ||
    target instanceof HTMLInputElement ||
    target instanceof HTMLTextAreaElement ||
    target instanceof HTMLSelectElement
  )
}

// Unknown is the main column before the page has read the workspace once: it
// says the page is connecting, that the service is unreachable, or that the
// service answered but could not read the workspace, and that it is still
// trying, and where. it never says there is nothing.
function Unknown({ knowledge, error }: { knowledge: Knowledge; error: string | null }) {
  if (knowledge === 'unreadable') {
    return (
      <section className="unknown unknown-unreadable" role="status">
        <h1>The service could not read the workspace</h1>
        <p>
          {`The service at ${serviceAddress()} answered, but could not read the workspace. mega keeps trying, and your projects and conversations appear as soon as a read succeeds.`}
        </p>
        {error && <p className="muted">Error: {error}</p>}
      </section>
    )
  }
  const unreachable = knowledge === 'unreachable'
  return (
    <section className={`unknown unknown-${knowledge}`} role="status">
      <h1>{unreachable ? 'The service is unreachable' : 'Connecting'}</h1>
      <p>
        {unreachable
          ? `mega cannot reach the service at ${serviceAddress()}. It keeps trying, and your projects and conversations appear as soon as it answers.`
          : `Reaching the service at ${serviceAddress()} to read your projects and conversations.`}
      </p>
      {unreachable && error && <p className="muted">Last error: {error}</p>}
    </section>
  )
}


import { useCallback, useEffect, useRef, useState } from 'react'
import { getEvents, getWorkspace } from './api'
import { parseHash, select, type Selection } from './selection'
import { useStream } from './stream'
import { merge, type ConversationLog } from './log'
import type { StreamFrame, Workspace } from './types'
import { AdoptDialog } from './components/AdoptDialog'
import { ConversationView } from './components/ConversationView'
import { LaunchDialog } from './components/LaunchDialog'
import { ProjectDetail } from './components/ProjectDetail'
import { RegisterProjectDialog } from './components/RegisterProjectDialog'
import { Sidebar } from './components/Sidebar'

export function App() {
  const [workspace, setWorkspace] = useState<Workspace | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
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

  const projects = workspace?.projects ?? []
  const conversations = workspace?.conversations ?? []
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
      />
    )
  } else {
    main = (
      <p className="placeholder">
        {workspace && projects.length === 0
          ? 'Register a project from an existing checkout to begin.'
          : 'Select a project or a conversation.'}
      </p>
    )
  }

  return (
    <div className="app">
      <Sidebar
        projects={projects}
        conversations={conversations}
        selection={selection}
        streamState={streamState}
        stale={stale}
        onRegister={() => setRegistering(true)}
      />
      <main className="main">
        {loadError && <p className="error">The workspace could not be read: {loadError}</p>}
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

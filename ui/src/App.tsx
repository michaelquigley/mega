import { useCallback, useState } from 'react'
import { getWorkspace } from './api'
import { useStream } from './stream'
import type { StreamFrame, Workspace } from './types'
import { ProjectDetail } from './components/ProjectDetail'
import { RegisterProjectDialog } from './components/RegisterProjectDialog'
import { Sidebar } from './components/Sidebar'

export function App() {
  const [workspace, setWorkspace] = useState<Workspace | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [registering, setRegistering] = useState(false)

  const refresh = useCallback(async () => {
    try {
      setWorkspace(await getWorkspace())
      setLoadError(null)
    } catch (error) {
      setLoadError(error instanceof Error ? error.message : String(error))
    }
  }, [])

  const onFrame = useCallback(
    (frame: StreamFrame) => {
      if (frame.workspace_changed) {
        void refresh()
      }
    },
    [refresh],
  )

  const streamState = useStream(refresh, onFrame)
  const selected = workspace?.projects.find((project) => project.id === selectedId) ?? null

  return (
    <div className="app">
      <Sidebar
        projects={workspace?.projects ?? []}
        selectedId={selected?.id ?? null}
        streamState={streamState}
        onSelect={setSelectedId}
        onRegister={() => setRegistering(true)}
      />
      <main className="main">
        {loadError && <p className="error">The workspace could not be read: {loadError}</p>}
        {selected ? (
          <ProjectDetail key={`${selected.id}:${selected.name}`} project={selected} />
        ) : (
          <p className="placeholder">
            {workspace && workspace.projects.length === 0
              ? 'Register a project from an existing checkout to begin.'
              : 'Select a project.'}
          </p>
        )}
      </main>
      {registering && (
        <RegisterProjectDialog
          onClose={() => setRegistering(false)}
          onRegistered={(project) => {
            setRegistering(false)
            setSelectedId(project.id)
          }}
        />
      )}
    </div>
  )
}

import type { StreamState } from '../stream'
import type { Project } from '../types'

interface SidebarProps {
  projects: Project[]
  selectedId: string | null
  streamState: StreamState
  onSelect: (id: string) => void
  onRegister: () => void
}

const streamLabels: Record<StreamState, string> = {
  connecting: 'Connecting',
  connected: 'Live',
  disconnected: 'Reconnecting',
}

export function Sidebar({ projects, selectedId, streamState, onSelect, onRegister }: SidebarProps) {
  return (
    <nav className="sidebar">
      <section>
        <h2>Needs you</h2>
        <p className="muted">Nothing waiting.</p>
      </section>
      <section className="projects">
        <div className="section-head">
          <h2>Projects</h2>
          <button type="button" onClick={onRegister}>
            Register
          </button>
        </div>
        {projects.length === 0 && <p className="muted">No projects yet.</p>}
        <ul>
          {projects.map((project) => (
            <li key={project.id}>
              <button
                type="button"
                className={project.id === selectedId ? 'tree-item selected' : 'tree-item'}
                aria-current={project.id === selectedId ? 'true' : undefined}
                onClick={() => onSelect(project.id)}
              >
                {project.name}
              </button>
            </li>
          ))}
        </ul>
      </section>
      <footer className={`stream stream-${streamState}`}>{streamLabels[streamState]}</footer>
    </nav>
  )
}

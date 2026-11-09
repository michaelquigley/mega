import { select, type Selection } from '../selection'
import type { StreamState } from '../stream'
import type { Conversation, Project } from '../types'
import { RuntimeMark } from './RuntimeMark'

interface SidebarProps {
  projects: Project[]
  conversations: Conversation[]
  selection: Selection | null
  streamState: StreamState
  stale: boolean
  onRegister: () => void
}

const streamLabels: Record<StreamState, string> = {
  connecting: 'Connecting',
  connected: 'Live',
  disconnected: 'Reconnecting',
}

export function Sidebar({ projects, conversations, selection, streamState, stale, onRegister }: SidebarProps) {
  const isSelected = (kind: Selection['kind'], id: string) => selection?.kind === kind && selection.id === id

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
                className={isSelected('project', project.id) ? 'tree-item selected' : 'tree-item'}
                aria-current={isSelected('project', project.id) ? 'true' : undefined}
                onClick={() => select({ kind: 'project', id: project.id })}
              >
                {project.name}
              </button>
              <ul className="tree-children">
                {conversations
                  .filter((conversation) => conversation.project_id === project.id)
                  .map((conversation) => (
                    <li key={conversation.id}>
                      <button
                        type="button"
                        className={isSelected('conversation', conversation.id) ? 'tree-item selected' : 'tree-item'}
                        aria-current={isSelected('conversation', conversation.id) ? 'true' : undefined}
                        onClick={() => select({ kind: 'conversation', id: conversation.id })}
                      >
                        <RuntimeMark conversation={conversation} stale={stale} />
                        <span className="tree-label">{conversation.name}</span>
                        {conversation.label && <span className="tag">{conversation.label}</span>}
                      </button>
                    </li>
                  ))}
              </ul>
            </li>
          ))}
        </ul>
      </section>
      <footer className={`stream stream-${streamState}`}>{streamLabels[streamState]}</footer>
    </nav>
  )
}

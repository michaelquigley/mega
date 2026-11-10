import { select, type Selection } from '../selection'
import type { StreamState } from '../stream'
import type { Conversation, Project } from '../types'
import { AttentionMark, Elapsed, ProjectMark } from './AttentionMark'

interface SidebarProps {
  projects: Project[]
  conversations: Conversation[]
  needsYou: string[]
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

export function Sidebar({ projects, conversations, needsYou, selection, streamState, stale, onRegister }: SidebarProps) {
  const isSelected = (kind: Selection['kind'], id: string) => selection?.kind === kind && selection.id === id
  const byId = new Map(conversations.map((conversation) => [conversation.id, conversation]))
  const projectNames = new Map(projects.map((project) => [project.id, project.name]))
  const waiting = needsYou.map((id) => byId.get(id)).filter((c): c is Conversation => c !== undefined)

  return (
    <nav className="sidebar">
      <section className="needs-you">
        <div className="section-head">
          <h2>Needs you</h2>
          <span className="muted shortcut" title="Next waiting conversation: Ctrl+/ anywhere, or n outside a text field">
            Ctrl+/ or n
          </span>
        </div>
        {waiting.length === 0 ? (
          <p className="muted">Nothing waiting.</p>
        ) : (
          <ul>
            {waiting.map((conversation) => (
              <li key={conversation.id}>
                <button
                  type="button"
                  className={isSelected('conversation', conversation.id) ? 'tree-item waiting selected' : 'tree-item waiting'}
                  aria-current={isSelected('conversation', conversation.id) ? 'true' : undefined}
                  onClick={() => select({ kind: 'conversation', id: conversation.id })}
                >
                  <AttentionMark conversation={conversation} stale={stale} />
                  <span className="tree-label">{conversation.name}</span>
                  <Elapsed conversation={conversation} />
                  <span className="muted waiting-project">{projectNames.get(conversation.project_id)}</span>
                </button>
              </li>
            ))}
          </ul>
        )}
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
          {projects.map((project) => {
            const inProject = conversations.filter((conversation) => conversation.project_id === project.id)
            return (
              <li key={project.id}>
                <button
                  type="button"
                  className={isSelected('project', project.id) ? 'tree-item project-item selected' : 'tree-item project-item'}
                  aria-current={isSelected('project', project.id) ? 'true' : undefined}
                  onClick={() => select({ kind: 'project', id: project.id })}
                >
                  <ProjectMark conversations={inProject} stale={stale} />
                  <span className="tree-label">{project.name}</span>
                </button>
                <ul className="tree-children">
                  {inProject.map((conversation) => (
                    <li key={conversation.id}>
                      <button
                        type="button"
                        className={isSelected('conversation', conversation.id) ? 'tree-item selected' : 'tree-item'}
                        aria-current={isSelected('conversation', conversation.id) ? 'true' : undefined}
                        onClick={() => select({ kind: 'conversation', id: conversation.id })}
                      >
                        <AttentionMark conversation={conversation} stale={stale} />
                        <span className="tree-label">{conversation.name}</span>
                        <Elapsed conversation={conversation} />
                        {conversation.label && <span className="tag">{conversation.label}</span>}
                      </button>
                    </li>
                  ))}
                </ul>
              </li>
            )
          })}
        </ul>
      </section>
      <footer className={`stream stream-${streamState}`}>{streamLabels[streamState]}</footer>
    </nav>
  )
}

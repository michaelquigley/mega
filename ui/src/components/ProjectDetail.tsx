import { useState, type FormEvent } from 'react'
import { select } from '../selection'
import { removeProject, renameProject } from '../api'
import type { Conversation, Project } from '../types'
import { RuntimeMark } from './RuntimeMark'

interface ProjectDetailProps {
  project: Project
  conversations: Conversation[]
  stale: boolean
  onLaunch: () => void
}

export function ProjectDetail({ project, conversations, stale, onLaunch }: ProjectDetailProps) {
  const [name, setName] = useState(project.name)
  const [error, setError] = useState<string | null>(null)
  const [confirming, setConfirming] = useState(false)

  const attempt = async (action: () => Promise<unknown>) => {
    setError(null)
    try {
      await action()
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure))
    }
  }

  const onRename = (event: FormEvent) => {
    event.preventDefault()
    void attempt(() => renameProject(project.id, name))
  }

  return (
    <article className="detail">
      <header>
        <h1>{project.name}</h1>
        <code>{project.checkout}</code>
      </header>
      {project.untrusted && (
        <p className="notice" role="note">
          Claude has not been trusted in this checkout, so it ignores the project&apos;s permission
          rules. Run <code>claude</code> here once and accept the trust dialog; Roundhouse will not
          accept it for you.
        </p>
      )}
      {error && <p className="error">{error}</p>}
      <section>
        <div className="section-head">
          <h2>Conversations</h2>
          <button type="button" onClick={onLaunch}>
            New conversation
          </button>
        </div>
        {conversations.length === 0 ? (
          <p className="muted">No conversations yet.</p>
        ) : (
          <ul className="conversation-list">
            {conversations.map((conversation) => (
              <li key={conversation.id}>
                <button
                  type="button"
                  className="tree-item"
                  onClick={() => select({ kind: 'conversation', id: conversation.id })}
                >
                  <RuntimeMark conversation={conversation} stale={stale} />
                  <span className="tree-label">{conversation.name}</span>
                  <span className="muted">{conversation.harness}</span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>
      <form className="inline-form" onSubmit={onRename}>
        <label htmlFor="project-name">Name</label>
        <input id="project-name" value={name} onChange={(event) => setName(event.target.value)} />
        <button type="submit" disabled={name.trim() === '' || name.trim() === project.name}>
          Rename
        </button>
      </form>
      <section className="danger">
        {confirming ? (
          <>
            <p>Remove this project from the workspace? The checkout and native histories stay untouched.</p>
            <button type="button" onClick={() => void attempt(() => removeProject(project.id))}>
              Remove
            </button>{' '}
            <button type="button" onClick={() => setConfirming(false)}>
              Cancel
            </button>
          </>
        ) : (
          <button type="button" onClick={() => setConfirming(true)}>
            Remove from workspace
          </button>
        )}
      </section>
    </article>
  )
}

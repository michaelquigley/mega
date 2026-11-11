import { useState, type FormEvent } from 'react'
import { select } from '../selection'
import { removeProject, renameProject } from '../api'
import type { Conversation, Project } from '../types'
import { AttentionMark } from './AttentionMark'
import { IconButton } from './Icon'

interface ProjectDetailProps {
  project: Project
  conversations: Conversation[]
  stale: boolean
  onLaunch: () => void
  onAdopt: () => void
  // why new work waits, or null: launches and adoptions wait, and say so.
  heldBy: string | null
}

export function ProjectDetail({ project, conversations, stale, onLaunch, onAdopt, heldBy }: ProjectDetailProps) {
  const [name, setName] = useState(project.name)
  const [error, setError] = useState<string | null>(null)
  const [confirming, setConfirming] = useState(false)
  // conversations whose runtime is in any state other than none; the service
  // refuses the removal while any remains.
  const unsettled = conversations.filter((conversation) => conversation.state !== 'none')

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
          <span className="section-actions">
            <IconButton icon="download" label={heldBy ??  'Adopt a session'} disabled={heldBy !== null} onClick={onAdopt} />
            <IconButton icon="add_comment" label={heldBy ??  'New conversation'} disabled={heldBy !== null} onClick={onLaunch} />
          </span>
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
                  <AttentionMark conversation={conversation} stale={stale} />
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
        <IconButton type="submit" icon="edit" label="Rename the project" word="Rename" disabled={name.trim() === '' || name.trim() === project.name} />
      </form>
      <section className="danger">
        {confirming ? (
          <>
            <p>
              Roundhouse forgets this project and its conversations: its own copy of their history and their
              reading positions are deleted. The checkout and the native sessions stay untouched, and the sessions
              can be adopted again if the checkout is registered again.
            </p>
            {unsettled.length > 0 && (
              <p className="muted">
                Release or resume first: {unsettled.map((conversation) => conversation.name).join(', ')}.
              </p>
            )}
            <span className="actions-row">
              <IconButton
                icon="delete"
                label="Remove the project from the workspace"
                word="Remove"
                disabled={unsettled.length > 0}
                onClick={() => void attempt(() => removeProject(project.id))}
              />
              <IconButton icon="close" label="Cancel" onClick={() => setConfirming(false)} />
            </span>
          </>
        ) : (
          <IconButton icon="delete" label="Remove from workspace" word="Remove from workspace" onClick={() => setConfirming(true)} />
        )}
      </section>
    </article>
  )
}

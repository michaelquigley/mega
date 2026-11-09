import { useState, type FormEvent } from 'react'
import { removeProject, renameProject } from '../api'
import type { Project } from '../types'

export function ProjectDetail({ project }: { project: Project }) {
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

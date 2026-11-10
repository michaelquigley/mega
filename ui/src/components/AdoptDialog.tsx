import { useEffect, useRef, useState, type FormEvent } from 'react'
import { adoptSession, discoverSessions, type DiscoveredSession } from '../api'
import { harnesses } from '../harnesses'
import type { Conversation } from '../types'
import { IconButton } from './Icon'

interface AdoptDialogProps {
  projectId: string
  onClose: () => void
  onAdopted: (conversation: Conversation) => void
}

// AdoptDialog lists the native sessions stored for the checkout that no
// conversation holds, and binds the one the operator picks. nothing is chosen
// for them, and no runtime starts until the first prompt.
export function AdoptDialog({ projectId, onClose, onAdopted }: AdoptDialogProps) {
  const dialogRef = useRef<HTMLDialogElement>(null)
  const [harness, setHarness] = useState('claude')
  const [sessions, setSessions] = useState<DiscoveredSession[] | null>(null)
  const [chosen, setChosen] = useState<string | null>(null)
  const [name, setName] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    dialogRef.current?.showModal()
  }, [])

  useEffect(() => {
    let cancelled = false
    discoverSessions(projectId, harness)
      .then(({ sessions: found }) => {
        if (!cancelled) {
          setSessions(found)
        }
      })
      .catch((failure: unknown) => {
        if (!cancelled) {
          setError(failure instanceof Error ? failure.message : String(failure))
        }
      })
    return () => {
      cancelled = true
    }
  }, [projectId, harness])

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault()
    if (!chosen) {
      return
    }
    setSubmitting(true)
    setError(null)
    try {
      onAdopted(await adoptSession(projectId, harness, chosen, name))
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure))
      setSubmitting(false)
    }
  }

  return (
    <dialog ref={dialogRef} onClose={onClose}>
      <form onSubmit={(event) => void onSubmit(event)}>
        <div className="dialog-head">
          <h2>Adopt a session</h2>
          <IconButton icon="close" label="Cancel" onClick={onClose} />
        </div>
        <label>
          Harness
          <select
            value={harness}
            onChange={(event) => {
              setHarness(event.target.value)
              setSessions(null)
              setChosen(null)
              setError(null)
            }}
          >
            {harnesses.map((h) => (
              <option key={h.value} value={h.value}>
                {h.label}
              </option>
            ))}
          </select>
        </label>
        {sessions === null && !error && <p className="muted">Looking for sessions in this checkout…</p>}
        {sessions?.length === 0 && <p className="muted">No sessions in this checkout that a conversation does not already hold.</p>}
        {sessions && sessions.length > 0 && (
          <fieldset className="sessions">
            <legend>Sessions in this checkout</legend>
            {sessions.map((session) => (
              <label key={session.native_id} className="option">
                <input
                  type="radio"
                  name="session"
                  checked={chosen === session.native_id}
                  onChange={() => setChosen(session.native_id)}
                />
                <span>
                  <span className="preview">{session.preview || <em>no first prompt</em>}</span>
                  <span className="muted session-meta">
                    {' '}
                    {session.updated_at ? new Date(session.updated_at).toLocaleString() : ''} · <code>{session.native_id}</code>
                  </span>
                </span>
              </label>
            ))}
          </fieldset>
        )}
        <label>
          Name
          <input
            placeholder="Defaults to the first prompt"
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        </label>
        <p className="muted">
          Close any terminal that has this session open before you prompt it here; an idle terminal
          still holds the session.
        </p>
        {error && <p className="error">{error}</p>}
        <div className="actions">
          <IconButton type="submit" icon="download" label="Adopt" disabled={submitting || !chosen} />
        </div>
      </form>
    </dialog>
  )
}

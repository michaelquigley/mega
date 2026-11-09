import { useEffect, useRef, useState, type FormEvent } from 'react'
import { startConversation } from '../api'
import { harnesses } from '../harnesses'
import type { Conversation } from '../types'

interface LaunchDialogProps {
  projectId: string
  onClose: () => void
  onLaunched: (conversation: Conversation) => void
}

export function LaunchDialog({ projectId, onClose, onLaunched }: LaunchDialogProps) {
  const dialogRef = useRef<HTMLDialogElement>(null)
  const [harness, setHarness] = useState('claude')
  const [name, setName] = useState('')
  const [prompt, setPrompt] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    dialogRef.current?.showModal()
  }, [])

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault()
    setSubmitting(true)
    setError(null)
    try {
      onLaunched(await startConversation(projectId, harness, name, prompt))
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure))
      setSubmitting(false)
    }
  }

  return (
    <dialog ref={dialogRef} onClose={onClose}>
      <form onSubmit={(event) => void onSubmit(event)}>
        <h2>New conversation</h2>
        <label>
          Harness
          <select value={harness} onChange={(event) => setHarness(event.target.value)}>
            {harnesses.map((h) => (
              <option key={h.value} value={h.value}>
                {h.label}
              </option>
            ))}
          </select>
        </label>
        <label>
          Opening prompt
          <textarea
            autoFocus
            required
            rows={6}
            value={prompt}
            onChange={(event) => setPrompt(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) {
                event.currentTarget.form?.requestSubmit()
              }
            }}
          />
        </label>
        <label>
          Name
          <input
            placeholder="Defaults to the prompt's first line"
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        </label>
        <p className="muted">
          Native settings are inherited: approvals reach you here, and the checkout&apos;s own
          instructions, hooks and permission rules apply as in a terminal.
        </p>
        {error && <p className="error">{error}</p>}
        <div className="actions">
          <button type="button" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" disabled={submitting || prompt.trim() === ''}>
            Launch
          </button>
        </div>
      </form>
    </dialog>
  )
}

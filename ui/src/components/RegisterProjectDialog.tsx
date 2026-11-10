import { useEffect, useRef, useState, type FormEvent } from 'react'
import { createProject } from '../api'
import type { Project } from '../types'
import { IconButton } from './Icon'

interface RegisterProjectDialogProps {
  onClose: () => void
  onRegistered: (project: Project) => void
}

export function RegisterProjectDialog({ onClose, onRegistered }: RegisterProjectDialogProps) {
  const dialogRef = useRef<HTMLDialogElement>(null)
  const [name, setName] = useState('')
  const [checkout, setCheckout] = useState('')
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
      onRegistered(await createProject(name, checkout))
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure))
      setSubmitting(false)
    }
  }

  return (
    <dialog ref={dialogRef} onClose={onClose}>
      <form onSubmit={(event) => void onSubmit(event)}>
        <div className="dialog-head">
          <h2>Register a project</h2>
          <IconButton icon="close" label="Cancel" onClick={onClose} />
        </div>
        <label>
          Checkout
          <input
            autoFocus
            required
            placeholder="/absolute/path/to/checkout"
            value={checkout}
            onChange={(event) => setCheckout(event.target.value)}
          />
        </label>
        <label>
          Name
          <input
            placeholder="Defaults to the directory name"
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        </label>
        {error && <p className="error">{error}</p>}
        <div className="actions">
          <IconButton type="submit" icon="create_new_folder" label="Register" disabled={submitting} />
        </div>
      </form>
    </dialog>
  )
}

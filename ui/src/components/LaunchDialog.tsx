import { useEffect, useRef, useState, type FormEvent } from 'react'
import { startConversation } from '../api'
import { harnesses } from '../harnesses'
import { useModels } from '../models'
import { permissionModes } from '../permissions'
import type { Conversation } from '../types'
import { ModelPicker } from './ModelPicker'

interface LaunchDialogProps {
  projectId: string
  // the claude mode the operator's settings name for the checkout, or empty.
  inheritedMode: string
  onClose: () => void
  onLaunched: (conversation: Conversation) => void
}

// a claude conversation starts in the mode the operator picks. roundhouse's
// own default is the manual mode, unless the operator's settings name one, in
// which case that is inherited by passing none (the empty value). the model
// and effort start at inherit, which passes none, and the lists come from the
// harness; what inherit means is read afresh each time the dialog opens.
export function LaunchDialog({ projectId, inheritedMode, onClose, onLaunched }: LaunchDialogProps) {
  const dialogRef = useRef<HTMLDialogElement>(null)
  const [harness, setHarness] = useState('claude')
  const [permissionMode, setPermissionMode] = useState(inheritedMode ? '' : 'default')
  const [choice, setChoice] = useState({ model: '', effort: '' })
  const { models, error: modelsError } = useModels(projectId, harness, true)
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
      onLaunched(
        await startConversation(projectId, harness, name, prompt, {
          permissionMode: harness === 'claude' ? permissionMode : '',
          ...choice,
        }),
      )
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
          <select
            value={harness}
            onChange={(event) => {
              setHarness(event.target.value)
              setChoice({ model: '', effort: '' })
            }}
          >
            {harnesses.map((h) => (
              <option key={h.value} value={h.value}>
                {h.label}
              </option>
            ))}
          </select>
        </label>
        {harness === 'claude' && (
          <label>
            Permission mode
            <select value={permissionMode} onChange={(event) => setPermissionMode(event.target.value)}>
              {inheritedMode && <option value="">Inherited from your settings: {inheritedMode}</option>}
              {permissionModes.map((mode) => (
                <option key={mode.value} value={mode.value}>
                  {mode.label}
                </option>
              ))}
            </select>
          </label>
        )}
        <ModelPicker
          harness={harness}
          models={models}
          model={choice.model}
          effort={choice.effort}
          inheritModel
          inheritEffort
          disabled={false}
          onChange={(model, effort) => setChoice({ model, effort })}
        />
        {models === null && !modelsError && <p className="muted">Asking the harness for its models…</p>}
        {modelsError && <p className="muted">The harness did not list its models ({modelsError}); inherit still launches.</p>}
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
          Native settings are inherited: approvals the mode asks for reach you here, and the
          checkout&apos;s own instructions, hooks and permission rules apply as in a terminal.
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

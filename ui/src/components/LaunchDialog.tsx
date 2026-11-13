import { useEffect, useRef, useState, type FormEvent } from 'react'
import { startConversation } from '../api'
import { harnesses } from '../harnesses'
import { effortsFor, useModels } from '../models'
import { permissionModes } from '../permissions'
import type { Conversation } from '../types'
import { ModelPicker } from './ModelPicker'
import { IconButton } from './Icon'

interface LaunchDialogProps {
  projectId: string
  // why each harness that cannot run from mega's environment cannot, by
  // harness; such a harness is offered disabled, with its reason.
  unavailable: Record<string, string>
  onClose: () => void
  onLaunched: (conversation: Conversation) => void
}

// a new conversation's model, effort and (claude) permission mode are
// concrete from the start. the dialog preselects what the harness's settings
// name, as the service copies them, read afresh each time it opens, and
// offers only concrete values. claude settings that name no effort level
// preselect none, and Launch waits until one is chosen.
export function LaunchDialog({ projectId, unavailable, onClose, onLaunched }: LaunchDialogProps) {
  const dialogRef = useRef<HTMLDialogElement>(null)
  const [harness, setHarness] = useState(() => harnesses.find((h) => !unavailable[h.value])?.value ?? 'claude')
  const cannotRun = unavailable[harness]
  const { models, error: modelsError } = useModels(projectId, harness, true)
  // what the operator has picked; an unpicked field shows the copy.
  const [picked, setPicked] = useState<{ harness: string; mode?: string; model?: string; effort?: string }>({ harness })
  const own = picked.harness === harness ? picked : { harness }
  const copy = models?.copy
  const permissionMode = own.mode ?? copy?.mode ?? 'default'
  const model = own.model ?? copy?.model ?? ''
  const effort = own.effort ?? copy?.effort ?? ''
  const needsEffort = model !== '' && effort === '' && effortsFor(models, model).length > 0
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
          model,
          effort,
        }),
      )
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure))
      setSubmitting(false)
    }
  }

  let launchLabel = 'Launch'
  if (cannotRun) {
    launchLabel = cannotRun
  } else if (models === null && !modelsError) {
    launchLabel = 'Waiting for the harness to list its models'
  } else if (needsEffort) {
    launchLabel = 'Choose an effort level first'
  }

  return (
    <dialog ref={dialogRef} onClose={onClose}>
      <form onSubmit={(event) => void onSubmit(event)}>
        <div className="dialog-head">
          <h2>New conversation</h2>
          <IconButton icon="close" label="Cancel" onClick={onClose} />
        </div>
        <label>
          Harness
          <select
            value={harness}
            onChange={(event) => setHarness(event.target.value)}
          >
            {harnesses.map((h) => (
              <option key={h.value} value={h.value} disabled={!!unavailable[h.value]} title={unavailable[h.value]}>
                {unavailable[h.value] ? `${h.label} (cannot run)` : h.label}
              </option>
            ))}
          </select>
        </label>
        {Object.entries(unavailable).map(([name, reason]) => (
          <p key={name} className="notice harness-missing" role="note">
            {reason}. mega cannot start {harnesses.find((h) => h.value === name)?.label ?? name} until it can be found in the
            project&apos;s environment, which the project page shows.
          </p>
        ))}
        {harness === 'claude' && (
          <label>
            Permission mode
            <select value={permissionMode} onChange={(event) => setPicked({ ...own, mode: event.target.value })}>
              {permissionModes.map((mode) => (
                <option key={mode.value} value={mode.value}>
                  {mode.label}
                </option>
              ))}
            </select>
          </label>
        )}
        <ModelPicker
          models={models}
          model={model}
          effort={effort}
          disabled={models === null}
          onChange={(nextModel, nextEffort) => setPicked({ ...own, model: nextModel, effort: nextEffort })}
        />
        {models === null && !modelsError && <p className="muted">Asking the harness for its models…</p>}
        {modelsError && <p className="muted">The harness did not list its models ({modelsError}); a launch takes what your settings name.</p>}
        {needsEffort && (
          <p className="muted" role="note">
            Your settings name no effort level for this model, so choose one.
          </p>
        )}
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
          The model, effort and mode start as your settings name them, and stay this
          conversation&apos;s until you change them. Approvals the mode asks for reach you here, and the
          checkout&apos;s own instructions, hooks and permission rules apply as in a terminal.
        </p>
        {error && <p className="error">{error}</p>}
        <div className="actions">
          <IconButton
            type="submit"
            icon="rocket_launch"
            label={launchLabel}
            disabled={submitting || prompt.trim() === '' || !!cannotRun || (models === null && !modelsError) || needsEffort}
          />
        </div>
      </form>
    </dialog>
  )
}

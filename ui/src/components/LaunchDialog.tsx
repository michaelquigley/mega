import { useEffect, useRef, useState, type FormEvent } from 'react'
import { startConversation } from '../api'
import { harnessOf, harnesses } from '../harnesses'
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
// offers only concrete values, except claude's mode with no copy (below),
// which the service copies at creation. claude settings that name no effort level
// preselect none, and Launch waits until one is chosen.
export function LaunchDialog({ projectId, unavailable, onClose, onLaunched }: LaunchDialogProps) {
  const dialogRef = useRef<HTMLDialogElement>(null)
  const promptRef = useRef<HTMLTextAreaElement>(null)
  const [harness, setHarness] = useState(() => harnesses.find((h) => !unavailable[h.value])?.value ?? 'claude')
  const { models, error: modelsError, errorCode: modelsCode } = useModels(projectId, harness, true)
  // a harness whose models were refused because it cannot run here is marked
  // as one the project lists as missing.
  const missing: Record<string, string> =
    modelsCode === 'executable_missing' && modelsError && !unavailable[harness] ? { ...unavailable, [harness]: modelsError } : unavailable
  const cannotRun = missing[harness]
  // what the operator has picked; an unpicked field shows the copy.
  const [picked, setPicked] = useState<{ harness: string; mode?: string; model?: string; effort?: string }>({ harness })
  const own = picked.harness === harness ? picked : { harness }
  const copy = models?.copy
  // the mode the operator picked, else the copy's. with neither, as when the
  // model list failed, the selector shows "As your settings name it" and the
  // launch sends none, so the service copies the mode claude's settings name,
  // as the list would have (permissions-never-weakened). ask stays a choice.
  const chosenMode = own.mode ?? copy?.mode
  const model = own.model ?? copy?.model ?? ''
  const effort = own.effort ?? copy?.effort ?? ''
  const needsEffort = model !== '' && effort === '' && effortsFor(models, model).length > 0
  const [name, setName] = useState('')
  const [prompt, setPrompt] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  // showModal moves the focus to the dialog's first control, its close
  // button, so the prompt is focused after it.
  useEffect(() => {
    dialogRef.current?.showModal()
    promptRef.current?.focus()
  }, [])

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault()
    setSubmitting(true)
    setError(null)
    try {
      onLaunched(
        await startConversation(projectId, harness, name, prompt, {
          permissionMode: harnessOf(harness)?.permissionModes ? (chosenMode ?? '') : '',
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
              <option key={h.value} value={h.value} disabled={!!missing[h.value]} title={missing[h.value]}>
                {missing[h.value] ? `${h.label} (cannot run)` : h.label}
              </option>
            ))}
          </select>
        </label>
        {Object.entries(missing).map(([name, reason]) => (
          <p key={name} className="notice harness-missing" role="note">
            {reason}.{' '}
            {/* a configured path's reason already says what to do. */}
            {!reason.includes('; install ') && (
              <>
                mega cannot start {harnesses.find((h) => h.value === name)?.label ?? name} until it can be found in the
                project&apos;s environment, which the project page shows.
              </>
            )}
          </p>
        ))}
        {harnessOf(harness)?.notice && !cannotRun && (
          <p className="harness-notice" role="note">
            {harnessOf(harness)?.notice}
          </p>
        )}
        {harnessOf(harness)?.permissionModes && (
          <label>
            Permission mode
            <select value={chosenMode ?? ''} onChange={(event) => setPicked({ ...own, mode: event.target.value })}>
              {chosenMode === undefined && <option value="">As your settings name it</option>}
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
          unavailable={modelsError !== null}
          loneEffort={harnessOf(harness)?.loneEffort}
          onChange={(nextModel, nextEffort) => setPicked({ ...own, model: nextModel, effort: nextEffort })}
        />
        {models === null && !modelsError && !cannotRun && <p className="muted">Asking the harness for its models…</p>}
        {modelsError && !cannotRun && (
          <p className="muted">
            The harness did not list its models: {modelsError}. A launch takes the model and effort your settings name, and is refused if
            they name none.
            {harnessOf(harness)?.permissionModes && ' Unless you choose a permission mode, it starts in the one your settings name.'}
          </p>
        )}
        {needsEffort && (
          <p className="muted" role="note">
            Your settings name no effort level for this model, so choose one.
          </p>
        )}
        <label>
          Opening prompt
          <textarea
            ref={promptRef}
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

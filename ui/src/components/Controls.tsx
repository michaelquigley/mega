import { useState } from 'react'
import { useModels } from '../models'
import { permissionModes } from '../permissions'
import type { Conversation } from '../types'
import { IconButton } from './Icon'
import { ModelPicker } from './ModelPicker'

// ModeControl changes a claude conversation's permission mode. while the
// runtime runs (live) it switches the session; otherwise it stores the choice
// for the next launch, and an empty choice shows as not chosen. it shows
// the mode the service last confirmed, so a change shows only once recorded. a
// mode inherited from the operator's settings that mega does not offer
// is shown as it is, and cannot be chosen again once left.
export function ModeControl({
  live,
  mode,
  inheritedMode,
  disabled,
  onChange,
}: {
  live: boolean
  mode: string
  inheritedMode: string
  disabled: boolean
  onChange: (mode: string) => void
}) {
  const offered = permissionModes.some((option) => option.value === mode)
  let unoffered = mode
  if (mode === '') {
    unoffered = inheritedMode ? `Not chosen: inherits ${inheritedMode} from your settings` : 'Not chosen: default'
  }
  const when = live ? 'Applies from the next tool call.' : 'Applies at the next launch.'
  return (
    <label className="bar-control mode-control" title={`Permission mode. ${when}`}>
      <span className="bar-label">Mode</span>
      <select aria-label="Permission mode" value={mode} disabled={disabled} onChange={(event) => onChange(event.target.value)}>
        {!offered && (
          <option value={mode} disabled>
            {unoffered}
          </option>
        )}
        {permissionModes.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    </label>
  )
}

// ModelControl changes a conversation's model and effort. while the runtime
// runs (live) it asks the harness: claude applies a change from the next
// response, codex with the next prompt. otherwise it stores the choice for the
// next launch. while live, claude cannot pick inherit again once left; codex
// can. it shows the stored choice; the header shows what the harness reports.
// the change goes with Apply, and says when it applies while it waits for it.
export function ModelControl({
  conversation,
  projectId,
  disabled,
  onApply,
}: {
  conversation: Conversation
  projectId: string
  disabled: boolean
  onApply: (model: string, effort: string) => Promise<boolean>
}) {
  const { models } = useModels(projectId, conversation.harness)
  const stored = { model: conversation.model_choice ?? '', effort: conversation.effort_choice ?? '' }
  const [draft, setDraft] = useState(stored)
  const live = conversation.state === 'running' && !!conversation.model
  // inherit may always be chosen with no runtime; while running only for
  // codex, which sends the config's values from the next prompt. claude has
  // no way back to the operator's settings mid-session.
  const inheritable = !live || conversation.harness === 'codex'
  const changed = draft.model !== stored.model || draft.effort !== stored.effort
  let when = 'Applies at the next launch.'
  if (live) {
    when = conversation.harness === 'codex' ? 'Applies from the next prompt.' : 'Applies from the next response.'
  }
  return (
    <div className="bar-control model-control" title={`Model and effort. ${when}`}>
      <ModelPicker
        harness={conversation.harness}
        models={models}
        model={draft.model}
        effort={draft.effort}
        inheritModel={inheritable || stored.model === ''}
        inheritEffort={inheritable || stored.effort === ''}
        disabled={disabled}
        onChange={(model, effort) => setDraft({ model, effort })}
      />
      {changed && (
        <>
          <IconButton icon="check" label={`Apply the model and effort. ${when}`} word="Apply" disabled={disabled} onClick={() => void onApply(draft.model, draft.effort)} />
          <IconButton icon="undo" label="Undo the change" disabled={disabled} onClick={() => setDraft(stored)} />
        </>
      )}
    </div>
  )
}

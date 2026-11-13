import { useState } from 'react'
import { useModels } from '../models'
import { permissionModes, shortMode } from '../permissions'
import type { Conversation } from '../types'
import { IconButton } from './Icon'
import { ModelPicker } from './ModelPicker'
import { ShortSelect } from './ShortSelect'

// ModeControl shows and changes a claude conversation's requested permission
// mode. while the runtime runs it asks claude to switch; otherwise it stores
// the request for the next launch. it shows the request as the service last
// recorded it, and nothing of what runs: each turn's line says that. a mode
// mega does not offer cannot be requested, and shows as it is.
export function ModeControl({ mode, disabled, onChange }: { mode: string; disabled: boolean; onChange: (mode: string) => void }) {
  const offered = permissionModes.some((option) => option.value === mode)
  return (
    <span className="bar-control mode-control">
      <ShortSelect label="Permission mode" value={mode} short={shortMode(mode)} disabled={disabled} onChange={onChange}>
        {!offered && (
          <option value={mode} disabled>
            {mode}
          </option>
        )}
        {permissionModes.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </ShortSelect>
    </span>
  )
}

// ModelControl shows and changes a conversation's requested model and effort.
// while the runtime runs it asks the harness: claude applies a change from
// the next response, codex with the next prompt. otherwise it stores the
// request for the next launch. it shows the request and nothing of what
// runs: each turn's line says what that turn ran with. a change goes with
// Apply.
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
  const changed = draft.model !== stored.model || draft.effort !== stored.effort
  return (
    <span className="bar-control model-control">
      <ModelPicker short models={models} model={draft.model} effort={draft.effort} disabled={disabled} onChange={(model, effort) => setDraft({ model, effort })} />
      {changed && (
        <>
          <IconButton icon="check" label="Apply the model and effort" word="Apply" disabled={disabled || draft.model === ''} onClick={() => void onApply(draft.model, draft.effort)} />
          <IconButton icon="undo" label="Undo the change" disabled={disabled} onClick={() => setDraft(stored)} />
        </>
      )}
    </span>
  )
}

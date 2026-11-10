import { effortAfterModel, effortsFor, findModel, inheritedEffortLabel, inheritedModel, modelLabel } from '../models'
import type { Models } from '../types'

interface ModelPickerProps {
  harness: string
  models: Models | null
  model: string
  effort: string
  // whether inherit (the empty choice) may be picked for each.
  inheritModel: boolean
  inheritEffort: boolean
  disabled: boolean
  onChange: (model: string, effort: string) => void
}

// ModelPicker is a model select and an effort select whose levels are those
// the chosen model takes; a model that takes none disables it. inherit is the
// empty choice, labelled with what it means where that is known. a choice
// the list does not hold (the list not yet loaded, or no longer offering it)
// is kept and shown as it is.
export function ModelPicker({ harness, models, model, effort, inheritModel, inheritEffort, disabled, onChange }: ModelPickerProps) {
  const inherited = inheritedModel(models, harness).label
  const efforts = effortsFor(models, model)
  const noEfforts = models !== null && efforts.length === 0
  const effortInherited = inheritedEffortLabel(models, model, harness)
  return (
    <>
      <label>
        Model
        <select
          value={model}
          disabled={disabled}
          onChange={(event) => onChange(event.target.value, effortAfterModel(models, event.target.value, effort, inheritEffort))}
        >
          {(inheritModel || model === '') && (
            <option value="" disabled={!inheritModel}>
              {inheritLabel(inheritModel, inherited)}
            </option>
          )}
          {model !== '' && !findModel(models, model) && <option value={model}>{model}</option>}
          {models?.models.map((option) => (
            <option key={option.id} value={option.id}>
              {modelLabel(option)}
            </option>
          ))}
        </select>
      </label>
      <label>
        Effort
        <select value={effort} disabled={disabled || (noEfforts && effort === '')} onChange={(event) => onChange(model, event.target.value)}>
          {noEfforts && effort === '' ? (
            <option value="">None for this model</option>
          ) : (
            (inheritEffort || effort === '') && (
              <option value="" disabled={!inheritEffort}>
                {inheritLabel(inheritEffort, effortInherited)}
              </option>
            )
          )}
          {effort !== '' && !efforts.includes(effort) && <option value={effort}>{effort}</option>}
          {efforts.map((level) => (
            <option key={level} value={level}>
              {level}
            </option>
          ))}
        </select>
      </label>
    </>
  )
}

// inheritLabel names the empty choice: inherit, when it may be picked, or not
// chosen, when it is the stored choice but may not be picked again.
function inheritLabel(allowed: boolean, means: string): string {
  if (allowed) {
    return means ? `Inherit: ${means}` : 'Inherit'
  }
  return means ? `Not chosen: inherits ${means}` : 'Not chosen'
}

import { effortAfterModel, effortsFor, modelLabel, shortModel } from '../models'
import type { Models } from '../types'
import { ShortSelect } from './ShortSelect'

interface ModelPickerProps {
  models: Models | null
  model: string
  effort: string
  disabled: boolean
  // short: the control bar's closed controls, which show short values; the
  // launch dialog has room for the full ones.
  short?: boolean
  onChange: (model: string, effort: string) => void
}

// ModelPicker is a model select and an effort select whose levels are those
// the chosen model takes; a model that takes none shows none. every value is
// concrete: a request the list does not hold (the list not yet loaded, no
// longer offering it, or a full id the harness reported) is kept and shown
// as it is. an effort the model takes but nobody has chosen shows as a
// choice still to make.
export function ModelPicker({ models, model, effort, disabled, short = false, onChange }: ModelPickerProps) {
  const efforts = effortsFor(models, model)
  const noEfforts = models !== null && efforts.length === 0
  const listed = models?.models.some((option) => option.id === model) ?? false
  const modelOptions = (
    <>
      {model === '' && (
        <option value="" disabled>
          {models === null ? 'Loading models…' : 'Choose a model'}
        </option>
      )}
      {model !== '' && !listed && <option value={model}>{model}</option>}
      {models?.models.map((option) => (
        <option key={option.id} value={option.id}>
          {modelLabel(option)}
        </option>
      ))}
    </>
  )
  const effortOptions = (
    <>
      {effort === '' && (
        <option value="" disabled>
          {noEfforts ? 'None for this model' : 'Choose an effort'}
        </option>
      )}
      {effort !== '' && !efforts.includes(effort) && <option value={effort}>{effort}</option>}
      {efforts.map((level) => (
        <option key={level} value={level}>
          {level}
        </option>
      ))}
    </>
  )
  const changeModel = (next: string) => onChange(next, effortAfterModel(models, next, effort))
  const changeEffort = (next: string) => onChange(model, next)
  const effortDisabled = disabled || (noEfforts && effort === '')
  if (short) {
    return (
      <>
        <ShortSelect label="Model" value={model} short={shortModel(models, model) || '…'} disabled={disabled} onChange={changeModel}>
          {modelOptions}
        </ShortSelect>
        <ShortSelect label="Effort" value={effort} short={effort || (noEfforts ? 'none' : 'choose')} disabled={effortDisabled} onChange={changeEffort}>
          {effortOptions}
        </ShortSelect>
      </>
    )
  }
  return (
    <>
      <label>
        Model
        <select value={model} disabled={disabled} onChange={(event) => changeModel(event.target.value)}>
          {modelOptions}
        </select>
      </label>
      <label>
        Effort
        <select value={effort} disabled={effortDisabled} onChange={(event) => changeEffort(event.target.value)}>
          {effortOptions}
        </select>
      </label>
    </>
  )
}

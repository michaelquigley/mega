import { useEffect, useState } from 'react'
import { getModels } from './api'
import type { ModelOption, Models } from './types'

// the model lists, per project and harness, for the page's life. the service
// caches each harness's list too; what a new conversation would copy from
// the settings is read afresh whenever a launch dialog asks with fresh set.
const cache = new Map<string, Promise<Models>>()

function load(projectId: string, harness: string, fresh: boolean): Promise<Models> {
  const key = `${projectId}\n${harness}`
  let pending = cache.get(key)
  if (!pending || fresh) {
    pending = getModels(projectId, harness)
    cache.set(key, pending)
    pending.catch(() => cache.delete(key))
  }
  return pending
}

export interface ModelsState {
  models: Models | null
  error: string | null
}

// useModels loads what a harness offers for a project's checkout. a harness
// that cannot list its models leaves the list empty and says why.
export function useModels(projectId: string, harness: string, fresh = false): ModelsState {
  const [state, setState] = useState<ModelsState & { key: string }>({ key: '', models: null, error: null })
  const key = `${projectId}\n${harness}`
  useEffect(() => {
    let current = true
    load(projectId, harness, fresh).then(
      (models) => current && setState({ key, models, error: null }),
      (failure: unknown) =>
        current && setState({ key, models: null, error: failure instanceof Error ? failure.message : String(failure) }),
    )
    return () => {
      current = false
    }
  }, [projectId, harness, fresh, key])
  return state.key === key ? state : { models: null, error: null }
}

// findModel finds a model by the id a request names, or by the id it
// resolves to, as claude reports it: an alias's own row before the default
// row that resolves to the same model.
export function findModel(models: Models | null, id: string | undefined): ModelOption | undefined {
  if (!models || !id) {
    return undefined
  }
  return (
    models.models.find((m) => m.id === id) ??
    models.models.find((m) => m.resolved === id && !m.is_default) ??
    models.models.find((m) => m.resolved === id)
  )
}

// effortsFor is the effort levels a model takes. a model the list does not
// hold, or no list, offers every level any model takes.
export function effortsFor(models: Models | null, model: string): string[] {
  if (!models) {
    return []
  }
  const option = findModel(models, model)
  if (option) {
    return option.efforts
  }
  return [...new Set(models.models.flatMap((m) => m.efforts))]
}

// modelLabel is a model as its open list names it: its name, with the id
// that tells two of the same name apart, and its description where the
// harness gives one.
export function modelLabel(option: ModelOption): string {
  const name = option.display_name || option.id
  const parts = [name === option.id ? name : `${name} · ${option.id}`, option.description].filter(Boolean)
  return parts.join(' — ')
}

// shortModel is a model as a closed control or a turn's line names it: its
// display name without a parenthetical ("Default (recommended)" is
// "Default"), else the id as given.
export function shortModel(models: Models | null, id: string | undefined): string {
  if (!id) {
    return ''
  }
  const option = findModel(models, id)
  const name = option?.display_name?.replace(/\s*\([^)]*\)\s*$/, '').trim()
  return name || id
}

// the effort levels both harnesses name, lowest first.
const effortOrder = ['none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max', 'ultra']

// effortAfterModel is the effort to keep when the model changes: the current
// one if the new model takes it; none for a model that takes none; else the
// highest level the model takes that is not above the current one, or the
// model's default level, or its first.
export function effortAfterModel(models: Models | null, model: string, effort: string): string {
  const efforts = effortsFor(models, model)
  if (effort && efforts.includes(effort)) {
    return effort
  }
  if (efforts.length === 0) {
    return ''
  }
  const rank = effortOrder.indexOf(effort)
  const below = efforts.filter((level) => effortOrder.indexOf(level) >= 0 && effortOrder.indexOf(level) <= rank)
  if (rank >= 0 && below.length > 0) {
    return below.reduce((a, b) => (effortOrder.indexOf(a) >= effortOrder.indexOf(b) ? a : b))
  }
  const option = findModel(models, model)
  return option?.default_effort && efforts.includes(option.default_effort) ? option.default_effort : efforts[0]
}

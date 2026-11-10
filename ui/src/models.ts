import { useEffect, useState } from 'react'
import { getModels } from './api'
import type { ModelOption, Models } from './types'

// the model lists, per project and harness, for the page's life. the service
// caches each harness's list too; what inherit means is read afresh whenever a
// launch dialog asks with fresh set.
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
// that cannot list its models leaves the list empty and says why; inherit
// stays available.
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

// findModel finds a model by the id a choice or a setting names, or by the id
// it resolves to, as claude's aliases do.
export function findModel(models: Models | null, id: string | undefined): ModelOption | undefined {
  if (!models || !id) {
    return undefined
  }
  return models.models.find((m) => m.id === id) ?? models.models.find((m) => m.resolved === id)
}

// fromSettings labels a value read from the operator's settings. codex's
// config/read is its own effective config, so the value is what a launch
// gets. claude's is read from its settings files only, without per-model
// overrides or environment variables, so it is labelled as what the settings
// name rather than as the effective value.
function fromSettings(harness: string, value: string): string {
  return harness === 'claude' ? `settings name ${value}` : `${value} (your settings)`
}

// inheritedModel is the model a launch with no choice gets, where known: the
// operator's settings, else the harness's default.
export function inheritedModel(models: Models | null, harness = ''): { label: string; option?: ModelOption } {
  if (!models) {
    return { label: '' }
  }
  if (models.inherited.model) {
    return { label: fromSettings(harness, models.inherited.model), option: findModel(models, models.inherited.model) }
  }
  const fallback = models.models.find((m) => m.is_default)
  return fallback ? { label: `${modelLabel(fallback)} (harness default)`, option: fallback } : { label: '' }
}

// effortsFor is the effort levels a model choice takes: the chosen model's,
// or for inherit, those of the model inherit means. when that model is not
// known, every level any model takes is offered.
export function effortsFor(models: Models | null, model: string): string[] {
  if (!models) {
    return []
  }
  const option = model ? findModel(models, model) : inheritedModel(models).option
  if (option) {
    return option.efforts
  }
  return [...new Set(models.models.flatMap((m) => m.efforts))]
}

export function modelLabel(option: ModelOption): string {
  const name = option.display_name || option.id
  return name === option.id ? name : `${name} · ${option.id}`
}

// inheritedEffortLabel says what an unchosen effort gets with a model choice.
export function inheritedEffortLabel(models: Models | null, model: string, harness: string): string {
  if (models?.inherited.effort) {
    return fromSettings(harness, models.inherited.effort)
  }
  const option = model ? findModel(models, model) : inheritedModel(models).option
  if (option?.default_effort) {
    return `${option.default_effort} (model default)`
  }
  return "the model's default"
}

// the effort levels both harnesses name, lowest first.
const effortOrder = ['none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max', 'ultra']

// effortAfterModel is the effort to keep when the model changes: the current
// one if the new model takes it; else unchosen where allowed or where the
// model takes none; else the highest level the model takes that is not above
// the current one, or the model's default level, or its first.
export function effortAfterModel(models: Models | null, model: string, effort: string, inheritAllowed: boolean): string {
  const efforts = effortsFor(models, model)
  if (effort && efforts.includes(effort)) {
    return effort
  }
  if (inheritAllowed || efforts.length === 0) {
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

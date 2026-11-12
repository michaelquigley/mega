// browser preferences: how this window arranges the side panel. they are kept
// in this browser's localStorage, like the reply box's height, and are not
// workspace state; a browser that keeps nothing still works with the defaults.

const keys = {
  width: 'mega.panelWidth',
  collapsed: 'mega.panelCollapsed',
  // folds: each project's fold choice. folded: the list of folded projects
  // kept before choices were, read once to carry it over.
  folds: 'mega.projectFolds',
  folded: 'mega.foldedProjects',
}

export const panelWidth = { min: 200, max: 480, initial: 280 }

function read(key: string): string | null {
  try {
    return window.localStorage.getItem(key)
  } catch {
    return null
  }
}

function write(key: string, value: string) {
  try {
    window.localStorage.setItem(key, value)
  } catch {
    // only a preference: it is simply not kept.
  }
}

export const clampWidth = (width: number) => Math.min(Math.max(Math.round(width), panelWidth.min), panelWidth.max)

export function readWidth(): number {
  const stored = Number(read(keys.width))
  return stored ? clampWidth(stored) : panelWidth.initial
}

export const saveWidth = (width: number) => write(keys.width, String(clampWidth(width)))

export const readCollapsed = () => read(keys.collapsed) === 'true'

export const saveCollapsed = (collapsed: boolean) => write(keys.collapsed, String(collapsed))

// a project's fold is the operator's choice when he has made one, kept in
// this browser: folded or unfolded. with no choice, a starred project is
// unfolded and any other folded. the stored list of folded projects from
// before carries over as explicit folded choices, once.
export type FoldChoice = 'folded' | 'unfolded'

function remove(key: string) {
  try {
    window.localStorage.removeItem(key)
  } catch {
    // only a preference.
  }
}

function readChoices(): Record<string, FoldChoice> {
  const choices: Record<string, FoldChoice> = {}
  const stored = read(keys.folds)
  if (stored !== null) {
    try {
      const parsed: unknown = JSON.parse(stored)
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        for (const [id, choice] of Object.entries(parsed)) {
          if (choice === 'folded' || choice === 'unfolded') {
            choices[id] = choice
          }
        }
      }
    } catch {
      // unreadable: no choices.
    }
    return choices
  }
  const legacy = read(keys.folded)
  if (legacy !== null) {
    try {
      const parsed: unknown = JSON.parse(legacy)
      if (Array.isArray(parsed)) {
        for (const id of parsed) {
          if (typeof id === 'string') {
            choices[id] = 'folded'
          }
        }
      }
    } catch {
      // unreadable: nothing to carry over.
    }
    write(keys.folds, JSON.stringify(choices))
    remove(keys.folded)
  }
  return choices
}

// Folds is what the tree needs to fold a project: the operator's choices,
// and the projects shown unfolded only because a conversation in them was
// opened, which is viewing, not a choice, and is never kept.
export interface Folds {
  choices: Record<string, FoldChoice>
  viewing: ReadonlySet<string>
}

let current: Folds = { choices: readChoices(), viewing: new Set() }
const foldListeners = new Set<() => void>()

function setFolds(next: Folds, store: boolean) {
  current = next
  if (store) {
    write(keys.folds, JSON.stringify(next.choices))
  }
  for (const listener of foldListeners) {
    listener()
  }
}

// isFolded is whether the tree folds a project: unfolded while viewed, else
// the operator's choice, else the default from its star.
export function isFolded(state: Folds, project: { id: string; starred: boolean }): boolean {
  if (state.viewing.has(project.id)) {
    return false
  }
  const choice = state.choices[project.id]
  return choice ? choice === 'folded' : !project.starred
}

// the folds: one record for the page, kept in the browser, which the tree
// reads, the fold control writes, and navigation that must not land on
// something hidden marks as viewed.
export const folds = {
  subscribe(listener: () => void) {
    foldListeners.add(listener)
    return () => foldListeners.delete(listener)
  },
  get: () => current,
  // toggle records the operator's choice: the opposite of what the tree shows.
  toggle(project: { id: string; starred: boolean }) {
    const choice: FoldChoice = isFolded(current, project) ? 'unfolded' : 'folded'
    const viewing = new Set(current.viewing)
    viewing.delete(project.id)
    setFolds({ choices: { ...current.choices, [project.id]: choice }, viewing }, true)
  },
  // view unfolds a project for now, recording nothing.
  view(id: string) {
    if (!current.viewing.has(id)) {
      setFolds({ choices: current.choices, viewing: new Set([...current.viewing, id]) }, false)
    }
  },
  // forget drops choices for projects that no longer exist.
  forget(known: Set<string>) {
    const ids = Object.keys(current.choices)
    if (ids.some((id) => !known.has(id)) || [...current.viewing].some((id) => !known.has(id))) {
      const choices = Object.fromEntries(Object.entries(current.choices).filter(([id]) => known.has(id)))
      setFolds({ choices, viewing: new Set([...current.viewing].filter((id) => known.has(id))) }, true)
    }
  },
}

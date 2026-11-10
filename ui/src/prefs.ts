// browser preferences: how this window arranges the side panel. they are kept
// in this browser's localStorage, like the reply box's height, and are not
// workspace state; a browser that keeps nothing still works with the defaults.

const keys = {
  width: 'roundhouse.panelWidth',
  collapsed: 'roundhouse.panelCollapsed',
  folded: 'roundhouse.foldedProjects',
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

function readFolded(): string[] {
  try {
    const parsed: unknown = JSON.parse(read(keys.folded) ?? '[]')
    return Array.isArray(parsed) ? parsed.filter((id): id is string => typeof id === 'string') : []
  } catch {
    return []
  }
}

// the folded projects: one list for the page, kept in the browser, which the
// tree reads and the navigation that must not land on something hidden
// writes.
let folded = readFolded()
const foldListeners = new Set<() => void>()

function setFolded(next: string[]) {
  folded = next
  write(keys.folded, JSON.stringify(next))
  for (const listener of foldListeners) {
    listener()
  }
}

export const folds = {
  subscribe(listener: () => void) {
    foldListeners.add(listener)
    return () => foldListeners.delete(listener)
  },
  get: () => folded,
  toggle(id: string) {
    setFolded(folded.includes(id) ? folded.filter((f) => f !== id) : [...folded, id])
  },
  unfold(id: string) {
    if (folded.includes(id)) {
      setFolded(folded.filter((f) => f !== id))
    }
  },
  // forget drops folded projects that no longer exist.
  forget(known: Set<string>) {
    if (folded.some((id) => !known.has(id))) {
      setFolded(folded.filter((id) => known.has(id)))
    }
  },
}

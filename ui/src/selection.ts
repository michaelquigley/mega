// the selected project or conversation lives in the URL hash, so a reload or a
// second tab opens the same place.

export type Selection = { kind: 'project' | 'conversation'; id: string }

export function parseHash(): Selection | null {
  const match = /^#\/(p|c)\/(.+)$/.exec(window.location.hash)
  if (!match) {
    return null
  }
  return { kind: match[1] === 'p' ? 'project' : 'conversation', id: decodeURIComponent(match[2]) }
}

export function select(selection: Selection) {
  window.location.hash = `#/${selection.kind === 'project' ? 'p' : 'c'}/${encodeURIComponent(selection.id)}`
}

import type { Project } from './types'

// byName orders projects by name, ignoring case, with the id as a stable
// tiebreak, so two browsers always agree on the order.
export function byName(a: Project, b: Project): number {
  const left = a.name.toLowerCase()
  const right = b.name.toLowerCase()
  if (left !== right) {
    return left < right ? -1 : 1
  }
  return a.id < b.id ? -1 : a.id > b.id ? 1 : 0
}

// treeGroups is the tree's order: starred projects first, then the rest, each
// group by name.
export function treeGroups(projects: Project[]): { starred: Project[]; rest: Project[] } {
  const sorted = [...projects].sort(byName)
  return { starred: sorted.filter((p) => p.starred), rest: sorted.filter((p) => !p.starred) }
}

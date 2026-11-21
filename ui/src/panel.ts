// the side panel's two lists of rows: the needs-you list and the projects tree.
export type PanelList = 'needs-you' | 'projects'

// focusPanel puts the focus on one of the side panel's lists itself, which
// holds it with no row highlighted: the first row key then highlights one
// (entryRow). it does nothing when the panel is the rail.
export function focusPanel(list: PanelList = 'projects') {
  document.querySelector<HTMLElement>(`nav.sidebar section.${list}`)?.focus({ preventScroll: true })
}

// entryRow is where the first row key in a focused list lands: the list's
// selected row, else its first. the needs-you list with no rows gives way to
// the projects tree.
export function entryRow(list: PanelList): HTMLElement | null {
  const order = list === 'needs-you' ? ['needs-you', 'projects'] : ['projects']
  for (const name of order) {
    const section = document.querySelector<HTMLElement>(`nav.sidebar section.${name}`)
    const row = section?.querySelector<HTMLElement>('.tree-item[aria-current="true"]') ?? section?.querySelector<HTMLElement>('.tree-item')
    if (row) {
      return row
    }
  }
  return null
}

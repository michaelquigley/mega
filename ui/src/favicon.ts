import { productName } from './brand'
import mark from '../public/favicon.svg?raw'
import needsYouMark from '../public/favicon-needs-you.svg?raw'

// the two states of the tab's mark, embedded in the bundle as data urls, so
// swapping them fetches nothing: the hub, and the hub inverted on the notice
// gold while something needs the operator.
const dataUrl = (svg: string) => `data:image/svg+xml,${encodeURIComponent(svg)}`
export const markIcon = dataUrl(mark)
export const needsYouIcon = dataUrl(needsYouMark)

const baseTitle = productName

// tabState is what the tab shows: the needs-you mark and the count while
// something needs the operator, and the shutdown gate's title, which wins
// while it applies. a page showing only last-known state shows neither the
// count nor the needs-you mark, since it cannot say they are live.
export function tabState(waiting: number, ready: boolean, stale: boolean): { title: string; icon: string } {
  const live = stale ? 0 : waiting
  const icon = live > 0 ? needsYouIcon : markIcon
  if (ready) {
    return { title: `Ready to shut down · ${baseTitle}`, icon }
  }
  return { title: live > 0 ? `(${live}) ${baseTitle}` : baseTitle, icon }
}

// showTab puts the state in the document: the title, and the icon link's href.
export function showTab({ title, icon }: { title: string; icon: string }) {
  document.title = title
  let link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (!link) {
    link = document.createElement('link')
    link.rel = 'icon'
    document.head.append(link)
  }
  link.type = 'image/svg+xml'
  if (link.getAttribute('href') !== icon) {
    link.setAttribute('href', icon)
  }
}

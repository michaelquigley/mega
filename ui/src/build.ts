import type { About } from './types'

// pageBuild is the identity of the interface this page is running: the
// content-hashed assets its own document loaded, sorted and joined by spaces,
// as the service reckons the interface it embeds (ui.Build). it is read once,
// as the page starts. a page served by the development server loads no
// hashed assets, so it has none, and never compares.
export const pageBuild = [...document.querySelectorAll('script[src], link[href]')]
  .map((element) => element.getAttribute('src') ?? element.getAttribute('href') ?? '')
  .filter((ref) => ref.startsWith('/assets/'))
  .sort()
  .join(' ')

// isNewer is whether the service serves another interface than this page's:
// after a rebuild and restart, an open page is still running the previous
// build's interface. it is never true without both identities.
export const isNewer = (serviceBuild: string) => pageBuild !== '' && serviceBuild !== '' && serviceBuild !== pageBuild

export const newerLabel = 'A newer mega is running; reload this page'

// aboutTitle names the build on the controls that open its panel.
export function aboutTitle(about: About | null): string {
  if (!about) {
    return 'mega: the build is not known yet'
  }
  return `mega ${about.version}${about.developer ? ', a developer build' : ''}: show the build`
}

// renders the mark's two states from ui/public into the PNGs beside this file,
// for the daemon's tray and desktop integration. no svg renderer is assumed:
// it draws each svg in headless chromium through playwright-core, loaded from
// the directory PLAYWRIGHT_DIR names, so the repository takes no dependency.
//
//   PLAYWRIGHT_DIR=<dir with node_modules/playwright-core> \
//     node cmd/mega/render-icons.mjs <chromium executable>
//
// it writes desktop-icon-{32,48,192,512}.png and tray-icon.png (128 px, which
// a panel scales down smoothly), and the same with -needs-you for the
// inverted state, with transparent corners.
import { createRequire } from 'node:module'
import { readFileSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const [chrome] = process.argv.slice(2)
if (!chrome || !process.env.PLAYWRIGHT_DIR) {
  console.error('usage: PLAYWRIGHT_DIR=<dir> node cmd/mega/render-icons.mjs <chromium executable>')
  process.exit(2)
}
const { chromium } = createRequire(join(process.env.PLAYWRIGHT_DIR, 'noop.js'))('playwright-core')
const states = [
  { svg: join(here, '..', '..', 'ui', 'public', 'favicon.svg'), suffix: '' },
  { svg: join(here, '..', '..', 'ui', 'public', 'favicon-needs-you.svg'), suffix: '-needs-you' },
]
const outputs = [
  ...[32, 48, 192, 512].map((size) => ({ size, name: `desktop-icon${'%s'}-${size}.png` })),
  { size: 128, name: `tray-icon${'%s'}.png` },
]
const browser = await chromium.launch({ executablePath: chrome })
const page = await browser.newPage({ deviceScaleFactor: 1 })
for (const state of states) {
  const uri = 'data:image/svg+xml,' + encodeURIComponent(readFileSync(state.svg, 'utf8'))
  for (const { size, name } of outputs) {
    await page.setContent(`<body style="margin:0;background:transparent"><img id="mark" src="${uri}" width="${size}" height="${size}" style="display:block"></body>`)
    await page.locator('#mark').evaluate((img) => img.decode())
    const png = await page.locator('#mark').screenshot({ omitBackground: true })
    const file = join(here, name.replace('%s', state.suffix))
    writeFileSync(file, png)
    console.log(file)
  }
}
await browser.close()

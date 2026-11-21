// where the focus goes in the side panel, in Chromium and in WebKit, the
// engine behind Safari, as a page on a Mac: the walk that found Safari's
// clicks leaving a row unfocused. it drives a running demo.mjs, which it
// changes only as a person would: what is open, folded and focused.
//
//   node probes/keys/focus-walk.mjs [URL] [chromium|webkit]
//
// URL is http://127.0.0.1:18490/ by default. WebKit needs
// `npm --prefix checks exec -- playwright install webkit` once.
import { createRequire } from 'node:module'
import { join, resolve } from 'node:path'

const require = createRequire(join(resolve(import.meta.dirname, '../../checks'), 'package.json'))
const { chromium, webkit } = require('playwright')
const url = process.argv[2] || 'http://127.0.0.1:18490/'
const engine = process.argv[3] === 'webkit' ? webkit : chromium

const browser = await engine.launch()
const page = await (await browser.newContext({ viewport: { width: 1280, height: 900 } })).newPage()
await page.addInitScript(() => Object.defineProperty(Navigator.prototype, 'platform', { get: () => 'MacIntel', configurable: true }))
await page.goto(url)
await page.locator('.side-foot .stream-connected').waitFor()
const active = () =>
  page.evaluate(() => {
    const e = document.activeElement
    const row = e?.matches?.('.tree-item') ? e.querySelector('.tree-label')?.textContent : ''
    return `${e?.tagName.toLowerCase()}.${(e?.className || '').split(' ')[0]} "${row ?? ''}"`
  })
const step = async (what, act) => {
  await act()
  await page.waitForTimeout(300)
  console.log(`${what.padEnd(44)} ${await active()}`)
}
console.log(`${process.argv[3] === 'webkit' ? 'webkit' : 'chromium'}, ${url}`)
await step('click a project row (billing)', () => page.locator('.projects .project-item', { hasText: 'billing' }).click())
await step('  then ArrowDown', () => page.keyboard.press('ArrowDown'))
await step('click a conversation (needs-you planner)', () => page.locator('.needs-you .tree-item', { hasText: 'planner' }).click())
await step('click the Projects heading', () => page.locator('.projects h2').click())
await step('  then ArrowDown', () => page.keyboard.press('ArrowDown'))
await step('click the Needs you heading', () => page.locator('.needs-you h2').click())
await step('  then ArrowDown', () => page.keyboard.press('ArrowDown'))
await step('click a conversation (tree reviewer)', async () => {
  await page.getByRole('button', { name: /^Expand all projects/ }).click()
  await page.locator('.projects .tree-children .tree-item', { hasText: 'reviewer' }).click()
})
await step('  then Control+Meta+P', () => page.keyboard.press('Control+Meta+KeyP'))
await step('  then ArrowDown', () => page.keyboard.press('ArrowDown'))
await step('collapse to the rail, then Control+Meta+P', async () => {
  await page.keyboard.press('Control+Meta+KeyS')
  await page.locator('nav.rail').waitFor()
  await page.keyboard.press('Control+Meta+KeyP')
})
// typing anywhere goes to the reply box, the first character with it, and a
// send keeps the cursor there.
const box = page.getByRole('textbox', { name: 'Reply' })
const typed = async (what) => console.log(`${what.padEnd(44)} ${await active()} value=${JSON.stringify(await box.inputValue())}`)
await page.locator('.projects .tree-children .tree-item', { hasText: 'reviewer' }).click()
await page.keyboard.press('Escape')
await page.waitForTimeout(300)
await typed('reviewer open again, then Escape')
await page.keyboard.type('echo')
await page.waitForTimeout(300)
await typed('  then type "echo"')
await page.locator('.projects .project-item', { hasText: 'docs-site' }).focus()
await page.keyboard.press('Shift+KeyX')
await page.waitForTimeout(300)
await typed('  focus a project row, then Shift+X')
await box.fill('echo\nsent from the probe')
await page.keyboard.press('Meta+Enter')
await page.waitForTimeout(500)
await typed('  then Meta+Enter')
await browser.close()

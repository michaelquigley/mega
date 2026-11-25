// probe: why the dialogs suite's "secret registered" step sometimes times out.
//
//   node probes/register-click/probe.mjs [TRIALS]
//
// run it as the checks are run (MEGA_CHECKS_DIR and MEGA_CHECK_BIN set, or
// through make checks' defaults). it uses the checks' own library, so it starts
// a scratch mega on the checks' port with its scratch in
// <checks root>/register-click-probe, against the committed fakes. nothing
// reaches a real harness or the operator's mega.
//
// forced: the dialog's first listing is held, the field selected as fill's
// first step does, the listing let go so the prefill lands, and the path
// inserted as fill's second step does. it says what the field then held and
// what the service answered.
//
// shift: the listing of a folder outside the projects root is held while
// the mouse is pressed on Register, then let go before the release. its note
// is longer than "Looking…" and moves the button down; this says by how much
// and whether the click still registers.
//
// timing: TRIALS times, once the field holds the projects root, as dialogs.mjs
// now waits for, a path outside the root is typed in full and Register
// clicked; it says how long after the field's input event, which starts the
// listing's 80 ms wait, the press came, and the release after the press.
//
// compare: TRIALS times, two registrations back to back, a folder inside the
// root and then a path outside it, each typed in full, its registration
// waited for through the API before the next dialog is opened. a controlled
// comparison, not a replay of the suite: the suite picked its inside folder
// through the list and typed only the outside path at once. it runs once
// typing the moment the dialog is open, and once waiting first for the field
// to hold the projects root, which the dialog fills in when its first listing
// arrives. a registration that does not land within three seconds prints what
// the service refused.
import { mkdirSync } from 'node:fs'
import { join } from 'node:path'
import { suite } from '../../checks/lib/mega.mjs'

const trials = Number(process.argv[2] || 20)
const t = await suite('register-click-probe', { projectsRoot: '~/Repos' })
const { url, until, workspace } = t
const home = join(t.dir, 'home')
for (let i = 0; i < 3 * trials + 2; i++) mkdirSync(join(home, 'Private', `secret${i}`), { recursive: true })
mkdirSync(join(home, 'Repos', 'alpha'), { recursive: true })

const page = await t.page({ width: 1280, height: 900 })
const refused = []
page.on('response', async (r) => {
  if (r.status() >= 400) refused.push(`${r.status()} ${r.request().method()} ${r.url().replace(url, '')} ${r.request().postData() ?? ''} -> ${(await r.text().catch(() => '')).slice(0, 300)}`)
})
await page.addInitScript(() => {
  window.__probe = []
  const mark = (what) => window.__probe.push(`${performance.now().toFixed(1)} ${what}`)
  window.__mark = mark
  for (const type of ['input', 'pointerdown', 'pointerup']) document.addEventListener(type, () => mark(type), true)
})
await page.goto(url)
const registered = async (path) => (await workspace()).projects.some((p) => p.checkout === path)
const marks = () => page.evaluate(() => window.__probe.splice(0))
const mark = (what) => page.evaluate((w) => window.__mark(w), what)

// openRegister is dialogs.mjs's as it was before the fix: it does not wait for
// the field to hold the projects root, so the comparison can measure the race.
// its second pass adds that wait itself. keep it this way.
const openRegister = async () => {
  await until('no register dialog open', async () => (await page.locator('dialog[open].register-dialog').count()) === 0)
  await page.getByRole('button', { name: 'Register a project' }).click()
  const dialog = page.locator('dialog[open].register-dialog')
  await dialog.waitFor()
  return dialog
}
const cancelAny = async () => {
  const open = page.locator('dialog[open].register-dialog')
  if ((await open.count()) > 0) await open.getByRole('button', { name: 'Cancel' }).click({ timeout: 5000 }).catch(() => {})
}
let next = 0
const outside = () => join(home, 'Private', `secret${next++}`)

// forced: fill's two steps by hand, with the dialog's first listing held so
// that the prefill lands between them, as it did in the suite's crashes.
{
  const path = outside()
  const held = []
  const hold = (route) => held.push(route)
  await page.route('**/api/folders?path=', hold)
  await until('no register dialog open', async () => (await page.locator('dialog[open].register-dialog').count()) === 0)
  await page.getByRole('button', { name: 'Register a project' }).click()
  const dialog = page.locator('dialog[open].register-dialog')
  await dialog.waitFor()
  const field = dialog.getByRole('textbox', { name: 'Checkout' })
  await until('the first listing asked for', async () => held.length > 0)
  await field.selectText()
  for (const route of held.splice(0)) await route.continue()
  await page.unroute('**/api/folders?path=', hold)
  await until('the prefill', async () => (await field.inputValue()) === '~/Repos/')
  await page.keyboard.insertText(path)
  const sent = await field.inputValue()
  refused.length = 0
  await dialog.getByRole('button', { name: 'Register' }).click()
  await until('a refusal', async () => refused.length > 0, 3000).catch(() => {})
  console.log(`forced: the field holds ${sent === '~/Repos/' + path ? "the typed path after '~/Repos/'" : JSON.stringify(sent)}; registered: ${await registered(path)}; refused: ${refused.map((r) => r.split(' -> ')[0]).join(' || ') || 'nothing'}`)
  await cancelAny()
}

// shift.
{
  const path = outside()
  const dialog = await openRegister()
  await until('the root listed', async () => (await dialog.locator('.folder-list [role=option]').count()) > 0)
  const held = []
  const hold = (route) => held.push(route)
  await page.route('**/api/folders?path=*Private*', hold)
  await dialog.getByRole('textbox', { name: 'Checkout' }).fill(path)
  await until('the listing asked for', async () => held.length > 0)
  const button = dialog.getByRole('button', { name: 'Register' })
  const before = await button.boundingBox()
  const x = before.x + before.width / 2
  const y = before.y + before.height / 2
  await page.mouse.move(x, y)
  await page.mouse.down()
  for (const route of held.splice(0)) await route.continue()
  await page.unroute('**/api/folders?path=*Private*', hold)
  await until('the note shown', async () => ((await dialog.locator('.folder-note').first().textContent()) ?? '').includes('outside the projects root'))
  const after = await button.boundingBox()
  const under = await page.evaluate(([px, py]) => document.elementFromPoint(px, py)?.closest('button')?.getAttribute('aria-label') ?? 'no button', [x, y])
  await page.mouse.up()
  await until('registered', () => registered(path), 3000).catch(() => {})
  console.log(`shift: Register moved from y ${before.y.toFixed(0)} to ${after.y.toFixed(0)} (${before.height.toFixed(0)} px tall); under the release: ${under}; registered: ${await registered(path)}`)
  await cancelAny()
}

// timing.
{
  const downs = []
  for (let i = 0; i < trials; i++) {
    const path = outside()
    const dialog = await openRegister()
    await until('the field holds the projects root', async () => (await dialog.getByRole('textbox', { name: 'Checkout' }).inputValue()) === '~/Repos/')
    await marks()
    await dialog.getByRole('textbox', { name: 'Checkout' }).fill(path)
    await dialog.getByRole('button', { name: 'Register' }).click()
    await until('registered', () => registered(path), 3000).catch(() => {})
    const log = await marks()
    const at = (what) => Number((log.find((l) => l.endsWith(what)) ?? 'NaN').split(' ')[0])
    downs.push([at('pointerdown') - at('input'), at('pointerup') - at('pointerdown')])
    await cancelAny()
  }
  const fmt = (xs) => `${Math.min(...xs).toFixed(1)} to ${Math.max(...xs).toFixed(1)} ms`
  console.log(`timing: press ${fmt(downs.map((d) => d[0]))} after the field's input event; release ${fmt(downs.map((d) => d[1]))} after the press`)
}

// compare: typing at once, then waiting for the prefill.
for (const wait of [false, true]) {
  let failed = 0
  let appended = 0
  const register = async (path, typed, name) => {
    const dialog = await openRegister()
    const field = dialog.getByRole('textbox', { name: 'Checkout' })
    if (wait) await until('the field holds the projects root', async () => (await field.inputValue()) === '~/Repos/')
    refused.length = 0
    await field.fill(typed)
    if (name) await dialog.getByRole('textbox', { name: 'Name' }).fill(name)
    await dialog.getByRole('button', { name: 'Register' }).click()
    let ok = true
    await until('registered', () => registered(path), 3000).catch(() => (ok = false))
    if (!ok) {
      failed++
      if (refused.some((r) => r.includes('"checkout":"~/Repos/' + typed.replace(/\/$/, '') + '"'))) appended++
      console.log(`  ${wait ? 'waiting' : 'at once'}: ${typed} not registered; ${await page.locator('dialog[open].register-dialog').count()} register dialogs open; refused: ${refused.join(' || ') || 'nothing'}`)
      await cancelAny()
    }
  }
  for (let i = 0; i < trials; i++) {
    const inside = `inside${wait ? trials + i : i}`
    mkdirSync(join(home, 'Repos', inside), { recursive: true })
    await register(join(home, 'Repos', inside), `~/Repos/${inside}/`, inside.replace('inside', 'inside '))
    const path = outside()
    await register(path, path)
  }
  console.log(`compare, ${wait ? 'waiting for the prefill' : 'typing at once'}: ${2 * trials} registrations, ${failed} not registered, ${appended} of them sent with the typed path after the prefill`)
}
await t.done()

// browser check: the project page's two dialogs, against the fakes.
//
//   make checks SUITE=dialogs
//
// the register dialog's folder picker, over a projects root in the scratch
// HOME (projects_root ~/Repos): the field starting at the root and
// completing as it is typed, Tab completing; folders only, a git checkout
// marked, a dot-folder, a file and a link leading out of the root left out;
// clicking in and going up, the way up stopping at the root; a folder outside
// the root listing nothing; a registered folder marked, not entered, and
// refused when named; the name following the folder until it is changed; the
// keyboard; a folder picked and registered; and a path outside the root typed
// in full still registered.
//
// the adopt dialog: sessions for each harness's fake, made by launching
// conversations and removing them, which leaves their native sessions for
// discovery, and one still held by a conversation. at three widths, narrow,
// 1280 and wide: the dialog's size, every row laid out the same (the choice,
// the first prompt on up to two lines, how long ago with the exact time as
// its title, the id small on one line), newest first; each harness's count
// on its control and the held one counted; the filter; the look at a picked
// session for each harness, beside the list; the terminal warning, pi's
// saying it has no lock; and adopting. it saves shots/register.png and
// shots/adopt-{narrow,1280,wide}.png.
import { mkdirSync, symlinkSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { suite } from './lib/mega.mjs'

const t = await suite('dialogs', { projectsRoot: '~/Repos' })
const { url, check, until, api, convOf, workspace } = t
const home = join(t.dir, 'home')
const text = async (loc) => ((await loc.textContent()) ?? '').replace(/\s+/g, ' ').trim()

// ---- the register dialog's picker ----
for (const dir of ['Repos/alpha/.git', 'Repos/alpha/inner', 'Repos/beta/.git', 'Repos/gamma', 'Repos/.hidden', 'Private/secret']) {
  mkdirSync(join(home, dir), { recursive: true })
}
writeFileSync(join(home, 'Repos', 'notes.txt'), 'a file, not a folder')
symlinkSync(join(home, 'Private'), join(home, 'Repos', 'link-out'))
check((await api('POST', '/api/projects', { name: 'Beta', checkout: join(home, 'Repos', 'beta') })).status === 201, 'beta is registered beforehand')

const page = await t.page({ width: 1280, height: 900 })
await page.goto(url)
// it waits for any register dialog still closing to be gone first: the
// service can confirm a registration before the page has closed the dialog
// that asked, and the next steps would find that one.
const openRegister = async () => {
  await until('no register dialog open', async () => (await page.locator('dialog[open].register-dialog').count()) === 0)
  await page.getByRole('button', { name: 'Register a project' }).click()
  const dialog = page.locator('dialog[open].register-dialog')
  await dialog.waitFor()
  return dialog
}
let dialog = await openRegister()
const field = dialog.getByRole('textbox', { name: 'Checkout' })
const nameField = dialog.getByRole('textbox', { name: 'Name' })
const rows = () => dialog.locator('.folder-list [role=option]')
// a folder's note: none, looking, empty, or why nothing is listed.
const names = () => dialog.locator('.folder-list .folder-name').allTextContents()
const at = () => text(dialog.locator('.folder-at'))
const up = dialog.getByRole('button', { name: /^Up a folder/ })
await until('the root listed', async () => (await rows().count()) > 0)
check((await field.inputValue()) === '~/Repos/' && (await at()) === '~/Repos', `the field starts at the projects root: "${await field.inputValue()}"`)
check(
  JSON.stringify(await names()) === JSON.stringify(['alpha', 'beta', 'gamma']) && (await rows().nth(0).locator('.tag').textContent()) === 'git' && (await rows().nth(2).locator('.tag').count()) === 0,
  `folders only, a git checkout marked, no dot-folder, file or link leading out: ${JSON.stringify(await names())}`,
)
const beta = rows().nth(1)
check((await beta.getAttribute('aria-disabled')) === 'true' && (await text(beta)).includes('registered as Beta'), `a registered folder says so: "${await text(beta)}"`)
check(await up.isDisabled(), 'at the root, there is no way up')
await dialog.screenshot({ path: join(t.dir, 'shots', 'register.png') })

// completion as it is typed, and Tab.
await field.fill('~/Repos/ga')
await until('the completion', async () => JSON.stringify(await names()) === '["gamma"]')
check(true, 'typing completes from the service: only the folders that start that way')
await field.press('Tab')
await until('gamma completed', async () => (await field.inputValue()) === '~/Repos/gamma/')
check((await nameField.inputValue()) === 'gamma', `Tab completes the folder, and the name follows it: "${await nameField.inputValue()}"`)

// clicking in, going up, and the root not left.
await up.click()
await until('back at the root', async () => (await field.inputValue()) === '~/Repos/' && (await rows().count()) === 3)
await rows().filter({ hasText: 'alpha' }).click()
await until('inside alpha', async () => (await field.inputValue()) === '~/Repos/alpha/' && JSON.stringify(await names()) === '["inner"]')
check((await at()) === '~/Repos/alpha' && !(await up.isDisabled()), 'a click goes into a folder, and the way up is offered')
await up.click()
await until('up again', async () => (await field.inputValue()) === '~/Repos/')
await beta.click({ force: true })
check((await field.inputValue()) === '~/Repos/', 'a registered folder is not gone into')
await field.fill('~/')
await until('outside the root', async () => (await text(dialog.locator('.folder-note').first())).includes('outside the projects root'))
check(
  (await text(dialog.locator('.folder-note').first())).includes('outside the projects root') && (await rows().count()) === 0 && (await up.isDisabled()),
  `a folder outside the root lists nothing: "${await text(dialog.locator('.folder-note').first())}"`,
)
await field.fill('~/Repos/beta')
await until('beta named', async () => (await dialog.locator('p.error').count()) === 1)
check(
  (await text(dialog.locator('p.error'))) === 'This folder is already registered as Beta.' && (await dialog.getByRole('button', { name: 'Register' }).isDisabled()),
  'naming a registered folder says so, and Register waits',
)

// the keyboard: Down into the list, arrows, Enter in, Left up.
await field.fill('~/Repos/')
await until('the root again', async () => (await rows().count()) === 3)
await field.press('ArrowDown')
await page.keyboard.press('ArrowDown')
await page.keyboard.press('ArrowDown')
const keyed = await dialog.locator('.folder-list [aria-selected=true] .folder-name').textContent()
await page.keyboard.press('Enter')
await until('in by Enter', async () => (await field.inputValue()) === '~/Repos/gamma/' && (await text(dialog.locator('.folder-note').first())) === 'No folders here.')
await page.keyboard.press('ArrowLeft')
check(keyed === 'gamma', 'arrows move through the folders')
await until('up by Left', async () => (await field.inputValue()) === '~/Repos/')
check(true, 'Enter goes into the folder, and Left comes back up')

// the name, changed by hand, stays; a folder picked is registered.
await rows().filter({ hasText: 'gamma' }).click()
await until('gamma picked', async () => (await field.inputValue()) === '~/Repos/gamma/')
await nameField.fill('the gamma project')
await up.click()
await rows().filter({ hasText: 'gamma' }).click()
check((await nameField.inputValue()) === 'the gamma project', 'a name changed by hand stays as the folder changes')
await dialog.getByRole('button', { name: 'Register' }).click()
await until('gamma registered', async () => (await workspace()).projects.some((p) => p.name === 'the gamma project' && p.checkout === join(home, 'Repos', 'gamma')))
check(true, 'the folder picked is registered, with its name')

// a path outside the root, typed in full, is still registered.
dialog = await openRegister()
await dialog.getByRole('textbox', { name: 'Checkout' }).fill(join(home, 'Private', 'secret'))
await dialog.getByRole('button', { name: 'Register' }).click()
await until('secret registered', async () => (await workspace()).projects.some((p) => p.checkout === join(home, 'Private', 'secret')))
check(true, 'a path outside the root typed in full is registered')
await page.close()

// ---- the adopt dialog ----
const P = await t.register('sessions', t.checkout('sessions', { git: true }))
const prompts = {
  claude: [
    'Refactor the event log index so a read by turns never splits a tool call from its result, and add a test that covers a turn imported from a terminal, where the prompt has no turn_end before it and a tool call waits for its result.',
    'Why does the activity line say "Working" after the turn ended?',
    'short one',
  ],
  codex: ['Profile the history read on a twenty thousand event log and report where the time goes, with numbers.', 'tidy the Makefile'],
  pi: ["Summarise this week's notes into a short changelog entry, newest first.", 'echo hello'],
}
const leave = async (harness, prompt) => {
  const id = (await t.launch(P.id, harness, prompt)).id
  await t.idle(id)
  await api('POST', `/api/conversations/${id}/release`)
  await until(`${harness} released`, async () => (await convOf(id)).state === 'none')
  const removed = await api('DELETE', `/api/conversations/${id}`)
  if (removed.status !== 204) throw new Error(`removing: ${removed.status}`)
}
for (const [harness, list] of Object.entries(prompts)) {
  for (const prompt of list) {
    await leave(harness, prompt)
    // a moment apart, so newest first is an order.
    await new Promise((r) => setTimeout(r, 1100))
  }
}
// one held by a conversation: discovery leaves it out and counts it.
const held = (await t.launch(P.id, 'claude', 'a session a conversation still holds')).id
await t.idle(held)

const widths = [
  ['narrow', 700],
  ['1280', 1280],
  ['wide', 1920],
]
for (const [label, width] of widths) {
  const p = await t.page({ width, height: 900 })
  await p.goto(`${url}/#/p/${P.id}`)
  await p.locator('article.detail').getByRole('button', { name: /Adopt a session/ }).click()
  const d = p.locator('dialog[open].adopt-dialog')
  await d.waitFor()
  const choice = (h) => d.locator('.harness-choice', { hasText: h })
  await until(`the counts at ${label}`, async () => (await text(choice('Claude Code').locator('.count'))) === '3' && (await text(choice('Codex').locator('.count'))) === '2' && (await text(choice('pi').locator('.count'))) === '2')
  check(true, `${label}: each harness's control shows its mark and its count: Claude Code 3, Codex 2, pi 2`)
  const box = await d.boundingBox()
  check(
    box.width >= Math.min(1120, width - 32) - 8 && box.height >= Math.min(880, 900 - 32) - 8,
    `${label}: the dialog is wide and most of the window's height: ${Math.round(box.width)}×${Math.round(box.height)}`,
  )
  const counts = await text(d.locator('[data-counts]'))
  check(counts.includes('3 sessions no conversation holds') && counts.includes('1 left out because a conversation already holds it'), `${label}: the held session is counted: "${counts}"`)
  const layout = await d.locator('label.session-row').evaluateAll((labels) =>
    labels.map((l) => {
      const lines = (e) => e.getClientRects().length
      const height = (e) => Math.round(e.getBoundingClientRect().height)
      const lh = parseFloat(getComputedStyle(l.querySelector('.session-preview')).lineHeight)
      return {
        preview: l.querySelector('.session-preview').textContent,
        previewLines: Math.round(height(l.querySelector('.session-preview')) / lh),
        agoLines: lines(l.querySelector('.session-ago')),
        agoTitle: l.querySelector('.session-ago').getAttribute('title') ?? '',
        idLines: lines(l.querySelector('.session-id')),
        radio: Math.round(l.querySelector('input').getBoundingClientRect().width),
      }
    }),
  )
  t.say(`  ${label}: ${JSON.stringify(layout.map(({ preview, ...rest }) => rest))}`)
  check(
    layout.length === 3 && layout.every((r) => r.previewLines >= 1 && r.previewLines <= 2 && r.agoLines === 1 && r.idLines === 1 && r.radio > 0 && r.agoTitle !== ''),
    `${label}: every row lays out the same: the first prompt on up to two lines, how long ago on one with the exact time as its title, the id on one`,
  )
  check(
    JSON.stringify(layout.map((r) => r.preview)) === JSON.stringify([...prompts.claude].reverse()),
    `${label}: newest first`,
  )
  // the filter.
  await d.getByRole('searchbox', { name: /Filter the sessions/ }).fill('event log')
  await until(`${label}: filtered`, async () => (await d.locator('label.session-row').count()) === 1)
  check((await text(d.locator('[data-counts]'))).includes('1 matching'), `${label}: the filter narrows the rows by first prompt`)
  await d.getByRole('searchbox', { name: /Filter the sessions/ }).fill('')
  // the look, for each harness's fake.
  for (const [harnessLabel, harness] of [
    ['Claude Code', 'claude'],
    ['Codex', 'codex'],
    ['pi', 'pi'],
  ]) {
    await choice(harnessLabel).click()
    const row = d.locator('label.session-row').last()
    await until(`${label}: ${harness}'s rows`, async () => (await d.locator('label.session-row').count()) === prompts[harness].length)
    await row.click()
    const look = d.locator('.session-look')
    await until(`${label}: ${harness}'s look`, async () => (await look.locator('h3').count()) > 0)
    const said = await text(look)
    check(said.startsWith('Began with') && said.includes(prompts[harness][0].slice(0, 40)), `${label}: ${harness}: picking a row shows how it began`)
    const warning = await text(d.locator('.adopt-foot p.muted'))
    check(harness === 'pi' ? warning.startsWith('pi has no lock') : warning.startsWith('Close any terminal'), `${label}: ${harness}: the terminal warning${harness === 'pi' ? ', saying pi has no lock' : ''}`)
    if (harness === 'claude') {
      const beside = await p.evaluate(() => {
        const list = document.querySelector('.session-list').getBoundingClientRect()
        const lookBox = document.querySelector('.session-look').getBoundingClientRect()
        return lookBox.left >= list.right - 1 ? 'beside' : lookBox.top >= list.bottom - 1 ? 'below' : 'overlapping'
      })
      check(beside === (width < 760 ? 'below' : 'beside'), `${label}: the look is ${beside} the list`)
      await p.screenshot({ path: join(t.dir, 'shots', `adopt-${label}.png`) })
    }
  }
  await p.close()
}

// adopting: the picked session becomes a conversation.
const ap = await t.page({ width: 1280, height: 900 })
await ap.goto(`${url}/#/p/${P.id}`)
await ap.locator('article.detail').getByRole('button', { name: /Adopt a session/ }).click()
const ad = ap.locator('dialog[open].adopt-dialog')
await until('the sessions to adopt', async () => (await ad.locator('label.session-row').count()) === 3)
await ad.locator('label.session-row', { hasText: 'short one' }).click()
await ad.getByRole('textbox', { name: 'Name' }).fill('adopted from the dialog')
await ad.getByRole('button', { name: 'Adopt' }).click()
await until('adopted', async () => (await workspace()).conversations.some((c) => c.name === 'adopted from the dialog' && c.harness === 'claude'))
check(true, 'adopting binds the picked session to a new conversation')

await t.done()

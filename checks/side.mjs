// browser check: the side panel and the rail, against the fakes.
//
//   make checks SUITE=side
//
// rebuilt from what the journals recorded of the lost side, indicators,
// background and project-remove scripts and from docs/current. what it
// checks: projects sorted by name ignoring case, the id breaking a tie,
// starred first in their own group; project rows bolder than conversations;
// a fold hiding conversations and keeping the rollup mark; stars moving a
// project to its group, seen by a second page from the stream alone; the
// panel's width resized within its bounds and kept; a removed project's
// fold forgotten; the rail: the count, one mark per project in tree order,
// the working arc, its width, kept across a reload, and a mark opening the
// panel on its project; a missing checkout's mark in the tree and on the
// rail, lifting when it is back; project removal refused while a runtime
// lives and naming it, then deleting the project's logs; the shortcuts,
// Ctrl+/ for the next waiting conversation and Ctrl+' for the panel, from
// anywhere, and no bare n; folds, by default from the star, by choice kept in
// the browser, and by navigation recording nothing, with the old folded list
// carried over; the build's label and panel, and a page older than the
// service saying so without reloading; and every control named.
import { existsSync, readFileSync, renameSync } from 'node:fs'
import { join } from 'node:path'
import { suite } from './lib/mega.mjs'

const t = await suite('side')
const { url, check, until, api, convOf, workspace } = t

// projects whose names test the order: two that differ only in case, and one
// whose checkout goes missing.
const reg = async (name) => t.register(name, t.checkout(name.toLowerCase() + '-' + Math.random().toString(16).slice(2, 6)))
const Alpha = await reg('Alpha')
const alpha = await reg('alpha')
const beta = await reg('beta')
const Delta = await reg('Delta')
const gamma = await reg('gamma')
const roamerPath = t.checkout('roamer')
const roamer = await t.register('roamer', roamerPath)

const launch = async (p, name, prompt) => (await t.launch(p.id, 'claude', prompt, { name })).id
const I = await launch(Alpha, 'idler', 'echo\nhello')
const R = await launch(beta, 'asker', 'echo\nhello')
const U = await launch(Delta, 'unread', 'echo\nhello')
const W = await launch(gamma, 'worker', 'echo\nhello')
const M = await launch(roamer, 'roaming', 'echo\nhello')
for (const id of [I, R, U, W, M]) await t.idle(id)
const prompt = async (id, text) => {
  const r = await api('POST', `/api/conversations/${id}/prompt`, { text })
  if (r.status !== 202) throw new Error(`prompting: ${r.status} ${JSON.stringify(r.body)}`)
}
// I, M and W read, so only R and U, each with a request, need the operator:
// two that stay waiting however often they are opened.
for (const id of [I, M, W]) {
  const c = await convOf(id)
  await api('POST', `/api/conversations/${id}/read`, { seq: c.last_seq })
}
await api('POST', `/api/conversations/${M}/release`)
await prompt(R, 'ask')
await prompt(U, 'ask')
await prompt(W, 'tool 600000')
await until('R and U waiting, W working', async () => {
  const [r, w, u] = await Promise.all([convOf(R), convOf(W), convOf(U)])
  return r.attention === 'request' && w.attention === 'running' && u.attention === 'request'
})

const page = await t.page({ width: 1280, height: 900 })
await page.goto(`${url}/`)
await page.locator('.side-foot .stream-connected').waitFor()
const names = () => page.locator('.projects .project-item .tree-label').allTextContents()
const row = (name) => page.locator('.projects .project-row', { has: page.locator('.tree-label', { hasText: new RegExp(`^${name}$`) }) })
const children = (name) => row(name).locator('xpath=..').locator('.tree-children .tree-item')

// sorting: by name ignoring case, the id breaking a tie.
const [first, second] = Alpha.id < alpha.id ? ['Alpha', 'alpha'] : ['alpha', 'Alpha']
await until('the tree', async () => (await names()).length === 6)
check(JSON.stringify(await names()) === JSON.stringify([first, second, 'beta', 'Delta', 'gamma', 'roamer']), `projects sort by name ignoring case, the id breaking a tie: ${JSON.stringify(await names())}`)

// folding, by default: with nothing starred and no choice made, every
// project is folded, each showing its rollup mark.
const choices = (p = page) => p.evaluate(() => JSON.parse(localStorage.getItem('mega.projectFolds') ?? '{}'))
const folded = async (name) => (await row(name).locator('.fold').getAttribute('aria-expanded')) === 'false' && (await children(name).count()) === 0
const allFolded = async () => (await Promise.all(['beta', 'Delta', 'gamma', 'roamer'].map(folded))).every(Boolean)
check(await allFolded(), 'an unstarred project with no choice is folded')
check((await row('beta').locator('.mark').getAttribute('title')) === '1 waiting for your answer' && (await row('gamma').locator('.mark').getAttribute('title')) === '1 working', 'a folded project keeps its rollup mark')

// the operator's fold or unfold is a choice, kept in this browser.
await row('beta').getByRole('button', { name: 'Show the conversations in beta' }).click()
await until('beta unfolded', async () => (await children('beta').count()) === 1)
check((await choices())[beta.id] === 'unfolded', 'unfolding records the choice "unfolded"')
const weight = (loc) => loc.evaluate((e) => getComputedStyle(e).fontWeight)
check((await weight(row('beta').locator('.tree-label'))) === '600' && (await weight(children('beta').first().locator('.tree-label'))) === '400', 'project rows are bolder than their conversations')
await row('beta').getByRole('button', { name: 'Hide the conversations in beta' }).click()
await until('beta folded', async () => folded('beta'))
check((await choices())[beta.id] === 'folded', 'folding records the choice "folded"')

// navigation never lands on something hidden, and records nothing: the
// needs-you list, and Ctrl+/, each unfold the project of what they open.
const needs = page.locator('.needs-you .tree-item')
await needs.filter({ hasText: 'asker' }).click()
await until('beta unfolded by the needs-you list', async () => (await children('beta').count()) === 1)
check(page.url().endsWith(`#/c/${R}`) && (await choices())[beta.id] === 'folded', 'the needs-you list opens the conversation and unfolds its project, recording no choice')
// the URL changes before the page has handled the hash, so each press waits
// for the conversation it opened to be shown.
const heading = () => page.locator('.conversation-head h1').textContent()
await page.keyboard.press('Control+Slash')
await until('Ctrl+/ on unread', async () => (await heading()) === 'unread' && (await children('Delta').count()) === 1)
check(!((Delta.id) in (await choices())), 'Ctrl+/ opens the next waiting conversation and unfolds its project, recording no choice')

// a bare n does nothing now: pressed outside any field, then Ctrl+/, and
// the selection has moved one step, not two (two would be back on unread).
await page.locator('.conversation-head h1').click()
await page.keyboard.press('n')
await page.keyboard.press('Control+Slash')
await until('Ctrl+/ back on asker', async () => (await heading()) === 'asker')
check(true, 'a bare n is no longer a shortcut: Ctrl+/ alone moved the selection')
// Ctrl+/ from the reply box types nothing.
const box = page.getByRole('textbox', { name: 'Reply' })
await box.click()
await box.fill('a draft')
await box.press('Control+Slash')
await until('Ctrl+/ from the reply box', async () => (await heading()) === 'unread')
check((await page.locator('.needs-you .shortcut').textContent()) === 'Ctrl+/' && (await page.locator('.needs-you .shortcut').getAttribute('title')) === 'Next waiting conversation: Ctrl+/ anywhere', 'Ctrl+/ works from the reply box, and the hint reads "Ctrl+/"')

// Ctrl+' shows and hides the side panel, from anywhere, the reply box too.
const rail = page.locator('nav.rail')
const collapseButton = page.getByRole('button', { name: "Collapse the side panel (Ctrl+')" })
check((await collapseButton.getAttribute('title')) === "Collapse the side panel (Ctrl+')", "the collapse control names Ctrl+'")
await box.click()
await box.fill('another draft')
await box.press("Control+Quote")
await rail.waitFor()
check((await box.inputValue()) === 'another draft' && (await page.getByRole('button', { name: "Expand the side panel (Ctrl+')" }).getAttribute('title')) === "Expand the side panel (Ctrl+')", "Ctrl+' from the reply box collapses the panel to the rail, typing nothing; the expand control names it")
await page.locator('.conversation-head h1').click()
await page.keyboard.press('Control+Quote')
await until('the panel back', async () => (await rail.count()) === 0 && (await page.locator('nav.sidebar').count()) === 1)
check(true, "Ctrl+' from the page shows the panel again")
await box.fill('')

// the choices outlast a reload; what navigation unfolded does not.
await page.goto(`${url}/#/p/${roamer.id}`)
await page.reload()
await page.locator('.side-foot .stream-connected').waitFor()
await until('the tree again', async () => (await names()).length === 6)
check((await folded('beta')) && (await folded('Delta')), 'after a reload, the choice stands and what navigation unfolded is folded again')

// stars: a starred project moves to its own group at the top, unfolded
// unless a choice says otherwise, and a second page sees it from the stream.
const other = await t.page()
await other.goto(`${url}/`)
await other.locator('.side-foot .stream-connected').waitFor()
const starredNames = (p) => p.locator('ul[aria-label="Starred projects"] .project-item .tree-label').allTextContents()
await row('gamma').getByRole('button', { name: 'Star gamma' }).click()
await until('gamma starred', async () => JSON.stringify(await starredNames(page)) === '["gamma"]')
await until('gamma unfolded', async () => (await children('gamma').count()) === 1)
check((await names())[0] === 'gamma' && !((gamma.id) in (await choices())), 'a star moves the project to the starred group, first, and with no choice it is unfolded')
await until('the other page sees it', async () => JSON.stringify(await starredNames(other)) === '["gamma"]')
check(true, 'a second page sees the star from the stream alone')
await row('beta').getByRole('button', { name: 'Star beta' }).click()
await until('two starred', async () => JSON.stringify(await starredNames(page)) === '["beta","gamma"]')
check((await folded('beta')) && (await page.locator('ul[aria-label="Other projects"]').count()) === 1, 'two starred sort by name; a star leaves a project folded by choice folded')
// a choice to unfold outlasts an unstar.
await row('gamma').getByRole('button', { name: 'Hide the conversations in gamma' }).click()
await row('gamma').getByRole('button', { name: 'Show the conversations in gamma' }).click()
await until('gamma unfolded by choice', async () => (await choices())[gamma.id] === 'unfolded')
await row('gamma').getByRole('button', { name: 'Unstar gamma' }).click()
await until('gamma unstarred', async () => JSON.stringify(await starredNames(page)) === '["beta"]')
check((await children('gamma').count()) === 1, 'an unstar leaves a project unfolded by choice unfolded')
await row('gamma').getByRole('button', { name: 'Star gamma' }).click()
await until('gamma starred again', async () => JSON.stringify(await starredNames(page)) === '["beta","gamma"]')
await other.close()

// the folded list kept before choices were carries over as folded choices,
// once: gamma, starred, stays folded; beta, starred with no choice there, is
// unfolded.
const carried = await t.page({
  init: (id) => {
    if (localStorage.getItem('mega.projectFolds') === null && localStorage.getItem('mega.foldedProjects') === null) {
      localStorage.setItem('mega.foldedProjects', JSON.stringify([id]))
    }
  },
  initArg: gamma.id,
})
await carried.goto(`${url}/#/p/${roamer.id}`)
await carried.locator('.side-foot .stream-connected').waitFor()
const carriedRow = (name) => carried.locator('.projects .project-row', { has: carried.locator('.tree-label', { hasText: new RegExp(`^${name}$`) }) })
await until('the carried tree', async () => (await carried.locator('.projects .project-item').count()) === 6)
const carriedChoices = await choices(carried)
check(
  (await carriedRow('gamma').locator('.fold').getAttribute('aria-expanded')) === 'false' &&
    (await carriedRow('beta').locator('.fold').getAttribute('aria-expanded')) === 'true' &&
    JSON.stringify(carriedChoices) === JSON.stringify({ [gamma.id]: 'folded' }) &&
    (await carried.evaluate(() => localStorage.getItem('mega.foldedProjects'))) === null,
  `the old folded list carries over as folded choices, and is gone: ${JSON.stringify(carriedChoices)}`,
)
await carried.close()

// the panel's width: resized from its handle within 200 to 480, and kept.
const side = page.locator('.side')
const width = async () => Math.round((await side.boundingBox()).width)
const handle = page.getByRole('separator', { name: 'Resize the side panel' })
await handle.focus()
for (let i = 0; i < 20; i++) await handle.press('ArrowRight')
await until('the widest', async () => (await width()) === 480)
for (let i = 0; i < 30; i++) await handle.press('ArrowLeft')
await until('the narrowest', async () => (await width()) === 200)
for (let i = 0; i < 5; i++) await handle.press('ArrowRight')
await until('280', async () => (await width()) === 280)
await page.reload()
await page.locator('.side-foot .stream-connected').waitFor()
check((await width()) === 280, 'the width is resized within 200 and 480, and kept across a reload')

// a removed project's choice is forgotten. alpha has no conversations, so
// nothing stands in the way of removing it.
await row('alpha').getByRole('button', { name: 'Show the conversations in alpha' }).click()
await until('the choice kept', async () => alpha.id in (await choices()))
check((await api('DELETE', `/api/projects/${alpha.id}`)).status === 204, 'alpha removed through the API')
await until('its choice forgotten', async () => !(alpha.id in (await choices())))
check(true, "a removed project's choice is forgotten")

// the rail: the count, one mark per project in tree order, the working arc.
await page.getByRole('button', { name: /^Collapse the side panel/ }).click()
await rail.waitFor()
const count = rail.locator('.rail-count')
const needing = (await workspace()).needs_you.length
check(needing === 2 && (await count.textContent()) === '2' && (await count.getAttribute('class')).includes('waiting') && (await count.getAttribute('title')) === '2 conversations need you', `the rail counts what needs you: "${await count.getAttribute('title')}"`)
const railNames = await rail.locator('.rail-mark').evaluateAll((ms) => ms.map((m) => m.getAttribute('aria-label').split(':')[0]))
check(JSON.stringify(railNames) === JSON.stringify(['beta', 'gamma', 'Alpha', 'Delta', 'roamer']), `one mark per project, in tree order: ${JSON.stringify(railNames)}`)
check((await rail.getByRole('button', { name: 'gamma' }).locator('.mark').getAttribute('class')).includes('mark-running'), "gamma's mark is the working arc")
check(Math.round((await rail.boundingBox()).width) <= 46, `the rail is at most 46px wide: ${Math.round((await rail.boundingBox()).width)}`)
await page.reload()
await rail.waitFor()
check(true, 'the collapse is kept across a reload')
await rail.getByRole('button', { name: 'Delta' }).click()
await until('the panel open on Delta', async () => page.url().endsWith(`#/p/${Delta.id}`) && (await row('Delta').getAttribute('class')).includes('selected'))
check(true, 'a rail mark opens the panel on its project')

// a missing checkout: its mark in the tree and on the rail, lifted once back.
renameSync(roamerPath, `${roamerPath}-away`)
await page.reload()
const missing = `The checkout is missing: '${roamerPath}' (does not exist)`
const treeMissing = row('roamer').locator('.checkout-missing-mark')
await until('the tree mark', async () => (await treeMissing.count()) === 1)
check((await treeMissing.getAttribute('title')) === missing, `the tree marks the project: "${await treeMissing.getAttribute('title')}"`)
await page.getByRole('button', { name: /^Collapse the side panel/ }).click()
const railMissing = rail.locator('.rail-mark.checkout-missing')
await railMissing.waitFor()
check((await railMissing.getAttribute('title')) === `roamer: ${missing}`, 'and so does its rail mark, ringed and titled')
renameSync(`${roamerPath}-away`, roamerPath)
await page.reload()
await rail.waitFor()
await until('the rail mark lifted', async () => (await rail.locator('.rail-mark.checkout-missing').count()) === 0)
await page.getByRole('button', { name: /^Expand the side panel/ }).click()
await until('the tree mark lifted', async () => (await row('roamer').locator('.checkout-missing-mark').count()) === 0)
check(true, 'back in place, both marks lift at the next read')

// project removal: refused while a runtime lives, naming it; then the
// project and its logs go.
await row('Delta').locator('.project-item').click()
await page.getByRole('button', { name: 'Remove from workspace' }).click()
const removeIt = page.getByRole('button', { name: 'Remove the project from the workspace' })
check((await removeIt.isDisabled()) && (await page.locator('.danger').textContent()).includes('Release or resume first: unread') && (await page.locator('.danger').textContent()).includes('The checkout and the native sessions stay untouched'), 'removal waits for the runtime, names it, and says what is kept')
const refused = await api('DELETE', `/api/projects/${Delta.id}`)
check(refused.status === 409 && refused.body.code === 'conflict', `the service refuses it too: ${refused.status} ${refused.body.code}`)
await api('POST', `/api/conversations/${U}/release`)
await until('Remove enabled', async () => removeIt.isEnabled())
await removeIt.click()
await until('Delta gone', async () => !(await names()).includes('Delta'))
check(!existsSync(join(t.dir, 'data', 'conversations', U)) && !(await workspace()).conversations.some((c) => c.id === U), 'removed, the project leaves the tree and its logs are deleted')

// the build: a label beside the connection status, and a control on the
// rail titled with it, each opening what the service says of its build.
const about = (await api('GET', '/api/version')).body
const sdk = JSON.parse(readFileSync(join(t.repo, 'shim/claude/node_modules/@anthropic-ai/claude-agent-sdk/package.json'), 'utf8')).version
const pageBuild = await page.evaluate(() =>
  [...document.querySelectorAll('script[src], link[href]')]
    .map((e) => e.getAttribute('src') ?? e.getAttribute('href'))
    .filter((r) => r.startsWith('/assets/'))
    .sort()
    .join(' '),
)
check(about.developer === true && about.claude_sdk === sdk && about.interface !== '' && about.interface === pageBuild, `the service answers for its build, and its interface is this page's: ${about.version}, sdk ${about.claude_sdk}, "${about.interface}"`)
const label = page.locator('.side-foot .version-label')
const buildTitle = `mega ${about.version}, a developer build: show the build`
check((await label.textContent()) === about.version && (await label.getAttribute('title')) === buildTitle, `the footer shows the version beside the connection status: "${await label.textContent()}"`)
await label.click()
const panel = page.locator('dialog[open]')
await panel.waitFor()
const said = await panel.textContent()
const logFile = join(t.dir, 'xdg/state/mega/mega.log')
check(
  said.includes(`${about.version} (developer build)`) && said.includes(about.go) && said.includes(about.target) && said.includes(logFile) && said.includes('This mega is writing to it.') && said.includes(sdk) && said.includes('Running the interface this mega serves.'),
  `its panel gives the version, Go, the target, the log file, the Claude Agent SDK and this page's standing`,
)
await panel.getByRole('button', { name: 'Close' }).click()
await until('the panel closed', async () => (await panel.count()) === 0)
await page.getByRole('button', { name: /^Collapse the side panel/ }).click()
const railBuild = rail.getByRole('button', { name: buildTitle })
await railBuild.click()
await panel.waitFor()
check((await railBuild.getAttribute('title')) === buildTitle && (await panel.textContent()).includes(logFile), 'the rail carries the build as a title, and opens the same panel')
await panel.getByRole('button', { name: 'Close' }).click()
await until('the panel closed again', async () => (await panel.count()) === 0)

// a page older than the service says so where the connection status is, and
// never reloads itself. the check stands in for a rebuilt service by giving
// this page's workspace reads another interface's identity.
await page.evaluate(() => {
  window.__stayed = true
})
await page.route('**/api/workspace', async (route) => {
  const response = await route.fetch()
  await route.fulfill({ response, headers: { ...response.headers(), 'mega-interface-build': '/assets/index-newer.js /assets/index-newer.css' } })
})
const newer = 'A newer mega is running; reload this page'
await api('PATCH', `/api/projects/${Alpha.id}`, { starred: true })
const dot = rail.locator('.rail-stream')
await until('the rail says so', async () => (await dot.getAttribute('title')) === newer && (await dot.getAttribute('class')).includes('stream-newer'))
check(true, 'on the rail, the connection dot says a newer mega is running')
await page.getByRole('button', { name: /^Expand the side panel/ }).click()
const status = page.locator('.side-foot .stream')
await until('the footer says so', async () => (await status.textContent()) === newer)
check((await status.getAttribute('class')).includes('stream-newer') && (await page.evaluate(() => window.__stayed)) === true, `the footer says "${newer}" where the connection status is, and the page has not reloaded`)
await page.unroute('**/api/workspace')
await api('PATCH', `/api/projects/${Alpha.id}`, { starred: false })
await until('Live again', async () => (await status.textContent()) === 'Live')
check(true, 'a read with the same interface as the page says Live again')

// every control in the panel and on the rail, past the tree's own rows,
// names itself in a title and a label.
const unnamed = async () =>
  page.evaluate(() =>
    [...document.querySelectorAll('nav.sidebar button:not(.tree-item), nav.sidebar [role=separator], nav.rail button, .side-handle')]
      .filter((e) => !e.getAttribute('title') || !e.getAttribute('aria-label'))
      .map((e) => e.outerHTML.slice(0, 80)),
  )
const openUnnamed = await unnamed()
await page.getByRole('button', { name: /^Collapse the side panel/ }).click()
await rail.waitFor()
const railUnnamed = await unnamed()
check(openUnnamed.length === 0 && railUnnamed.length === 0, `every control has a title and a label${openUnnamed.length + railUnnamed.length ? `: ${JSON.stringify([...openUnnamed, ...railUnnamed])}` : ''}`)

await t.done()

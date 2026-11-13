// browser check: the input panel, against the fakes.
//
//   make checks SUITE=panel
//
// rebuilt from what the journals recorded of the lost panel, panelscroll,
// start, idle-mode and models scripts and from docs/current. what it checks:
// the reply box resting at one line and opening to its height on focus, the
// line above the panel staying put as it opens and closes and a view at the
// bottom staying there; a control in the bar
// keeping it open and a press outside closing it; a draft kept across a switch
// of conversation; Ctrl+Enter sending, closing and emptying the box and going
// to the end, following the reply; a refused send keeping the box open,
// focused and with its draft, and saying why; Send and Start disabled with the
// reason while a shutdown is armed or the checkout is missing, the shutdown's
// first when both apply; Start on a released conversation, which runs with
// the mode asked for and no turn; a new conversation's request concrete
// from creation, for each harness, and the launch dialog preselecting it
// with no inherit choice; the bar's closed controls showing the request in
// short values (claude's default mode as ask), the full descriptions in
// their lists, and no word for where a value came from or when it applies;
// the permission mode switched live, and stored when nothing runs; a model
// and effort applied, for claude and for codex, shown on the bar at once
// while the harness still runs the old ones, with the header keeping
// identity only; the harness's mark leftmost on the bar,
// collapsed and open; the bar on one row; and the dock keeping the newest content in view as its
// height changes while the view follows the bottom (a turn's activity line,
// a docked request, the reply box growing), never moving a view scrolled up.
import { renameSync } from 'node:fs'
import { suite } from './lib/mega.mjs'

const t = await suite('panel')
const { url, check, until, api, convOf } = t

const P = await t.register('panel', t.checkout('panel'))
const C = (await t.launch(P.id, 'claude', 'burst 40')).id
const D = (await t.launch(P.id, 'claude', 'echo\nhello')).id
const X = (await t.launch(P.id, 'codex', 'hello')).id
const gonePath = t.checkout('gone')
const M = await t.register('gone', gonePath)
const G = (await t.launch(M.id, 'claude', 'echo\nhello')).id
for (const id of [C, D, X, G]) await t.idle(id)
for (const id of [D, G]) await api('POST', `/api/conversations/${id}/release`)
await until('D and G released', async () => (await convOf(D)).state === 'none' && (await convOf(G)).state === 'none')

const page = await t.page({ width: 1280, height: 700 })
const cluster = page.locator('.panel-cluster')
const box = page.getByRole('textbox', { name: 'Reply' })
const expanded = async () => (await page.locator('.panel-cluster.expanded').count()) === 1
const height = async () => Math.round((await box.boundingBox()).height)
const go = (id) =>
  page.evaluate((c) => {
    location.hash = `#/c/${c}`
  }, id)
const send = page.locator('form.reply button[type=submit]')
// Start is the bar's other button while nothing runs: its label is its reason
// when it is held, so it is found by place, not name.
const start = page.locator('form.reply .bar-start button:not([type=submit])')

await page.goto(`${url}/#/c/${C}`)
await cluster.waitFor()

// resting and opening. the history moves with the panel's top edge: the line
// just above it stays just above it, and a view at the bottom stays there.
const scroller = page.locator('.main')
const fromBottom = () => scroller.evaluate((m) => m.scrollHeight - m.scrollTop - m.clientHeight)
const lineAbove = () =>
  page.evaluate(() => {
    const top = document.querySelector('.panel-cluster').getBoundingClientRect().top
    const above = [...document.querySelectorAll('.events [data-seq]')].filter((e) => e.getBoundingClientRect().bottom <= top).at(-1)
    return { seq: above.dataset.seq, top }
  })
const gapOf = (seq) =>
  page.evaluate((s) => document.querySelector('.panel-cluster').getBoundingClientRect().top - document.querySelector(`.events [data-seq="${s}"]`).getBoundingClientRect().bottom, seq)
// a press outside the cluster that moves nothing: in the main column's empty
// margin beside the history, mid-height. (a click on the header would scroll
// it into view first.)
const outside = async () => {
  const main = await scroller.boundingBox()
  const events = await page.locator('.events').boundingBox()
  const right = main.x + main.width - (events.x + events.width)
  if (right < 24) throw new Error(`no margin beside the history to press in: ${right}px`)
  await page.mouse.click(events.x + events.width + right / 2, main.y + main.height / 3)
}
const resting = await height()
check(!(await expanded()) && resting >= 30 && resting <= 60, `the box rests at one line: ${resting}px`)
await until('the history taller than the view', async () => (await scroller.evaluate((m) => m.scrollHeight - m.clientHeight)) > 600)
await scroller.evaluate((m) => {
  m.scrollTop = m.scrollHeight - m.clientHeight - 300
})
const anchor = await lineAbove()
const gapBefore = await gapOf(anchor.seq)
await box.click()
await until('the box open', expanded)
await until('the box at its height', async () => (await height()) === 120)
check(true, 'focus opens it to its height: 120px')
const risen = anchor.top - (await lineAbove()).top
const gapOpen = await gapOf(anchor.seq)
check(risen > 60 && Math.abs(gapOpen - gapBefore) <= 1, `opening keeps the line above the panel where it was: ${gapBefore.toFixed(1)}px from the edge, then ${gapOpen.toFixed(1)}px, as the edge rose ${Math.round(risen)}px`)
await outside()
await until('the box shut', async () => !(await expanded()) && (await height()) === resting)
check(Math.abs((await gapOf(anchor.seq)) - gapBefore) <= 1, 'and closing returns it')
await scroller.evaluate((m) => {
  m.scrollTop = m.scrollHeight
})
await until('at the bottom', async () => (await fromBottom()) <= 1)
await box.click()
await until('the box open at the bottom', async () => (await expanded()) && (await height()) === 120)
const openBottom = await fromBottom()
await outside()
await until('the box shut at the bottom', async () => !(await expanded()) && (await height()) === resting)
check(openBottom <= 1 && (await fromBottom()) <= 1, `a view at the bottom stays there through opening and closing: ${Math.round(openBottom)}px, then ${Math.round(await fromBottom())}px`)
await box.click()
await until('the box open again', expanded)

// a control in the bar keeps it open; a press outside closes it.
await page.getByRole('button', { name: 'More' }).click()
await page.getByRole('menuitem', { name: 'Release' }).waitFor()
check(await expanded(), 'a control in the bar keeps the box open')
await outside()
await until('the box shut', async () => !(await expanded()))
check((await page.getByRole('menuitem', { name: 'Release' }).count()) === 0 && (await height()) === resting, 'a press outside closes the box and the menu')

// a draft is kept across a switch of conversation.
await box.fill('a draft kept across a switch')
await go(D)
await until('D shown', async () => (await page.locator('.conversation-head h1').textContent()) !== '' && (await box.inputValue()) === '')
await go(C)
await until('C shown with its draft', async () => (await box.inputValue()) === 'a draft kept across a switch')
check(true, 'a draft is kept across a switch')

// Ctrl+Enter sends, closes and empties the box, and goes to the end,
// following the reply.
await page.evaluate(() => {
  document.querySelector('.main').scrollTop = 0
})
await box.fill('drip 4 150')
await box.press('Control+Enter')
await until('the box shut and empty', async () => !(await expanded()) && (await box.inputValue()) === '')
check(await box.evaluate((b) => b !== document.activeElement), 'Ctrl+Enter sends: the box closes, empties and lets go of focus')
await until('the reply landed', async () => (await page.locator('.events .message.assistant', { hasText: 'Drip 4.' }).count()) === 1 && !(await convOf(C)).turn_active)
const atEnd = await page.evaluate(() => {
  const m = document.querySelector('.main')
  return m.scrollHeight - m.scrollTop - m.clientHeight
})
check(atEnd <= 32, `the view went to the end and followed the reply: ${Math.round(atEnd)}px from the bottom`)

// a refused send keeps the box open, focused and with its draft.
await page.route('**/api/conversations/*/prompt', (route) => route.fulfill({ status: 409, contentType: 'application/json', body: '{"code":"conflict","message":"refused by the check"}' }))
await box.click()
await box.fill('a draft the refusal must not lose')
await box.press('Control+Enter')
await until('the refusal shown', async () => ((await page.locator('.panel-cluster .error').textContent().catch(() => '')) ?? '').includes('refused by the check'))
check((await expanded()) && (await box.inputValue()) === 'a draft the refusal must not lose' && (await box.evaluate((b) => b === document.activeElement)), 'a refused send keeps the box open, focused and with its draft, and says why')
await page.unroute('**/api/conversations/*/prompt')
await box.fill('')
await outside()

// the reasons on Send and Start: a shutdown armed.
const pendingReason = 'Waiting to shut down once every agent is idle; cancel the shutdown to start new work'
check((await api('POST', '/api/shutdown/arm')).status === 202, 'a shutdown is armed')
await until('Send held', async () => (await send.getAttribute('aria-label')) === pendingReason && (await send.isDisabled()))
check(true, 'Send is disabled with the shutdown as its reason')
await go(D)
await until('Start held', async () => (await start.getAttribute('aria-label')) === pendingReason && (await start.isDisabled()))
check(true, 'and so is Start')
check((await api('POST', '/api/shutdown/cancel')).status === 202, 'the shutdown is cancelled')
await until('Start free', async () => (await start.getAttribute('aria-label')) === 'Start the runtime on this session, sending nothing' && (await start.isEnabled()))
check(true, 'Start is offered again')

// the reasons: a checkout gone, and a shutdown's reason first when both apply.
renameSync(gonePath, `${gonePath}-away`)
await go(G)
await page.reload()
const missing = `The checkout is missing: '${gonePath}' (does not exist)`
await until('Start held by the checkout', async () => (await start.getAttribute('aria-label')) === missing && (await start.isDisabled()))
check(true, `Start is disabled with the missing checkout as its reason: "${missing}"`)
const refused = await api('POST', `/api/conversations/${G}/start`)
check(refused.status === 409 && refused.body.code === 'checkout_missing', `the service refuses the Start too: ${refused.status} ${refused.body.code}`)
await api('POST', '/api/shutdown/arm')
await until('the shutdown first', async () => (await start.getAttribute('aria-label')) === pendingReason)
check(true, "with a shutdown armed too, the shutdown's reason comes first")
await api('POST', '/api/shutdown/cancel')
renameSync(`${gonePath}-away`, gonePath)
await page.reload()
await until('Start free again', async () => (await start.getAttribute('aria-label')) === 'Start the runtime on this session, sending nothing')
check(true, 'the checkout back, Start is offered')

// the control bar shows the request and nothing else: each closed control a
// short value, the full descriptions in its open list, and no word for
// where a value came from or when it applies. the bar stays one row; the
// header keeps identity only.
const bar = page.locator('form.reply .control-bar')
const oneRow = () =>
  page.evaluate(() => {
    const items = [...document.querySelectorAll('form.reply .control-bar > .bar-start > *, form.reply .control-bar > .bar-end > *')].filter((e) => e.getBoundingClientRect().height > 0)
    const middles = items.map((e) => {
      const r = e.getBoundingClientRect()
      return r.top + r.height / 2
    })
    return Math.max(...middles) - Math.min(...middles) <= 4
  })
const headerText = () => page.locator('.conversation-head').textContent()
const barShot = (name) => bar.screenshot({ path: `${t.dir}/shots/${name}.png` })
// shorts reads the closed controls, mode first when there is one.
const shorts = async () => (await bar.locator('.short-value').allTextContents()).join(' · ')
// words reads everything the bar says: its text, its options, and every
// name and title on it.
const words = () =>
  bar.evaluate((b) =>
    [b.textContent, ...[...b.querySelectorAll('[aria-label],[title]')].flatMap((e) => [e.getAttribute('aria-label'), e.getAttribute('title')])].filter(Boolean).join(' | '),
  )
const sourceWords = /\b(inherit\w*|requested|next)\b/i
const mode = page.getByRole('combobox', { name: 'Permission mode' })
const pickers = page.locator('.model-control select')

// a new conversation's request is concrete from creation, copied once from
// the settings: claude's scratch settings name an effort only, so it takes
// the list's default model and the manual mode, which shows as ask.
for (const id of [C, D, G]) {
  const c = await convOf(id)
  check(c.model_choice === 'default' && c.effort_choice === 'medium' && c.permission_mode_choice === 'default', `claude conversation ${id} was created with a concrete request: ${c.model_choice} · ${c.effort_choice} · ${c.permission_mode_choice}`)
}
const x = await convOf(X)
check(x.model_choice === 'fake-large' && x.effort_choice === 'medium' && !x.permission_mode_choice, `the codex conversation was created with its config's model and effort: ${x.model_choice} · ${x.effort_choice}`)
for (const gone of ['inherited_permission_mode', 'permission_mode_inherited', 'permission_mode_requested', 'model_next', 'effort_next']) {
  check(!(gone in x) && !(gone in (await convOf(G))) && !(gone in (await t.projectOf(P.id))), `the service sends no ${gone}`)
}

// a released claude conversation: its closed controls show the request in
// short values.
await until('the short values on G', async () => (await shorts()) === 'ask · Default · medium')
check((await mode.inputValue()) === 'default' && (await mode.locator('option:checked').textContent()) === 'Ask: ask for each approval', 'claude\'s default mode shows as "ask" closed, and is described in full in its list')
const modelList = await pickers.nth(0).locator('option').allTextContents()
check(modelList.includes('Default (recommended) · default — Fake Opus') && modelList.includes('Fake Sonnet · fake-sonnet — The middle stand-in.'), `the open model list carries the full names and descriptions: ${modelList.join(' / ')}`)
check(!sourceWords.test(await words()), `no control or note on the bar says inherit, requested or next: "${await words()}"`)
check(await oneRow(), 'the control bar is one row')

// a mode chosen with nothing running is stored for the next launch, and the
// operator's gesture recorded as one event, drawn in the history, asking
// nothing of the operator; the bar shows the request at once.
const gBefore = await convOf(G)
await mode.selectOption('auto')
await until('the stored choice', async () => (await convOf(G)).permission_mode_choice === 'auto')
await until('auto on the bar', async () => (await shorts()) === 'auto · Default · medium')
const gAfter = await convOf(G)
const requested = (await api('GET', `/api/conversations/${G}/events?since=${gBefore.last_seq}`)).body.events
check(
  gAfter.last_seq === gBefore.last_seq + 1 && requested.length === 1 && requested[0].status === 'mode_requested' && requested[0].origin === 'service' && requested[0].facts.permission_mode === 'auto',
  `with nothing running the mode is stored and recorded as one mode_requested event: ${JSON.stringify(requested.map((e) => [e.status, e.origin, e.facts]))}`,
)
await until('the request in the history', async () => (await page.locator('.events .system', { hasText: 'Permission mode auto requested: the next launch passes it' }).count()) === 1)
check(gAfter.attention === gBefore.attention, `the history shows "Permission mode auto requested", and attention stays ${gAfter.attention}`)
check(!sourceWords.test(await words()) && (await oneRow()), 'still no word for when it applies, on one row')

// Start on a released conversation: it runs with the mode asked for, and no
// turn.
await go(D)
await until('Start offered on D', async () => (await start.isEnabled()))
await start.click()
await until('D running', async () => (await convOf(D)).state === 'running')
await until('the mode it asked for', async () => (await convOf(D)).permission_mode === 'default' && (await mode.inputValue()) === 'default')
const dNow = await convOf(D)
check(!dNow.turn_active && (await page.locator('.activity').count()) === 0, `Start runs the runtime, sending nothing, in the mode asked for: ${dNow.permission_mode}`)
await page.getByRole('button', { name: 'More' }).click()
check(await page.getByRole('menuitem', { name: 'Release' }).isEnabled(), 'and Release is offered')
await page.keyboard.press('Escape')
await outside()

// the header keeps identity only: the harness and its version in words, and
// no model, effort or permission mode, which the control bar shows.
const headFacts = await page.locator('.conversation-head .facts').first().textContent()
const head = await headerText()
check(headFacts === 'Claude Code · vfake' && !head.includes('fake-opus') && !head.includes('effort') && !head.includes('permissions') && !head.includes('long-lived'), `the header names the harness and version in words, and no model, effort or mode: "${headFacts}"`)

// the harness's mark, beside the model, collapsed and open.
const claudeMark = bar.getByRole('img', { name: 'Claude Code' })
// leftmost: no other item of the bar starts left of the mark.
const leftmost = (mark) =>
  mark.evaluate((m) => {
    const left = m.getBoundingClientRect().left
    return [...document.querySelectorAll('form.reply .control-bar > .bar-start > *, form.reply .control-bar > .bar-end > *')].every((e) => e === m || e.getBoundingClientRect().left > left)
  })
check((await claudeMark.isVisible()) && (await claudeMark.getAttribute('title')) === 'Claude Code' && (await leftmost(claudeMark)), 'a claude conversation shows the Claude Code mark on the bar, by name, leftmost, with the box collapsed')
await barShot('bar-claude-collapsed')
await box.click()
await until('the box open for the mark', expanded)
check(await claudeMark.isVisible(), 'and with the box open')
await barShot('bar-claude-open')
await outside()
await until('the box shut for the mark', async () => !(await expanded()))

// the mode switched live.
await mode.selectOption('plan')
await until('the live mode', async () => (await convOf(D)).permission_mode === 'plan' && (await shorts()) === 'plan · Default · medium')
check(!sourceWords.test(await words()), 'a live switch shows the request, plan, and says nothing of when it applies')
// the request is recorded before claude is asked, and once claude confirms,
// the history draws only claude's report.
const dEvents = (await api('GET', `/api/conversations/${D}/events?since=0`)).body.events
const planTrail = dEvents.filter((e) => e.kind === 'runtime' && ['mode_requested', 'mode_changed'].includes(e.status) && e.facts?.permission_mode === 'plan').map((e) => e.status)
await until('the confirmation in the history', async () => (await page.locator('.events .system', { hasText: /^Permission mode plan/ }).count()) >= 1)
const planRows = await page.locator('.events .system', { hasText: /^Permission mode plan/ }).allTextContents()
check(
  planTrail.join(',') === 'mode_requested,mode_changed' && planRows.length === 1 && !planRows[0].includes('requested'),
  `the live request is recorded first, and the history shows only claude's confirmation: ${planTrail.join(', ')} / "${planRows.join('", "')}"`,
)

// a claude model and effort: the bar shows the request as soon as it is
// made, while claude still runs the old model until the next response.
await pickers.nth(0).selectOption('fake-sonnet')
await pickers.nth(1).selectOption('low')
await page.getByRole('button', { name: 'Apply the model and effort', exact: true }).click()
await until('the claude request shown', async () => (await convOf(D)).model_choice === 'fake-sonnet' && (await shorts()) === 'plan · Fake Sonnet · low')
check((await convOf(D)).model === 'fake-opus-1' && !(await headerText()).includes('fake-sonnet'), `applied, the bar shows the request, Fake Sonnet at low, while claude still reports ${(await convOf(D)).model}`)
check((await oneRow()) && !sourceWords.test(await words()), 'the control bar stays one row, with no next-send note')
await box.click()
await box.fill('echo\nagain')
await box.press('Control+Enter')
await until('claude reports the model', async () => (await convOf(D)).model === 'fake-sonnet-1')
check((await shorts()) === 'plan · Fake Sonnet · low', 'the next response runs it, and the bar still shows the request')

// a codex conversation: its request in short values, then a change.
await go(X)
await until('X shown', async () => (await page.locator('.conversation-head .facts').first().textContent()) === 'Codex · vfake')
check((await convOf(X)).model === 'fake-large' && (await convOf(X)).effort === 'medium', 'a codex conversation runs fake-large at medium')
await until('the codex shorts', async () => (await shorts()) === 'Fake Large · medium')
check((await mode.count()) === 0 && !sourceWords.test(await words()) && (await oneRow()), 'a codex bar shows "Fake Large · medium", no mode, and no source or next words, on one row')
const codexMark = bar.getByRole('img', { name: 'Codex' })
check((await codexMark.isVisible()) && (await codexMark.getAttribute('title')) === 'Codex' && (await leftmost(codexMark)) && (await bar.getByRole('img', { name: 'Claude Code' }).count()) === 0, 'a codex conversation shows the Codex mark on the bar, by name, leftmost, with the box collapsed')
await barShot('bar-codex-collapsed')
await box.click()
await until('the codex box open', expanded)
check(await codexMark.isVisible(), 'and with the box open')
await barShot('bar-codex-open')
await outside()
await pickers.nth(0).selectOption('fake-small')
await pickers.nth(1).selectOption('low')
await page.getByRole('button', { name: 'Apply the model and effort', exact: true }).click()
await until('the codex request shown', async () => (await convOf(X)).model_choice === 'fake-small' && (await shorts()) === 'Fake Small · low')
check((await convOf(X)).model === 'fake-large' && (await oneRow()), 'applied, the bar shows the request while codex still runs fake-large, on one row')
await box.click()
await box.fill('hello again')
await box.press('Control+Enter')
await until('the turn reports them', async () => (await convOf(X)).model === 'fake-small' && (await convOf(X)).effort === 'low', 30000)
check(true, 'the next turn runs them')

// the launch dialog offers concrete values only, preselected from the
// settings as the service copies them, and a launch from it is concrete.
await page.goto(`${url}/#/p/${P.id}`)
await page.locator('.section-actions button[aria-label="New conversation"]').click()
const dialog = page.locator('dialog[open]')
await dialog.waitFor()
const field = (name) => dialog.getByRole('combobox', { name })
await until('the claude copy preselected', async () => (await field('Model').inputValue()) === 'default' && (await field('Effort').inputValue()) === 'medium')
check((await field('Permission mode').inputValue()) === 'default', 'the dialog preselects the copy for claude: default · medium · default')
const dialogWords = async () => (await dialog.locator('select').allTextContents()).join(' ') + (await dialog.textContent())
check(!/\binherit\w*/i.test(await dialogWords()), 'the dialog offers no inherit choice')
await field('Harness').selectOption('codex')
await until('the codex copy preselected', async () => (await field('Model').inputValue()) === 'fake-large' && (await field('Effort').inputValue()) === 'medium')
check((await field('Permission mode').count()) === 0 && !/\binherit\w*/i.test(await dialogWords()), 'for codex: fake-large · medium, no mode, no inherit')
await dialog.getByRole('textbox', { name: 'Opening prompt' }).fill('hello from the dialog')
await dialog.getByRole('button', { name: 'Launch' }).click()
await until('the dialog launched', async () => (await t.workspace()).conversations.some((c) => c.name === 'hello from the dialog'))
const launched = (await t.workspace()).conversations.find((c) => c.name === 'hello from the dialog')
check(launched.model_choice === 'fake-large' && launched.effort_choice === 'medium', `a codex conversation launched from the dialog is concrete: ${launched.model_choice} · ${launched.effort_choice}`)
await until('the dialog conversation idle', async () => !(await convOf(launched.id)).turn_active)
await go(D)
// the marks in the dark theme, collapsed and open, for each harness.
const dark = await t.page({ width: 1280, height: 700, colorScheme: 'dark' })
for (const [id, label, name] of [
  [D, 'Claude Code', 'claude'],
  [X, 'Codex', 'codex'],
]) {
  await dark.goto(`${url}/#/c/${id}`)
  const darkBar = dark.locator('form.reply .control-bar')
  const mark = darkBar.getByRole('img', { name: label })
  await mark.waitFor()
  await darkBar.screenshot({ path: `${t.dir}/shots/bar-${name}-collapsed-dark.png` })
  await dark.getByRole('textbox', { name: 'Reply' }).click()
  await until(`the ${name} box open in the dark`, async () => (await dark.locator('.panel-cluster.expanded').count()) === 1)
  check(await mark.isVisible(), `in the dark theme, the ${label} mark shows, collapsed and open`)
  await darkBar.screenshot({ path: `${t.dir}/shots/bar-${name}-open-dark.png` })
  await dark.locator('.conversation-head h1').click()
}
await dark.close()
await t.shot(page, 'panel')

// the dock keeps the newest content in view. while the view follows the
// bottom, any change in the dock's height (the activity line appearing as a
// turn starts, a docked request, the reply box growing) keeps the newest row
// above the dock; a view scrolled up is never moved by it. the fake's hold
// verb starts a turn and says nothing until it is interrupted, so the dock
// grows with no output to re-pin the view.
const H = (await t.launch(P.id, 'claude', 'burst 40', { name: 'dock' })).id
await t.idle(H)
await go(H)
await until('H shown', async () => (await page.locator('.conversation-head h1').textContent()) === 'dock')
const activityLine = page.locator('.panel-cluster .activity')
const docked = page.locator('.panel-cluster .docked section.request-card')
// the dock's top edge and the bottom edge of the last history row, and of the
// last prompt block, as the browser paints them: read after two frames, since
// between react's commit and the next frame's resize observers a layout read
// sees a state that is never painted.
const edges = () =>
  page.evaluate(async () => {
    await new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)))
    const dock = document.querySelector('.panel-cluster').getBoundingClientRect().top
    const rows = [...document.querySelectorAll('.events [data-seq]')]
    const prompts = [...document.querySelectorAll('.events .message.user')]
    return { dock, row: rows.at(-1).getBoundingClientRect().bottom, prompt: prompts.at(-1).getBoundingClientRect().bottom, prompts: prompts.length }
  })
const interrupt = async (id) => {
  await api('POST', `/api/conversations/${id}/interrupt`)
  await until('the turn ended', async () => !(await convOf(id)).turn_active)
}
await scroller.evaluate((m) => {
  m.scrollTop = m.scrollHeight
})
await until('H at the bottom', async () => (await fromBottom()) <= 1)

// 1. from the bottom, a prompt sent with the reply held: the prompt block
// stays above the dock once the activity line is in it. the order that hides
// the line is the prompt shown and the view sent to the end first, then the
// workspace read that says the turn started, which brings the activity line.
// the check makes that order certain: workspace reads wait until the prompt
// row is shown and the box has closed.
// chromium's scroll anchoring moves a view at the bottom down with a growing
// sticky dock on its own, which hides the defect; firefox, the operator's
// browser, does not. the dock's steps turn anchoring off, so they hold for a
// browser that does not anchor, and the page must keep the view pinned
// itself.
await page.addStyleTag({ content: '.main { overflow-anchor: none }' })
const promptsBefore = (await edges()).prompts
const heldReads = []
await page.route('**/api/workspace', (route) => heldReads.push(route))
await box.click()
await box.fill('hold')
await box.press('Control+Enter')
await until('the prompt shown and the box closed', async () => (await edges()).prompts === promptsBefore + 1 && !(await expanded()) && (await convOf(H)).turn_active)
check((await activityLine.count()) === 0, 'the prompt is shown before the page has read that its turn started')
for (const route of heldReads) await route.continue()
await page.unroute('**/api/workspace')
await until('the held turn under way, its activity line shown', async () => await activityLine.isVisible())
const held = await edges()
check(held.prompt <= held.dock + 0.5, `sent from the bottom, the prompt block stays above the dock once the activity line shows: its bottom at ${held.prompt.toFixed(1)}, the dock's top at ${held.dock.toFixed(1)}`)
await interrupt(H)

// 2. while following, a docked request appears: the last row stays above it.
await until('following again', async () => (await fromBottom()) <= 1)
check((await api('POST', `/api/conversations/${H}/prompt`, { text: 'ask' })).status === 202, 'a request is asked for while following')
await docked.waitFor()
const asked = await edges()
check(asked.row <= asked.dock + 0.5, `while following, a docked request leaves the last row above the dock: its bottom at ${asked.row.toFixed(1)}, the dock's top at ${asked.dock.toFixed(1)}`)
await docked.first().getByRole('button', { name: 'Deny', exact: true }).click()
await until('the request cleared and the turn over', async () => (await docked.count()) === 0 && !(await convOf(H)).turn_active)

// 3. scrolled up by a few hundred pixels, neither moves the view. the prompt
// comes from the API here, since a send from the box goes to the end.
await until('at the bottom before scrolling up', async () => (await fromBottom()) <= 1)
await scroller.evaluate(async (m) => {
  m.scrollTop -= 400
  await new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)))
})
const firstInView = () =>
  page.evaluate(() => {
    const row = [...document.querySelectorAll('.events [data-seq]')].find((e) => e.getBoundingClientRect().bottom > 0)
    return { seq: row.dataset.seq, top: row.getBoundingClientRect().top }
  })
const topOf = (seq) => page.evaluate((s) => document.querySelector(`.events [data-seq="${s}"]`).getBoundingClientRect().top, seq)
const inView = await firstInView()
check((await api('POST', `/api/conversations/${H}/prompt`, { text: 'hold' })).status === 202, 'scrolled up, a held turn is started')
await until('the held turn under way while scrolled up', async () => (await convOf(H)).turn_active && (await activityLine.isVisible()))
const afterHold = await topOf(inView.seq)
check(Math.abs(afterHold - inView.top) <= 0.5, `scrolled up, the activity line appearing does not move the view: row ${inView.seq} at ${inView.top.toFixed(1)}, then ${afterHold.toFixed(1)}`)
await interrupt(H)
const inView2 = await firstInView()
check((await api('POST', `/api/conversations/${H}/prompt`, { text: 'ask' })).status === 202, 'scrolled up, a request is asked for')
await docked.waitFor()
const afterAsk = await topOf(inView2.seq)
check(Math.abs(afterAsk - inView2.top) <= 0.5, `scrolled up, a docked request does not move the view: row ${inView2.seq} at ${inView2.top.toFixed(1)}, then ${afterAsk.toFixed(1)}`)
await docked.first().getByRole('button', { name: 'Deny', exact: true }).click()
await until('the request cleared again', async () => (await docked.count()) === 0 && !(await convOf(H)).turn_active)

// 4. following, the reply box growing with its lines keeps the last row
// above the dock.
await scroller.evaluate((m) => {
  m.scrollTop = m.scrollHeight
})
await until('back at the bottom', async () => (await fromBottom()) <= 1)
await box.click()
await until('the box open', expanded)
const opened = await height()
await box.fill(Array.from({ length: 14 }, (_, i) => `line ${i + 1} of a long draft`).join('\n'))
await until('the box grown', async () => (await height()) > opened)
const grown = await edges()
check(grown.row <= grown.dock + 0.5, `following, the reply box growing to ${await height()}px keeps the last row above the dock: its bottom at ${grown.row.toFixed(1)}, the dock's top at ${grown.dock.toFixed(1)}`)
await box.fill('')
await outside()
await t.shot(page, 'dock')

await t.done()

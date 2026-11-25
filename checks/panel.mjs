// browser check: the input panel, against the fakes.
//
//   make checks SUITE=panel
//
// rebuilt from what the journals recorded of the lost panel, panelscroll,
// start, idle-mode and models scripts and from docs/current. what it checks:
// the reply box resting at one line with the cursor in it when the page
// loads, its title naming Ctrl+Space, focus alone leaving it at one line and
// the first character opening it to its height, the line above the panel
// staying put as it opens and closes and a view at the bottom staying there;
// a draft opening it on focus; a control in the bar keeping it open and a
// press outside closing it; a draft kept across a switch of conversation,
// the box open on return with the cursor at its end; Ctrl+Enter sending, closing and emptying the box and going
// to the end, following the reply; a refused send keeping the box open,
// focused and with its draft, and saying why; Send and Start disabled with the
// reason while a shutdown is armed or the checkout is missing, the shutdown's
// first when both apply; Start on a released conversation, which runs with
// the mode asked for and no turn; a new conversation's request concrete
// from creation, for each harness, and the launch dialog preselecting it
// with no inherit choice; with its model list failed, the dialog's mode
// selector showing "As your settings name it", a launch with no mode chosen
// starting in the mode the checkout's claude settings name, and ask picked
// sending default; the bar's closed controls showing the request in
// short values (claude's default mode as ask), the full descriptions in
// their lists, and no word for where a value came from or when it applies;
// the permission mode switched live, and stored when nothing runs; a model
// and effort applied, for claude and for codex, shown on the bar at once
// while the harness still runs the old ones, with the header keeping
// identity only; the harness's mark leftmost on the bar, for claude, codex
// and pi; a pi conversation with no mode control and no MCP item, saying how
// pi asks, in the launch dialog too, and with no effort control for a model
// that takes no choice of levels,
// collapsed and open; the bar on one row; and the dock keeping the newest content in view as its
// height changes while the view follows the bottom (a turn's activity line,
// a docked request, the reply box growing), never moving a view scrolled up.
// and @~/ searching the directories named beyond the checkout: its paths
// shown with ~, newest first, no dot-directory and nothing outside it, and a
// pick written per harness, a quoted mention for claude and the bare
// absolute path for codex and pi; and @~/Screenshots/, the second named
// directory, looked in directly and listing its own files; saved as
// shots/path-search-{outside,second}.png.
// and the focus: the cursor in the reply box, at one line, after a pick in
// the side panel or the needs-you list, Ctrl+/, launching and adopting;
// never with a dialog open, nor from a field being typed in; nothing read
// behind a dialog, after a switch by the address or on output arriving, and
// read once it closes; nothing read while the view is scrolled up, nor when a
// dialog closes over a view scrolled up, and read on return to the newest
// output; and a failed read tried again; Ctrl+Space from
// the history and the side panel, and nothing with a dialog open or the
// cursor already there; Escape leaving the box for the history, whose keys
// then scroll, and closing the path list first; Page Up and Page Down
// scrolling the history from an empty box and ending following, and the
// box's own with a draft; and the allow keys from a box the switch focused
// and from the history.
import { mkdirSync, renameSync, utimesSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { suite } from './lib/mega.mjs'

// ~/Sandbox and ~/Screenshots, in the suite's scratch HOME, are named beyond
// the checkout for the @ search, in that order.
const t = await suite('panel', { searchDirs: ['~/Sandbox', '~/Screenshots'] })
const { url, check, must, until, api, convOf } = t

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
// wheel scrolls the history as a reader does, with the mouse wheel over it,
// and waits two frames for the page to have read the scroll. only the reader
// ends following, so a scripted scroll that moves the view up is no reader
// and ends nothing.
const settled = () => page.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))))
const wheel = async (dy) => {
  const b = await scroller.boundingBox()
  await page.mouse.move(b.x + b.width / 2, b.y + 80)
  const before = await scroller.evaluate((m) => m.scrollTop)
  await page.mouse.wheel(0, dy)
  await until('the wheel to scroll the view', async () => (await scroller.evaluate((m) => m.scrollTop)) !== before)
  let last = -1
  await until('the view to settle', async () => {
    await settled()
    const now = await scroller.evaluate((m) => m.scrollTop)
    const still = now === last
    last = now
    return still
  })
}
const focused = () => box.evaluate((b) => b === document.activeElement)
await until('the cursor in the reply box on loading the page', focused)
const resting = await height()
check(!(await expanded()) && resting >= 30 && resting <= 60, `loading the page on a conversation puts the cursor in its reply box, which rests at one line: ${resting}px`)
const boxTitle = (await box.getAttribute('title')) ?? ''
check(boxTitle.includes('Ctrl+Space') && (await box.getAttribute('aria-keyshortcuts')) === 'Control+Space', `the reply box's title names Ctrl+Space: "${boxTitle}"`)
await until('the history taller than the view', async () => (await scroller.evaluate((m) => m.scrollHeight - m.clientHeight)) > 600)
await scroller.evaluate((m) => {
  m.scrollTop = m.scrollHeight
})
await wheel(-300)
const anchor = await lineAbove()
const gapBefore = await gapOf(anchor.seq)
await box.click()
await settled()
check((await focused()) && !(await expanded()) && (await height()) === resting && Math.abs((await gapOf(anchor.seq)) - gapBefore) <= 1, 'focus alone leaves the empty box at one line, and the history where it was')
await page.keyboard.type('a')
await until('the box open', expanded)
await until('the box at its height', async () => (await height()) === 120)
check(true, 'the first character opens it to its height: 120px')
const risen = anchor.top - (await lineAbove()).top
const gapOpen = await gapOf(anchor.seq)
check(risen > 60 && Math.abs(gapOpen - gapBefore) <= 1, `opening keeps the line above the panel where it was: ${gapBefore.toFixed(1)}px from the edge, then ${gapOpen.toFixed(1)}px, as the edge rose ${Math.round(risen)}px`)
await outside()
await until('the box shut', async () => !(await expanded()) && (await height()) === resting)
check(Math.abs((await gapOf(anchor.seq)) - gapBefore) <= 1, 'and closing returns it')
// focus on a box holding a draft opens it.
await box.click()
await until('the box open on its draft', async () => (await expanded()) && (await height()) === 120)
check((await box.inputValue()) === 'a', 'focus on a box holding a draft opens it')
await box.fill('')
await outside()
await until('the box shut and empty', async () => !(await expanded()) && (await box.inputValue()) === '')
await scroller.evaluate((m) => {
  m.scrollTop = m.scrollHeight
})
await until('at the bottom', async () => (await fromBottom()) <= 1)
await box.click()
await settled()
check(!(await expanded()) && (await fromBottom()) <= 1, 'at the bottom, focus alone opens nothing either')
await page.keyboard.type('b')
await until('the box open at the bottom', async () => (await expanded()) && (await height()) === 120)
const openBottom = await fromBottom()
await outside()
await until('the box shut at the bottom', async () => !(await expanded()) && (await height()) === resting)
check(openBottom <= 1 && (await fromBottom()) <= 1, `a view at the bottom stays there through opening at the first character and closing: ${Math.round(openBottom)}px, then ${Math.round(await fromBottom())}px`)
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
await until('the cursor in D’s reply box', focused)
await settled()
check(!(await expanded()) && (await height()) === resting, 'a switch puts the cursor in the reply box, at one line')
await go(C)
await until('C shown with its draft', async () => (await box.inputValue()) === 'a draft kept across a switch')
await until('C’s box open on its draft', async () => (await focused()) && (await expanded()))
const caret = await box.evaluate((b) => [b.selectionStart, b.value.length])
check(caret[0] === caret[1], `a draft is kept across a switch, and on return the box is open with the cursor at its end: ${caret[0]} of ${caret[1]}`)

// Ctrl+Enter sends, closes and empties the box, and goes to the end,
// following the reply.
await wheel(-1e6)
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
await page.keyboard.type('x')
await until('the box open for the mark', expanded)
check(await claudeMark.isVisible(), 'and with the box open')
await barShot('bar-claude-open')
await box.fill('')
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
await page.keyboard.type('x')
await until('the codex box open', expanded)
check(await codexMark.isVisible(), 'and with the box open')
await barShot('bar-codex-open')
await box.fill('')
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

// a pi conversation: pi's mark, the request pi's own settings resolve, no
// permission mode control and no MCP item, since pi has neither for mega,
// pi's own words about how it asks, and the effort control only where the
// model takes a choice of levels.
const PiConv = (await t.launch(P.id, 'pi', 'echo hello')).id
await t.idle(PiConv)
await go(PiConv)
await until('the pi conversation shown', async () => (await page.locator('.conversation-head .facts').first().textContent()).startsWith('pi'))
const pc = await convOf(PiConv)
check(pc.model_choice === 'fake/large' && pc.effort_choice === 'medium' && !pc.permission_mode_choice, `a pi conversation copies pi's own model and thinking level, and no mode: ${pc.model_choice} · ${pc.effort_choice}`)
await until('the pi shorts', async () => (await shorts()) === 'Fake Large · medium')
const piMark = bar.getByRole('img', { name: 'pi' })
check((await mode.count()) === 0 && (await oneRow()) && (await piMark.isVisible()) && (await leftmost(piMark)), 'a pi bar shows pi\'s mark leftmost and the request, with no permission mode control, on one row')
await page.getByRole('button', { name: 'More', exact: true }).click()
await page.getByRole('menuitem', { name: 'Release' }).waitFor()
check((await page.getByRole('menuitem', { name: /MCP server/ }).count()) === 0 && (await page.locator('.control-bar .more .more-mark').count()) === 0, 'and no MCP item in More, since pi runs its own MCP servers and mega cannot list them')
await page.keyboard.press('Escape')
await outside()
const piNotice = await page.locator('.harness-notice').textContent()
check(piNotice.includes('pi asks for nothing unless an extension asks') && piNotice.includes('runs its own MCP servers, which mega cannot list'), `the conversation says how pi asks: "${piNotice}"`)
await barShot('bar-pi-collapsed')
// a model with one level or none: no effort control at all.
await pickers.nth(0).selectOption('fake/small')
check((await pickers.count()) === 1, 'a pi model with no thinking levels to choose from has no effort control')
await page.getByRole('button', { name: 'Apply the model and effort', exact: true }).click()
await until('the pi request applied', async () => (await convOf(PiConv)).model_choice === 'fake/small' && (await shorts()) === 'Fake Small')
const small = await convOf(PiConv)
t.say(`  applied: ${small.model_choice} · "${small.effort_choice ?? ''}", one row ${await oneRow()}`)
check(!small.effort_choice && (await oneRow()), 'applied, the request is Fake Small with no effort, on one row')
await barShot('bar-pi-small')

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
// for pi: its own words about how it asks, no mode, and no effort control
// for a model that takes no choice of levels.
const fresh = await t.page({ width: 1280, height: 700 })
await fresh.goto(`${url}/#/p/${P.id}`)
await fresh.locator('.section-actions button[aria-label="New conversation"]').click()
const piDialog = fresh.locator('dialog[open]')
await piDialog.waitFor()
const piField = (name) => piDialog.getByRole('combobox', { name })
await piField('Harness').selectOption('pi')
await until('the pi copy preselected', async () => (await piField('Model').inputValue()) === 'fake/large' && (await piField('Effort').inputValue()) === 'medium')
const dialogNotice = await piDialog.locator('.harness-notice').textContent()
check((await piField('Permission mode').count()) === 0 && dialogNotice.includes('pi asks for nothing unless an extension asks'), `for pi: no mode, and the dialog says how pi asks: "${dialogNotice}"`)
await piField('Model').selectOption('fake/small')
check((await piField('Effort').count()) === 0, 'and no effort control for a model with no levels to choose')
await fresh.close()
// a model list that fails with anything but executable_missing leaves the
// dialog no copy and Launch enabled. untouched, the mode selector shows "As
// your settings name it", and a launch sends no mode, so the service copies
// the one the checkout's claude settings name (permissions-never-weakened);
// ask stays a choice, and picking it sends default. the page's list is
// failed in the browser, so the service's own copy still succeeds.
const plannedPath = t.checkout('planned')
mkdirSync(join(plannedPath, '.claude'))
writeFileSync(join(plannedPath, '.claude/settings.json'), JSON.stringify({ permissions: { defaultMode: 'plan' } }) + '\n')
const planned = await t.register('planned', plannedPath)
const failing = await t.page({ width: 1280, height: 700 })
await failing.route((u) => u.pathname === `/api/projects/${planned.id}/models`, (route) =>
  route.fulfill({ status: 502, contentType: 'application/json', body: JSON.stringify({ code: 'harness_failed', message: 'the fake list failed' }) }),
)
// failedLaunch opens the dialog over the failed list, picks mode unless it
// is undefined, and launches: what the selector showed, the mode the launch
// sent, the mode the conversation was made with, and the mode its runtime
// reports once its turn has ended.
const failedLaunch = async (name, mode) => {
  await failing.goto(`${url}/#/p/${planned.id}`)
  await failing.locator('.section-actions button[aria-label="New conversation"]').click()
  const failDialog = failing.locator('dialog[open]')
  await failDialog.waitFor()
  await until('the failed list', async () => ((await failDialog.textContent()) ?? '').includes('did not list its models: the fake list failed'))
  const selector = failDialog.getByRole('combobox', { name: 'Permission mode' })
  const shown = { value: await selector.inputValue(), label: await selector.evaluate((s) => s.selectedOptions[0]?.textContent ?? '') }
  const words = (await failDialog.locator('p.muted').allTextContents()).join(' ')
  if (mode !== undefined) await selector.selectOption(mode)
  await failDialog.getByRole('textbox', { name: 'Opening prompt' }).fill(name)
  const sent = failing.waitForRequest((r) => r.method() === 'POST' && new URL(r.url()).pathname === `/api/projects/${planned.id}/conversations`)
  await failDialog.getByRole('button', { name: 'Launch' }).click()
  const body = (await sent).postDataJSON()
  await until(`the launch "${name}"`, async () => (await t.workspace()).conversations.some((c) => c.name === name))
  const conv = (await t.workspace()).conversations.find((c) => c.name === name)
  await t.idle(conv.id)
  return { shown, words, sent: body.permission_mode, made: conv.permission_mode_choice, ran: (await convOf(conv.id)).permission_mode }
}
const untouched = await failedLaunch('hello with no list')
check(
  untouched.words.includes('Unless you choose a permission mode, it starts in the one your settings name.'),
  `with the list failed, the dialog says a launch takes the settings' mode unless one is chosen: "${untouched.words}"`,
)
const pickedAsk = await failedLaunch('hello, picking ask with no list', 'default')
check(
  untouched.shown.value === '' &&
    untouched.shown.label === 'As your settings name it' &&
    untouched.sent === '' &&
    untouched.made === 'plan' &&
    untouched.ran === 'plan' &&
    pickedAsk.shown.label === 'As your settings name it' &&
    pickedAsk.sent === 'default' &&
    pickedAsk.made === 'default' &&
    pickedAsk.ran === 'default',
  `with the list failed, the selector shows "${untouched.shown.label}", an untouched launch sends "${untouched.sent}", is made with the settings' ${untouched.made} and runs in ${untouched.ran}, and picking ask sends "${pickedAsk.sent}", is made with ${pickedAsk.made} and runs in ${pickedAsk.ran}`,
)
await failing.close()
await go(D)
// the marks in the dark theme, collapsed and open, for each harness.
const dark = await t.page({ width: 1280, height: 700, colorScheme: 'dark' })
for (const [id, label, name] of [
  [D, 'Claude Code', 'claude'],
  [X, 'Codex', 'codex'],
  [PiConv, 'pi', 'pi'],
]) {
  await dark.goto(`${url}/#/c/${id}`)
  const darkBar = dark.locator('form.reply .control-bar')
  const mark = darkBar.getByRole('img', { name: label })
  await mark.waitFor()
  await darkBar.screenshot({ path: `${t.dir}/shots/bar-${name}-collapsed-dark.png` })
  const darkBox = dark.getByRole('textbox', { name: 'Reply' })
  await darkBox.click()
  await dark.keyboard.type('x')
  await until(`the ${name} box open in the dark`, async () => (await dark.locator('.panel-cluster.expanded').count()) === 1)
  check(await mark.isVisible(), `in the dark theme, the ${label} mark shows, collapsed and open`)
  await darkBar.screenshot({ path: `${t.dir}/shots/bar-${name}-open-dark.png` })
  await darkBox.fill('')
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
    return {
      dock,
      row: rows.at(-1).getBoundingClientRect().bottom,
      prompt: prompts.at(-1).getBoundingClientRect().bottom,
      promptTop: prompts.at(-1).getBoundingClientRect().top,
      prompts: prompts.length,
      replies: document.querySelectorAll('.events .message.assistant').length,
    }
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
await wheel(-400)
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
await box.fill('a')
await until('the box open', expanded)
const opened = await height()
await box.fill(Array.from({ length: 14 }, (_, i) => `line ${i + 1} of a long draft`).join('\n'))
await until('the box grown', async () => (await height()) > opened)
const grown = await edges()
check(grown.row <= grown.dock + 0.5, `following, the reply box growing to ${await height()}px keeps the last row above the dock: its bottom at ${grown.row.toFixed(1)}, the dock's top at ${grown.dock.toFixed(1)}`)
await box.fill('')
await outside()
await t.shot(page, 'dock')

// 5. sent from above with the reply held: the view goes to the end and
// follows, so the prompt row is in view above the dock before any output
// arrives. the old fault, a scroll event queued by mega's own pin and read
// after the activity line grew the dock, which ended following, was a race
// that one send rarely loses; this sends five times.
const heldFromAbove = []
for (let i = 0; i < 5; i++) {
  await wheel(-1e6)
  const before = await edges()
  await box.click()
  await box.fill('hold')
  await box.press('Control+Enter')
  await until('the held turn under way, sent from above', async () => (await edges()).prompts === before.prompts + 1 && (await convOf(H)).turn_active && (await activityLine.isVisible()))
  await settled()
  const e = await edges()
  heldFromAbove.push({ ok: e.promptTop >= 0 && e.prompt <= e.dock + 0.5 && e.replies === before.replies, top: e.promptTop, bottom: e.prompt, dock: e.dock })
  await interrupt(H)
}
check(
  heldFromAbove.every((h) => h.ok),
  `sent from above five times with the reply held, the prompt row is in view above the dock before any output: ${heldFromAbove.map((h) => `${h.top.toFixed(0)}–${h.bottom.toFixed(0)} under ${h.dock.toFixed(0)}`).join(', ')}`,
)

// 6. while following, the reader wheels up during a streaming reply: the view
// stays where they put it, and the newer-activity control offers the rest.
await scroller.evaluate((m) => {
  m.scrollTop = m.scrollHeight
})
await until('following before the stream', async () => (await fromBottom()) <= 1)
const repliesBefore = (await edges()).replies
check((await api('POST', `/api/conversations/${H}/prompt`, { text: 'drip 8 250' })).status === 202, 'following, a streaming reply is asked for')
await page.locator('.events .message.streaming').waitFor()
await wheel(-300)
const placed = await firstInView()
await until('the streamed reply landed', async () => (await edges()).replies >= repliesBefore + 8 && !(await convOf(H)).turn_active, 30000)
const stayed = await topOf(placed.seq)
const newerShown = await page.locator('.newer button').isVisible()
check(
  Math.abs(stayed - placed.top) <= 0.5 && (await fromBottom()) > 32 && newerShown,
  `the reader's wheel during a streaming reply keeps the view where they put it: row ${placed.seq} at ${placed.top.toFixed(1)}, then ${stayed.toFixed(1)}; ${Math.round(await fromBottom())}px from the bottom; newer activity ${newerShown ? 'offered' : 'not offered'}`,
)

// 7. while following, the view is moved up during a streaming reply with no
// input event at all, by setting the position from the page, as the find
// bar does, or a scrollbar the page cannot see. any move up that is not
// mega's own ends following, so the view stays where it was put.
await scroller.evaluate((m) => {
  m.scrollTop = m.scrollHeight
})
await until('following before the second stream', async () => (await fromBottom()) <= 1)
const repliesBefore2 = (await edges()).replies
check((await api('POST', `/api/conversations/${H}/prompt`, { text: 'drip 8 250' })).status === 202, 'following, another streaming reply is asked for')
await page.locator('.events .message.streaming').waitFor()
await scroller.evaluate((m) => {
  m.scrollTop -= 300
})
await settled()
const put = await firstInView()
await until('the second streamed reply landed', async () => (await edges()).replies >= repliesBefore2 + 8 && !(await convOf(H)).turn_active, 30000)
const kept = await topOf(put.seq)
check(
  Math.abs(kept - put.top) <= 0.5 && (await fromBottom()) > 32 && (await page.locator('.newer button').isVisible()),
  `a move up with no input event during a streaming reply keeps the view where it was put: row ${put.seq} at ${put.top.toFixed(1)}, then ${kept.toFixed(1)}; ${Math.round(await fromBottom())}px from the bottom`,
)

// ---- the focus and the keys that move it ----
// opening a conversation, however it is opened, puts the cursor in its reply
// box, at one line: picked in the side panel or the needs-you list, by
// Ctrl+/, launched, adopted, or the page loaded on it (above). never while a
// dialog is open, and never from a field being typed in. Ctrl+Space puts the
// cursor there from anywhere; Escape leaves it for the history, whose keys
// then scroll; Page Up and Page Down read the history from an empty box.
const heading = () => page.locator('.conversation-head h1').textContent()
const atRest = async (what) => {
  await until(`the cursor in the reply box after ${what}`, focused)
  await settled()
  return !(await expanded()) && (await height()) === resting
}
const W = (await t.launch(P.id, 'claude', 'ask', { name: 'waiter' })).id
await until('waiter asks', async () => (await convOf(W)).pending.length === 1)
await go(H)
await until('H shown again', async () => (await heading()) === 'dock')
await outside()
const inProject = (name) => page.locator('.tree-children .tree-item', { has: page.locator('.tree-label', { hasText: new RegExp(`^${name}$`) }) })
await inProject('waiter').click()
await until('waiter shown from the side panel', async () => (await heading()) === 'waiter')
check(await atRest('the side panel'), 'picking a conversation in the side panel puts the cursor in its reply box, at one line')
await go(H)
await until('H shown for the needs-you list', async () => (await heading()) === 'dock')
await outside()
await page.locator('.needs-you .tree-item', { hasText: 'waiter' }).click()
await until('waiter shown from the needs-you list', async () => (await heading()) === 'waiter')
check(await atRest('the needs-you list'), 'picking it in the needs-you list does the same')
await go(H)
await until('H shown for Ctrl+/', async () => (await heading()) === 'dock')
await outside()
await page.keyboard.press('Control+Slash')
await until('Ctrl+/ moved the selection', async () => (await heading()) !== 'dock')
check(await atRest('Ctrl+/'), `Ctrl+/ does the same, on "${await heading()}"`)
// launching, from the project page's dialog.
await page.evaluate((p) => {
  location.hash = `#/p/${p}`
}, P.id)
await page.locator('.section-actions button[aria-label="New conversation"]').click()
const launchDialog = page.locator('dialog[open]')
await launchDialog.waitFor()
await launchDialog.getByRole('textbox', { name: 'Opening prompt' }).fill('launched for the focus')
await launchDialog.getByRole('button', { name: 'Launch' }).click()
await until('the launched conversation shown', async () => (await heading()) === 'launched for the focus')
check(await atRest('launching'), 'launching puts the cursor in the new conversation’s reply box')
const launchedForFocus = (await t.workspace()).conversations.find((c) => c.name === 'launched for the focus').id
await t.idle(launchedForFocus)
// adopting: a session left by a conversation launched, released and removed.
const left = (await t.launch(P.id, 'claude', 'a session to adopt for the focus')).id
await t.idle(left)
await api('POST', `/api/conversations/${left}/release`)
await until('the session to adopt released', async () => (await convOf(left)).state === 'none')
must((await api('DELETE', `/api/conversations/${left}`)).status === 204, 'the conversation holding the session to adopt is removed')
await page.evaluate((p) => {
  location.hash = `#/p/${p}`
}, P.id)
await page.getByRole('button', { name: /Adopt a session/ }).click()
const adoptDialog = page.locator('dialog[open].adopt-dialog')
await until('the session to adopt listed', async () => (await adoptDialog.locator('label.session-row', { hasText: 'a session to adopt for the focus' }).count()) === 1)
await adoptDialog.locator('label.session-row', { hasText: 'a session to adopt for the focus' }).click()
await adoptDialog.getByRole('textbox', { name: 'Name' }).fill('adopted for the focus')
await adoptDialog.getByRole('button', { name: 'Adopt' }).click()
await until('the adopted conversation shown', async () => (await heading()) === 'adopted for the focus')
check(await atRest('adopting'), 'adopting puts the cursor in the adopted conversation’s reply box')

// never while a dialog is open. Ctrl+/ does nothing there, so nothing is
// opened, and marked read, behind it; a switch by the address, the one way
// left, leaves the focus in the dialog; and so does Ctrl+Space. and nothing
// is read behind a dialog: a conversation switched to by the address, or
// given output while it is open, stays unread until the last dialog closes,
// and is read then.
const inDialog = () => page.evaluate(() => document.activeElement?.closest('dialog[open]') !== null)
const unread = async (id) => (await convOf(id)).attention === 'unread'
await go(H)
await until('H shown for the dialog', async () => (await heading()) === 'dock')
check((await api('POST', `/api/conversations/${C}/prompt`, { text: 'echo\nC has news while H is open' })).status === 202, 'C, not open, is given output')
await t.idle(C)
await until('C unread', () => unread(C))
must((await t.workspace()).needs_you.length > 0, 'something waits, for Ctrl+/ to move to')
await page.locator('.side-foot .version-label').click()
await page.locator('dialog[open]').waitFor()
await page.keyboard.press('Control+Slash')
await page.waitForTimeout(800)
check((await heading()) === 'dock' && page.url().endsWith(`#/c/${H}`) && (await inDialog()), 'with a dialog open, Ctrl+/ does nothing: the selection and the focus stay put')
await go(C)
await until('a switch behind the dialog', async () => (await heading()) !== 'dock')
await settled()
check(await inDialog(), `a switch by the address behind the dialog (to "${await heading()}") leaves the focus in the dialog`)
await page.keyboard.press('Control+Space')
await settled()
check(await inDialog(), 'and Ctrl+Space does nothing while a dialog is open')
// the page has C's history, loaded and current, behind the dialog.
const replyShown = (text) => page.locator('.events .message.assistant[data-seq]', { hasText: text }).isVisible()
await until('C’s output shown behind the dialog', () => replyShown('C has news while H is open'))
await page.waitForTimeout(800)
check(await unread(C), 'switched to by the address behind a dialog, its output shown behind it, C stays unread while the dialog is open')
await page.keyboard.press('Escape')
await until('the dialog closed', async () => (await page.locator('dialog[open]').count()) === 0)
await until('C read once the dialog closed', async () => !(await unread(C)))
check(true, 'and once the dialog closes, C is read')
// output arriving in the open conversation while a dialog is open.
await page.locator('.side-foot .version-label').click()
await page.locator('dialog[open]').waitFor()
check((await api('POST', `/api/conversations/${C}/prompt`, { text: 'echo\nnews while the dialog is open' })).status === 202, 'C, open behind a dialog, is given output')
await t.idle(C)
await until('the new output shown behind the dialog', () => replyShown('news while the dialog is open'))
await page.waitForTimeout(800)
check(await unread(C), 'output arriving in the open conversation while a dialog is open stays unread')
await page.keyboard.press('Escape')
await until('the dialog closed again', async () => (await page.locator('dialog[open]').count()) === 0)
await until('C read once the dialog closed again', async () => !(await unread(C)))
check(true, 'and is read once the dialog closes')

// a read waits for the view: output arriving while the view is scrolled up,
// "Newer activity below" showing, stays unread, and is read once the view is
// back at the newest output; the same when a dialog closes over a view still
// scrolled up. a read that fails is tried again, after a wait.
const offered = () => page.locator('.newer button').isVisible()
const outputFor = async (text) => {
  check((await api('POST', `/api/conversations/${C}/prompt`, { text: `echo\n${text}` })).status === 202, `C is given output: "${text}"`)
  await t.idle(C)
  await until(`"${text}" on the page`, () => replyShown(text))
}
await scroller.evaluate((m) => {
  m.scrollTop = m.scrollHeight
})
await until('C at the bottom and read', async () => (await fromBottom()) <= 1 && !(await unread(C)))
await wheel(-600)
await outputFor('news below the view')
await until('newer activity offered', offered)
await page.waitForTimeout(800)
check(await unread(C), 'scrolled up, with "Newer activity below" showing, new output stays unread')
await page.locator('.newer button').click()
await until('read on return to the newest output', async () => !(await unread(C)))
check(true, 'back at the newest output, it is read')
await wheel(-600)
await page.locator('.side-foot .version-label').click()
await page.locator('dialog[open]').waitFor()
await outputFor('news under a dialog, below the view')
await page.keyboard.press('Escape')
await until('the dialog closed over the scrolled-up view', async () => (await page.locator('dialog[open]').count()) === 0)
await page.waitForTimeout(800)
check(await unread(C), 'a dialog closing over a view still scrolled up reads nothing')
await scroller.evaluate((m) => {
  m.scrollTop = m.scrollHeight
})
await until('read at the bottom after the dialog', async () => !(await unread(C)))
check(true, 'and the view brought back to the newest output reads it')
// the one read the return makes fails, once; the page tries again.
await wheel(-600)
await outputFor('news whose first read fails')
const readCalls = []
await page.route('**/api/conversations/*/read', (route) => {
  readCalls.push(Date.now())
  return readCalls.length === 1
    ? route.fulfill({ status: 500, contentType: 'application/json', body: '{"code":"internal","message":"refused by the check"}' })
    : route.continue()
})
await page.locator('.newer button').click()
await until('the first read refused', async () => readCalls.length >= 1)
await page.waitForTimeout(500)
check(await unread(C), 'a read that fails leaves the conversation unread')
await until('the failed read tried again and read', async () => !(await unread(C)))
await page.waitForTimeout(800)
const retryAfter = readCalls.length === 2 ? readCalls[1] - readCalls[0] : 0
check(readCalls.length === 2 && retryAfter >= 1500, `a read that fails once is tried again, once, after a wait: ${readCalls.length} reads, ${retryAfter}ms apart`)
await page.unroute('**/api/conversations/*/read')
// never from a field being typed in: one the check adds to the page, outside
// mega's own, stands in for any.
await page.evaluate(() => {
  const field = document.createElement('input')
  field.id = 'check-field'
  field.setAttribute('aria-label', 'a field the check adds')
  document.body.append(field)
})
await page.locator('#check-field').fill('typing')
const from = await heading()
await go(from === 'dock' ? C : H)
await until('a switch while typing', async () => (await heading()) !== from)
await settled()
check(await page.evaluate(() => document.activeElement?.id === 'check-field'), 'a switch never takes the focus from a field being typed in')
await page.evaluate(() => document.getElementById('check-field').remove())

// Ctrl+Space from the history and from the side panel; with the cursor
// already in the box it does nothing, the caret where it was.
await go(H)
await until('H shown for Ctrl+Space', async () => (await heading()) === 'dock')
await outside()
check(await page.evaluate(() => document.activeElement === document.querySelector('.main')), 'a press in the history gives it the focus')
await page.keyboard.press('Control+Space')
await until('Ctrl+Space from the history', focused)
check(!(await expanded()), 'Ctrl+Space from the history puts the cursor in the reply box, at one line')
await inProject('waiter').focus()
await page.keyboard.press('Control+Space')
await until('Ctrl+Space from the side panel', focused)
check((await heading()) === 'dock', 'Ctrl+Space from the side panel does the same, and switches nothing')
await box.fill('abc')
await box.evaluate((b) => b.setSelectionRange(1, 1))
await page.keyboard.press('Control+Space')
await settled()
check((await box.inputValue()) === 'abc' && (await box.evaluate((b) => b.selectionStart)) === 1, 'with the cursor already there, Ctrl+Space does nothing: the draft and caret as they were')

// Escape leaves the box for the history, its draft kept, and the keys then
// scroll the history; with the path list open, it closes that first.
await scroller.evaluate((m) => {
  m.scrollTop = m.scrollHeight
})
await until('H at the bottom for Escape', async () => (await fromBottom()) <= 1)
await page.keyboard.press('Escape')
await until('Escape left the box', async () => !(await focused()))
check(
  (await page.evaluate(() => document.activeElement === document.querySelector('.main'))) && !(await expanded()) && (await box.inputValue()) === 'abc',
  'Escape leaves the box for the history: the box closes, its draft kept',
)
const beforeKey = await scroller.evaluate((m) => m.scrollTop)
await page.keyboard.press('PageUp')
await until('the history scrolled by its own key', async () => (await scroller.evaluate((m) => m.scrollTop)) < beforeKey)
check(true, 'and the keys then scroll the history')
await box.click()
await box.fill('look @')
const pathList = page.locator('.path-list')
await until('the path list open', async () => await pathList.isVisible())
await page.keyboard.press('Escape')
await until('the path list closed', async () => !(await pathList.isVisible()))
check(await focused(), 'with the path list open, Escape closes the list first, the cursor still in the box')
await page.keyboard.press('Escape')
await until('then leaves', async () => !(await focused()))
check(true, 'and a second Escape leaves the box')
await box.fill('')
await outside()

// Page Up and Page Down read the history from an empty box, and that scroll
// is the reader's own: it ends following, so a reply arriving does not move
// the view. with a draft they are the box's.
await scroller.evaluate((m) => {
  m.scrollTop = m.scrollHeight
})
await until('H at the bottom for the page keys', async () => (await fromBottom()) <= 1)
await page.keyboard.press('Control+Space')
await until('the cursor back for the page keys', focused)
const bottomTop = await scroller.evaluate((m) => m.scrollTop)
await page.keyboard.press('PageUp')
await until('Page Up scrolled the history', async () => (await scroller.evaluate((m) => m.scrollTop)) < bottomTop - 100)
await settled()
const pagedTo = await firstInView()
const pagedReplies = (await edges()).replies
check((await api('POST', `/api/conversations/${H}/prompt`, { text: 'drip 4 150' })).status === 202, 'paged up from an empty box, a reply is asked for')
await until('the paged-up reply landed', async () => (await edges()).replies >= pagedReplies + 4 && !(await convOf(H)).turn_active, 30000)
const pagedStayed = await topOf(pagedTo.seq)
check(
  (await focused()) && Math.abs(pagedStayed - pagedTo.top) <= 0.5 && (await page.locator('.newer button').isVisible()),
  `Page Up from an empty box scrolls the history and ends following, as a wheel does: row ${pagedTo.seq} at ${pagedTo.top.toFixed(1)}, then ${pagedStayed.toFixed(1)}, newer activity offered`,
)
const upTop = await scroller.evaluate((m) => m.scrollTop)
await page.keyboard.press('PageDown')
await until('Page Down scrolled the history', async () => (await scroller.evaluate((m) => m.scrollTop)) > upTop)
check(await focused(), 'Page Down scrolls it back down, the cursor staying in the box')
// with a draft the keys are the box's: the page leaves them to the browser,
// which moves the caret (and, as before, may pass a key the box cannot use
// on to the history).
await box.fill(Array.from({ length: 30 }, (_, i) => `draft line ${i + 1}`).join('\n'))
await box.evaluate((b) => b.setSelectionRange(b.value.length, b.value.length))
const draftEnd = await box.evaluate((b) => b.value.length)
await page.keyboard.press('PageUp')
await settled()
const caretUp = await box.evaluate((b) => b.selectionStart)
check(caretUp < draftEnd, `with a draft, Page Up is the box’s: its caret moved up, from ${draftEnd} to ${caretUp}`)
await box.fill('')
await outside()

// the allow keys, as before: with the cursor put in the box by a switch,
// Ctrl+Enter allows the oldest docked approval; from the history, where
// Escape leaves it, it allows as from the page itself.
const pendingOf = async (id) => (await convOf(id)).pending.length
// the keys approve only what the fake raised: exactly one permission request,
// Bash running echo fake, as a step the suite cannot go past.
const isTheFakesAsk = async (id) => {
  const listed = (await convOf(id)).pending
  return listed.length === 1 && listed[0].kind === 'permission' && listed[0].payload?.tool === 'Bash' && listed[0].payload?.input?.command === 'echo fake'
}
await inProject('waiter').click()
await until('waiter shown for the allow keys', async () => (await heading()) === 'waiter' && (await focused()))
await page.waitForTimeout(800)
must(await isTheFakesAsk(W), 'the request the keys will allow is the fake’s own: Bash, echo fake')
await page.keyboard.press('Control+Enter')
await until('the switch’s Ctrl+Enter allowed', async () => (await pendingOf(W)) === 0)
check(true, 'with the cursor put in the box by a switch, Ctrl+Enter allows the oldest docked approval')
await t.idle(W)
check((await api('POST', `/api/conversations/${W}/prompt`, { text: 'ask' })).status === 202, 'waiter asks again')
await until('waiter docked again', async () => (await page.locator('.panel-cluster .docked section.request-card').count()) === 1)
await box.click()
await page.keyboard.press('Escape')
await until('the history focused for the allow keys', async () => page.evaluate(() => document.activeElement === document.querySelector('.main')))
await page.waitForTimeout(800)
must(await isTheFakesAsk(W), 'again, the request the keys will allow is the fake’s own: Bash, echo fake')
await page.keyboard.press('Control+Enter')
await until('the history’s Ctrl+Enter allowed', async () => (await pendingOf(W)) === 0)
check(true, 'from the history, where Escape leaves the cursor, Ctrl+Enter allows as from the page itself')
await t.idle(W)

// @~/ searches the directory named beyond the checkout, ~/Sandbox: the list
// shows its paths with ~, newest first, and none in a dot-directory or
// outside it; a pick is written as the harness reliably resolves it
// (capability report, 2026-11-16): for claude a mention as the list shows
// it, quoted for a space; for codex and pi the bare absolute path.
const home = join(t.dir, 'home')
const sandbox = join(home, 'Sandbox')
const made = (rel, minutesAgo) => {
  const full = join(sandbox, rel)
  mkdirSync(join(full, '..'), { recursive: true })
  writeFileSync(full, rel)
  const when = new Date(Date.now() - minutesAgo * 60000)
  utimesSync(full, when, when)
}
made('older notes.txt', 30)
made('shot one.png', 2)
// newer, but deeper: the directory's own files come first.
made('shots/deep.png', 1)
made('.cache/hidden.png', 0)
mkdirSync(join(home, 'Private'), { recursive: true })
writeFileSync(join(home, 'Private', 'secret.txt'), 'not named')
const list = page.locator('.path-list')
const options = () => list.getByRole('option').allTextContents()
const pickNewest = async (id) => {
  await page.evaluate((id) => {
    location.hash = `#/c/${id}`
  }, id)
  await box.waitFor()
  await box.click()
  await box.fill('look at @~/')
  await until('the list of the named directory', async () => (await list.getByRole('option').count()) > 0)
  const shown = await options()
  const label = await list.getByRole('listbox').getAttribute('aria-label')
  await box.press('Enter')
  const value = await box.inputValue()
  await box.fill('')
  return { shown, label, value }
}
const atClaude = await pickNewest(C)
t.say(`  @~/ for claude: ${JSON.stringify(atClaude)}`)
check(
  atClaude.label === 'Paths in the directories named beyond the checkout' &&
    atClaude.shown.slice(0, 3).join(',') === '~/Sandbox/shot one.png,~/Sandbox/older notes.txt,~/Sandbox/shots/deep.png' &&
    atClaude.shown.includes('~/Sandbox/') &&
    !atClaude.shown.some((p) => p.includes('.cache') || p.includes('Private')),
  `@~/ opens the list of ~/Sandbox, shown with ~, its own files newest first and then deeper ones, with no dot-directory and nothing outside it: ${JSON.stringify(atClaude.shown)}`,
)
check(atClaude.value === 'look at @"~/Sandbox/shot one.png" ', `for claude, the pick is a mention as the list shows it, quoted for its space: "${atClaude.value}"`)
await box.fill('look at @~/Sa')
await until('the list again', async () => (await list.getByRole('option').count()) > 0)
await list.screenshot({ path: `${t.dir}/shots/path-search-outside.png` })
await box.fill('look at @~/Private')
await until('the search settled', async () => (await list.locator('.path-note').count()) === 1)
check((await list.locator('.path-note').textContent()) === 'No matching paths', 'a directory not named is never listed, however it is typed')
// the second named directory, looked in directly: its files, newest first,
// whatever the first holds.
for (const [name, minutesAgo] of [
  ['first.png', 20],
  ['second.png', 10],
  ['third.png', 5],
]) {
  const full = join(home, 'Screenshots', name)
  mkdirSync(join(home, 'Screenshots'), { recursive: true })
  writeFileSync(full, name)
  const when = new Date(Date.now() - minutesAgo * 60000)
  utimesSync(full, when, when)
}
await box.fill('look at @~/Screenshots/')
await until('the second directory looked in', async () => (await options()).includes('~/Screenshots/third.png'))
const second = await options()
check(
  second.join(',') === '~/Screenshots/third.png,~/Screenshots/second.png,~/Screenshots/first.png',
  `@~/Screenshots/ lists that directory's own files, newest first: ${JSON.stringify(second)}`,
)
await list.screenshot({ path: `${t.dir}/shots/path-search-second.png` })
await box.fill('')
for (const [id, name] of [
  [X, 'codex'],
  [PiConv, 'pi'],
]) {
  const picked = await pickNewest(id)
  check(picked.value === `look at ${join(sandbox, 'shot one.png')} `, `for ${name}, the pick is the bare absolute path: "${picked.value}"`)
}

await t.done()

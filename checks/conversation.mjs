// browser check: the conversation view on long conversations, against the
// fakes. it is the start of the conversation view suite (the recent tail);
// the rest of that surface (structure, markdown, copy) is rebuilt when a batch
// next changes it.
//
//   make checks SUITE=conversation
//
// two conversations are launched on the fake claude and released, the
// service is stopped, and lib/gen.mjs extends each log to about twenty
// thousand events; B's log then ends in a native event the page is never
// sent. what it checks: opening reads only the tail, at the newest reply, ten
// turns, saying how much is earlier, and moves the reading position; Show
// earlier adds ten turns holding the view; the native toggle refetches what
// is shown and hiding fetches nothing; Show all loads everything holding the
// view; a log ending in a native event is read through it and goes idle; a
// streamed reply keeps the rendered rows at the tail with no long task; a
// dropped stream names what it waits for in the footer, and the reconnect
// reads only what was missed; a held, then failed, opening read names the
// request, its time, the attempt and the error, and lands on the retry; the
// connecting view names a failing workspace read. and the column: the
// conversation's 960px column centred in its pane, with the panel open and
// collapsed to the rail, its header, history, input panel, show-earlier bar,
// newer-activity control and the shutdown banner sharing it; the project page
// and the connecting view centred the same way; and the labels, none on the
// operator's own prompts, "outside mega" on one from outside, "From outside
// mega" on its reply. and each turn's line: hidden at rest, shown on hover
// and on focus with nothing moving, staying in the notice colour when the
// fake reports another model than the one asked for; after what it ran with,
// when its prompt was sent and how long it took, the turn in flight when it
// started, and a turn recorded before mega kept what it ran with its time
// alone, with its date from another day; each prompt's time as its title,
// "Imported" on one from outside; every time in the service's zone while
// the page reports UTC. and a pi conversation: pi's own words about
// how it asks under its header, its prompt, tool call and reply as the
// others', the prompt's bookkeeping hidden, its gated tool call waiting on
// the dialog rather than running, and its turn line with no mode. and
// actions, for each harness's fake: a failed one marked while collapsed;
// expanded, the whole invocation, past the one line's cut, then the result;
// another tool's input field by field; the copy control; an action
// recorded with nothing its adapter reads, showing its one line; and a web
// search the fake codex reports for the turn before while the next runs,
// with no spinner and not named on the activity line, as it is not that
// turn's, and a message of the turn before, and text streamed for it,
// leaving the next turn's streaming reply be. it saves
// each harness's expanded command and input as
// shots/action-{claude,codex,pi}{,-input}.png.
// and the runtime-started line: unmarked, with
// no added words, for a version inside the tested range and for one newer,
// the normal state; marked in the notice colour for one older, the range
// after the version and the sentence in its title; and nothing refused. and the MCP servers: an item in the More menu with the
// count, a mark on More while a server failed or needs signing in, in view at
// the bottom, and the list the item opens, with each
// status, an error, the harness's own place to sign in, tools on expansion,
// and nothing to change; a sign-in followed; never attention; nothing with no
// runtime; for claude and for codex. it saves the centred conversation, light
// and dark, with the panel open and collapsed, and the start lines,
// shots/start-{tested,newer,older}.png, and the pi conversation,
// shots/pi-conversation.png.
import { readFileSync, rmSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { seedExchange } from '../shim/claude/fake/sessionStore.mjs'
import { appendImported, appendNative, appendTurns } from './lib/gen.mjs'
import { suite } from './lib/mega.mjs'

// the service runs in one time zone and the pages in another, UTC, as a
// hardened browser reports; every time the page says is the service's.
const serviceZone = 'Asia/Kolkata'
const t = await suite('conversation', { tz: serviceZone })
const { url, port, check, until, api, convOf, workspace } = t

// two conversations, launched and released, then made long while the service
// is stopped.
const P = await t.register('long', t.checkout('long'))
const A = (await t.launch(P.id, 'claude', 'echo\nhello')).id
const B = (await t.launch(P.id, 'claude', 'echo\nhello')).id
const C = (await t.launch(P.id, 'claude', 'echo\nhello', { name: 'imported' })).id
const gap = await t.launch(P.id, 'claude', 'echo\nmanaged before the gap', { name: 'handoff gap' })
const unknown = await t.launch(P.id, 'claude', 'echo\nunknown session', { name: 'unknown session' })
const legacy = await t.launch(P.id, 'claude', 'echo\nlegacy completed', { name: 'legacy loss' })
for (const id of [A, B, C, gap.id]) {
  await t.idle(id)
  await api('POST', `/api/conversations/${id}/release`)
}
await until('all released', async () => (await convOf(A)).state === 'none' && (await convOf(B)).state === 'none' && (await convOf(C)).state === 'none' && (await convOf(gap.id)).state === 'none')
await t.stop()
const logOf = (id) => join(t.dir, 'data', 'conversations', id, 'events.jsonl')
const terminalPrompt = 'terminal prompt with attachment'
const terminalReply = 'terminal reply during handoff'
const nativeHistory = seedExchange(join(t.dir, 'fakeshim'), gap.native_id, {
  prompt: terminalPrompt,
  reply: terminalReply,
  attachment: true,
})

const readLog = (id) => readFileSync(logOf(id), 'utf8').trim().split('\n').map(JSON.parse)
const writeLog = (id, rows) => writeFileSync(logOf(id), rows.map((ev) => JSON.stringify(ev) + '\n').join(''))
const registry = join(t.dir, 'data/registry.yaml')
function rewriteConversation(id, rewrite) {
  const doc = readFileSync(registry, 'utf8')
  const rows = doc.split(/(?=^[ \t]*- )/m)
  const index = rows.findIndex((row) => new RegExp(`^[ \t]+id: ['"]?${id}['"]?$`, 'm').test(row))
  if (index < 0) throw new Error(`could not find scratch conversation ${id}`)
  rows[index] = rewrite(rows[index])
  writeFileSync(registry, rows.join(''))
}
const loseBinding = (row) => row.replace(/^[ \t]*native_id: .*\n/m, '')
rewriteConversation(gap.id, loseBinding)
rewriteConversation(unknown.id, (row) => loseBinding(row).replace(/runtime: none/, 'runtime: running').replace(/output_seq: \d+/, 'output_seq: 0'))
writeLog(unknown.id, readLog(unknown.id).filter((ev) => ev.status === 'starting'))
const prior = readLog(legacy.id).filter((ev) => ev.kind !== 'runtime' || ev.status !== 'stopped' && ev.status !== 'stopping')
const past = Date.now() - 3600_000
const legacyEvents = [
  { kind: 'user', text: 'prompt before supervision was lost', origin: 'managed', at: new Date(past).toISOString() },
  { kind: 'assistant', text: 'reply before supervision was lost', origin: 'managed', at: new Date(past + 1000).toISOString() },
  { kind: 'runtime', status: 'unsupervised', text: 'legacy loss record', origin: 'service', at: new Date(past + 2000).toISOString() },
]
writeLog(legacy.id, [...prior, ...legacyEvents.map((ev, i) => ({ ...ev, seq: prior.at(-1).seq + i + 1 }))])
rewriteConversation(legacy.id, (row) => row.replace(/runtime: none/, 'runtime: unsupervised'))
const genA = appendTurns(logOf(A), 20000, 1)
const genB = appendTurns(logOf(B), 20000, 2)
appendNative(logOf(B))
appendImported(logOf(C))
t.say(`generated ${genA.seq} events in ${genA.turns} turns, and ${genB.seq} in ${genB.turns}`)
await t.start()

const page = await t.page({ width: 1440, height: 900, timezoneId: 'UTC' })
// inZone formats a time as the page does, in a zone: the service's unless
// another is named.
const inZone = (at, options, zone = serviceZone) =>
  page.evaluate(([at, options, zone]) => new Intl.DateTimeFormat([], { ...options, timeZone: zone }).format(new Date(at)), [at, options, zone])
const minute = { hour: '2-digit', minute: '2-digit' }
const toSecond = { weekday: 'short', year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit', timeZoneName: 'short' }
// lasted is how long a turn took, as the activity line says it.
const lasted = (ms) => {
  const s = Math.floor(ms / 1000)
  if (s < 1) return 'under 1s'
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  return m < 60 ? `${m}m ${s % 60}s` : `${Math.floor(m / 60)}h ${m % 60}m`
}
// turnTimes is when each ended turn of a conversation's newest ten was sent,
// its prompt's time, and when it ended.
const turnTimes = async (id) => {
  const evs = (await api('GET', `/api/conversations/${id}/events?turns=10`)).body.events
  const out = []
  let sent
  for (const e of evs) {
    if (e.kind === 'user' && !sent) sent = e.at
    if (e.kind === 'turn_end') {
      if (sent) out.push({ sent, ended: e.at })
      sent = undefined
    }
  }
  return out
}
const reads = []
page.on('request', (r) => {
  if (r.url().includes('/events')) reads.push(r.url().replace(url, ''))
})
const rows = () =>
  page.evaluate(() => {
    const rs = [...document.querySelectorAll('.events [data-seq]')].map((e) => Number(e.dataset.seq))
    return { count: rs.length, first: rs[0], last: rs.at(-1) }
  })
const inView = (n) => {
  const h = [...document.querySelectorAll('h3,h4,h5,h6')].find((e) => e.textContent === n)
  if (!h) return false
  const r = h.getBoundingClientRect()
  return r.top >= 0 && r.bottom <= window.innerHeight
}
const earlierText = () => page.locator('.earlier .muted').first().textContent()
const firstInView = () =>
  page.evaluate(() => {
    const e = [...document.querySelectorAll('.events [data-seq]')].find((x) => x.getBoundingClientRect().bottom > 0)
    return { seq: e.dataset.seq, top: e.getBoundingClientRect().top }
  })
const topOf = (seq) => page.evaluate((s) => document.querySelector(`.events [data-seq="${s}"]`).getBoundingClientRect().top, seq)

// open A: the tail, at the newest activity.
await page.goto(`${url}/#/c/${A}`)
await page.waitForFunction(inView, `turn ${genA.turns}`, { timeout: 30000, polling: 50 })
const tailA = (await api('GET', `/api/conversations/${A}/events?turns=10`)).body
check(reads.length === 1 && /events\?turns=10$/.test(reads[0]), `opening reads only the tail: ${reads.join(', ')}`)
let r = await rows()
check(r.first >= tailA.from && r.count < 400, `only the tail is rendered: ${r.count} rows from seq ${r.first} (tail from ${tailA.from})`)
check((await earlierText()).startsWith(`${tailA.earlier.turns} earlier turns`), `the page says how much is earlier: "${await earlierText()}"`)
check((await page.locator('.events .message.user').count()) === 10, 'ten turns shown')
await until('A read through the tail', async () => (await convOf(A)).read_seq === tailA.through)
check(true, `the reading position follows the newest event given: ${tailA.through}`)
// those turns have no record of what they ran with, so their lines have
// their time alone; generated a week ago, each has its date.
const genTimes = await turnTimes(A)
const genLast = genTimes.at(-1)
const genLine = (await page.locator('.events .turn-end:not(.pending) .turn-line').last().textContent()) ?? ''
const genWant = `Sent ${await inZone(genLast.sent, { month: 'short', day: 'numeric' })} ${await inZone(genLast.sent, minute)} · took ${lasted(Date.parse(genLast.ended) - Date.parse(genLast.sent))}`
check(
  (await page.locator('.events .turn-end').count()) >= 10 &&
    (await page.locator('.events .turn-line').count()) === (await page.locator('.events .turn-end:not(.pending)').count()) &&
    genLine === genWant &&
    (await inZone(genLast.sent, minute, 'UTC')) !== (await inZone(genLast.sent, minute)),
  `turns with no record of what they ran with still show their time, from another day with its date, in the service's zone: "${genLine}"`,
)
await t.shot(page, 'tail')

// the column: 960 pixels, in the middle of its pane, and everything in the
// conversation shares it. it re-centres when the panel collapses to the rail.
const column = (p, sel) =>
  p.evaluate((s) => {
    const main = document.querySelector('.main').getBoundingClientRect()
    const pane = document.querySelector('.main').clientWidth
    const e = document.querySelector(s).getBoundingClientRect()
    return { left: Math.round(e.left - main.left), right: Math.round(main.left + pane - e.right), width: Math.round(e.width), x: Math.round(e.left), end: Math.round(e.right) }
  }, sel)
const centred = (c) => Math.abs(c.left - c.right) <= 1
const open = await column(page, '.conversation')
check(open.width === 960 && centred(open), `the conversation's column is 960px, centred in its pane: ${open.left}px either side`)
const inColumn = async (sel) => {
  const c = await column(page, sel)
  const conv = await column(page, '.conversation')
  return c.x === conv.x && c.end === conv.end
}
check((await inColumn('.conversation-head')) && (await inColumn('.events')) && (await inColumn('.panel-cluster')) && (await inColumn('.earlier')), 'the header, the history, the input panel and the show-earlier bar share the column')
await t.shot(page, 'centred-light-panel')
await page.getByRole('button', { name: /^Collapse the side panel/ }).click()
await page.locator('nav.rail').waitFor()
await until('re-centred', async () => {
  const c = await column(page, '.conversation')
  return centred(c) && c.left > open.left
})
const railed = await column(page, '.conversation')
check(railed.width === 960, `collapsed to the rail, the column re-centres in the wider pane: ${railed.left}px either side`)
await t.shot(page, 'centred-light-rail')
await page.getByRole('button', { name: /^Expand the side panel/ }).click()
await page.locator('nav.sidebar').waitFor()
// and the banners that span it: the shutdown gate's.
check((await api('POST', '/api/shutdown/arm')).status === 202, 'a shutdown armed, for its banner')
await page.locator('.gate-banner').waitFor()
check(await inColumn('.gate-banner'), 'the shutdown banner spans the same column')
await api('POST', '/api/shutdown/cancel')
await until('the banner gone', async () => (await page.locator('.gate-banner').count()) === 0)

// toTop scrolls to the top of what is shown as a reader does, with the
// mouse wheel: only the reader ends following, and a view still following
// the bottom would be put back there at the next change of the dock.
const toTop = async () => {
  const box = await page.locator('.main').boundingBox()
  await page.mouse.move(box.x + box.width / 2, box.y + 80)
  await page.mouse.wheel(0, -1e7)
  await until('the view at the top', async () => (await page.evaluate(() => document.querySelector('.main').scrollTop)) === 0)
  await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))))
}

// show earlier: ten more turns above, the view held.
await toTop()
const anchor = await firstInView()
reads.length = 0
await page.getByRole('button', { name: 'Show the ten turns before these' }).click()
await until('ten more prompts', async () => (await page.locator('.events .message.user').count()) === 20)
const moved = await topOf(anchor.seq)
check(Math.abs(moved - anchor.top) <= 2, `show earlier holds the view: row ${anchor.seq} at ${Math.round(anchor.top)} then ${Math.round(moved)}`)
check(reads.length === 1 && reads[0].includes(`before=${tailA.from}`) && reads[0].includes('turns=10'), `show earlier reads the ten turns before: ${reads.join(', ')}`)
check((await earlierText()).startsWith(`${tailA.earlier.turns - 10} earlier turns`), `the count drops by ten: "${await earlierText()}"`)

// the native toggle refetches what is shown, with native events.
const shownFrom = (await rows()).first
reads.length = 0
await page.getByRole('button', { name: 'More' }).click()
await page.getByRole('menuitemcheckbox', { name: 'Native events' }).click()
await until('native rows', async () => (await page.locator('.events details.native').count()) > 0)
check(reads.length === 1 && reads[0].includes('native=1') && reads[0].includes('since='), `showing native events refetches what is shown: ${reads.join(', ')}`)
reads.length = 0
await page.getByRole('button', { name: 'More' }).click()
await page.getByRole('menuitemcheckbox', { name: 'Native events' }).click()
await until('no native rows', async () => (await page.locator('.events details.native').count()) === 0)
check(reads.length === 0, `hiding them fetches nothing: ${reads.join(', ')}`)
check((await rows()).first <= shownFrom, 'the stretch shown is unchanged')

// show all.
await toTop()
const anchor2 = await firstInView()
reads.length = 0
await page.getByRole('button', { name: 'Show the whole history' }).click()
await until('the earlier bar goes', async () => (await page.locator('.earlier').count()) === 0, 60000)
const moved2 = await topOf(anchor2.seq)
r = await rows()
check(r.first === 1 && (await page.locator('.events .message.user').count()) === genA.turns + 1, `show all loads everything: ${r.count} rows from seq ${r.first}`)
check(Math.abs(moved2 - anchor2.top) <= 2, `show all holds the view: row ${anchor2.seq} at ${Math.round(anchor2.top)} then ${Math.round(moved2)}`)
check(reads.length === 1 && /events\?before=\d+$/.test(reads[0]), `show all reads everything before: ${reads.join(', ')}`)

// B: a log that ends in a native event. the reading position reaches it, and
// B's unread clears, although the page was never sent that event.
const bBefore = await convOf(B)
check(bBefore.attention === 'unread', `B starts unread (read ${bBefore.read_seq}, last ${bBefore.last_seq})`)
await page.evaluate((id) => {
  location.hash = `#/c/${id}`
}, B)
await page.waitForFunction(inView, `turn ${genB.turns}`, { timeout: 30000, polling: 50 })
await until('B read through its native last event', async () => (await convOf(B)).read_seq === bBefore.last_seq)
check((await convOf(B)).attention === 'idle', `B is read through its native last event ${bBefore.last_seq} and idle`)

// a streamed reply on B re-renders only the loaded tail, without long tasks.
await page.evaluate(() => {
  window.__long = []
  new PerformanceObserver((l) => {
    for (const e of l.getEntries()) window.__long.push(Math.round(e.duration))
  }).observe({ type: 'longtask', buffered: false })
})
const body = Array.from({ length: 60 }, (_, i) => `Line ${i + 1} of a streamed reply, with \`code\` and **bold** words.`).join('\n\n')
const before = (await rows()).count
// read from higher up, so new activity arrives below the view. the view
// leaves the bottom by the reader's own wheel: only the reader ends
// following, so a scripted scroll would not. a scroll event is delivered
// before the next frame, so after two frames the page knows the view has left
// the bottom.
await toTop()
check((await api('POST', `/api/conversations/${B}/prompt`, { text: `stream 25 12\n${body}` })).status === 202, 'a streamed reply is asked for')
await page.locator('.message.streaming').waitFor({ timeout: 20000 })
const newerButton = page.locator('.newer button')
await newerButton.waitFor()
const newerAt = await newerButton.boundingBox()
const columnNow = await column(page, '.conversation')
check(Math.abs(newerAt.x + newerAt.width / 2 - (columnNow.x + columnNow.end) / 2) <= 1, 'the newer-activity control sits in the middle of the column')
await until('the reply lands', async () => (await page.locator('.message.streaming').count()) === 0 && (await page.getByText('Line 60 of a streamed reply').count()) > 0, 60000)
const longTasks = await page.evaluate(() => window.__long)
const after = (await rows()).count
check(after - before < 10 && before < 400, `the history held stays the tail: ${before} rows, then ${after}`)
check(longTasks.filter((d) => d >= 100).length === 0, `streaming caused no long task of 100 ms or more: ${JSON.stringify(longTasks)}`)

// the operator's own prompts carry no label; a prompt and a reply from
// outside mega say so.
await page.evaluate((id) => {
  location.hash = `#/c/${id}`
}, C)
await until('C shown', async () => (await page.locator('.events .message.assistant', { hasText: 'a reply given in a terminal' }).count()) === 1)
const prompts = page.locator('.events .message.user')
const own = prompts.filter({ hasText: 'echo' })
const outside = prompts.filter({ hasText: 'a prompt typed in a terminal' })
const reply = page.locator('.events .message.assistant', { hasText: 'a reply given in a terminal' })
check((await own.locator('.who').count()) === 0 && (await prompts.locator('.who', { hasText: /^you/i }).count()) === 0, "the operator's own prompts carry no label")
check((await outside.locator('.who').textContent()) === 'outside mega' && (await reply.locator('.who').textContent()) === 'From outside mega', 'a prompt from outside mega reads "outside mega", and its reply "From outside mega"')
const outsideTitle = (await outside.getAttribute('title')) ?? ''
check(outsideTitle.startsWith('Imported '), `its title says when mega imported it, not when it was sent: "${outsideTitle}"`)
await t.shot(page, 'outside')

// each turn's line: what it ran with, hidden at rest, shown while the
// pointer is over the turn or focus is inside it, in space the turn already
// has, so nothing moves. a turn that ran with another model than it asked
// for keeps its line, in the notice colour, naming both. the fake's
// fallback verb reports another model for one turn.
const L = (await t.launch(P.id, 'claude', 'echo\nhello', { name: 'turn lines' })).id
await t.idle(L)
await page.evaluate((id) => {
  location.hash = `#/c/${id}`
}, L)
const lineOf = (block) => block.locator('.turn-end:not(.pending) .turn-line')
const firstTurn = page.locator('.events .turn-block').filter({ has: page.locator('.turn-end:not(.pending)') }).first()
const firstLine = lineOf(firstTurn)
await firstLine.waitFor({ state: 'attached' })
const opacity = (loc) => loc.evaluate((e) => getComputedStyle(e).opacity)
// layout is where every row of the history and the reply box stand.
const layout = () =>
  page.evaluate(() =>
    JSON.stringify([...document.querySelectorAll('.events [data-seq], .events .message, form.reply')].map((e) => {
      const r = e.getBoundingClientRect()
      return [Math.round(r.top * 10), Math.round(r.left * 10), Math.round(r.height * 10), Math.round(r.width * 10)]
    })),
  )
// turnShot saves a turn with a margin, since its line overhangs the turn's
// separator into the gap below.
const turnShot = async (block, name) => {
  const b = await block.boundingBox()
  await page.screenshot({ path: `${t.dir}/shots/${name}.png`, clip: { x: b.x - 8, y: b.y - 8, width: b.width + 16, height: b.height + 16 } })
}
const rest = async () => {
  await page.mouse.move(2, 2)
  await page.evaluate(() => document.activeElement instanceof HTMLElement && document.activeElement.blur())
}
await rest()
await until('the line hidden at rest', async () => (await opacity(firstLine)) === '0')
const lineText = await firstLine.textContent()
const [firstTimes] = await turnTimes(L)
const firstWant = `Ran Fake Opus · medium · ask · sent ${await inZone(firstTimes.sent, minute)} · took ${lasted(Date.parse(firstTimes.ended) - Date.parse(firstTimes.sent))}`
check(lineText === firstWant, `a turn carries one line, hidden at rest: what it ran with, when its prompt was sent and how long it took: "${lineText}"`)
check(
  (await inZone(firstTimes.sent, minute, 'UTC')) !== (await inZone(firstTimes.sent, minute)) && (await page.evaluate(() => Intl.DateTimeFormat().resolvedOptions().timeZone)) === 'UTC',
  'the time is the service’s, not the page’s own UTC',
)
const lineTitle = await firstLine.getAttribute('title')
check(
  (lineTitle ?? '').startsWith('Asked for ') && (lineTitle ?? '').endsWith(`. Sent ${await inZone(firstTimes.sent, toSecond)}; ended ${await inZone(firstTimes.ended, toSecond)}`),
  `its title has both times to the second, with the zone: "${lineTitle}"`,
)
const promptTitle = await firstTurn.locator('.message.user').getAttribute('title')
check(promptTitle === `Sent ${await inZone(firstTimes.sent, toSecond)}`, `the operator's prompt carries its time as its title: "${promptTitle}"`)
const atRest = await layout()
await turnShot(firstTurn, 'turn-rest')
await firstTurn.locator('.message.assistant').hover()
await until('the line shown on hover', async () => (await opacity(firstLine)) === '1')
check((await layout()) === atRest, 'shown while the pointer is over the turn, and nothing moves')
await turnShot(firstTurn, 'turn-hover')
await rest()
await until('the line hidden again', async () => (await opacity(firstLine)) === '0')
await firstTurn.locator('.message.assistant .block-copy button').first().focus()
await until('the line shown on focus', async () => (await opacity(firstLine)) === '1')
check((await layout()) === atRest, 'shown while focus is inside the turn, and nothing moves')
await rest()
const reply2 = page.getByRole('textbox', { name: 'Reply' })
await reply2.click()
await reply2.fill('fallback fake-sonnet-1')
await reply2.press('Control+Enter')
await until('the fallback turn ended', async () => {
  const c = await convOf(L)
  return !c.turn_active && (await page.locator('.events .turn-line.differs').count()) === 1
}, 30000)
await rest()
const differs = page.locator('.events .turn-line.differs')
const differsStyle = await differs.evaluate((e) => {
  const s = getComputedStyle(e)
  const probe = document.createElement('span')
  probe.style.background = 'var(--notice-bg)'
  probe.style.borderColor = 'var(--notice-line)'
  probe.style.borderStyle = 'solid'
  document.body.append(probe)
  const want = getComputedStyle(probe)
  const out = { opacity: s.opacity, bg: s.backgroundColor, border: s.borderTopColor, wantBg: want.backgroundColor, wantBorder: want.borderTopColor }
  probe.remove()
  return out
})
const differsText = await differs.textContent()
check(
  differsStyle.opacity === '1' && differsStyle.bg === differsStyle.wantBg && differsStyle.border === differsStyle.wantBorder,
  `a turn that ran with another model keeps its line at rest, in the notice colour: ${JSON.stringify(differsStyle)}`,
)
check(differsText.startsWith('Ran Fake Sonnet, asked Default · medium · ask · sent '), `and names both: "${differsText}"`)
check((await opacity(firstLine)) === '0', 'while a turn that ran as asked keeps its line hidden')
await turnShot(page.locator('.events .turn-block').filter({ has: page.locator('.turn-line.differs') }), 'turn-mismatch')
// the turn in flight shows its line on hover once it has run, in the space
// its separator will take.
await reply2.click()
await reply2.fill('drip 6 400')
await reply2.press('Control+Enter')
await rest()
const pending = page.locator('.events .turn-end.pending .turn-line')
await until('the turn in flight has run', async () => (await pending.count()) === 1 && ((await pending.textContent()) ?? '').startsWith('Ran'), 30000)
const flightBefore = await layout()
await page.locator('.events .turn-block').last().locator('.message.user').hover()
await until('the in-flight line on hover', async () => (await opacity(pending)) === '1')
const flightSent = (await api('GET', `/api/conversations/${L}/events?turns=1`)).body.events.filter((e) => e.kind === 'user').at(-1).at
check(
  (await pending.textContent()) === `Ran Fake Opus · medium · ask · sent ${await inZone(flightSent, minute)}` && (await layout()) === flightBefore,
  `the turn in flight shows what it runs with and when it started on hover, and nothing moves: "${await pending.textContent()}"`,
)
await t.idle(L)
await rest()
await page.evaluate((id) => {
  location.hash = `#/c/${id}`
}, C)
await until('C shown again', async () => (await page.locator('.events .message.assistant', { hasText: 'a reply given in a terminal' }).count()) === 1)

// the runtime-started line names the version the harness reported. one
// older than mega's tested range is marked on that line, in the notice
// colour, with the range after the version and the sentence in its title.
// one inside the range, and one newer, the normal state for a harness that
// updates itself, are not marked and gain no words; nothing is refused for
// its version. the fake shim reports the version a file beside it names.
const versionFile = join(t.dir, 'fakeshim', 'claude-version')
const claudeTested = '2.1.318 to 2.1.326'
const startLine = page.locator('.events p.system', { hasText: /^Runtime started/ }).first()
const startedWith = async (version, name) => {
  writeFileSync(versionFile, version + '\n')
  const id = (await t.launch(P.id, 'claude', 'echo\nhello', { name })).id
  await t.idle(id)
  await page.evaluate((id) => {
    location.hash = `#/c/${id}`
  }, id)
  await until(`the start line of ${name}`, async () => (await startLine.count()) === 1 && (await startLine.textContent()).includes(`v${version}`))
  return startLine.evaluate((e) => {
    const s = getComputedStyle(e)
    const probe = document.createElement('p')
    probe.className = 'notice'
    document.body.append(probe)
    const want = getComputedStyle(probe)
    // the line stands in the history's column, where its neighbours do.
    const r = e.getBoundingClientRect()
    const column = e.closest('.events').getBoundingClientRect()
    const inColumn = r.width > 0 && r.left >= column.left && r.right <= column.right && r.top >= column.top && r.bottom <= column.bottom
    const out = { className: e.className, title: e.getAttribute('title'), text: e.textContent, bg: s.backgroundColor, noticeBg: want.backgroundColor, border: s.borderTopStyle, inColumn }
    probe.remove()
    return out
  })
}
// lineShot saves the start line with a margin, once it is in view and has
// stopped moving.
const lineShot = async (name) => {
  await startLine.scrollIntoViewIfNeeded()
  let b = await startLine.boundingBox()
  await until(`the start line still, for ${name}`, async () => {
    await new Promise((r) => setTimeout(r, 100))
    const now = await startLine.boundingBox()
    const still = now && b && now.y === b.y && now.x === b.x
    b = now
    return still
  })
  await page.screenshot({ path: `${t.dir}/shots/${name}.png`, clip: { x: b.x - 24, y: b.y - 24, width: b.width + 48, height: b.height + 48 } })
}
const inside = await startedWith('2.1.320', 'version inside')
t.say(`  inside the range: ${JSON.stringify(inside)}`)
check(inside.inColumn && inside.className === 'system' && inside.title === null && !inside.text.includes('tested') && inside.bg !== inside.noticeBg, `a start inside the tested range is not marked: "${inside.text}"`)
await lineShot('start-tested')
const newer = await startedWith('2.1.999', 'version newer')
t.say(`  newer than the range: ${JSON.stringify(newer)}`)
check(
  newer.inColumn && newer.className === 'system' && newer.title === null && newer.bg !== newer.noticeBg && newer.text === 'Runtime started · fake-opus-1 · v2.1.999',
  `a start newer than the tested range, the normal state, is not marked and gains no words: "${newer.text}"`,
)
await lineShot('start-newer')
const older = await startedWith('2.1.300', 'version older')
t.say(`  older than the range: ${JSON.stringify(older)}`)
check(
  older.inColumn && older.className === 'system version-older' && older.bg === older.noticeBg && older.border === 'solid' && older.text.includes(`v2.1.300 · older than the ${claudeTested} mega was tested with`),
  `a start older than the tested range is marked on its line, in the notice colour, naming the range: "${older.text}"`,
)
check(older.title?.includes("expects Claude Code '2.1.318' or later, so some calls may fail"), `and its title says what the agent sdk expects: "${older.title}"`)
await lineShot('start-older')
check((await workspace()).conversations.filter((c) => c.name?.startsWith('version ')).every((c) => c.state === 'running'), 'none of them was refused for its version')
rmSync(versionFile)
await page.evaluate((id) => {
  location.hash = `#/c/${id}`
}, C)
await until('C shown again', async () => (await page.locator('.events .message.assistant', { hasText: 'a reply given in a terminal' }).count()) === 1)

// MCP servers: an item in the control bar's More menu, carrying the count,
// opens the list; a server that failed or needs signing in also marks the
// More button. read only, and never attention. C, released, has nothing to
// ask.
const more = page.getByRole('button', { name: 'More', exact: true })
const mcpItem = page.getByRole('menuitem', { name: /MCP server/ })
const mcpMark = page.locator('.control-bar .more .more-mark')
const markText = 'An MCP server failed or needs signing in: the More menu lists the servers'
const dialog = page.locator('dialog[open]')
const openMcpItem = async (text) => {
  await more.click()
  await until(`the MCP item to read "${text}"`, async () => (await mcpItem.getAttribute('aria-label')) === text, 30000)
}
check((await page.locator('.conversation-head .mcp-summary').count()) === 0, 'the header no longer carries the MCP servers')
await openMcpItem('MCP servers: not running')
check((await mcpMark.count()) === 0, 'a conversation with no runtime: the More menu says its MCP servers are not running, and More carries no mark')
await mcpItem.click()
await dialog.waitFor()
await until('the list read', async () => (await dialog.textContent()).includes('This conversation is not running, so there are no servers to ask.'))
check(true, 'and the list it opens says so')
await dialog.getByRole('button', { name: 'Close' }).click()
await until('the list closed', async () => (await dialog.count()) === 0)

// a claude conversation: the fake's fixture connects, notes needs signing
// in, broken failed. the mark on More is in view at the bottom of the
// conversation, without scrolling.
const M = (await t.launch(P.id, 'claude', 'echo\nhello', { name: 'servers' })).id
await page.evaluate((id) => {
  location.hash = `#/c/${id}`
}, M)
await until('the mark on More', async () => (await mcpMark.count()) === 1, 30000)
const markBox = await mcpMark.boundingBox()
const viewport = page.viewportSize()
check((await mcpMark.getAttribute('aria-label')) === markText && markBox.y >= 0 && markBox.y + markBox.height <= viewport.height, `a failed server marks the More button, in view at the bottom: "${await mcpMark.getAttribute('aria-label')}"`)
const settled = '3 MCP servers, 1 needs sign-in, 1 failed'
await openMcpItem(settled)
check(true, `the More menu's item carries the count and says so: "${settled}"`)
await mcpItem.click()
await dialog.waitFor()
await until('the claude list', async () => (await dialog.locator('.mcp-server').count()) === 3)
const server = (name) => dialog.locator('.mcp-server', { has: page.locator('.mcp-name', { hasText: new RegExp(`^${name}$`) }) })
const statusOf = async (name) => server(name).locator('.mcp-status').textContent()
check((await statusOf('broken')) === 'Failed' && (await server('broken').locator('.error').textContent()) === 'spawn /nonexistent/mcp-server ENOENT', 'a failed server says why')
check((await statusOf('notes')) === 'Needs sign-in' && (await server('notes').locator('.notice').textContent()).includes('run claude in this checkout and use /mcp') && (await server('notes').locator('.notice').textContent()).includes('mega does not sign in for you'), 'a server that needs signing in says it is done in the harness, and where')
const fixtureTools = server('fixture').locator('.mcp-tools li')
check((await statusOf('fixture')) === 'Connected' && !(await fixtureTools.first().isVisible()), 'a connected server lists its tools only on expansion')
await server('fixture').locator('summary').click()
await until('the tools shown', async () => (await fixtureTools.first().isVisible()))
check(JSON.stringify(await fixtureTools.locator('code').allTextContents()) === '["echo","pick_color"]', 'expanded, it lists its tools by name; claude gives no descriptions')
check((await dialog.locator('button').count()) === 1, 'the list is read only: its one control is Close')
await t.shot(page, 'mcp-claude')

// a sign-in, done in the harness, shows in the open list and the menu item;
// the failed server keeps the mark.
check((await api('POST', `/api/conversations/${M}/prompt`, { text: 'mcp-sign-in' })).status === 202, 'the fake signs notes in on its next turn')
await until('the open list after the sign-in', async () => (await statusOf('notes')) === 'Connected')
await dialog.getByRole('button', { name: 'Close' }).click()
await until('the list closed again', async () => (await dialog.count()) === 0)
await openMcpItem('3 MCP servers, 1 failed')
check((await mcpMark.count()) === 1, 'the open list and the menu item follow the change, and the failed server keeps the mark')
await page.keyboard.press('Escape')
await until('the turn read', async () => !(await convOf(M)).turn_active && (await convOf(M)).attention === 'idle')
check(!(await workspace()).needs_you.includes(M), 'a failed server is not attention: the conversation is idle and not in needs-you')

// a codex conversation: its own servers, with descriptions, and its own
// sign-in.
const X = (await t.launch(P.id, 'codex', 'hello', { name: 'codex servers' })).id
await page.evaluate((id) => {
  location.hash = `#/c/${id}`
}, X)
await until('the codex mark', async () => (await mcpMark.count()) === 1, 30000)
await openMcpItem(settled)
await mcpItem.click()
await dialog.waitFor()
await until('the codex list', async () => (await dialog.locator('.mcp-server').count()) === 3)
check((await server('docs').locator('.notice').textContent()).includes('codex mcp login docs'), 'a codex server that needs signing in names codex mcp login')
await server('fixture').locator('summary').click()
await until('the codex tools', async () => (await fixtureTools.first().isVisible()))
check((await fixtureTools.first().textContent()) === 'echo Echo the given text back, and log the call.', `codex's tools carry their descriptions: "${await fixtureTools.first().textContent()}"`)
await dialog.getByRole('button', { name: 'Close' }).click()
await until('the codex list closed', async () => (await dialog.count()) === 0)

// released, the servers go with the runtime, and so does the mark.
await page.evaluate((id) => {
  location.hash = `#/c/${id}`
}, M)
await api('POST', `/api/conversations/${M}/release`)
await until('released', async () => (await convOf(M)).state === 'none' && (await mcpMark.count()) === 0)
await openMcpItem('MCP servers: not running')
await page.keyboard.press('Escape')
check((await convOf(M)).mcp === undefined, 'released, the menu item says not running, the mark goes, and the workspace read carries no servers')
await page.evaluate((id) => {
  location.hash = `#/c/${id}`
}, B)
await until('B shown again', async () => (await page.getByText('Line 60 of a streamed reply').count()) > 0, 30000)

// a reconnect reads only what the page missed.
await until('B idle', async () => !(await convOf(B)).turn_active)
const through = (await api('GET', `/api/conversations/${B}/events?turns=1`)).body.through
reads.length = 0
await t.stop()
const footer = page.locator('.side-foot .stream')
const waiting = new RegExp(`the stream at ws://127\\.0\\.0\\.1:${port}/api/stream · \\d+s · attempt \\d+ · last error: the connection closed \\(code \\d+\\)`)
await until('the footer to name what it waits for', async () => waiting.test((await footer.getAttribute('title')) ?? ''))
check(true, `the footer names what it waits for: "${await footer.getAttribute('title')}"`)
await t.start()
await page.locator('.side-foot .stream-connected').waitFor({ timeout: 30000 })
await until('the reconnect read', async () => reads.some((u) => /events\?since=\d+$/.test(u)))
// the stop recorded the runtime's release, which the page may have been given
// before the stream dropped, so it asks from wherever its own through stood.
const since = Number(/events\?since=(\d+)$/.exec(reads.find((u) => u.includes('since=')))[1])
check(reads.length === 1 && since >= through, `a reconnect reads only what it missed, after ${through} or later: ${reads.join(', ')}`)

// loading says what it waits for: a held, then failing, opening read. the
// check holds each response until it has seen what the page says meanwhile.
let release = null
const held = []
await page.route('**/events?turns=10', async (route) => {
  held.push(route)
  await new Promise((r) => (release = r))
  if (held.length === 1) {
    await route.fulfill({ status: 503, contentType: 'application/json', body: '{"code":"unavailable","message":"held by the check"}' })
  } else {
    await route.continue()
  }
})
await page.evaluate((id) => {
  location.hash = `#/c/${id}`
}, A)
const loading = page.locator('.loading')
await until('the loading line, a second in', async () => /Loading history: GET \/api\/conversations\/[0-9a-f]+\/events\?turns=10 · [1-9]\d*s/.test((await loading.textContent().catch(() => '')) ?? ''))
check(true, `the loading line names the request and how long: "${await loading.textContent()}"`)
release()
await until('the second attempt', async () => held.length === 2 && /attempt 2 · last error: held by the check/.test((await loading.textContent().catch(() => '')) ?? ''))
check(true, `then the attempt and the last error: "${await loading.textContent()}"`)
release()
await page.waitForFunction(inView, `turn ${genA.turns}`, { timeout: 30000, polling: 50 })
await page.unroute('**/events?turns=10')
check(true, 'and the tail lands after the retry')

// connecting says what it waits for: a workspace read that fails.
const fresh = await t.page()
await fresh.route('**/api/workspace', (route) => route.abort('connectionrefused'))
await fresh.goto(`${url}/`)
const waits = fresh.locator('.waiting-list li')
await until('the connecting view on its third attempt', async () =>
  (await waits.allTextContents()).some((w) => /^GET \/api\/workspace · \d+s · attempt [3-9] · last error: /.test(w)),
)
check(true, `the connecting view names the read, attempts and error: ${JSON.stringify(await waits.allTextContents())}`)
check(centred(await column(fresh, '.unknown')), 'the connecting view is centred in the pane')
await t.shot(fresh, 'connecting')

// the project page is centred the same way.
await page.evaluate((id) => {
  location.hash = `#/p/${id}`
}, P.id)
await page.locator('article.detail').waitFor()
check(centred(await column(page, 'article.detail')), 'the project page is centred in the pane')

// a pi conversation: pi's own words under the header about how it asks, its
// prompt, its tool call and reply in the history as the others', and each
// turn's line saying what it ran with, with no permission mode. the prompt
// pi records under its own identity is bookkeeping, shown only with the
// native events. the extension's gate is blocked: allowing it is not what
// this check is about, and a check allows only the approval it is about.
const Pi = (await t.launch(P.id, 'pi', 'echo hello from pi', { name: 'pi' })).id
await t.idle(Pi)
await page.evaluate((id) => {
  location.hash = `#/c/${id}`
}, Pi)
await until('the pi reply', async () => (await page.locator('.events .message.assistant', { hasText: 'hello from pi' }).count()) === 1)
const piNote = await page.locator('.conversation .harness-notice').textContent()
check(piNote.includes('pi asks for nothing unless an extension asks') && piNote.includes('which mega cannot list'), `the conversation says how pi asks, under its header: "${piNote}"`)
const ask = await api('POST', `/api/conversations/${Pi}/prompt`, { text: 'ask' })
check(ask.status === 202, 'a prompt to pi is accepted')
await until('the gate', async () => (await convOf(Pi)).pending.length === 1)
const gate = (await convOf(Pi)).pending[0]
const gateTitle = gate.payload?.questions?.[0]?.question
if (gate.kind !== 'question' || gateTitle !== 'Approve bash command: sleep 1') throw new Error(`stopped before answering: ${JSON.stringify(gate)}`)
// the gated bash call has not run while its dialog waits: its row says so,
// and is not shown as running.
const gatedRow = page.locator('.events details.tool', { hasText: 'bash sleep 1' })
await until('the gated row', async () => (await gatedRow.count()) === 1)
check(
  ((await gatedRow.locator('summary').textContent()) ?? '').includes('waiting on a dialog') && (await gatedRow.locator('.spinner').count()) === 0,
  `a pi action whose dialog is pending says it waits on the dialog, not that it runs: "${await gatedRow.locator('summary').textContent()}"`,
)
await api('POST', `/api/conversations/${Pi}/answer`, { request_id: gate.id, answers: { [gateTitle]: 'Block' } })
await t.idle(Pi)
await until('the tool row and reply', async () => (await page.locator('.events .message.assistant', { hasText: /^blocked$/ }).count()) === 1)
const piTools = await page.locator('.events details.tool summary').allTextContents()
check(piTools.some((tool) => tool.includes('bash sleep 1')), `the tool call is a row in the history: ${JSON.stringify(piTools)}`)
check((await page.locator('.events .message.user', { hasText: /^ask$/ }).count()) === 1 && (await page.locator('.events', { hasText: 'user/recorded by pi' }).count()) === 0, 'the prompt shows once, and the record of pi\'s identity for it is hidden with the native events')
const piLine = await page.locator('.events .turn-end:not(.pending) .turn-line').last().textContent()
check(/^Ran Fake Large · medium · sent \S+( [AP]M)? · took /.test(piLine ?? ''), `each turn's line says what it ran with, and no mode: "${piLine}"`)
await t.shot(page, 'pi-conversation')

// actions, for each harness's fake: collapsed, one line, a failed one marked
// in the error colour; expanded, what it invoked in full, past the line's
// two hundred bytes, then what came back; a copy control for the
// invocation; and an action recorded with nothing its adapter can read,
// showing the one line it has.
await page.context().grantPermissions(['clipboard-read', 'clipboard-write'], { origin: url })
const rowsOf = () => page.locator('.events details.tool')
const errorColour = await page.evaluate(() => {
  const probe = document.createElement('span')
  probe.style.color = 'var(--error)'
  document.body.append(probe)
  const colour = getComputedStyle(probe).color
  probe.remove()
  return colour
})
for (const [harness, other, otherFields] of [
  ['claude', 'Edit', 'file_path,old_string,new_string'],
  ['codex', 'edit', 'update,add'],
  ['pi', 'read', 'path,limit,offset'],
]) {
  const id = (await t.launch(P.id, harness, 'actions', { name: `${harness} actions` })).id
  await t.idle(id)
  await page.evaluate((id) => {
    location.hash = `#/c/${id}`
  }, id)
  await until(`${harness}'s three actions`, async () => (await rowsOf().count()) === 3)
  const [long, edit, failed] = [rowsOf().nth(0), rowsOf().nth(1), rowsOf().nth(2)]
  const line = (await long.locator('summary').textContent()) ?? ''
  const failedSummary = failed.locator('summary .tool-error')
  check(
    (await failed.getAttribute('open')) === null &&
      (await failedSummary.textContent()) === ' failed' &&
      (await failedSummary.evaluate((e) => getComputedStyle(e).color)) === errorColour &&
      (await long.locator('summary .tool-error').count()) === 0,
    `${harness}: a failed action is marked while collapsed, in the error colour; one that ran is not`,
  )
  await long.locator('summary').click()
  const command = long.locator('pre.command')
  await until(`${harness}: the command read`, async () => (await command.count()) === 1)
  const said = (await command.textContent()) ?? ''
  const parts = await long.locator('.tool-part').allTextContents()
  const order = await long.evaluate((row) => {
    const at = (sel) => [...row.querySelectorAll('*')].indexOf(row.querySelector(sel))
    return at('pre.command') < at('.tool-output:not(.invocation) pre')
  })
  check(
    !line.includes('END-OF-FAKE-COMMAND') && said.includes('END-OF-FAKE-COMMAND') && said.includes('\n') && parts.join(',') === 'Command,Result' && order,
    `${harness}: expanded, the whole command, which the one line cut short, then the result: ${JSON.stringify(parts)}, ${said.length} bytes`,
  )
  await long.locator('.invocation').getByRole('button', { name: 'Copy the command' }).click()
  const copied = await page.evaluate(() => navigator.clipboard.readText())
  check(copied === said, `${harness}: its copy control takes the whole command`)
  await long.evaluate((row) => row.scrollIntoView({ block: 'start' }))
  await long.screenshot({ path: `${t.dir}/shots/action-${harness}.png` })
  await edit.locator('summary').click()
  await until(`${harness}: the ${other} read`, async () => (await edit.locator('.invocation-fields dt').count()) > 0)
  const names = (await edit.locator('.invocation-fields dt').allTextContents()).join(',')
  check(
    (await edit.locator('.tool-part').first().textContent()) === `Invoked ${harness === 'codex' ? 'edit' : other}` && names === otherFields,
    `${harness}: another tool shows its name and its input, field by field: ${names}`,
  )
  // the row at the top of the view, clear of the panel pinned at the foot.
  await edit.evaluate((row) => row.scrollIntoView({ block: 'start' }))
  await edit.screenshot({ path: `${t.dir}/shots/action-${harness}-input.png` })
}
// an action from the generated history, whose payload no adapter reads.
await page.evaluate((id) => {
  location.hash = `#/c/${id}`
}, A)
await until('A shown again', async () => (await rowsOf().count()) > 0)
const old = rowsOf().last()
const oldLine = ((await old.locator('summary').textContent()) ?? '').replace(/ (no result recorded|failed)$/, '')
await old.locator('summary').click()
await until('the old action read', async () => (await old.locator('.invocation-summary').count()) === 1)
check(
  ((await old.textContent()) ?? '').includes('Only its one line was recorded') && (await old.locator('.invocation-summary').textContent()) === oldLine,
  `an action recorded with nothing its adapter reads shows the line it has: "${oldLine.slice(0, 60)}"`,
)

// a tool call codex reports after its turn ended, here a web search, which
// has no result of its own, arriving while the next turn runs: it is not that
// turn's, so it shows no spinner, only that no result was recorded.
const W = (await t.launch(P.id, 'codex', 'hello', { name: 'codex late search' })).id
await t.idle(W)
await page.evaluate((id) => {
  location.hash = `#/c/${id}`
}, W)
const lateSent = await api('POST', `/api/conversations/${W}/prompt`, { text: 'late-search' })
t.must(lateSent.status === 202, `the late-search prompt was taken: ${lateSent.status}`)
await until('the late search row', async () => (await rowsOf().count()) === 1)
const lateRow = rowsOf().first()
const lateLine = (await lateRow.locator('summary').textContent()) ?? ''
t.must(lateLine.includes('fake late search') && (await convOf(W)).turn_active, `the late search is the fake's, shown while the next turn runs: "${lateLine}"`)
check(
  (await lateRow.locator('[aria-label="Running"]').count()) === 0 && lateLine.endsWith('no result recorded'),
  `a tool call reported after its turn shows no spinner in the next turn: "${lateLine}"`,
)
const lateActivity = page.locator('.panel-cluster .activity-text')
await until('the running turn’s activity line', async () => (await lateActivity.count()) === 1)
const lateSaid = (await lateActivity.textContent()) ?? ''
check(lateSaid.startsWith('Working') && !lateSaid.includes('fake late search'), `nor does the activity line name it as the turn’s work: "${lateSaid}"`)
await api('POST', `/api/conversations/${W}/interrupt`)
await until('the late search turn ended', async () => !(await convOf(W)).turn_active)
// and a message codex reports for the turn before, arriving while the next
// turn streams its reply: the reply streaming so far stays, and so does
// Writing.
const lateMessage = await api('POST', `/api/conversations/${W}/prompt`, { text: 'late-message' })
t.must(lateMessage.status === 202, `the late-message prompt was taken: ${lateMessage.status}`)
await until('the late message', async () => (await page.locator('.events .message.assistant:not(.streaming)', { hasText: 'LATE-MESSAGE' }).count()) === 1)
const streaming = page.locator('.events .message.assistant.streaming')
const streamingSaid = (await streaming.count()) === 1 ? ((await streaming.textContent()) ?? '') : ''
const writingSaid = (await lateActivity.textContent()) ?? ''
check(streamingSaid.includes('STREAMING-NOW') && writingSaid.startsWith('Writing'), `a message reported after its turn leaves the next turn's streaming reply be: "${streamingSaid}", "${writingSaid}"`)
check(!streamingSaid.includes('STALE-STREAM'), `and text streamed for the turn before is not part of it: "${streamingSaid}"`)
await api('POST', `/api/conversations/${W}/interrupt`)
await until('the late message turn ended', async () => !(await convOf(W)).turn_active)

// dark: the centred conversation, with the panel open and collapsed.
const dark = await t.page({ width: 1440, height: 900, colorScheme: 'dark' })
await dark.goto(`${url}/#/c/${A}`)
await dark.waitForFunction(inView, `turn ${genA.turns}`, { timeout: 30000, polling: 50 })
check(centred(await column(dark, '.conversation')), 'in the dark scheme too')
await t.shot(dark, 'centred-dark-panel')
await dark.getByRole('button', { name: /^Collapse the side panel/ }).click()
await dark.locator('nav.rail').waitFor()
await until('dark re-centred', async () => centred(await column(dark, '.conversation')) && (await column(dark, '.conversation')).left > open.left)
await t.shot(dark, 'centred-dark-rail')

// Exact native history is available before the next managed launch, even
// when the binding exists only in the log. The native dialog shares no rows
// or acknowledgments with the conversation timeline.
const nativeDialog = page.locator('dialog:modal', { has: page.getByRole('heading', { name: 'Native transcript' }) })
const moreMenu = page.getByRole('button', { name: 'More', exact: true })
const transcriptItem = page.getByRole('menuitem', { name: 'View native transcript' })
const gapLink = page.locator('.history-gap').getByRole('button', { name: 'View native transcript' })
const closeTranscript = async () => {
  await nativeDialog.getByRole('button', { name: 'Close' }).click()
  await nativeDialog.waitFor({ state: 'hidden' })
}
await page.goto(`${url}/#/c/${gap.id}`)
await gapLink.waitFor()
await until('the visible gap to be read', async () => {
  const view = await convOf(gap.id)
  return view.read_seq === view.last_seq
})
check(await gapLink.count() === 1 && (await convOf(gap.id)).native_id === gap.native_id, 'one visible handoff gap is acknowledged and offers the exact id held in the log')
const gapBefore = await convOf(gap.id)
await gapLink.click()
await nativeDialog.getByText(terminalPrompt, { exact: true }).waitFor()
check(
  (await nativeDialog.getByText(terminalReply, { exact: true }).count()) === 1 &&
    (await nativeDialog.locator('.message.user', { hasText: terminalPrompt }).locator('.who').textContent()) === 'You' &&
    (await nativeDialog.locator('.message.assistant', { hasText: terminalReply }).locator('.who').textContent()) === 'Claude Code' &&
    !(await nativeDialog.textContent()).includes('outside mega'),
  'the standard modal shows the terminal exchange by speaker, including a prompt with text and an attachment',
)
await closeTranscript()
check((await page.locator('.events').textContent()).includes('managed before the gap') && !(await page.locator('.events').textContent()).includes(terminalReply), 'the managed timeline shows what mega witnessed')
await page.reload()
await gapLink.waitFor()
await moreMenu.click()
await transcriptItem.click()
await nativeDialog.getByText(terminalReply, { exact: true }).waitFor()
const gapAfter = await convOf(gap.id)
check(await gapLink.count() === 1 && gapBefore.last_seq === gapAfter.last_seq && gapBefore.read_seq === gapAfter.read_seq, 'More can reread the same transcript after reload without moving the reading position')
await closeTranscript()

nativeHistory.empty()
await gapLink.click()
await nativeDialog.getByText('No messages to show.', { exact: true }).waitFor()
check(true, 'a confirmed empty Claude session says No messages to show')
await closeTranscript()
nativeHistory.missing()
await gapLink.click()
await nativeDialog.getByRole('alert').filter({ hasText: 'Transcript unavailable' }).waitFor()
check((await convOf(gap.id)).native_id === gap.native_id, 'a missing Claude session reports Transcript unavailable and the conversation survives')
await closeTranscript()
nativeHistory.restore()

// Losing a first launch before its identity was recorded still lets the
// operator acknowledge the loss and remove the Mega conversation.
await page.goto(`${url}/#/c/${unknown.id}`)
await page.locator('.history-gap').getByText(/Adopt a session/).waitFor()
await moreMenu.click()
check(await gapLink.count() === 0 && await transcriptItem.count() === 0, 'an unknown session shows Adopt guidance and offers no transcript')
const unknownBefore = await convOf(unknown.id)
await page.getByRole('menuitem', { name: 'Resume', exact: true }).click()
await until('the acknowledgment to clear supervision loss', async () => (await convOf(unknown.id)).state === 'none')
const unknownAfter = await convOf(unknown.id)
check(!unknownAfter.native_id && unknownAfter.last_seq === unknownBefore.last_seq, 'Resume acknowledges an unknown session without starting a runtime')
check((await api('DELETE', `/api/conversations/${unknown.id}`)).status === 204, 'the acknowledged conversation can be removed')

await page.goto(`${url}/#/c/${legacy.id}`)
await page.getByText('legacy loss record', { exact: false }).waitFor()
await moreMenu.click()
await transcriptItem.click()
await nativeDialog.getByText('legacy completed', { exact: true }).waitFor()
check(true, 'More opens the bound transcript for an Unsupervised entry from an older build')
await closeTranscript()
await moreMenu.click()
await page.getByRole('menuitem', { name: 'Resume', exact: true }).click()
await until('legacy supervision loss acknowledged', async () => (await convOf(legacy.id)).state === 'none')
check((await api('POST', `/api/conversations/${legacy.id}/prompt`, { text: 'echo\nreply after the gap' })).status === 202, 'the next prompt starts after acknowledging the old gap')
await t.idle(legacy.id)
await t.stop()
const replay = readLog(legacy.id)
const secondPrompt = replay.findLast((ev) => ev.kind === 'user')
const secondEnd = replay.findLast((ev) => ev.kind === 'turn_end')
const secondStart = Math.floor(Date.parse(secondPrompt.at) / 1000) * 1000
secondPrompt.at = new Date(secondStart).toISOString()
secondEnd.at = new Date(secondStart + 10_000).toISOString()
writeLog(legacy.id, replay)
await t.start()
await page.reload()
const oldTurn = page.locator('.events .turn-block').filter({ hasText: 'reply before supervision was lost' })
const nextTurn = page.locator('.events .turn-block').filter({ hasText: 'reply after the gap' })
await nextTurn.locator('.message.assistant').waitFor()
const oldOutcomes = await oldTurn.locator('.turn-end').count()
const nextLine = await nextTurn.locator('.turn-line').textContent()
check(await oldTurn.count() === 1 && oldOutcomes === 0 && nextLine.includes('took 10s'), `the old turn has no recorded outcome (${oldOutcomes}) and the next turn took ten seconds from its own prompt: "${nextLine}"`)
const legacyTail = (await api('GET', `/api/conversations/${legacy.id}/events?turns=1`)).body
const legacyEarlier = (await api('GET', `/api/conversations/${legacy.id}/events?turns=1&before=${legacyTail.from}`)).body
check(legacyTail.events.some((ev) => ev.text === 'reply after the gap') && legacyEarlier.events.some((ev) => ev.text === 'reply before supervision was lost'), 'paging back one turn crosses the old Unsupervised boundary')

await t.done()

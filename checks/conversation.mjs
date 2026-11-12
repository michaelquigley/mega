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
// mega" on its reply. it saves the centred conversation, light and dark,
// with the panel open and collapsed.
import { join } from 'node:path'
import { appendImported, appendNative, appendTurns } from './lib/gen.mjs'
import { suite } from './lib/mega.mjs'

const t = await suite('conversation')
const { url, port, check, until, api, convOf } = t

// two conversations, launched and released, then made long while the service
// is stopped.
const P = await t.register('long', t.checkout('long'))
const A = (await t.launch(P.id, 'claude', 'echo\nhello')).id
const B = (await t.launch(P.id, 'claude', 'echo\nhello')).id
const C = (await t.launch(P.id, 'claude', 'echo\nhello', { name: 'imported' })).id
for (const id of [A, B, C]) {
  await t.idle(id)
  await api('POST', `/api/conversations/${id}/release`)
}
await until('all released', async () => (await convOf(A)).state === 'none' && (await convOf(B)).state === 'none' && (await convOf(C)).state === 'none')
await t.stop()
const logOf = (id) => join(t.dir, 'data', 'conversations', id, 'events.jsonl')
const genA = appendTurns(logOf(A), 20000, 1)
const genB = appendTurns(logOf(B), 20000, 2)
appendNative(logOf(B))
appendImported(logOf(C))
t.say(`generated ${genA.seq} events in ${genA.turns} turns, and ${genB.seq} in ${genB.turns}`)
await t.start()

const page = await t.page({ width: 1440, height: 900 })
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

// show earlier: ten more turns above, the view held.
await page.evaluate(() => {
  document.querySelector('.main').scrollTop = 0
})
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
await page.evaluate(() => {
  document.querySelector('.main').scrollTop = 0
})
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
// read from higher up, so new activity arrives below the view. a scroll
// event is delivered before the next frame, so after two frames the page
// knows the view has left the bottom.
await page.evaluate(async () => {
  document.querySelector('.main').scrollTop = 0
  await new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)))
})
await until('the view at the top', async () => (await page.evaluate(() => document.querySelector('.main').scrollTop)) === 0)
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
await t.shot(page, 'outside')
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

await t.done()

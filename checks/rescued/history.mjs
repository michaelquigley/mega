// browser check for the recent tail, on two generated conversations of about
// twenty thousand events each, on a scratch service at 18470. B's log ends in
// a native event the page is never sent.
// usage: node history.mjs <chrome> <mega> <config> <claudecfg> <A> <B> <shots>
import { chromium } from 'playwright-core'
import { spawn } from 'node:child_process'
import { openSync } from 'node:fs'
const [exe, bin, cfg, claudecfg, A, B, shots] = process.argv.slice(2)
const url = 'http://127.0.0.1:18470'
const say = (s) => console.log(`[${new Date().toISOString().slice(11, 19)}] ${s}`)
const out = openSync(`${shots}/service.out`, 'a')
let svc
const startService = async () => {
  svc = spawn(bin, ['--config', cfg], { stdio: ['ignore', out, out], env: { ...process.env, CLAUDE_CONFIG_DIR: claudecfg } })
  for (let i = 0; i < 300; i++) { try { if ((await fetch(`${url}/api/workspace`)).ok) return } catch {} await new Promise((r) => setTimeout(r, 100)) }
  throw new Error('the service did not start')
}
const stopService = () => new Promise((r) => { svc.once('exit', r); svc.kill('SIGINT') })
const stop = async (code) => { try { await stopService() } catch {} process.exit(code) }
process.on('uncaughtException', (e) => { console.log('crashed:', e.stack); stop(2) })
const api = async (method, path, body) => { const r = await fetch(url + path, { method, headers: { 'Content-Type': 'application/json', Origin: url }, body: body ? JSON.stringify(body) : undefined }); const t = await r.text(); return { status: r.status, body: t ? JSON.parse(t) : null } }
const conv = async (id) => (await api('GET', '/api/workspace')).body.conversations.find((c) => c.id === id)
const until = async (what, pred, ms = 20000) => { const end = Date.now() + ms; while (Date.now() < end) { if (await pred()) return; await new Promise((r) => setTimeout(r, 100)) } throw new Error(`timed out: ${what}`) }
const failures = []; const check = (ok, what) => { say(`${ok ? 'ok  ' : 'FAIL'} ${what}`); if (!ok) failures.push(what) }
const errors = []

await startService()
const b = await chromium.launch({ executablePath: exe })
const ctx = await b.newContext({ viewport: { width: 1440, height: 900 } })
const page = await ctx.newPage()
page.on('pageerror', (e) => errors.push(String(e)))
const reads = []
page.on('request', (r) => { if (r.url().includes('/events')) reads.push(r.url().replace(url, '')) })
const rows = () => page.evaluate(() => { const rs = [...document.querySelectorAll('.events [data-seq]')].map((e) => Number(e.dataset.seq)); return { count: rs.length, first: rs[0], last: rs.at(-1) } })
const inView = (n) => { const h = [...document.querySelectorAll('h3,h4,h5,h6')].find((e) => e.textContent === n); if (!h) return false; const r = h.getBoundingClientRect(); return r.top >= 0 && r.bottom <= window.innerHeight }
const earlierText = () => page.locator('.earlier .muted').first().textContent()

// open A: the tail, at the newest activity.
const t0 = Date.now()
await page.goto(`${url}/#/c/${A}`)
await page.waitForFunction(inView, 'turn 413', { timeout: 30000, polling: 50 })
say(`A open at its newest reply after ${Date.now() - t0} ms`)
const tailA = (await api('GET', `/api/conversations/${A}/events?turns=10`)).body
check(reads.length === 1 && /events\?turns=10$/.test(reads[0]), `opening reads only the tail: ${reads.join(', ')}`)
let r = await rows()
check(r.first >= tailA.from && r.count < 400, `only the tail is rendered: ${r.count} rows from seq ${r.first} (tail from ${tailA.from})`)
check((await earlierText()).startsWith(`${tailA.earlier.turns} earlier turns`), `the page says how much is earlier: "${await earlierText()}"`)
const userRows = await page.locator('.events .message.user').count()
check(userRows === 10, `ten turns shown: ${userRows} prompts`)
await until('A read', async () => (await conv(A)).read_seq === tailA.through)
check(true, `the reading position follows the newest event given: ${tailA.through}`)
await page.screenshot({ path: `${shots}/tail.png` })

// show earlier: ten more turns above, the view held.
await page.evaluate(() => { document.querySelector('.main').scrollTop = 0 })
const anchor = await page.evaluate(() => { const e = [...document.querySelectorAll('.events [data-seq]')].find((x) => x.getBoundingClientRect().bottom > 0); return { seq: e.dataset.seq, top: e.getBoundingClientRect().top } })
reads.length = 0
await page.getByRole('button', { name: 'Show the ten turns before these' }).click()
await until('ten more prompts', async () => (await page.locator('.events .message.user').count()) === 20)
const moved = await page.evaluate((seq) => document.querySelector(`.events [data-seq="${seq}"]`).getBoundingClientRect().top, anchor.seq)
check(Math.abs(moved - anchor.top) <= 2, `show earlier holds the view: row ${anchor.seq} at ${Math.round(anchor.top)} then ${Math.round(moved)}`)
check(reads.length === 1 && reads[0].includes(`before=${tailA.from}`) && reads[0].includes('turns=10'), `show earlier reads the ten turns before: ${reads.join(', ')}`)
check((await earlierText()).startsWith(`${tailA.earlier.turns - 10} earlier turns`), `the count drops by ten: "${await earlierText()}"`)
await page.screenshot({ path: `${shots}/earlier.png` })

// the native toggle refetches what is shown, with native events.
const shownFrom = (await rows()).first
reads.length = 0
await page.getByRole('button', { name: 'More' }).click()
await page.getByRole('menuitemcheckbox', { name: 'Native events' }).click()
await until('native rows', async () => (await page.locator('.events details.native').count()) > 0)
const natives = await page.locator('.events details.native').count()
check(reads.length === 1 && reads[0].includes('native=1') && reads[0].includes('since='), `showing native events refetches what is shown: ${reads.join(', ')} (${natives} native rows)`)
reads.length = 0
await page.getByRole('button', { name: 'More' }).click()
await page.getByRole('menuitemcheckbox', { name: 'Native events' }).click()
await until('no native rows', async () => (await page.locator('.events details.native').count()) === 0)
check(reads.length === 0, `hiding them fetches nothing: ${reads.join(', ')}`)
check((await rows()).first <= shownFrom, 'the stretch shown is unchanged')

// show all.
await page.evaluate(() => { document.querySelector('.main').scrollTop = 0 })
const anchor2 = await page.evaluate(() => { const e = [...document.querySelectorAll('.events [data-seq]')].find((x) => x.getBoundingClientRect().bottom > 0); return { seq: e.dataset.seq, top: e.getBoundingClientRect().top } })
reads.length = 0
const t1 = Date.now()
await page.getByRole('button', { name: 'Show the whole history' }).click()
await until('the earlier bar goes', async () => (await page.locator('.earlier').count()) === 0, 60000)
const allTook = Date.now() - t1
const moved2 = await page.evaluate((seq) => document.querySelector(`.events [data-seq="${seq}"]`).getBoundingClientRect().top, anchor2.seq)
r = await rows()
check(r.first === 1 && (await page.locator('.events .message.user').count()) === 414, `show all loads everything: ${r.count} rows from seq ${r.first}, in ${allTook} ms`)
check(Math.abs(moved2 - anchor2.top) <= 2, `show all holds the view: row ${anchor2.seq} at ${Math.round(anchor2.top)} then ${Math.round(moved2)}`)
check(reads.length === 1 && /events\?before=\d+$/.test(reads[0]), `show all reads everything before: ${reads.join(', ')}`)

// B: a log that ends in a native event. the reading position reaches it, and
// B's unread clears, although the page was never sent that event.
const bBefore = await conv(B)
check(bBefore.attention === 'unread', `B starts unread (read ${bBefore.read_seq}, last ${bBefore.last_seq})`)
reads.length = 0
await page.evaluate((id) => { location.hash = `#/c/${id}` }, B)
await page.waitForFunction(inView, 'turn 399', { timeout: 30000, polling: 50 })
await until('B read', async () => (await conv(B)).read_seq === bBefore.last_seq)
check((await conv(B)).attention === 'idle', `B is read through its native last event ${bBefore.last_seq} and idle`)

// a streamed reply on B re-renders only the loaded tail, without long tasks.
await page.evaluate(() => { window.__long = []; new PerformanceObserver((l) => { for (const e of l.getEntries()) window.__long.push(Math.round(e.duration)) }).observe({ type: 'longtask', buffered: false }) })
const body = Array.from({ length: 60 }, (_, i) => `Line ${i + 1} of a streamed reply, with \`code\` and **bold** words.`).join('\n\n')
const before = (await rows()).count
check((await api('POST', `/api/conversations/${B}/prompt`, { text: `stream 25 12\n${body}` })).status === 202, 'a streamed reply is asked for')
await page.locator('.message.streaming').waitFor({ timeout: 20000 })
await until('the reply lands', async () => (await page.locator('.message.streaming').count()) === 0 && (await page.getByText('Line 60 of a streamed reply').count()) > 0, 60000)
const longTasks = await page.evaluate(() => window.__long)
const after = (await rows()).count
check(after - before < 10 && before < 400, `the history held stays the tail: ${before} rows, then ${after}`)
check(longTasks.filter((d) => d >= 100).length === 0, `streaming caused no long task of 100 ms or more: ${JSON.stringify(longTasks)}`)
await page.screenshot({ path: `${shots}/streamed.png` })

// a reconnect reads only what the page missed.
await until('B idle', async () => !(await conv(B)).turn_active)
await page.waitForTimeout(500)
const through = (await api('GET', `/api/conversations/${B}/events?turns=1`)).body.through
reads.length = 0
await stopService()
await page.locator('.side-foot .stream-disconnected').waitFor({ timeout: 20000 })
await page.waitForTimeout(2500)
const footTitle = await page.locator('.side-foot .stream').getAttribute('title')
check(/the stream at ws:\/\/127\.0\.0\.1:18470\/api\/stream · \d+s · attempt \d+ · last error: the connection closed \(code \d+\)/.test(footTitle ?? ''), `the footer names what it waits for: "${footTitle}"`)
await startService()
await page.locator('.side-foot .stream-connected').waitFor({ timeout: 30000 })
await page.waitForTimeout(500)
// the stop recorded the runtime's release, which the page may have been given
// before the stream dropped, so it asks from wherever its own through stood.
const since = Number(/events\?since=(\d+)$/.exec(reads[0] ?? '')?.[1] ?? -1)
check(reads.length === 1 && since >= through, `a reconnect reads only what it missed, after ${through} or later: ${reads.join(', ')}`)

// loading says what it waits for: a slow, then failing, opening read.
let held = 0
await page.route('**/events?turns=10', async (route) => { held++; if (held === 1) { await new Promise((r) => setTimeout(r, 2500)); await route.fulfill({ status: 503, contentType: 'application/json', body: '{"code":"unavailable","message":"held by the check"}' }) } else { await new Promise((r) => setTimeout(r, 1500)); await route.continue() } })
await page.evaluate((id) => { location.hash = `#/c/${id}` }, A)
await page.locator('.loading').waitFor({ timeout: 5000 })
await page.waitForTimeout(2000)
const first = await page.locator('.loading').textContent()
check(/Loading history: GET \/api\/conversations\/[0-9a-f]+\/events\?turns=10 · [12]s/.test(first), `the loading line names the request and how long: "${first}"`)
await until('the second attempt', async () => /attempt 2 · last error: held by the check/.test((await page.locator('.loading').textContent().catch(() => '')) ?? ''), 8000)
check(true, `then the attempt and the last error: "${await page.locator('.loading').textContent()}"`)
await page.waitForFunction(inView, 'turn 413', { timeout: 30000, polling: 50 })
await page.unroute('**/events?turns=10')
check(true, 'and the tail lands after the retry')

// connecting says what it waits for: a workspace read that fails.
const fresh = await ctx.newPage()
await fresh.route('**/api/workspace', (route) => route.abort('connectionrefused'))
await fresh.goto(`${url}/`)
await fresh.locator('.waiting-list li').first().waitFor({ timeout: 10000 })
await fresh.waitForTimeout(3500)
const waits = await fresh.locator('.waiting-list li').allTextContents()
check(waits.some((w) => /^GET \/api\/workspace · \d+s · attempt \d+ · last error: /.test(w)), `the connecting view names the read, attempts and error: ${JSON.stringify(waits)}`)
await fresh.screenshot({ path: `${shots}/connecting.png` })
await fresh.close()

check(errors.length === 0, `no page errors: ${errors.join('; ')}`)
await b.close()
say(failures.length ? `${failures.length} failed` : 'all passed')
await stop(failures.length ? 1 : 0)

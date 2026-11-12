// timings on two large generated conversations, on a scratch service at 18470:
// a read that returns nothing new, the tail read (the read a page makes on
// opening), time to first content on a fresh page, and a switch between the
// two. usage: node timing.mjs <chrome> <mega> <config> <claudecfg> <A> <B> <lastTurnA> <lastTurnB> <openingQuery>
import { chromium } from 'playwright-core'
import { spawn } from 'node:child_process'
const [exe, bin, cfg, claudecfg, A, B, turnA, turnB, opening] = process.argv.slice(2)
const url = 'http://127.0.0.1:18470'
const svc = spawn(bin, ['--config', cfg], { stdio: 'ignore', env: { ...process.env, CLAUDE_CONFIG_DIR: claudecfg } })
const stop = (code) => { svc.kill('SIGINT'); setTimeout(() => process.exit(code), 1500) }
process.on('uncaughtException', (e) => { console.log('crashed:', e.stack); stop(2) })
for (let i = 0; i < 300; i++) { try { if ((await fetch(`${url}/api/workspace`)).ok) break } catch {} await new Promise((r) => setTimeout(r, 100)) }
const ms = (t) => `${Math.round(t)} ms`
const timed = async (path) => { const t = performance.now(); const r = await fetch(url + path); const body = await r.arrayBuffer(); return { t: performance.now() - t, bytes: body.byteLength, status: r.status } }
const median = (xs) => xs.sort((a, b) => a - b)[Math.floor(xs.length / 2)]
const ws = await (await fetch(`${url}/api/workspace`)).json()
const last = ws.conversations.find((c) => c.id === A).last_seq
await timed(`/api/conversations/${A}/events?since=${last}`) // opens the log
for (const [what, path] of [['nothing new', `/api/conversations/${A}/events?since=${last}`], ['opening read', `/api/conversations/${A}/events?${opening}`]]) {
  const runs = []
  let bytes = 0
  for (let i = 0; i < 5; i++) { const r = await timed(path); runs.push(r.t); bytes = r.bytes }
  console.log(`${what}: median ${ms(median(runs))} of 5, ${(bytes / 1e6).toFixed(2)} MB (${path.split('?')[1]})`)
}
const b = await chromium.launch({ executablePath: exe })
const inView = (name) => (n) => { const h = [...document.querySelectorAll('h3,h4,h5,h6')].find((e) => e.textContent === n); if (!h) return false; const r = h.getBoundingClientRect(); return r.top >= 0 && r.bottom <= window.innerHeight }
for (let round = 0; round < 3; round++) {
  const ctx = await b.newContext({ viewport: { width: 1440, height: 900 } })
  const page = await ctx.newPage()
  const t0 = performance.now()
  await page.goto(`${url}/#/c/${A}`)
  await page.locator('.events .message').first().waitFor({ timeout: 120000 })
  const first = performance.now() - t0
  await page.waitForFunction(inView(), `turn ${turnA}`, { timeout: 120000, polling: 50 })
  const newest = performance.now() - t0
  const t1 = performance.now()
  await page.evaluate((id) => { location.hash = `#/c/${id}` }, B)
  await page.waitForFunction(inView(), `turn ${turnB}`, { timeout: 120000, polling: 50 })
  const switched = performance.now() - t1
  const t2 = performance.now()
  await page.evaluate((id) => { location.hash = `#/c/${id}` }, A)
  await page.waitForFunction(inView(), `turn ${turnA}`, { timeout: 120000, polling: 50 })
  const back = performance.now() - t2
  console.log(`fresh page ${round + 1}: first content ${ms(first)}, newest reply in view ${ms(newest)}; switch A→B ${ms(switched)}, B→A ${ms(back)}`)
  await ctx.close()
}
await b.close()
stop(0)

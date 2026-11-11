#!/usr/bin/env node
// idle resume probe (2026-11-10): what a resumed codex thread with no turn
// costs and does.
//
//   node idle-resume.mjs --cwd DIR --log logs/x.jsonl [--hold SECONDS] [--thread ID]
//
// with no --thread it first seeds a fresh thread with one short turn on its
// own app-server, which then exits. a second app-server resumes the thread
// with thread/resume and sends no turn/start, holding it for --hold seconds
// (120 by default). it records every frame, the rollout file's size, and the
// memory of the app-server and everything below it, then closes stdin and
// times the exit.
import { spawn } from 'node:child_process'
import { createWriteStream, existsSync, readFileSync, readdirSync, statSync } from 'node:fs'
import { resolve } from 'node:path'
import { createInterface } from 'node:readline'

const argv = process.argv.slice(2)
const opt = {}
for (let i = 0; i < argv.length; i++) {
  if (!argv[i].startsWith('--')) continue
  const next = argv[i + 1]
  if (next === undefined || next.startsWith('--')) opt[argv[i].slice(2)] = true
  else { opt[argv[i].slice(2)] = next; i++ }
}
const cwd = resolve(opt.cwd ?? '.')
const hold = Number(opt.hold ?? 120) * 1000
const log = createWriteStream(opt.log ?? `logs/idle-resume-${Date.now()}.jsonl`)
const t0 = Date.now()
const rec = (kind, data) => log.write(JSON.stringify({ ms: Date.now() - t0, kind, ...data }) + '\n')
const say = (s) => process.stderr.write(`[idle +${((Date.now() - t0) / 1000).toFixed(1)}s] ${s}\n`)
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

const tree = (root) => {
  const procs = []
  for (const name of readdirSync('/proc')) {
    if (!/^\d+$/.test(name)) continue
    try {
      const stat = readFileSync(`/proc/${name}/stat`, 'utf8')
      procs.push({ pid: Number(name), ppid: Number(stat.slice(stat.lastIndexOf(')') + 2).split(' ')[1]) })
    } catch {}
  }
  const below = new Set([root])
  let grew = true
  while (grew) {
    grew = false
    for (const p of procs) if (!below.has(p.pid) && below.has(p.ppid)) { below.add(p.pid); grew = true }
  }
  const kb = (pid, file, key) => { try { const m = readFileSync(`/proc/${pid}/${file}`, 'utf8').match(new RegExp(`^${key}:\\s+(\\d+) kB`, 'm')); return m ? Number(m[1]) : null } catch { return null } }
  return [...below].map((pid) => {
    let cmd = ''
    try { cmd = readFileSync(`/proc/${pid}/cmdline`, 'utf8').split('\0').filter(Boolean).slice(0, 3).join(' ').slice(0, 120) } catch {}
    return { pid, cmd, rssKb: kb(pid, 'status', 'VmRSS'), pssKb: kb(pid, 'smaps_rollup', 'Pss') }
  })
}

function server() {
  const child = spawn('codex', ['app-server'], { cwd, stdio: ['pipe', 'pipe', 'pipe'] })
  let nextId = 1
  const waiting = new Map()
  const frames = []
  const send = (frame) => { rec('send', { frame }); child.stdin.write(JSON.stringify(frame) + '\n') }
  const request = (method, params) => new Promise((res, rej) => { const id = nextId++; waiting.set(id, { res, rej }); send({ id, method, params }) })
  createInterface({ input: child.stdout }).on('line', (line) => {
    if (!line.trim()) return
    const msg = JSON.parse(line)
    rec('recv', { frame: msg })
    frames.push({ at: Date.now() - t0, method: msg.method ?? null, id: msg.id ?? null })
    if (msg.id !== undefined && msg.method === undefined) {
      const w = waiting.get(msg.id)
      if (w) { waiting.delete(msg.id); msg.error ? w.rej(new Error(JSON.stringify(msg.error))) : w.res(msg.result) }
      return
    }
    if (msg.id !== undefined && msg.method) send({ id: msg.id, result: { decision: 'decline' } })
  })
  child.stderr.on('data', (d) => rec('stderr', { text: String(d) }))
  const exited = new Promise((r) => child.on('exit', (code, signal) => r({ code, signal, at: Date.now() })))
  const init = async () => {
    await request('initialize', { clientInfo: { name: 'roundhouse-probe', version: '0' }, capabilities: { experimentalApi: true } })
    send({ method: 'initialized', params: {} })
  }
  return { child, request, frames, exited, init }
}

const summary = { cwd, hold: hold / 1000 }
let thread = opt.thread
if (!thread) {
  const seed = server()
  await seed.init()
  const r = await seed.request('thread/start', { cwd })
  thread = r.thread.id
  let done = false
  const watch = setInterval(() => { if (seed.frames.some((f) => f.method === 'turn/completed')) done = true }, 100)
  await seed.request('turn/start', { threadId: thread, input: [{ type: 'text', text: 'Reply with the single word SEEDED. Do not run any commands.' }] })
  while (!done) await sleep(100)
  clearInterval(watch)
  seed.child.stdin.end()
  await seed.exited
  say(`seeded ${thread}`)
}
summary.thread = thread

const s = server()
const started = Date.now()
await s.init()
const r = await s.request('thread/resume', { threadId: thread })
const rollout = r.thread.path
const size = () => (existsSync(rollout) ? { bytes: statSync(rollout).size, mtime: statSync(rollout).mtime.toISOString() } : { exists: false })
summary.resumed = { ms: Date.now() - started, model: r.model, effort: r.reasoningEffort, approvalPolicy: r.approvalPolicy, priorTurns: (r.thread.turns ?? []).length, status: r.thread.status, cliVersion: r.thread.cliVersion, rollout }
summary.rolloutAtResume = size()
say(`resumed ${JSON.stringify(summary.resumed)}`)
const memory = (label) => { const procs = tree(s.child.pid); rec('memory', { label, procs }); say(`memory ${label}: ${procs.map((p) => `${p.cmd.split(' ')[0].split('/').pop()} rss ${Math.round(p.rssKb / 1024)}M pss ${Math.round((p.pssKb ?? 0) / 1024)}M`).join('; ')}`); return procs }
summary.memory = {}
let last = 0
for (const sec of [5, 30, 60, 120, 300, 600].filter((x) => x * 1000 <= hold)) {
  await sleep(sec * 1000 - last)
  last = sec * 1000
  summary.memory[`at${sec}s`] = memory(`${sec}s`)
}
if (hold > last) await sleep(hold - last)
summary.rolloutIdle = size()
summary.framesWhileIdle = s.frames.filter((f) => f.method)
const pids = tree(s.child.pid).map((p) => p.pid)
const closing = Date.now()
s.child.stdin.end()
const exit = await Promise.race([s.exited, sleep(60000).then(() => null)])
for (let i = 0; i < 100 && pids.some((p) => existsSync(`/proc/${p}`)); i++) await sleep(100)
summary.close = { exit: exit && { code: exit.code, signal: exit.signal, ms: exit.at - closing }, processesGoneMs: Date.now() - closing, stillRunning: pids.filter((p) => existsSync(`/proc/${p}`)) }
summary.rolloutAfter = size()
rec('summary', summary)
log.end()
console.log(JSON.stringify(summary, null, 2))
process.exit(0)

#!/usr/bin/env node
// mcp status probe (2026-11-12): what codex app-server says of its MCP
// servers, with and without a thread, and when.
//
//   node mcp-status.mjs --cwd DIR --log logs/x.jsonl [--other DIR] [--kill]
//
// one app-server, started in --cwd. it asks mcpServerStatus/list with no
// thread loaded, then starts an ephemeral thread in --cwd (no rollout) and
// lists with that thread at 0 to 4 s, recording every
// mcpServer/startupStatus/updated notification and when it came. --other
// starts a second ephemeral thread in another checkout, to see whether the
// list differs per thread. --kill then kills the fixture server's process
// below this probe and lists again. it never logs in, refreshes, enables or
// disables a server, and writes no configuration.
//
// the summary on stdout names the servers the fixtures define (fixture,
// project, broken); the operator's own are counted by status. the full log
// goes to --log (logs/ is gitignored).
import { spawn } from 'node:child_process'
import { createWriteStream, readFileSync, readdirSync } from 'node:fs'
import { resolve } from 'node:path'
import { createInterface } from 'node:readline'

const argv = process.argv.slice(2)
const opt = {}
for (let i = 0; i < argv.length; i++) {
  if (!argv[i].startsWith('--')) continue
  const next = argv[i + 1]
  if (next === undefined || next.startsWith('--')) opt[argv[i].slice(2)] = true
  else {
    opt[argv[i].slice(2)] = next
    i++
  }
}
const cwd = resolve(opt.cwd ?? '.')
const log = createWriteStream(opt.log ?? `logs/mcp-status-${Date.now()}.jsonl`)
const t0 = Date.now()
const rec = (kind, data) => log.write(JSON.stringify({ ms: Date.now() - t0, kind, ...data }) + '\n')
const say = (s) => process.stderr.write(`[mcp +${((Date.now() - t0) / 1000).toFixed(1)}s] ${s}\n`)
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
const named = new Set(['fixture', 'project', 'broken'])

const child = spawn('codex', ['app-server'], { cwd, stdio: ['pipe', 'pipe', 'pipe'] })
let nextId = 1
const waiting = new Map()
const notes = []
const send = (frame) => {
  rec('send', { frame })
  child.stdin.write(JSON.stringify(frame) + '\n')
}
const request = (method, params) =>
  new Promise((res, rej) => {
    const id = nextId++
    waiting.set(id, { res, rej })
    send({ id, method, params })
  })
createInterface({ input: child.stdout }).on('line', (line) => {
  if (!line.trim()) return
  const msg = JSON.parse(line)
  rec('recv', { frame: msg })
  if (msg.id !== undefined && msg.method === undefined) {
    const w = waiting.get(msg.id)
    if (w) {
      waiting.delete(msg.id)
      msg.error ? w.rej(new Error(JSON.stringify(msg.error))) : w.res(msg.result)
    }
    return
  }
  if (msg.method?.startsWith('mcpServer')) {
    const p = msg.params ?? {}
    notes.push({ at: Date.now() - t0, method: msg.method, name: named.has(p.name) ? p.name : '(operator)', status: p.status ?? null, error: p.error ?? null, failureReason: p.failureReason ?? null, thread: p.threadId ? 'yes' : 'no', keys: Object.keys(p).sort() })
  }
  if (msg.id !== undefined && msg.method) send({ id: msg.id, result: { decision: 'decline' } })
})
child.stderr.on('data', (d) => rec('stderr', { text: String(d) }))

const summarise = (data) => {
  const out = { counts: {}, named: {}, keys: new Set(), toolKeys: new Set() }
  for (const s of data) {
    Object.keys(s).forEach((k) => out.keys.add(k))
    const tools = Object.values(s.tools ?? {})
    tools.forEach((t) => Object.keys(t).forEach((k) => out.toolKeys.add(k)))
    const brief = { auth: s.authStatus, runtime: s.runtimeStatus ?? null, tools: tools.length, described: tools.filter((t) => t.description).length, toolsError: s.toolsError ?? null }
    if (named.has(s.name)) out.named[s.name] = { ...brief, toolNames: tools.map((t) => t.name), descriptions: tools.map((t) => t.description ?? null) }
    else {
      const key = `${s.authStatus}/${s.runtimeStatus ?? 'null'}`
      out.counts[key] = (out.counts[key] ?? 0) + 1
    }
  }
  return { ...out, keys: [...out.keys].sort(), toolKeys: [...out.toolKeys].sort() }
}
const lists = []
const list = async (label, threadId) => {
  const asked = Date.now()
  try {
    const r = await request('mcpServerStatus/list', threadId ? { threadId } : {})
    lists.push({ label, at: asked - t0, answeredIn: Date.now() - asked, total: r.data.length, next: r.nextCursor ?? null, ...summarise(r.data) })
  } catch (e) {
    lists.push({ label, at: asked - t0, error: String(e) })
  }
}

await request('initialize', { clientInfo: { name: 'mega-probe', version: '0' }, capabilities: { experimentalApi: true } })
send({ method: 'initialized', params: {} })
const version = await request('config/read', { cwd }).then(() => null, () => null)
await list('no thread', null)
await list('no thread, again', null)

const started = Date.now()
const t1 = (await request('thread/start', { cwd, ephemeral: true })).thread.id
say(`thread 1 started in ${Date.now() - started} ms`)
for (const ms of [0, 200, 500, 1000, 2000, 4000]) {
  const due = started + ms
  if (Date.now() < due) await sleep(due - Date.now())
  await list(`thread 1 +${ms}`, t1)
}
await list('no thread, with thread 1 loaded', null)

if (opt.other) {
  const other = resolve(opt.other)
  const s2 = Date.now()
  const t2 = (await request('thread/start', { cwd: other, ephemeral: true })).thread.id
  for (const ms of [0, 1000, 3000]) {
    const due = s2 + ms
    if (Date.now() < due) await sleep(due - Date.now())
    await list(`thread 2 (other) +${ms}`, t2)
  }
  await list('thread 1 again', t1)
}

// the fixture server's process: node running that script, below this probe.
const fixturePids = () => {
  const procs = []
  for (const name of readdirSync('/proc')) {
    if (!/^\d+$/.test(name)) continue
    try {
      const stat = readFileSync(`/proc/${name}/stat`, 'utf8')
      const args = readFileSync(`/proc/${name}/cmdline`, 'utf8').split('\0')
      procs.push({ pid: Number(name), ppid: Number(stat.slice(stat.lastIndexOf(')') + 2).split(' ')[1]), args })
    } catch {}
  }
  const below = new Set([process.pid])
  let grew = true
  while (grew) {
    grew = false
    for (const p of procs)
      if (!below.has(p.pid) && below.has(p.ppid)) {
        below.add(p.pid)
        grew = true
      }
  }
  return procs.filter((p) => below.has(p.pid) && /(^|\/)node$/.test(p.args[0]) && (p.args[1] ?? '').endsWith('fixture-server.mjs')).map((p) => p.pid)
}
if (opt.kill) {
  const pids = fixturePids()
  for (const pid of pids) {
    try {
      process.kill(pid, 'SIGKILL')
    } catch {}
  }
  const killed = Date.now()
  say(`killed ${pids.length} fixture server process(es)`)
  for (const ms of [200, 1000, 3000]) {
    const due = killed + ms
    if (Date.now() < due) await sleep(due - Date.now())
    await list(`killed +${ms}`, t1)
  }
}

child.stdin.end()
await new Promise((r) => child.on('exit', r))
console.log(JSON.stringify({ cwd, version, lists, notes }, null, 1))
log.end()

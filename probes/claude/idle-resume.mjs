#!/usr/bin/env node
// idle resume probe (2026-11-10): what a resumed claude session does with no
// prompt.
//
//   node idle-resume.mjs --cwd DIR --log logs/x.jsonl [--hold SECONDS] [--then-prompt]
//                        [--session UUID] [--mcp SCRIPT] [--exec PATH] [--facts] [--set-mode MODE]
//                        [--hook] [--mode MODE]
//
// with no --session it first seeds a fresh session with one short turn. it
// then resumes that session with streaming input and sends nothing, holding it
// for --hold seconds (300 by default). throughout it records every message,
// whether system/init arrives, mcpServerStatus() (with --mcp, a stdio MCP
// server passed for this query only), the transcript's size and mtime, and
// the memory of the processes the query started. at the end it either closes
// input and times the exit, or with --then-prompt sends one prompt first and
// records which session answers. --facts asks getContextUsage for the model
// while idle; --set-mode switches the permission mode while idle; --hook
// records what a SessionStart callback is told; --mode passes a launch mode.
import { createWriteStream, readFileSync, readdirSync, statSync, existsSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { homedir } from 'node:os'
import { randomUUID } from 'node:crypto'
import { query } from '@anthropic-ai/claude-agent-sdk'

const argv = process.argv.slice(2)
const opt = {}
for (let i = 0; i < argv.length; i++) {
  if (!argv[i].startsWith('--')) continue
  const next = argv[i + 1]
  if (next === undefined || next.startsWith('--')) opt[argv[i].slice(2)] = true
  else { opt[argv[i].slice(2)] = next; i++ }
}
const cwd = resolve(opt.cwd ?? '.')
const hold = Number(opt.hold ?? 300) * 1000
const exec = opt.exec ?? '/home/michael/.local/share/claude/versions/2.1.318'
const log = createWriteStream(opt.log ?? `logs/idle-resume-${Date.now()}.jsonl`)
const t0 = Date.now()
const rec = (kind, data) => log.write(JSON.stringify({ ms: Date.now() - t0, kind, ...data }) + '\n')
const say = (s) => process.stderr.write(`[idle +${((Date.now() - t0) / 1000).toFixed(1)}s] ${s}\n`)
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
const deny = async (_tool, _input, o) => ({ behavior: 'deny', message: 'idle probe: no tools', toolUseID: o.toolUseID })

// the transcript claude keeps for this checkout.
const transcriptDir = join(process.env.CLAUDE_CONFIG_DIR ?? join(homedir(), '.claude'), 'projects', cwd.replace(/[^A-Za-z0-9]/g, '-'))
const transcript = (id) => {
  const file = join(transcriptDir, `${id}.jsonl`)
  if (!existsSync(file)) return { exists: false }
  const st = statSync(file)
  return { exists: true, bytes: st.size, mtime: st.mtime.toISOString(), lines: readFileSync(file, 'utf8').trim().split('\n').length }
}

// memory of every process below this one, by command.
const children = () => {
  const procs = []
  for (const name of readdirSync('/proc')) {
    if (!/^\d+$/.test(name)) continue
    try {
      const stat = readFileSync(`/proc/${name}/stat`, 'utf8')
      const ppid = Number(stat.slice(stat.lastIndexOf(')') + 2).split(' ')[1])
      procs.push({ pid: Number(name), ppid })
    } catch {}
  }
  const below = new Set([process.pid])
  let grew = true
  while (grew) {
    grew = false
    for (const p of procs) if (!below.has(p.pid) && below.has(p.ppid)) { below.add(p.pid); grew = true }
  }
  const kb = (pid, file, key) => { try { const m = readFileSync(`/proc/${pid}/${file}`, 'utf8').match(new RegExp(`^${key}:\\s+(\\d+) kB`, 'm')); return m ? Number(m[1]) : null } catch { return null } }
  return [...below].map((pid) => {
    let cmd = ''
    try { cmd = readFileSync(`/proc/${pid}/cmdline`, 'utf8').split('\0').filter(Boolean).slice(0, 3).join(' ').slice(0, 120) } catch {}
    return { pid, self: pid === process.pid, cmd, rssKb: kb(pid, 'status', 'VmRSS'), pssKb: kb(pid, 'smaps_rollup', 'Pss') }
  })
}
const memory = (label) => { const procs = children(); rec('memory', { label, procs }); say(`memory ${label}: ${procs.map((p) => `${p.self ? 'probe' : p.cmd.split(' ')[0].split('/').pop()} rss ${Math.round(p.rssKb / 1024)}M pss ${Math.round((p.pssKb ?? 0) / 1024)}M`).join('; ')}`); return procs }

const summary = { cwd, hold: hold / 1000 }

// seed a fresh session with one short turn, unless one is named.
let session = opt.session
if (!session) {
  session = randomUUID()
  const seed = query({ prompt: 'Reply with the single word SEEDED.', options: { cwd, pathToClaudeCodeExecutable: exec, sessionId: session, permissionMode: 'default', canUseTool: deny } })
  for await (const m of seed) { rec('seed', { msg: m }); if (m.type === 'result') summary.seed = { result: m.subtype, text: m.result } }
  say(`seeded ${session}`)
}
summary.session = session
summary.transcriptBefore = transcript(session)

// resume it with nothing to say.
let wake = null
const queue = []
let closed = false
const push = (m) => { queue.push(m); if (wake) { const w = wake; wake = null; w() } }
async function* input() {
  while (true) {
    if (queue.length) { yield queue.shift(); continue }
    if (closed) return
    await new Promise((r) => { wake = r })
  }
}
const resumedAt = Date.now()
const options = { cwd, pathToClaudeCodeExecutable: exec, resume: session, canUseTool: deny, includePartialMessages: true }
if (opt.hook) {
  // a SessionStart callback, observing only: what claude itself reports about
  // the session as it starts, with no turn.
  options.hooks = { SessionStart: [{ hooks: [async (input) => { summary.sessionStart = { at: Date.now() - resumedAt, source: input.source, session: input.session_id, model: input.model ?? null, permissionMode: input.permission_mode ?? null }; rec('session_start', { input }); return {} }] }] }
}
if (opt.mcp) options.mcpServers = { fixture: { type: 'stdio', command: 'node', args: [resolve(opt.mcp)] } }
// --mode passes a permission mode; with none, claude's settings choose.
if (typeof opt.mode === 'string') options.permissionMode = opt.mode
const q = query({ prompt: input(), options })
const messages = []
let ended = null
const reading = (async () => {
  for await (const m of q) {
    messages.push({ at: Date.now() - resumedAt, type: m.type, subtype: m.subtype ?? null, session: m.session_id ?? null, model: m.model ?? null, permissionMode: m.permissionMode ?? null })
    rec('msg', { msg: m })
    if (m.type !== 'stream_event') say(`message ${m.type}${m.subtype ? '/' + m.subtype : ''}`)
  }
  ended = Date.now()
})()

const init = await q.initializationResult().then((r) => ({ ms: Date.now() - resumedAt, version: r.claude_code_version ?? null, models: r.models?.length ?? null }), (e) => ({ error: String(e) }))
summary.initializationResult = init
say(`initializationResult ${JSON.stringify(init)}`)
const mcp = async (label) => { const s = await q.mcpServerStatus().then((r) => r.map((x) => ({ name: x.name, status: x.status, tools: x.tools?.length ?? null })), (e) => ({ error: String(e) })); rec('mcp', { label, status: s }); say(`mcp ${label}: ${JSON.stringify(s)}`); return s }
summary.mcp = { early: await mcp('early') }
if (opt.facts) {
  // what the idle session can report without a turn: the model from
  // getContextUsage's summary (local estimates), timed.
  const at = Date.now()
  summary.contextUsage = await q.getContextUsage({ detail: 'summary' }).then((r) => ({ ms: Date.now() - at, model: r.model, totalTokens: r.totalTokens, maxTokens: r.maxTokens }), (e) => ({ error: String(e) }))
  say(`getContextUsage ${JSON.stringify(summary.contextUsage)}`)
}
if (typeof opt['set-mode'] === 'string') {
  // a mode switch while idle: whether it is confirmed, and what the stream says.
  const at = Date.now()
  const n = messages.length
  summary.setMode = await q.setPermissionMode(opt['set-mode']).then(() => ({ ok: true, ms: Date.now() - at }), (e) => ({ error: String(e) }))
  await sleep(1500)
  summary.setMode.messages = messages.slice(n).filter((m) => m.type !== 'stream_event')
  say(`setPermissionMode ${JSON.stringify(summary.setMode)}`)
}
await sleep(3000)
summary.mcp.at3s = await mcp('3s')
summary.memory = { at5s: (await sleep(2000), memory('5s')) }
const marks = [30, 60, 120, 300, 600, 900, 1800].map((s) => s * 1000).filter((ms) => ms <= hold)
let last = 5000
for (const ms of marks) {
  await sleep(ms - last)
  last = ms
  summary.memory[`at${ms / 1000}s`] = memory(`${ms / 1000}s`)
  rec('transcript', { label: `${ms / 1000}s`, ...transcript(session) })
}
if (hold > last) await sleep(hold - last)
summary.mcp.atEnd = await mcp('end')
summary.transcriptIdle = transcript(session)
summary.messagesWhileIdle = messages.filter((m) => m.type !== 'stream_event')

if (opt['then-prompt']) {
  const before = messages.length
  push({ type: 'user', message: { role: 'user', content: 'Reply with the single word RESUMED.' }, parent_tool_use_id: null })
  while (!messages.slice(before).some((m) => m.type === 'result')) await sleep(100)
  summary.prompted = messages.slice(before).filter((m) => m.type !== 'stream_event')
  summary.memory.afterPrompt = memory('after prompt')
}
// close input on the session and time the exit.
const closing = Date.now()
const pids = children().filter((p) => !p.self).map((p) => p.pid)
closed = true
if (wake) { const w = wake; wake = null; w() }
const before = messages.length
await Promise.race([reading, sleep(120000)])
summary.close = {
  iteratorEndedMs: ended ? ended - closing : null,
  messagesAfterClose: messages.slice(before).filter((m) => m.type !== 'stream_event'),
}
for (let i = 0; i < 100 && pids.some((p) => existsSync(`/proc/${p}`)); i++) await sleep(100)
summary.close.processesGoneMs = Date.now() - closing
summary.close.stillRunning = pids.filter((p) => existsSync(`/proc/${p}`))
summary.transcriptAfter = transcript(session)
rec('summary', summary)
log.end()
console.log(JSON.stringify(summary, null, 2))
process.exit(0)

#!/usr/bin/env node
// model and effort probe (2026-11-10): what claude code reports about models
// and effort through the agent sdk, and whether the runtime switches take.
//
//   node models.mjs --cwd DIR --log logs/x.jsonl [--model M] [--effort E] [--exec PATH] [--idle-only]
//
// --idle-only asks for the model list under a fresh session id, closes input
// without a prompt, and reports how long that took and whether the session
// left a transcript or shows up in listSessions.
//
// starts a session with streaming input and no prompt; times supportedModels()
// and initializationResult() before any turn; then runs three marker turns:
// as launched, after setModel('sonnet'), and after
// applyFlagSettings({effortLevel: 'low'}), recording each turn's system/init
// model and effort and any other message that mentions effort.
import { createWriteStream } from 'node:fs'
import { resolve } from 'node:path'
import { randomUUID } from 'node:crypto'
import { existsSync, readdirSync } from 'node:fs'
import { join } from 'node:path'
import { homedir } from 'node:os'
import { listSessions, query } from '@anthropic-ai/claude-agent-sdk'

const argv = process.argv.slice(2)
const opt = {}
for (let i = 0; i < argv.length; i++) {
  if (!argv[i].startsWith('--')) continue
  const next = argv[i + 1]
  if (next === undefined || next.startsWith('--')) opt[argv[i].slice(2)] = true
  else { opt[argv[i].slice(2)] = next; i++ }
}
const log = createWriteStream(opt.log ?? `logs/models-${Date.now()}.jsonl`)
const t0 = Date.now()
const rec = (kind, data) => log.write(JSON.stringify({ ms: Date.now() - t0, kind, ...data }) + '\n')
const say = (s) => process.stderr.write(`[models +${Date.now() - t0}ms] ${s}\n`)

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

const options = {
  cwd: resolve(opt.cwd ?? '.'),
  pathToClaudeCodeExecutable: opt.exec ?? '/home/michael/.local/share/claude/versions/2.1.318',
  permissionMode: 'default',
  canUseTool: async (_tool, _input, o) => ({ behavior: 'deny', message: 'models probe: no tools', toolUseID: o.toolUseID }),
}
const sessionId = randomUUID()
if (opt['idle-only']) options.sessionId = sessionId
if (opt.model) options.model = opt.model
if (opt.effort) options.effort = opt.effort
rec('options', { model: options.model ?? null, effort: options.effort ?? null })
const q = query({ prompt: input(), options })
const summary = { options: { model: options.model ?? null, effort: options.effort ?? null }, turns: [] }

const timed = async (name, fn) => {
  const start = Date.now()
  try {
    const r = await Promise.race([fn(), new Promise((_, rej) => setTimeout(() => rej(new Error('timeout 60s')), 60000))])
    say(`${name} answered after ${Date.now() - start} ms`)
    return { ok: true, ms: Date.now() - start, at: Date.now() - t0, result: r }
  } catch (e) {
    say(`${name} failed after ${Date.now() - start} ms: ${e}`)
    return { ok: false, ms: Date.now() - start, error: String(e) }
  }
}

const idleModels = await timed('supportedModels (idle)', () => q.supportedModels())
summary.idleModels = idleModels.ok
  ? { ms: idleModels.ms, at: idleModels.at, models: idleModels.result.map((m) => ({ value: m.value, resolvedModel: m.resolvedModel, displayName: m.displayName, supportsEffort: m.supportsEffort, supportedEffortLevels: m.supportedEffortLevels })) }
  : idleModels
rec('supported_models', { result: idleModels })
const init = await timed('initializationResult (idle)', () => q.initializationResult())
summary.initKeys = init.ok ? Object.keys(init.result) : init
rec('initialization_result', { result: init })

if (opt['idle-only']) {
  closed = true
  if (wake) { const w = wake; wake = null; w() }
  for await (const m of q) rec('msg', { msg: m })
  summary.idleOnly = { sessionId, totalMs: Date.now() - t0 }
  const transcripts = []
  const projects = join(homedir(), '.claude', 'projects')
  for (const dir of readdirSync(projects)) if (existsSync(join(projects, dir, `${sessionId}.jsonl`))) transcripts.push(join(dir, `${sessionId}.jsonl`))
  summary.idleOnly.transcripts = transcripts
  const listed = await listSessions({ dir: options.cwd })
  summary.idleOnly.listed = listed.some((x) => x.sessionId === sessionId)
  say(`idle only: closed after ${summary.idleOnly.totalMs} ms; transcripts ${JSON.stringify(transcripts)}; listed ${summary.idleOnly.listed}`)
  rec('summary', summary)
  log.end()
  console.log(JSON.stringify(summary, null, 2))
  process.exit(0)
}

const steps = [
  { name: 'as launched', before: null },
  { name: "after setModel('sonnet')", before: () => q.setModel('sonnet') },
  { name: "after applyFlagSettings({effortLevel:'low'})", before: () => q.applyFlagSettings({ effortLevel: 'low' }) },
]
let step = -1
let current = null
const next = async () => {
  step++
  if (step >= steps.length) { closed = true; if (wake) { const w = wake; wake = null; w() } return }
  const s = steps[step]
  current = { name: s.name, before: null, init: null, effortMentions: [] }
  summary.turns.push(current)
  if (s.before) {
    const r = await timed(s.name.replace('after ', ''), s.before)
    current.before = { ok: r.ok, ms: r.ms, error: r.error ?? null }
  }
  say(`turn ${step + 1}: ${s.name}`)
  push({ type: 'user', message: { role: 'user', content: 'Reply with the word ALPHA-1a2b. Nothing else.' }, parent_tool_use_id: null })
}
await next()
for await (const m of q) {
  rec('msg', { msg: m })
  const text = JSON.stringify(m)
  if (m.type === 'system' && m.subtype === 'init') {
    current.init = { model: m.model, effort: m.effort === undefined ? '(absent)' : m.effort, permissionMode: m.permissionMode }
    say(`init model=${m.model} effort=${current.init.effort}`)
  } else if (/effort/i.test(text) && current) {
    current.effortMentions.push({ type: m.type, subtype: m.subtype ?? null, excerpt: text.slice(Math.max(0, text.search(/effort/i) - 80), text.search(/effort/i) + 120) })
  }
  if (m.type === 'assistant') current.model = m.message?.model ?? null
  if (m.type === 'result') { current.result = m.subtype; await next() }
}
summary.totalMs = Date.now() - t0
rec('summary', summary)
log.end()
console.log(JSON.stringify(summary, null, 2))

#!/usr/bin/env node
// background jobs probe (2026-11-10): what claude code reports on the stream
// about work that outlives a turn.
//
//   node background.mjs --cwd DIR --log logs/x.jsonl [--exec PATH] [--scenario bash|two|agent] [--linger SECONDS]
//
// starts a session with streaming input, sends one prompt that backgrounds
// work, answers tool permission only for exactly the commands the scenario
// names, and keeps reading after the turn's result for --linger seconds (90
// by default) to see the job finish and whatever that does to the session:
// task frames, background_tasks_changed snapshots, a turn nobody prompted.
// then it closes input and times how long the cli takes to exit.
//
//   bash   one Bash call with run_in_background: sleep 15 && echo BG-ONE-DONE
//   two    two background Bash calls at once (sleep 10, sleep 20), to see each
//          snapshot of the list, down to the last
//   agent  one subagent launched with run_in_background, to see whether it
//          appears as a task and with what task_type
import { createWriteStream } from 'node:fs'
import { resolve } from 'node:path'
import { query } from '@anthropic-ai/claude-agent-sdk'

const argv = process.argv.slice(2)
const opt = {}
for (let i = 0; i < argv.length; i++) {
  if (!argv[i].startsWith('--')) continue
  const next = argv[i + 1]
  if (next === undefined || next.startsWith('--')) opt[argv[i].slice(2)] = true
  else { opt[argv[i].slice(2)] = next; i++ }
}
const scenario = opt.scenario ?? 'bash'
const linger = Number(opt.linger ?? 90) * 1000
const log = createWriteStream(opt.log ?? `logs/background-${scenario}-${Date.now()}.jsonl`)
const t0 = Date.now()
const rec = (kind, data) => log.write(JSON.stringify({ ms: Date.now() - t0, kind, ...data }) + '\n')
const say = (s) => process.stderr.write(`[background +${((Date.now() - t0) / 1000).toFixed(1)}s] ${s}\n`)

const prompts = {
  bash: 'Using the Bash tool with run_in_background set to true, run exactly `sleep 15 && echo BG-ONE-DONE`. Do not wait for it. Reply BG-STARTED and stop. Nothing else.',
  two: 'Using the Bash tool with run_in_background set to true, start these two commands as two separate background Bash calls: `sleep 10 && echo BG-A-DONE` and `sleep 20 && echo BG-B-DONE`. Do not wait for them. Reply BG-TWO-STARTED and stop. Nothing else.',
  agent: 'Use the Agent tool with run_in_background set to true to launch one general-purpose subagent whose whole task is: reply with exactly SUB-DONE and nothing else, using no tools. Do not wait for it. Reply SUB-STARTED and stop. Nothing else.',
}
// the commands each scenario may run; anything else is denied.
const allowed = {
  bash: [/^sleep 15 && echo BG-ONE-DONE$/],
  two: [/^sleep 10 && echo BG-A-DONE$/, /^sleep 20 && echo BG-B-DONE$/],
  agent: [],
}

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

const decisions = []
const options = {
  cwd: resolve(opt.cwd ?? '.'),
  pathToClaudeCodeExecutable: opt.exec ?? '/home/michael/.local/share/claude/versions/2.1.318',
  permissionMode: 'default',
  canUseTool: async (tool, input, o) => {
    let allow = false
    if (tool === 'Bash') allow = allowed[scenario].some((re) => re.test(String(input.command ?? '').trim()))
    if (tool === 'Agent' || tool === 'Task') allow = scenario === 'agent'
    decisions.push({ ms: Date.now() - t0, tool, input, allow })
    say(`canUseTool ${tool} ${JSON.stringify(input).slice(0, 120)} -> ${allow ? 'allow' : 'deny'}`)
    return allow ? { behavior: 'allow', updatedInput: input, toolUseID: o.toolUseID } : { behavior: 'deny', message: 'background probe: not this one', toolUseID: o.toolUseID }
  },
}
const q = query({ prompt: input(), options })
push({ type: 'user', message: { role: 'user', content: prompts[scenario] }, parent_tool_use_id: null })

// a short line per message, for the summary; the log keeps everything.
const brief = (m) => {
  if (m.type === 'system') {
    const o = { type: 'system', subtype: m.subtype }
    for (const k of ['task_id', 'task_type', 'description', 'is_backgrounded', 'tool_use_id', 'status', 'summary', 'output_file']) if (m[k] !== undefined) o[k] = m[k]
    if (m.patch) o.patch = m.patch
    if (m.tasks) o.tasks = m.tasks.map((t) => ({ task_id: t.task_id, task_type: t.task_type, description: t.description, keys: Object.keys(t) }))
    return o
  }
  if (m.type === 'result') return { type: 'result', subtype: m.subtype, num_turns: m.num_turns }
  if (m.type === 'assistant') return { type: 'assistant', content: (m.message?.content ?? []).map((c) => (c.type === 'text' ? `text:${c.text.slice(0, 80)}` : `${c.type}:${c.name ?? ''}${c.input ? ' ' + JSON.stringify(c.input).slice(0, 100) : ''}`)) }
  if (m.type === 'user') {
    const content = m.message?.content
    const items = Array.isArray(content) ? content.map((c) => (c.type === 'text' ? `text:${c.text.slice(0, 160)}` : c.type)) : [`text:${String(content).slice(0, 160)}`]
    return { type: 'user', items, isMeta: m.isMeta ?? m.is_meta ?? null, origin: m.origin ?? null }
  }
  return { type: m.type }
}

const timeline = []
let results = 0
let lingerTimer = null
const closeInput = () => { closed = true; if (wake) { const w = wake; wake = null; w() } }
for await (const m of q) {
  rec('msg', { msg: m })
  if (m.type === 'stream_event') continue
  const b = { s: +((Date.now() - t0) / 1000).toFixed(1), ...brief(m) }
  timeline.push(b)
  say(JSON.stringify(b).slice(0, 220))
  if (m.type === 'result') {
    results++
    if (results === 1) {
      say(`first turn ended; lingering ${linger / 1000}s`)
      lingerTimer = setTimeout(() => { say('closing input'); timeline.push({ s: +((Date.now() - t0) / 1000).toFixed(1), type: 'probe', note: 'input closed' }); closeInput() }, linger)
    }
  }
}
clearTimeout(lingerTimer)
const summary = { scenario, totalSeconds: (Date.now() - t0) / 1000, results, decisions, timeline }
rec('summary', summary)
log.end()
console.log(JSON.stringify(summary, null, 2))

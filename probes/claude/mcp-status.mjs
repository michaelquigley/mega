#!/usr/bin/env node
// mcp status probe (2026-11-12): what a claude session says of its MCP
// servers, and when.
//
//   node mcp-status.mjs --cwd DIR --log logs/x.jsonl [--mcp SCRIPT] [--kill]
//                       [--add-broken] [--prompt] [--exec PATH]
//
// it starts a fresh session with streaming input and no prompt, and polls
// mcpServerStatus() from the first moment, at 0 to 12 s, recording each
// server's status, scope, source, error and tool count, and every message on
// the stream meanwhile. --mcp passes a stdio MCP server for this query only.
// --kill then kills that server's process and polls again. --add-broken calls
// setMcpServers with the fixture and a server whose command does not exist,
// and polls again. --prompt sends one short prompt last, to record what
// system/init says of the servers and their tools. it never toggles,
// reconnects or authenticates a server, and writes nothing to any settings.
//
// the summary on stdout names only the passed fixture server; the operator's
// own servers are counted by status and scope. the full log goes to --log
// (logs/ is gitignored).
import { createWriteStream, readFileSync, readdirSync } from 'node:fs'
import { resolve } from 'node:path'
import { randomUUID } from 'node:crypto'
import { query } from '@anthropic-ai/claude-agent-sdk'

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
const exec = opt.exec ?? `${process.env.HOME}/.local/share/claude/versions/2.1.318`
const log = createWriteStream(opt.log ?? `logs/mcp-status-${Date.now()}.jsonl`)
const t0 = Date.now()
const rec = (kind, data) => log.write(JSON.stringify({ ms: Date.now() - t0, kind, ...data }) + '\n')
const say = (s) => process.stderr.write(`[mcp +${((Date.now() - t0) / 1000).toFixed(1)}s] ${s}\n`)
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
const deny = async (_tool, _input, o) => ({ behavior: 'deny', message: 'mcp probe: no tools', toolUseID: o.toolUseID })

// the fixture is named; everything else is counted.
const fixtureName = 'fixture'
const summarise = (servers) => {
  const counts = {}
  let fixture = null
  for (const s of servers) {
    if (s.name === fixtureName) {
      fixture = { status: s.status, scope: s.scope ?? null, source: s.source ?? null, error: s.error ?? null, serverInfo: s.serverInfo ?? null, tools: (s.tools ?? []).map((t) => ({ name: t.name, description: t.description ?? null })) }
      continue
    }
    const key = `${s.status}/${s.scope ?? s.source ?? '?'}`
    counts[key] = (counts[key] ?? 0) + 1
  }
  const withTools = servers.filter((s) => Array.isArray(s.tools)).length
  return { total: servers.length, withTools, others: counts, fixture, keys: [...new Set(servers.flatMap((s) => Object.keys(s)))].sort() }
}

let release
const hold = new Promise((r) => (release = r))
let sendPrompt
const prompted = new Promise((r) => (sendPrompt = r))
async function* input() {
  const text = await Promise.race([prompted, hold.then(() => null)])
  if (text) {
    yield { type: 'user', message: { role: 'user', content: text }, parent_tool_use_id: null, session_id: '' }
    await hold
  }
}

const options = { cwd, pathToClaudeCodeExecutable: exec, sessionId: randomUUID(), permissionMode: 'default', canUseTool: deny }
if (opt.mcp) options.mcpServers = { [fixtureName]: { type: 'stdio', command: 'node', args: [resolve(opt.mcp)] } }
const q = query({ prompt: input(), options })

const summary = { cwd, polls: [], messages: [], phases: {} }
let inits = []
;(async () => {
  for await (const m of q) {
    rec('message', { message: m })
    const brief = { at: Date.now() - t0, type: m.type, subtype: m.subtype ?? null }
    if (m.type === 'system' && m.subtype === 'init') {
      brief.mcp_servers = m.mcp_servers.map((s) => (s.name === fixtureName ? s : { status: s.status, source: s.source ?? null }))
      brief.mcp_tools = m.tools.filter((t) => t.startsWith(`mcp__${fixtureName}__`))
      brief.mcp_tool_count = m.tools.filter((t) => t.startsWith('mcp__')).length
      inits.push(brief)
    }
    summary.messages.push(brief)
    if (m.type === 'result') say('turn ended')
  }
})().catch((e) => rec('stream_error', { error: String(e) }))

const poll = async (phase, at) => {
  for (const ms of at) {
    const due = phase.start + ms
    if (Date.now() < due) await sleep(due - Date.now())
    const asked = Date.now()
    try {
      const servers = await q.mcpServerStatus()
      rec('status', { phase: phase.name, servers })
      summary.polls.push({ phase: phase.name, at: asked - t0, answeredIn: Date.now() - asked, ...summarise(servers) })
    } catch (e) {
      summary.polls.push({ phase: phase.name, at: asked - t0, error: String(e) })
    }
  }
}

await poll({ name: 'start', start: t0 }, [0, 100, 250, 500, 1000, 1500, 2000, 3000, 5000, 8000, 12000])

// the fixture server's process: node running that script, and below this
// probe, so nothing else that merely names it is touched.
const fixturePids = () => {
  const procs = []
  for (const name of readdirSync('/proc')) {
    if (!/^\d+$/.test(name)) continue
    try {
      const stat = readFileSync(`/proc/${name}/stat`, 'utf8')
      const argv = readFileSync(`/proc/${name}/cmdline`, 'utf8').split('\0')
      procs.push({ pid: Number(name), ppid: Number(stat.slice(stat.lastIndexOf(')') + 2).split(' ')[1]), argv })
    } catch {}
  }
  const below = new Set([process.pid])
  let grew = true
  while (grew) {
    grew = false
    for (const p of procs) if (!below.has(p.pid) && below.has(p.ppid)) { below.add(p.pid); grew = true }
  }
  return procs.filter((p) => below.has(p.pid) && /(^|\/)node$/.test(p.argv[0]) && (p.argv[1] ?? '').endsWith('fixture-server.mjs')).map((p) => p.pid)
}

if (opt.kill) {
  const pids = fixturePids()
  summary.phases.kill = { at: Date.now() - t0, pids: pids.length }
  for (const pid of pids) {
    try {
      process.kill(pid, 'SIGKILL')
    } catch {}
  }
  say(`killed ${pids.length} fixture server process(es)`)
  await poll({ name: 'killed', start: Date.now() }, [0, 200, 1000, 3000, 6000])
}

if (opt['add-broken']) {
  const servers = { [fixtureName]: { type: 'stdio', command: 'node', args: [resolve(opt.mcp)] }, broken: { type: 'stdio', command: '/nonexistent/mcp-probe-server' } }
  const asked = Date.now()
  try {
    const result = await q.setMcpServers(servers)
    summary.phases.setMcpServers = { at: asked - t0, answeredIn: Date.now() - asked, result: { added: result.added, removed: result.removed, errors: result.errors } }
  } catch (e) {
    summary.phases.setMcpServers = { error: String(e) }
  }
  await poll({ name: 'broken', start: Date.now() }, [0, 500, 2000, 5000])
  const last = (await q.mcpServerStatus()).find((s) => s.name === 'broken')
  summary.phases.broken = last ? { status: last.status, error: last.error ?? null, scope: last.scope ?? null, source: last.source ?? null } : null
}

if (opt.prompt) {
  summary.phases.prompt = { at: Date.now() - t0 }
  sendPrompt('Reply with the single word OK.')
  while (!summary.messages.some((m) => m.type === 'result')) await sleep(100)
  await poll({ name: 'after-turn', start: Date.now() }, [0])
}

summary.inits = inits
release()
q.close?.()
await sleep(500)
console.log(JSON.stringify(summary, null, 1))
log.end()
process.exit(0)

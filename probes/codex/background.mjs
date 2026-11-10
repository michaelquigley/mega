#!/usr/bin/env node
// background terminals probe (2026-11-10): whether a codex turn can leave a
// command running after it ends, and what the app-server says about it.
//
//   node background.mjs --cwd DIR --log logs/x.jsonl [--poll-seconds 50] [--session]
//
// starts a thread, runs one turn that asks for a command that keeps running
// after the reply, then polls thread/backgroundTerminals/list every two
// seconds after turn/completed, recording every frame the server sends,
// notifications included, and whether anything starts a turn on its own when
// the command ends. approvals are accepted only for the exact command; every
// other server request is declined.
import { spawn } from 'node:child_process'
import { createWriteStream } from 'node:fs'
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
const pollSeconds = Number(opt['poll-seconds'] ?? 50)
const log = createWriteStream(opt.log ?? `logs/background-${Date.now()}.jsonl`)
const t0 = Date.now()
const s = () => +((Date.now() - t0) / 1000).toFixed(1)
const rec = (kind, data) => log.write(JSON.stringify({ s: s(), kind, ...data }) + '\n')
const say = (x) => process.stderr.write(`[codex-bg +${s()}s] ${x}\n`)
const command = 'sleep 30 && echo BG-CODEX-DONE'

const child = spawn('codex', ['app-server'], { cwd, stdio: ['pipe', 'pipe', 'pipe'] })
child.stderr.on('data', (d) => rec('stderr', { data: String(d).slice(0, 1000) }))
let nextId = 1
const waiting = new Map()
const send = (frame) => { rec('send', { frame }); child.stdin.write(JSON.stringify(frame) + '\n') }
const request = (method, params) => new Promise((res, rej) => { const id = nextId++; waiting.set(id, { res, rej }); send({ id, method, params }) })
const notifications = []
let turnCompleted = null
const turnsStarted = []

createInterface({ input: child.stdout }).on('line', (line) => {
  if (!line.trim()) return
  const msg = JSON.parse(line)
  rec('recv', { frame: msg })
  if (msg.id !== undefined && msg.method === undefined) {
    const w = waiting.get(msg.id)
    if (w) { waiting.delete(msg.id); msg.error ? w.rej(new Error(JSON.stringify(msg.error))) : w.res(msg.result) }
    return
  }
  if (msg.id !== undefined && msg.method) {
    // a server request: accept the probe's own command, decline the rest.
    const p = msg.params ?? {}
    const text = JSON.stringify(p)
    const ok = text.includes('sleep 30') && text.includes('BG-CODEX-DONE')
    say(`server request ${msg.method} -> ${ok ? 'accept' : 'decline'}`)
    send({ id: msg.id, result: { decision: ok ? 'accept' : 'decline' } })
    return
  }
  const brief = { s: s(), method: msg.method }
  const p = msg.params ?? {}
  if (p.item) brief.item = { type: p.item.type, status: p.item.status, command: p.item.command, processId: p.item.processId }
  if (p.turn) brief.turn = { id: p.turn.id, status: p.turn.status }
  if (!/delta$/i.test(msg.method)) { notifications.push(brief); say(JSON.stringify(brief).slice(0, 200)) }
  if (msg.method === 'turn/started') turnsStarted.push(s())
  if (msg.method === 'turn/completed') turnCompleted = turnCompleted ?? s()
})

await request('initialize', { clientInfo: { name: 'roundhouse-probe', version: '0' }, capabilities: { experimentalApi: true } })
send({ method: 'initialized', params: {} })
const started = await request('thread/start', { cwd })
const threadId = started.thread.id
say(`thread ${threadId}`)
// --session asks for the command as a live terminal session (unified exec
// with a short yield), rather than leaving the model to detach it.
const prompt = opt.session
  ? `Run this exact command directly, with no nohup, no & and no redirection: \`${command}\`. Use a short yield time (about one second) so the tool returns while the command is still running as a terminal session, and do not poll or wait for it. Then reply BG-STARTED immediately. Nothing else.`
  : `Start this exact shell command so that it keeps running in the background after you reply, without waiting for it to finish: \`${command}\`. Then reply BG-STARTED immediately. Nothing else.`
await request('turn/start', { threadId, input: [{ type: 'text', text: prompt }] })
while (turnCompleted === null) await new Promise((r) => setTimeout(r, 200))
say('turn completed; polling background terminals')
const polls = []
for (let i = 0; i < pollSeconds / 2; i++) {
  try {
    const list = await request('thread/backgroundTerminals/list', { threadId })
    polls.push({ s: s(), data: (list.data ?? []).map((t) => ({ command: t.command, processId: t.processId, itemId: t.itemId, osPid: t.osPid ?? null })) })
    say(`list: ${JSON.stringify(polls.at(-1).data).slice(0, 200)}`)
  } catch (e) {
    polls.push({ s: s(), error: String(e) })
    say(`list failed: ${e}`)
  }
  await new Promise((r) => setTimeout(r, 2000))
}
child.stdin.end()
await new Promise((r) => child.on('exit', r))
const summary = { threadId, turnCompleted, turnsStarted, polls, notifications }
rec('summary', summary)
log.end()
console.log(JSON.stringify(summary, null, 2))

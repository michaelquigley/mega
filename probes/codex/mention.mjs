#!/usr/bin/env node
// @-mention probe (2026-11-10): whether codex treats "@path" in a turn's text
// as a file mention, as its terminal does, and what its `mention` input item
// does with a path.
//
//   node mention.mjs --cwd DIR --log logs/x.jsonl
//
// two turns in one thread, each asking for the exact contents of
// fixture.txt and forbidding commands: the first names it as "@fixture.txt"
// in plain text, the second passes it as a {type: "mention"} input item.
// every approval is declined. records whether a command ran, the user
// message codex stored, and the reply.
import { spawn } from 'node:child_process'
import { createWriteStream, readFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
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
const content = readFileSync(join(cwd, 'fixture.txt'), 'utf8').trim()
const log = createWriteStream(opt.log ?? `logs/mention-${Date.now()}.jsonl`)
const t0 = Date.now()
const rec = (kind, data) => log.write(JSON.stringify({ ms: Date.now() - t0, kind, ...data }) + '\n')
const child = spawn('codex', ['app-server'], { cwd, stdio: ['pipe', 'pipe', 'pipe'] })
let nextId = 1
const waiting = new Map()
const send = (frame) => { rec('send', { frame }); child.stdin.write(JSON.stringify(frame) + '\n') }
const request = (method, params) => new Promise((res, rej) => { const id = nextId++; waiting.set(id, { res, rej }); send({ id, method, params }) })
let turn = null
const turns = []
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
    turn?.declined.push(msg.method)
    send({ id: msg.id, result: { decision: 'decline' } })
    return
  }
  const it = msg.params?.item
  if (turn && msg.method === 'item/completed' && it) {
    if (it.type === 'commandExecution') turn.commands.push(it.command)
    if (it.type === 'userMessage') turn.userMessage = it.content
    if (it.type === 'agentMessage') turn.reply = it.text
  }
  if (turn && msg.method === 'turn/completed') turn.done = true
})
await request('initialize', { clientInfo: { name: 'roundhouse-probe', version: '0' }, capabilities: { experimentalApi: true } })
send({ method: 'initialized', params: {} })
const thread = (await request('thread/start', { cwd })).thread.id
const ask = 'Reply with the exact contents of the file I mention, and nothing else. Do not run any commands. If you cannot see its contents, reply NOT-ATTACHED.'
for (const [name, input] of [
  ['text @fixture.txt', [{ type: 'text', text: `${ask} @fixture.txt` }]],
  ['mention item', [{ type: 'text', text: ask }, { type: 'mention', name: 'fixture.txt', path: join(cwd, 'fixture.txt') }]],
]) {
  turn = { name, commands: [], declined: [], userMessage: null, reply: null, done: false }
  turns.push(turn)
  try {
    await request('turn/start', { threadId: thread, input })
  } catch (e) {
    turn.error = String(e)
    continue
  }
  while (!turn.done) await new Promise((r) => setTimeout(r, 200))
  turn.attached = turn.reply?.includes(content) ?? false
}
child.stdin.end()
await new Promise((r) => child.on('exit', r))
const summary = { thread, fixture: content, turns }
rec('summary', summary)
log.end()
console.log(JSON.stringify(summary, null, 2))

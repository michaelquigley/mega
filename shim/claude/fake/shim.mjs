// a stand-in for ../shim.mjs, for browser checks that need no model turns. it
// speaks the same frames on stdio (see the protocol at the top of ../shim.mjs)
// and never runs claude. the service finds a shim by directory, and wants the
// agent sdk installed there, so a check sets up a scratch directory:
//
//   mkdir -p DIR/node_modules/@anthropic-ai/claude-agent-sdk
//   ln -s "$PWD/shim/claude/fake/shim.mjs" DIR/shim.mjs
//
// and runs a scratch service whose config sets claude.shim to DIR (and
// claude.executable to anything; it is never run). the service starts the shim
// in DIR, so the sessions and messages it records for list and messages are
// kept there, never in the checkout.
//
// a prompt's first line drives a turn:
//   burst N          N assistant messages at once
//   drip N MS        N assistant messages MS apart, each streamed first
//   ask              a permission request; the turn ends once it is answered
//   echo             one assistant message: the lines after the first
//   stream MS N      the lines after the first, streamed N characters every
//                    MS, then landed as one assistant message
//   anything else    one assistant message, "ok"
import { appendFileSync, existsSync, readFileSync } from 'node:fs'
import { randomUUID } from 'node:crypto'
import { createInterface } from 'node:readline'
import { join } from 'node:path'

const store = process.cwd()
const [cmd, ...rest] = process.argv.slice(2)
const arg = (name) => {
  const i = rest.indexOf(name)
  return i < 0 ? undefined : rest[i + 1]
}
const out = (frame) => process.stdout.write(JSON.stringify(frame) + '\n')
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms))

if (cmd === 'list') {
  const cwd = arg('--cwd')
  const file = join(store, 'sessions.jsonl')
  if (existsSync(file)) {
    for (const line of readFileSync(file, 'utf8').split('\n').filter(Boolean)) {
      const session = JSON.parse(line)
      if (session.cwd === cwd) {
        out({ sessionId: session.id, firstPrompt: session.first, cwd, createdAt: session.at, lastModified: session.at })
      }
    }
  }
  process.exit(0)
}
if (cmd === 'messages') {
  const file = join(store, `messages-${arg('--session')}.jsonl`)
  if (existsSync(file)) {
    process.stdout.write(readFileSync(file, 'utf8'))
  }
  process.exit(0)
}
if (cmd !== 'run') {
  process.stderr.write(`unknown command '${cmd}'\n`)
  process.exit(2)
}

const session = arg('--session-id') ?? arg('--resume')
const cwd = arg('--cwd')
const fresh = arg('--session-id') !== undefined
let first = null
let waiting = null

// keep records a message for later imports, then sends it.
const keep = (msg) => {
  appendFileSync(join(store, `messages-${session}.jsonl`), JSON.stringify(msg) + '\n')
  out({ type: 'event', msg })
}
const say = (text) =>
  keep({
    type: 'assistant',
    uuid: randomUUID(),
    session_id: session,
    parent_tool_use_id: null,
    message: { role: 'assistant', content: [{ type: 'text', text }] },
  })
const delta = (text) =>
  out({
    type: 'event',
    msg: {
      type: 'stream_event',
      session_id: session,
      parent_tool_use_id: null,
      event: { type: 'content_block_delta', delta: { type: 'text_delta', text } },
    },
  })
const end = (subtype = 'success') =>
  keep({ type: 'result', subtype, is_error: subtype !== 'success', uuid: randomUUID(), session_id: session })
const filler = (i) =>
  `Message ${i}. ` + 'This line stands in for a paragraph of agent output, long enough to wrap. '.repeat(3)

out({
  type: 'event',
  msg: {
    type: 'system',
    subtype: 'init',
    uuid: randomUUID(),
    session_id: session,
    model: 'fake',
    claude_code_version: 'fake',
    permissionMode: 'default',
    cwd,
  },
})

async function turn(text) {
  const newline = text.indexOf('\n')
  const head = newline < 0 ? text : text.slice(0, newline)
  const body = newline < 0 ? '' : text.slice(newline + 1)
  const [verb, a, b] = head.trim().split(/\s+/)
  if (verb === 'burst') {
    for (let i = 1; i <= Number(a); i++) say(filler(i))
    end()
  } else if (verb === 'drip') {
    for (let i = 1; i <= Number(a); i++) {
      await sleep(Number(b))
      delta(`Streaming ${i}…`)
      await sleep(Number(b) / 2)
      say(`Drip ${i}. ` + filler(i))
    }
    end()
  } else if (verb === 'ask') {
    waiting = 'req-' + randomUUID().slice(0, 8)
    out({ type: 'request', id: waiting, kind: 'permission', tool: 'Bash', input: { command: 'echo fake' }, toolUseId: 'tu-' + waiting })
  } else if (verb === 'echo') {
    say(body)
    end()
  } else if (verb === 'stream') {
    const size = Number(b) || 16
    for (let i = 0; i < body.length; i += size) {
      await sleep(Number(a))
      delta(body.slice(i, i + size))
    }
    say(body)
    end()
  } else {
    say('ok')
    end()
  }
}

const lines = createInterface({ input: process.stdin })
lines.on('line', (line) => {
  const frame = JSON.parse(line)
  if (frame.type === 'prompt') {
    if (first === null && fresh) {
      first = frame.text
      appendFileSync(join(store, 'sessions.jsonl'), JSON.stringify({ id: session, cwd, first, at: Date.now() }) + '\n')
    }
    keep({ type: 'user', uuid: frame.uuid, session_id: session, parent_tool_use_id: null, message: { role: 'user', content: frame.text } })
    void turn(frame.text)
  } else if (frame.type === 'answer') {
    if (frame.id !== waiting) {
      out({ type: 'answer_rejected', id: frame.id })
      return
    }
    waiting = null
    out({ type: 'answer_accepted', id: frame.id })
    say(`Answered: ${frame.decision}`)
    end()
  } else if (frame.type === 'interrupt') {
    if (waiting) {
      out({ type: 'request_resolved', id: waiting, outcome: 'aborted' })
      waiting = null
    }
    out({ type: 'interrupted', response: {} })
    end('error_during_execution')
  } else if (frame.type === 'close') {
    out({ type: 'exit', code: 0 })
    process.exit(0)
  }
})

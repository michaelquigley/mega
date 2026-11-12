// appends generated turns to a scratch mega event log, for checks that need a
// long conversation: realistic shares of native events, tool calls with their
// results, replies with markdown and code, and the odd approval. about seven
// in ten events are native, as in Michael's real logs. it writes mega's own
// events only; each one's raw payload is a synthetic stand-in of a comparable
// size, never a harness's frame, since harness protocol stays in the adapters.
// it is seeded, so the same seed writes the same conversation. it writes only
// scratch logs.
//
//   appendTurns(path, targetEvents, seed) -> { seq, turns }
import { appendFileSync, readFileSync } from 'node:fs'

// mulberry32: a small seeded generator, enough for test data.
function seeded(seed) {
  let a = seed >>> 0
  return () => {
    a = (a + 0x6d2b79f5) >>> 0
    let t = a
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

const words = 'the service log index turn event page stream render scroll read position native tool call result checkout harness session prompt reply request approval build test gate review'.split(' ')

export function appendTurns(path, target, seed) {
  const rnd = seeded(seed)
  const int = (lo, hi) => lo + Math.floor(rnd() * (hi - lo + 1))
  const pick = (xs) => xs[Math.floor(rnd() * xs.length)]
  const prose = (n) => Array.from({ length: n }, () => pick(words)).join(' ')
  const code = (n) => Array.from({ length: n }, (_, i) => `func f${i}(x int) int { return x * ${i} } // ${prose(6)}`).join('\n')
  const id = () => Array.from({ length: 4 }, () => Math.floor(rnd() * 0x100000000).toString(16).padStart(8, '0')).join('-')

  const lines = readFileSync(path, 'utf8').trim().split('\n')
  let seq = JSON.parse(lines.at(-1)).seq
  let at = Date.parse('2026-11-09T09:00:00Z')
  const out = []
  const emit = (ev) => {
    seq++
    at += 200 + Math.floor(rnd() * 5800)
    out.push(JSON.stringify({ origin: 'managed', ...ev, seq, at: new Date(at).toISOString() }))
  }
  const native = (size) => {
    const nid = id()
    const label = pick(['status', 'progress', 'stream', 'limits', 'hook'])
    emit({ kind: 'native', native_id: nid, text: label, raw: { synthetic: 'native', label, id: nid, detail: prose(Math.floor(size / 6)), blob: 'x'.repeat(Math.floor(size / 2)) } })
  }

  let turn = 0
  while (seq < target) {
    turn++
    if (turn % 25 === 1) {
      emit({ kind: 'runtime', origin: 'service', status: 'starting' })
      emit({ kind: 'runtime', status: 'started', facts: { session: 'gen', model: 'fake-opus-1', version: 'fake' } })
    }
    emit({ kind: 'user', native_id: id(), text: `turn ${turn}: ${prose(int(8, 60))}` })
    native(3000)
    for (let call = 0, calls = int(3, 9); call < calls; call++) {
      const tid = `toolu_${turn}_${call}`
      const tool = pick(['Bash', 'Read', 'Edit', 'Grep'])
      emit({ kind: 'tool_use', tool, tool_use_id: tid, native_id: `${tid}u`, text: `${tool} ${prose(5)}`, raw: { synthetic: 'tool_use', id: tid, input: { command: prose(20), body: code(8) } } })
      if (tool === 'Bash' && rnd() < 0.15) {
        const rid = `req_${turn}_${call}`
        emit({ kind: 'request', origin: 'service', req: { id: rid, kind: 'permission', payload: { tool: 'Bash', input: { command: prose(10) } } } })
        emit({ kind: 'request_resolved', origin: 'service', req: { id: rid, kind: 'permission', outcome: 'answered' } })
      }
      for (let n = int(2, 6); n > 0; n--) native(int(900, 2600))
      emit({ kind: 'tool_result', tool_use_id: tid, native_id: `${tid}r`, text: code(int(4, 40)), raw: { synthetic: 'tool_result', id: tid, content: code(int(4, 30)) } })
    }
    for (let n = int(4, 12); n > 0; n--) native(int(900, 2600))
    const reply = `## turn ${turn}\n\n${prose(int(40, 200))}\n\n\`\`\`go\n${code(int(2, 12))}\n\`\`\`\n\n- ${prose(12)}\n- ${prose(10)}`
    emit({ kind: 'assistant', native_id: id(), text: reply, raw: { synthetic: 'assistant', text: reply } })
    emit({ kind: 'turn_end', status: 'success', text: 'success', native_id: id(), raw: { synthetic: 'turn_end' } })
  }
  appendFileSync(path, out.join('\n') + '\n')
  return { seq, turns: turn }
}

// appendNative adds one native event, which a page is never sent unless it
// asks for native events.
export function appendNative(path) {
  const last = JSON.parse(readFileSync(path, 'utf8').trim().split('\n').at(-1))
  const ev = { at: last.at, kind: 'native', native_id: 'gen-tail-native', origin: 'managed', raw: { synthetic: 'native', label: 'status' }, seq: last.seq + 1, text: 'status' }
  appendFileSync(path, JSON.stringify(ev) + '\n')
  return ev.seq
}

// appendImported adds a turn read from outside mega, as an import after a
// terminal visit records it: a prompt and a reply, each of origin native.
export function appendImported(path) {
  const last = JSON.parse(readFileSync(path, 'utf8').trim().split('\n').at(-1))
  const events = [
    { at: last.at, kind: 'user', native_id: 'gen-imported-user', origin: 'native', seq: last.seq + 1, text: 'a prompt typed in a terminal' },
    { at: last.at, kind: 'assistant', native_id: 'gen-imported-reply', origin: 'native', seq: last.seq + 2, text: 'a reply given in a terminal' },
  ]
  appendFileSync(path, events.map((ev) => JSON.stringify(ev)).join('\n') + '\n')
  return last.seq + 2
}

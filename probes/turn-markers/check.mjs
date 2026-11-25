// check.mjs reads the recorded probe logs of all three harnesses and reports,
// for each, whether every turn has the harness's own start marker before its
// end, whether every start is followed by its end, and what output arrives
// with no turn open. it reads only frame types and turn ids, never message
// text. run from the repository: node probes/turn-markers/check.mjs
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative } from 'node:path'

const root = new URL('..', import.meta.url).pathname

function logs(dir) {
  const out = []
  for (const name of readdirSync(dir)) {
    const path = join(dir, name)
    if (statSync(path).isDirectory()) {
      out.push(...logs(path))
    } else if (name.endsWith('.jsonl')) {
      out.push(path)
    }
  }
  return out.sort()
}

function lines(path) {
  const out = []
  for (const line of readFileSync(path, 'utf8').split('\n')) {
    if (!line.trim()) continue
    try {
      out.push(JSON.parse(line))
    } catch {
      // a truncated last line, as a killed probe leaves
    }
  }
  return out
}

// tally keeps one harness's counts and the files behind each irregularity.
function tally(name) {
  return { name, files: 0, turns: 0, repeated: 0, afterEnd: 0, endWithoutStart: [], startWithoutEnd: [], startInsideTurn: [], outputOutsideTurn: [] }
}

function report(t) {
  console.log(`${t.name}: ${t.files} logs, ${t.turns} turns with a start and an end`)
  if (t.repeated) console.log(`  a turn's end logged again, as a log with two clients holds every frame twice: ${t.repeated}`)
  if (t.name === 'codex') console.log(`  items of a turn already ended, output after its turn, which opens none: ${t.afterEnd}`)
  for (const [label, list] of [
    ['an end with no start since the last end', t.endWithoutStart],
    ['a start with no end before the log ends', t.startWithoutEnd],
    ['a start while a turn is open', t.startInsideTurn],
    ['output with no turn open (for codex, before its turn was named)', t.outputOutsideTurn],
  ]) {
    console.log(`  ${label}: ${list.length}`)
    for (const where of list) console.log(`    ${where}`)
  }
}

// claude: a turn starts at system/init and ends at result. the probe logs
// each sdk message as {kind: msg, msg}, and each prompt it sends as send_turn.
function claude() {
  const t = tally('claude')
  for (const path of logs(join(root, 'claude/logs'))) {
    const rel = relative(root, path)
    let open = false
    let any = false
    for (const rec of lines(path)) {
      if (rec.kind !== 'msg' || !rec.msg) continue
      any = true
      const m = rec.msg
      if (m.type === 'system' && m.subtype === 'init') {
        if (open) t.startInsideTurn.push(rel)
        open = true
      } else if (m.type === 'result') {
        if (!open) t.endWithoutStart.push(rel)
        else t.turns++
        open = false
      } else if (m.type === 'assistant' || (m.type === 'user' && !m.isReplay)) {
        if (!open) t.outputOutsideTurn.push(`${rel} (${m.type}${m.parent_tool_use_id ? ', a subagent\'s' : ''})`)
      }
    }
    if (open) t.startWithoutEnd.push(rel)
    if (any) t.files++
  }
  return t
}

// codex: a turn is named by turn/started or by the reply to turn/start, and
// ends at turn/completed. items carry their turn's id.
function codex() {
  const t = tally('codex')
  for (const path of logs(join(root, 'codex/logs'))) {
    const rel = relative(root, path)
    const starts = new Map() // json-rpc id of a turn/start sent -> true
    const named = new Set()
    const ended = new Set()
    let any = false
    for (const rec of lines(path)) {
      const f = rec.frame
      if (!f) continue
      if (rec.kind === 'send' && f.method === 'turn/start') {
        starts.set(f.id, true)
        continue
      }
      if (rec.kind !== 'recv') continue
      any = true
      if (f.id !== undefined && starts.has(f.id) && f.result?.turn?.id) {
        const id = f.result.turn.id
        if (!named.has(id) && !ended.has(id)) named.add(id)
        continue
      }
      const id = f.params?.turn?.id ?? f.params?.turnId
      if (f.method === 'turn/started') {
        if (!named.has(id) && !ended.has(id)) named.add(id)
      } else if (f.method === 'turn/completed') {
        if (ended.has(id)) t.repeated++
        else if (!named.has(id)) t.endWithoutStart.push(`${rel} (${id})`)
        else t.turns++
        named.delete(id)
        ended.add(id)
      } else if (f.method === 'item/completed' || f.method === 'item/started') {
        if (id && ended.has(id)) t.afterEnd++
        else if (id && !named.has(id)) t.outputOutsideTurn.push(`${rel} (${f.method} before its turn was named)`)
      }
    }
    for (const id of named) t.startWithoutEnd.push(`${rel} (${id})`)
    if (any) t.files++
  }
  return t
}

// pi: a turn starts at agent_start and ends at agent_settled, the end the
// adapter reads.
function pi() {
  const t = tally('pi')
  for (const path of logs(join(root, 'pi/logs'))) {
    const rel = relative(root, path)
    let open = false
    let any = false
    for (const rec of lines(path)) {
      if (rec.kind === 'probe_start') {
        // a log a second run was appended to: the first run's open turn
        // was ended by its kill.
        if (open) t.startWithoutEnd.push(rel)
        open = false
        continue
      }
      const f = rec.frame
      if (rec.kind !== 'frame' || !f) continue
      any = true
      if (f.type === 'agent_start') {
        // pi sends it again for each retry inside one run.
        if (open) t.startInsideTurn.push(rel)
        open = true
      } else if (f.type === 'agent_settled') {
        if (!open) t.endWithoutStart.push(rel)
        else t.turns++
        open = false
      } else if (f.type === 'message_end' && f.message?.role === 'assistant') {
        if (!open) t.outputOutsideTurn.push(rel)
      }
    }
    if (open) t.startWithoutEnd.push(rel)
    if (any) t.files++
  }
  return t
}

for (const t of [claude(), codex(), pi()]) report(t)

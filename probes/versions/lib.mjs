// the plumbing both version drivers share: a running scratch mega reached over
// its HTTP API, the steps and their ok/FAIL lines, the waits, the environment
// guard, and the summary. a driver never starts or stops mega; the runner
// (claude.sh, codex.sh) does.
//
// a step is check() or must(). must() is a step the driver cannot go past when
// it fails: before any Allow, what is about to be allowed is asserted with
// it, so nothing unexpected is ever approved.
import { writeFileSync } from 'node:fs'

// exit codes: a failed step, and a driver that could not run at all.
export const FAILED = 1
export const CANNOT_RUN = 3

export const turnTimeout = 4 * 60 * 1000

export const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

export function cannotRun(why) {
  console.error(`cannot run: ${why}`)
  process.exit(CANNOT_RUN)
}

// stopped is thrown by must() and by a wait that timed out: the driver ends
// there, with its summary.
class Stopped extends Error {}

// probe connects a driver to the mega MEGA_PROBE_URL names. MEGA_PROBE_SUMMARY,
// when set, is where the JSON summary is written as well as printed.
export function probe(harness) {
  const url = process.env.MEGA_PROBE_URL
  if (!url) cannotRun('MEGA_PROBE_URL is not set; run the driver through its runner, or set it to the scratch mega')
  const parsed = new URL(url)
  if (parsed.hostname !== '127.0.0.1' && parsed.hostname !== 'localhost') cannotRun(`${url} is not on loopback`)
  if (parsed.port === '6342' || parsed.port === '8470' || parsed.port === '') cannotRun(`port ${parsed.port || 'the default'} is the operator’s mega, the default or the one before it; the probe uses a scratch port`)

  const begun = Date.now()
  const steps = { passed: 0, failed: [] }
  const summary = { harness, url }
  const say = (s) => console.log(`[${new Date().toTimeString().slice(0, 8)}] ${s}`)
  // note is a line that reports and is not a step.
  const note = (s) => say(`info ${s}`)
  const check = (ok, what) => {
    say(`${ok ? 'ok  ' : 'FAIL'} ${what}`)
    if (ok) steps.passed++
    else steps.failed.push(what)
    return ok
  }
  const must = (ok, what) => {
    check(ok, what)
    if (!ok) throw new Stopped(`stopped before going on: ${what}`)
  }

  // until waits on a condition, every 250 ms, and stops the driver when it
  // never holds.
  const until = async (what, pred, ms = turnTimeout) => {
    const end = Date.now() + ms
    let last
    while (Date.now() < end) {
      try {
        const got = await pred()
        if (got) return got
      } catch (e) {
        if (e instanceof Stopped) throw e
        last = e
      }
      await sleep(250)
    }
    check(false, `timed out after ${Math.round(ms / 1000)} s waiting for ${what}${last ? ` (last error: ${last.message})` : ''}`)
    throw new Stopped(`stopped: ${what} never came`)
  }

  const api = async (method, path, body) => {
    const r = await fetch(url + path, {
      method,
      headers: { 'Content-Type': 'application/json', Origin: url },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
    const text = await r.text()
    let parsedBody = null
    try {
      parsedBody = text ? JSON.parse(text) : null
    } catch {
      parsedBody = { raw: text }
    }
    return { status: r.status, body: parsedBody }
  }
  const workspace = async () => {
    const r = await api('GET', '/api/workspace')
    if (r.status !== 200) throw new Error(`workspace read: ${r.status}`)
    return r.body
  }
  const projectOf = async (id) => (await workspace()).projects.find((p) => p.id === id)
  const convOf = async (id) => (await workspace()).conversations.find((c) => c.id === id)
  // events reads a conversation's whole log, native events included.
  const events = async (id, since = 0) => {
    const r = await api('GET', `/api/conversations/${id}/events?since=${since}&native=1`)
    if (r.status !== 200) throw new Error(`events read: ${r.status} ${JSON.stringify(r.body)}`)
    return r.body.events
  }
  const lastSeq = async (id) => {
    const evs = await events(id)
    return evs.length ? evs[evs.length - 1].seq : 0
  }
  const lastAssistant = async (id) => {
    const evs = await events(id)
    for (let i = evs.length - 1; i >= 0; i--) if (evs[i].kind === 'assistant') return evs[i].text ?? ''
    return ''
  }
  const short = (s, n = 80) => {
    const one = String(s ?? '').replace(/\s+/g, ' ').trim()
    return one.length > n ? `${one.slice(0, n)}…` : one
  }

  // startup reports what mega's start-up check found of the harness: where,
  // its version, and where that stands against harness.Tested. it is
  // reported, not checked: a build outside the range is what the probe is
  // for.
  const startup = async (name) => {
    const h = await until(`mega's start-up check of ${name}`, async () => (await workspace()).harnesses?.find((x) => x.name === name), 30000)
    note(`mega's start-up check: ${name} ${h.found ? `found, ${h.where}` : `not found: ${h.reason}`}; version ${h.version || 'unread'}${h.standing ? `, ${h.standing} against ${h.tested}` : ''}${h.note ? ` (${h.note})` : ''}`)
    summary.startup = { found: h.found, version: h.version, standing: h.standing, tested: h.tested }
    return h
  }

  // register adds the fixture as a project and gives it the local override
  // activation, which needs no approval. it returns the project and the time
  // just before the override was set, which the guard compares against.
  const register = async (name, checkout, activate) => {
    const r = await api('POST', '/api/projects', { name, checkout })
    must(r.status === 201, `registered ${name} at ${checkout}${r.status === 201 ? '' : `: ${r.status} ${JSON.stringify(r.body)}`}`)
    const before = Date.now()
    const o = await api('PUT', `/api/projects/${r.body.id}/environment/override`, { activate })
    must(o.status === 200 && o.body?.id === r.body.id, `the project's local override is the activation \`${activate}\`${o.status === 200 ? '' : `: ${o.status} ${JSON.stringify(o.body)}`}`)
    return { project: r.body, overrideAt: before }
  }

  // fromOverride resolves the project from its override, as the project
  // page's Resolve does, since nothing resolves a project in the background,
  // and returns that resolution as the workspace holds it. it counts when it
  // came from the override, after the override was set, with no error.
  const fromOverride = async (projectId, overrideAt) => {
    const r = await api('POST', `/api/projects/${projectId}/environment/resolve`)
    must(r.status === 200, `the project resolved from its override${r.status === 200 ? '' : `: ${r.status} ${JSON.stringify(r.body)}`}`)
    const e = (await projectOf(projectId))?.environment
    const resolved = e?.resolved
    must(
      e?.source === 'override' && resolved?.source === 'override' && Date.parse(resolved.at) >= overrideAt - 1000,
      `the project's environment is resolved from the override, after it was set (source ${e?.source ?? 'none'}, resolution ${resolved ? `${resolved.source} at ${resolved.at}` : 'none'})`,
    )
    must(!resolved.error, `the override's resolution has no error${resolved.error ? `: ${resolved.error}` : ''}`)
    return resolved
  }

  // resolvedValues resolves the project now, as the project page's "show
  // values" does, and returns only the values of the names asked for. the
  // rest are never kept or printed.
  const resolvedValues = async (projectId, names) => {
    const r = await api('POST', `/api/projects/${projectId}/environment/resolve?values=1`)
    if (r.status !== 200) return { source: null, values: {}, error: `${r.status} ${JSON.stringify(r.body)}` }
    const values = {}
    for (const n of names) values[n] = r.body.values?.[n]
    return { source: r.body.source, changed: r.body.changed ?? [], values }
  }

  // launch starts a conversation with its opening prompt.
  const launch = async (projectId, body) => {
    const r = await api('POST', `/api/projects/${projectId}/conversations`, body)
    must(r.status === 201, `launched a ${body.harness} conversation${r.status === 201 ? '' : `: ${r.status} ${JSON.stringify(r.body)}`}`)
    return r.body
  }
  const prompt = async (id, text) => {
    const r = await api('POST', `/api/conversations/${id}/prompt`, { text })
    must(r.status === 202, `the prompt was accepted${r.status === 202 ? '' : `: ${r.status} ${JSON.stringify(r.body)}`}`)
  }
  // turnEnd waits for the first turn_end after seq and for the conversation
  // to be idle again, and returns that event.
  const turnEnd = async (id, after, what = 'the turn to end') => {
    let ended = null
    await until(what, async () => {
      if (!ended) ended = (await events(id, after)).find((e) => e.kind === 'turn_end')
      if (!ended) return false
      const c = await convOf(id)
      return c && c.state === 'running' && !c.turn_active
    })
    return ended
  }

  // approval waits for the conversation's pending requests after seq, and
  // asserts there is exactly one, a permission request that want accepts,
  // before anything answers it. a turn that ends without one stops the
  // driver at once, with the reply: it never raised the request.
  const approval = async (id, after, describe, want) => {
    const c = await until(`the approval for ${describe}`, async () => {
      const conv = await convOf(id)
      if (conv?.pending?.length) return conv
      const ended = (await events(id, after)).find((e) => e.kind === 'turn_end')
      if (ended) {
        must(false, `the turn ended ${ended.status} without raising the approval for ${describe}; the reply was "${short(await lastAssistant(id), 200)}"`)
      }
      return null
    })
    must(c.pending.length === 1, `exactly one request is pending (${c.pending.length}); none was answered`)
    const req = c.pending[0]
    must(req.kind === 'permission', `it is a permission request (${req.kind}); it was not answered`)
    const why = want(req.payload ?? {})
    must(why === '', `it is the approval for ${describe}${why ? `, not ${why}; it was not answered` : ''}`)
    return req
  }
  // answer answers a request as a browser does and returns the error code,
  // or empty for an accepted answer.
  const answer = async (id, requestId, decision) => {
    const r = await api('POST', `/api/conversations/${id}/answer`, { request_id: requestId, decision })
    return r.status === 202 ? '' : (r.body?.code ?? `http ${r.status}`)
  }
  const interrupt = async (id) => {
    const r = await api('POST', `/api/conversations/${id}/interrupt`)
    must(r.status === 202, `the interrupt was accepted${r.status === 202 ? '' : `: ${r.status} ${JSON.stringify(r.body)}`}`)
  }
  // interrupted checks the turn ended interrupted and the request aborted.
  const interrupted = async (id, after, requestId) => {
    const ended = await turnEnd(id, after, 'the interrupted turn to end')
    check(ended.status === 'interrupted', `the turn ended ${ended.status}, want interrupted`)
    const resolved = (await events(id, after)).find((e) => e.kind === 'request_resolved' && e.req?.id === requestId)
    check(resolved?.req?.outcome === 'aborted', `the pending request was ${resolved?.req?.outcome ?? 'not resolved'}, want aborted`)
    const c = await convOf(id)
    check((c.pending ?? []).length === 0, `nothing is left pending (${(c.pending ?? []).length})`)
  }

  const setMode = async (id, mode) => {
    const r = await api('POST', `/api/conversations/${id}/permission-mode`, { mode })
    const c = await convOf(id)
    check(r.status === 200 && r.body?.outcome === 'confirmed' && c.permission_mode === mode,
      `a live switch to ${mode} was confirmed (${r.status} ${r.body?.outcome ?? JSON.stringify(r.body)}${r.body?.message ? `: ${r.body.message}` : ''}; live mode ${c.permission_mode})`)
  }

  // mcpCounts waits for the runtime to report its MCP servers and counts them
  // by status. names are never printed.
  const mcpCounts = async (id, ms = 90000) => {
    const read = await until('the runtime’s MCP servers to be reported', async () => {
      const r = await api('GET', `/api/conversations/${id}/mcp`)
      return r.status === 200 && r.body.reported ? r.body : null
    }, ms)
    const counts = { servers: read.servers.length }
    for (const s of read.servers) counts[s.status] = (counts[s.status] ?? 0) + 1
    check(true, `MCP status: ${Object.entries(counts).map(([k, v]) => `${k} ${v}`).join(', ')}`)
    return counts
  }

  // release releases the runtime and waits for it to be seen to exit.
  const release = async (id) => {
    const r = await api('POST', `/api/conversations/${id}/release`)
    must(r.status === 202, `the release was accepted${r.status === 202 ? '' : `: ${r.status} ${JSON.stringify(r.body)}`}`)
    await until('the runtime to exit', async () => (await convOf(id))?.state === 'none', 120000)
    const evs = await events(id)
    const last = evs[evs.length - 1]
    check(last?.kind === 'runtime' && last.status === 'stopped', `released, and the runtime was seen to stop (${last?.status} "${short(last?.text)}")`)
  }

  // imports are the events recorded from native history after seq: origin
  // native, which nothing a live runtime reports carries.
  const imports = async (id, after) => (await events(id, after)).filter((e) => e.origin === 'native').map((e) => ({ kind: e.kind, text: short(e.text, 60) }))

  // startIdle starts a released conversation with no prompt and checks it
  // resumed the same session, opened no turn, imported nothing and recorded
  // no message over a few idle seconds.
  const startIdle = async (id) => {
    const before = await convOf(id)
    const seq = await lastSeq(id)
    const r = await api('POST', `/api/conversations/${id}/start`)
    must(r.status === 202, `a start without a prompt was accepted${r.status === 202 ? '' : `: ${r.status} ${JSON.stringify(r.body)}`}`)
    const c = await until('the start', async () => {
      const v = await convOf(id)
      return v?.state === 'running' && v.version ? v : null
    }, 120000)
    check(!c.turn_active && c.native_id === before.native_id, `started on the same session with no turn (session ${c.native_id}, turn ${c.turn_active})`)
    await sleep(5000)
    const recorded = (await events(id, seq)).filter((e) => ['user', 'assistant', 'tool_use', 'tool_result', 'turn_end'].includes(e.kind) && e.origin !== 'native')
    check(recorded.length === 0, `an idle start recorded no message or turn${recorded.length ? `: ${recorded.map((e) => e.kind).join(', ')}` : ''}`)
    const got = await imports(id, seq)
    check(got.length === 0, `the start imported nothing${got.length ? `: ${got.map((e) => `${e.kind} "${e.text}"`).join('; ')}` : ''}`)
    const v = await convOf(id)
    check(true, `the idle start reports ${v.version} running ${v.model}`)
    return { view: v, imports: got }
  }

  // sameSession checks the conversation is still on its first session, by
  // its record and by the newest start's own report.
  // idleSwitch switches an idle conversation's model to another the model
  // list offers, prefer if it is one, and checks it stays idle: over a few
  // seconds no turn opens and no message is recorded, a harness's own echo of
  // the switch arriving only as a native event. then a prompt, whose turn must
  // ask for the new model and run on it. claude code 2.1.327 answered such a
  // switch with a user message marked as a replay, which mega once took for a
  // turn that never ended (capability report, 2026-11-18).
  const idleSwitch = async (id, models, { prefer, outcome, ask, reply }) => {
    const c = await convOf(id)
    must(c.state === 'running' && !c.turn_active, `idle before the model switch (state ${c.state}, turn ${c.turn_active})`)
    const runs = (m) => m.resolved || m.id
    const others = models.filter((m) => m.id !== 'default' && m.id !== c.model_choice && m.id !== c.model && runs(m) !== c.model)
    const pick = others.find((m) => m.id === prefer) ?? others[0]
    must(!!pick, `the model list offers a model besides the one running, ${c.model}`)
    const efforts = pick.efforts ?? []
    const effort = efforts.includes('low') ? 'low' : pick.default_effort || efforts[0] || ''
    const seq = await lastSeq(id)
    const r = await api('POST', `/api/conversations/${id}/model`, { model: pick.id, effort })
    check(r.status === 200 && r.body?.outcome === outcome,
      `an idle switch from ${c.model} to ${pick.id} at ${effort || 'no effort'} answered ${r.body?.outcome ?? `${r.status} ${JSON.stringify(r.body)}`}, want ${outcome}`)
    await sleep(5000)
    const after = await events(id, seq)
    const recorded = after.filter((e) => ['user', 'assistant', 'tool_use', 'tool_result', 'turn_end'].includes(e.kind))
    const natives = after.filter((e) => e.kind === 'native').map((e) => e.text)
    const v = await convOf(id)
    check(recorded.length === 0, `the idle switch recorded no message or turn${recorded.length ? `: ${recorded.map((e) => `${e.kind} "${short(e.text, 60)}"`).join('; ')}` : ''}`)
    check(!v.turn_active && v.attention !== 'running', `after the idle switch the conversation is idle (turn ${v.turn_active}, attention ${v.attention}); native events: ${natives.length ? natives.join(', ') : 'none'}`)
    const before = await lastSeq(id)
    await prompt(id, ask)
    const ended = await turnEnd(id, before, 'the turn after the idle switch')
    await expectReply(id, reply)
    const turn = ended.turn ?? {}
    check(turn.asked?.model === pick.id && turn.ran?.model === runs(pick),
      `the turn after the idle switch asked for ${turn.asked?.model} and ran on ${turn.ran?.model}, want ${pick.id} on ${runs(pick)}`)
    return { from: c.model, to: pick.id, runs: runs(pick), effort, outcome: r.body?.outcome, natives, ran: turn.ran }
  }

  const sameSession = async (id, nativeId, what) => {
    const c = await convOf(id)
    const starts = (await events(id)).filter((e) => e.kind === 'runtime' && e.status === 'started' && e.facts?.session)
    const reported = starts.length ? starts[starts.length - 1].facts.session : ''
    check(c.native_id === nativeId && (reported === '' || reported === nativeId), `${what} stayed on session ${nativeId} (recorded ${c.native_id}, reported ${reported || 'none'})`)
  }

  const expectReply = async (id, ...want) => {
    const reply = await lastAssistant(id)
    const missing = want.filter((w) => !reply.includes(w))
    check(missing.length === 0, `the reply has ${want.join(', ')}${missing.length ? `; it lacks ${missing.join(', ')}: "${short(reply, 200)}"` : ''}`)
    return reply
  }

  // turns reads every turn_end and checks none reported a difference between
  // what it asked for and what ran; started reads every version the
  // runtime's starts reported.
  const turns = async (id) => {
    const ends = (await events(id)).filter((e) => e.kind === 'turn_end')
    const differing = ends.filter((e) => e.turn?.differs?.length).map((e) => ({ seq: e.seq, differs: e.turn.differs, asked: e.turn.asked, ran: e.turn.ran }))
    const unrecorded = ends.filter((e) => !e.turn).length
    check(differing.length === 0, `no turn's line reported a difference, across ${ends.length} turns${unrecorded ? ` (${unrecorded} without a record)` : ''}${differing.length ? `: ${JSON.stringify(differing)}` : ''}`)
    return { turns: ends.length, differing }
  }
  const started = async (id) => {
    const versions = new Set()
    for (const e of await events(id)) if (e.kind === 'runtime' && e.status === 'started' && e.facts?.version) versions.add(e.facts.version)
    return [...versions]
  }

  // run runs the driver's steps, then prints the summary and exits. a
  // stopped driver or a crash still prints it.
  const run = async (body) => {
    let crashed = null
    try {
      await body()
    } catch (e) {
      if (e instanceof Stopped) say(e.message)
      else {
        crashed = e
        steps.failed.push(`crashed: ${e.message}`)
        say(`crashed: ${e.stack}`)
      }
    }
    const total = steps.passed + steps.failed.length
    summary.seconds = Math.round((Date.now() - begun) / 1000)
    summary.steps = { total, passed: steps.passed, failed: steps.failed }
    say(steps.failed.length ? `${steps.failed.length} of ${total} steps failed` : `all ${total} steps passed`)
    const json = JSON.stringify(summary, null, 2)
    console.log(`SUMMARY ${json}`)
    if (process.env.MEGA_PROBE_SUMMARY) writeFileSync(process.env.MEGA_PROBE_SUMMARY, json + '\n')
    process.exit(steps.failed.length || crashed ? FAILED : 0)
  }

  return {
    url, summary, say, note, check, must, until, api, workspace, projectOf, convOf, events, lastSeq, lastAssistant, short,
    startup, register, fromOverride, resolvedValues, launch, prompt, turnEnd, approval, answer, interrupt, interrupted,
    setMode, mcpCounts, release, imports, startIdle, idleSwitch, sameSession, expectReply, turns, started, run,
  }
}

// flag reads a boolean command-line flag.
export const flag = (name) => process.argv.slice(2).includes(name)

// option reads a command-line option's value.
export const option = (name) => {
  const args = process.argv.slice(2)
  const i = args.indexOf(name)
  return i < 0 ? undefined : args[i + 1]
}

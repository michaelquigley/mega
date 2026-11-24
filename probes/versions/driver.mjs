// the Claude Code version driver: every call mega makes of Claude Code, made
// over a running scratch mega's HTTP API, against the build its config names
// in claude.executable. it mirrors the e2e round trips (internal/e2e), which
// cannot switch the updater off, since a project's environment never takes
// mega's own; here the fixture's local override does.
//
//   MEGA_PROBE_URL=http://127.0.0.1:PORT MEGA_E2E_CLAUDE_FIXTURE=... node driver.mjs [--setup-only]
//
// run it through claude.sh, which starts and stops the scratch mega. it
// allows only Bash `sleep 3` after checking the request. A second request for
// that command is interrupted without an answer. --setup-only stops after
// the environment guard, before anything runs Claude Code.
import { cannotRun, flag, probe, sleep } from './lib.mjs'

const markerPrompt = 'Reply with the project instruction marker token from your project instructions, followed by the word ALPHA-1a2b. Nothing else.'
const approvalPrompt = 'Using the Bash tool, run exactly `sleep 3` and then reply DELTA-7a8b. Nothing else.'
const startPrompt = 'Reply with the word BETA-START-9e1f. Nothing else.'
const switchPrompt = 'Reply with the word BETA-SWITCH-4c2d. Nothing else.'
const returnPrompt = 'List every marker token you have seen in this conversation so far, in order, one per line, then append SDK-RETURN-c3d4 on its own line. Nothing else.'
const activation = 'export DISABLE_AUTOUPDATER=1 DISABLE_UPDATES=1'
const updaterNames = ['DISABLE_AUTOUPDATER', 'DISABLE_UPDATES']

const fixture = process.env.MEGA_E2E_CLAUDE_FIXTURE
if (!fixture) cannotRun('MEGA_E2E_CLAUDE_FIXTURE is not set: the probe needs the disposable claude-rt1 fixture (see internal/e2e/doc.go)')

const p = probe('claude')
const { summary, check, must } = p

// the one approval this driver answers: Bash, exactly `sleep 3`.
const sleep3 = (payload) => {
  const command = payload.input?.command
  if (payload.tool !== 'Bash') return `${payload.tool}`
  if (command !== 'sleep 3') return `Bash \`${command}\``
  return ''
}

await p.run(async () => {
  // setup: the fixture, the override that switches the updater off, and the
  // project's own resolution from it.
  await p.startup('claude')
  const { project, overrideAt } = await p.register('claude-rt1', fixture, activation)
  const resolved = await p.fromOverride(project.id, overrideAt)
  const { values } = await p.resolvedValues(project.id, updaterNames)
  for (const name of updaterNames) {
    // a name the operator's login shell already sets to 1 is not one the
    // activation changed, and so has no value to show; it is set either way.
    const changed = resolved.changed.includes(name)
    check(!changed || values[name] === '1', `${name} is ${changed ? `set to ${values[name]} by the override` : 'unchanged by the override, so already set in the login shell'}`)
  }
  summary.environment = { source: resolved.source, changed: resolved.changed }
  if (flag('--setup-only')) {
    check(true, 'setup only: stopping before anything runs Claude Code')
    return
  }

  // the model list, without a session.
  const t0 = Date.now()
  const models = await p.api('GET', `/api/projects/${project.id}/models?harness=claude`)
  const ms = Date.now() - t0
  must(models.status === 200, `the model list answered${models.status === 200 ? '' : `: ${models.status} ${JSON.stringify(models.body)}`}`)
  summary.models = { count: models.body.models.length, version: models.body.version, ms, copy: models.body.copy }
  check(models.body.models.length > 0 && !!models.body.version, `the model list: ${models.body.models.length} models, at version ${models.body.version}, in ${(ms / 1000).toFixed(1)} s`)

  // a launch and the marker turn.
  const conv = await p.launch(project.id, { harness: 'claude', name: 'versions probe', prompt: markerPrompt, permission_mode: 'default' })
  const id = conv.id
  await p.turnEnd(id, 0, 'the marker turn')
  await p.expectReply(id, 'ROUNDHOUSE-MARKER-7f3a', 'ALPHA-1a2b')
  const c = await p.convOf(id)
  const session = c.native_id
  summary.conversation = id
  summary.native_id = session
  check(!!session && c.permission_mode === 'default' && !!c.version && !!c.model, `the started facts: ${c.version} running ${c.model} in ${c.permission_mode} mode, session ${session}`)

  // an approval, asserted before Allow, that the turn waits on.
  let seq = await p.lastSeq(id)
  await p.prompt(id, approvalPrompt)
  const first = await p.approval(id, seq, 'Bash `sleep 3`', sleep3)
  check(first.payload?.permission_mode === 'default', `the request was raised in ${first.payload?.permission_mode} mode, want default`)
  await sleep(4000)
  check((await p.convOf(id)).turn_active, 'the turn waits while the approval is pending')
  must((await p.answer(id, first.id, 'allow')) === '', 'Allow was accepted')
  await p.turnEnd(id, seq, 'the approved turn')
  await p.expectReply(id, 'DELTA-7a8b')
  const late = await p.answer(id, first.id, 'deny')
  check(late === 'not_pending', `a late answer is refused as not pending (${late || 'accepted'})`)

  // an interrupt with a second approval pending: never answered.
  seq = await p.lastSeq(id)
  await p.prompt(id, approvalPrompt)
  const second = await p.approval(id, seq, 'Bash `sleep 3`', sleep3)
  await p.interrupt(id)
  await p.interrupted(id, seq, second.id)

  // a live switch to plan and back, each confirmed.
  await p.setMode(id, 'plan')
  await p.setMode(id, 'default')

  // MCP status, by counts.
  summary.mcp = await p.mcpCounts(id)

  // a release, then a start without a prompt on the same session.
  await p.release(id)
  const idle = await p.startIdle(id)
  summary.idle_start = { version: idle.view.version, model: idle.view.model }
  summary.imports = { start: idle.imports }

  // a prompt answered in the resumed session.
  seq = await p.lastSeq(id)
  await p.prompt(id, startPrompt)
  await p.turnEnd(id, seq, 'the prompt in the resumed session')
  await p.expectReply(id, 'BETA-START-9e1f')
  await p.sameSession(id, session, 'the prompt after the start')

  // a model switch while idle, which claude confirms at once, then a prompt
  // that must run on the new model. whether the build echoes the switch as a
  // replayed user message is recorded: mega maps it as a native event,
  // "user/replay".
  summary.idle_switch = await p.idleSwitch(id, models.body.models, { prefer: 'sonnet', outcome: 'confirmed', ask: switchPrompt, reply: 'BETA-SWITCH-4c2d' })
  summary.idle_switch.echo = summary.idle_switch.natives.includes('user/replay')

  // a release, and an exact-id resume by a prompt that must remember.
  await p.release(id)
  seq = await p.lastSeq(id)
  await p.prompt(id, returnPrompt)
  await p.turnEnd(id, seq, 'the resumed turn')
  await p.expectReply(id, 'ROUNDHOUSE-MARKER-7f3a', 'ALPHA-1a2b', 'DELTA-7a8b', 'SDK-RETURN-c3d4')
  await p.sameSession(id, session, 'the exact-id resume')
  const resumeImports = await p.imports(id, seq)
  check(resumeImports.length === 0, `the exact-id resume imported ${resumeImports.length} events, want zero`)
  summary.imports.resume = resumeImports

  // Release so the session holds the completed exchange and the bookkeeping
  // Claude wrote for the interrupt and model switch.
  await p.release(id)

  // every turn's line, read before the conversation is removed.
  summary.turns = await p.turns(id)
  summary.versions = await p.started(id)
  summary.version = summary.versions.length === 1 ? summary.versions[0] : summary.versions
  check(summary.versions.length === 1, `every start reported one version: ${summary.versions.join(', ')}`)

  // the session's own history comes into mega one way, by adoption: the
  // conversation is removed, and its session discovered and adopted again.
  const removed = await p.api('DELETE', `/api/conversations/${id}`)
  must(removed.status === 204, `the conversation was removed${removed.status === 204 ? '' : `: ${removed.status} ${JSON.stringify(removed.body)}`}`)
  const found = await p.api('GET', `/api/projects/${project.id}/discover?harness=claude`)
  must(found.status === 200 && (found.body.sessions ?? []).some((s) => s.native_id === session), `discovery lists session ${session}`)
  const adopted = await p.api('POST', `/api/projects/${project.id}/adopt`, { harness: 'claude', native_id: session, name: 'versions probe, adopted' })
  must(adopted.status === 201, `adopted session ${session}${adopted.status === 201 ? '' : `: ${adopted.status} ${JSON.stringify(adopted.body)}`}`)
  const messages = (await p.events(adopted.body.id)).filter((e) => e.origin === 'native' && (e.kind === 'user' || e.kind === 'assistant'))
  const knownReply = messages.some(({ kind, text }) => kind === 'assistant' && text?.includes('DELTA-7a8b'))
  const syntheticSpeech = messages.some(({ text = '' }) => text.includes('No response requested.') || text === 'assistant/synthetic')
  const localCommandSpeech = messages.some(({ text = '' }) => /<local-command-|<command-name>\/model<\/command-name>|user\/local-command|^Set model to\b/m.test(text))
  check(knownReply, 'the adoption’s snapshot contains the known assistant reply DELTA-7a8b')
  check(!syntheticSpeech, 'the adoption’s snapshot omits Claude’s synthetic interruption from speech')
  check(!localCommandSpeech, 'the adoption’s snapshot omits Claude’s local model command from speech')
  summary.snapshot = { conversation: adopted.body.id, messages: messages.length, known_reply: knownReply, synthetic_speech: syntheticSpeech, local_command_speech: localCommandSpeech }
})

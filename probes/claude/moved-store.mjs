#!/usr/bin/env node
// moved store probe (2026-11-11): whether a claude session resumes by exact id
// from a new checkout path once its session store directory has been moved to
// the name that path maps to, with its history intact. it rehearses moving a
// project's checkout and its store together.
//
//   node moved-store.mjs --cwd NEWPATH --session UUID --log logs/x.jsonl [--exec PATH]
//
// the caller has already copied the checkout to NEWPATH and moved a copy of
// the old path's store directory to the name NEWPATH maps to. the probe reads
// the session back through the sdk from NEWPATH (listSessions,
// getSessionMessages), then resumes it by id with one prompt asking for the
// replies it gave before, and records which session answered, the reply, and
// where the transcript grew.
import { createWriteStream, existsSync, readFileSync, readdirSync, statSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { homedir } from 'node:os'
import { getSessionMessages, listSessions, query } from '@anthropic-ai/claude-agent-sdk'

const argv = process.argv.slice(2)
const opt = {}
for (let i = 0; i < argv.length; i++) {
  if (!argv[i].startsWith('--')) continue
  const next = argv[i + 1]
  if (next === undefined || next.startsWith('--')) opt[argv[i].slice(2)] = true
  else { opt[argv[i].slice(2)] = next; i++ }
}
const cwd = resolve(opt.cwd)
const session = opt.session
const log = createWriteStream(opt.log ?? `logs/moved-store-${Date.now()}.jsonl`)
const t0 = Date.now()
const rec = (kind, data) => log.write(JSON.stringify({ ms: Date.now() - t0, kind, ...data }) + '\n')
const projects = join(process.env.CLAUDE_CONFIG_DIR ?? join(homedir(), '.claude'), 'projects')
const storeOf = (path) => join(projects, path.replace(/[^A-Za-z0-9]/g, '-'))
const transcript = join(storeOf(cwd), `${session}.jsonl`)
const size = () => (existsSync(transcript) ? { bytes: statSync(transcript).size, lines: readFileSync(transcript, 'utf8').trim().split('\n').length } : { exists: false })
const storesBefore = new Set(readdirSync(projects))

const summary = { cwd, session, store: storeOf(cwd), transcriptBefore: size() }
const listed = await listSessions({ dir: cwd })
summary.listed = listed.some((s) => s.sessionId === session)
const before = await getSessionMessages(session, { dir: cwd })
summary.messagesBefore = before.length
summary.repliesBefore = before.filter((m) => m.type === 'assistant').map((m) => (m.message?.content ?? []).filter((c) => c.type === 'text').map((c) => c.text).join(''))
rec('read', { listed: summary.listed, messages: before.length })

const q = query({
  prompt: 'List, in order and comma-separated, the exact words you replied with in your earlier turns of this conversation, then the word MOVED. Nothing else. Do not use any tools.',
  options: {
    cwd,
    resume: session,
    pathToClaudeCodeExecutable: opt.exec ?? '/home/michael/.local/share/claude/versions/2.1.318',
    permissionMode: 'default',
    disallowedTools: ['Read', 'Bash', 'Glob', 'Grep', 'Agent', 'Task'],
    canUseTool: async (_tool, _input, o) => ({ behavior: 'deny', message: 'moved store probe: no tools', toolUseID: o.toolUseID }),
  },
})
for await (const m of q) {
  if (m.type === 'stream_event') continue
  rec('msg', { msg: m })
  if (m.type === 'system' && m.subtype === 'init') summary.init = { session: m.session_id, cwd: m.cwd }
  if (m.type === 'assistant') summary.reply = (m.message?.content ?? []).filter((c) => c.type === 'text').map((c) => c.text).join('')
  if (m.type === 'result') summary.result = m.subtype
}
summary.transcriptAfter = size()
summary.newStores = readdirSync(projects).filter((d) => !storesBefore.has(d))
const after = await getSessionMessages(session, { dir: cwd })
summary.messagesAfter = after.length
summary.resumedSameSession = summary.init?.session === session
rec('summary', summary)
log.end()
console.log(JSON.stringify(summary, null, 2))

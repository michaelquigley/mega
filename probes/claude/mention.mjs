#!/usr/bin/env node
// @-mention probe (2026-11-10): whether claude code treats "@path" in a
// programmatic prompt as a file mention, as its terminal does.
//
//   node mention.mjs --cwd DIR --log logs/x.jsonl [--exec PATH] [--plain]
//                    [--file NAME] [--form bare|quoted|escaped]
//
// sends one prompt naming @fixture.txt with every file-reading tool
// disallowed, so the model can only answer if the mention attached the file.
// records every message, and whether any carries the file's content before
// the reply.
import { createWriteStream, readFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { query } from '@anthropic-ai/claude-agent-sdk'

const argv = process.argv.slice(2)
const opt = {}
for (let i = 0; i < argv.length; i++) {
  if (!argv[i].startsWith('--')) continue
  const next = argv[i + 1]
  if (next === undefined || next.startsWith('--')) opt[argv[i].slice(2)] = true
  else { opt[argv[i].slice(2)] = next; i++ }
}
const cwd = resolve(opt.cwd ?? '.')
// --file names the file to mention (fixture.txt by default); --form says how
// the mention is written: bare (@name), quoted (@"name") or escaped
// (@name with each space backslashed), for a name with spaces.
const file = typeof opt.file === 'string' ? opt.file : 'fixture.txt'
const content = readFileSync(join(cwd, file), 'utf8').trim()
const form = typeof opt.form === 'string' ? opt.form : 'bare'
const mention = { bare: `@${file}`, quoted: `@"${file}"`, escaped: `@${file.replaceAll(' ', '\\ ')}` }[form]
const log = createWriteStream(opt.log ?? `logs/mention-${Date.now()}.jsonl`)
const t0 = Date.now()
const rec = (kind, data) => log.write(JSON.stringify({ ms: Date.now() - t0, kind, ...data }) + '\n')
// --plain names the file without the @, as a control.
const prompt = `Reply with the exact contents of ${opt.plain ? file : mention} and nothing else. If you cannot see its contents, reply NOT-ATTACHED.`
const q = query({
  prompt,
  options: {
    cwd,
    pathToClaudeCodeExecutable: opt.exec ?? '/home/michael/.local/share/claude/versions/2.1.318',
    permissionMode: 'default',
    disallowedTools: ['Read', 'Bash', 'Glob', 'Grep', 'Agent', 'Task'],
    canUseTool: async (tool, _input, o) => ({ behavior: 'deny', message: 'mention probe: no tools', toolUseID: o.toolUseID }),
  },
})
const summary = { prompt, fixture: content, tools: [], carriesContent: [], reply: null }
for await (const m of q) {
  rec('msg', { msg: m })
  if (m.type === 'stream_event') continue
  const text = JSON.stringify(m)
  if (m.type === 'assistant') {
    for (const c of m.message?.content ?? []) {
      if (c.type === 'tool_use') summary.tools.push(c.name)
      if (c.type === 'text') summary.reply = c.text
    }
  } else if (text.includes(content)) {
    summary.carriesContent.push({ type: m.type, subtype: m.subtype ?? null, isMeta: m.isMeta ?? m.is_meta ?? null })
  }
  if (m.type === 'result') summary.result = m.subtype
}
summary.attached = summary.reply?.includes(content) ?? false
rec('summary', summary)
log.end()
console.log(JSON.stringify(summary, null, 2))

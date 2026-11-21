// a seeded mega to review the side panel and the keys by hand, in any
// browser: a scratch mega on its own port, data and XDG directories, with
// the fake Claude, so nothing reaches a real harness or costs a model turn.
// four projects, one starred and one with no conversations; two
// conversations waiting on an approval, one working, one with unread output.
// Ctrl+C stops it and removes its scratch.
//
//   node probes/keys/demo.mjs [PORT]     PORT 18490 by default
//
// it needs the UI built (npm --prefix ui run build) and the shim's
// dependencies (npm --prefix shim/claude ci), which make checks also does.
// on a Mac the project environments resolve through zsh, since the system
// bash 3.2 closes the resolver's fd 3 (roadmap: mega-on-macos-and-windows).
import { spawn, execFileSync } from 'node:child_process'
import { existsSync, mkdirSync, mkdtempSync, openSync, realpathSync, rmSync, symlinkSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'

const repo = resolve(import.meta.dirname, '../..')
const port = Number(process.argv[2] || 18490)
if ([6342, 8470].includes(port)) {
  console.error(`port ${port} is the operator's mega; use another`)
  process.exit(3)
}
const url = `http://127.0.0.1:${port}`
if (!existsSync(join(repo, 'ui/dist/index.html')) || !existsSync(join(repo, 'shim/claude/node_modules'))) {
  console.error("the UI build or the shim's dependencies are missing: npm --prefix ui run build; npm --prefix shim/claude ci")
  process.exit(3)
}
const answers = async () => {
  try {
    return (await fetch(`${url}/api/workspace`)).ok
  } catch {
    return false
  }
}
if (await answers()) {
  console.error(`something already answers at ${url}; stop it, or name another port`)
  process.exit(3)
}

// realpath: npm and the resolver both trip on /tmp being a link on a Mac.
const dir = mkdtempSync(join(realpathSync(tmpdir()), 'mega-keys-demo-'))
for (const d of ['checkouts', 'data', 'fakeshim', 'home/.claude', 'xdg/state', 'xdg/config', 'xdg/data', 'claudecfg']) {
  mkdirSync(join(dir, d), { recursive: true })
}
const bin = join(dir, 'mega')
console.log('building mega …')
execFileSync('go', ['build', '-o', bin, './cmd/mega'], { cwd: repo, stdio: 'inherit' })

symlinkSync(join(repo, 'shim/claude/fake/shim.mjs'), join(dir, 'fakeshim/shim.mjs'))
symlinkSync(join(repo, 'shim/claude/node_modules'), join(dir, 'fakeshim/node_modules'))
// the scratch HOME's profile puts this node, which the fakes run on, on PATH.
const profile = `export PATH="${dirname(process.execPath)}:$PATH"\n`
writeFileSync(join(dir, 'home/.profile'), profile)
writeFileSync(join(dir, 'home/.zprofile'), profile)
const settings = JSON.stringify({ effortLevel: 'medium' }) + '\n'
writeFileSync(join(dir, 'claudecfg/settings.json'), settings)
writeFileSync(join(dir, 'home/.claude/settings.json'), settings)
const mac = process.platform === 'darwin'
const config = join(dir, 'mega.yaml')
writeFileSync(
  config,
  [
    `listen: 127.0.0.1:${port}`,
    `data: ${join(dir, 'data')}`,
    'name: keys-demo',
    'claude:',
    '  node: node',
    `  shim: ${join(dir, 'fakeshim')}`,
    `  executable: ${existsSync('/bin/false') ? '/bin/false' : '/usr/bin/false'}`,
    'environment:',
    `  shell: ${mac ? '/bin/zsh' : '/bin/bash'}`,
    '',
  ].join('\n'),
)

const log = openSync(join(dir, 'service.log'), 'a')
const svc = spawn(bin, ['--config', config], {
  stdio: ['ignore', log, log],
  env: {
    ...process.env,
    HOME: join(dir, 'home'),
    XDG_STATE_HOME: join(dir, 'xdg/state'),
    XDG_CONFIG_HOME: join(dir, 'xdg/config'),
    XDG_DATA_HOME: join(dir, 'xdg/data'),
  },
})
let stopping = false
const stop = async (code) => {
  if (stopping) {
    return
  }
  stopping = true
  if (svc.exitCode === null) {
    svc.kill('SIGINT')
    await new Promise((done) => svc.once('exit', done))
  }
  rmSync(dir, { recursive: true, force: true })
  console.log(`stopped; removed ${dir}`)
  process.exit(code)
}
process.on('SIGINT', () => void stop(0))
process.on('SIGTERM', () => void stop(0))
svc.once('exit', (status) => {
  if (!stopping) {
    console.error(`mega exited (${status}); its log was ${join(dir, 'service.log')}`)
    void stop(1)
  }
})

const sleep = (ms) => new Promise((done) => setTimeout(done, ms))
const until = async (what, pred, ms = 20000) => {
  for (const end = Date.now() + ms; Date.now() < end; await sleep(150)) {
    if (await pred()) {
      return
    }
  }
  throw new Error(`timed out waiting for ${what}`)
}
const api = async (method, path, body) => {
  const r = await fetch(`${url}${path}`, { method, headers: body ? { 'content-type': 'application/json' } : {}, body: body ? JSON.stringify(body) : undefined })
  const text = await r.text()
  return { status: r.status, body: text ? JSON.parse(text) : null }
}
const must = async (what, call, status) => {
  const r = await call
  if (r.status !== status) {
    throw new Error(`${what}: ${r.status} ${JSON.stringify(r.body)}`)
  }
  return r.body
}
const conversation = async (id) => (await api('GET', '/api/workspace')).body.conversations.find((c) => c.id === id)

try {
  await until('mega to answer', answers)
  const project = async (name, starred = false) => {
    const path = join(dir, 'checkouts', name)
    mkdirSync(path)
    const p = await must(`registering ${name}`, api('POST', '/api/projects', { name, checkout: path }), 201)
    await must(`the environment of ${name}`, api('PUT', `/api/projects/${p.id}/environment/override`, { activate: `export CLAUDE_CONFIG_DIR=${join(dir, 'claudecfg')}` }), 200)
    if (starred) {
      await must(`starring ${name}`, api('PATCH', `/api/projects/${p.id}`, { starred: true }), 200)
    }
    return p
  }
  // each conversation opens with a short reply, then is prompted with what it
  // should be doing while the page is reviewed.
  const converse = async (p, name, next, read) => {
    const c = await must(`launching ${name}`, api('POST', `/api/projects/${p.id}/conversations`, { harness: 'claude', prompt: 'echo\nhello', name }), 201)
    await until(`${name} to finish its first turn`, async () => {
      const now = await conversation(c.id)
      return now?.state === 'running' && !now.turn_active
    })
    if (read) {
      await api('POST', `/api/conversations/${c.id}/read`, { seq: (await conversation(c.id)).last_seq })
    }
    if (next) {
      await must(`prompting ${name}`, api('POST', `/api/conversations/${c.id}/prompt`, { text: next }), 202)
    }
  }
  const gateway = await project('api-gateway', true)
  const billing = await project('billing')
  await project('docs-site')
  const web = await project('web-console')
  await converse(gateway, 'planner', 'ask', true)
  await converse(gateway, 'reviewer', null, true)
  await converse(billing, 'migrator', 'tool 3600000', true)
  await converse(billing, 'notes', null, false)
  await converse(web, 'fixer', 'ask', true)
  console.log(`\nmega is ready at ${url}/  (Ctrl+C stops it and removes ${dir})\n`)
} catch (failure) {
  console.error(String(failure))
  await stop(1)
}

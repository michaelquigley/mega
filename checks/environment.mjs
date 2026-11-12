// browser check: a project's environment on its page, against the fakes.
//
//   node checks/environment.mjs <playwright-core dir> <chrome> <mega binary> <scratch dir>
//
// everything it writes goes under the scratch dir: two checkouts it creates
// (alpha, a git repository with a .mega/environment.yaml; beta, with none),
// the service's config and data, its XDG state, config and data directories,
// and screenshots. the service listens on 127.0.0.1:18470, never 8470. the
// fake claude shim and fake codex run in place of the harnesses, so it costs
// no model turns. the environment resolves through /bin/bash, the operator's
// interactive shell, from mega's scrubbed base; the activations it runs are
// the ones this check writes and approves through the service, in its own
// scratch workspace.
//
// what it checks: the unapproved state and its refusal on New conversation
// and Adopt; approval; a launch whose header shows the environment's
// fingerprint and changed names; a changed file shown beside the approved
// version (from git), its refusal on Send and Start, and the header saying the
// runtime started in an older environment; approving again; the override
// (ignore the file, a changed file labelled as such under it, then use it
// again); the changed variables' values shown
// only on request; a project with no file; no page errors and nothing fetched
// off 127.0.0.1.
import { spawn, execFileSync } from 'node:child_process'
import { mkdirSync, openSync, rmSync, symlinkSync, writeFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'

const [playwrightDir, chrome, bin, scratch] = process.argv.slice(2)
if (!scratch) {
  console.error('usage: node checks/environment.mjs <playwright-core dir> <chrome> <mega binary> <scratch dir>')
  process.exit(2)
}
const { chromium } = createRequire(join(resolve(playwrightDir), 'package.json'))('playwright-core')
const repo = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const url = 'http://127.0.0.1:18470'
const S = resolve(scratch)
const say = (s) => console.log(`[${new Date().toISOString().slice(11, 19)}] ${s}`)
const failures = []
const check = (ok, what) => {
  say(`${ok ? 'ok  ' : 'FAIL'} ${what}`)
  if (!ok) failures.push(what)
}

// the scratch workspace: checkouts, a fake shim directory, config, XDG dirs.
rmSync(S, { recursive: true, force: true })
for (const d of ['checkouts/alpha', 'checkouts/beta', 'data', 'shots', 'fakeshim', 'xdg/state', 'xdg/config', 'xdg/data', 'claudecfg']) {
  mkdirSync(join(S, d), { recursive: true })
}
symlinkSync(join(repo, 'shim/claude/fake/shim.mjs'), join(S, 'fakeshim/shim.mjs'))
symlinkSync(join(repo, 'shim/claude/node_modules'), join(S, 'fakeshim/node_modules'))
const alpha = join(S, 'checkouts/alpha')
const beta = join(S, 'checkouts/beta')
const envFile = (body) => {
  mkdirSync(join(alpha, '.mega'), { recursive: true })
  writeFileSync(join(alpha, '.mega/environment.yaml'), body)
}
const git = (...args) =>
  execFileSync('git', args, { cwd: alpha, env: { ...process.env, GIT_AUTHOR_NAME: 'check', GIT_AUTHOR_EMAIL: 'check@localhost', GIT_COMMITTER_NAME: 'check', GIT_COMMITTER_EMAIL: 'check@localhost' } })
git('init', '-q')
envFile('activate: export MEGA_MARKER=alpha-check-marker\n')
git('add', '.mega/environment.yaml')
git('commit', '-q', '-m', 'environment')
const config = join(S, 'mega.yaml')
writeFileSync(
  config,
  [
    'listen: 127.0.0.1:18470',
    `data: ${join(S, 'data')}`,
    'claude:',
    '  node: node',
    `  shim: ${join(S, 'fakeshim')}`,
    '  executable: /bin/false',
    'codex:',
    `  binary: ${join(repo, 'internal/harness/codex/fake/codex')}`,
    'environment:',
    '  shell: /bin/bash',
    '',
  ].join('\n'),
)

const out = openSync(join(S, 'service.log'), 'a')
const svc = spawn(bin, ['--config', config], {
  stdio: ['ignore', out, out],
  env: { ...process.env, XDG_STATE_HOME: join(S, 'xdg/state'), XDG_CONFIG_HOME: join(S, 'xdg/config'), XDG_DATA_HOME: join(S, 'xdg/data'), CLAUDE_CONFIG_DIR: join(S, 'claudecfg') },
})
let stopped = false
const stop = (code) => {
  if (!stopped) {
    stopped = true
    svc.kill('SIGINT')
  }
  setTimeout(() => process.exit(code), 1500)
}
process.on('uncaughtException', (e) => {
  console.log('crashed:', e.stack)
  stop(2)
})
for (let i = 0; i < 100; i++) {
  try {
    if ((await fetch(`${url}/api/workspace`)).ok) break
  } catch {}
  await new Promise((r) => setTimeout(r, 100))
}
const api = async (method, path, body) => {
  const r = await fetch(url + path, { method, headers: { 'Content-Type': 'application/json', Origin: url }, body: body ? JSON.stringify(body) : undefined })
  const t = await r.text()
  return { status: r.status, body: t ? JSON.parse(t) : null }
}
const until = async (what, pred, ms = 20000) => {
  const end = Date.now() + ms
  while (Date.now() < end) {
    if (await pred()) return
    await new Promise((r) => setTimeout(r, 100))
  }
  throw new Error(`timed out: ${what}`)
}
const workspace = async () => (await api('GET', '/api/workspace')).body
const projectOf = async (id) => (await workspace()).projects.find((p) => p.id === id)
const convOf = async (id) => (await workspace()).conversations.find((c) => c.id === id)

const A = (await api('POST', '/api/projects', { name: 'alpha', checkout: alpha })).body
const B = (await api('POST', '/api/projects', { name: 'beta', checkout: beta })).body

const browser = await chromium.launch({ executablePath: chrome })
const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 } })
const page = await ctx.newPage()
const errors = []
const offsite = []
page.on('pageerror', (e) => errors.push(String(e)))
page.on('request', (r) => {
  if (!r.url().startsWith(url) && !r.url().startsWith('ws://127.0.0.1:18470') && !r.url().startsWith('data:')) offsite.push(r.url())
})
const shot = (name) => page.screenshot({ path: join(S, 'shots', `${name}.png`), fullPage: true })
const env = page.locator('section.environment')

// the unapproved state: shown, never run, and refused on the controls.
await page.goto(`${url}/#/p/${A.id}`)
await env.waitFor()
await until('the unapproved notice', async () => (await env.textContent()).includes('not approved'))
check((await env.locator('pre.env-file').first().textContent()).includes('MEGA_MARKER=alpha-check-marker'), 'the unapproved file is shown with its content')
const refusal = (await projectOf(A.id)).environment.refusal.message
check(refusal.includes('.mega/environment.yaml') && refusal.includes('has not been approved'), `the refusal names the file: ${refusal}`)
const held = page.locator(`.section-actions button[aria-label="${refusal}"]`)
check((await held.count()) === 2 && (await held.nth(0).isDisabled()) && (await held.nth(1).isDisabled()), 'New conversation and Adopt carry the refusal as their reason, disabled')
check((await projectOf(A.id)).environment.resolved === undefined, 'nothing resolved an unapproved file')
await shot('unapproved')

// approval.
await env.getByRole('button', { name: 'Approve this version, so mega runs it' }).click()
await until('approved', async () => (await projectOf(A.id)).environment.status === 'ready')
await until('the page says approved', async () => (await env.textContent()).includes('approved'))
check((await page.locator('.section-actions button[aria-label="New conversation"]').isEnabled()), 'New conversation is enabled once approved')

// a launch: the header shows the environment by fingerprint and names.
const conv = (await api('POST', `/api/projects/${A.id}/conversations`, { harness: 'claude', prompt: 'echo\nhello' })).body
await until('running', async () => {
  const c = await convOf(conv.id)
  return c.state === 'running' && !c.turn_active && c.environment
})
const started = (await convOf(conv.id)).environment
check(started.source === 'file' && started.changed.includes('MEGA_MARKER') && started.fingerprint.startsWith('sha256:'), `the runtime's environment: ${JSON.stringify(started)}`)
await page.goto(`${url}/#/c/${conv.id}`)
const facts = page.locator('.conversation-head .facts', { hasText: 'environment' })
await facts.waitFor()
check((await facts.textContent()).includes(started.fingerprint) && (await facts.textContent()).includes('MEGA_MARKER'), 'the header shows the fingerprint and the changed names')
const resolved = (await projectOf(A.id)).environment.resolved
check(resolved.changed.includes('MEGA_MARKER') && !JSON.stringify(resolved).includes('alpha-check-marker'), 'the resolution on the workspace read carries names, not values')

// a changed file: both versions side by side, refused on Send and Start, and
// the running conversation shown as started in an older environment.
envFile('activate: |\n  export MEGA_MARKER=alpha-check-marker\n  export EXTRA=changed\n')
await until('changed', async () => (await projectOf(A.id)).environment.status === 'changed')
const changedRefusal = (await projectOf(A.id)).environment.refusal.message
await until('the header says older', async () => (await facts.textContent()).includes('older environment'))
check(true, 'the header says the runtime started in an older environment')
const send = page.locator(`.panel-cluster button[aria-label="${changedRefusal}"]`)
check((await send.count()) >= 1, `Send carries the refusal: ${changedRefusal}`)
await shot('stale-header')
await api('POST', `/api/conversations/${conv.id}/release`)
await until('released', async () => (await convOf(conv.id)).state === 'none')
const start = page.locator(`.panel-cluster button[aria-label="${changedRefusal}"]`)
await until('Start shows the refusal', async () => (await start.count()) >= 2)
check(await start.last().isDisabled(), 'Start carries the refusal and is disabled')
await page.goto(`${url}/#/p/${A.id}`)
const compare = env.locator('.env-compare')
await compare.waitFor()
await until('the approved version from git', async () => (await compare.locator('pre.env-file').count()) === 2)
const versions = await compare.locator('pre.env-file').allTextContents()
check(!versions[0].includes('EXTRA') && versions[1].includes('EXTRA=changed'), 'the approved and current versions sit side by side')
check((await compare.textContent()).includes('from commit') || (await compare.textContent()).includes('from the git index'), 'the approved version says where it was found')
await shot('changed')

// approving again lifts the refusal.
await compare.getByRole('button', { name: 'Approve the changed version, so mega runs it' }).click()
await until('ready again', async () => (await projectOf(A.id)).environment.status === 'ready')
check((await page.locator('.section-actions button[aria-label="New conversation"]').isEnabled()), 'approving again lifts the refusal')

// values only on request.
check((await env.locator('table.env-values').count()) === 0, 'no values until asked')
await env.getByRole('button', { name: 'Show the values of the variables the activation changes' }).click()
await env.locator('table.env-values').waitFor()
const table = await env.locator('table.env-values').textContent()
check(table.includes('MEGA_MARKER') && table.includes('alpha-check-marker') && table.includes('EXTRA'), 'the values show on request')
await env.getByRole('button', { name: 'Hide the values' }).click()
check((await env.locator('table.env-values').count()) === 0, 'hiding them removes them from the page')

// the override: ignore the file, then use it again.
await env.locator('details.env-override summary').click()
await env.getByRole('button', { name: "Ignore the repository's file here" }).click()
await until('ignored', async () => (await projectOf(A.id)).environment.source === 'ignored')
await until('the page says ignored', async () => (await env.textContent()).includes('is ignored here'))
check(true, 'the override ignores the file')
// the override makes the environment ready; it says nothing about the file,
// which is labelled from its own fingerprint.
envFile('activate: export MEGA_MARKER=edited-under-the-override\n')
await until('the label follows the file', async () => (await env.locator('.env-version .facts').first().textContent()).includes('changed since it was approved'))
check(true, 'under the override, a changed file is labelled changed, never approved')
await shot('override')
await env.getByRole('button', { name: "Use the repository's file again" }).click()
await until('the file again', async () => (await projectOf(A.id)).environment.source === 'file')
check((await projectOf(A.id)).environment.status === 'changed', 'clearing the override uses the file again, and its change refuses')

// a project with no file.
await page.goto(`${url}/#/p/${B.id}`)
await env.waitFor()
check((await env.textContent()).includes('No .mega/environment.yaml'), 'a project with no file says so')

check(errors.length === 0, `no page errors: ${errors.join('; ')}`)
check(offsite.length === 0, `nothing fetched off 127.0.0.1: ${offsite.join(', ')}`)
await browser.close()
say(failures.length ? `${failures.length} failed` : 'all passed')
stop(failures.length ? 1 : 0)

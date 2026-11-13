// the plumbing every browser check shares: a scratch mega on a scratch port,
// with scratch data, scratch XDG directories and a scratch HOME, running the
// committed fakes in place of the harnesses; the browser; and the steps.
//
//   import { suite } from './lib/mega.mjs'
//   const t = await suite('requests')
//   ... t.check(ok, 'what it shows') ...
//   await t.done()
//
// a suite is its steps and nothing else. everything it writes goes under its
// scratch directory, MEGA_CHECKS_DIR/<suite> (default $TMPDIR/mega-checks),
// emptied first and left afterwards for its log and screenshots. the binary
// is MEGA_CHECK_BIN, or one built into the scratch directory. the port is
// MEGA_CHECK_PORT, default 18470; 8470 is the operator's and is refused.
import { execFileSync, spawn } from 'node:child_process'
import { existsSync, mkdirSync, openSync, rmSync, symlinkSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { chromium } from 'playwright'

export const repo = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..')

// exit codes the runner reads: a failed step, and a check that could not run.
export const FAILED = 1
export const CANNOT_RUN = 3

// cannotRun stops a check that could not even start, saying why.
export function cannotRun(why) {
  console.error(`cannot run: ${why}`)
  process.exit(CANNOT_RUN)
}

// browserPath is the Chromium the pinned Playwright drives. a missing browser
// is a failure with the command that installs it, never a silent skip.
export function browserPath() {
  const path = chromium.executablePath()
  if (!existsSync(path)) {
    cannotRun(`the Chromium build Playwright expects is not installed (${path}); run 'make checks-browser' once`)
  }
  return path
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

// until waits on a condition, checking it every 50 ms, and fails with what it
// waited for when the condition never holds. it is the only wait a check
// uses: never a fixed delay.
export async function until(what, pred, ms = 20000) {
  const end = Date.now() + ms
  let last
  while (Date.now() < end) {
    try {
      if (await pred()) return
    } catch (e) {
      last = e
    }
    await sleep(50)
  }
  throw new Error(`timed out waiting for ${what}${last ? ` (last error: ${last.message})` : ''}`)
}

// suite sets up a check: its scratch directory, the binary, the fakes, the
// service and the browser.
export async function suite(name) {
  const port = Number(process.env.MEGA_CHECK_PORT || 18470)
  if (port === 8470) {
    cannotRun('port 8470 is the operator’s mega; checks use another (MEGA_CHECK_PORT)')
  }
  const url = `http://127.0.0.1:${port}`
  const root = resolve(process.env.MEGA_CHECKS_DIR || join(tmpdir(), 'mega-checks'))
  const dir = join(root, name)
  rmSync(dir, { recursive: true, force: true })
  for (const d of ['checkouts', 'data', 'shots', 'fakeshim', 'home', 'xdg/state', 'xdg/config', 'xdg/data', 'claudecfg']) {
    mkdirSync(join(dir, d), { recursive: true })
  }
  const chrome = browserPath()

  let bin = process.env.MEGA_CHECK_BIN
  if (!bin) {
    bin = join(root, 'mega')
    if (!existsSync(join(repo, 'ui/dist/index.html')) || !existsSync(join(repo, 'shim/claude/node_modules'))) {
      cannotRun("the UI build or the shim's dependencies are missing; run 'make checks', which builds them")
    }
    execFileSync('go', ['build', '-o', bin, './cmd/mega'], { cwd: repo, stdio: 'inherit' })
  }
  if (!existsSync(bin)) {
    cannotRun(`no mega binary at ${bin}`)
  }

  // the fake claude shim, beside the real shim's dependencies, since mega's
  // preflight checks for the sdk; the fake codex is run where it is.
  symlinkSync(join(repo, 'shim/claude/fake/shim.mjs'), join(dir, 'fakeshim/shim.mjs'))
  symlinkSync(join(repo, 'shim/claude/node_modules'), join(dir, 'fakeshim/node_modules'))

  // a scratch HOME: project environments resolve through a login shell from
  // it, so the operator's shell start-up files and Claude settings are never
  // read. its profile puts this node, which the fakes run on, on PATH.
  writeFileSync(join(dir, 'home/.profile'), `export PATH="${dirname(process.execPath)}:$PATH"\n`)
  // the scratch Claude settings, both where the override points Claude and in
  // the scratch HOME, name an effort level, as Michael's do: claude's model
  // list names no default effort, so a new conversation copies this one.
  // they name no model and no mode, so a new conversation copies the list's
  // default model and the manual mode.
  const claudeSettings = JSON.stringify({ effortLevel: 'medium' }) + '\n'
  writeFileSync(join(dir, 'claudecfg/settings.json'), claudeSettings)
  mkdirSync(join(dir, 'home/.claude'), { recursive: true })
  writeFileSync(join(dir, 'home/.claude/settings.json'), claudeSettings)
  const config = join(dir, 'mega.yaml')
  writeFileSync(
    config,
    [
      `listen: 127.0.0.1:${port}`,
      `data: ${join(dir, 'data')}`,
      'claude:',
      '  node: node',
      `  shim: ${join(dir, 'fakeshim')}`,
      '  executable: /bin/false',
      'codex:',
      `  binary: ${join(repo, 'internal/harness/codex/fake/codex')}`,
      'environment:',
      '  shell: /bin/bash',
      '',
    ].join('\n'),
  )

  const steps = { passed: 0, failed: [] }
  const say = (s) => console.log(`[${new Date().toISOString().slice(11, 19)}] ${s}`)
  const check = (ok, what) => {
    say(`${ok ? 'ok  ' : 'FAIL'} ${what}`)
    if (ok) steps.passed++
    else steps.failed.push(what)
  }
  // must is a step the suite cannot go past when it fails: before any Allow,
  // what is about to be allowed is checked with it, so nothing unexpected is
  // ever approved.
  const must = (ok, what) => {
    check(ok, what)
    if (!ok) throw new Error(`stopped before going on: ${what}`)
  }

  // the service, which a suite may stop and start again on the same data.
  let svc = null
  let exited = null
  const log = openSync(join(dir, 'service.log'), 'a')
  const answers = async () => {
    try {
      return (await fetch(`${url}/api/workspace`)).ok
    } catch {
      return false
    }
  }
  const start = async () => {
    if (await answers()) {
      cannotRun(`something already answers at ${url}; stop it, or set MEGA_CHECK_PORT`)
    }
    svc = spawn(bin, ['--config', config], {
      stdio: ['ignore', log, log],
      env: {
        ...process.env,
        HOME: join(dir, 'home'),
        XDG_STATE_HOME: join(dir, 'xdg/state'),
        XDG_CONFIG_HOME: join(dir, 'xdg/config'),
        XDG_DATA_HOME: join(dir, 'xdg/data'),
      },
    })
    exited = new Promise((r) => svc.once('exit', (code, signal) => r({ code, signal })))
    let gone = false
    exited.then(() => (gone = true))
    await until('the service to answer', async () => {
      if (gone) throw new Error(`the service exited at start; see ${join(dir, 'service.log')}`)
      return answers()
    }, 30000)
  }
  // stop asks the service to stop as a terminal's Ctrl+C does and waits for
  // it to exit; one that will not is killed and counted as a failure.
  const stop = async () => {
    if (!svc) return
    const running = svc
    svc = null
    if (running.exitCode === null && running.signalCode === null) {
      running.kill('SIGINT')
    }
    const settled = await Promise.race([exited, sleep(30000).then(() => null)])
    if (!settled) {
      running.kill('SIGKILL')
      check(false, 'the service stopped within 30 s of SIGINT')
    }
  }
  // stopped waits for a service that was asked to stop some other way (a
  // shutdown from the page) to exit, and reports how it ended.
  const stopped = async (ms = 30000) => {
    if (!svc) return null
    const settled = await Promise.race([exited, sleep(ms).then(() => null)])
    if (settled) svc = null
    return settled
  }
  await start()

  const api = async (method, path, body) => {
    const r = await fetch(url + path, { method, headers: { 'Content-Type': 'application/json', Origin: url }, body: body ? JSON.stringify(body) : undefined })
    const text = await r.text()
    return { status: r.status, body: text ? JSON.parse(text) : null }
  }
  const workspace = async () => (await api('GET', '/api/workspace')).body
  const projectOf = async (id) => (await workspace()).projects.find((p) => p.id === id)
  const convOf = async (id) => (await workspace()).conversations.find((c) => c.id === id)

  // checkout makes a scratch checkout, a git repository when asked.
  const checkout = (name, { git = false } = {}) => {
    const path = join(dir, 'checkouts', name)
    mkdirSync(path, { recursive: true })
    if (git) {
      execFileSync('git', ['init', '-q'], { cwd: path })
    }
    return path
  }
  // register adds a project. unless asked not to, it gives the project the
  // override activation that points Claude at the scratch settings, since
  // mega's own environment does not reach what it starts.
  const register = async (name, path, { override = true } = {}) => {
    const r = await api('POST', '/api/projects', { name, checkout: path })
    if (r.status !== 201) throw new Error(`registering ${name}: ${r.status} ${JSON.stringify(r.body)}`)
    if (override) {
      const o = await api('PUT', `/api/projects/${r.body.id}/environment/override`, { activate: `export CLAUDE_CONFIG_DIR=${join(dir, 'claudecfg')}` })
      if (o.status !== 200) throw new Error(`overriding ${name}'s environment: ${o.status} ${JSON.stringify(o.body)}`)
    }
    return r.body
  }
  // launch starts a conversation and waits until its first turn has ended.
  const launch = async (projectId, harness, prompt, extra = {}) => {
    const r = await api('POST', `/api/projects/${projectId}/conversations`, { harness, prompt, ...extra })
    if (r.status !== 201) throw new Error(`launching: ${r.status} ${JSON.stringify(r.body)}`)
    return r.body
  }
  const idle = (id) =>
    until(`conversation ${id} to finish its turn`, async () => {
      const c = await convOf(id)
      return c && c.state === 'running' && !c.turn_active
    })

  // pages share one browser; each records page errors and any request that
  // leaves loopback.
  const browser = await chromium.launch({ executablePath: chrome })
  const errors = []
  const offsite = []
  const consoleErrors = []
  const pages = []
  const page = async ({ width = 1280, height = 900, colorScheme = 'light', init, initArg } = {}) => {
    const ctx = await browser.newContext({ viewport: { width, height }, colorScheme })
    if (init) await ctx.addInitScript(init, initArg)
    const p = await ctx.newPage()
    pages.push(p)
    p.on('pageerror', (e) => errors.push(`${e}${e.stack ? `\n${e.stack}` : ''}`))
    p.on('console', (m) => {
      if (m.type() === 'error') consoleErrors.push(m.text())
    })
    p.on('request', (r) => {
      const u = r.url()
      if (!u.startsWith(url) && !u.startsWith(`ws://127.0.0.1:${port}`) && !u.startsWith('data:')) offsite.push(u)
    })
    return p
  }
  const shot = (p, name) => p.screenshot({ path: join(dir, 'shots', `${name}.png`), fullPage: true })

  // done closes everything, adds the page-wide checks, prints the summary the
  // runner reads, and exits.
  const done = async () => {
    check(errors.length === 0, `no page errors${errors.length ? `: ${errors.join('; ')}` : ''}`)
    check(offsite.length === 0, `nothing fetched off 127.0.0.1${offsite.length ? `: ${offsite.join(', ')}` : ''}`)
    await browser.close().catch(() => {})
    await stop()
    const total = steps.passed + steps.failed.length
    say(steps.failed.length ? `${steps.failed.length} of ${total} steps failed` : `all ${total} steps passed`)
    console.log(`RESULT ${JSON.stringify({ suite: name, steps: total, failed: steps.failed.length })}`)
    process.exit(steps.failed.length ? FAILED : 0)
  }

  // a crash ends the check as a failure, the service stopped first.
  // each open page is saved as it stood, a screenshot and its text, so a
  // crash can be read afterwards.
  process.on('uncaughtException', async (e) => {
    console.log(`crashed: ${e.stack}`)
    steps.failed.push(`crashed: ${e.message}`)
    if (errors.length) console.log(`page errors:\n${errors.join('\n')}`)
    if (consoleErrors.length) console.log(`console errors:\n${consoleErrors.join('\n')}`)
    for (const [i, p] of pages.entries()) {
      if (p.isClosed()) continue
      await p.screenshot({ path: join(dir, 'shots', `crash-${i}.png`), fullPage: true, timeout: 5000 }).catch(() => {})
      const text = await p.evaluate(() => document.body.innerText).catch((err) => `(unreadable: ${err.message})`)
      writeFileSync(join(dir, `crash-${i}.txt`), `${p.url()}\n\n${text}\n`)
      console.log(`page ${i} (${p.url()}) saved to crash-${i}.png and crash-${i}.txt`)
    }
    await browser.close().catch(() => {})
    await stop()
    console.log(`RESULT ${JSON.stringify({ suite: name, steps: steps.passed + steps.failed.length, failed: steps.failed.length })}`)
    process.exit(FAILED)
  })
  process.on('unhandledRejection', (e) => {
    throw e
  })

  return {
    name,
    url,
    port,
    dir,
    repo,
    say,
    check,
    must,
    until,
    api,
    workspace,
    projectOf,
    convOf,
    checkout,
    register,
    launch,
    idle,
    page,
    shot,
    start,
    stop,
    stopped,
    done,
  }
}

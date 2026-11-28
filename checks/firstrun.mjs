// browser check: the first run, on a machine that has never had mega, with
// no harness installed.
//
//   make checks SUITE=firstrun
//
// a bare suite (lib/mega.mjs): no fakes, no Claude settings, a config that
// names no harness path, and no harness on mega's PATH or the project's. it
// is what a newcomer sees between `make` and installing Claude Code and
// Codex, and it is the walk the clean-machine rehearsal of 2026-11-13 made.
// nothing it does can reach a real harness, since none can be found. Claude
// Code's default, the bare name claude, is on no PATH, and the scratch HOME
// has no build in the installer's versions folder; Codex's and pi's defaults,
// bare names, are on no PATH.
//
// what it checks:
// - the start-up log: one line per harness, saying it cannot run and why,
//   Claude Code's naming the versions folder it looked in;
// - the empty workspace's first page, light and dark: what mega needs, each
//   harness missing with its reason, the register control named in words,
//   and that control carrying a word;
// - the register dialog: the focus in its checkout field, refusing a
//   relative path and a path that does not exist with what is wrong, then
//   registering a git checkout and opening its page;
// - the project page, before any dialog: nothing resolved by registering or
//   reading the workspace, so nothing listed missing and "Not resolved yet";
//   a trust notice that does not tell anyone to run a Claude that is not
//   installed and gives the start-up check's reason itself; and the New
//   conversation control carrying a word;
// - the new-conversation dialog: resolving the project as it opens, which
//   then lists all three harnesses as missing, Claude's reason naming where
//   it looked, what to do and the config file mega read; the focus in the
//   opening prompt, the harnesses marked as unable to run, the model list
//   saying it is unavailable, Launch disabled and in view at 1280x900 with
//   the notices; and, once it closes, the trust notice pointing to the
//   environment section;
// - the API: listing Claude's models and launching either harness refused as
//   executable_missing, with the reason;
// - never "internal error", and no answer of 500, anywhere on the walk;
// - a restart: the project kept, nothing resolved and nothing listed missing
//   before anything is opened, "Not resolved yet", and the start-up log's
//   lines again;
// - --listen: another mega on the port the flag names, and one refused for an
//   address that is not loopback;
// - the harnesses installed, as stand-ins that only say their versions: a
//   native Claude Code build in the versions folder, found there with no
//   claude on the PATH, and a whole Codex package on the PATH, inside its
//   range. a mega on fresh data says each version in its start-up log and
//   on its first page, in one short sentence: a Claude Code newer than its
//   range plainly, as the normal state it is, and in `mega version` and
//   /api/version too; then one older, marked in the notice colour with the
//   stronger sentence, and a warning in the log;
// - an update behind the same path, with mega running: Claude Code's link
//   moved to a new build, Codex's and pi's programs replaced in place, and
//   the first page and the open version panel following with no restart;
// - no page errors and nothing fetched off 127.0.0.1.
//
// it saves each page as shots/{empty,empty-dark,register,register-refused,
// project,launch-missing,restarted,versions-newer,versions-older}.png. each step's text is printed, so the
// run's output is a record of the words a newcomer reads.
import { execFileSync, spawn } from 'node:child_process'
import { chmodSync, copyFileSync, existsSync, mkdirSync, readFileSync, renameSync, rmSync, symlinkSync, utimesSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { checkPort, requireScratchPort, suite, tested } from './lib/mega.mjs'

// the other megas this suite starts take the four ports above its own, each
// refused before anything starts if it is the operator's or something
// already answers there.
for (const offset of [1, 2, 3, 4]) await requireScratchPort(checkPort() + offset)

const t = await suite('firstrun', { bare: true })
const { url, check, until, api, workspace } = t
const demo = t.checkout('demo', { git: true })
// where the default, the bare name claude, falls back to with none on the
// PATH: every reason for its absence names this folder.
const claudePath = join(t.dir, 'home/.local/share/claude/versions')

const text = async (locator) => ((await locator.textContent()) ?? '').replace(/\s+/g, ' ').trim()
const shown = (what, words) => t.say(`  ${what}: "${words}"`)
// soon is a bounded wait that answers instead of throwing, so a step that
// does not hold is a failed step and the walk goes on.
const soon = async (pred, ms = 15000) => {
  try {
    await until('a step', pred, ms)
    return true
  } catch {
    return false
  }
}
// the log mega writes when it is not on a terminal, and what it printed.
const startupLog = () =>
  [join(t.dir, 'xdg/state/mega/mega.log'), join(t.dir, 'service.log')]
    .filter((f) => existsSync(f))
    .map((f) => readFileSync(f, 'utf8'))
    .join('\n')
const harnessLines = (log) => log.split('\n').filter((l) => /harness '(claude|codex|pi)'/.test(l))
// each harness has its own line in a start-up's log.
const eachHarness = (lines) => ['claude', 'codex', 'pi'].every((name) => lines.some((l) => l.includes(`harness '${name}'`)))

// every answer of 500, and every "internal error" shown, on the whole walk.
const fiveHundreds = []
const watch = (p) =>
  p.on('response', (r) => {
    if (r.status() >= 500) fiveHundreds.push(`${r.status()} ${r.url()}`)
  })
const internalShown = []
const noInternal = async (p, where) => {
  const body = await p.evaluate(() => document.body.innerText)
  if (/internal error/i.test(body)) internalShown.push(where)
}

// the start-up log: one line per harness, found or missing and why.
const atStart = await soon(async () => eachHarness(harnessLines(startupLog())), 15000)
const lines = harnessLines(startupLog())
for (const line of lines) shown('the start-up log', line.trim())
check(atStart && lines.some((l) => l.includes("harness 'claude'") && l.includes('cannot run') && l.includes(claudePath)), 'the start-up log says Claude Code cannot run, naming the versions folder it looked in')
check(lines.some((l) => l.includes("harness 'codex'") && l.includes('cannot run')), 'the start-up log says Codex cannot run')
check(lines.some((l) => l.includes("harness 'pi'") && l.includes('cannot run')), 'the start-up log says pi cannot run')

// the empty workspace: what mega needs, what is missing, and where to begin.
const page = await t.page()
watch(page)
const shot = (name) => t.shot(page, name)
await page.goto(url)
const firstRun = page.locator('main .first-run')
const hasFirstRun = await soon(async () => (await firstRun.count()) > 0, 10000)
const first = hasFirstRun ? await text(firstRun) : await text(page.locator('main'))
shown('the first page', first)
check(hasFirstRun && first.includes('Register a project') && first.includes('side panel'), 'the empty workspace names the register control in words, and where it is')
const claudeItem = firstRun.locator('[data-harness="claude"]')
const codexItem = firstRun.locator('[data-harness="codex"]')
const firstClaude = await soon(async () => (await claudeItem.count()) > 0 && (await text(claudeItem)).includes(claudePath), 10000)
shown('the first page on Claude Code', firstClaude ? await text(claudeItem) : '(nothing)')
check(firstClaude && (await text(claudeItem)).includes('Claude Code') && (await text(claudeItem)).includes(t.config), 'the empty workspace says Claude Code is missing, where it looked, and which config file to set it in')
const firstCodex = await soon(async () => (await codexItem.count()) > 0 && (await text(codexItem)).includes("'codex' was not found"), 10000)
check(firstCodex, 'the empty workspace says Codex is missing, and why')
const empty = page.locator('section.projects p.muted')
check((await text(empty)) === 'No projects yet.', 'the side panel says there are no projects yet')
const register = page.locator('section.projects button[aria-label="Register a project"]')
check((await register.count()) === 1 && (await register.isEnabled()), 'the register control is in the side panel, enabled once the service answered')
const registerWord = (await register.locator('.word').count()) > 0 ? await text(register.locator('.word')) : ''
check(registerWord !== '', `the register control carries a word while the workspace is empty: "${registerWord}"`)
check((await page.locator('dialog[open]').count()) === 0, 'nothing opens by itself on the first page')
await noInternal(page, 'the first page')
await shot('empty')
const dark = await t.page({ colorScheme: 'dark' })
watch(dark)
await dark.goto(url)
await soon(async () => (await dark.locator('main .first-run [data-harness="codex"]').count()) > 0, 10000)
await t.shot(dark, 'empty-dark')

// the register dialog: what it asks, where the focus is, and what it says
// about a wrong path.
await register.click()
const dialog = page.locator('dialog[open]')
await dialog.waitFor()
check((await text(dialog.locator('h2'))) === 'Register a project', 'the dialog is titled Register a project')
const checkoutBox = dialog.locator('input').nth(0)
check((await checkoutBox.getAttribute('placeholder')) === '~/path/to/checkout', 'the checkout field asks for a path, ~ for the home directory')
const startsHome = await soon(async () => (await checkoutBox.inputValue()) === '~/', 5000)
check(startsHome, `with no projects root configured, the field starts at the home directory: "${await checkoutBox.inputValue()}"`)
const registerFocus = await soon(async () => (await page.evaluate(() => document.activeElement?.getAttribute('placeholder'))) === '~/path/to/checkout', 3000)
shown('focus on opening', await page.evaluate(() => document.activeElement?.getAttribute('aria-label') ?? document.activeElement?.getAttribute('placeholder') ?? document.activeElement?.tagName))
check(registerFocus, 'the register dialog opens with the focus in its checkout field')
await shot('register')
const refusedFor = async (path) => {
  await checkoutBox.fill(path)
  await dialog.locator('button[aria-label="Register"]').click()
  const error = dialog.locator('p.error')
  await until(`a refusal for ${path}`, async () => (await error.count()) > 0 && (await text(error)) !== '')
  return text(error)
}
const relative = await refusedFor('work/demo')
shown('a relative path', relative)
check(relative.includes('absolute'), 'a relative path is refused as not absolute')
const nowhere = await refusedFor(`${demo}-nowhere`)
shown('a path that does not exist', nowhere)
check(nowhere.includes(`${demo}-nowhere`), 'a path that does not exist is refused, naming it')
await shot('register-refused')
check((await workspace()).projects.length === 0, 'neither refusal registered anything')

// registering: the dialog closes on the project's page.
await checkoutBox.fill(demo)
await dialog.locator('button[aria-label="Register"]').click()
await until('the project page', async () => (await page.locator('article.detail h1').count()) > 0)
check((await text(page.locator('article.detail h1'))) === 'demo', 'the project is named after its directory, and its page opens')
check((await page.locator('dialog[open]').count()) === 0, 'the dialog closed')
const P = (await workspace()).projects[0]
check(P && P.checkout === demo, 'the project is registered at its checkout')

// the project page, before any dialog: registering resolved nothing, and
// reading the workspace resolves nothing however often it is read, so the
// environment section says it is not resolved yet and lists nothing missing.
const missingList = page.locator('section.environment ul.env-missing')
const missingNames = async () => ((await workspace()).projects[0]?.environment?.missing ?? []).map((m) => m.harness).sort().join(',')
const resolvedAt = async () => (await workspace()).projects[0]?.environment?.resolved?.at
for (let i = 0; i < 5; i++) await workspace()
await page.waitForTimeout(500)
shown('the environment section, before any dialog', await text(page.locator('section.environment')))
check((await resolvedAt()) === undefined && (await missingNames()) === '' && (await missingList.count()) === 0, 'before any dialog, registering and reading the workspace resolved nothing, and nothing is listed missing')
check((await text(page.locator('section.environment'))).includes('Not resolved yet'), 'the environment section says "Not resolved yet"')
const notices = page.locator('article.detail p.notice')
for (let i = 0; i < (await notices.count()); i++) {
  shown('a notice on the project page', await text(notices.nth(i)))
}
const trustNotice = page.locator('article.detail p.notice', { hasText: /trust/i })
const trust = (await trustNotice.count()) > 0 ? await text(trustNotice.first()) : ''
check(!trust.includes('Run claude here once'), `the trust notice does not tell anyone to run a Claude that is not installed: "${trust}"`)
check(trust.includes("'claude' was not found on your shell's PATH") && !trust.includes('environment section'), 'before a resolution, the trust notice gives the start-up check\'s reason and points to no section')
const newConversation = page.locator('article.detail button[aria-label="New conversation"]')
const newWord = (await newConversation.locator('.word').count()) > 0 ? await text(newConversation.locator('.word')) : ''
check(newWord !== '', `the New conversation control carries a word while the workspace has no conversation: "${newWord}"`)
await noInternal(page, 'the project page')
await shot('project')

// the new-conversation dialog: the focus in the prompt, every harness
// marked, no model list to wait for, and Launch disabled and in view.
await newConversation.click()
const launch = page.locator('dialog[open]')
await launch.waitFor()
// opening the dialog resolves the project: its reads need the environment.
const resolvedByDialog = await soon(async () => (await resolvedAt()) !== undefined && (await missingNames()) === 'claude,codex,pi', 15000)
shown('what the project lists as missing, once the dialog is open', await missingNames())
check(resolvedByDialog, 'opening the new-conversation dialog resolves the project, which then lists every harness as missing')
const claudeMissing = ((await workspace()).projects[0]?.environment?.missing ?? []).find((m) => m.harness === 'claude')?.reason ?? ''
shown('the project on Claude Code', claudeMissing)
check(
  claudeMissing.includes("'claude' was not found") && claudeMissing.includes(claudePath) && claudeMissing.includes('claude.executable') && claudeMissing.includes(t.config) && /install claude code/i.test(claudeMissing),
  "Claude Code is missing because 'claude' is on no PATH and no build is installed, and the reason says to install it or set claude.executable in the config file mega read",
)
const launchFocus = await soon(async () => (await page.evaluate(() => document.activeElement?.tagName === 'TEXTAREA' && document.activeElement.closest('dialog') !== null)), 3000)
shown('focus on opening', await page.evaluate(() => document.activeElement?.getAttribute('aria-label') ?? document.activeElement?.tagName))
check(launchFocus, 'the new-conversation dialog opens with the focus in the opening prompt')
const harnessSelect = launch.locator('select').first()
const bothMarked = await soon(async () => (await launch.locator('p.harness-missing').count()) === 3, 10000)
const markedTexts = await launch.locator('p.harness-missing').allTextContents()
for (const m of markedTexts) shown('the dialog on a missing harness', m.replace(/\s+/g, ' ').trim())
check(bothMarked && markedTexts.some((m) => m.includes(claudePath)) && markedTexts.some((m) => m.includes("'pi' was not found")), 'the dialog marks Claude Code as unable to run, naming where it looked, as it does Codex and pi')
check((await harnessSelect.locator('option[value="codex"]').isDisabled()) && (await text(harnessSelect.locator('option[value="codex"]'))) === 'Codex (cannot run)', 'Codex is offered disabled, marked cannot run')
check((await harnessSelect.locator('option[value="claude"]').isDisabled()) && (await text(harnessSelect.locator('option[value="claude"]'))).endsWith('(cannot run)'), 'Claude Code is offered disabled, marked cannot run')
const modelSelect = launch.getByRole('combobox', { name: 'Model' })
const modelSettled = await soon(async () => !(await text(modelSelect)).includes('Loading models'), 10000)
shown('the model list', await text(modelSelect))
check(modelSettled && /unavailable/i.test(await text(modelSelect)), 'the model list says it is unavailable, not that it is loading')
const dialogText = await text(launch)
check(!dialogText.includes('a launch takes what your settings name') && !/internal error/i.test(dialogText), 'the dialog says nothing stale: no "internal error", no "a launch takes what your settings name"')
await launch.locator('textarea').fill('say hello')
const launchButton = launch.locator('button[type="submit"]')
shown('the launch control', (await launchButton.getAttribute('aria-label')) ?? '')
check(await launchButton.isDisabled(), 'Launch is disabled for a harness that cannot run')
const box = await launchButton.boundingBox()
check(box !== null && box.y >= 0 && box.y + box.height <= 900, `Launch is in view at 1280x900 with the notices showing: ${box ? `${Math.round(box.y)}–${Math.round(box.y + box.height)}` : 'not laid out'}`)
await noInternal(page, 'the new-conversation dialog')
await shot('launch-missing')
await page.keyboard.press('Escape')
await until('the dialog closed', async () => (await page.locator('dialog[open]').count()) === 0)
const trustResolved = (await trustNotice.count()) > 0 ? await text(trustNotice.first()) : ''
shown('the trust notice once the project has a resolution', trustResolved)
check(trustResolved.includes('the environment section below says what is missing'), 'once the project has a resolution, the trust notice points to the environment section')

// the API: each refusal coded, with its reason; never a 500.
const models = await api('GET', `/api/projects/${P.id}/models?harness=claude`)
shown('the API on Claude Code’s models', `${models.status} ${models.body?.code}: ${models.body?.message}`)
check(models.status === 409 && models.body.code === 'executable_missing' && models.body.message.includes(claudePath), 'listing Claude Code’s models is refused as executable_missing, naming where it looked')
const byApi = await api('POST', `/api/projects/${P.id}/conversations`, { harness: 'claude', prompt: 'say hello', effort: 'low' })
shown('the API on launching Claude Code', `${byApi.status} ${byApi.body?.code}: ${byApi.body?.message}`)
check(byApi.status === 409 && byApi.body.code === 'executable_missing' && byApi.body.message.includes(claudePath), 'launching Claude Code is refused as executable_missing, naming where it looked')
const codexApi = await api('POST', `/api/projects/${P.id}/conversations`, { harness: 'codex', prompt: 'say hello' })
shown('the API on launching Codex', `${codexApi.status} ${codexApi.body?.code}: ${codexApi.body?.message}`)
check(codexApi.status === 409 && codexApi.body.code === 'executable_missing', 'launching Codex is refused as executable_missing')
check((await workspace()).conversations.length === 0, 'the refused launches made no conversation')

// a restart: the project stays, and nothing resolves it until something
// needs its environment.
await t.stop()
const before = startupLog().length
await t.start()
await page.reload()
await until('the project page again', async () => (await page.locator('article.detail h1').count()) > 0)
const after = (await workspace()).projects[0]
check(after.id === P.id && after.checkout === demo, 'the project is still registered after a restart')
for (let i = 0; i < 5; i++) await workspace()
await page.waitForTimeout(500)
shown('what the project lists as missing after the restart, before anything is opened', await missingNames())
check((await resolvedAt()) === undefined && (await missingNames()) === '', 'after a restart nothing has resolved the project, and nothing is listed missing')
const again = harnessLines(startupLog().slice(before))
check(eachHarness(again), `the start-up log says again what each harness is, Claude Code, Codex and pi: ${again.length} lines`)
shown('the environment section after the restart', await text(page.locator('section.environment')))
check((await text(page.locator('section.environment'))).includes('Not resolved yet'), 'after a restart the environment section says "Not resolved yet"')
await noInternal(page, 'the project page after the restart')
await shot('restarted')

// --listen: another mega, on its own scratch data, on the port the flag
// names; and one refused for an address that is not loopback.
const other = join(t.dir, 'other')
mkdirSync(join(other, 'data'), { recursive: true })
const otherEnv = { ...process.env, PATH: t.servicePath, HOME: join(t.dir, 'home'), XDG_STATE_HOME: join(other, 'state'), XDG_CONFIG_HOME: join(other, 'config'), XDG_DATA_HOME: join(other, 'xdgdata') }
const run = (listen) => {
  const p = spawn(t.bin, ['--config', t.config, '--data', join(other, 'data'), '--listen', listen], { stdio: ['ignore', 'pipe', 'pipe'], env: otherEnv })
  let out = ''
  p.stdout.on('data', (d) => (out += d))
  p.stderr.on('data', (d) => (out += d))
  const exited = new Promise((r) => p.once('exit', (code) => r(code)))
  return { p, exited, out: () => out }
}
const otherPort = t.port + 1
const flagged = run(`127.0.0.1:${otherPort}`)
const answered = await soon(async () => {
  try {
    return (await fetch(`http://127.0.0.1:${otherPort}/api/workspace`)).ok
  } catch {
    return false
  }
}, 15000)
check(answered, `--listen 127.0.0.1:${otherPort} serves on that port, over the config file's`)
flagged.p.kill('SIGINT')
await flagged.exited
const refused = run(`0.0.0.0:${otherPort}`)
const refusedCode = await Promise.race([refused.exited, new Promise((r) => setTimeout(() => r('still running'), 10000))])
if (refusedCode === 'still running') refused.p.kill('SIGKILL')
// with no terminal, mega says why it stopped in its log file.
const otherLog = join(other, 'state/mega/mega.log')
const refusal = (existsSync(otherLog) ? readFileSync(otherLog, 'utf8') : '') + refused.out()
shown('--listen 0.0.0.0', (refusal.match(/[^"]*not a loopback address[^"]*/) ?? [''])[0])
check(refusedCode !== 0 && refusedCode !== 'still running' && refusal.includes("listen '0.0.0.0:"), '--listen with an address that is not loopback is refused, as the config value is')

// the harnesses installed, as stand-ins that only say their versions: a
// native Claude Code build in the versions folder the default falls back
// to, and the fake Codex package, inside its range, on the PATH. neither
// ever runs a conversation here. a mega on fresh data shows its first page
// with them found: first with a Claude Code newer than its range, the normal
// state for a harness that updates itself, said plainly; then with one
// older, which needs a look and is marked.
const claudeTested = tested('claude')
const codexTested = tested('codex')
const codexVersion = '0.166.0'
symlinkSync(join(t.repo, 'internal/harness/codex/fake/codex'), join(t.dir, 'bin/codex'))
const installBuild = (version) => {
  rmSync(claudePath, { recursive: true, force: true })
  execFileSync('go', ['build', '-o', join(claudePath, version), './internal/harness/claude/fakebuild'], { cwd: t.repo, stdio: 'inherit' })
}
// freshMega starts a mega on fresh data and its own port, and returns its
// address, its log and how to stop it.
const freshMega = (name, port) => {
  const fresh = join(t.dir, name)
  mkdirSync(join(fresh, 'data'), { recursive: true })
  const env = { ...otherEnv, XDG_STATE_HOME: join(fresh, 'state'), XDG_CONFIG_HOME: join(fresh, 'config'), XDG_DATA_HOME: join(fresh, 'xdgdata') }
  const p = spawn(t.bin, ['--config', t.config, '--data', join(fresh, 'data'), '--listen', `127.0.0.1:${port}`], { stdio: ['ignore', 'pipe', 'pipe'], env })
  let out = ''
  p.stdout.on('data', (d) => (out += d))
  p.stderr.on('data', (d) => (out += d))
  const exited = new Promise((r) => p.once('exit', (code) => r(code)))
  const log = () => (existsSync(join(fresh, 'state/mega/mega.log')) ? readFileSync(join(fresh, 'state/mega/mega.log'), 'utf8') : '') + out
  const url = `http://127.0.0.1:${port}`
  // ready waits until it answers: spawned is not yet listening, and a page
  // sent there at once can lose that race on a busy machine.
  const ready = () => until(`${name} listening`, async () => (await fetch(`${url}/api/version`).catch(() => null))?.ok === true, 20000)
  return { url, env, log, ready, stop: async () => (p.kill('SIGINT'), await exited) }
}
// harnessLine is the start-up log's line for a harness that can run, with
// its level.
const harnessLine = (log, name) => {
  const line = harnessLines(log).find((l) => l.includes(`harness '${name}' can run`)) ?? ''
  return { line, level: (line.match(/"level":"(\w+)"/) ?? [])[1] }
}
const freshClaude = page.locator('main .first-run [data-harness="claude"]')
const freshCodex = page.locator('main .first-run [data-harness="codex"]')
const classes = async (loc) => ((await loc.getAttribute('class')) ?? '').split(' ')

// a newer Claude Code: plain wherever the version is said, past any range
// the round trips have admitted.
const newer = '2.1.999'
installBuild(newer)
const n = freshMega('fresh-newer', t.port + 2)
const newerRead = await soon(async () => harnessLine(n.log(), 'claude').line !== '' && harnessLine(n.log(), 'codex').line !== '', 20000)
const nc = harnessLine(n.log(), 'claude')
const nx = harnessLine(n.log(), 'codex')
shown('the start-up log, a newer Claude Code', nc.line.trim())
shown('the start-up log, Codex', nx.line.trim())
check(
  newerRead && nc.level === 'INFO' && nc.line.includes(join(claudePath, newer)) && nc.line.includes('the newest build installed') && nc.line.includes(`version '${newer}', newer than the '${claudeTested}' mega was tested with`),
  'the start-up log says a newer Claude Code plainly, at info, found as the newest build installed',
)
check(nx.level === 'INFO' && nx.line.includes(`version '${codexVersion}', of the '${codexTested}' mega was tested with`), 'the start-up log says Codex’s version, inside its range')
await n.ready()
await page.goto(n.url)
const newerShown = await soon(async () => (await freshClaude.count()) > 0 && (await text(freshClaude)).includes(newer) && (await text(freshCodex)).includes(codexVersion), 15000)
shown('the first page on a newer Claude Code', newerShown ? await text(freshClaude) : '(nothing)')
shown('the first page on Codex', newerShown ? await text(freshCodex) : '(nothing)')
check(
  newerShown && (await text(freshClaude)) === `Claude Code can run: version ${newer}, at '${join(claudePath, newer)}', newer than the ${claudeTested} mega was tested with.` && !(await classes(freshClaude)).includes('notice'),
  'the first page says a newer Claude Code in one short plain sentence: its build, where it is and the range, with no node or shim and no notice colour',
)
check(
  newerShown && (await text(freshCodex)) === `Codex can run: version ${codexVersion}, at '${join(t.dir, 'bin/codex')}', within the ${codexTested} mega was tested with.` && !(await classes(freshCodex)).includes('notice'),
  'the first page says Codex the same way, within its range',
)
const newerVersion = await (await fetch(`${n.url}/api/version`)).json()
const byName = Object.fromEntries((newerVersion.harnesses ?? []).map((h) => [h.name, h]))
shown('/api/version on the harnesses', JSON.stringify({ tested: newerVersion.tested, harnesses: newerVersion.harnesses?.map((h) => ({ name: h.name, version: h.version, standing: h.standing, note: h.note })) }))
check(
  newerVersion.tested?.claude === claudeTested && newerVersion.tested?.codex === codexTested && byName.claude?.version === newer && byName.claude?.standing === 'newer' && !byName.claude?.note && byName.codex?.standing === 'tested',
  '/api/version names the tested ranges and the versions found, a newer one with no sentence',
)
await t.shot(page, 'versions-newer')
await n.stop()
// `mega version`, in the same environment, says the same.
const said = execFileSync(t.bin, ['version', '--config', t.config], { env: n.env, encoding: 'utf8' })
for (const line of said.split('\n').filter((l) => /^ {2}(claude|codex):/.test(l))) shown('mega version', line.trim())
check(said.includes(`claude: tested ${claudeTested}; found '${newer}', newer, claude code '${join(claudePath, newer)}'`), '`mega version` says Claude Code’s tested range and the newer build found, plainly')
check(said.includes(`codex: tested ${codexTested}; found '${codexVersion}'`), '`mega version` says Codex’s tested range and the version found')

// an older Claude Code: marked, with the stronger sentence, wherever the
// version is said.
const older = '2.1.300'
installBuild(older)
const o = freshMega('fresh-older', t.port + 3)
const olderRead = await soon(async () => harnessLine(o.log(), 'claude').line !== '', 20000)
const oc = harnessLine(o.log(), 'claude')
shown('the start-up log, an older Claude Code', oc.line.trim())
check(
  olderRead && oc.level === 'WARN' && oc.line.includes(`Claude Code '${older}' is older than the builds mega was tested with, '${claudeTested}'; the Agent SDK mega runs expects Claude Code '2.1.318' or later`),
  'the start-up log warns of an older Claude Code, with the stronger sentence',
)
await o.ready()
await page.goto(o.url)
const olderShown = await soon(async () => (await freshClaude.count()) > 0 && (await text(freshClaude)).includes(older), 15000)
shown('the first page on an older Claude Code', olderShown ? await text(freshClaude) : '(nothing)')
check(
  olderShown && (await text(freshClaude)).startsWith(`Claude Code can run: at '${join(claudePath, older)}'. Claude Code '${older}' is older than the builds mega was tested with`) && (await classes(freshClaude)).includes('notice'),
  'the first page marks an older Claude Code in the notice colour, with the stronger sentence',
)
await t.shot(page, 'versions-older')
await o.stop()

// an update behind the same path, with mega running: Claude Code's updater
// moving the link ~/.local/bin/claude names to a new build, and an npm
// update replacing Codex's and pi's programs in place, a new file under the
// same name with the modification time npm's tarballs carry. the first page
// and the version panel, already open, follow without a restart or a reload.
const npmTime = new Date('1985-10-26T08:15:00Z')
// replace writes a program's new file beside it and moves it over the old
// one, as an installer does.
const replace = (path, write) => {
  write(`${path}.new`)
  chmodSync(`${path}.new`, 0o755)
  utimesSync(`${path}.new`, npmTime, npmTime)
  renameSync(`${path}.new`, path)
}
const relink = (target, link) => {
  rmSync(`${link}.new`, { force: true })
  symlinkSync(target, `${link}.new`)
  renameSync(`${link}.new`, link)
}
for (const v of ['2.1.330', '2.1.331']) copyFileSync(join(claudePath, older), join(claudePath, v))
for (const v of ['2.1.330', '2.1.331']) chmodSync(join(claudePath, v), 0o755)
relink(join(claudePath, '2.1.330'), join(t.dir, 'bin/claude'))
const codexPkg = join(t.dir, 'codex-package')
mkdirSync(codexPkg)
for (const f of ['codex', 'codex-code-mode-host']) replace(join(codexPkg, f), (to) => copyFileSync(join(t.repo, 'internal/harness/codex/fake', f), to))
const codexManifest = (version) => writeFileSync(join(codexPkg, 'codex-package.json'), JSON.stringify({ ...JSON.parse(readFileSync(join(t.repo, 'internal/harness/codex/fake/codex-package.json'), 'utf8')), version }))
codexManifest('0.166.0')
relink(join(codexPkg, 'codex'), join(t.dir, 'bin/codex'))
const piPkg = join(t.dir, 'pi-package')
mkdirSync(piPkg)
const piSays = (version) => (to) => writeFileSync(to, `#!/usr/bin/env node\n// a stand-in for pi: it says its version only.\nprocess.stdout.write('${version}\\n')\n`)
replace(join(piPkg, 'pi'), piSays('1.5.2'))
relink(join(piPkg, 'pi'), join(t.dir, 'bin/pi'))
const freshPi = page.locator('main .first-run [data-harness="pi"]')
const u = freshMega('fresh-update', t.port + 4)
await u.ready()
await page.goto(u.url)
const beforeUpdate = await soon(async () => (await freshPi.count()) > 0 && (await text(freshClaude)).includes('2.1.330') && (await text(freshCodex)).includes('0.166.0') && (await text(freshPi)).includes('1.5.2'), 20000)
shown('the first page before the update', beforeUpdate ? `${await text(freshClaude)} / ${await text(freshCodex)} / ${await text(freshPi)}` : '(nothing)')
check(beforeUpdate && (await text(freshClaude)).includes(`at '${join(t.dir, 'bin/claude')}'`), 'before the update, the first page says each harness’s version, Claude Code at the link on the PATH')
await page.locator('.side-foot .version-label').click()
const versionPanel = page.locator('dialog[open]')
await versionPanel.waitFor()
const panelSays = async (name) => ((await versionPanel.locator(`dd[data-harness="${name}"]`).textContent()) ?? '').trim()
check((await panelSays('claude')).startsWith('Version 2.1.330;') && (await panelSays('pi')).startsWith('Version 1.5.2;'), 'the version panel, opened, says the same versions')
relink(join(claudePath, '2.1.331'), join(t.dir, 'bin/claude'))
codexManifest('0.167.0')
replace(join(codexPkg, 'codex'), (to) => copyFileSync(join(t.repo, 'internal/harness/codex/fake/codex'), to))
replace(join(piPkg, 'pi'), piSays('1.5.3'))
const followed = await soon(async () => (await panelSays('claude')).startsWith('Version 2.1.331;') && (await panelSays('codex')).startsWith('Version 0.167.0;') && (await panelSays('pi')).startsWith('Version 1.5.3;'), 20000)
shown('the version panel after the update', `${await panelSays('claude')} / ${await panelSays('codex')} / ${await panelSays('pi')}`)
check(followed, 'the open version panel follows the update of all three, with no restart and no reload')
await page.keyboard.press('Escape')
const pageFollowed = await soon(async () => (await text(freshClaude)).includes('2.1.331') && (await text(freshCodex)).includes('0.167.0') && (await text(freshPi)).includes('1.5.3'), 5000)
shown('the first page after the update', `${await text(freshClaude)} / ${await text(freshCodex)} / ${await text(freshPi)}`)
check(pageFollowed && !(await text(freshPi)).includes('older'), 'the first page follows too, each harness at its new version')
await u.stop()
rmSync(join(t.dir, 'bin/claude'))
rmSync(join(t.dir, 'bin/pi'))

// nothing on the walk said "internal error", and nothing answered 500.
check(internalShown.length === 0, `no page said "internal error"${internalShown.length ? `: ${internalShown.join(', ')}` : ''}`)
check(fiveHundreds.length === 0, `nothing answered 500${fiveHundreds.length ? `: ${fiveHundreds.join(', ')}` : ''}`)

await t.done()

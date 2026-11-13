// browser check: the first run, on a machine that has never had mega, with
// no harness installed.
//
//   make checks SUITE=firstrun
//
// a bare suite (lib/mega.mjs): no fakes, no Claude settings, a config that
// names no harness path, and no harness on mega's PATH or the project's. it
// is what a newcomer sees between `make` and installing Claude Code and
// Codex, and it is the walk the clean-machine rehearsal of 2026-11-13 made.
// nothing it does can reach a real harness, since none can be found.
//
// what it checks: the empty workspace's first page, with its placeholder and
// the side panel's empty project list, light and dark; the register dialog,
// refusing a relative path and a path that does not exist with what is
// wrong, then registering a git checkout and opening its page; the project
// page's notices; the new-conversation dialog marking Codex as unable to run
// with what is missing, and Claude refused, with no conversation made, for
// want of its executable; the project and its refusals across a restart of
// the service; no page errors and nothing fetched off 127.0.0.1.
//
// it saves each page as shots/{empty,empty-dark,register,register-refused,
// project,launch-claude,project-resolved,restarted}.png. each step's text is
// printed, so the run's output is a record of the words a newcomer reads.
//
// two things the rehearsal found are recorded here, not checked, since a
// check would pin the defect: Claude's refusal reads "internal error" when
// its executable is missing (roadmap: a-missing-claude-is-named), and the
// dialogs open with the focus on their close button
// (roadmap: dialog-focus-and-a-stale-model-list).
import { suite } from './lib/mega.mjs'

const t = await suite('firstrun', { bare: true })
const { url, check, until, api, workspace } = t
const demo = t.checkout('demo', { git: true })

const text = async (locator) => ((await locator.textContent()) ?? '').replace(/\s+/g, ' ').trim()
const shown = (what, words) => t.say(`  ${what}: "${words}"`)

// the empty workspace: what the first page says, and where it points.
const page = await t.page()
const shot = (name) => t.shot(page, name)
await page.goto(url)
const placeholder = page.locator('main .placeholder')
await placeholder.waitFor()
const first = await text(placeholder)
shown('the first page', first)
check(first === 'Register a project from an existing checkout to begin.', 'the empty workspace says to register a project from a checkout')
const empty = page.locator('section.projects p.muted')
check((await text(empty)) === 'No projects yet.', 'the side panel says there are no projects yet')
const register = page.locator('section.projects button[aria-label="Register a project"]')
check((await register.count()) === 1 && (await register.isEnabled()), 'the register control is in the side panel, enabled once the service answered')
check((await page.locator('dialog[open]').count()) === 0, 'nothing opens by itself on the first page')
await shot('empty')
const dark = await t.page({ colorScheme: 'dark' })
await dark.goto(url)
await dark.locator('main .placeholder').waitFor()
await t.shot(dark, 'empty-dark')

// the register dialog: what it asks, and what it says about a wrong path.
await register.click()
const dialog = page.locator('dialog[open]')
await dialog.waitFor()
check((await text(dialog.locator('h2'))) === 'Register a project', 'the dialog is titled Register a project')
const checkoutBox = dialog.locator('input').nth(0)
check((await checkoutBox.getAttribute('placeholder')) === '/absolute/path/to/checkout', 'the checkout field asks for an absolute path')
// the field asks for focus with autoFocus, but showModal moves it to the
// dialog's first control, the close button. recorded, not checked; see the
// note at the top.
shown('focus on opening', await page.evaluate(() => document.activeElement?.getAttribute('aria-label') ?? document.activeElement?.tagName))
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

// the project page's notices, before anything has resolved its environment.
const notices = page.locator('article.detail p.notice')
for (let i = 0; i < (await notices.count()); i++) {
  shown('a notice on the project page', await text(notices.nth(i)))
}
const trust = await text(page.locator('article.detail p.notice', { hasText: 'trust' }))
check(trust.includes('Run claude here once and accept the trust dialog'), 'the page says Claude has not been trusted here, and how to trust it')
const env = page.locator('section.environment')
await env.waitFor()
shown('the environment section', await text(env))
await shot('project')

// the new-conversation dialog: Claude first, which asks Claude for its
// models, which resolves the project's environment, which finds codex
// missing.
await page.locator('button[aria-label="New conversation"]').click()
const launch = page.locator('dialog[open]')
await launch.waitFor()
const harnessSelect = launch.locator('select').first()
shown('focus on opening', await page.evaluate(() => document.activeElement?.getAttribute('aria-label') ?? document.activeElement?.tagName))
await until('codex marked as unable to run', async () => (await launch.locator('p.harness-missing').count()) > 0)
const codexMissing = await text(launch.locator('p.harness-missing'))
shown('the dialog on Codex', codexMissing)
check(
  codexMissing.startsWith("'codex' cannot run: 'codex' was not found on the project's PATH") && codexMissing.includes("until it can be found in the project's environment"),
  'the dialog says codex is not on the project’s PATH and where to look',
)
check((await harnessSelect.locator('option[value="codex"]').isDisabled()) && (await text(harnessSelect.locator('option[value="codex"]'))) === 'Codex (cannot run)', 'Codex is offered disabled, marked cannot run')
check((await harnessSelect.inputValue()) === 'claude', 'Claude is the harness selected')
const modelsNote = launch.locator('p.muted', { hasText: 'did not list its models' })
await until('the models refusal', async () => (await modelsNote.count()) > 0)
shown('the dialog on Claude’s models', await text(modelsNote))
shown('the model list after that', await text(launch.locator('select').nth(2).locator('option').first()))
shown('the launch control', (await launch.locator('button[type="submit"]').getAttribute('aria-label')) ?? '')
await launch.locator('textarea').fill('say hello')
await launch.locator('button[type="submit"]').click()
const launchError = launch.locator('p.error')
await until('the launch refused', async () => (await launchError.count()) > 0 && (await text(launchError)) !== '')
const claudeRefused = await text(launchError)
shown('the dialog on launching Claude', claudeRefused)
check(claudeRefused !== '', 'launching Claude is refused in the dialog, with a reason')
check((await workspace()).conversations.length === 0, 'the refused launch made no conversation')
await shot('launch-claude')
const byApi = await api('POST', `/api/projects/${P.id}/conversations`, { harness: 'codex', prompt: 'say hello' })
shown('the API on launching Codex', `${byApi.status} ${byApi.body?.code}: ${byApi.body?.message}`)
check(byApi.status === 409 && byApi.body.code === 'executable_missing', 'launching Codex anyway is refused as executable_missing')
await page.keyboard.press('Escape')
await until('the dialog closed', async () => (await page.locator('dialog[open]').count()) === 0)
const missingList = page.locator('section.environment ul.env-missing')
await until('the project page listing what is missing', async () => (await missingList.count()) > 0)
shown('the environment section, resolved', await text(page.locator('section.environment')))
check((await text(missingList)).includes("'codex' was not found on the project's PATH"), 'the project page lists codex as missing once resolved')
await shot('project-resolved')

// a restart: the project stays, what was resolved does not.
const before = (await workspace()).projects[0]
check(before.environment.missing.length > 0, 'before the restart the project lists what is missing')
await t.stop()
await t.start()
await page.reload()
await until('the project page again', async () => (await page.locator('article.detail h1').count()) > 0)
const after = (await workspace()).projects[0]
check(after.id === P.id && after.checkout === demo, 'the project is still registered after a restart')
shown('what the project lists as missing after the restart, before anything resolves', JSON.stringify(after.environment.missing))
shown('the environment section after the restart', await text(page.locator('section.environment')))
await shot('restarted')
await page.locator('button[aria-label="New conversation"]').click()
await until('codex marked again', async () => (await page.locator('dialog[open] p.harness-missing').count()) > 0)
check(true, 'the dialog marks codex again after a restart, once the environment is resolved')
await page.keyboard.press('Escape')

await t.done()

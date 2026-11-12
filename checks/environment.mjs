// browser check: a project's environment on its page, against the fakes.
//
//   make checks SUITE=environment
//
// its two scratch checkouts are alpha, a git repository with a
// .mega/environment.yaml, and beta, with none. neither gets the helper's
// Claude-settings override, since this suite is about the file and no file;
// the scratch HOME keeps the operator's settings out instead. the activations
// it runs are the ones it writes and approves through the service, in its
// own scratch workspace.
//
// what it checks: the unapproved state and its refusal on New conversation
// and Adopt; approval; a launch whose header shows the environment's
// fingerprint and changed names; a changed file shown beside the approved
// version (from git), its refusal on Send and Start, and the header saying the
// runtime started in an older environment; approving again; the changed
// variables' values shown only on request; an approval recording the names
// its activation changes, a sourced file that adds one refused with the added
// name shown (never its value), and approving again taking the new names; the
// override (ignore the file, a changed file labelled as such under it, then
// use it again); a project with no file; no page errors and nothing fetched
// off 127.0.0.1.
import { execFileSync } from 'node:child_process'
import { mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { suite } from './lib/mega.mjs'

const t = await suite('environment')
const { url, check, until, api, projectOf, convOf } = t
const alpha = t.checkout('alpha', { git: true })
const beta = t.checkout('beta')
const envFile = (body) => {
  mkdirSync(join(alpha, '.mega'), { recursive: true })
  writeFileSync(join(alpha, '.mega/environment.yaml'), body)
}
const git = (...args) =>
  execFileSync('git', args, { cwd: alpha, env: { ...process.env, GIT_AUTHOR_NAME: 'check', GIT_AUTHOR_EMAIL: 'check@localhost', GIT_COMMITTER_NAME: 'check', GIT_COMMITTER_EMAIL: 'check@localhost' } })
envFile('activate: export MEGA_MARKER=alpha-check-marker\n')
git('add', '.mega/environment.yaml')
git('commit', '-q', '-m', 'environment')

const A = await t.register('alpha', alpha, { override: false })
const B = await t.register('beta', beta, { override: false })
const page = await t.page()
const shot = (name) => t.shot(page, name)
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
const facts = page.locator('.conversation-head .facts', { hasText: /^environment sha256:/ })
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

// the approval holds the activation to the names it changed when approved.
// the file now sources a script in the checkout, which the approval does not
// cover; a change there that adds a name is refused until approved again.
writeFileSync(join(alpha, 'envrc-check'), 'export MEGA_MARKER=alpha-check-marker\n')
envFile('activate: . ./envrc-check\n')
await until('the new file waits', async () => (await projectOf(A.id)).environment.status === 'changed')
await page.reload()
await env.waitFor()
await env.locator('.env-compare').waitFor()
await env.getByRole('button', { name: 'Approve the changed version, so mega runs it' }).click()
await until('ready with its names', async () => {
  const e = (await projectOf(A.id)).environment
  return e.status === 'ready' && e.approved.names_known && e.approved.changed.join() === 'MEGA_MARKER'
})
await until('the page says what it changes', async () => (await env.textContent()).includes('approved as changing MEGA_MARKER'))
check(true, 'approving records the names the activation changes')
writeFileSync(join(alpha, 'envrc-check'), 'export MEGA_MARKER=alpha-check-marker\nexport GIT_SSH_COMMAND=check-only\n')
await env.getByRole('button', { name: 'Resolve the environment now' }).click()
await until('names changed', async () => (await projectOf(A.id)).environment.status === 'names_changed')
const namesRefusal = (await projectOf(A.id)).environment.refusal
check(namesRefusal.code === 'environment_names_changed' && namesRefusal.message.includes('GIT_SSH_COMMAND'), `a changed set of names is refused with its own code: ${namesRefusal.message}`)
await until('the page shows the added name', async () => (await env.locator('.env-names-changed').count()) === 1 && (await env.locator('.env-added').textContent()) === 'GIT_SSH_COMMAND')
check((await page.locator(`.section-actions button[aria-label="${namesRefusal.message}"]`).count()) === 2, 'New conversation and Adopt carry the names refusal')
check(!(await env.textContent()).includes('check-only'), 'the page shows the name, never its value')
check((await env.locator('p.error').count()) === 0 && (await env.textContent()).includes('approved'), 'the refusal is shown once, and the file is still the approved one')
await shot('names-changed')
await env.getByRole('button', { name: 'Approve again, with the names it changes now' }).click()
await until('ready again with the new names', async () => {
  const e = (await projectOf(A.id)).environment
  return e.status === 'ready' && e.approved.changed.join() === 'GIT_SSH_COMMAND,MEGA_MARKER'
})
await until('New conversation is enabled again', async () => page.locator('.section-actions button[aria-label="New conversation"]').isEnabled())
check(true, 'approving again takes the new names and lifts the refusal')

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

await t.done()

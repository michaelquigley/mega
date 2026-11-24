// browser check: requests, against the fakes.
//
//   make checks SUITE=requests
//
// rebuilt from what the journals recorded of the lost mcp-cards, stage3 and
// read-gate scripts and from docs/current. what it checks: a permission card
// docked above the input panel, not in the history, with its one-line row
// there; Allow and Deny with icon and word; focus on the card leaving it where
// it is and the reply box shut; Allow and Deny (with a reason) each clearing
// it, the answer reaching the turn and the row saying answered; answering
// keeping an open box and its draft; two requests stacked oldest first; a
// question card answering only once a choice is made; a request answered on
// another page refused on a page that still shows it, and by the API; an MCP
// approval from the fake codex, with its server, tool and parameter and no
// reason box, allowed for the session or denied; a typed form needing its
// required field; Interrupt aborting a pending card, with a late answer
// refused; a question the fake codex asks in its message, its card steering
// the answer into the waiting turn, and a steer written that codex never
// replies to closing the card, the question's history saying delivery is
// unconfirmed after a reload too, with one steer, one resolution, and a
// second answer sending nothing. and pi, from the fake pi: its extension's gate docked as a question
// from a pi extension, with Dismiss and no answer of one's own or allow for
// the session, answered; Dismiss sending pi's cancel; an input and an editor
// dialog; a dialog that expires; an interrupt that waits on a dialog until it
// is dismissed; and a release with a dialog pending, whose dialog warns that
// pi is ended, leaving the dialog never answered. the allow keys: Ctrl+Enter
// answering the oldest approval from the empty box and from the page, one
// per press, with the key in the Allow's title; nothing from another field,
// with a draft, on a repeat, on a request that just docked, with a dialog
// open, on a question or on a pi dialog; Ctrl+Shift+Enter allowing for the
// session where codex offers it, and nothing where Claude does not. it saves
// the pi dialog, the release warning, the keyed approval and the unconfirmed
// question as shots/{pi-dialog,pi-release-warning,approval-key,
// question-unconfirmed}.png. it approves only
// where the approval itself is under test, and only once it has checked, as
// a step it cannot go past, that the request is exactly the one the fake
// raised; where the decision is incidental, it denies.
import { existsSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { reconnectable, suite } from './lib/mega.mjs'

const t = await suite('requests')
const { url, check, must, until, api, convOf } = t

const P = await t.register('requests', t.checkout('requests'))
const C = (await t.launch(P.id, 'claude', 'echo\nhello')).id
const X = (await t.launch(P.id, 'codex', 'hello')).id
await t.idle(C)
await t.idle(X)

const page = await t.page()
const docked = page.locator('.panel-cluster .docked section.request-card')
const rows = (p, text) => p.locator('.events p.system', { hasText: text })
const reply = (p, text) => p.locator('.events .message.assistant', { hasText: text })
const prompt = async (id, text) => {
  const r = await api('POST', `/api/conversations/${id}/prompt`, { text })
  if (r.status !== 202) throw new Error(`prompting: ${r.status} ${JSON.stringify(r.body)}`)
}
const pending = async (id) => (await convOf(id)).pending

await page.goto(`${url}/#/c/${C}`)
await page.locator('.panel-cluster').waitFor()

// a permission card: docked, above the box, never in the history.
await prompt(C, 'ask')
await docked.waitFor()
const card = docked.first()
must((await card.getAttribute('aria-label')) === 'Approval' && (await card.locator('h2').textContent()) === 'Approval: Bash' && (await card.locator('pre').textContent()) === 'echo fake', 'the permission card names the tool and the command')
check((await page.locator('.events section.request-card').count()) === 0, 'the card is docked, not in the history')
await until('its one-line row in the history', async () => (await rows(page, 'Approval for Bash: waiting for you').count()) === 1)
check(true, 'the history keeps a one-line row for it')
const cardBox = await card.boundingBox()
const formBox = await page.locator('.panel-cluster form.reply').boundingBox()
check(cardBox.y + cardBox.height <= formBox.y + 1, 'the card sits above the reply box')
const allow = card.getByRole('button', { name: 'Allow', exact: true })
const deny = card.getByRole('button', { name: 'Deny', exact: true })
check((await allow.locator('.icon').count()) === 1 && (await allow.locator('.word').textContent()) === 'Allow' && (await deny.locator('.word').textContent()) === 'Deny', 'Allow and Deny carry an icon and a word')

// focus on the card leaves it where it is: the box does not open under it.
await allow.focus()
await until('focus on Allow', async () => allow.evaluate((b) => b === document.activeElement))
const focusedBox = await card.boundingBox()
check((await page.locator('.panel-cluster.expanded').count()) === 0 && Math.abs(focusedBox.y - cardBox.y) < 1, 'focus on the card does not open the box or move the card')

// the only Allow on a permission: the one request the fake raised, and no
// other.
const asked = await pending(C)
must(asked.length === 1 && asked[0].kind === 'permission' && asked[0].payload.tool === 'Bash' && asked[0].payload.input?.command === 'echo fake', 'the one pending request is the fake\'s Bash call, echo fake')
await allow.click()
await until('the card to clear', async () => (await docked.count()) === 0)
await until('the answer to reach the turn', async () => (await reply(page, 'Answered: allow').count()) === 1)
await until('the row to say answered', async () => (await rows(page, 'Approval for Bash: answered').count()) === 1)
check(true, 'Allow clears the card, reaches the turn and the row says answered')

// Deny, with a reason.
await prompt(C, 'ask')
await docked.waitFor()
await docked.first().getByPlaceholder('Reason, sent with a denial').fill('not now, says the check')
await docked.first().getByRole('button', { name: 'Deny', exact: true }).click()
await until('the denied card to clear', async () => (await docked.count()) === 0)
await until('the denial to reach the turn', async () => (await reply(page, 'Answered: deny').count()) === 1)
check((await rows(page, 'Approval for Bash: answered').count()) === 2, 'Deny with a reason clears the card and reaches the turn')

// answering keeps an open box and its draft.
const box = page.getByRole('textbox', { name: 'Reply' })
await box.click()
await box.fill('a draft the answer must not lose')
await until('the box open', async () => (await page.locator('.panel-cluster.expanded').count()) === 1)
await prompt(C, 'ask')
await docked.waitFor()
await docked.first().getByRole('button', { name: 'Deny', exact: true }).click()
await until('the card to clear again', async () => (await docked.count()) === 0)
await until('the answer again', async () => (await reply(page, 'Answered: deny').count()) === 2)
check((await page.locator('.panel-cluster.expanded').count()) === 1 && (await box.inputValue()) === 'a draft the answer must not lose', 'answering a docked request keeps the box open with its draft')
await page.locator('.conversation-head h1').click()
await until('the box shut', async () => (await page.locator('.panel-cluster.expanded').count()) === 0)

// two requests, stacked oldest first.
await prompt(C, 'ask2')
await until('two cards', async () => (await docked.count()) === 2)
check((await docked.nth(0).locator('pre').textContent()) === 'echo first' && (await docked.nth(1).locator('pre').textContent()) === 'echo second', 'two requests stack oldest first')
for (let i = 0; i < 2; i++) {
  await docked.first().getByRole('button', { name: 'Deny', exact: true }).click()
  await until(`card ${i + 1} answered`, async () => (await docked.count()) === 1 - i)
}
await t.idle(C)
check(true, 'both answered, and the turn ends')

// the allow keys: Ctrl+Enter answers the oldest docked approval as its Allow
// does, and Ctrl+Shift+Enter its allow for the session where the harness
// offers one; only from an empty reply box or the page itself, with no
// dialog open, only a deliberate press, and one request per press. a press
// that should do nothing is given time to do something before the check.
const settle = () => page.waitForTimeout(800)
const keys = (shift = false) => page.keyboard.press(shift ? 'Control+Shift+Enter' : 'Control+Enter')
const toPage = () => page.evaluate(() => (document.activeElement instanceof HTMLElement ? document.activeElement.blur() : undefined))
const onPage = () => page.evaluate(() => document.activeElement === document.body)
const answered = (decision) => reply(page, `Answered: ${decision}`).count()
const titleOf = (i, name) => docked.nth(i).getByRole('button', { name, exact: true }).getAttribute('title')
await prompt(C, 'ask2')
await until('two cards for the keys', async () => (await docked.count()) === 2)
check(
  (await titleOf(0, 'Allow')) === 'Allow (Ctrl+Enter)' && (await titleOf(1, 'Allow')) === 'Allow' && (await titleOf(0, 'Deny')) === 'Deny',
  'the oldest approval’s Allow names its key in its title; the next one’s waits its turn, and Deny has no key',
)
// a picture of the card with the title its Allow carries, which the browser
// draws outside the page: the check writes it under the card, and takes it
// away again.
await docked.nth(0).evaluate((card) => {
  const said = document.createElement('p')
  said.className = 'check-caption'
  said.style.cssText = 'font: 12px monospace; margin: 6px 0 0; padding: 4px 6px; border: 1px dashed currentColor;'
  said.textContent = `Allow's title, drawn here by the check: "${card.querySelector('button[data-answer="allow"]').title}"`
  card.append(said)
})
await docked.nth(0).screenshot({ path: `${t.dir}/shots/approval-key.png` })
await page.locator('.check-caption').evaluate((said) => said.remove())
// what the presses answer, those that should and those that should not: the
// fake's two Bash calls, oldest first, looked at before the first press.
const keyed = await pending(C)
must(
  keyed.length === 2 && keyed.every((r) => r.kind === 'permission' && r.payload.tool === 'Bash') && keyed.map((r) => r.payload.input?.command).sort().join(',') === 'echo first,echo second',
  'the two pending requests are the fake’s Bash calls, echo first and echo second',
)
let allows = await answered('allow')
// another field: the denial's reason box.
await docked.nth(0).getByPlaceholder('Reason, sent with a denial').focus()
await keys()
await settle()
check((await docked.count()) === 2 && (await pending(C)).length === 2, 'Ctrl+Enter in another field answers nothing')
// a repeating key, from the empty box.
await box.fill('')
await box.focus()
await box.evaluate((b) => b.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', code: 'Enter', ctrlKey: true, repeat: true, bubbles: true, cancelable: true })))
await settle()
check((await docked.count()) === 2 && (await answered('allow')) === allows, 'a repeating Ctrl+Enter answers nothing')
// a draft: sending wins, and a turn waiting on its approvals sends nothing.
await box.fill('a draft that is not an answer')
await keys()
await settle()
check((await docked.count()) === 2 && (await box.inputValue()) === 'a draft that is not an answer' && (await answered('allow')) === allows, 'with a draft in the box, Ctrl+Enter answers nothing and the draft stays')
await box.fill('')
await keys()
await until('the oldest answered by its key', async () => (await docked.count()) === 1 && (await answered('allow')) === allows + 1)
check((await docked.first().locator('pre').textContent()) === 'echo second', 'Ctrl+Enter from the empty box answers the oldest approval, Allow, as its button does')
await settle()
check((await docked.count()) === 1 && (await titleOf(0, 'Allow')) === 'Allow (Ctrl+Enter)', 'one press answers one request: the next waits for its own press, and now names the key')
await toPage()
check(await onPage(), 'focus is on the page itself')
await keys()
await until('the next answered from the page', async () => (await docked.count()) === 0 && (await answered('allow')) === allows + 2)
check(true, 'Ctrl+Enter from the page itself answers the next')
await t.idle(C)
// a request that docked under the press, and the session-wide key where it
// is not offered: Claude's approvals have no allow for the session.
await toPage()
await prompt(C, 'ask')
await docked.waitFor()
await keys()
await settle()
check((await docked.count()) === 1 && (await pending(C)).length === 1, 'a request docked less than half a second before the press is not answered')
await keys(true)
await settle()
check((await docked.count()) === 1 && (await pending(C)).length === 1, 'Ctrl+Shift+Enter does nothing where the harness offers no allow for the session')
await page.locator('.side-foot .version-label').click()
await page.locator('dialog[open]').waitFor()
await keys()
await settle()
await page.keyboard.press('Escape')
check((await docked.count()) === 1 && (await pending(C)).length === 1, 'nor does Ctrl+Enter while a dialog is open')
const denies = await answered('deny')
await docked.first().getByRole('button', { name: 'Deny', exact: true }).click()
await until('the keyed test denied', async () => (await docked.count()) === 0 && (await answered('deny')) === denies + 1)
await t.idle(C)

// a question card answers only once a choice is made.
await prompt(C, 'askq')
await docked.waitFor()
const q = docked.first()
const answer = q.getByRole('button', { name: 'Answer', exact: true })
check((await q.getAttribute('aria-label')) === 'Question' && (await q.locator('legend').textContent()).includes('Which color do you prefer?') && (await answer.isDisabled()), 'the question card offers its choices, Answer waiting for one')
await settle()
await toPage()
await keys()
await keys(true)
await settle()
check((await docked.count()) === 1 && (await pending(C)).length === 1, 'neither key answers a question')
const questionAsked = await pending(C)
must(questionAsked.length === 1 && questionAsked[0].kind === 'question' && JSON.stringify(questionAsked[0].payload).includes('Which color do you prefer?'), 'the question answered is the fake’s own: Which color do you prefer?')
await q.getByLabel('Blue').check()
await until('Answer enabled', async () => answer.isEnabled())
await answer.click()
await until('the question to clear', async () => (await docked.count()) === 0)
await until('the choice to reach the turn', async () => (await reply(page, 'Answered: Blue').count()) === 1)
await until('its row', async () => (await rows(page, 'Question: answered').count()) === 1)
check(true, 'the answered question clears and its choice reaches the turn')

// answered on another page: a page still showing the card is refused, as is
// the API. this page's workspace reads are held, so its card stays.
const other = await t.page()
await other.goto(`${url}/#/c/${C}`)
await prompt(C, 'ask')
await docked.waitFor()
await other.locator('.panel-cluster .docked section.request-card').waitFor()
const held = []
await page.route('**/api/workspace', (route) => held.push(route))
const requestID = (await pending(C))[0].id
await other.locator('.panel-cluster .docked section.request-card').getByRole('button', { name: 'Deny', exact: true }).click()
await until('the other page answered it', async () => (await pending(C)).length === 0)
await docked.first().getByRole('button', { name: 'Deny', exact: true }).click()
await until('the stale card to say why', async () => (await docked.first().locator('.error').count()) === 1)
check(true, `a page still showing it is refused: "${await docked.first().locator('.error').textContent()}"`)
const again = await api('POST', `/api/conversations/${C}/answer`, { request_id: requestID, decision: 'deny' })
check(again.status === 409 && again.body.code === 'not_pending', `and so is the API: ${again.status} ${again.body.code}`)
for (const route of held) await route.continue()
await page.unroute('**/api/workspace')
await page.reload()
await until('the stale card gone once the page reads again', async () => (await docked.count()) === 0)
check(true, 'the page drops the card once it reads the workspace again')
await other.close()

// an MCP approval from the fake codex: server, tool and parameter, no reason
// box, and an allow for the session.
await page.goto(`${url}/#/c/${X}`)
await page.locator('.panel-cluster').waitFor()
await prompt(X, 'mcp hello-from-the-check')
await docked.waitFor()
const mcp = docked.first()
const mcpText = await mcp.textContent()
must((await mcp.locator('h2').textContent()) === 'Approval: fixture/echo' && mcpText.includes('MCP server fixture wants to run its tool echo') && (await mcp.locator('.mcp-parameters').textContent()).includes('hello-from-the-check'), 'the MCP card names the server, the tool and the parameter')
check((await mcp.getByPlaceholder('Reason, sent with a denial').count()) === 0 && (await mcp.getByRole('button', { name: 'Allow this tool for the session' }).count()) === 1, 'it asks no reason, and offers the session where codex does')
// the only Allow for the session: the fake's own echo tool with this
// parameter, and no other request.
const mcpAsked = await pending(X)
must(mcpAsked.length === 1 && mcpAsked[0].payload.mcp_server === 'fixture' && mcpAsked[0].payload.mcp_tool === 'echo' && JSON.stringify(mcpAsked[0].payload.parameters).includes('hello-from-the-check'), 'the one pending request is the fake\'s fixture/echo call with this parameter')
await mcp.getByRole('button', { name: 'Allow this tool for the session' }).click()
await until('the MCP card to clear', async () => (await docked.count()) === 0)
await until('the tool to run, for the session', async () => (await reply(page, 'echo: hello-from-the-check (approved for the session)').count()) === 1)
check(true, 'allowed for the session, the tool runs')
await t.idle(X)
// Ctrl+Shift+Enter where codex offers the session: what it answers is the
// fake's echo tool with this parameter, checked first.
await prompt(X, 'mcp keyed-for-the-session')
await docked.waitFor()
const sessionKeyed = await pending(X)
must(sessionKeyed.length === 1 && sessionKeyed[0].payload.mcp_server === 'fixture' && sessionKeyed[0].payload.mcp_tool === 'echo' && JSON.stringify(sessionKeyed[0].payload.parameters).includes('keyed-for-the-session'), 'the one pending request is the fake\'s fixture/echo call with this parameter')
check((await titleOf(0, 'Allow this tool for the session')) === 'Allow this tool for the session (Ctrl+Shift+Enter)', 'its allow for the session names its key')
await settle()
await toPage()
await keys(true)
await until('allowed for the session by its key', async () => (await reply(page, 'echo: keyed-for-the-session (approved for the session)').count()) === 1 && (await docked.count()) === 0)
check(true, 'Ctrl+Shift+Enter allows the tool for the session where codex offers it')
await t.idle(X)
await prompt(X, 'mcp declined-by-the-check')
await docked.waitFor()
await docked.first().getByRole('button', { name: 'Deny', exact: true }).click()
await until('the denial', async () => (await reply(page, 'DECLINED').count()) === 1)
// the card leaves once the denial is recorded, which the reply can reach the
// page a moment before.
await until('the denied card to clear', async () => (await docked.count()) === 0)
check((await reply(page, 'echo: declined-by-the-check').count()) === 0, 'denied, the tool does not run')
await t.idle(X)

// a typed form: the required field first.
await prompt(X, 'form')
await docked.waitFor()
const form = docked.first()
const formAnswer = form.getByRole('button', { name: 'Answer', exact: true })
check((await form.locator('h2').textContent()) === 'Question from fixture' && (await form.textContent()).includes('Which color do you prefer?') && (await formAnswer.isDisabled()), 'the form names its server and waits for its required field')
const formAsked = await pending(X)
must(formAsked.length === 1 && formAsked[0].kind === 'question' && formAsked[0].payload.message === 'Which color do you prefer?', 'the form answered is the fixture server’s own')
await form.getByLabel('Blue').check()
await form.getByLabel(/Note/).fill('typed by the check')
await until('Answer enabled', async () => formAnswer.isEnabled())
await formAnswer.click()
await until('the form answered', async () => (await reply(page, 'elicitation answered').count()) === 1)
const formReply = await reply(page, 'elicitation answered').textContent()
check(formReply.includes('Blue') && formReply.includes('typed by the check') && (await docked.count()) === 0, 'the form answer reaches the turn, typed')
await t.idle(X)

// Interrupt aborts a pending card; a late answer is refused.
await prompt(X, 'mcp interrupted-by-the-check')
await docked.waitFor()
const lateID = (await pending(X))[0].id
await page.getByRole('button', { name: 'Interrupt', exact: true }).click()
await until('the card aborted', async () => (await docked.count()) === 0 && (await pending(X)).length === 0)
await until('its row says aborted', async () => (await rows(page, 'aborted').count()) >= 1)
const late = await api('POST', `/api/conversations/${X}/answer`, { request_id: lateID, decision: 'deny' })
check(late.status === 409 && late.body.code === 'not_pending', `Interrupt aborts the card, and a late answer is refused: ${late.status} ${late.body.code}`)
await t.shot(page, 'requests')

// a question codex asks in its message, while its turn waits: the card steers
// the answer into that turn, once. each steer the fake receives is in its
// log, beside its threads in the scratch HOME.
const steerLog = join(t.dir, 'home', '.fake-codex', 'steers.jsonl')
const steers = () => (existsSync(steerLog) ? readFileSync(steerLog, 'utf8').split('\n').filter(Boolean).length : 0)
const outcomes = async (id, requestID) =>
  (await api('GET', `/api/conversations/${id}/events?since=0`)).body.events.filter((e) => e.kind === 'request_resolved' && e.req?.id === requestID).map((e) => e.req.outcome)
const answerBlue = async () => {
  const asked = docked.first()
  await asked.getByLabel('Blue').check()
  await asked.getByRole('button', { name: 'Answer', exact: true }).click()
}
await prompt(X, 'askq')
await docked.waitFor()
const steered = await pending(X)
must(steered.length === 1 && steered[0].kind === 'question' && JSON.stringify(steered[0].payload).includes('Which color do you prefer?'), 'the question codex asks in its message docks while its turn waits, and is the fake’s own')
const steersBefore = steers()
await answerBlue()
await until('the answer steered into the turn', async () => (await reply(page, 'answered: Blue').count()) === 1 && (await docked.count()) === 0)
check(steers() === steersBefore + 1 && (await outcomes(X, steered[0].id)).join() === 'answered', 'the card steers the answer into the turn that asked, once, and the question is answered')
await t.idle(X)

// a steer written that codex never replies to, here because the app-server
// exits on it: the card closes, and the question's history says Codex did not
// confirm the answer, after a reload too. one steer, one resolution, and a
// second answer sends nothing.
const unconfirmed = 'Question: Codex did not confirm whether it received your answer'
const L = (await t.launch(P.id, 'codex', 'hello')).id
await t.idle(L)
await page.goto(`${url}/#/c/${L}`)
await page.locator('.panel-cluster').waitFor()
await prompt(L, 'askq lose')
await docked.waitFor()
const lost = await pending(L)
must(lost.length === 1 && lost[0].kind === 'question' && JSON.stringify(lost[0].payload).includes('Which color do you prefer?'), 'the question answered is the fake’s own')
const lostBefore = steers()
await answerBlue()
await until('the card to close', async () => (await docked.count()) === 0 && (await pending(L)).length === 0)
await until('the question’s history to say the answer is unconfirmed', async () => (await rows(page, unconfirmed).count()) === 1)
const lostOutcomes = await outcomes(L, lost[0].id)
check(steers() === lostBefore + 1 && lostOutcomes.join() === 'unconfirmed', `one steer, and one resolution, unconfirmed: ${steers() - lostBefore} sent, ${JSON.stringify(lostOutcomes)}`)
await t.shot(page, 'question-unconfirmed')
await page.reload()
await page.locator('.panel-cluster').waitFor()
await until('the history after a reload', async () => (await rows(page, unconfirmed).count()) === 1)
check((await docked.count()) === 0, 'after a reload there is no card, and the question’s history still says Codex did not confirm the answer')
const lostAgain = await api('POST', `/api/conversations/${L}/answer`, { request_id: lost[0].id, answers: { 'Which color do you prefer?': 'Red' } })
check(lostAgain.status === 409 && lostAgain.body.code === 'not_pending' && steers() === lostBefore + 1, `a second answer is refused, and sends nothing: ${lostAgain.status} ${lostAgain.body.code}`)

// pi: every dialog an extension raises is a question, docked as the others,
// never an approval and never allowed for the session. each offers Dismiss,
// pi's own cancel, sent only when it is pressed.
const Pi = (await t.launch(P.id, 'pi', 'echo hello')).id
await t.idle(Pi)
await page.evaluate((id) => {
  location.hash = `#/c/${id}`
}, Pi)
await until('the pi conversation', async () => (await reply(page, 'hello').count()) === 1)
await prompt(Pi, 'ask')
await docked.waitFor()
const piCard = docked.first()
must(
  (await piCard.getAttribute('aria-label')) === 'Question' &&
    (await piCard.locator('h2').textContent()) === 'Question from a pi extension' &&
    (await piCard.textContent()).includes('Approve bash command: sleep 1'),
  "the extension's gate is a question from a pi extension, never an approval",
)
const dismiss = piCard.getByRole('button', { name: /^Dismiss/ })
check(
  (await dismiss.count()) === 1 &&
    (await dismiss.locator('.word').textContent()) === 'Dismiss' &&
    (await piCard.getByPlaceholder('Or write your own answer').count()) === 0 &&
    (await piCard.getByRole('button', { name: /for the session/ }).count()) === 0,
  "it offers Dismiss, takes only pi's own choices, and has no allow for the session",
)
await piCard.screenshot({ path: `${t.dir}/shots/pi-dialog.png` })
// a pi dialog is a question: neither key answers it.
await settle()
await toPage()
await keys()
await keys(true)
await settle()
check((await docked.count()) === 1 && (await pending(Pi)).length === 1, 'neither key answers a pi dialog')
// the only Allow to a pi gate: the one the fake raised, checked above.
await piCard.getByLabel('Allow').check()
await piCard.getByRole('button', { name: 'Answer', exact: true }).click()
await until('the gate answered', async () => (await reply(page, 'ran').count()) === 1 && (await docked.count()) === 0)
check(true, 'answered, the gated command runs and the turn goes on')
await t.idle(Pi)

// Dismiss sends pi's cancel: the extension gets no answer.
await prompt(Pi, 'choose')
await docked.waitFor()
await docked.first().getByRole('button', { name: /^Dismiss/ }).click()
await until('the choice dismissed', async () => (await reply(page, 'dismissed').count()) === 1 && (await docked.count()) === 0)
check(true, 'Dismiss reaches pi as its cancel')
await t.idle(Pi)

// an input dialog is a one-field form; an editor's is several lines,
// prefilled.
await prompt(Pi, 'input')
await docked.waitFor()
const inputAsked = await pending(Pi)
must(inputAsked.length === 1 && JSON.stringify(inputAsked[0].payload).includes('Your name?'), 'the input dialog answered is the fake’s own: Your name?')
await docked.first().getByRole('textbox').fill('Ada')
await docked.first().getByRole('button', { name: 'Answer', exact: true }).click()
await until('the input answered', async () => (await reply(page, 'you typed: Ada').count()) === 1)
await t.idle(Pi)
await prompt(Pi, 'editor')
await docked.waitFor()
const editorAsked = await pending(Pi)
must(editorAsked.length === 1 && JSON.stringify(editorAsked[0].payload).includes('Edit the note'), 'the editor dialog answered is the fake’s own: Edit the note')
const editorBox = docked.first().locator('textarea')
check((await editorBox.inputValue()) === 'line one\nline two', 'an editor dialog is several lines, prefilled as pi gave it')
await editorBox.fill('line one\nline three')
await docked.first().getByRole('button', { name: 'Answer', exact: true }).click()
await until('the editor answered', async () => (await reply(page, 'line three').count()) === 1)
check(true, 'input and editor answers reach pi as text')
await t.idle(Pi)

// a dialog with its own time limit expires when pi settles it, and says so.
await prompt(Pi, 'timed')
await docked.waitFor()
await until('the dialog expired', async () => (await docked.count()) === 0 && (await reply(page, 'timed out').count()) === 1, 10000)
const expired = rows(page, 'expired')
await until('its row says expired', async () => (await expired.count()) === 1)
t.say(`  the row: "${await expired.textContent()}"`)
check((await expired.textContent()).includes('its time ran out before an answer'), 'the row says the dialog expired, in plain words')
await t.idle(Pi)

// an interrupt with a dialog pending waits on it, and says so; the turn ends
// once the operator dismisses the dialog. mega answers it for nobody.
await prompt(Pi, 'ask')
await docked.waitFor()
await page.getByRole('button', { name: 'Interrupt', exact: true }).click()
const waiting = page.locator('.events p.system.waiting')
await until('the interrupt waiting', async () => (await waiting.count()) === 1)
t.say(`  the row: "${await waiting.textContent()}"`)
check((await waiting.textContent()).startsWith("Interrupt waiting on a dialog: the interrupt waits on pi's dialog 'Approve bash command: sleep 1'"), 'the interrupt says it waits on the dialog, naming it')
await new Promise((r) => setTimeout(r, 800))
check((await docked.count()) === 1 && (await convOf(Pi)).turn_active, 'the dialog stays docked and the turn goes on until it is answered')
// with the interrupt under way, a chosen answer is refused, as a conflict,
// and the page holds it; Dismiss stays live. the answer tried is Block, the
// gate's own refusal, after the request is checked to be the gate.
const gate = (await pending(Pi))[0]
must(gate?.kind === 'question' && gate.payload?.questions?.[0]?.question === 'Approve bash command: sleep 1', 'the pending request is the extension’s gate')
const chosen = await api('POST', `/api/conversations/${Pi}/answer`, { request_id: gate.id, answers: { [gate.payload.questions[0].question]: 'Block' } })
check(chosen.status === 409 && chosen.body?.code === 'conflict' && /Deny or Dismiss can still be pressed/.test(chosen.body?.message ?? ''), `after the interrupt a chosen answer is refused: ${chosen.status} ${JSON.stringify(chosen.body)}`)
const stoppingCard = docked.first()
await until('the card holds its answer', async () => (await stoppingCard.locator('.request-stopping').count()) === 1)
check(
  (await stoppingCard.getByRole('button', { name: 'Answer', exact: true }).isDisabled()) && !(await stoppingCard.getByRole('button', { name: /^Dismiss/ }).isDisabled()),
  `the page holds the answer, saying why, and leaves Dismiss live: "${await stoppingCard.locator('.request-stopping').textContent()}"`,
)
check((await pending(Pi)).length === 1, 'the request is still pending')
await docked.first().getByRole('button', { name: /^Dismiss/ }).click()
await until('the turn interrupted', async () => !(await convOf(Pi)).turn_active && (await docked.count()) === 0)
check((await rows(page, 'Turn ended: interrupted').count()) === 1, 'dismissed, the interrupt goes through and the turn ends interrupted')

// releasing with a dialog pending: the release dialog says pi is ended, and
// the dialog is recorded as never answered.
await prompt(Pi, 'ask')
await docked.waitFor()
await page.getByRole('button', { name: 'More', exact: true }).click()
await page.getByRole('menuitem', { name: 'Release' }).click()
const releaseDialog = page.locator('dialog[open]')
await releaseDialog.waitFor()
const warning = releaseDialog.locator('p.release-pi-dialog')
check(
  (await warning.count()) === 1 &&
    (await warning.textContent()).includes('pi cannot be stopped politely while it waits, so releasing ends its process') &&
    (await warning.textContent()).includes('the command the dialog gates does not run'),
  'the release dialog warns that pi is ended and the gated command does not run',
)
const confirmation = await releaseDialog.textContent()
check(
  confirmation.includes('Release stops the runtime. The next prompt or Start brings the conversation back on the same session.') &&
    !confirmation.includes('terminal') &&
    (await releaseDialog.locator('.command').count()) === 0 &&
    (await releaseDialog.getByRole('button', { name: /Copy/ }).count()) === 0,
  'the release dialog is a confirmation: it says what Release does, and offers no terminal command',
)
await releaseDialog.screenshot({ path: `${t.dir}/shots/pi-release-warning.png` })
await releaseDialog.getByRole('button', { name: 'Release', exact: true }).click()
await until('pi ended', async () => (await convOf(Pi)).state === 'none', 30000)
const never = rows(page, 'never answered')
await until('its row says never answered', async () => (await never.count()) === 1)
t.say(`  the row: "${await never.textContent()}"`)
check((await reply(page, 'ran').count()) === 1, 'the gated command did not run')

// while the page is stale, after a reconnect and before the workspace has
// been read again, a docked request is held, saying why, and so are the
// allow keys; once it has caught up, it is answerable again.
await t.idle(C)
const rp = await t.page()
const stream = await reconnectable(rp)
await rp.goto(`${url}/#/c/${C}`)
await rp.locator('.panel-cluster').waitFor()
await prompt(C, 'ask')
const rDocked = rp.locator('.panel-cluster .docked section.request-card')
await rDocked.waitFor()
const rAllow = rDocked.first().getByRole('button', { name: 'Allow', exact: true })
const rDeny = rDocked.first().getByRole('button', { name: 'Deny', exact: true })
await until('the card answerable', async () => !(await rAllow.isDisabled()))
await stream.drop()
await until('the page catching up', async () => (await rp.locator('.side-foot .stream').textContent()) === 'Catching up' && stream.held() > 0)
const heldNote = rDocked.first().locator('.request-held')
check(
  (await rAllow.isDisabled()) && (await rDeny.isDisabled()) && (await heldNote.textContent()).startsWith('Held until the page has caught up with mega'),
  `while the page is stale, the docked request's answers are held, saying why: "${await heldNote.textContent()}"`,
)
await rp.getByRole('textbox', { name: 'Reply' }).focus()
await new Promise((r) => setTimeout(r, 800))
await rp.keyboard.press('Control+Enter')
await new Promise((r) => setTimeout(r, 800))
check((await pending(C)).length === 1, 'and Ctrl+Enter from the empty box answers nothing')
await rDocked.first().screenshot({ path: `${t.dir}/shots/request-held.png` })
await stream.catchUp()
await until('caught up', async () => (await rp.locator('.side-foot .stream').textContent()) === 'Live')
await until('the card answerable again', async () => !(await rDeny.isDisabled()) && (await heldNote.count()) === 0)
await rDeny.click()
await until('answered once caught up', async () => (await pending(C)).length === 0)
check(true, 'once the page has caught up, the request is answerable again, and answered')

await t.done()

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
// refused. it approves only where the approval itself is under test, and
// only once it has checked, as a step it cannot go past, that the request is
// exactly the one the fake raised; where the decision is incidental, it
// denies.
import { suite } from './lib/mega.mjs'

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

// a question card answers only once a choice is made.
await prompt(C, 'askq')
await docked.waitFor()
const q = docked.first()
const answer = q.getByRole('button', { name: 'Answer', exact: true })
check((await q.getAttribute('aria-label')) === 'Question' && (await q.locator('legend').textContent()).includes('Which color do you prefer?') && (await answer.isDisabled()), 'the question card offers its choices, Answer waiting for one')
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
await prompt(X, 'mcp declined-by-the-check')
await docked.waitFor()
await docked.first().getByRole('button', { name: 'Deny', exact: true }).click()
await until('the denial', async () => (await reply(page, 'DECLINED').count()) === 1)
check((await docked.count()) === 0, 'denied, the tool does not run')
await t.idle(X)

// a typed form: the required field first.
await prompt(X, 'form')
await docked.waitFor()
const form = docked.first()
const formAnswer = form.getByRole('button', { name: 'Answer', exact: true })
check((await form.locator('h2').textContent()) === 'Question from fixture' && (await form.textContent()).includes('Which color do you prefer?') && (await formAnswer.isDisabled()), 'the form names its server and waits for its required field')
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

await t.done()

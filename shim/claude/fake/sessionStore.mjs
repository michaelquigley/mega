import { appendFileSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { randomUUID } from 'node:crypto'
import { join } from 'node:path'

export const sessionIndex = (store) => join(store, 'sessions.jsonl')
export const messageFile = (store, session) => join(store, `messages-${session}.jsonl`)

// Add a terminal exchange to a released fake session. The browser can then
// exercise the distinction between a listed session with no messages and a
// session the fake no longer lists.
export function seedExchange(store, session, { prompt, reply, attachment = false }) {
  const index = sessionIndex(store)
  const path = messageFile(store, session)
  const belongsToSession = (line) => line && JSON.parse(line).id === session
  const listed = readFileSync(index, 'utf8').split('\n').find(belongsToSession)
  if (!listed) {
    throw new Error(`no fake Claude session '${session}'`)
  }
  const message = (role, content) => JSON.stringify({
    type: role,
    uuid: randomUUID(),
    session_id: session,
    message: { role, content },
  })
  const content = attachment ? [{ type: 'text', text: prompt }, { type: 'image' }] : prompt
  const added = [
    message('user', content),
    message('assistant', [{ type: 'text', text: reply }]),
  ]
  appendFileSync(path, added.join('\n') + '\n')
  const populated = readFileSync(path, 'utf8')

  return {
    empty() {
      writeFileSync(path, '')
    },
    missing() {
      rmSync(path, { force: true })
      const others = readFileSync(index, 'utf8').split('\n').filter((line) => !belongsToSession(line))
      writeFileSync(index, others.join('\n'))
    },
    restore() {
      writeFileSync(path, populated)
      appendFileSync(index, listed + '\n')
    },
  }
}

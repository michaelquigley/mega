// seeds session files straight into the folder pi, and the fake pi, keep for
// a checkout: sessions/--<real path, / replaced by ->--/ in ~/.pi/agent under
// home, each a version 3 header and one user message, as pi 1.5.2 writes
// them, with the file's time set to the session's, which is what pi's
// discovery orders by. for a check that needs more sessions than it could
// launch. kept here, beside the fake pi, so pi's session format stays with
// its adapter; checks/dialogs.mjs calls it. each session is { prompt, at },
// at a Date. it returns the sessions' ids, in the order given.
import { mkdirSync, realpathSync, utimesSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'

export function seedSessions(home, cwd, sessions) {
  const real = realpathSync(cwd)
  const folder = join(home, '.pi', 'agent', 'sessions', '--' + real.replace(/^\//, '').replace(/[/\\:]/g, '-') + '--')
  mkdirSync(folder, { recursive: true })
  return sessions.map(({ prompt, at }, i) => {
    const id = `00000000-0000-4000-8000-${String(i).padStart(12, '0')}`
    const file = join(folder, `${at.toISOString().replace(/[:.]/g, '-')}_${id}.jsonl`)
    const lines = [
      { type: 'session', version: 3, id, timestamp: at.toISOString(), cwd: real },
      { type: 'message', id: 'm1', parentId: null, timestamp: at.toISOString(), message: { role: 'user', content: [{ type: 'text', text: prompt }], timestamp: at.getTime() } },
    ]
    writeFileSync(file, lines.map((l) => JSON.stringify(l)).join('\n') + '\n')
    utimesSync(file, at, at)
    return id
  })
}

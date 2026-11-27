// seeds threads straight into the fake codex's thread store, threads.json in
// a CODEX_HOME, else ~/.fake-codex under home, in the shape the fake keeps
// them, for a check that needs more threads than it could launch. kept here,
// beside the fake codex, so the store's shape stays with it;
// checks/dialogs.mjs calls it. each thread is { preview, createdAt,
// updatedAt }, times in whole seconds, as codex reports them. it returns the
// threads' ids, in the order given.
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'

export function seedThreads(home, cwd, threads) {
  const dir = join(home, '.fake-codex')
  const file = join(dir, 'threads.json')
  const store = existsSync(file) ? JSON.parse(readFileSync(file, 'utf8')) : {}
  const ids = threads.map(({ preview, createdAt, updatedAt }, i) => {
    const id = `019a0000-0000-7000-8000-${String(i).padStart(12, '0')}`
    store[id] = { id, cwd, preview, createdAt, updatedAt, turns: [] }
    return id
  })
  mkdirSync(dir, { recursive: true })
  writeFileSync(file, JSON.stringify(store))
  return ids
}

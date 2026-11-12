// runs the browser checks: every suite in checks/, or the ones named.
//
//   make checks                 every suite
//   make checks SUITE=requests  one suite (a space-separated list works too)
//
// each suite runs as its own process, one after another, against the
// committed fakes, and prints its steps; its output also goes to
// <scratch>/<suite>.out. the report gives each suite's steps, failures and
// time. it exits 0 when every suite passed, 1 when a step failed, and 3 when
// a suite could not run at all (the browser missing, say).
import { spawn, execFileSync } from 'node:child_process'
import { createWriteStream, existsSync, mkdirSync, readdirSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { browserPath, CANNOT_RUN, repo } from './lib/mega.mjs'

const here = join(repo, 'checks')
const all = readdirSync(here)
  .filter((f) => f.endsWith('.mjs') && f !== 'run.mjs')
  .map((f) => f.replace(/\.mjs$/, ''))
  .sort()
const asked = process.argv.slice(2).flatMap((a) => a.split(/[\s,]+/)).filter(Boolean)
const unknown = asked.filter((s) => !all.includes(s))
if (unknown.length) {
  console.error(`no such suite: ${unknown.join(', ')}; the suites are ${all.join(', ')}`)
  process.exit(2)
}
const suites = asked.length ? asked : all

// the browser first: a missing one stops everything with the fix, before any
// service is started.
browserPath()

const root = process.env.MEGA_CHECKS_DIR || join(tmpdir(), 'mega-checks')
mkdirSync(root, { recursive: true })
const env = { ...process.env, MEGA_CHECKS_DIR: root }
if (!env.MEGA_CHECK_BIN) {
  env.MEGA_CHECK_BIN = join(root, 'mega')
  console.log(`building mega into ${env.MEGA_CHECK_BIN}`)
  execFileSync('go', ['build', '-o', env.MEGA_CHECK_BIN, './cmd/mega'], { cwd: repo, stdio: 'inherit' })
}
if (!existsSync(env.MEGA_CHECK_BIN)) {
  console.error(`cannot run: no mega binary at ${env.MEGA_CHECK_BIN}`)
  process.exit(CANNOT_RUN)
}

const run = (name) =>
  new Promise((resolve) => {
    const started = Date.now()
    const out = createWriteStream(join(root, `${name}.out`))
    const child = spawn(process.execPath, [join(here, `${name}.mjs`)], { cwd: here, env, stdio: ['ignore', 'pipe', 'pipe'] })
    let result = null
    let tail = ''
    const take = (chunk) => {
      const text = chunk.toString()
      out.write(text)
      tail = (tail + text).slice(-4000)
      for (const line of text.split('\n')) {
        if (line.startsWith('RESULT ')) {
          result = JSON.parse(line.slice(7))
        } else if (line.trim()) {
          process.stdout.write(`  ${name}: ${line}\n`)
        }
      }
    }
    child.stdout.on('data', take)
    child.stderr.on('data', take)
    child.on('exit', (code) => {
      out.end()
      resolve({ name, code, result, seconds: (Date.now() - started) / 1000, tail })
    })
  })

const results = []
for (const name of suites) {
  console.log(`\n== ${name}`)
  results.push(await run(name))
}

console.log('\nsuite           steps  failed  time')
let worst = 0
for (const r of results) {
  const steps = r.result ? String(r.result.steps) : '-'
  const failed = r.result ? String(r.result.failed) : r.code === CANNOT_RUN ? 'could not run' : 'crashed'
  console.log(`${r.name.padEnd(15)} ${steps.padStart(5)}  ${failed.padStart(6)}  ${r.seconds.toFixed(1)}s`)
  const code = r.code === 0 && r.result && r.result.failed === 0 ? 0 : r.code === CANNOT_RUN ? CANNOT_RUN : 1
  worst = Math.max(worst, code)
}
console.log(worst === 0 ? `\nall ${results.length} suites passed` : `\nsome suites did not pass; their output is in ${root}/<suite>.out`)
process.exit(worst)

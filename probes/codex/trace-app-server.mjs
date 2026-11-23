#!/usr/bin/env node
// Copy beside a symlink named codex in scratch. This records JSON-RPC headers
// while piping the request and response bytes unchanged.
import { spawn } from 'node:child_process'
import { appendFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { createInterface } from 'node:readline'

const dir = dirname(process.argv[1])
const args = process.argv.slice(2)
const appServer = args[0] === 'app-server'
const child = spawn(join(dir, 'codex'), args, {
  stdio: appServer ? ['pipe', 'pipe', 'inherit'] : 'inherit',
})

child.on('error', (err) => {
  console.error(err)
  process.exitCode = 1
})

if (appServer) {
  const observe = (stream, direction) => {
    const lines = createInterface({ input: stream, crlfDelay: Infinity })
    lines.on('line', (line) => {
      let frame
      try {
        frame = JSON.parse(line)
      } catch {
        return
      }
      if (!frame || typeof frame !== 'object' || Array.isArray(frame)) return
      const { id, method } = frame
      const reply = method === undefined && id !== undefined
        ? ('error' in frame ? 'error' : 'success')
        : undefined
      const row = { at: new Date().toISOString(), direction, id, method, reply }
      appendFileSync(join(dir, 'app-server-trace.jsonl'), JSON.stringify(row) + '\n', { mode: 0o600 })
    })
    return lines
  }
  const input = observe(process.stdin, 'to-codex')
  observe(child.stdout, 'from-codex')
  child.stdin.on('error', (err) => {
    if (err.code !== 'EPIPE') throw err
  })
  process.stdin.pipe(child.stdin)
  child.stdout.pipe(process.stdout)
  child.on('close', () => {
    input.close()
    process.stdin.pause()
  })
}
child.on('close', (code) => { process.exitCode = code ?? 1 })

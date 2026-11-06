#!/usr/bin/env node
// NOT RUN in the recorded investigation: starting a loopback WebSocket listener
// was denied by the session's permission classifier. Run only with the
// operator's approval. Two clients against ONE
// `codex app-server --listen ws://127.0.0.1:PORT`: client A starts a thread
// and a turn that needs approval; client B rejoins the thread with
// thread/resume. Records which client receives the approval request, whether a
// duplicate answer errors, and what each connection observes.
//   node probe-ws.mjs --cwd DIR --log logs/ws.jsonl [--port 45010] [--answerer A|B]
import { spawn } from 'node:child_process';
import { createWriteStream } from 'node:fs';
import { resolve } from 'node:path';
const argv = process.argv.slice(2); const opt = {};
for (let i = 0; i < argv.length; i++) { const a = argv[i]; if (!a.startsWith('--')) continue; const n = argv[i + 1]; if (n === undefined || n.startsWith('--')) opt[a.slice(2)] = true; else { opt[a.slice(2)] = n; i++; } }
const ts = () => new Date().toISOString(); const say = s => process.stderr.write(`[ws ${ts()}] ${s}\n`);
const log = createWriteStream(opt.log ?? `logs/ws-${Date.now()}.jsonl`, { flags: 'a' });
const rec = (kind, data) => log.write(JSON.stringify({ t: ts(), kind, ...data }) + '\n');
const cwd = resolve(opt.cwd ?? '.'); const port = Number(opt.port ?? 45010); const answerer = opt.answerer ?? 'B';
const server = spawn('codex', ['app-server', '--listen', `ws://127.0.0.1:${port}`], { cwd, stdio: ['ignore', 'pipe', 'pipe'] });
server.stdout.on('data', d => rec('server_stdout', { data: String(d).slice(0, 500) }));
server.stderr.on('data', d => rec('server_stderr', { data: String(d).replace(/\x1b\[[0-9;]*m/g, '').slice(0, 500) }));
server.on('exit', (code, sig) => { rec('server_exit', { code, sig }); say(`server exit ${code} ${sig}`); });
await new Promise(r => setTimeout(r, 1500));

function client(name) {
  const ws = new WebSocket(`ws://127.0.0.1:${port}`);
  let id = 1; const pending = new Map(); const listeners = new Set(); const seen = []; const serverRequests = [];
  const c = { name, ws, seen, serverRequests, listeners };
  ws.onmessage = ev => {
    const m = JSON.parse(ev.data); rec('recv', { client: name, frame: m });
    if (m.id !== undefined && m.method === undefined) { const p = pending.get(m.id); pending.delete(m.id); if (!p) return; m.error ? p.rej(Object.assign(new Error(m.error.message), { rpc: m.error })) : p.res(m.result); return; }
    if (m.id !== undefined && m.method) { serverRequests.push({ id: m.id, method: m.method, command: m.params?.command }); say(`${name} <- SERVER REQUEST ${m.method} id=${m.id}`); for (const fn of listeners) fn(m); return; }
    seen.push(m.method); for (const fn of listeners) fn(m);
    if (['thread/status/changed', 'turn/started', 'turn/completed', 'serverRequest/resolved', 'thread/closed', 'error'].includes(m.method)) say(`${name} <- ${m.method}`);
  };
  c.send = o => { rec('send', { client: name, frame: o }); ws.send(JSON.stringify(o)); };
  c.request = (method, params) => new Promise((res, rej) => { const i = id++; pending.set(i, { res, rej }); c.send({ jsonrpc: '2.0', id: i, method, params }); });
  c.respond = (rid, result) => c.send({ jsonrpc: '2.0', id: rid, result });
  c.ready = new Promise((res, rej) => { ws.onopen = res; ws.onerror = () => rej(new Error('ws error')); });
  c.waitFor = (pred, ms = 240000) => new Promise((res, rej) => { const t = setTimeout(() => { listeners.delete(fn); rej(new Error(name + ' timeout')); }, ms); const fn = m => { if (pred(m)) { clearTimeout(t); listeners.delete(fn); res(m); } }; listeners.add(fn); });
  c.init = async () => { const r = await c.request('initialize', { clientInfo: { name: `roundhouse-ws-${name}`, title: `Roundhouse ws ${name}`, version: '0.0.0' }, capabilities: { experimentalApi: true, requestAttestation: false } }); c.send({ jsonrpc: '2.0', method: 'initialized', params: {} }); return r; };
  return c;
}
const summary = {};
try {
  const A = client('A'); await A.ready; await A.init();
  const started = await A.request('thread/start', { cwd }); const threadId = started.thread.id; summary.threadId = threadId;
  const B = client('B'); await B.ready; await B.init();
  const rj = await B.request('thread/resume', { threadId, cwd }).catch(e => ({ error: String(e), rpc: e.rpc }));
  summary.B_resume = rj.error ? rj : { status: rj.thread?.status, priorTurns: rj.thread?.turns?.length };
  summary.loaded = (await A.request('thread/loaded/list', {})).data;
  const gotA = A.waitFor(m => m.method === 'item/commandExecution/requestApproval', 120000).catch(() => null);
  const gotB = B.waitFor(m => m.method === 'item/commandExecution/requestApproval', 120000).catch(() => null);
  const turnDoneA = A.waitFor(m => m.method === 'turn/completed', 240000);
  const turnDoneB = B.waitFor(m => m.method === 'turn/completed', 240000).catch(() => null);
  await A.request('turn/start', { threadId, input: [{ type: 'text', text: 'Create a file named ws-probe.txt in the working directory containing WS-MULTI-1 using a shell command, then reply WS-MULTI-1. Nothing else.', text_elements: [] }] });
  await Promise.race([gotA, gotB, new Promise(r => setTimeout(r, 120000))]);
  await new Promise(r => setTimeout(r, 3000));
  summary.approvalSeenBy = { A: A.serverRequests.map(x => x.id), B: B.serverRequests.map(x => x.id) };
  const pref = answerer === 'A' ? A : B; const other = pref === A ? B : A;
  const who = pref.serverRequests[0] ? pref : (other.serverRequests[0] ? other : null);
  if (who) {
    const req = who.serverRequests[0]; rec('approval_answer', { client: who.name, id: req.id }); who.respond(req.id, { decision: 'accept' });
    const dupHolder = who === A ? B : A;
    if (dupHolder.serverRequests.find(x => x.id === req.id)) { await new Promise(r => setTimeout(r, 1500)); rec('duplicate_answer', { client: dupHolder.name, id: req.id }); dupHolder.respond(req.id, { decision: 'accept' }); }
  } else summary.note = 'no approval request received by either client';
  summary.turnStatus = (await turnDoneA).params.turn.status;
  summary.B_saw_turn_completed = !!(await Promise.race([turnDoneB, new Promise(r => setTimeout(() => r(null), 5000))]));
  summary.A_notifications = [...new Set(A.seen)]; summary.B_notifications = [...new Set(B.seen)];
  B.ws.close(); await new Promise(r => setTimeout(r, 2000));
  summary.loadedAfterBClose = (await A.request('thread/loaded/list', {})).data;
  A.ws.close(); await new Promise(r => setTimeout(r, Number(opt['keep-alive-ms'] ?? 8000)));
  summary.serverAliveAfterAllClientsClosed = server.exitCode === null;
  const C = client('C'); await C.ready; await C.init();
  summary.loadedAfterAllClosed = (await C.request('thread/loaded/list', {})).data;
  const r3 = await C.request('thread/read', { threadId, includeTurns: true }); summary.C_read_turns = r3.thread.turns.length; summary.C_read_status = r3.thread.status;
  C.ws.close();
} catch (e) { summary.error = String(e); say(`error ${e}`); }
rec('summary', summary); server.kill('SIGTERM'); await new Promise(r => setTimeout(r, 1500)); log.end();
console.log(JSON.stringify(summary, null, 2)); process.exit(0);

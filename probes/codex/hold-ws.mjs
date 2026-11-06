#!/usr/bin/env node
// Loopback multi-client test with a held approval, for the native remote TUI.
// Phase 1: client A starts a read-only thread and completes one trivial turn so
// the rollout exists; client B rejoins with thread/resume; A starts a turn that
// needs approval (file write under read-only sandbox); record which clients
// receive the server request; B answers; A sends a duplicate answer.
// Phase 2 (hold): A starts another write turn and nobody answers. The script
// writes status to --status-file and stays alive, logging everything either
// client observes, until --stop-file appears. Michael connects
// `codex --remote ws://127.0.0.1:PORT` during the hold.
//   node hold-ws.mjs --cwd DIR --log logs/hold.jsonl --port 45011 --status-file S --stop-file F
import { spawn } from 'node:child_process';
import { createWriteStream, existsSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
const argv = process.argv.slice(2); const opt = {};
for (let i = 0; i < argv.length; i++) { const a = argv[i]; if (!a.startsWith('--')) continue; const n = argv[i + 1]; if (n === undefined || n.startsWith('--')) opt[a.slice(2)] = true; else { opt[a.slice(2)] = n; i++; } }
const ts = () => new Date().toISOString(); const say = s => process.stderr.write(`[hold ${ts()}] ${s}\n`);
const log = createWriteStream(opt.log ?? `logs/hold-${Date.now()}.jsonl`, { flags: 'a' });
const rec = (kind, data) => log.write(JSON.stringify({ t: ts(), kind, ...data }) + '\n');
const cwd = resolve(opt.cwd ?? '.'); const port = Number(opt.port ?? 45011);
const statusFile = opt['status-file']; const stopFile = opt['stop-file'];
const status = { phase: 'starting', port };
const flush = () => { if (statusFile) writeFileSync(statusFile, JSON.stringify(status, null, 2)); };
const server = spawn('codex', ['app-server', '--listen', `ws://127.0.0.1:${port}`], { cwd, stdio: ['ignore', 'pipe', 'pipe'] });
server.stdout.on('data', d => rec('server_stdout', { data: String(d).slice(0, 500) }));
server.stderr.on('data', d => rec('server_stderr', { data: String(d).replace(/\x1b\[[0-9;]*m/g, '').slice(0, 500) }));
server.on('exit', (code, sig) => { rec('server_exit', { code, sig }); say(`server exit ${code} ${sig}`); });
await new Promise(r => setTimeout(r, 1500));
function client(name) {
  const ws = new WebSocket(`ws://127.0.0.1:${port}`);
  let id = 1; const pending = new Map(); const listeners = new Set(); const serverRequests = [];
  const c = { name, ws, serverRequests, listeners, seen: [] };
  ws.onmessage = ev => {
    const m = JSON.parse(ev.data); rec('recv', { client: name, frame: m });
    if (m.id !== undefined && m.method === undefined) { const p = pending.get(m.id); pending.delete(m.id); if (!p) { say(`${name} <- unmatched response id=${m.id} ${JSON.stringify(m).slice(0, 160)}`); return; } m.error ? p.rej(Object.assign(new Error(m.error.message), { rpc: m.error })) : p.res(m.result); return; }
    if (m.id !== undefined && m.method) { serverRequests.push({ id: m.id, method: m.method, t: ts(), command: m.params?.command, threadId: m.params?.threadId }); say(`${name} <- SERVER REQUEST ${m.method} id=${m.id}`); for (const fn of listeners) fn(m); return; }
    c.seen.push(m.method); for (const fn of listeners) fn(m);
    if (['thread/status/changed', 'turn/started', 'turn/completed', 'serverRequest/resolved', 'thread/closed', 'error', 'item/completed'].includes(m.method)) say(`${name} <- ${m.method} ${JSON.stringify(m.params).slice(0, 140)}`);
  };
  ws.onclose = () => { say(`${name} socket closed`); rec('client_closed', { client: name }); };
  c.send = o => { rec('send', { client: name, frame: o }); ws.send(JSON.stringify(o)); };
  c.request = (method, params) => new Promise((res, rej) => { const i = id++; pending.set(i, { res, rej }); c.send({ jsonrpc: '2.0', id: i, method, params }); });
  c.respond = (rid, result) => c.send({ jsonrpc: '2.0', id: rid, result });
  c.ready = new Promise((res, rej) => { ws.onopen = res; ws.onerror = () => rej(new Error('ws error')); });
  c.waitFor = (pred, ms = 240000) => new Promise((res, rej) => { const t = setTimeout(() => { listeners.delete(fn); rej(new Error(name + ' timeout')); }, ms); const fn = m => { if (pred(m)) { clearTimeout(t); listeners.delete(fn); res(m); } }; listeners.add(fn); });
  c.init = async () => { const r = await c.request('initialize', { clientInfo: { name: `roundhouse-hold-${name}`, title: `Roundhouse hold ${name}`, version: '0.0.0' }, capabilities: { experimentalApi: true, requestAttestation: false } }); c.send({ jsonrpc: '2.0', method: 'initialized', params: {} }); return r; };
  return c;
}
const summary = {};
try {
  const A = client('A'); await A.ready; await A.init();
  const started = await A.request('thread/start', { cwd, approvalPolicy: 'on-request', sandbox: 'read-only' });
  const threadId = started.thread.id; summary.threadId = threadId; status.threadId = threadId; status.phase = 'phase1'; flush();
  say(`thread ${threadId} sandbox=${JSON.stringify(started.sandbox).slice(0, 80)} approval=${JSON.stringify(started.approvalPolicy)}`);
  const d1 = A.waitFor(m => m.method === 'turn/completed');
  await A.request('turn/start', { threadId, input: [{ type: 'text', text: 'Reply with exactly WS-HOLD-0 and nothing else.', text_elements: [] }] });
  await d1; await new Promise(r => setTimeout(r, 1500));
  const B = client('B'); await B.ready; await B.init();
  const rj = await B.request('thread/resume', { threadId, cwd }).catch(e => ({ error: String(e), rpc: e.rpc }));
  summary.B_resume = rj.error ? rj : { status: rj.thread?.status, priorTurns: rj.thread?.turns?.length };
  say(`B resume -> ${JSON.stringify(summary.B_resume)}`);
  const gotA = A.waitFor(m => m.method === 'item/commandExecution/requestApproval', 150000).catch(() => null);
  const gotB = B.waitFor(m => m.method === 'item/commandExecution/requestApproval', 150000).catch(() => null);
  const doneA = A.waitFor(m => m.method === 'turn/completed', 300000);
  const doneB = B.waitFor(m => m.method === 'turn/completed', 300000).catch(() => null);
  await A.request('turn/start', { threadId, input: [{ type: 'text', text: 'Create a file named ws-multi.txt in the working directory containing WS-MULTI-1 using a shell command (it will need approval), then reply WS-MULTI-1. Nothing else.', text_elements: [] }] });
  await Promise.race([Promise.all([gotA, gotB]), new Promise(r => setTimeout(r, 150000))]);
  await new Promise(r => setTimeout(r, 4000));
  summary.approvalSeenBy = { A: A.serverRequests.map(x => x.id), B: B.serverRequests.map(x => x.id) };
  say(`approval seen by ${JSON.stringify(summary.approvalSeenBy)}`);
  const bReq = B.serverRequests[0]; const aReq = A.serverRequests[0];
  if (bReq) { rec('approval_answer', { client: 'B', id: bReq.id }); B.respond(bReq.id, { decision: 'accept' }); }
  else if (aReq) { rec('approval_answer', { client: 'A', id: aReq.id }); A.respond(aReq.id, { decision: 'accept' }); }
  if (bReq && aReq) { await new Promise(r => setTimeout(r, 2000)); rec('duplicate_answer', { client: 'A', id: aReq.id }); A.respond(aReq.id, { decision: 'accept' }); summary.duplicateAnswerSent = true; }
  summary.turnStatus = (await doneA).params.turn.status;
  summary.B_saw_turn_completed = !!(await Promise.race([doneB, new Promise(r => setTimeout(() => r(null), 5000))]));
  summary.A_notifications = [...new Set(A.seen)]; summary.B_notifications = [...new Set(B.seen)];
  summary.fileWritten = existsSync(resolve(cwd, 'ws-multi.txt'));
  rec('phase1_summary', summary); say(`phase1 ${JSON.stringify(summary).slice(0, 400)}`);
  // Phase 2: hold a pending approval for the remote TUI.
  status.phase = 'hold'; status.phase1 = summary; flush();
  A.listeners.add(m => { if (m.method && m.id !== undefined) { status.pendingRequests = A.serverRequests.map(x => ({ id: x.id, method: x.method, t: x.t })); flush(); } });
  await A.request('turn/start', { threadId, input: [{ type: 'text', text: 'Create a file named ws-hold.txt in the working directory containing WS-HOLD-1 using a shell command (it will need approval), then reply WS-HOLD-1. Nothing else.', text_elements: [] }] });
  say('hold phase: waiting for stop file; nobody answers the next approval');
  while (!(stopFile && existsSync(stopFile))) await new Promise(r => setTimeout(r, 1000));
  summary.holdSeenA = A.seen.slice(-40); summary.holdServerRequestsA = A.serverRequests; summary.holdServerRequestsB = B.serverRequests;
  summary.holdLoaded = await A.request('thread/loaded/list', {}).catch(e => String(e));
  summary.holdRead = await A.request('thread/read', { threadId, includeTurns: true }).then(r => ({ turns: r.thread.turns.length, status: r.thread.status })).catch(e => String(e));
  B.ws.close(); A.ws.close();
} catch (e) { summary.error = String(e); say(`error ${e}`); }
status.phase = 'done'; status.summary = summary; flush();
rec('summary', summary); server.kill('SIGTERM'); await new Promise(r => setTimeout(r, 1500)); log.end();
console.log(JSON.stringify(summary, null, 2)); process.exit(0);

#!/usr/bin/env node
// Roundhouse Codex probe: drive `codex app-server` over stdio JSON-RPC.
// Starts or resumes a persisted thread, runs scripted turns, answers approval
// and user-input server requests by policy, and logs every frame to JSONL.
//
// Usage:
//   node probe.mjs run --cwd DIR --turns scenarios/x.json --log logs/x.jsonl \
//        [--resume THREAD_ID] [--approve REGEX] [--approve-delay-ms N] [--answer REGEX] \
//        [--interrupt-after-ms N [--interrupt-turn I]] [--interrupt-on-approval] \
//        [--stale-after-interrupt] [--kill-owner-after-ms N] [--kill-server-after-ms N] \
//        [--approval-policy untrusted|on-request|never] [--sandbox read-only|workspace-write]
//   node probe.mjs list --cwd DIR
//   node probe.mjs read --thread THREAD_ID
//
// Policy: an approval whose "<command or reason>" matches --approve is accepted
// (after --approve-delay-ms); otherwise declined. requestUserInput questions are
// answered with the first option whose label matches --answer (default: first).
// No dependencies. Protocol shapes taken from `codex app-server generate-ts`
// for the installed version (see README.md).

import { spawn } from 'node:child_process';
import { createWriteStream, readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { createInterface } from 'node:readline';

const argv = process.argv.slice(2);
const cmd = argv.shift();
const opt = {};
for (let i = 0; i < argv.length; i++) {
  const a = argv[i]; if (!a.startsWith('--')) continue;
  const k = a.slice(2); const n = argv[i + 1];
  if (n === undefined || n.startsWith('--')) opt[k] = true; else { opt[k] = n; i++; }
}
const ts = () => new Date().toISOString();
const say = s => process.stderr.write(`[probe ${ts()}] ${s}\n`);
let logEnded = false;
const log = createWriteStream(opt.log ?? `logs/run-${Date.now()}.jsonl`, { flags: 'a' });
const rec = (kind, data) => { if (!logEnded) log.write(JSON.stringify({ t: ts(), kind, ...data }) + '\n'); };

// ---- transport -----------------------------------------------------------
const child = spawn('codex', ['app-server'], { cwd: opt.cwd ? resolve(opt.cwd) : process.cwd(), stdio: ['pipe', 'pipe', 'pipe'] });
rec('spawn', { pid: child.pid, cwd: opt.cwd ? resolve(opt.cwd) : process.cwd() });
child.stderr.on('data', d => rec('server_stderr', { data: String(d).slice(0, 2000) }));
let nextId = 1; const pending = new Map();
function send(obj) { rec('send', { frame: obj }); child.stdin.write(JSON.stringify(obj) + '\n'); }
function request(method, params) {
  const id = nextId++;
  return new Promise((res, rej) => { pending.set(id, { res, rej, method }); send({ jsonrpc: '2.0', id, method, params }); });
}
function notify(method, params) { send({ jsonrpc: '2.0', method, params }); }
function respond(id, result) { send({ jsonrpc: '2.0', id, result }); }
function respondError(id, message) { send({ jsonrpc: '2.0', id, error: { code: -32000, message } }); }
const listeners = new Set();
const onNotification = fn => { listeners.add(fn); return () => listeners.delete(fn); };
const waitFor = (pred, timeoutMs = 300000) => new Promise((res, rej) => {
  const t = setTimeout(() => { off(); rej(new Error('timeout waiting for notification')); }, timeoutMs);
  const off = onNotification(m => { if (pred(m)) { clearTimeout(t); off(); res(m); } });
});
const exitP = new Promise(r => child.on('exit', (code, sig) => { rec('server_exit', { code, sig }); say(`app-server exited code=${code} sig=${sig}`); r({ code, sig }); }));

const lastAgentText = new Map();
const summary = { thread: null, turns: [], serverRequests: [], statusChanges: [], exit: null };
let currentThreadId = null; let currentTurnId = null;

createInterface({ input: child.stdout }).on('line', line => {
  let m; try { m = JSON.parse(line); } catch { rec('recv_unparsed', { line: line.slice(0, 500) }); return; }
  rec('recv', { frame: m });
  if (m.id !== undefined && m.method === undefined) { // response
    const p = pending.get(m.id); if (!p) return; pending.delete(m.id);
    if (m.error) p.rej(Object.assign(new Error(`${p.method}: ${m.error.message}`), { rpc: m.error })); else p.res(m.result);
    return;
  }
  if (m.id !== undefined && m.method) { handleServerRequest(m); return; }
  // notification
  for (const fn of listeners) fn(m);
  const p = m.params ?? {};
  switch (m.method) {
    case 'thread/status/changed': summary.statusChanges.push({ t: ts(), status: p.status }); say(`thread status -> ${JSON.stringify(p.status)}`); break;
    case 'turn/started': say(`turn started ${p.turn?.id}`); break;
    case 'turn/completed': say(`turn completed ${p.turn?.id} status=${p.turn?.status} error=${p.turn?.error ? JSON.stringify(p.turn.error).slice(0, 200) : 'null'}`); break;
    case 'item/started': say(`item started ${p.item?.type} ${p.item?.id} ${p.item?.command ? JSON.stringify(p.item.command).slice(0, 100) : ''}`); break;
    case 'item/completed': if (p.item?.type === 'agentMessage') { lastAgentText.set(p.turnId, (lastAgentText.get(p.turnId) ? lastAgentText.get(p.turnId) + '\n' : '') + p.item.text); say(`agent message: ${p.item.text.slice(0, 300)}`); } else say(`item completed ${p.item?.type} ${p.item?.id} ${p.item?.status ?? ''} ${p.item?.exitCode !== undefined ? 'exit=' + p.item.exitCode : ''}`); break;
    case 'serverRequest/resolved': say(`serverRequest resolved ${JSON.stringify(p.requestId)}`); break;
    case 'error': say(`error notification: ${JSON.stringify(p).slice(0, 300)}`); break;
    case 'item/agentMessage/delta': case 'item/reasoning/summaryTextDelta': case 'item/reasoning/textDelta': case 'item/commandExecution/outputDelta': case 'thread/tokenUsage/updated': case 'account/rateLimits/updated': break;
    default: say(`notification ${m.method}`);
  }
});

const approveRe = opt.approve ? new RegExp(opt.approve) : null;
const answerRe = opt.answer ? new RegExp(opt.answer) : null;
const approveDelay = Number(opt['approve-delay-ms'] ?? 0);
const resolved = new Set();
onNotification(m => { if (m.method === 'serverRequest/resolved') resolved.add(JSON.stringify(m.params.requestId)); });

async function handleServerRequest(m) {
  const p = m.params ?? {}; const started = Date.now();
  const entry = { requestId: m.id, method: m.method, threadId: p.threadId, turnId: p.turnId, itemId: p.itemId, approvalId: p.approvalId, command: p.command, reason: p.reason, cwd: p.cwd, kind: p.kind, proposedExecpolicyAmendment: p.proposedExecpolicyAmendment, questions: p.questions, t: ts() };
  summary.serverRequests.push(entry);
  rec('server_request', entry);
  say(`SERVER REQUEST ${m.method} id=${m.id} command=${JSON.stringify(p.command ?? null)} reason=${JSON.stringify(p.reason ?? null)}`);
  if (m.method === 'item/tool/requestUserInput') {
    const answers = {};
    for (const q of p.questions ?? []) {
      const opts = q.options ?? [];
      const pick = (answerRe && opts.find(o => answerRe.test(o.label))) ?? opts[0];
      answers[q.id] = { answers: [pick ? pick.label : 'no option'] };
    }
    rec('user_input_answer', { requestId: m.id, answers });
    say(`answering requestUserInput ${JSON.stringify(answers)}`);
    respond(m.id, { answers }); return;
  }
  if (m.method === 'item/commandExecution/requestApproval' || m.method === 'item/fileChange/requestApproval' || m.method === 'item/permissions/requestApproval') {
    const key = `${p.command ?? ''} ${p.reason ?? ''}`;
    if (opt['interrupt-on-approval'] && currentTurnId) {
      say('interrupting while approval pending');
      const r = await request('turn/interrupt', { threadId: currentThreadId, turnId: currentTurnId }).catch(e => ({ error: String(e) }));
      rec('interrupt_during_approval', { requestId: m.id, response: r });
      await new Promise(r => setTimeout(r, 3000));
      const wasResolved = resolved.has(JSON.stringify(m.id));
      rec('approval_state_after_interrupt', { requestId: m.id, serverRequestResolvedSeen: wasResolved });
      say(`after interrupt: serverRequest/resolved seen for this request = ${wasResolved}`);
      if (opt['stale-after-interrupt']) {
        say('sending STALE accept for the interrupted approval');
        rec('stale_response_sent', { requestId: m.id, decision: 'accept' });
        respond(m.id, { decision: 'accept' });
      } else { respond(m.id, { decision: 'decline' }); }
      return;
    }
    const accept = approveRe && approveRe.test(key);
    if (accept && approveDelay) { await new Promise(r => setTimeout(r, approveDelay)); rec('approval_wait', { requestId: m.id, waitedMs: Date.now() - started, resolvedMeanwhile: resolved.has(JSON.stringify(m.id)) }); }
    const decision = accept ? 'accept' : 'decline';
    rec('approval_decision', { requestId: m.id, decision });
    say(`${decision} ${m.method}`);
    respond(m.id, { decision }); return;
  }
  say(`unhandled server request ${m.method}; replying error`);
  respondError(m.id, 'probe: unhandled server request');
}

// ---- commands ------------------------------------------------------------
async function init() {
  const r = await request('initialize', { clientInfo: { name: 'roundhouse-probe', title: 'Roundhouse probe', version: '0.0.0' }, capabilities: { experimentalApi: true, requestAttestation: false } });
  notify('initialized', {});
  rec('initialized', { result: r });
  return r;
}
async function shutdown() {
  child.stdin.end();
  const r = await Promise.race([exitP, new Promise(res => setTimeout(() => res('still_running_after_15s'), 15000))]);
  if (r === 'still_running_after_15s') { say('server did not exit after stdin close; sending SIGTERM'); child.kill('SIGTERM'); summary.exit = await exitP; summary.exitNote = 'needed SIGTERM'; } else summary.exit = r;
}

if (cmd === 'list') {
  await init();
  const r = await request('thread/list', { cwd: resolve(opt.cwd ?? '.'), limit: Number(opt.limit ?? 20) });
  console.log(JSON.stringify(r.data.map(t => ({ id: t.id, cwd: t.cwd, path: t.path, preview: t.preview?.slice(0, 80), status: t.status, model: t.model, createdAt: t.createdAt, updatedAt: t.updatedAt, source: t.source })), null, 2));
  await shutdown(); process.exit(0);
}
if (cmd === 'read') {
  await init();
  const r = await request('thread/read', { threadId: opt.thread, includeTurns: true });
  const t = r.thread;
  console.log(JSON.stringify({ id: t.id, cwd: t.cwd, path: t.path, status: t.status, model: t.model, cliVersion: t.cliVersion, source: t.source }, null, 2));
  for (const turn of t.turns ?? []) for (const it of turn.items ?? []) {
    if (it.type === 'userMessage') console.log(`user\t${turn.id}\t${(it.content ?? []).map(c => c.text ?? `[${c.type}]`).join(' ').slice(0, 200)}`);
    else if (it.type === 'agentMessage') console.log(`agent\t${turn.id}\t${it.text.slice(0, 200)}`);
    else console.log(`${it.type}\t${turn.id}\t${(it.command ?? '').toString().slice(0, 100)} ${it.status ?? ''}`);
  }
  await shutdown(); process.exit(0);
}
if (cmd !== 'run') { console.error('unknown command'); process.exit(2); }

const cwd = resolve(opt.cwd ?? '.');
const turns = JSON.parse(readFileSync(opt.turns, 'utf8')).map(t => typeof t === 'string' ? { prompt: t } : t);
rec('probe_start', { cwd, opt });
const hard = setTimeout(() => { say('hard timeout'); rec('hard_timeout', {}); child.kill('SIGKILL'); process.exit(3); }, Number(opt['timeout-ms'] ?? 600000));

try {
  await init();
  const startParams = { cwd };
  if (opt['approval-policy']) startParams.approvalPolicy = opt['approval-policy'];
  if (opt.sandbox) startParams.sandbox = opt.sandbox;
  const r = opt.resume ? await request('thread/resume', { threadId: opt.resume, ...startParams }) : await request('thread/start', startParams);
  currentThreadId = r.thread.id;
  summary.thread = { id: r.thread.id, sessionId: r.thread.sessionId, path: r.thread.path, cwd: r.cwd, model: r.model, modelProvider: r.modelProvider, approvalPolicy: r.approvalPolicy, approvalsReviewer: r.approvalsReviewer, sandbox: r.sandbox, instructionSources: r.instructionSources, reasoningEffort: r.reasoningEffort, status: r.thread.status, priorTurns: (r.thread.turns ?? []).length, forkedFromId: r.thread.forkedFromId, cliVersion: r.thread.cliVersion };
  say(`${opt.resume ? 'resumed' : 'started'} thread ${r.thread.id} path=${r.thread.path} model=${r.model} approval=${JSON.stringify(r.approvalPolicy)} sandbox=${JSON.stringify(r.sandbox).slice(0, 120)} priorTurns=${(r.thread.turns ?? []).length} instructions=${JSON.stringify(r.instructionSources)}`);

  if (opt['kill-server-after-ms']) setTimeout(() => { say('SIGTERM app-server mid-run'); rec('server_sigterm', {}); child.kill('SIGTERM'); }, Number(opt['kill-server-after-ms']));
  if (opt['kill-owner-after-ms']) setTimeout(() => { rec('owner_sigkill', { pid: process.pid, serverPid: child.pid }); say(`SIGKILL self pid=${process.pid}; app-server pid=${child.pid}`); logEnded = true; log.end(() => process.kill(process.pid, 'SIGKILL')); }, Number(opt['kill-owner-after-ms']));

  for (let i = 0; i < turns.length; i++) {
    const t = turns[i];
    const tr = { i: i + 1, prompt: t.prompt, sentAt: ts() };
    summary.turns.push(tr);
    say(`send turn ${i + 1}: ${t.prompt.slice(0, 80)}`);
    const completed = waitFor(m => m.method === 'turn/completed' && m.params.threadId === currentThreadId && m.params.turn?.id === currentTurnId);
    const resp = await request('turn/start', { threadId: currentThreadId, input: [{ type: 'text', text: t.prompt, text_elements: [] }] });
    currentTurnId = resp.turn.id; tr.turnId = currentTurnId;
    rec('turn_accepted', { turnId: currentTurnId, i: i + 1 });
    if (opt['interrupt-after-ms'] && i + 1 === Number(opt['interrupt-turn'] ?? 1)) {
      setTimeout(async () => {
        say(`interrupting turn ${currentTurnId}`); rec('interrupt_sent', { turnId: currentTurnId });
        const ir = await request('turn/interrupt', { threadId: currentThreadId, turnId: currentTurnId }).catch(e => ({ error: String(e), rpc: e.rpc }));
        rec('interrupt_response', { turnId: currentTurnId, response: ir }); say(`interrupt response ${JSON.stringify(ir)}`);
      }, Number(opt['interrupt-after-ms']));
    }
    const done = await completed;
    tr.status = done.params.turn.status; tr.error = done.params.turn.error; tr.completedAt = ts();
    tr.agentText = (done.params.turn.items ?? []).filter(x => x.type === 'agentMessage').map(x => x.text).join('\n---\n');
    if (!tr.agentText) tr.agentText = lastAgentText.get(currentTurnId) ?? '';
  }
  if (opt['stale-after-interrupt'] && opt['interrupt-after-ms']) { /* stale for interrupted turn approvals handled in handler */ }
  say('all turns done; shutting down');
  await shutdown();
} catch (e) {
  rec('probe_error', { error: String(e), rpc: e?.rpc });
  say(`probe error: ${e}`);
  summary.error = String(e);
  try { await shutdown(); } catch {}
} finally { clearTimeout(hard); }
rec('summary', summary); logEnded = true; log.end();
console.log(JSON.stringify(summary, null, 2));
process.exit(0);

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
//        [--relay SOCK [--hold]]  owner mode: server requests wait for a relay client's answer
//        [--steer-answer TEXT]    when an agentMessage with questions[] (async question) appears,
//                                 send TEXT into the active turn with turn/steer
//        [--start-json JSON] [--turn-json JSON]  extra params merged into thread/start|resume and turn/start
//        [--config k=v]           passed to `codex app-server -c k=v` (repeatable via comma)
//        [--linger-ms N]          wait N ms after the last turn before closing stdin
//        [--close-on-approval]     suspension test: close stdin while an approval is pending, record exit
//        [--stale-after-harness-resolve]  in relay mode, send a raw late JSON-RPC accept 2 s after the
//                                 server itself resolved a pending request (records the server's reaction)
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
import { createRelay } from '../common/relay.mjs';

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
const serverArgs = ['app-server']; for (const kv of String(opt.config ?? '').split(',').filter(Boolean)) serverArgs.push('-c', kv);
const relay = opt.relay ? createRelay({ socketPath: opt.relay, log: (k, d) => rec(k, d) }) : null;
const child = spawn('codex', serverArgs, { cwd: opt.cwd ? resolve(opt.cwd) : process.cwd(), stdio: ['pipe', 'pipe', 'pipe'] });
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
let currentThreadId = null; let currentTurnId = null; let steered = false;

createInterface({ input: child.stdout }).on('line', line => {
  let m; try { m = JSON.parse(line); } catch { rec('recv_unparsed', { line: line.slice(0, 500) }); return; }
  rec('recv', { frame: m });
  if (relay) relay.event('msg', m);
  if (relay && m.method === 'serverRequest/resolved' && m.params?.requestId !== undefined) { if (relay.resolve(String(m.params.requestId), 'resolved_by_harness')) rec('relay_resolved_by_harness', { requestId: m.params.requestId }); }
  if (relay && m.method === 'thread/status/changed') relay.setState({ status: m.params?.status });
  if (opt['steer-answer'] && m.method === 'item/completed' && m.params?.item?.type === 'agentMessage' && Array.isArray(m.params.item.questions) && m.params.item.questions.length && !steered) {
    steered = true; const turnId = m.params.turnId ?? currentTurnId;
    rec('async_question_seen', { turnId, itemId: m.params.item.id, delivery: m.params.item.delivery, questions: m.params.item.questions });
    say(`async question seen in turn ${turnId}: ${JSON.stringify(m.params.item.questions).slice(0, 200)}; steering answer`);
    setTimeout(async () => {
      const r = await request('turn/steer', { threadId: currentThreadId, expectedTurnId: turnId, input: [{ type: 'text', text: opt['steer-answer'], text_elements: [] }] }).catch(e => ({ error: String(e), rpc: e.rpc }));
      rec('steer_response', { turnId, response: r }); say(`turn/steer -> ${JSON.stringify(r)}`);
    }, Number(opt['steer-delay-ms'] ?? 2000));
  }
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
  if (relay && (m.method === 'item/tool/requestUserInput' || m.method.endsWith('/requestApproval'))) {
    const kind = m.method === 'item/tool/requestUserInput' ? 'question' : 'permission';
    say(`relay: waiting for a client to answer ${kind} ${m.id}`);
    const r = await relay.request(String(m.id), kind, { method: m.method, params: p });
    rec('relay_request_outcome', { requestId: m.id, ...r });
    if (!r.answered) {
      say(`request ${m.id} ${r.outcome} before any client answered; no response sent`);
      if (opt['stale-after-harness-resolve']) { setTimeout(() => { rec('stale_raw_response_sent', { requestId: m.id, decision: 'accept' }); say(`sending RAW STALE accept for resolved request ${m.id}`); respond(m.id, { decision: 'accept' }); }, 2000); }
      return;
    }
    if (kind === 'question') respond(m.id, { answers: r.result.answers ?? {} }); else respond(m.id, { decision: r.result.decision ?? 'decline' });
    return;
  }
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
    if (opt['close-on-approval']) {
      say('approval pending: closing stdin WITHOUT answering (suspension test)'); rec('close_on_approval', { requestId: m.id });
      const t0 = Date.now(); child.stdin.end();
      const r = await Promise.race([exitP, new Promise(res => setTimeout(() => res('still_running_after_20s'), 20000))]);
      rec('close_on_approval_exit', { requestId: m.id, result: r, afterMs: Date.now() - t0 }); say(`after stdin close with approval pending: ${JSON.stringify(r)} (${Date.now() - t0} ms)`);
      summary.exit = r === 'still_running_after_20s' ? null : r; summary.closeOnApproval = { requestId: m.id, result: r };
      if (r === 'still_running_after_20s') { say('server still alive 20 s after stdin close; SIGTERM own child'); child.kill('SIGTERM'); summary.exit = await exitP; summary.exitNote = 'needed SIGTERM'; }
      rec('summary', summary); logEnded = true; log.end(); console.log(JSON.stringify(summary, null, 2)); process.exit(0);
    }
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

let holdResolve = null; let closing = false;
const cwd = resolve(opt.cwd ?? '.');
const turns = JSON.parse(readFileSync(opt.turns, 'utf8')).map(t => typeof t === 'string' ? { prompt: t } : t);
rec('probe_start', { cwd, opt });
const hard = setTimeout(() => { say('hard timeout'); rec('hard_timeout', {}); child.kill('SIGKILL'); process.exit(3); }, Number(opt['timeout-ms'] ?? 600000));

try {
  await init();
  const startParams = { cwd };
  if (opt['approval-policy']) startParams.approvalPolicy = opt['approval-policy'];
  if (opt.sandbox) startParams.sandbox = opt.sandbox;
  if (opt['start-json']) Object.assign(startParams, JSON.parse(opt['start-json']));
  const r = opt.resume ? await request('thread/resume', { threadId: opt.resume, ...startParams }) : await request('thread/start', startParams);
  currentThreadId = r.thread.id;
  summary.thread = { id: r.thread.id, sessionId: r.thread.sessionId, path: r.thread.path, cwd: r.cwd, model: r.model, modelProvider: r.modelProvider, approvalPolicy: r.approvalPolicy, approvalsReviewer: r.approvalsReviewer, sandbox: r.sandbox, instructionSources: r.instructionSources, reasoningEffort: r.reasoningEffort, status: r.thread.status, priorTurns: (r.thread.turns ?? []).length, forkedFromId: r.thread.forkedFromId, cliVersion: r.thread.cliVersion };
  say(`${opt.resume ? 'resumed' : 'started'} thread ${r.thread.id} path=${r.thread.path} model=${r.model} approval=${JSON.stringify(r.approvalPolicy)} sandbox=${JSON.stringify(r.sandbox).slice(0, 120)} priorTurns=${(r.thread.turns ?? []).length} instructions=${JSON.stringify(r.instructionSources)}`);

  if (opt['kill-server-after-ms']) setTimeout(() => { say('SIGTERM app-server mid-run'); rec('server_sigterm', {}); child.kill('SIGTERM'); }, Number(opt['kill-server-after-ms']));
  if (opt['kill-owner-after-ms']) setTimeout(() => { rec('owner_sigkill', { pid: process.pid, serverPid: child.pid }); say(`SIGKILL self pid=${process.pid}; app-server pid=${child.pid}`); logEnded = true; log.end(() => process.kill(process.pid, 'SIGKILL')); }, Number(opt['kill-owner-after-ms']));

  if (relay) {
    relay.onPrompt(text => { turns.push({ prompt: text, origin: 'relay-client' }); if (holdResolve) { const r = holdResolve; holdResolve = null; r(); } });
    relay.onControl(async kind => {
      if (kind === 'interrupt' && currentTurnId) { const ir = await request('turn/interrupt', { threadId: currentThreadId, turnId: currentTurnId }).catch(e => ({ error: String(e) })); rec('interrupt_response', { via: 'relay', response: ir }); }
      if (kind === 'close') { rec('relay_close', {}); closing = true; if (holdResolve) { const r = holdResolve; holdResolve = null; r(); } }
    });
  }
  for (let i = 0; i < turns.length || (relay && opt.hold && !closing); i++) {
    if (i >= turns.length) { say('turns exhausted; holding for relay clients'); if (relay) relay.setState({ phase: 'idle', awaiting: 'client' }); await new Promise(r => { holdResolve = r; }); if (closing || i >= turns.length) { i--; if (closing) break; continue; } }
    const t = turns[i];
    if (relay) relay.setState({ phase: 'working', turn: i + 1 });
    const tr = { i: i + 1, prompt: t.prompt, sentAt: ts() };
    summary.turns.push(tr);
    say(`send turn ${i + 1}: ${t.prompt.slice(0, 80)}`);
    const completed = waitFor(m => m.method === 'turn/completed' && m.params.threadId === currentThreadId && m.params.turn?.id === currentTurnId);
    const resp = await request('turn/start', { threadId: currentThreadId, input: [{ type: 'text', text: t.prompt, text_elements: [] }], ...(opt['turn-json'] ? JSON.parse(opt['turn-json']) : {}) });
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
    if (relay) relay.setState({ phase: 'idle', lastTurn: tr.status });
  }
  if (opt['stale-after-interrupt'] && opt['interrupt-after-ms']) { /* stale for interrupted turn approvals handled in handler */ }
  if (opt['linger-ms']) { say(`lingering ${opt['linger-ms']} ms before shutdown`); await new Promise(r => setTimeout(r, Number(opt['linger-ms']))); }
  say('all turns done; shutting down');
  await shutdown();
} catch (e) {
  rec('probe_error', { error: String(e), rpc: e?.rpc });
  say(`probe error: ${e}`);
  summary.error = String(e);
  try { await shutdown(); } catch {}
} finally { clearTimeout(hard); }
if (relay) relay.close();
rec('summary', summary); logEnded = true; log.end();
console.log(JSON.stringify(summary, null, 2));
process.exit(0);

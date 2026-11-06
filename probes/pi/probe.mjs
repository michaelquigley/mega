#!/usr/bin/env node
// Roundhouse pi probe: drive `pi --mode rpc` over stdio JSONL with a persisted
// session, run scripted turns, answer extension UI dialogs by policy, and log
// every frame. Plain Node, no dependencies.
//
// Usage:
//   node probe.mjs run --cwd DIR --turns scenarios/x.json --log logs/x.jsonl \
//        [--session-id UUID | --session PATH_OR_ID] [--approve-project] [--model M] \
//        [--answer REGEX] [--answer-delay-ms N] [--interrupt-on-dialog] \
//        [--stale-after-cancel] [--never-answer] [--interrupt-after-ms N] \
//        [--kill-owner-after-ms N] [--timeout-ms N] [--turn-timeout-ms N]
//        [--relay SOCK [--hold]]  owner mode: dialogs wait for a relay client's answer
//                                ({value}|{confirmed}|{cancelled:true}); clients may send
//                                prompts/interrupt/close; --hold keeps the owner alive after
//                                the turns file until a client sends close
//        [--no-prompt]            with --hold: open the session and wait without sending turns
//   node probe.mjs messages --cwd DIR --session PATH_OR_ID [--approve-project]
//
// Turns file: JSON array of prompt strings or {"prompt": "..."} objects.
// "prompt accepted" = the command response (disposition); "run settled" =
// the agent_settled event (per the installed docs/rpc.md).

import { spawn } from 'node:child_process';
import { createWriteStream, readFileSync } from 'node:fs';
import { resolve } from 'node:path';
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
const cwd = resolve(opt.cwd ?? '.');
const logPath = opt.log ?? `logs/run-${Date.now()}.jsonl`;
const log = createWriteStream(logPath, { flags: 'a' });
let logOpen = true;
const rec = (kind, data) => { if (logOpen) log.write(JSON.stringify({ t: ts(), kind, ...data }) + '\n'); };
const say = s => process.stderr.write(`[probe ${ts()}] ${s}\n`);

const relay = opt.relay ? createRelay({ socketPath: opt.relay, log: (k, d) => rec(k, d) }) : null;
const args = ['--mode', 'rpc'];
if (opt['approve-project']) args.push('--approve');
if (opt['session-id']) args.push('--session-id', opt['session-id']);
if (opt.session) args.push('--session', opt.session);
if (opt.model) args.push('--model', opt.model);
rec('probe_start', { cwd, args, pid: process.pid });
const child = spawn('pi', args, { cwd, stdio: ['pipe', 'pipe', 'pipe'] });
rec('child_spawned', { childPid: child.pid });
say(`spawned pi pid=${child.pid} args=${args.join(' ')}`);
child.stderr.on('data', d => rec('stderr', { data: d.toString().slice(0, 2000) }));

// ---- framing: split on LF bytes only, honor backpressure by using data events (no pause)
let buf = Buffer.alloc(0);
const pending = new Map();      // id -> resolve(response)
const waiters = [];             // [{ test(frame) -> bool, resolve }]
const handlers = [];            // frame listeners
child.stdout.on('data', chunk => {
  buf = Buffer.concat([buf, chunk]);
  let idx;
  while ((idx = buf.indexOf(0x0a)) !== -1) {
    let line = buf.subarray(0, idx); buf = buf.subarray(idx + 1);
    if (line.length && line[line.length - 1] === 0x0d) line = line.subarray(0, -1);
    if (!line.length) continue;
    let frame; try { frame = JSON.parse(line.toString('utf8')); } catch (e) { rec('unparseable', { line: line.toString().slice(0, 300) }); continue; }
    rec('frame', { frame: shrink(frame) });
    if (relay) relay.event('msg', shrink(frame));
    if (frame.type === 'response' && frame.id && pending.has(frame.id)) { pending.get(frame.id)(frame); pending.delete(frame.id); }
    for (const h of handlers) h(frame);
    for (let i = waiters.length - 1; i >= 0; i--) if (waiters[i].test(frame)) { waiters[i].resolve(frame); waiters.splice(i, 1); }
  }
});
function shrink(f) { // keep logs readable: trim deltas
  if (f.type === 'message_update' || f.type === 'text_delta') return { type: f.type, trimmed: true };
  return f;
}
let exited = null;
const exitP = new Promise(r => child.on('exit', (code, sig) => { exited = { code, sig }; rec('child_exit', exited); say(`pi exited code=${code} sig=${sig}`); r(exited); }));

let seq = 0;
function send(obj) {
  const line = JSON.stringify(obj) + '\n';
  rec('send', { obj });
  return new Promise(r => { if (!child.stdin.write(line)) child.stdin.once('drain', r); else r(); });
}
async function request(obj, timeoutMs = 60000) {
  const id = `p${++seq}`; obj.id = id;
  const p = new Promise(r => pending.set(id, r));
  await send(obj);
  return Promise.race([p, new Promise((_, rej) => setTimeout(() => rej(new Error(`timeout waiting response for ${obj.type}`)), timeoutMs).unref())]);
}
const waitFor = (test, timeoutMs) => Promise.race([
  new Promise(r => waiters.push({ test, resolve: r })),
  new Promise((_, rej) => setTimeout(() => rej(new Error('timeout waiting for event')), timeoutMs).unref()),
]);

// ---- extension UI dialog policy
const answerRe = opt.answer ? new RegExp(opt.answer) : null;
const summary = { sessionId: null, sessionFile: null, model: null, dialogs: [], turns: [], events: {} };
handlers.push(f => { if (f.type && f.type !== 'response') summary.events[f.type] = (summary.events[f.type] ?? 0) + 1; });
handlers.push(async f => {
  if (f.type !== 'extension_ui_request') return;
  const dialog = ['select', 'confirm', 'input', 'editor'].includes(f.method);
  say(`extension_ui_request id=${f.id} method=${f.method} title=${JSON.stringify(f.title ?? f.message ?? '')}`);
  const d = { id: f.id, method: f.method, title: f.title, options: f.options, at: ts() };
  summary.dialogs.push(d);
  if (!dialog) return;
  if (relay) {
    d.policy = 'relay';
    say(`relay: waiting for a client to answer dialog ${f.id}${f.timeout ? ` (timeout ${f.timeout} ms)` : ''}`);
    if (f.timeout) setTimeout(() => { if (relay.resolve(f.id, 'timeout_by_harness')) { rec('relay_dialog_timed_out', { dialogId: f.id }); say(`dialog ${f.id} timed out on the agent side`); } }, f.timeout + 200);
    const r = await relay.request(f.id, 'dialog', { method: f.method, title: f.title, message: f.message, options: f.options, timeout: f.timeout });
    rec('relay_request_outcome', { dialogId: f.id, ...r });
    if (!r.answered) { d.outcome = r.outcome; return; }
    const res = r.result ?? {};
    let resp;
    if (res.cancelled) resp = { type: 'extension_ui_response', id: f.id, cancelled: true };
    else if (f.method === 'confirm') resp = { type: 'extension_ui_response', id: f.id, confirmed: !!res.confirmed };
    else resp = { type: 'extension_ui_response', id: f.id, value: res.value };
    d.answer = resp; d.answeredAt = ts();
    await send(resp); say(`relay answer forwarded: ${JSON.stringify(res)}`);
    return;
  }
  if (opt['never-answer']) { d.policy = 'never-answer'; return; }
  if (opt['interrupt-on-dialog']) {
    say('abort while dialog pending');
    const r = await request({ type: 'abort' }, 30000).catch(e => ({ error: String(e) }));
    d.abortResponse = r; rec('abort_during_dialog', { dialogId: f.id, response: r });
    if (opt['stale-after-cancel']) {
      await new Promise(r => setTimeout(r, 1500));
      const resp = f.method === 'confirm' ? { type: 'extension_ui_response', id: f.id, confirmed: true } : { type: 'extension_ui_response', id: f.id, value: (f.options ?? ['x'])[0] };
      await send(resp); d.stale = resp; rec('stale_dialog_response_sent', { dialogId: f.id });
      say('sent STALE dialog response after abort');
    }
    return;
  }
  const delay = Number(opt['answer-delay-ms'] ?? 0);
  if (delay) await new Promise(r => setTimeout(r, delay));
  let resp;
  if (f.method === 'confirm') resp = { type: 'extension_ui_response', id: f.id, confirmed: true };
  else if (f.method === 'select') { const pick = (answerRe && (f.options ?? []).find(o => answerRe.test(o))) ?? (f.options ?? [])[0]; resp = { type: 'extension_ui_response', id: f.id, value: pick }; }
  else resp = { type: 'extension_ui_response', id: f.id, value: 'probe-text' };
  d.answer = resp; d.answeredAt = ts();
  await send(resp); say(`answered ${f.method} with ${JSON.stringify(resp.value ?? resp.confirmed)}`);
});

const hardTimeout = setTimeout(async () => { say('hard timeout'); rec('hard_timeout', {}); child.kill('SIGTERM'); }, Number(opt['timeout-ms'] ?? 600000));

async function state(tag) {
  const r = await request({ type: 'get_state' });
  const d = r.data ?? {};
  rec('state', { tag, data: { sessionId: d.sessionId, sessionFile: d.sessionFile, model: d.model && { provider: d.model.provider, id: d.model.id }, isStreaming: d.isStreaming, messageCount: d.messageCount, sessionName: d.sessionName } });
  return d;
}

if (cmd === 'messages') {
  const d = await state('messages');
  const r = await request({ type: 'get_messages' });
  for (const m of r.data?.messages ?? []) {
    const c = m.content; const text = typeof c === 'string' ? c : (c ?? []).map(b => b.type === 'text' ? b.text : b.type === 'toolCall' ? `[toolCall ${b.name} ${JSON.stringify(b.arguments ?? b.input ?? {}).slice(0, 100)}]` : `[${b.type}]`).join(' | ');
    console.log(`${m.role}\t${(m.toolName ? '[' + m.toolName + '] ' : '') + String(text).replace(/\n/g, ' ').slice(0, 220)}`);
  }
  console.error(`sessionId=${d.sessionId} file=${d.sessionFile} messages=${(r.data?.messages ?? []).length}`);
  child.stdin.end(); await exitP; clearTimeout(hardTimeout); logOpen = false; log.end(); process.exit(0);
}
if (cmd !== 'run') { console.error('unknown command'); process.exit(2); }

const turns = opt.turns ? JSON.parse(readFileSync(opt.turns, 'utf8')).map(t => typeof t === 'string' ? { prompt: t } : t) : [];
let closeRequested = false; let wakeHold = null; let inTurn = false;
if (relay) {
  relay.onPrompt(text => {
    if (text.startsWith('RAW:')) { const frame = JSON.parse(text.slice(4)); rec('relay_raw_frame', { frame }); say(`relay raw frame -> pi: ${JSON.stringify(frame).slice(0, 160)}`); send(frame); return; }
    turns.push({ prompt: text, origin: 'relay-client' }); if (wakeHold) { const w = wakeHold; wakeHold = null; w(); }
  });
  relay.onControl(async kind => {
    if (kind === 'interrupt') { rec('relay_abort', {}); const r = await request({ type: 'abort' }, 60000).catch(e => ({ error: String(e) })); rec('abort_response', { via: 'relay', response: r }); say(`relay abort -> ${JSON.stringify(r).slice(0, 120)}`); }
    if (kind === 'close') {
      closeRequested = true; rec('relay_close', { inTurn }); say(`relay client asked to close (inTurn=${inTurn})`);
      if (inTurn) { say('closing stdin while a turn is in flight'); rec('stdin_closed_during_turn', {}); child.stdin.end(); }
      if (wakeHold) { const w = wakeHold; wakeHold = null; w(); }
    }
  });
}
try {
  const s0 = await state('start');
  summary.sessionId = s0.sessionId; summary.sessionFile = s0.sessionFile; summary.model = s0.model && `${s0.model.provider}/${s0.model.id}`;
  say(`session ${s0.sessionId} file=${s0.sessionFile} model=${summary.model} messages=${s0.messageCount}`);
  for (let i = 0; i < turns.length || (relay && opt.hold && !closeRequested); i++) {
    if (i >= turns.length) { relay.setState({ phase: 'idle', awaiting: 'client' }); say('holding for relay clients (prompt or close)'); await new Promise(r => { wakeHold = r; }); if (closeRequested || i >= turns.length) { i--; if (closeRequested) break; continue; } }
    const t = turns[i]; const rec_t = { i: i + 1, prompt: t.prompt, sentAt: ts() };
    if (relay) relay.setState({ phase: 'working', turn: i + 1 });
    summary.turns.push(rec_t);
    say(`send turn ${i + 1}: ${t.prompt.slice(0, 80)}`);
    const settled = waitFor(f => f.type === 'agent_settled', Number(opt['turn-timeout-ms'] ?? 300000));
    if (opt['interrupt-after-ms'] && i + 1 === Number(opt['interrupt-turn'] ?? 1)) {
      setTimeout(async () => { say('abort active turn'); rec('abort_sent', { turn: i + 1 }); const r = await request({ type: 'abort' }, 60000).catch(e => ({ error: String(e) })); rec('abort_response', { response: r }); say(`abort response: ${JSON.stringify(r)}`); }, Number(opt['interrupt-after-ms']));
    }
    if (opt['kill-owner-after-ms'] && i === 0) {
      setTimeout(() => { rec('owner_sigkill', { pid: process.pid, childPid: child.pid }); say(`SIGKILL self pid=${process.pid}; pi child pid=${child.pid}`); logOpen = false; log.end(() => process.kill(process.pid, 'SIGKILL')); }, Number(opt['kill-owner-after-ms']));
    }
    const resp = await request({ type: 'prompt', message: t.prompt }, 60000);
    rec_t.accepted = { success: resp.success, disposition: resp.data?.disposition, error: resp.error, at: ts() };
    say(`prompt response success=${resp.success} disposition=${resp.data?.disposition ?? ''} ${resp.error ?? ''}`);
    if (!resp.success || resp.data?.disposition === 'handled') { settled.catch(() => {}); continue; }
    inTurn = true;
    const ev = await Promise.race([settled, exitP.then(e => ({ type: 'child_exit', ...e }))]);
    inTurn = false;
    if (ev.type === 'child_exit') { rec_t.childExitDuringTurn = ev; say(`pi exited during the turn (code=${ev.code}); no agent_settled`); break; }
    rec_t.settled = { aborted: ev.aborted, at: ts() };
    const last = await request({ type: 'get_last_assistant_text' }).catch(() => null);
    rec_t.lastAssistantText = last?.data?.text ?? last?.data ?? null;
    say(`settled aborted=${ev.aborted}; last assistant text: ${String(rec_t.lastAssistantText ?? '').replace(/\n/g, ' / ').slice(0, 200)}`);
    if (relay) relay.setState({ phase: 'idle', lastSettled: { aborted: ev.aborted } });
  }
  summary.end = await state('end');
} catch (e) {
  rec('probe_error', { error: String(e) }); say(`probe error: ${e}`);
}
say('closing stdin for orderly shutdown');
if (!child.stdin.writableEnded) child.stdin.end();
const ex = await Promise.race([exitP, new Promise(r => setTimeout(() => r('no-exit-after-20s'), 20000))]);
if (ex === 'no-exit-after-20s') { rec('no_exit', {}); say('pi did not exit within 20s of stdin close; SIGTERM'); child.kill('SIGTERM'); await exitP; }
clearTimeout(hardTimeout);
if (relay) relay.close();
rec('summary', summary); logOpen = false; log.end();
console.log(JSON.stringify(summary, null, 2));

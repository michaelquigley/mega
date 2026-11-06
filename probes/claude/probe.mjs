#!/usr/bin/env node
// Roundhouse Claude probe: launch or resume a persisted Claude Code session
// through the Agent SDK, drive scripted turns over streaming input, answer
// permission requests and AskUserQuestion with a declared policy, and log
// every SDK message to a JSONL file.
//
// Usage:
//   node probe.mjs run --cwd DIR --turns scenarios/x.json --log logs/x.jsonl \
//        [--session-id UUID | --resume UUID] [--permission-mode default|auto|...] \
//        [--approve REGEX] [--approve-delay-ms N] [--answer REGEX] \
//        [--interrupt-on-permission] [--stale-after-abort] \
//        [--interrupt-after-ms N [--interrupt-turn I]] [--kill-owner-after-ms N] \
//        [--setting-sources user,project,local] [--exec PATH]
//        [--relay SOCK [--hold]]   owner mode: requests wait for a relay client's answer;
//                                 clients may send prompts; with --hold the owner keeps the
//                                 session open after the turns file until a client sends close
//   node probe.mjs list --cwd DIR
//   node probe.mjs info --session SESSION_ID [--cwd DIR]
//   node probe.mjs messages --session SESSION_ID [--cwd DIR]
//
// Turns file: JSON array; each item is a prompt string or {"prompt": "..."}.
// Policy: a tool request whose "ToolName:<json input>" matches --approve is
// allowed (after --approve-delay-ms); anything else is denied with a message.
// AskUserQuestion is answered by choosing the first option whose label matches
// --answer (default: first option).

import { createWriteStream, readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import {
  query, listSessions, getSessionInfo, getSessionMessages,
} from '@anthropic-ai/claude-agent-sdk';
import { createRelay } from '../common/relay.mjs';

const argv = process.argv.slice(2);
const cmd = argv.shift();
const opt = {};
for (let i = 0; i < argv.length; i++) {
  const a = argv[i];
  if (!a.startsWith('--')) continue;
  const k = a.slice(2);
  const next = argv[i + 1];
  if (next === undefined || next.startsWith('--')) opt[k] = true; else { opt[k] = next; i++; }
}

const DEFAULT_EXEC = '/home/michael/.local/share/claude/versions/2.1.318';

function ts() { return new Date().toISOString(); }

if (cmd === 'list') {
  const s = await listSessions({ dir: opt.cwd ? resolve(opt.cwd) : undefined, limit: Number(opt.limit ?? 20) });
  console.log(JSON.stringify(s, null, 2));
  process.exit(0);
}
if (cmd === 'info') {
  console.log(JSON.stringify(await getSessionInfo(opt.session, { dir: opt.cwd ? resolve(opt.cwd) : undefined }), null, 2));
  process.exit(0);
}
if (cmd === 'messages') {
  const m = await getSessionMessages(opt.session, { dir: opt.cwd ? resolve(opt.cwd) : undefined, includeSystemMessages: true });
  for (const x of m) {
    const c = x.message?.content;
    const text = typeof c === 'string' ? c : (c ?? []).map(b => b.type === 'text' ? b.text : b.type === 'tool_use' ? `[tool_use ${b.name} ${JSON.stringify(b.input).slice(0, 120)}]` : b.type === 'tool_result' ? `[tool_result ${String(typeof b.content === 'string' ? b.content : JSON.stringify(b.content)).slice(0, 120)}]` : `[${b.type}]`).join(' | ');
    console.log(`${x.type}\t${x.uuid ?? ''}\t${text.slice(0, 300)}`);
  }
  process.exit(0);
}
if (cmd !== 'run') { console.error('unknown command'); process.exit(2); }

const cwd = resolve(opt.cwd ?? '.');
const turns = JSON.parse(readFileSync(opt.turns, 'utf8')).map(t => typeof t === 'string' ? { prompt: t } : t);
const log = createWriteStream(opt.log ?? `logs/run-${Date.now()}.jsonl`, { flags: 'a' });
const relay = opt.relay ? createRelay({ socketPath: opt.relay, log: (k, d) => rec(k, d) }) : null;
const approveRe = opt.approve ? new RegExp(opt.approve) : null;
const answerRe = opt.answer ? new RegExp(opt.answer) : null;
const approveDelay = Number(opt['approve-delay-ms'] ?? 0);

let logEnded = false;
function rec(kind, data) {
  const line = JSON.stringify({ t: ts(), kind, ...data });
  if (!logEnded && log.writable) log.write(line + '\n');
  return line;
}
function say(s) { process.stderr.write(`[probe ${ts()}] ${s}\n`); }

// Streaming input: a push queue the generator drains; closing it ends the session.
let pushResolve = null; const queue = []; let closed = false;
function push(msg) { queue.push(msg); if (pushResolve) { const r = pushResolve; pushResolve = null; r(); } }
function close() { closed = true; if (pushResolve) { const r = pushResolve; pushResolve = null; r(); } }
async function* input() {
  while (true) {
    if (queue.length) { yield queue.shift(); continue; }
    if (closed) return;
    await new Promise(r => { pushResolve = r; });
  }
}

const abort = new AbortController();
let q;
const summary = { session_id: null, init: null, requests: [], results: [], state_changes: [], turns: [] };

const canUseTool = async (toolName, input, o) => {
  const started = Date.now();
  const key = `${toolName}:${JSON.stringify(input)}`;
  const base = { toolName, toolUseID: o.toolUseID, requestId: o.requestId, decisionReason: o.decisionReason, blockedPath: o.blockedPath, title: o.title, suggestions: o.suggestions, input };
  say(`can_use_tool ${toolName} requestId=${o.requestId} toolUseID=${o.toolUseID} reason=${JSON.stringify(o.decisionReason ?? null)}`);
  rec('permission_request', base);
  const aborted = new Promise(r => o.signal.addEventListener('abort', () => r('aborted'), { once: true }));
  if (relay) {
    const kind = toolName === 'AskUserQuestion' ? 'question' : 'permission';
    aborted.then(() => { if (relay.resolve(o.requestId, 'aborted')) rec('relay_request_aborted', { requestId: o.requestId }); });
    say(`relay: waiting for a client to answer ${kind} ${o.requestId}`);
    const r = await relay.request(o.requestId, kind, { toolName, input, toolUseID: o.toolUseID, title: o.title, decisionReason: o.decisionReason });
    rec('relay_request_outcome', { requestId: o.requestId, ...r });
    if (!r.answered) return { behavior: 'deny', message: `probe: request ${r.outcome} before any client answered`, toolUseID: o.toolUseID };
    if (kind === 'question') return { behavior: 'allow', updatedInput: { ...input, answers: r.result.answers ?? {} }, toolUseID: o.toolUseID };
    if (r.result.decision === 'allow') return { behavior: 'allow', updatedInput: input, toolUseID: o.toolUseID };
    return { behavior: 'deny', message: r.result.message ?? 'denied by relay client', toolUseID: o.toolUseID };
  }

  if (toolName === 'AskUserQuestion') {
    const answers = {};
    for (const qn of input.questions ?? []) {
      const opts = qn.options ?? [];
      const pick = (answerRe && opts.find(x => answerRe.test(x.label))) ?? opts[0];
      answers[qn.question] = pick ? pick.label : 'no option';
    }
    rec('question_answer', { requestId: o.requestId, answers });
    say(`answering AskUserQuestion with ${JSON.stringify(answers)}`);
    return { behavior: 'allow', updatedInput: { ...input, answers }, toolUseID: o.toolUseID };
  }

  if (opt['interrupt-on-permission'] && q) {
    say('interrupting while permission request pending');
    const ir = await q.interrupt().catch(e => ({ error: String(e) }));
    rec('interrupt_during_permission', { requestId: o.requestId, interruptResponse: ir });
    const outcome = await Promise.race([aborted, new Promise(r => setTimeout(() => r('timeout'), 5000))]);
    rec('permission_signal_after_interrupt', { requestId: o.requestId, outcome, waitedMs: Date.now() - started });
    say(`permission signal after interrupt: ${outcome}`);
    if (opt['stale-after-abort']) {
      await new Promise(r => setTimeout(r, 1500));
      rec('stale_response_sent', { requestId: o.requestId, behavior: 'allow', note: 'allow returned after the request signal aborted' });
      say('sending STALE allow for aborted request');
      return { behavior: 'allow', updatedInput: input, toolUseID: o.toolUseID };
    }
    return { behavior: 'deny', message: 'probe: denied after interrupt', toolUseID: o.toolUseID };
  }

  if (approveRe && approveRe.test(key)) {
    if (approveDelay) {
      const outcome = await Promise.race([aborted, new Promise(r => setTimeout(() => r('delay_elapsed'), approveDelay))]);
      rec('permission_wait', { requestId: o.requestId, outcome, waitedMs: Date.now() - started });
      if (outcome === 'aborted') return { behavior: 'deny', message: 'probe: request aborted while waiting', toolUseID: o.toolUseID };
    }
    rec('permission_decision', { requestId: o.requestId, behavior: 'allow' });
    say(`allow ${toolName}`);
    return { behavior: 'allow', updatedInput: input, toolUseID: o.toolUseID };
  }
  rec('permission_decision', { requestId: o.requestId, behavior: 'deny' });
  say(`deny ${toolName}`);
  return { behavior: 'deny', message: 'probe policy: not approved', toolUseID: o.toolUseID };
};

const options = {
  cwd,
  pathToClaudeCodeExecutable: opt.exec ?? DEFAULT_EXEC,
  abortController: abort,
  canUseTool,
  stderr: d => rec('stderr', { data: d.slice(0, 2000) }),
  includePartialMessages: false,
};
if (opt['permission-mode']) options.permissionMode = opt['permission-mode'];
if (opt['session-id']) options.sessionId = opt['session-id'];
if (opt.resume) options.resume = opt.resume;
if (opt['setting-sources'] !== undefined) options.settingSources = opt['setting-sources'] === '' ? [] : String(opt['setting-sources']).split(',');
if (opt.model) options.model = opt.model;
rec('probe_start', { cwd, options: { ...options, canUseTool: undefined, stderr: undefined, abortController: undefined } });

let turnIdx = 0;
let holding = false;
function sendNext() {
  if (turnIdx >= turns.length) {
    if (relay && opt.hold) { if (!holding) { holding = true; say('all turns done; holding for relay clients (send close to end)'); relay.setState({ phase: 'idle', awaiting: 'client' }); } return; }
    say('all turns done; closing input'); close(); return;
  }
  const t = turns[turnIdx++];
  summary.turns.push({ i: turnIdx, prompt: t.prompt, sent_at: ts() });
  rec('send_turn', { i: turnIdx, prompt: t.prompt });
  say(`send turn ${turnIdx}: ${t.prompt.slice(0, 80)}`);
  if (relay) relay.setState({ phase: 'working', turn: turnIdx });
  push({ type: 'user', message: { role: 'user', content: t.prompt }, parent_tool_use_id: null });
  const iTurn = Number(opt['interrupt-turn'] ?? 1);
  if (opt['interrupt-after-ms'] && turnIdx === iTurn) {
    setTimeout(async () => {
      say(`interrupting active turn ${turnIdx}`);
      rec('interrupt_sent', { turn: turnIdx });
      const ir = await q.interrupt().catch(e => ({ error: String(e) }));
      rec('interrupt_response', { turn: turnIdx, response: ir });
      say(`interrupt response: ${JSON.stringify(ir)}`);
    }, Number(opt['interrupt-after-ms']));
  }
  if (opt['kill-owner-after-ms'] && turnIdx === 1) {
    setTimeout(() => {
      rec('owner_sigkill', { pid: process.pid });
      say(`SIGKILL self (pid ${process.pid}); CLI child is orphaned without stdin close`);
      log.end(() => process.kill(process.pid, 'SIGKILL'));
    }, Number(opt['kill-owner-after-ms']));
  }
}

if (relay) {
  relay.onPrompt(text => { turns.push({ prompt: text, origin: 'relay-client' }); if (holding) { holding = false; sendNext(); } });
  relay.onControl(async kind => {
    if (kind === 'interrupt') { rec('relay_interrupt', {}); const ir = await q.interrupt().catch(e => ({ error: String(e) })); rec('interrupt_response', { via: 'relay', response: ir }); }
    if (kind === 'close') { say('relay client asked to close; closing input'); rec('relay_close', {}); close(); }
  });
}
q = query({ prompt: input(), options });
sendNext();
const hardTimeout = setTimeout(() => { say('hard timeout; aborting'); rec('hard_timeout', {}); abort.abort(); }, Number(opt['timeout-ms'] ?? 600000));

try {
  for await (const m of q) {
    rec('msg', { msg: m });
    if (relay) relay.event('msg', m);
    if (m.type === 'system' && m.subtype === 'init') {
      summary.session_id = m.session_id;
      summary.init = { cwd: m.cwd, model: m.model, permissionMode: m.permissionMode, claude_code_version: m.claude_code_version, apiKeySource: m.apiKeySource, tools: m.tools, mcp_servers: m.mcp_servers, slash_commands: m.slash_commands, skills: m.skills, plugins: m.plugins?.map(p => p.name), output_style: m.output_style, agents: m.agents };
      say(`init session_id=${m.session_id} model=${m.model} permissionMode=${m.permissionMode} cwd=${m.cwd} tools=${m.tools.length} mcp=${m.mcp_servers.length}`);
    } else if (m.type === 'system' && m.subtype === 'session_state_changed') {
      summary.state_changes.push({ t: ts(), state: m.state });
      say(`state -> ${m.state}`);
    } else if (m.type === 'assistant') {
      for (const b of m.message.content) {
        if (b.type === 'text') say(`assistant text: ${b.text.slice(0, 200)}`);
        else if (b.type === 'tool_use') say(`assistant tool_use ${b.name} id=${b.id} ${JSON.stringify(b.input).slice(0, 160)}`);
      }
    } else if (m.type === 'user' && Array.isArray(m.message.content)) {
      for (const b of m.message.content) if (b.type === 'tool_result') say(`tool_result for ${b.tool_use_id}: ${String(typeof b.content === 'string' ? b.content : JSON.stringify(b.content)).slice(0, 160)}`);
    } else if (m.type === 'result') {
      summary.results.push({ subtype: m.subtype, is_error: m.is_error, num_turns: m.num_turns, duration_ms: m.duration_ms, session_id: m.session_id, result: m.subtype === 'success' ? m.result : undefined, errors: m.errors });
      say(`result ${m.subtype} is_error=${m.is_error} num_turns=${m.num_turns} session=${m.session_id}`);
      if (relay) relay.setState({ phase: 'idle', lastResult: m.subtype });
      const t = turns[turnIdx - 1];
      if (t?.after === 'interrupt') { /* handled elsewhere */ }
      sendNext();
    }
  }
  rec('stream_end', {});
  say('stream ended');
} catch (e) {
  rec('stream_error', { error: String(e), stack: e?.stack });
  say(`stream error: ${e}`);
} finally {
  clearTimeout(hardTimeout);
}
if (relay) relay.close();
rec('summary', summary);
logEnded = true;
log.end();
console.log(JSON.stringify(summary, null, 2));

#!/usr/bin/env node
// roundhouse claude shim: a policy-free bridge between the roundhouse service
// and the claude agent sdk, speaking one JSON object per line on stdio.
//
//   node shim.mjs run --cwd DIR (--session-id UUID | --resume UUID) --exec PATH [--permission-mode MODE]
//   node shim.mjs messages --session UUID --cwd DIR
//   node shim.mjs list --cwd DIR
//
// run, shim to service:
//   {"type":"event","msg":<sdk message verbatim>}
//   {"type":"request","id":<requestId>,"kind":"permission"|"question","tool":...,"input":{...},...}
//   {"type":"request_resolved","id":...,"outcome":"aborted"}
//   {"type":"answer_accepted","id":...}     the answer reached the waiting request
//   {"type":"answer_rejected","id":...}     no request with that id was waiting
//   {"type":"interrupted","response":{...}}
//   {"type":"permission_mode_set","id":...,"mode":...}            the sdk switched
//   {"type":"permission_mode_set","id":...,"mode":...,"error":...}  it did not
//   {"type":"error","message":...}
//   {"type":"exit","code":N}
// run, service to shim:
//   {"type":"prompt","uuid":...,"text":...}
//   {"type":"answer","id":...,"decision":"allow"|"deny","message":...}
//   {"type":"answer","id":...,"answers":{"<question>":"<label>"}}
//   {"type":"interrupt"}
//   {"type":"set_permission_mode","id":...,"mode":...}
//   {"type":"close"}
//
// the shim decides nothing: every request waits for the service's answer, the
// permission mode is whatever the service passes (the operator's choice), and
// with no --permission-mode the option is left out so claude's own settings
// decide. setting sources stay the default, so approvals reach the operator
// and project settings load as they would in a terminal.

import { createInterface } from 'node:readline';
import { pathToFileURL } from 'node:url';

export function parseArgs(argv) {
  const opt = {};
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (!a.startsWith('--')) continue;
    const next = argv[i + 1];
    if (next === undefined || next.startsWith('--')) opt[a.slice(2)] = true;
    else { opt[a.slice(2)] = next; i++; }
  }
  return opt;
}

// writer emits one JSON object per line.
function writer(stdout) {
  return obj => stdout.write(JSON.stringify(obj) + '\n');
}

// input is the streaming prompt source the sdk drains; ending it ends the session
// once the cli is idle.
function inputQueue() {
  const queue = [];
  let wake = null;
  let ended = false;
  const poke = () => { if (wake) { const w = wake; wake = null; w(); } };
  return {
    push(msg) { queue.push(msg); poke(); },
    end() { ended = true; poke(); },
    async *iterate() {
      while (true) {
        if (queue.length) { yield queue.shift(); continue; }
        if (ended) return;
        await new Promise(r => { wake = r; });
      }
    },
  };
}

export async function run(opt, sdk, io) {
  const emit = writer(io.stdout);
  const input = inputQueue();
  const pending = new Map(); // requestId -> resolve(answer)
  let q = null;
  let interrupting = Promise.resolve();

  const canUseTool = (toolName, toolInput, o) => new Promise(resolve => {
    const id = o.requestId;
    const kind = toolName === 'AskUserQuestion' ? 'question' : 'permission';
    const onAbort = () => {
      if (!pending.delete(id)) return;
      emit({ type: 'request_resolved', id, outcome: 'aborted' });
      resolve({ behavior: 'deny', message: 'the request was aborted', toolUseID: o.toolUseID });
    };
    pending.set(id, answer => {
      o.signal.removeEventListener('abort', onAbort);
      if (kind === 'question') {
        resolve({ behavior: 'allow', updatedInput: { ...toolInput, answers: answer.answers ?? {} }, toolUseID: o.toolUseID });
      } else if (answer.decision === 'allow') {
        resolve({ behavior: 'allow', updatedInput: toolInput, toolUseID: o.toolUseID });
      } else {
        resolve({ behavior: 'deny', message: answer.message ?? '', toolUseID: o.toolUseID });
      }
    });
    o.signal.addEventListener('abort', onAbort, { once: true });
    emit({
      type: 'request', id, kind, tool: toolName, input: toolInput,
      toolUseId: o.toolUseID, title: o.title, displayName: o.displayName, description: o.description,
      decisionReason: o.decisionReason, blockedPath: o.blockedPath, suggestions: o.suggestions,
    });
    if (o.signal.aborted) onAbort();
  });

  const options = {
    cwd: opt.cwd,
    pathToClaudeCodeExecutable: opt.exec,
    canUseTool,
    includePartialMessages: true,
    stderr: data => io.stderr.write(data),
  };
  if (typeof opt['permission-mode'] === 'string') options.permissionMode = opt['permission-mode'];
  if (opt['session-id']) options.sessionId = opt['session-id'];
  if (opt.resume) options.resume = opt.resume;

  // close waits for any interrupt in flight: closing input while a request is
  // still pending keeps the cli alive for minutes (capability report, 2.1.318).
  const close = () => { interrupting.finally(() => input.end()); };

  const lines = createInterface({ input: io.stdin, crlfDelay: Infinity });
  lines.on('line', line => {
    if (!line.trim()) return;
    let m;
    try { m = JSON.parse(line); } catch { emit({ type: 'error', message: 'unreadable input line' }); return; }
    if (m.type === 'prompt') {
      input.push({ type: 'user', message: { role: 'user', content: m.text }, parent_tool_use_id: null, uuid: m.uuid });
    } else if (m.type === 'answer') {
      const deliver = pending.get(m.id);
      if (!deliver) { emit({ type: 'answer_rejected', id: m.id }); return; }
      pending.delete(m.id);
      deliver(m);
      emit({ type: 'answer_accepted', id: m.id });
    } else if (m.type === 'interrupt') {
      if (!q) return;
      interrupting = q.interrupt()
        .then(response => emit({ type: 'interrupted', response: response ?? {} }))
        .catch(e => emit({ type: 'error', message: `interrupt: ${e?.message ?? e}` }));
    } else if (m.type === 'set_permission_mode') {
      // takes effect from the next tool call; a request already pending stays
      // pending (capability report, 2.1.318).
      const reply = { type: 'permission_mode_set', id: m.id, mode: m.mode };
      if (!q) { emit({ ...reply, error: 'no session yet' }); return; }
      q.setPermissionMode(m.mode)
        .then(() => emit(reply))
        .catch(e => emit({ ...reply, error: String(e?.message ?? e) }));
    } else if (m.type === 'close') {
      close();
    }
  });
  lines.on('close', close);

  let code = 0;
  try {
    q = sdk.query({ prompt: input.iterate(), options });
    for await (const msg of q) emit({ type: 'event', msg });
  } catch (e) {
    emit({ type: 'error', message: String(e?.message ?? e) });
    code = 1;
  }
  lines.close();
  emit({ type: 'exit', code });
  return code;
}

export async function messages(opt, sdk, io) {
  const emit = writer(io.stdout);
  const list = await sdk.getSessionMessages(opt.session, { dir: opt.cwd });
  for (const m of list) emit(m);
  return 0;
}

export async function list(opt, sdk, io) {
  const emit = writer(io.stdout);
  const sessions = await sdk.listSessions({ dir: opt.cwd });
  for (const s of sessions) emit(s);
  return 0;
}

export async function main(argv, sdk, io) {
  const cmd = argv[0];
  const opt = parseArgs(argv.slice(1));
  const required = { run: ['cwd', 'exec'], messages: ['session', 'cwd'], list: ['cwd'] }[cmd];
  if (!required) { io.stderr.write(`unknown command '${cmd}'\n`); return 2; }
  const missing = required.filter(k => typeof opt[k] !== 'string');
  if (missing.length) { io.stderr.write(`missing --${missing.join(', --')}\n`); return 2; }
  if (cmd === 'run' && !opt['session-id'] === !opt.resume) {
    io.stderr.write('exactly one of --session-id and --resume is required\n');
    return 2;
  }
  return { run, messages, list }[cmd](opt, sdk, io);
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) {
  const sdk = await import('@anthropic-ai/claude-agent-sdk');
  const code = await main(process.argv.slice(2), sdk, process).catch(e => {
    process.stderr.write(`${e?.stack ?? e}\n`);
    return 1;
  });
  process.stdout.write('', () => process.exit(code));
}

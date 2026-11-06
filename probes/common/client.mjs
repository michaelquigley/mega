#!/usr/bin/env node
// Generic relay client ("browser-equivalent"). Connects to an owner's relay
// socket, prints every frame as JSON lines on stdout, and can answer the
// first pending request, submit a prompt, or disconnect after a delay.
//   node client.mjs --socket PATH [--since N] [--watch-ms N] [--disconnect-after-ms N]
//        [--answer-json '{"decision":"allow"}' [--answer-id ID] [--answer-delay-ms N]]
//        [--prompt TEXT] [--close]
import { connect } from 'node:net';
const argv = process.argv.slice(2); const opt = {};
for (let i = 0; i < argv.length; i++) { const a = argv[i]; if (!a.startsWith('--')) continue; const n = argv[i + 1]; if (n === undefined || n.startsWith('--')) opt[a.slice(2)] = true; else { opt[a.slice(2)] = n; i++; } }
const ts = () => new Date().toISOString();
const say = s => process.stderr.write(`[client ${ts()}] ${s}\n`);
const out = o => process.stdout.write(JSON.stringify({ t: ts(), ...o }) + '\n');
const sock = connect(opt.socket);
const send = o => sock.write(JSON.stringify(o) + '\n');
let buf = ''; let answered = false; let lastSeq = Number(opt.since ?? 0); const seen = { events: 0, requests: 0, resolved: 0 };
const pendingNow = new Map();
async function maybeAnswer() {
  if (answered || !opt['answer-json']) return;
  const target = opt['answer-id'] ? pendingNow.get(opt['answer-id']) : [...pendingNow.values()][0];
  if (!target) return;
  answered = true;
  if (opt['answer-delay-ms']) await new Promise(r => setTimeout(r, Number(opt['answer-delay-ms'])));
  say(`answering request ${target.id} (${target.kind}) with ${opt['answer-json']}`);
  send({ type: 'answer', id: target.id, result: JSON.parse(opt['answer-json']) });
}
sock.on('connect', () => { say(`connected since=${lastSeq}`); send({ type: 'hello', since: lastSeq }); if (opt.prompt) { say('sending prompt'); send({ type: 'prompt', text: opt.prompt }); } if (opt.interrupt) send({ type: 'interrupt' }); });
sock.on('data', d => {
  buf += d; let i;
  while ((i = buf.indexOf('\n')) >= 0) {
    const line = buf.slice(0, i); buf = buf.slice(i + 1); if (!line.trim()) continue;
    const m = JSON.parse(line); out(m);
    if (m.seq) lastSeq = Math.max(lastSeq, m.seq);
    if (m.type === 'snapshot') { for (const r of m.pending) pendingNow.set(r.id, r); say(`snapshot: state=${JSON.stringify(m.state)} pending=${m.pending.map(r => r.id).join(',') || 'none'} seq=${m.seq}`); maybeAnswer(); }
    else if (m.type === 'event') { seen.events++; }
    else if (m.type === 'request') { seen.requests++; pendingNow.set(m.id, m); say(`request ${m.id} ${m.kind}`); maybeAnswer(); }
    else if (m.type === 'request_resolved') { seen.resolved++; pendingNow.delete(m.id); say(`request ${m.id} resolved: ${m.outcome} by ${m.by}`); }
    else if (m.type === 'answer_rejected') { say(`answer rejected for ${m.id}: ${m.reason}`); }
  }
});
sock.on('close', () => { say(`disconnected; lastSeq=${lastSeq} ${JSON.stringify(seen)} stillPending=${[...pendingNow.keys()].join(',') || 'none'}`); process.exit(0); });
sock.on('error', e => { say(`error ${e.message}`); process.exit(1); });
if (opt['disconnect-after-ms']) setTimeout(() => { say('disconnecting (simulated client loss)'); sock.destroy(); }, Number(opt['disconnect-after-ms']));
if (opt['watch-ms']) setTimeout(() => { if (opt.close) send({ type: 'close' }); sock.end(); }, Number(opt['watch-ms']));

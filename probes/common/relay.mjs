// Tiny owner-side relay for the reconnect tests. In-memory only: events are
// kept in a ring in the owner process and replayed to clients that connect
// with `since`. Nothing here survives an owner crash, and nothing here is a
// harness guarantee; it is the shim the brief allows.
//
// Wire (JSON lines over a unix socket):
//   client -> owner : {type:'hello', since?}  {type:'answer', id, result}
//                     {type:'prompt', text}   {type:'interrupt'}  {type:'close'}
//   owner  -> client: {type:'snapshot', state, pending:[request...], seq}
//                     {type:'event', seq, t, kind, data}
//                     {type:'request', seq, t, id, kind, payload}
//                     {type:'request_resolved', seq, t, id, outcome, by}
//                     {type:'answer_rejected', id, reason}
import { createServer } from 'node:net';
import { unlinkSync, existsSync } from 'node:fs';

export function createRelay({ socketPath, log = () => {}, ringSize = 5000 }) {
  const ring = []; let seq = 0;
  const pending = new Map();      // id -> { req, resolve }
  const clients = new Set();
  let state = {}; const promptHandlers = []; const controlHandlers = [];
  const now = () => new Date().toISOString();
  const push = frame => { frame.seq = ++seq; frame.t = frame.t ?? now(); ring.push(frame); if (ring.length > ringSize) ring.shift(); for (const c of clients) write(c, frame); return frame; };
  const write = (c, obj) => { try { c.write(JSON.stringify(obj) + '\n'); } catch {} };
  if (existsSync(socketPath)) unlinkSync(socketPath);
  const server = createServer(sock => {
    let buf = '';
    sock.on('data', d => {
      buf += d; let i;
      while ((i = buf.indexOf('\n')) >= 0) {
        const line = buf.slice(0, i); buf = buf.slice(i + 1); if (!line.trim()) continue;
        let m; try { m = JSON.parse(line); } catch { continue; }
        log('relay_client_msg', { msg: m });
        if (m.type === 'hello') {
          clients.add(sock);
          write(sock, { type: 'snapshot', state, pending: [...pending.values()].map(p => p.req), seq });
          const since = Number(m.since ?? seq);
          for (const f of ring) if (f.seq > since) write(sock, f);
        } else if (m.type === 'answer') {
          const p = pending.get(m.id);
          if (!p) { write(sock, { type: 'answer_rejected', id: m.id, reason: 'not_pending' }); log('relay_answer_rejected', { id: m.id }); continue; }
          pending.delete(m.id);
          push({ type: 'request_resolved', id: m.id, outcome: 'answered', by: 'client' });
          p.resolve({ answered: true, result: m.result });
        } else if (m.type === 'prompt') { for (const h of promptHandlers) h(m.text); }
        else if (m.type === 'interrupt' || m.type === 'close') { for (const h of controlHandlers) h(m.type); }
      }
    });
    sock.on('close', () => { clients.delete(sock); log('relay_client_closed', {}); });
    sock.on('error', () => { clients.delete(sock); });
  });
  server.listen(socketPath);
  return {
    event: (kind, data) => push({ type: 'event', kind, data }),
    request: (id, kind, payload) => new Promise(resolve => {
      const req = { type: 'request', id, kind, payload }; pending.set(id, { req, resolve }); push(req);
    }),
    resolve: (id, outcome) => { const p = pending.get(id); if (!p) return false; pending.delete(id); push({ type: 'request_resolved', id, outcome, by: 'harness' }); p.resolve({ answered: false, outcome }); return true; },
    isPending: id => pending.has(id),
    setState: s => { state = { ...state, ...s }; push({ type: 'event', kind: 'state', data: state }); },
    onPrompt: fn => promptHandlers.push(fn),
    onControl: fn => controlHandlers.push(fn),
    clientCount: () => clients.size,
    close: () => { for (const c of clients) c.end(); server.close(); try { unlinkSync(socketPath); } catch {} },
  };
}

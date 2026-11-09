// round-trips the shim's own protocol against a fake sdk; no claude process runs.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { PassThrough } from 'node:stream';
import { main } from './shim.mjs';

function harness() {
  const stdin = new PassThrough();
  const stdout = new PassThrough();
  const stderr = new PassThrough();
  const frames = [];
  const waiters = [];
  let buf = '';
  stdout.on('data', d => {
    buf += d;
    let i;
    while ((i = buf.indexOf('\n')) >= 0) {
      const frame = JSON.parse(buf.slice(0, i));
      buf = buf.slice(i + 1);
      frames.push(frame);
      for (const w of [...waiters]) if (w.match(frame)) { waiters.splice(waiters.indexOf(w), 1); w.resolve(frame); }
    }
  });
  const next = match => {
    const found = frames.find(match);
    if (found) return Promise.resolve(found);
    return new Promise(resolve => waiters.push({ match, resolve }));
  };
  const send = obj => stdin.write(JSON.stringify(obj) + '\n');
  return { io: { stdin, stdout, stderr }, frames, next, send };
}

// fakeSdk runs one turn per prompt: each prompt asks for one tool permission and
// reports the decision as assistant text.
function fakeSdk(seen) {
  return {
    query({ prompt, options }) {
      seen.options = options;
      let interruptAbort = null;
      const gen = (async function* () {
        yield { type: 'system', subtype: 'init', session_id: options.sessionId ?? options.resume };
        for await (const user of prompt) {
          seen.prompts.push(user);
          const controller = new AbortController();
          interruptAbort = () => controller.abort();
          const tool = user.message.content === 'ask' ? 'AskUserQuestion' : 'Bash';
          const decision = await options.canUseTool(tool, { command: 'ls' }, {
            requestId: `req-${seen.prompts.length}`, toolUseID: `tool-${seen.prompts.length}`, signal: controller.signal,
          });
          yield { type: 'assistant', uuid: `a-${seen.prompts.length}`, message: { content: [{ type: 'text', text: JSON.stringify(decision) }] } };
          yield { type: 'result', subtype: 'success' };
        }
      })();
      gen.interrupt = async () => { interruptAbort?.(); return { still_queued: [] }; };
      return gen;
    },
  };
}

test('relays prompts, requests, answers, interrupts and exit', async () => {
  const h = harness();
  const seen = { prompts: [] };
  const done = main(['run', '--cwd', '/tmp/x', '--exec', '/bin/claude', '--session-id', 's-1'], fakeSdk(seen), h.io);

  assert.equal((await h.next(f => f.type === 'event')).msg.subtype, 'init');
  assert.equal(seen.options.permissionMode, 'default');
  assert.equal(seen.options.settingSources, undefined, 'default setting sources must not be narrowed');
  assert.equal(seen.options.sessionId, 's-1');

  h.send({ type: 'prompt', uuid: 'u-1', text: 'run it' });
  const request = await h.next(f => f.type === 'request');
  assert.deepEqual([request.id, request.kind, request.tool, request.toolUseId], ['req-1', 'permission', 'Bash', 'tool-1']);
  assert.equal(seen.prompts[0].uuid, 'u-1', 'the service-minted uuid reaches the sdk');

  h.send({ type: 'answer', id: 'req-1', decision: 'deny', message: 'not now' });
  assert.equal((await h.next(f => f.type === 'answer_accepted')).id, 'req-1');
  const reply = await h.next(f => f.type === 'event' && f.msg.uuid === 'a-1');
  assert.deepEqual(JSON.parse(reply.msg.message.content[0].text), { behavior: 'deny', message: 'not now', toolUseID: 'tool-1' });

  h.send({ type: 'answer', id: 'req-1', decision: 'allow' });
  assert.equal((await h.next(f => f.type === 'answer_rejected')).id, 'req-1', 'a second answer is reported, not delivered');

  h.send({ type: 'prompt', uuid: 'u-2', text: 'ask' });
  const question = await h.next(f => f.type === 'request' && f.id === 'req-2');
  assert.equal(question.kind, 'question');
  h.send({ type: 'answer', id: 'req-2', answers: { 'Which?': 'Blue' } });
  const answered = await h.next(f => f.type === 'event' && f.msg.uuid === 'a-2');
  assert.deepEqual(JSON.parse(answered.msg.message.content[0].text).updatedInput.answers, { 'Which?': 'Blue' });

  h.send({ type: 'prompt', uuid: 'u-3', text: 'run again' });
  await h.next(f => f.type === 'request' && f.id === 'req-3');
  h.send({ type: 'interrupt' });
  assert.equal((await h.next(f => f.type === 'request_resolved')).id, 'req-3');
  await h.next(f => f.type === 'interrupted');

  h.send({ type: 'close' });
  assert.equal(await done, 0);
  assert.equal(h.frames.at(-1).type, 'exit');
});

test('refuses to start without exactly one session flag', async () => {
  const h = harness();
  const code = await main(['run', '--cwd', '/tmp/x', '--exec', '/bin/claude'], fakeSdk({ prompts: [] }), h.io);
  assert.equal(code, 2);
});

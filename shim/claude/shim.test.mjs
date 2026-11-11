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
      gen.setModel = async model => {
        if (model === 'refuse') throw new Error('no such model');
        (seen.models ??= []).push(model);
      };
      gen.applyFlagSettings = async settings => { (seen.flags ??= []).push(settings); };
      gen.supportedModels = async () => [{ value: 'opus', displayName: 'Opus', supportedEffortLevels: ['low', 'high'] }];
      gen.initializationResult = async () => {
        if (seen.initFails) throw new Error('cli gone');
        return { claude_code_version: '2.1.318' };
      };
      gen.getContextUsage = async opts => { seen.usage = opts; return { model: 'claude-opus-5-5' }; };
      gen.setPermissionMode = async mode => {
        if (mode === 'refuse') throw new Error('unsupported mode');
        seen.modes.push(mode);
      };
      return gen;
    },
  };
}

test('relays prompts, requests, answers, interrupts and exit', async () => {
  const h = harness();
  const seen = { prompts: [], modes: [] };
  const done = main(['run', '--cwd', '/tmp/x', '--exec', '/bin/claude', '--session-id', 's-1', '--permission-mode', 'default'], fakeSdk(seen), h.io);

  assert.equal((await h.next(f => f.type === 'event')).msg.subtype, 'init');
  assert.equal(seen.options.permissionMode, 'default', 'the mode the service passed');
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

test('passes the chosen mode, omits it when none is given, and switches on request', async () => {
  const h = harness();
  const seen = { prompts: [], modes: [] };
  const done = main(['run', '--cwd', '/tmp/x', '--exec', '/bin/claude', '--resume', 's-2', '--permission-mode', 'auto'], fakeSdk(seen), h.io);
  await h.next(f => f.type === 'event');
  assert.equal(seen.options.permissionMode, 'auto');

  h.send({ type: 'set_permission_mode', id: 'm-1', mode: 'plan' });
  assert.deepEqual(await h.next(f => f.type === 'permission_mode_set' && f.id === 'm-1'), { type: 'permission_mode_set', id: 'm-1', mode: 'plan' });
  assert.deepEqual(seen.modes, ['plan']);
  h.send({ type: 'set_permission_mode', id: 'm-2', mode: 'refuse' });
  const refused = await h.next(f => f.type === 'permission_mode_set' && f.id === 'm-2');
  assert.equal(refused.error, 'unsupported mode', 'a failed switch is reported, not hidden');
  h.send({ type: 'close' });
  assert.equal(await done, 0);

  const h2 = harness();
  const seen2 = { prompts: [], modes: [] };
  const done2 = main(['run', '--cwd', '/tmp/x', '--exec', '/bin/claude', '--session-id', 's-3'], fakeSdk(seen2), h2.io);
  await h2.next(f => f.type === 'event');
  assert.equal('permissionMode' in seen2.options, false, 'no mode passed: claude\'s own settings decide');
  h2.send({ type: 'close' });
  assert.equal(await done2, 0);
});

test('refuses to start without exactly one session flag', async () => {
  const h = harness();
  const code = await main(['run', '--cwd', '/tmp/x', '--exec', '/bin/claude'], fakeSdk({ prompts: [], modes: [] }), h.io);
  assert.equal(code, 2);
});

test('passes model and effort, switches both on request, and lists models idle', async () => {
  const h = harness();
  const seen = { prompts: [], modes: [] };
  const done = main(['run', '--cwd', '/tmp/x', '--exec', '/bin/claude', '--session-id', 's-4', '--model', 'haiku', '--effort', 'low'], fakeSdk(seen), h.io);
  await h.next(f => f.type === 'event');
  assert.equal(seen.options.model, 'haiku');
  assert.equal(seen.options.effort, 'low');
  h.send({ type: 'set_model', id: 'x-1', model: 'sonnet' });
  assert.deepEqual(await h.next(f => f.type === 'model_set' && f.id === 'x-1'), { type: 'model_set', id: 'x-1', model: 'sonnet' });
  h.send({ type: 'set_effort', id: 'x-2', effort: 'high' });
  assert.deepEqual(await h.next(f => f.type === 'effort_set' && f.id === 'x-2'), { type: 'effort_set', id: 'x-2', effort: 'high' });
  assert.deepEqual(seen.models, ['sonnet']);
  assert.deepEqual(seen.flags, [{ effortLevel: 'high' }]);
  h.send({ type: 'set_model', id: 'x-3', model: 'refuse' });
  assert.equal((await h.next(f => f.type === 'model_set' && f.id === 'x-3')).error, 'no such model');
  h.send({ type: 'close' });
  assert.equal(await done, 0);

  const h2 = harness();
  const seen2 = { prompts: [], modes: [] };
  const done2 = main(['run', '--cwd', '/tmp/x', '--exec', '/bin/claude', '--session-id', 's-5'], fakeSdk(seen2), h2.io);
  await h2.next(f => f.type === 'event');
  assert.equal('model' in seen2.options, false, 'no model chosen: the settings decide');
  assert.equal('effort' in seen2.options, false, 'no effort chosen: the settings decide');
  h2.send({ type: 'close' });
  assert.equal(await done2, 0);

  const h3 = harness();
  const seen3 = { prompts: [], modes: [] };
  assert.equal(await main(['models', '--cwd', '/tmp/x', '--exec', '/bin/claude'], fakeSdk(seen3), h3.io), 0);
  const listed = await h3.next(f => 'models' in f);
  assert.equal(listed.version, '2.1.318');
  assert.equal(listed.models[0].value, 'opus');
  assert.equal(seen3.prompts.length, 0, 'the models command sends no prompt');
});


test('an idle launch reports ready without a prompt, and takes one later', async () => {
  const h = harness();
  const seen = { prompts: [], modes: [] };
  const done = main(['run', '--cwd', '/tmp/x', '--exec', '/bin/claude', '--resume', 's-6', '--idle'], fakeSdk(seen), h.io);
  const ready = await h.next(f => f.type === 'ready');
  assert.deepEqual(ready, { type: 'ready', version: '2.1.318', model: 'claude-opus-5-5' });
  assert.deepEqual(seen.usage, { detail: 'summary' }, 'the model comes from the summary, which runs no turn');
  assert.equal(seen.prompts.length, 0, 'ready sends no prompt');
  assert.equal(seen.options.resume, 's-6');
  h.send({ type: 'prompt', uuid: 'u-1', text: 'go' });
  await h.next(f => f.type === 'request');
  assert.equal(seen.prompts.length, 1);
  h.send({ type: 'close' });
  h.send({ type: 'answer', id: 'req-1', decision: 'deny' });
  assert.equal(await done, 0);

  const h2 = harness();
  const seen2 = { prompts: [], modes: [] };
  const done2 = main(['run', '--cwd', '/tmp/x', '--exec', '/bin/claude', '--session-id', 's-7'], fakeSdk(seen2), h2.io);
  await h2.next(f => f.type === 'event');
  h2.send({ type: 'close' });
  assert.equal(await done2, 0);
  assert.equal(h2.frames.some(f => f.type === 'ready'), false, 'a launch with a prompt coming reports no ready');
});

test('an idle launch whose initialize fails reports an error and no ready', async () => {
  const h = harness();
  const seen = { prompts: [], modes: [], initFails: true };
  const done = main(['run', '--cwd', '/tmp/x', '--exec', '/bin/claude', '--resume', 's-8', '--idle'], fakeSdk(seen), h.io);
  const error = await h.next(f => f.type === 'error');
  assert.match(error.message, /did not answer initialize: cli gone/);
  assert.equal(h.frames.some(f => f.type === 'ready'), false);
  h.send({ type: 'close' });
  assert.equal(await done, 0);
});

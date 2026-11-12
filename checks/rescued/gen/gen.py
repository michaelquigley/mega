# appends generated turns to a scratch mega event log: realistic shares of
# native frames, tool calls with results, replies and the odd approval.
# usage: gen.py <events.jsonl> <target events> <seed>
import json, random, sys, uuid, datetime
path, target, seed = sys.argv[1], int(sys.argv[2]), int(sys.argv[3])
rnd = random.Random(seed)
lines = open(path).read().splitlines()
seq = json.loads(lines[-1])['seq']
at = datetime.datetime(2026, 11, 9, 9, 0, tzinfo=datetime.timezone.utc)
words = ('the service log index turn event page stream render scroll read position native tool call result '
         'checkout harness session prompt reply request approval build test gate review').split()
def prose(n):
    return ' '.join(rnd.choice(words) for _ in range(n))
def code(n):
    return '\n'.join(f'func f{i}(x int) int {{ return x * {i} }} // {prose(6)}' for i in range(n))
out = open(path, 'a')
def emit(**ev):
    global seq, at
    seq += 1
    at += datetime.timedelta(seconds=rnd.uniform(0.2, 6))
    ev['seq'] = seq
    ev['at'] = at.isoformat().replace('+00:00', 'Z')
    ev.setdefault('origin', 'managed')
    out.write(json.dumps(ev, separators=(',', ':')) + '\n')
def native(size):
    nid = str(uuid.UUID(int=rnd.getrandbits(128)))
    kind = rnd.choice(['system/status', 'stream_event', 'system/task_progress', 'rate_limit_event', 'system/hook_response'])
    raw = {'type': kind.split('/')[0], 'subtype': kind.split('/')[-1], 'uuid': nid, 'session_id': 'gen',
           'payload': {'detail': prose(size // 6), 'blob': 'x' * (size // 2)}}
    emit(kind='native', native_id=nid, text=kind, raw=raw)
turn = 0
while seq < target:
    turn += 1
    if turn % 25 == 1:
        emit(kind='runtime', origin='service', status='starting')
        emit(kind='runtime', status='started', facts={'session': 'gen', 'model': 'fake-opus-1', 'version': 'fake'})
    emit(kind='user', native_id=str(uuid.UUID(int=rnd.getrandbits(128))), text=f'turn {turn}: ' + prose(rnd.randint(8, 60)))
    native(3000)
    for call in range(rnd.randint(3, 9)):
        tid = f'toolu_{turn}_{call}'
        tool = rnd.choice(['Bash', 'Read', 'Edit', 'Grep'])
        emit(kind='tool_use', tool=tool, tool_use_id=tid, native_id=tid + 'u', text=f'{tool} {prose(5)}',
             raw={'type': 'tool_use', 'id': tid, 'name': tool, 'input': {'command': prose(20), 'body': code(8)}})
        if tool == 'Bash' and rnd.random() < 0.15:
            rid = f'req_{turn}_{call}'
            emit(kind='request', origin='service', req={'id': rid, 'kind': 'permission', 'payload': {'tool': 'Bash', 'input': {'command': prose(10)}}})
            emit(kind='request_resolved', origin='service', req={'id': rid, 'kind': 'permission', 'outcome': 'answered'})
        for _ in range(rnd.randint(2, 6)):
            native(rnd.randint(900, 2600))
        emit(kind='tool_result', tool_use_id=tid, native_id=tid + 'r', text=code(rnd.randint(4, 40)),
             raw={'type': 'tool_result', 'tool_use_id': tid, 'content': code(rnd.randint(4, 30))})
    for _ in range(rnd.randint(4, 12)):
        native(rnd.randint(900, 2600))
    reply = f'## turn {turn}\n\n' + prose(rnd.randint(40, 200)) + '\n\n```go\n' + code(rnd.randint(2, 12)) + '\n```\n\n- ' + prose(12) + '\n- ' + prose(10)
    emit(kind='assistant', native_id=str(uuid.UUID(int=rnd.getrandbits(128))), text=reply, raw={'message': {'content': [{'type': 'text', 'text': reply}]}})
    emit(kind='turn_end', status='success', text='success', native_id=str(uuid.UUID(int=rnd.getrandbits(128))), raw={'type': 'result', 'subtype': 'success'})
out.close()
print(path, seq, turn)

#!/usr/bin/env bash
set -u; cd "$(dirname "$0")"
F=/home/michael/Repos/q/research/roundhouse-fixtures/pi-rt1; C=../common/client.mjs
SOCK=/run/user/1000/roundhouse-probe/pi-5b2.sock; SID=$(python3 -c 'import uuid;print(uuid.uuid4())'); echo "suspend-pending-b2=$SID" >> logs/followup-session-ids.txt
node probe.mjs run --cwd $F --session-id "$SID" --approve-project --model openai-codex/gpt-6-astra --log logs/fu-suspend-b2.jsonl --relay $SOCK --hold --no-prompt --timeout-ms 400000 2>logs/fu-suspend-b2.stderr.txt >logs/fu-suspend-b2.summary.json &
OWNER=$!; for i in $(seq 1 20); do [ -S $SOCK ] && break; sleep 1; done; sleep 2
CHILD=$(jq -r 'select(.kind=="child_spawned") | .childPid' logs/fu-suspend-b2.jsonl); echo "owner=$OWNER pi=$CHILD"
timeout 60 node $C --socket $SOCK --prompt "Using the bash tool, run exactly \`sleep 2\` and then reply ABORT-PENDING-2. Nothing else." --watch-ms 9000 > logs/fu-sb2.1.jsonl 2>/dev/null
jq -c 'select(.type=="request") | {id, kind}' logs/fu-sb2.1.jsonl
echo "-- abort via relay with the dialog pending; owner's abort request has a 60 s deadline"
timeout 30 node $C --socket $SOCK --interrupt --watch-ms 12000 > logs/fu-sb2.2.jsonl 2>/dev/null
echo "abort response so far: $(jq -c 'select(.kind=="abort_response") | .response' logs/fu-suspend-b2.jsonl | head -1)"
echo "-- close stdin now (dialog pending, abort unanswered)"; T0=$(date +%s)
timeout 20 node $C --socket $SOCK --watch-ms 1000 --close >/dev/null 2>&1
for i in $(seq 1 40); do kill -0 $OWNER 2>/dev/null || break; sleep 1; done
echo "owner alive after close? $(kill -0 $OWNER 2>/dev/null && echo yes || echo no) ($(( $(date +%s) - T0 )) s)"
ps -p $CHILD >/dev/null 2>&1 && echo "pi $CHILD ALIVE" || echo "pi $CHILD gone"; echo "children of $CHILD: $(ps -o pid= --ppid $CHILD 2>/dev/null | wc -l)"
tail -5 logs/fu-suspend-b2.stderr.txt | cut -c1-200
jq -c 'select(.kind=="abort_response" or .kind=="child_exit" or .kind=="relay_close" or .kind=="stdin_closed_during_turn" or .kind=="no_exit")' logs/fu-suspend-b2.jsonl | cut -c1-200
jq -c 'select(.kind=="frame" and .t > "'"$(jq -r 'select(.kind=="stdin_closed_during_turn") | .t' logs/fu-suspend-b2.jsonl)"'") | .frame | {type, command, success, error}' logs/fu-suspend-b2.jsonl | head -5 | cut -c1-200
FILE=$(jq -r 'select(.kind=="state") | .data.sessionFile' logs/fu-suspend-b2.jsonl | head -1)
echo "-- session file message roles:"; jq -r 'select(.type=="message") | .message.role + " " + (.message.content | if type=="string" then .[0:50] else map(.type + ":" + ((.text // .name // "") | .[0:40])) | join(" ") end)' "$FILE" | cut -c1-140
if kill -0 $OWNER 2>/dev/null; then echo "cleanup: SIGTERM my owner $OWNER"; kill -TERM $OWNER; sleep 2; ps -p $CHILD >/dev/null 2>&1 && { echo "cleanup: SIGTERM my pi child $CHILD"; kill -TERM $CHILD; }; fi
rm -f $SOCK; echo "===== DONE ====="

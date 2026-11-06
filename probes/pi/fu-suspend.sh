#!/usr/bin/env bash
# T5b: dialog pending, no answer -> abort -> stdin close. T5c: active command, abort vs wait.
set -u
cd "$(dirname "$0")"
F=/home/michael/Repos/q/research/roundhouse-fixtures/pi-rt1; C=../common/client.mjs
run_owner() { # name sock sid
  node probe.mjs run --cwd $F --session-id "$3" --approve-project --model openai-codex/gpt-6-astra --log "logs/$1.jsonl" --relay "$2" --hold --no-prompt --timeout-ms 400000 2>"logs/$1.stderr.txt" >"logs/$1.summary.json" &
  OWNER=$!; for i in $(seq 1 20); do [ -S "$2" ] && break; sleep 1; done; sleep 2
  CHILD=$(jq -r 'select(.kind=="child_spawned") | .childPid' "logs/$1.jsonl"); echo "owner=$OWNER pi=$CHILD"
}
procs() { ps -p "$1" >/dev/null 2>&1 && echo "pi $1 ALIVE" || echo "pi $1 gone"; echo "children of $1: $(ps -o pid= --ppid "$1" 2>/dev/null | wc -l)"; }

echo "===== T5b: dialog pending, unanswered -> abort -> close ====="
SOCK=/run/user/1000/roundhouse-probe/pi-5b.sock; SID=$(python3 -c 'import uuid;print(uuid.uuid4())'); echo "suspend-pending-b=$SID" >> logs/followup-session-ids.txt
run_owner fu-suspend-b $SOCK $SID
timeout 60 node $C --socket $SOCK --prompt "Using the bash tool, run exactly \`sleep 2\` and then reply ABORT-PENDING-1. Nothing else." --watch-ms 9000 > logs/fu-sb.1.jsonl 2>/dev/null
jq -c 'select(.type=="request") | {id, kind}' logs/fu-sb.1.jsonl
echo "-- abort via relay with the dialog pending (owner waits up to 60 s for a response)"
T0=$(date +%s); timeout 30 node $C --socket $SOCK --interrupt --watch-ms 20000 > logs/fu-sb.2.jsonl 2>/dev/null
echo "abort response after $(( $(date +%s) - T0 )) s:"; jq -c 'select(.kind=="abort_response")' logs/fu-suspend-b.jsonl | cut -c1-200 || true
echo "-- close stdin with the dialog still pending"; T0=$(date +%s); timeout 20 node $C --socket $SOCK --watch-ms 1000 --close >/dev/null 2>&1
for i in $(seq 1 30); do kill -0 $OWNER 2>/dev/null || break; sleep 1; done; echo "owner alive after close? $(kill -0 $OWNER 2>/dev/null && echo yes || echo no) (waited $(( $(date +%s) - T0 )) s)"
sleep 1; procs $CHILD; tail -4 logs/fu-suspend-b.stderr.txt | cut -c1-200
jq -c 'select(.kind=="abort_response" or .kind=="child_exit" or .kind=="relay_close" or .kind=="no_exit")' logs/fu-suspend-b.jsonl | cut -c1-200
FILE=$(jq -r 'select(.kind=="state") | .data.sessionFile' logs/fu-suspend-b.jsonl | head -1)
echo "-- session file message roles:"; jq -r 'select(.type=="message") | .message.role + " " + (.message.content | if type=="string" then .[0:50] else map(.type + ":" + ((.text // .name // "") | .[0:40])) | join(" ") end)' "$FILE" | cut -c1-140

echo; echo "===== T5c-1: active command (python sleep 25 approved) -> abort (stop) -> close ====="
SOCK=/run/user/1000/roundhouse-probe/pi-5c.sock; SID=$(python3 -c 'import uuid;print(uuid.uuid4())'); echo "suspend-active-abort=$SID" >> logs/followup-session-ids.txt
run_owner fu-suspend-c1 $SOCK $SID
timeout 60 node $C --socket $SOCK --prompt "Using the bash tool, run exactly \`python3 -c 'import time; time.sleep(25); print(\"slept\")'\` in the foreground and then reply ACTIVE-1. Nothing else." --answer-json '{"value":"Allow"}' --watch-ms 9000 > logs/fu-sc1.1.jsonl 2>/dev/null
PY=$(pgrep -f -P $CHILD 'python3 -c' | head -1); echo "python child pid: ${PY:-none} (parent pi $CHILD)"
echo "-- abort now (stop)"; T0=$(date +%s); timeout 30 node $C --socket $SOCK --interrupt --watch-ms 8000 > logs/fu-sc1.2.jsonl 2>/dev/null
jq -c 'select(.kind=="abort_response") | .response' logs/fu-suspend-c1.jsonl | cut -c1-120; jq -c 'select(.type=="event" and .kind=="msg" and .data.type=="agent_settled") | .data' logs/fu-sc1.2.jsonl
[ -n "${PY:-}" ] && { ps -p $PY >/dev/null 2>&1 && echo "python $PY ALIVE after abort" || echo "python $PY gone after abort"; }
timeout 20 node $C --socket $SOCK --watch-ms 1000 --close >/dev/null 2>&1; for i in $(seq 1 30); do kill -0 $OWNER 2>/dev/null || break; sleep 1; done; echo "owner gone after $(( $(date +%s) - T0 )) s from abort"; procs $CHILD
FILE=$(jq -r 'select(.kind=="state") | .data.sessionFile' logs/fu-suspend-c1.jsonl | head -1); echo "-- session file roles:"; jq -r 'select(.type=="message") | .message.role' "$FILE" | tr '\n' ' '; echo

echo; echo "===== T5c-2: active command -> wait until settled -> close ====="
SOCK=/run/user/1000/roundhouse-probe/pi-5c2.sock; SID=$(python3 -c 'import uuid;print(uuid.uuid4())'); echo "suspend-active-wait=$SID" >> logs/followup-session-ids.txt
run_owner fu-suspend-c2 $SOCK $SID
timeout 60 node $C --socket $SOCK --prompt "Using the bash tool, run exactly \`python3 -c 'import time; time.sleep(25); print(\"slept\")'\` in the foreground and then reply ACTIVE-2. Nothing else." --answer-json '{"value":"Allow"}' --watch-ms 9000 > logs/fu-sc2.1.jsonl 2>/dev/null
T0=$(date +%s); until grep -q 'settled' logs/fu-suspend-c2.stderr.txt; do sleep 2; [ $(( $(date +%s) - T0 )) -gt 120 ] && break; done; echo "settled after $(( $(date +%s) - T0 )) s: $(grep -o 'settled.*' logs/fu-suspend-c2.stderr.txt | cut -c1-80)"
timeout 20 node $C --socket $SOCK --watch-ms 1000 --close >/dev/null 2>&1; for i in $(seq 1 30); do kill -0 $OWNER 2>/dev/null || break; sleep 1; done; procs $CHILD
FILE=$(jq -r 'select(.kind=="state") | .data.sessionFile' logs/fu-suspend-c2.jsonl | head -1); echo "-- session file roles:"; jq -r 'select(.type=="message") | .message.role' "$FILE" | tr '\n' ' '; echo
ls /run/user/1000/roundhouse-probe/
echo "===== DONE ====="

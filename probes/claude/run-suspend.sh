#!/usr/bin/env bash
# Claude suspension scenarios (follow-up brief, section 2): idle stop and reopen,
# stop with a permission pending, stop vs wait with an active command.
set -u
F=${FIXTURE:-/home/michael/Repos/q/research/roundhouse-fixtures/claude-rt1}
SOCK=${SOCK:-/run/user/$(id -u)/rh-claude-s.sock}; C=../common/client.mjs
procs() { pgrep -af "$1" | grep -v -E 'pgrep|run-suspend' | wc -l; }
children() { ps -eo pid,cmd | grep -E 'python3 -c .?import time; time\.sleep\(25\)' | grep -v grep | wc -l; }
ONLY=${ONLY:-all}   # ONLY=active runs just the S3/S4 active-command cases
if [ "$ONLY" = all ]; then
echo "===== S1 idle: complete a turn, stop cleanly, verify exit ====="
S1=$(python3 -c 'import uuid;print(uuid.uuid4())'); echo "suspend-idle=$S1" | tee -a logs/recovery-session-ids.txt
node probe.mjs run --cwd "$F" --session-id "$S1" --permission-mode default --turns scenarios/suspend-idle.json --log logs/suspend-idle.jsonl 2>logs/suspend-idle.stderr.txt >logs/suspend-idle.summary.json; echo "probe exit=$? result=$(jq -r '.results[0].result' logs/suspend-idle.summary.json)"
sleep 2; echo "CLI processes for $S1 after probe exit: $(procs "$S1")"; echo "registry entries for session: $(grep -l "$S1" ~/.claude/sessions/*.json 2>/dev/null | wc -l)"
echo "-- reopen with NO prompt (empty turns): does init arrive and the process exit without model work?"
node probe.mjs run --cwd "$F" --resume "$S1" --permission-mode default --turns scenarios/empty.json --log logs/suspend-reopen-empty.jsonl 2>logs/suspend-reopen-empty.stderr.txt >logs/suspend-reopen-empty.summary.json; echo "reopen-empty exit=$? init=$(jq -c '.init | {cwd, model, permissionMode}' logs/suspend-reopen-empty.summary.json) results=$(jq -c '.results|length' logs/suspend-reopen-empty.summary.json) msgs=$(jq -r 'select(.kind=="msg") | .msg.type' logs/suspend-reopen-empty.jsonl | sort | uniq -c | tr '\n' ' ')"
echo "-- stored history without a process: getSessionMessages"; node probe.mjs messages --session "$S1" --cwd "$F" | cut -c1-120
echo "-- marked follow-up"; node probe.mjs run --cwd "$F" --resume "$S1" --permission-mode default --turns scenarios/suspend-follow.json --log logs/suspend-follow.jsonl 2>logs/suspend-follow.stderr.txt >logs/suspend-follow.summary.json; echo "follow exit=$?"; jq -r '.results[0].result' logs/suspend-follow.summary.json
P=~/.claude/projects/-home-michael-Repos-q-research-roundhouse-fixtures-claude-rt1; echo "chain:"; jq -r 'select(.type=="user" or .type=="assistant") | "\(.uuid[0:8]) <- \((.parentUuid//"root")[0:8]) \(.type)"' $P/$S1.jsonl
echo; echo "===== S2 pending permission: stop with a request pending (no approval) ====="
rm -f "$SOCK"; S2=$(python3 -c 'import uuid;print(uuid.uuid4())'); echo "suspend-pending=$S2" | tee -a logs/recovery-session-ids.txt
node probe.mjs run --cwd "$F" --session-id "$S2" --permission-mode default --turns scenarios/int-permission.json --log logs/suspend-pending.jsonl --relay "$SOCK" --hold --timeout-ms 300000 2>logs/suspend-pending.stderr.txt >logs/suspend-pending.summary.json &
O2=$!; for i in $(seq 1 30); do [ -S "$SOCK" ] && break; sleep 1; done
for i in $(seq 1 30); do grep -q 'relay: waiting' logs/suspend-pending.stderr.txt && break; sleep 2; done; echo "permission pending at $(date -u +%T); sending close (stdin end) without answering"
T0=$(date +%s); node $C --socket "$SOCK" --close --watch-ms 1000 > logs/suspend-pending.client.jsonl 2>&1
for i in $(seq 1 30); do kill -0 $O2 2>/dev/null || break; sleep 1; done; kill -0 $O2 2>/dev/null && echo "owner still alive after 30 s; waiting (bounded by probe timeout) to see whether the CLI ends the request itself"; wait $O2; echo "owner exit=$? after $(( $(date +%s) - T0 )) s; CLI procs for $S2: $(procs "$S2")"
echo "relay outcome: $(jq -c 'select(.kind=="relay_request_outcome" or .kind=="relay_request_aborted")' logs/suspend-pending.jsonl | cut -c1-200)"; echo "results: $(jq -c 'select(.kind=="msg" and .msg.type=="result") | .msg | {subtype,is_error}' logs/suspend-pending.jsonl)"
echo "transcript tail:"; jq -r 'select(.type=="user" or .type=="assistant") | .type + ": " + (.message.content | if type=="string" then .[0:80] else map(if .type=="text" then .text[0:80] elif .type=="tool_use" then "[tool_use " + .name + "]" elif .type=="tool_result" then "[tool_result " + ((.content|tostring)[0:80]) + "]" else "[" + .type + "]" end)|join(" ") end)' $P/$S2.jsonl | tail -4
fi
echo; echo "===== S3 active command: stop (interrupt) then close, vs S4 wait-until-idle then close ====="
for MODE in interrupt wait; do
  rm -f "$SOCK"; S=$(python3 -c 'import uuid;print(uuid.uuid4())'); echo "suspend-active-$MODE=$S" | tee -a logs/recovery-session-ids.txt
  node probe.mjs run --cwd "$F" --session-id "$S" --permission-mode default --turns scenarios/suspend-active.json --log logs/suspend-active-$MODE.jsonl --relay "$SOCK" --hold --timeout-ms 300000 2>logs/suspend-active-$MODE.stderr.txt >logs/suspend-active-$MODE.summary.json &
  O=$!; for i in $(seq 1 30); do [ -S "$SOCK" ] && break; sleep 1; done
  node $C --socket "$SOCK" --answer-json '{"decision":"allow"}' --watch-ms 12000 > logs/suspend-active-$MODE.clientA.jsonl 2>&1
  echo "[$MODE] command running? children=$(children)"
  T0=$(date +%s)
  if [ "$MODE" = interrupt ]; then node $C --socket "$SOCK" --interrupt --watch-ms 4000 > logs/suspend-active-$MODE.clientI.jsonl 2>&1; echo "[$MODE] after interrupt: children=$(children)"; fi
  if [ "$MODE" = wait ]; then for i in $(seq 1 40); do grep -q 'result success' logs/suspend-active-$MODE.stderr.txt && break; sleep 2; done; echo "[$MODE] waited $(( $(date +%s) - T0 )) s for result; children=$(children)"; fi
  node $C --socket "$SOCK" --close --watch-ms 1000 > logs/suspend-active-$MODE.clientC.jsonl 2>&1
  for i in $(seq 1 40); do kill -0 $O 2>/dev/null || break; sleep 1; done; wait $O; echo "[$MODE] owner exit=$? total $(( $(date +%s) - T0 )) s; CLI procs: $(procs "$S"); children=$(children)"
  echo "[$MODE] results: $(jq -c 'select(.kind=="msg" and .msg.type=="result") | .msg | {subtype,is_error}' logs/suspend-active-$MODE.jsonl | tr '\n' ' ')"
done
rm -f "$SOCK"

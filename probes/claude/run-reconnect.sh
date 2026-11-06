#!/usr/bin/env bash
# Claude reconnect scenarios (follow-up brief, section 1). Owner = probe.mjs in
# --relay --hold mode; clients = ../common/client.mjs. Socket path must be short.
set -u
F=${FIXTURE:-/home/michael/Repos/q/research/roundhouse-fixtures/claude-rt1}
SOCK=${SOCK:-/run/user/$(id -u)/rh-claude.sock}; rm -f "$SOCK"
C=../common/client.mjs; L=logs/reconnect
S=$(python3 -c 'import uuid;print(uuid.uuid4())'); echo "reconnect=$S" | tee -a logs/recovery-session-ids.txt
node probe.mjs run --cwd "$F" --session-id "$S" --permission-mode default --turns scenarios/reconnect-work.json --log $L.jsonl --relay "$SOCK" --hold --timeout-ms 900000 2>$L.stderr.txt >$L.summary.json &
OWNER=$!; for i in $(seq 1 30); do [ -S "$SOCK" ] && break; sleep 1; done; [ -S "$SOCK" ] || { echo "socket never appeared"; kill $OWNER; exit 1; }; echo "owner pid $OWNER, socket up"
echo "== T1: client A answers the approval then drops mid-command =="
node $C --socket "$SOCK" --answer-json '{"decision":"allow"}' --disconnect-after-ms 14000 > $L.clientA.jsonl 2> $L.clientA.err; cat $L.clientA.err
echo "-- waiting (max 120 s) for the owner to receive the result with no client connected"
for i in $(seq 1 60); do grep -q 'RECONNECT-WORK-1' $L.stderr.txt && grep -q 'result success' $L.stderr.txt && break; sleep 2; done
echo "owner result seen: $(grep -c 'result success' $L.stderr.txt); client closes so far: $(grep -c relay_client_closed $L.jsonl)"
echo "== T1b: fresh client B replays from seq 0 =="
node $C --socket "$SOCK" --since 0 --watch-ms 4000 > $L.clientB.jsonl 2> $L.clientB.err; cat $L.clientB.err
echo "B saw result: $(jq -c 'select(.type=="event" and .data.type=="result") | .data.result' $L.clientB.jsonl)"
echo "B saw request_resolved: $(jq -c 'select(.type=="request_resolved") | {id,outcome,by}' $L.clientB.jsonl)"
echo "== T2: client C asks for a question, drops before answering =="
node $C --socket "$SOCK" --prompt 'Use the AskUserQuestion tool to ask me which color I prefer, options Red and Blue. After I answer, reply with my color followed by RECONNECT-Q-2. Nothing else.' --disconnect-after-ms 9000 > $L.clientC.jsonl 2> $L.clientC.err; cat $L.clientC.err
sleep 4
echo "== T2b: client D connects; snapshot should list the pending question; D answers Blue =="
node $C --socket "$SOCK" --since 0 --answer-json '{"answers":{"Which color do you prefer?":"Blue"}}' --watch-ms 15000 > $L.clientD.jsonl 2> $L.clientD.err; cat $L.clientD.err
echo "D snapshot pending: $(jq -c 'select(.type=="snapshot") | .pending | map({id,kind})' $L.clientD.jsonl)"
echo "D saw assistant text: $(jq -r 'select(.type=="event" and .data.type=="assistant") | .data.message.content[] | select(.type=="text") | .text' $L.clientD.jsonl | tail -1)"
echo "== T3: client E triggers a permission and drops; F interrupts while E is away; G reconnects and answers late =="
node $C --socket "$SOCK" --prompt 'Using the Bash tool, run exactly `sleep 2` and then reply RECONNECT-P-3. Nothing else.' --disconnect-after-ms 9000 > $L.clientE.jsonl 2> $L.clientE.err; cat $L.clientE.err
REQ=$(jq -r 'select(.type=="request") | .id' $L.clientE.jsonl | tail -1); echo "pending request id seen by E: $REQ"
node $C --socket "$SOCK" --interrupt --watch-ms 5000 > $L.clientF.jsonl 2> $L.clientF.err; cat $L.clientF.err
node $C --socket "$SOCK" --since 0 --answer-id "$REQ" --answer-json '{"decision":"allow"}' --watch-ms 5000 > $L.clientG.jsonl 2> $L.clientG.err; cat $L.clientG.err
echo "G snapshot pending: $(jq -c 'select(.type=="snapshot") | .pending | map(.id)' $L.clientG.jsonl)"
echo "G answer_rejected: $(jq -c 'select(.type=="answer_rejected")' $L.clientG.jsonl)"
echo "== T4: client H sends a follow-up, then closes the owner =="
node $C --socket "$SOCK" --prompt 'Reply with exactly RECONNECT-AFTER-4 and nothing else.' --watch-ms 15000 --close > $L.clientH.jsonl 2> $L.clientH.err; cat $L.clientH.err
echo "H saw: $(jq -r 'select(.type=="event" and .data.type=="assistant") | .data.message.content[] | select(.type=="text") | .text' $L.clientH.jsonl | tail -1)"
wait $OWNER; echo "owner exit=$?"
echo "== owner narration =="; grep -E 'relay|result|interrupt|can_use_tool|assistant text' $L.stderr.txt | cut -c1-170

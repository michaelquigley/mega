#!/usr/bin/env bash
# Grouped suspension exercise (follow-up brief, section 2): two idle managed
# sessions (one Claude, one Codex) recorded in a static manifest, stopped, then
# reopened from the manifest with their histories, then one marked follow-up each.
set -u
CF=/home/michael/Repos/q/research/roundhouse-fixtures/claude-rt1; XF=/home/michael/Repos/q/research/roundhouse-fixtures/codex-rt1
M=${MANIFEST:-grouped-manifest.json}; L=logs; mkdir -p $L
echo "===== 1. create two idle managed sessions ====="
CS=$(python3 -c 'import uuid;print(uuid.uuid4())')
( cd ../claude && echo '["Reply with the project instruction marker token followed by GROUP-CLAUDE-1. Nothing else."]' > scenarios/group-1.json && node probe.mjs run --cwd $CF --session-id $CS --permission-mode default --turns scenarios/group-1.json --log logs/group-claude-1.jsonl 2>logs/group-claude-1.stderr.txt >logs/group-claude-1.summary.json; echo "claude exit=$? $(jq -r '.results[0].result' logs/group-claude-1.summary.json)" )
( cd ../codex && echo '["Reply with the project instruction marker token followed by GROUP-CODEX-1. Nothing else."]' > scenarios/group-1.json && node probe.mjs run --cwd $XF --turns scenarios/group-1.json --log logs/group-codex-1.jsonl --approve 'approval-probe' 2>logs/group-codex-1.stderr.txt >logs/group-codex-1.summary.json; echo "codex exit=$? $(jq -r '.turns[0].agentText' logs/group-codex-1.summary.json)" )
XT=$(jq -r '.thread.id' ../codex/logs/group-codex-1.summary.json); XP=$(jq -r '.thread.path' ../codex/logs/group-codex-1.summary.json)
cat > $M <<JSON
{
  "project": "roundhouse-grouped-test",
  "suspended_at": "$(date -u +%FT%TZ)",
  "conversations": [
    { "harness": "claude", "version": "2.1.318", "native_id": "$CS", "cwd": "$CF", "transcript": "$HOME/.claude/projects/-home-michael-Repos-q-research-roundhouse-fixtures-claude-rt1/$CS.jsonl" },
    { "harness": "codex", "version": "0.166.0", "native_id": "$XT", "cwd": "$XF", "rollout": "$XP" }
  ]
}
JSON
echo "manifest:"; cat $M
echo "===== 2. verify both harness processes exited (bounded 10 s) ====="
for i in $(seq 1 10); do c=$(pgrep -f "$CS" | grep -v pgrep | wc -l); x=$(pgrep -f "$XT" | grep -v pgrep | wc -l); [ "$c" = 0 ] && [ "$x" = 0 ] && break; sleep 1; done; echo "claude procs=$c codex procs=$x (after ${i}s)"; echo "claude live registry: $(claude agents --json --all 2>/dev/null | jq -c "[.[] | select(.sessionId==\"$CS\")]")"
echo "===== 3. reopen from the manifest without new work ====="
echo "-- claude: stored history (no process)"; ( cd ../claude && node probe.mjs info --session $CS --cwd $CF | jq -c '{sessionId, cwd, firstPrompt: .firstPrompt[0:50]}'; node probe.mjs messages --session $CS --cwd $CF | cut -c1-110 )
echo "-- codex: thread/resume with no turn (process, history, no model work)"; ( cd ../codex && echo '[]' > scenarios/empty.json && node probe.mjs run --cwd $XF --resume $XT --turns scenarios/empty.json --log logs/group-codex-reopen.jsonl --approve 'approval-probe' 2>logs/group-codex-reopen.stderr.txt >logs/group-codex-reopen.summary.json; echo "codex reopen exit=$? priorTurns=$(jq -c '.thread.priorTurns // .thread' logs/group-codex-reopen.summary.json | cut -c1-200) turnsStarted=$(jq -c '.turns|length' logs/group-codex-reopen.summary.json)"; grep -c 'turn/started' logs/group-codex-reopen.jsonl | sed 's/^/turn\/started frames: /' )
echo "===== 4. one marked follow-up each ====="
( cd ../claude && echo '["List every marker token you have seen in this conversation so far, in order, one per line, then append GROUP-CLAUDE-2 on its own line. Nothing else."]' > scenarios/group-2.json && node probe.mjs run --cwd $CF --resume $CS --permission-mode default --turns scenarios/group-2.json --log logs/group-claude-2.jsonl 2>logs/group-claude-2.stderr.txt >logs/group-claude-2.summary.json; echo "claude follow-up exit=$?"; jq -r '.results[0].result' logs/group-claude-2.summary.json; echo "chain entries: $(jq -r 'select(.type=="user" or .type=="assistant") | .uuid' $HOME/.claude/projects/-home-michael-Repos-q-research-roundhouse-fixtures-claude-rt1/$CS.jsonl | wc -l)" )
( cd ../codex && echo '["List every marker token you have seen in this conversation so far, in order, one per line, then append GROUP-CODEX-2 on its own line. Nothing else."]' > scenarios/group-2.json && node probe.mjs run --cwd $XF --resume $XT --turns scenarios/group-2.json --log logs/group-codex-2.jsonl --approve 'approval-probe' 2>logs/group-codex-2.stderr.txt >logs/group-codex-2.summary.json; echo "codex follow-up exit=$?"; jq -r '.turns[0].agentText' logs/group-codex-2.summary.json )
echo "===== 5. processes after the exercise ====="; sleep 2; echo "claude procs=$(pgrep -f "$CS" | grep -v pgrep | wc -l) codex procs=$(pgrep -f "$XT" | grep -v pgrep | wc -l)"

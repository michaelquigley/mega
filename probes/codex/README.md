# Codex app-server probe

Minimal, dependency-free Node scripts that drive the installed `codex app-server` (codex-cli 0.166.0) over stdio JSON-RPC for the Roundhouse harness investigation.

## Requirements

- Node 24 (tested with v24.12.0) and `codex` on `PATH` (tested: `/home/michael/local/node/bin/codex`, codex-cli 0.166.0).
- The operator's existing Codex login. Nothing here changes `~/.codex/config.toml` or auth.
- Does not use or touch the shared app-server daemon (`codex app-server daemon ...`, `codex agents`).

## Files

- `probe.mjs`: spawns its own `codex app-server` child over stdio, runs scripted turns, answers approval and user-input server requests by policy, logs every frame to JSONL.
- `probe-ws.mjs`: two-client test against one `codex app-server --listen ws://127.0.0.1:PORT`. Written, not run: the session's permission classifier denied starting a loopback listener. Run only with the operator's approval.
- `scenarios/*.json`: turn lists used by the recorded runs.
- `logs/`: gitignored. Each run writes `<name>.jsonl` (all frames), `<name>.stderr.txt` (probe narration), `<name>.summary.json`.

## Usage

```
cd probes/codex
node probe.mjs run --cwd <fixture-dir> --turns scenarios/rt1-managed.json \
  --log logs/rt1-managed.jsonl --approve 'approval-probe' --approve-delay-ms 4000 --answer '^Blue$'
node probe.mjs run --cwd <fixture-dir> --resume <thread-id> --turns scenarios/rt1-resume.json --log logs/x.jsonl --approve 'approval-probe'
node probe.mjs list --cwd <fixture-dir>
node probe.mjs read --thread <thread-id>
```

Flags: `--approve REGEX` (accept matching command approvals, else decline), `--approve-delay-ms N`, `--answer REGEX` (option label for `item/tool/requestUserInput`), `--interrupt-after-ms N [--interrupt-turn I]`, `--interrupt-on-approval`, `--stale-after-interrupt`, `--kill-owner-after-ms N`, `--kill-server-after-ms N`, `--approval-policy`, `--sandbox`, `--timeout-ms`.

## Protocol source

Shapes were taken from the installed binary, not from memory:

```
codex app-server generate-json-schema --out <dir>
codex app-server generate-ts --out <dir>
```

## Follow-up flags (reconnect and suspension)

- `--relay SOCK [--hold]`: owner mode. Every server frame is published to relay clients; approval and `requestUserInput` server requests wait for a relay client's answer (`{"decision":"accept"|"decline"}` or `{"answers":{...}}`); a `serverRequest/resolved` from the server marks the request resolved for clients; relay prompts become `turn/start`; `--hold` keeps the owner alive after the turns file until a client sends `close`. Use a short socket path such as `/run/user/1000/rh-codex.sock` (long scratchpad paths fail with `EINVAL`). Client: `node ../common/client.mjs`.
- `--stale-after-harness-resolve`: in relay mode, send a raw late JSON-RPC accept 2 s after the server itself resolved a pending request, to record the server's reaction.
- `--close-on-approval`: close stdin while an approval is pending and record whether the app-server exits.
- `--steer-answer TEXT [--steer-delay-ms N]`: when an `agentMessage` item carries `questions[]` (default-mode async question), send TEXT into the active turn with `turn/steer`.
- `--start-json JSON`, `--turn-json JSON`: extra params merged into `thread/start`/`thread/resume` and `turn/start`.
- `--config k=v[,k=v]`: passed to `codex app-server -c`.
- `scenarios/none.json` (`[]`) resumes a thread without starting a turn.

See `NOTES.md` for the first investigation and `NOTES-followup.md` for the reconnect, suspension and steering results.

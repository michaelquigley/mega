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

## MCP elicitations (2026-11-09)

- `--elicit PLAN`: answers successive `mcpServer/elicitation/request` frames from a `;`-separated plan: `accept`, `decline`, `accept-meta:<json>` (accept with that `_meta`), `form:<json>` (accept with that `content`), or `interrupt` (interrupt the turn, wait, record whether `serverRequest/resolved` followed, then send a stale accept). Every elicitation frame is logged whole.
- The `codex-rt1` fixture's MCP server is `mcp/fixture-server.mjs`, named in the fixture's `.codex/config.toml`. Its `echo` tool logs each call to `mcp/mcp-calls.log`; its `pick_color` tool raises a typed form elicitation.
- Scenarios: `mcp-accept-decline.json`, `mcp-session.json`, `mcp-form.json`, `mcp-interrupt.json`, `mcp-always.json`. Run `mcp-always` only under a scratch `CODEX_HOME`, and restore the fixture's `.codex/config.toml` afterwards: "always" writes an approval into the config that defines the server.

## Model and effort (2026-11-10)

- `node probe.mjs models --cwd DIR [--include-hidden]` runs `model/list` and `config/read {cwd}` on a thread-less app-server and prints the catalog with the checkout's effective `model` and `model_reasoning_effort`.
- A turns-file item may carry `"turn_json"`, merged into that turn's `turn/start` only. The scenarios are `effort-override.json` and `model-override.json`, the latter with `--start-json '{"model":"gpt-6-luna"}'`.

## Background terminals (2026-11-10)

`node background.mjs --cwd DIR --log logs/x.jsonl [--poll-seconds 50] [--session]` runs one turn that asks for a command that outlives it. It then polls `thread/backgroundTerminals/list` every two seconds after `turn/completed`, recording every frame. Without `--session` the model is left to background the command its own way (it detached it with `nohup`). With `--session` it is asked to run it directly with a short yield, so it stays a unified-exec terminal session. Approvals are accepted only for the exact command.


# Codex findings (fork notes, to be merged into docs/research/harness-capabilities.md)

Tested 2026-11-05 on fortyfive (Ubuntu 26.04.1, Linux 7.0.0-42-generic).

- codex-cli 0.166.0, `/home/michael/local/node/bin/codex` -> `.../node_modules/@openai/codex/bin/codex.js`, Node v24.12.0.
- Auth: operator's existing ChatGPT login (`account/updated` reported `authMode: chatgpt`). No API key, no config changes.
- Operator config (`~/.codex/config.toml`) sets `model = "gpt-6-astra"` and `model_reasoning_effort = "high"`; it sets no `approval_policy` or `sandbox_mode`, so the defaults below are the installed defaults.
- Protocol reference: `codex app-server generate-json-schema` / `generate-ts` output for 0.166.0 (version-matched). The upstream page https://developers.openai.com/codex/app-server was fetched 2026-11-05; it shows no publication date. Claims from it are labelled "docs".
- Fixture: `/home/michael/Repos/q/research/roundhouse-fixtures/codex-rt1` (git, `fixture.txt` with `FIXTURE-TOKEN-91c2`, `AGENTS.md` with `ROUNDHOUSE-MARKER-7f3a`).
- All runs used a private `codex app-server` child over stdio. The operator's shared daemon was not touched.

## Results

| # | Scenario | Result | Continuity | Notes |
|---|----------|--------|------------|-------|
| 1 | Start persisted thread | pass | n/a | `thread/start {cwd}` -> thread `01a1ad7d-899f-77c3-b17c-bc847cdb51a4`, rollout `~/.codex/sessions/2026/11/05/rollout-2026-11-05T15-54-08-<id>.jsonl`, model `gpt-6-astra`, `approvalPolicy: on-request`, `sandbox: {type: readOnly, networkAccess: false}`, `instructionSources: [<fixture>/AGENTS.md]`. |
| 2 | Two marker turns, AGENTS.md marker | pass | kept | `ROUNDHOUSE-MARKER-7f3a ALPHA-1a2b`; `cat fixture.txt` ran with no approval (read-only sandbox allows it) -> `FIXTURE-TOKEN-91c2 BETA-3c4d`. |
| 3a | Genuine approval | pass | kept | File write escaped the read-only sandbox -> server request `item/commandExecution/requestApproval` (JSON-RPC id 0). Probe held it 4 s; the agent waited (no further items until the response). `thread/status/changed` carried `activeFlags: ["waitingOnApproval"]` while pending and `[]` after; `serverRequest/resolved {threadId, requestId}` was emitted on our reply. |
| 3b | Explicit user-input request | partial | kept | `item/tool/requestUserInput` never fired. Server stderr: `request_user_input is unavailable in Default mode`. The model fell back to an `agentMessage` item with `delivery: "async"` and `questions: [{title, options}]`, then polled with 60 s `sleep` tool items and the turn did not end on its own. The run was stopped by killing the probe (my `pkill` also hit the shell; the thread persisted). The answer was delivered as the next `turn/start` text ("Blue") after resume and honoured (`DELTA-7a8b`). `turn/steer` into the sleeping turn was not tested. |
| 4 | Resume in a new app-server | pass | kept | `thread/resume {threadId, cwd}` -> `thread.turns` length 4 (including the aborted question turn), same cwd/model/sandbox/approval policy, same `instructionSources`. New turns appended; a second write triggered a fresh approval request with the same shape. Marker list after resume: `ROUNDHOUSE-MARKER-7f3a, ALPHA-1a2b, BETA-3c4d, FIXTURE-TOKEN-91c2, GAMMA-5e6f, DELTA-7a8b, RESUME-SDK-c3d4`. |
| 5 | Native TUI resume / `codex --remote` | blocked | ? | Needs Michael's terminal. See "Blocked" below. |
| 6a | Interrupt active turn | pass | kept | Foreground `python3 -c 'time.sleep(45)'` (no approval needed under read-only sandbox). `turn/interrupt` -> `{}`; `turn/completed` with `status: "interrupted"`; the python child was gone; next turn completed normally (`AFTER-INTERRUPT-dd44`). |
| 6b | Interrupt while approval pending | pass | kept | `turn/interrupt` during a pending `requestApproval`: turn ended `interrupted`, server emitted `serverRequest/resolved` for the pending request itself (observed, matches docs). Our later `decline` reply produced no error frame. Next turn fine. |
| 6c | Stale `accept` after the request was cleared | pass (safe) | kept | Reply sent ~3 s after interruption: no error frame, no `item/*` activity, `int-probe.txt` was not created. The stale approval was silently ignored. |
| 6d | Owner (stdio client) SIGKILL mid-turn | pass | kept | App-server exited within 5 s of its client dying; the python sleeper was gone; rollout ends with `event_msg/turn_aborted`. Resume later worked (`OWNER-KILL-1`, `RECOVERED-ff66`). No unobserved extra turn was produced (contrast with Claude). |
| 6e | SIGTERM app-server mid-turn, then resume | pass | kept | Exit code 0 within ~30 ms; rollout has `turn_aborted`; `thread/resume` in a new process showed 1 prior turn and accepted new turns. |
| 7 | Multiple clients on one thread | blocked (test), documented | ? | Installed binary has `--listen ws://IP:PORT` and `unix://PATH`. Schema text: `thread/resume` on a running thread "rejoins" it; `thread/unsubscribe` is per connection; `thread/loaded/list` lists in-memory threads. The loopback two-client test (`probe-ws.mjs`) was denied by the session's permission classifier ("Expose Local Services"), so routing of approval requests across clients is untested here. |

## Observations worth keeping

- Wire format: newline-delimited JSON over stdio; server frames omit `"jsonrpc":"2.0"` (docs say so; observed on all 161 frames of one run). Server-originated request ids start at 0 per connection and are distinct from client ids.
- Startup noise: `remoteControl/status/changed` and `account/updated` notifications arrive after `initialize` regardless of threads.
- Turn lifecycle signals that were reliable: `turn/started`, `item/started`, `item/completed`, `turn/completed {turn.status: completed|interrupted|failed}`, `thread/status/changed {idle | active{activeFlags}}`. `activeFlags` is the explicit "input required" signal: `waitingOnApproval`, and per schema also `waitingOnUserInput`.
- `thread/start` response and `session_meta` label the thread `source: "vscode"` even though `originator` is `roundhouse-probe`. Unknown client names appear to be bucketed as vscode. Inference, not verified.
- Approval request params include `command`, `cwd`, `reason`, `commandActions`, `proposedExecpolicyAmendment` and an `availableDecisions` list (`accept`, `acceptWithExecpolicyAmendment`, `cancel`). Our probe answered with `decision: "accept"` and it was honoured.
- Rollout file (`~/.codex/sessions/YYYY/MM/DD/rollout-<ts>-<id>.jsonl`) entry types: `session_meta`, `turn_context`, `response_item/*`, `event_msg/*` (`task_started`, `task_complete`, `turn_aborted`, `item_completed`, `token_count`), `token_usage_record`, `world_state`. Writes land during the turn; a dead server leaves `turn_aborted` rather than a dangling turn.
- `thread/list {cwd}` and `thread/read {threadId, includeTurns}` work against stored rollouts without loading them (`status: notLoaded`). Discovery of externally created threads is therefore possible from any app-server process.
- The shared daemon (`~/.codex/app-server-daemon`, `codex agents`, `codex remote-control`) exists on this machine and is how the TUI/VS Code share threads. It was deliberately not used; multi-client behaviour there is untested.

## Documentation claims checked

- docs: "`thread/start` ... automatically subscribes you to turn/item events for that thread" — consistent with observation (no explicit subscribe call was needed).
- docs: "`turn/interrupt` ... success is `{}` and the turn ends with `status: "interrupted"`" — observed exactly.
- docs: pending `requestUserInput`/approval "cleared by turn start, turn completion, or turn interruption" emits `serverRequest/resolved` — observed for approval on interruption.
- docs: WebSocket transport "experimental and unsupported"; `codex app-server --listen ws://127.0.0.1:4500` + `codex --remote ws://127.0.0.1:4500` for the remote TUI — flags exist in 0.166.0 (`--listen`, `--remote`, `--remote-auth-token-env`); untested.
- schema comment on `thread/resume`: "If thread_id identifies a running thread, app-server rejoins that thread" — not exercised (needs two connections to one process).

## Blocked items and what to ask Michael

1. Native TUI resume of the managed thread (round-trip step 4-6): `cd /home/michael/Repos/q/research/roundhouse-fixtures/codex-rt1 && codex resume 01a1ad7d-899f-77c3-b17c-bc847cdb51a4` Expect the prior markers visible; send `List every marker token you have seen in this conversation so far, in order, then append TERMINAL-MARKER-e9d1.`; quit. Then rerun `node probe.mjs run --resume 01a1ad7d-899f-77c3-b17c-bc847cdb51a4 ...` and check the list includes `TERMINAL-MARKER-e9d1` (and `thread/read` shows the turn).
2. Live remote TUI against a managed app-server (needs a loopback listener, which this session's classifier denied). With approval: start `codex app-server --listen ws://127.0.0.1:45010` in the fixture dir (background), run `node probe-ws.mjs` or a plain client to `thread/start`, then Michael runs `codex --remote ws://127.0.0.1:45010`, opens the same thread, and alternates input while the probe holds an approval pending. Observe: who sees the approval, whether the TUI shows it, whether both can answer.
3. Resuming a thread that the TUI currently has open from a second app-server process (same hazard class as the Claude interactive-terminal finding): untested.

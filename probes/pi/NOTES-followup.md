# pi follow-up findings (2026-11-06)

Tested: `@earendil-works/pi-coding-agent` 1.1.0 (`pi` at `/home/michael/local/node/bin/pi`, Node v24.12.0, Ubuntu 26.04.1). Probe: `probes/pi/probe.mjs` with the new `--relay` owner mode plus `probes/common/relay.mjs` and `client.mjs`. Fixture: `/home/michael/Repos/q/research/roundhouse-fixtures/pi-rt1`, extension `.pi/extensions/roundhouse.ts` (now with a bounded 8 s dialog for commands containing `timed`). Model `openai-codex/gpt-6-astra` through the existing login, `--approve` for the fixture's project extension as before. Session ids per run: `logs/followup-session-ids.txt`. Logs: `logs/fu-*.jsonl` (gitignored).

Everything the relay does (snapshot of pending dialogs, replay from `since`, first answer wins, `answer_rejected`) is in-memory shim behavior in the owner process. pi itself offers one stdin/stdout client and no replay; nothing below claims crash durability.

## Results

| # | Scenario | Result | Observed |
| --- | --- | --- | --- |
| F1 | Reconnect during work: client answers the approval dialog, drops mid-command | pass | pi kept running the 20 s command after the client left; owner received `agent_settled aborted:false` and `RECONNECT-WORK-1`; fresh client with `since=0` replayed 66 events incl. the dialog, its resolution and `agent_settled`, without resubmitting the prompt. |
| F2 | Reconnect with a dialog pending | pass | Client left 3 s after the `select` appeared; 10 s later a fresh client's snapshot listed the dialog as pending (`state.phase: working`); it answered `Allow` once; the command ran (`tool_execution_end`), `RECONNECT-PENDING-1`. |
| F3a | Dialog resolved by another client while the first is away | pass | Second client answered `Block`; the reconnecting client's snapshot showed `pending=none`; its late answer got `answer_rejected: not_pending` (shim). A raw late `extension_ui_response` sent to pi for the already-answered id produced no frame, no error, no effect (harness ignores it silently). |
| F3b | Bounded dialog timeout while the client is away | pass | Request carried `timeout: 8000`; pi auto-resolved after 8.0 s with `undefined` (hook blocked the command: `blocked by approver (choice=undefined)`); pi emits no dedicated timeout event, so the owner marks the request resolved from its own timer; reconnecting client saw no pending request, late answer rejected. |
| F4 | Idle suspension and reopen | pass | Turn completed, stdin closed, pi exited (code 0) within 1 s, no child processes. Reopen with `--session <id>` and only `get_state` + `get_messages`: 3 messages restored, no `message_start` (no model call). Follow-ups `SUSPEND-RESUME-3` quoted the first user message verbatim; 7 messages, zero branch breaks. |
| F5a | Suspend with a dialog pending: cancel through the protocol | pass | `{"cancelled": true}` reached the extension as `undefined`; the hook blocked the command (`blocked by approver (choice=undefined)`); run settled normally (`CANCEL-1`); stdin close then exited cleanly. Cancelling a dialog denies the fixture operation and does not abort the run. |
| F5b | Suspend with a dialog pending: abort, then stdin close | pass (with caveat) | `abort` got no response while the dialog was pending (same as the earlier finding). Closing stdin anyway: pi exited with code 0 within 20 ms, no `agent_settled`, no further frames. Session file ends `user`, `assistant(toolCall)` with no `toolResult`. |
| F5c | Active command: stop (`abort`) vs wait-until-idle | pass | Stop: `abort` -> `success:true`, `agent_settled aborted:true`, the Python child was gone immediately; stdin close exited cleanly; file holds `toolResult` then an assistant message. Wait: `agent_settled` after 22 s (`ACTIVE-2`), close, exit; same roles in the file. No surviving process in either case. |

## Observations (installed behavior)

- Shutdown is stdin EOF. pi exited in under 1 s in every case, including with a dialog pending and an `abort` outstanding. It never needed a signal. The owner process in F5b stayed alive only because the probe itself was awaiting the dialog promise; that is a probe artifact, not pi behavior.
- A pending dialog blocks `abort` (no response) but not shutdown. The safe suspension order is therefore: answer `cancelled: true` (fixture operation denied, run settles), or simply close stdin and accept a dangling `toolCall`. Resume tolerated that dangling `toolCall` in the earlier investigation.
- `timeout` on `ctx.ui.select` works in RPC mode: the request frame carries `timeout` in ms, the agent side resolves `undefined` when it expires, and the client receives nothing to announce it. A client must either track the deadline itself or infer resolution from the following `tool_execution_end`.
- Late or duplicate `extension_ui_response` frames are ignored silently by pi (no error, no acknowledgement). Only the first answer has an effect; the shim's `answer_rejected` is the only visible signal.
- Reopening a session without a prompt is possible and does not start model work: `--session <id>` plus `get_state`/`get_messages` only. Readiness is the process being up with the session loaded; there is no separate "activate" step.
- Reconnect treatment differs by kind: outputs are replayable from the shim ring; a pending request must be shown from the snapshot (the harness will not resend it); a request's resolution must be learned from the shim (`request_resolved`) because the harness emits no event for an answered or timed-out dialog except the downstream tool result.
- Relay socket paths must be short; a path under the session scratchpad directory fails with `listen EINVAL`. Sockets were created under `/run/user/1000/roundhouse-probe/` and removed after each run.
- One run (first F5 attempt) was terminated from outside with exit 144 together with another probe; it was repeated. No `pkill`/`killall` was used here; leftover owner processes were stopped by their own pid.

## Not tested

- Terminal-mode dialog rendering of the timed dialog (needs Michael; the TUI shows a live countdown per the installed type comments).
- Owner crash with a dialog pending (the earlier investigation covers owner SIGKILL mid-command; the relay adds nothing durable).

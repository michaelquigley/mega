# pi findings (fork notes, 2026-11-05)

Tested: `@earendil-works/pi-coding-agent` 1.1.0 (`pi` at `/home/michael/local/node/bin/pi`, Node v24.12.0, Ubuntu 26.04.1). Probe: `probes/pi/probe.mjs`. Fixture: `/home/michael/Repos/q/research/roundhouse-fixtures/pi-rt1`. Sessions live in `~/.pi/agent/sessions/--home-michael-Repos-q-research-roundhouse-fixtures-pi-rt1--/<timestamp>_<uuid>.jsonl`.

Sources: the installed package ships its docs locally under `node_modules/@earendil-works/pi-coding-agent/docs/` (`rpc.md`, `rpc-commands.md`, `rpc-extension-ui.md`, `json.md`, `sessions.md`, `session-format.md`, `extensions.md`, `security.md`, `configuration.md`). These are version-matched and undated. The brief's GitHub leads point at `badlogic/pi-mono`; the installed package's repository field is `github.com/earendil-works/pi` (`packages/coding-agent`), and its docs link `rpc-types.ts` there. Treat the brief's URLs as stale leads.

## Provider note

The operator's default pi model is `ollama/qwen3.8-27b` and no Ollama server was listening on 127.0.0.1:11434, so the first managed run produced only provider errors (observation below). All passing runs use `--model openai-codex/gpt-6-astra`, the model the operator's Codex config already uses through the same ChatGPT login. `gpt-5.5` and `gpt-5.3-codex-spark` were rejected: "not supported when using Codex with a ChatGPT account". No new authentication or spending source was introduced.

## Results

| # | Scenario | Result | Continuity | Session |
|---|---|---|---|---|
| 1 | Managed persisted session, 2 marker turns, project marker from `AGENTS.md` | pass | n/a | `217e42ff…` |
| 2 | Explicit user question via extension tool (`ask_user` → `ctx.ui.select`) | pass | n/a | `217e42ff…` |
| 3 | Genuine approval (extension `tool_call` hook → `select`), agent waits for the answer | pass | n/a | `217e42ff…` |
| 4 | Clean shutdown, reopen exact session in a new RPC process: history, cwd, instructions, model, approval | pass | preserved | `217e42ff…` |
| 5 | Native TUI resume of the exact session, add a terminal turn, return to RPC | blocked (needs Michael) | untested | `217e42ff…` |
| 6 | Interrupt an active tool turn (`abort` during a 45 s Python sleep) | pass | preserved; next turn accepted | `3c1dff95…` |
| 7 | `abort` while an extension dialog is pending | partial: abort never answered until the dialog is answered | preserved after answer | `74645129…`, `e8663de9…` |
| 8 | Dialog never answered | fail-open hang: run blocks indefinitely; stdin close still shuts down cleanly | partial (user + toolCall persisted, no result) | `a8153d3f…` |
| 9 | Owner (probe) SIGKILLed mid-turn | pi exits at once; running command killed; turn not completed | user + toolCall persisted; resume works | `77c323dd…` |
| 10 | Resume after 9 | pass | prior turns visible, dangling toolCall tolerated | `77c323dd…` |
| 11 | Two RPC processes on the same session id concurrently | fail (no lock): both write the same file, forming two branches | diverged | `b448dddd…` |
| 12 | Provider unreachable (default Ollama model down) | observation | errors persisted as assistant messages | `fe9b8088…`, `35b89eba…` |
| 13 | Extension dialog in terminal mode vs RPC | blocked (needs Michael) | — | — |

## Observations (installed behavior)

Session identity and persistence.

- `pi --mode rpc --session-id <uuid>` creates the session with that id (stderr: "No project session found with id …; creating a new session with that id"). `get_state` returns `sessionId`, `sessionFile`, `model`, `isStreaming`, `messageCount`. The file header line is `{"type":"session","version":3,"id":…,"cwd":…}`.
- Reopening by exact id with `--session <uuid>` in a new process restored 16 messages, the same cwd, the model recorded by the session's `model_change` entry (no `--model` was passed on resume), and the `AGENTS.md` marker (it is persisted inside the system message's `project_context` section and was re-answered correctly). The approval dialog fired again on resume. Markers after the round trip: `ROUNDHOUSE-MARKER-7f3a, ALPHA-1a2b, BETA-3c4d, FIXTURE-TOKEN-91c2, GAMMA-5e6f, DELTA-7a8b, RPC-RETURN-c3d4`.
- Clean shutdown is closing stdin; pi exited with code 0 within about 40 ms in every run, including with a dialog still pending.

Signals.

- `prompt` response `data.disposition: "started"` is acceptance only. `agent_settled {aborted}` is the settle signal; `turn_end`/`agent_end` arrive earlier and can be followed by retries. Observed in the error run: each prompt produced `auto_retry_start` x3 (`errorMessage: "Connection error."`), then `agent_settled aborted:false`, with assistant messages persisted as `stopReason: "error"`. So "settled" does not mean "succeeded"; a client must read `message_end.message.stopReason`. In RPC mode the Ollama and Codex failures both surfaced only as "Connection error."; print mode gave the specific Codex message.
- Explicit input requests are exactly the `extension_ui_request` frames with `method` in `select | confirm | input | editor`. They carry an `id`, `title`, `options` and optional `timeout`. There is no other protocol-level question or approval record. A prose question from the model is an ordinary assistant text followed by `agent_settled`.
- Approval ordering: `tool_execution_start` for `bash` is emitted before the `extension_ui_request` from the `tool_call` hook (07.245 vs 07.246), and the command only ran after the answer (`tool_execution_update` at 11.258 after the answer at 11.250, `tool_execution_end` 3 s later). A client must not treat `tool_execution_start` as "running"; the approval dialog can still be pending.
- Fire-and-forget `notify` requests appear at session start (MCP server `mercurius` failing to connect, from the operator's user-level MCP config). RPC sessions load the user's MCP config and extensions unless `--no-mcp` / `--no-extensions` are passed.

Permissions.

- pi has no built-in approval system (`docs/security.md`: "it does not ask for confirmation before running tools"). Approval is an extension concern: a `tool_call` handler that calls `ctx.ui.confirm/select` and returns `{block: true}`. The `read` tool and `bash cat` ran with no prompt. The probe's gate is therefore test configuration, not harness policy, and nothing in the protocol identifies a dialog as a permission request versus any other dialog.
- Project-local extensions need project trust. RPC mode cannot show the trust prompt; with the default `defaultProjectTrust: "ask"` protected resources are skipped silently unless `--approve` is passed for the run (documented in `security.md`, used here). `AGENTS.md` loads regardless of trust. The operator's `~/.pi/agent/trust.json` was not modified.

Interruption and recovery.

- `abort` during an active bash tool: response `success:true`, `agent_settled aborted:true`, the child Python process was gone, the next prompt was accepted and answered.
- `abort` while a `select` dialog was pending: no response and no events for 118 s; the `tool_call` handler was blocked on `await ctx.ui.select`, and abort waits for idle. When the dialog was answered (the `--stale-after-cancel` run, 32 s later), the run immediately settled with `aborted:true` and the next prompt worked. So a dialog answer after an abort is not rejected; it is what releases the abort. The installed docs say a dialog with a `timeout` auto-resolves; the fixture dialog had none.
- Owner killed with SIGKILL while a 30 s command ran: pi was gone within 5 s (stdin EOF → shutdown), the command was killed, and the file holds `user` + `assistant(toolCall)` without a `toolResult`. `--session` resume accepted a new prompt and answered with the full marker list. Contrast with Claude Code, where the orphaned CLI finished the turn.
- Two processes on one session id: no lock, no error. The second process loaded the file mid-flight and attached its branch to the first process's user message; both kept appending to one file (14 lines, two branches). Each process answered from its own in-memory branch. Treat concurrent openers as a hard failure to prevent in Roundhouse, not a harness feature.

Multi-client.

- The RPC protocol is one stdin/stdout pair per process; there is no attach, subscribe, or second-client concept in `rpc.md`/`rpc-commands.md`. `docs/rpc.md` names `RpcClient` (TypeScript) and an in-process SDK as the alternatives. A pending `extension_ui_request` is only visible to the single client that owns the pipe; a client that reconnects to a new process cannot see it (and a new process cannot adopt a running one).

## Documentation claims (not verified here)

- `switch_session`, `fork`, `clone`, `get_tree`, `get_entries` commands exist for in-process session management (`rpc-commands.md`).
- TUI `/tree`, `/fork`, `/clone` branch semantics (`sessions.md`).
- `custom()`, `onTerminalInput()` and several `ctx.ui` methods are no-ops or unsupported in RPC mode (`rpc-extension-ui.md`); `ctx.hasUI` is `true` in RPC mode.

## Blocked: terminal steps for Michael

Run in `/home/michael/Repos/q/research/roundhouse-fixtures/pi-rt1`:

1. `pi --session 217e42ff-129d-442f-bda5-96b630a35338`. Expect the native project-trust prompt (the fixture has `.pi/extensions`); accept it for this fixture only, or decline and note that the approval extension then does not load. Check that the earlier markers (`ALPHA-1a2b` … `ZETA-1b2c`) are visible. Send: `List every marker token you have seen in this conversation so far, in order, then append TERMINAL-MARKER-e9d1.` Then send: `Using the bash tool, run exactly sleep 2 and reply TUI-APPROVAL-ok.` and note how the approval `select` dialog renders in the TUI. Exit with `/quit` (or Ctrl+C twice).
2. Afterwards I reopen the same id over RPC with `scenarios/recover.json`-style prompt and check that `TERMINAL-MARKER-e9d1` is in the list and that the file still has a single branch.
3. Optional: start a fresh `pi` session in the same directory, send `Reply with the project marker followed by NATIVE-ORIGIN-55aa.`, exit, and I discover it by scanning the session directory (there is no list-sessions RPC command; discovery is filesystem-based: `~/.pi/agent/sessions/--<cwd>--/`).

## Probe limitations

- `get_last_assistant_text` returns `{text}`; the probe prints it. Trimmed deltas in logs. The `messages` subcommand prints `AgentMessage` content types as the installed version emits them.
- The owner-kill case leaves no summary file (the process dies); use the `.jsonl` log and the session file.

# Harness capabilities: Claude Code, Codex, pi

Evidence-backed findings from the mega integration investigation (its brief, `docs/future/agent-workspace-investigation.md`, was retired on 2026-11-11 and lives in git history). Each scenario reports a result (`pass`, `partial`, `fail`, `blocked`), observed behavior, continuity, reproduction, and sanitized excerpts. Statements are tagged `[observed]`, `[docs]` (documentation claim, not verified here), or `[inference]`.

## Tested environment

| Item | Value |
| --- | --- |
| Host | `fortyfive`, Ubuntu 26.04.1 LTS, Linux 7.0.0-42-generic, x86_64 |
| Claude Code | 2.1.318, native install, `~/.local/share/claude/versions/2.1.318` (build 2026-11-03 per SDK manifest) |
| Claude Agent SDK | `@anthropic-ai/claude-agent-sdk` 0.3.318, installed locally under `probes/claude/` |
| Codex | codex-cli 0.166.0, `~/local/node/bin/codex` (npm `@openai/codex`) |
| pi | 1.1.0, `~/local/node/bin/pi` (npm `@earendil-works/pi-coding-agent`) |
| Runtimes | Node v24.12.0, Python 3.14.4, Go 1.27.1 |
| Auth | Operator's existing logins. Claude init reported `apiKeySource: "none"` (subscription login). No credentials copied. |
| Config sources present | `~/.claude/settings.json`, `~/.claude.json`, `~/.claude/CLAUDE.md`; `~/.codex/config.toml`; `~/.pi/agent/settings.json`. Contents not dumped. |
| Documentation | Five Claude Code pages fetched 2026-11-05 (headless, agent-sdk/sessions, agent-sdk/permissions, agent-sdk/user-input, remote-control). None shows a publication date. The installed `sdk.d.ts` was used as the version-matched schema. |

Fixtures: disposable git repositories under `/home/michael/Repos/q/research/roundhouse-fixtures/` (`claude-rt1`, `codex-rt1`, `pi-rt1`), each with `fixture.txt` (token `FIXTURE-TOKEN-91c2`) and a project instruction file carrying `ROUNDHOUSE-MARKER-7f3a`. The Claude fixture also has `.claude/settings.json` with `allow: ["Read", "Bash(cat *)"]`, `ask: ["Bash(sleep *)"]`, and two logging hooks.

## Claude Code

Probe: `probes/claude/probe.mjs` (see its README). All runs used `permissionMode: 'default'` so approvals reach the host.

### Integration surface `[observed]`

- Launch: `query({ prompt: AsyncIterable<SDKUserMessage>, options })` with `cwd`, `sessionId` (new) or `resume` (exact id), `canUseTool`, `pathToClaudeCodeExecutable`. The SDK spawns the installed CLI in stream-json mode; one `query` is one CLI process.
- Identity: `system/init` carries `session_id`, `cwd`, `model`, `permissionMode`, `claude_code_version`, `tools[]`, `mcp_servers[]` with status, `slash_commands`, `skills`, `plugins`, `output_style`. An `init` is re-emitted at the start of every turn; early inits can list fewer MCP tools because servers are still connecting (43 tools and 3 MCP servers on the first init, 51 and 7 later in the same session).
- Persistence: `~/.claude/projects/<cwd with non-alphanumerics replaced by '-'>/<session_id>.jsonl`. Entries carry `entrypoint` (`sdk-cli` for SDK turns, `cli` for terminal turns), `cwd`, `version`, `gitBranch`, `permissionMode`, and a `parentUuid` chain. Side entries include `queue-operation`, `last-prompt` (with `leafUuid`), `ai-title`, `session-owner`, `tools-used`, `attachment`.
- Discovery (SDK, filesystem based): `listSessions({dir})` returns `sessionId`, `firstPrompt`, `summary`, `cwd`, `gitBranch`, `createdAt`, `lastModified`; `getSessionInfo(id)`; `getSessionMessages(id, {dir})` returns the user/assistant chain. Live processes: `claude agents --json --all` lists interactive and background sessions with `pid`, `kind`, `cwd`, `sessionId`, `state`. The per-process registry `~/.claude/sessions/<pid>.json` (fields include `kind`, `status`, `cwd`, `sessionId`, `messagingSocketPath`) exists but is undocumented.
- Permission request: `canUseTool(toolName, input, { requestId, toolUseID, signal, suggestions, decisionReason, blockedPath, title, ... })`. Resolution is the return value `{behavior:'allow', updatedInput}` or `{behavior:'deny', message}`. `requestId` is a UUID distinct from `toolUseID`.
- Explicit user question: `AskUserQuestion` arrives as a `canUseTool` call with `toolName === 'AskUserQuestion'`; the host answers with `updatedInput: { ...input, answers: { [questionText]: label } }`. The tool result then reads `Your questions have been answered: "Which color do you prefer?"="Blue"`.
- Turn end: `result` message with `subtype` (`success`, `error_during_execution`, ...), `is_error`, `num_turns`, `stop_reason`, `permission_denials`, `total_cost_usd`, `usage`.
- Other stream signals seen: `system/task_started` and `system/task_notification` for background shell tasks, `system/hook_started` and `hook_response`, `rate_limit_event`, `system/thinking_tokens`. `system/session_state_changed` (`idle`/`running`/`requires_action`) is declared in `sdk.d.ts` but was never emitted in these runs.
- Control: `query.interrupt()` returned `{ still_queued: [] }`. Closing the input iterable ends the process when idle (about 0.4 s). `pending_permission_requests` on the `initialize` response and an initialize re-send for "a client joining an already-initialized session" are declared in `sdk.d.ts` `[docs]`, untested.

### Scenario results

| Id | Scenario | Result | Continuity |
| --- | --- | --- | --- |
| C1 | Managed launch, chosen session id, two marked turns, project marker | pass | n/a |
| C2 | Explicit question (`AskUserQuestion`) answered by host | pass | n/a |
| C3 | Real permission request (`Bash(sleep 3)`), delayed approval | pass | agent waited |
| C4 | Clean shutdown at idle | pass | transcript complete |
| C5 | Reopen exact session in native terminal, read history, add marked turn (Michael) | pass | same session id, same file |
| C6 | Reopen through SDK after the terminal turn | pass | all 8 tokens recited |
| C7 | Configuration across the transition | partial | see below |
| C8 | Adopt a session first created in the terminal | pass | full history |
| C9 | SDK resume while an interactive terminal holds the session open | fail (hazard) | same id, two branches |
| C10 | Background session (`claude --bg`), native attach, SDK access | partial | see below |
| R1 | Interrupt while a permission request is pending | pass | next turn accepted |
| R2 | Stale approval after the request was aborted | pass (ignored) | no side effect |
| R3 | Interrupt an active tool turn | pass | tool killed, next turn accepted |
| R4 | SIGKILL the SDK host while a tool runs | partial | CLI kept working unobserved, then exited; all persisted |
| R5 | Browser-equivalent disconnect and central-client restart | not run | needs a loopback shim; see open questions |

#### C1 to C4: managed session `edfe4ade-9f59-4db3-89bc-75fcc8b52c44`

Reproduction:

```
cd probes/claude
node probe.mjs run --cwd /home/michael/Repos/q/research/roundhouse-fixtures/claude-rt1 \
  --session-id <uuid> --permission-mode default --turns scenarios/rt1-managed.json \
  --log logs/rt1-managed.jsonl --approve '^Bash:.*"command":"(cat fixture\.txt|sleep 3)"' \
  --approve-delay-ms 4000 --answer '^Blue$'
```

Observed `[observed]`:

- Turn 1 answered `ROUNDHOUSE-MARKER-7f3a ALPHA-1a2b`: the project `CLAUDE.md` loaded with default `settingSources`.
- Turn 2 ran `cat fixture.txt` with no permission request and answered `FIXTURE-TOKEN-91c2 BETA-3c4d`. The project allow rule did not cause this: stderr said `Ignoring 2 permissions.allow entries from .claude/settings.json: this workspace has not been trusted. Run Claude Code interactively here once and accept the trust dialog, or set projects["<cwd>"].hasTrustDialogAccepted: true in ~/.claude.json`. The read-only command passed under built-in behavior `[inference]`.
- Turn 3: `AskUserQuestion` request `requestId 40dffd26-…`, `toolUseID toolu_01D1M5…`, answered `Blue`; reply `Blue GAMMA-5e6f`.
- Turn 4: `Bash sleep 3` request `requestId 4ff82e59-…` at 20:30:29.547Z, allowed at 20:30:33.551Z after the 4 s delay, tool result at 20:30:36.576Z, reply `DELTA-7a8b`. No assistant output arrived while the request was pending: the agent waits.
- Closing the input ended the CLI 0.47 s after the last `result`.

#### C5 and C6: terminal round trip

Michael ran `claude --resume edfe4ade-…` in the fixture directory, accepted the trust dialog, saw the four earlier exchanges rendered (including `User answered Claude's questions: Which color do you prefer? → Blue`), sent `TERMINAL-MARKER-e9d1`, and exited. The transcript gained entries with `entrypoint: "cli"` in the same file and chain. SDK re-entry with `--resume edfe4ade-…` and `scenarios/rt1-sdk-return.json` answered:

```
ROUNDHOUSE-MARKER-7f3a
ALPHA-1a2b
FIXTURE-TOKEN-91c2
BETA-3c4d
GAMMA-5e6f
DELTA-7a8b
TERMINAL-MARKER-e9d1
SDK-RETURN-c3d4
```

The project marker still answered, `Bash(sleep 2)` still produced a `canUseTool` request, cwd was unchanged, and the session id was unchanged. No fork occurred in either direction.

#### C7: what changed across the transition `[observed]`

| Aspect | SDK before terminal visit | After terminal visit |
| --- | --- | --- |
| Project permission rules | Ignored (workspace untrusted, stderr warning) | Loaded, no warning |
| Project `CLAUDE.md` | Loaded | Loaded |
| Project hooks (`SessionStart`, `UserPromptSubmit`) | Fire in SDK sessions with default `settingSources` (`.hook-log` written, `hook_started` events) | Same |
| `settingSources: ['user']` | No project hooks, no `CLAUDE.md` (`NO-MARKER`) | Same |
| User MCP servers | Connected in SDK sessions (7 configured, 3 connected, 4 pending or needs-auth) | Same |
| Auto-memory | Not created | Interactive visit created `<project dir>/memory/` |
| Permission mode in transcript | `default` (explicitly requested) | Terminal turns record no mode; `claude --bg` turns record `auto` |
| Trust state | `hasTrustDialogAccepted` absent | Set to `true` in `~/.claude.json` by the terminal visit |

The headless documentation says print mode loads project hooks and `.mcp.json` "even in a folder you've never trusted" `[docs]`; the installed version loads hooks but drops project permission rules until trust is granted interactively `[observed]`.

#### C8: adoption of a native-origin session

Michael created `aa205d40-0fba-4235-b974-680c0359e659` with a plain `claude` and one prompt, then exited. `listSessions({dir: <fixture>})` listed it with `firstPrompt` starting `Reply with the project instruction marker followed by NATIVE-ORIGIN-55aa`, which identified it without newest-session selection. SDK resume answered `ROUNDHOUSE-MARKER-7f3a / NATIVE-ORIGIN-55aa / ADOPTED-7e8f` and a `Bash(sleep 2)` approval behaved as in C3. Discovery, stored-history access and cold resumption all work; live control of the terminal that created it is a separate question (C9, C10).

#### C9: concurrent writers, the hazard

While Michael held `claude --resume aa205d40-…` open at the prompt, the SDK resumed the same id and completed a turn (`LIVE-PROBE-gg77`) with no refusal. Michael then sent a turn in the terminal; its reply listed the markers up to `ETA-3d4e` plus `TERMINAL-AFTER-SPLIT-hh88` and did not include `LIVE-PROBE-gg77`. The transcript now holds two branches from the same parent:

```
sdk-cli user 902a2d0d <- 79f7c733   "Reply with exactly LIVE-PROBE-gg77"
sdk-cli assistant f7db94a7           LIVE-PROBE-gg77
cli     user a513ca0a <- 79f7c733   "List every marker ... TERMINAL-AFTER-SPLIT-hh88"
cli     assistant 4b87d817           ... ETA-3d4e, TERMINAL-AFTER-SPLIT-hh88
```

`getSessionMessages` returned the terminal branch; a subsequent SDK `resume` continued the SDK branch (`... LIVE-PROBE-gg77, AFTER-SPLIT-SDK-ii99`) `[observed]`. Interactive sessions do not lock the transcript against a second writer, and readers choose branches differently `[inference: the resume appears to follow the latest `last-prompt` leaf, which the still-open terminal had not yet written]`. mega must treat "is a live process holding this session" as a precondition it checks itself (`claude agents --json`), not something the CLI enforces for interactive sessions.

#### C10: background sessions and native attach

- `claude --bg "<prompt>"` in the fixture directory printed `backgrounded · 0c61d24f` and started a local daemon ("Starting background service…"). It worked under the subscription login despite the help text's "(API key users only)" clause.
- `claude agents --json --all` listed it: `{"id":"0c61d24f","kind":"background","cwd":...,"sessionId":"0c61d24f-6bb5-…","state":"done","status":"idle","pid":1707351}`.
- `claude logs 0c61d24f` prints raw terminal output with ANSI sequences: the background session is a pty, not a structured stream.
- SDK `resume` of the running background session was refused: `Error: That session is running in the background (0c61d24f). Run claude attach 0c61d24f to open it, or claude stop 0c61d24f first to resume it here. Add --fork-session to branch off a copy instead.` This is the protective behavior that interactive sessions lack.
- Michael ran `claude attach 0c61d24f`, saw the earlier exchange, and sent a marker turn; the reply was `ROUNDHOUSE-MARKER-7f3a ATTACH-MARKER-cc33` (it omitted its own earlier `BG-MARKER-aa11`, a model choice rather than a history loss: the transcript chain is intact). Ctrl+Z detached and left it running.
- After `claude stop 0c61d24f` (`state: "stopped"`), SDK resume answered `ROUNDHOUSE-MARKER-7f3a / BG-MARKER-aa11 / ATTACH-MARKER-cc33 / BG-STOPPED-SDK-jj00`.
- Internals seen but not used: supervisor `~/.claude/daemon.status.json`, `~/.claude/daemon.lock`, control socket `/tmp/cc-daemon-1000/<id>/control.sock`, per-session `pty/<id>.sock` and `rv/<id>.sock`. These are undocumented and out of scope for a supported integration.

#### R1 to R4: interruption and recovery (fresh sessions, ids in `probes/claude/logs/recovery-session-ids.txt`)

- R1 `--interrupt-on-permission`: `interrupt()` was called while `canUseTool` for `Bash(sleep 5)` was pending. The request's `signal` aborted within 8 ms; the stream then carried a tool result `The user doesn't want to proceed with this tool use…` and `result error_during_execution, is_error: true, num_turns: 3`. The next prompt was accepted and answered normally.
- R2 `--stale-after-abort`: returning `{behavior:'allow'}` 1.5 s after the abort produced no error, no tool execution, and no stream message. The SDK drops the late answer silently; whether the CLI would reject a raw late `control_response` was not observed because the SDK never sent one `[inference]`. Do not rely on a native rejection to catch stale approvals.
- R3 `--interrupt-after-ms 15000` during a foreground `python3 -c 'import time; time.sleep(45)'`: `interrupt()` returned `{"still_queued":[]}` in 9 ms, the tool result was the same rejection text, the Python process was gone afterwards, and `result` was `error_during_execution` with `errors: ["[ede_diagnostic] result_type=user last_content_type=n/a stop_reason=tool_use"]`. The next prompt was accepted.
- R4 `--kill-owner-after-ms 10000` (process-owner termination, not machine power loss): the SDK host was SIGKILLed while a backgrounded `sleep 30` ran. The CLI did not exit: it stayed alive about 40 s, received the task notification, ran a further model turn (`OWNER-KILL-1`) that nobody consumed, persisted it, and then exited. A later SDK resume recited `ROUNDHOUSE-MARKER-7f3a / OWNER-KILL-1 / RECOVERED-ff66`. Terminating the owner therefore neither stops the harness promptly nor loses history, but it does allow unobserved model turns.
- Long hold: a `Bash(sleep 2)` permission request held for 150 s (`--approve-delay-ms 150000`) was still accepted; the tool ran and the turn completed (`LONG-HOLD-mm22`). No timeout or park message appeared on the stream in that window `[observed]`.
- Fixture note: the installed Bash tool blocks bare `sleep N` (`Blocked: standalone sleep 40. To wait for a condition, use Monitor…`); the model then reran it with `run_in_background: true`, and closing the SDK input while that task ran did not end the process until the task completed and a follow-up turn had run (E1 in the logs).

### Claude answers to the report questions

1. A faithful conversation surface without terminal parsing: yes `[observed]`. The stream carries user, assistant, tool use, tool result, question, permission and result events with ids; the on-disk transcript holds the same chain for history.
2. Signals: working = messages flowing after a prompt (no dedicated state event was emitted); explicit input required = `canUseTool` with `AskUserQuestion`; permission required = any other `canUseTool`; turn ended = `result`; failed = `result.is_error` with `subtype`; unknown = a process alive with no `result` and no pending request. A prose question is an ordinary assistant text with `stop_reason: end_turn`; the protocol does not distinguish it `[observed]`.
3. Cross-mode continuity in both directions: yes for the same id when the previous writer process has exited `[observed]`; an idle but open terminal is still a writer (C9). No history replacement, no fork, `entrypoint` records who wrote each turn.
4. Changes across the transition: trust state and project permission rules, auto-memory, permission mode; tools, instructions and MCP servers were equivalent; queued input and background tasks do not transfer (a background task finished inside the orphaned process in R4).
5. Live attachment: the native terminal can attach to a background session (`claude --bg` + `claude attach`), and the SDK is refused while it runs; the SDK cannot attach to a running terminal or background process through a supported interface. Supervising a `--bg` session structurally would require the undocumented daemon sockets, so `[blocked]` as a supported path. Version: 2.1.318.
6. Independently started terminal sessions: discovery `pass` (`listSessions`, `claude agents --json`), stored-history access `pass`, cold resumption `pass`, live control `fail` (no supported channel, and a hot resume forks the chain).
7. Multiple clients: one SDK host per CLI process was tested; the protocol declares multi-client semantics for hosted sessions `[docs]`, untested locally.
8. Recoverable from native history (owner-process termination only; whole-machine power loss was not tested and no durability claim is made for it): everything persisted up to the last `result`, including turns completed by an orphaned CLI. Lost or uncertain: in-flight approvals (cancelled on interrupt), background task state, and which branch a reader will follow after a concurrent write.
9. Terminal-controlled sessions observable by a local service: yes for history, by tailing the transcript file and `claude agents --json` for liveness; no supported live event channel.
10. Minimum promise for Claude: start, resume by exact id, question and approval handling, interrupt, and recovery from the transcript; visible differences: trust and permission rules until the directory has been opened interactively, no live attachment to terminal sessions, and the concurrent-writer hazard.

## Codex

Probe: `probes/codex/probe.mjs` (stdio JSON-RPC to a private `codex app-server` child), `probes/codex/probe-ws.mjs` and `probes/codex/hold-ws.mjs` (two clients on one loopback WebSocket app-server). Full fork notes: `probes/codex/NOTES.md`. Protocol shapes came from `codex app-server generate-json-schema` for 0.166.0. The operator's shared app-server daemon was not used.

Installed defaults observed: `approvalPolicy: on-request`, `sandbox: {type: readOnly, networkAccess: false}`, model `gpt-6-astra` (from the operator's config), ChatGPT login.

### Integration surface `[observed]`

- Launch: `initialize` + `initialized`, then `thread/start {cwd, approvalPolicy?, sandbox?}`; response carries `thread.id`, `thread.path` (rollout file), `model`, `approvalPolicy`, `sandbox`, `instructionSources` (the fixture `AGENTS.md` was listed). `thread/resume {threadId, cwd}` returns the stored turns.
- Persistence: `~/.codex/sessions/YYYY/MM/DD/rollout-<timestamp>-<thread id>.jsonl` with `session_meta`, `turn_context`, `response_item/*`, `event_msg/*` (`task_started`, `task_complete`, `turn_aborted`, ...). A dead server leaves `turn_aborted`, not a dangling turn.
- Discovery: `thread/list {cwd}` and `thread/read {threadId, includeTurns}` work against stored rollouts from any app-server process (`status: notLoaded`); `thread/loaded/list` shows threads loaded in that process.
- Turn lifecycle: `turn/started`, `item/started`, `item/completed`, `turn/completed {turn.status: completed|interrupted|failed}`, and `thread/status/changed {idle | active{activeFlags}}` where `activeFlags: ["waitingOnApproval"]` is the explicit input-required signal (schema also lists `waitingOnUserInput`).
- Permission request: server-originated JSON-RPC request `item/commandExecution/requestApproval` with `command`, `cwd`, `reason`, `availableDecisions`; answered with `{decision: "accept"}` etc. Ids start at 0 per connection. `serverRequest/resolved {threadId, requestId}` is emitted when any client answers or the turn ends.
- Explicit user question: `item/tool/requestUserInput` exists in the schema but never fired; server stderr said `request_user_input is unavailable in Default mode`. The model instead emitted an `agentMessage` with `delivery: "async"` and a `questions[]` array and polled with 60 s `sleep` items; the turn did not end on its own `[observed]`. The answer was accepted as the next `turn/start` after resume.
- Control: `turn/interrupt` returned `{}` and the turn completed with `status: "interrupted"`.
- Transport: the installed binary accepts `--listen ws://127.0.0.1:PORT` and `unix://PATH` (stderr: "binds localhost only"); the TUI accepts `--remote ws://...`. Documentation calls the WebSocket transport experimental and unsupported `[docs]`.

### Scenario results

| Id | Scenario | Result | Continuity |
| --- | --- | --- | --- |
| X1 | Start persisted thread, two marked turns, `AGENTS.md` marker | pass | n/a |
| X2 | Genuine approval (file write under read-only sandbox), delayed answer | pass | agent waited 4 s |
| X3 | Explicit user-input request | partial | `requestUserInput` unavailable in default mode; async question + sleep loop |
| X4 | Resume in a new app-server process | pass | 4 prior turns, same cwd, model, sandbox, policy, instructions |
| X5 | Native `codex resume <id>` in the TUI, add marked turn (Michael) | pass | history visible, same rollout |
| X6 | Second app-server `thread/resume` while the TUI holds the thread | pass (refused) | `thread 01a1ad7d-… already has an active writer` |
| X7 | Probe resume after the TUI turns | pass | all 12 tokens recited, single chain |
| X8 | Two clients on one loopback app-server, approval routing | pass | both receive the request; first answer wins |
| X9 | Native `codex --remote` TUI joins the managed thread with an approval pending (Michael) | pass | TUI showed and resolved the pending approval; probe clients saw everything |
| R1 | Interrupt active turn (Python sleep) | pass | `status: interrupted`, child killed, next turn fine |
| R2 | Interrupt with approval pending | pass | server emitted `serverRequest/resolved`; late decline ignored |
| R3 | Stale accept after the request was cleared | pass (ignored) | no error frame, command not executed |
| R4 | Owner (stdio client) SIGKILL mid-turn | pass | app-server exited within 5 s, `turn_aborted` persisted, no unobserved turn |
| R5 | SIGTERM app-server mid-turn, resume | pass | exit 0, `turn_aborted`, resume fine |

#### X5 to X7: terminal round trip

Michael ran `codex resume 01a1ad7d-899f-77c3-b17c-bc847cdb51a4` in the fixture, saw the earlier exchanges, and added `TERMINAL-MARKER-e9d1`. While the TUI stayed open, the probe's `thread/resume` from a private app-server was rejected: `thread/resume: thread 01a1ad7d-… already has an active writer` (the probe's app-server then exited 0). Michael added `TERMINAL-AFTER-CHECK-hh88` and quit. The probe then resumed and recited:

```
ROUNDHOUSE-MARKER-7f3a, ALPHA-1a2b, BETA-3c4d, FIXTURE-TOKEN-91c2, GAMMA-5e6f, DELTA-7a8b,
RESUME-SDK-c3d4, EPSILON-9f0a, ZETA-1b2c, TERMINAL-MARKER-e9d1, TERMINAL-AFTER-CHECK-hh88, SDK-RETURN-2-pp44
```

Codex enforces single-writer access across processes for a thread the TUI holds, which Claude Code does not do for interactive sessions `[observed]`.

#### X8 and X9: multiple clients and the native remote TUI

`hold-ws.mjs` started `codex app-server --listen ws://127.0.0.1:45011` (approved by Michael for this test, loopback only). Client A started a read-only thread and completed one turn; client B then joined with `thread/resume` (`priorTurns: 1`, status idle). A started a turn needing approval:

- Both A and B received `item/commandExecution/requestApproval` with the same id 0 at the same instant, and both saw `thread/status/changed … waitingOnApproval`.
- B answered `accept`; both received `serverRequest/resolved {requestId: 0}`, the command ran, both saw `item/completed` and `turn/completed`. A's duplicate `accept` 2 s later produced no error frame and no effect.
- A then started a turn whose approval (id 1) nobody answered. Michael connected `codex --remote ws://127.0.0.1:45011`, opened the thread, and the TUI showed the pending approval. He approved it; A and B saw `serverRequest/resolved 1`, the command ran (`ws-hold.txt` written), and the turn finished `WS-HOLD-1`. His follow-up prompt arrived at the probe clients as a `userMessage` item with the TUI's `clientId`, and its reply `REMOTE-TUI-nn33` was observed by both.
- The app-server stayed alive with the thread loaded after all probe clients disconnected (`serverAliveAfterAllClientsClosed: true`), and a new client could `thread/read` it.

Approval requests are therefore broadcast to every connected client, the first answer wins, duplicates are dropped silently, and the native TUI is a peer client on the same thread. Codex offers no exclusive input ownership; mega would impose it `[inference from X8/X9]`.

### Codex answers to the report questions

1. Faithful surface without terminal parsing: yes; items, deltas, approvals and status flags are all structured.
2. Signals: working = `thread/status/changed active` with empty flags; explicit input or permission required = `activeFlags` plus the server request; turn ended = `turn/completed` with status; failed = `status: failed` or `error` notification; prose question = ordinary `agentMessage` ending the turn, but in default mode a blocking question degrades to an async `agentMessage` with `questions[]` plus a sleep loop, which a client must recognize.
3. Cross-mode continuity: yes in both directions, single chain, and the TUI-held thread is protected by the active-writer check.
4. Changes across the transition: none observed in cwd, model, sandbox, policy or instructions; thread source is labelled `vscode` for unknown clients `[inference]`.
5. Live attachment: yes, via a shared app-server (`--listen` + `--remote`), experimental per docs; a TUI started on its own is not reachable from another app-server (X6 refusal).
6. Independently started TUI threads: discovery pass (`thread/list`), stored history pass, cold resumption pass once the TUI exits, live control only through a shared app-server.
7. Multiple clients: all receive requests, first answer wins.
8. Recoverable: everything up to the abort marker; a killed owner loses the in-flight turn but leaves a consistent rollout.
9. Terminal-controlled sessions observable: only when the TUI runs against a shared app-server; a standalone TUI leaves just the rollout file.
10. Minimum promise: start, resume, approvals, interrupt, recovery, and (experimental) live multi-client attach; differences: default-mode user-input requests are not blocking, WebSocket transport is unsupported upstream.

## pi

Probe: `probes/pi/probe.mjs` (JSONL RPC over stdio to `pi --mode rpc`). Full fork notes: `probes/pi/NOTES.md`. The installed package ships version-matched docs locally (`docs/rpc.md`, `rpc-commands.md`, `rpc-extension-ui.md`, `sessions.md`, `session-format.md`, `security.md`); the brief's `badlogic/pi-mono` URLs are stale leads for this package (`earendil-works/pi`).

Provider note: the operator's default pi model is `ollama/qwen3.8-27b` and no Ollama server was running, so passing runs used `--model openai-codex/gpt-6-astra` through the existing Codex login. No new authentication or spending.

### Integration surface `[observed]`

- Launch: `pi --mode rpc --session-id <uuid>` creates a persisted session; `--session <id|path>` reopens an exact one. `get_state` returns `sessionId`, `sessionFile`, `model`, `isStreaming`, `messageCount`. Session files: `~/.pi/agent/sessions/--<cwd with / replaced by ->--/<timestamp>_<uuid>.jsonl`, header `{"type":"session","version":3,"id":…,"cwd":…}`, entries linked by `parentId`.
- Turn lifecycle: `prompt` response `disposition: "started"` is acceptance only; `agent_settled {aborted}` is the settle signal; `message_end.message.stopReason` distinguishes success from `error`. `tool_execution_start` fires before any approval dialog, so it does not mean the command is running.
- Permissions: none built in (`security.md`: it does not ask for confirmation before running tools). The fixture's approval is a project-local extension (`.pi/extensions/roundhouse.ts`) whose `tool_call` hook calls `ctx.ui.select` and blocks on decline. Project extensions need project trust; RPC mode cannot show the trust prompt, so `--approve` was passed per run. `AGENTS.md` loads regardless of trust.
- Explicit input: only `extension_ui_request` frames (`select | confirm | input | editor`) with an `id`; nothing marks a dialog as a permission request versus any other dialog. A prose question is plain assistant text then `agent_settled`.
- Control: `abort` during a tool call returns `success: true` and settles `aborted: true`. `abort` while a dialog is pending gets no response until the dialog is answered.
- Clients: one stdin/stdout pair per process; no attach, subscribe or list-sessions in the protocol. Discovery is by scanning the session directory.

### Scenario results

| Id | Scenario | Result | Continuity |
| --- | --- | --- | --- |
| P1 | Managed persisted session, two marked turns, `AGENTS.md` marker | pass | n/a |
| P2 | Explicit question via extension tool (`ctx.ui.select`) | pass | n/a |
| P3 | Genuine approval via extension `tool_call` hook, delayed answer | pass | agent waited |
| P4 | Reopen exact session in a new RPC process | pass | 16 messages, cwd, model from `model_change`, instructions, approval dialog again |
| P5 | Native `pi --session <id>` in the TUI, marked turn and approval dialog (Michael) | pass | history visible; trust prompt accepted; dialog rendered |
| P6 | RPC reopen after the TUI turns | pass | all 12 tokens recited, single chain |
| P7 | Two RPC processes on one session id | fail (hazard) | no lock, two branches in one file |
| R1 | Interrupt active tool turn | pass | child killed, next turn accepted |
| R2 | `abort` while a dialog is pending | partial | abort unanswered until the dialog is answered; the late answer releases it |
| R3 | Dialog never answered | blocked run | run blocks indefinitely on the unanswered dialog (no `timeout` set); the gated command does not run; stdin close still exits cleanly |
| R4 | Owner SIGKILL mid-turn | pass | pi exits at once, command killed, `user` + `toolCall` persisted without result; resume works |

#### P5 and P6: terminal round trip

Michael ran `pi --session 217e42ff-129d-442f-bda5-96b630a35338` in the fixture, accepted the project-trust prompt (this modified `~/.pi/agent/trust.json`, native behavior), saw the earlier markers, added `TERMINAL-MARKER-e9d1`, and ran the `sleep 2` prompt, which rendered the extension's approval `select` dialog in the TUI and answered `TUI-APPROVAL-ok`. RPC reopen recited:

```
ROUNDHOUSE-MARKER-7f3a / ALPHA-1a2b / BETA-3c4d / FIXTURE-TOKEN-91c2 / GAMMA-5e6f / DELTA-7a8b /
RPC-RETURN-c3d4 / EPSILON-9f0a / ZETA-1b2c / TERMINAL-MARKER-e9d1 / TUI-APPROVAL-ok / RPC-RETURN-2-pp44
```

The session file shows one chain through the TUI turns. Two extra session files appeared in the fixture directory at 02:15:50Z and 02:16:21Z whose first user message is the `sleep 2` prompt; their origin was not verified (probably additional native sessions started during Step I) and they were not used.

### pi answers to the report questions

1. Faithful surface: yes for conversation and tool events; approvals exist only if an extension provides them.
2. Signals: working = events after `disposition: started`; explicit input = `extension_ui_request`; permission required = indistinguishable from any other dialog; turn ended = `agent_settled`; failed = `stopReason: error`; prose question = text then `agent_settled`.
3. Cross-mode continuity: yes in both directions once the previous writer process has exited; an open idle TUI still holds the file (P7).
4. Changes across the transition: project trust (TUI prompt vs `--approve` flag), extension availability depends on it; model restored from the session file.
5. Live attachment: none; one client per process.
6. Independently started sessions: discovery by directory scan, stored history yes, cold resume yes, live control no.
7. Multiple clients: not supported; concurrent openers double-write one file.
8. Recoverable: everything persisted up to the kill; dangling `toolCall` tolerated on resume.
9. Terminal-controlled sessions observable: only by tailing the session file.
10. Minimum promise: start, resume, dialogs, interrupt, recovery; differences: no native permission model, dialogs can hang forever, no concurrency protection.

## Cross-harness comparison

| Behavior | Claude Code 2.1.318 | Codex 0.166.0 | pi 1.1.0 |
| --- | --- | --- | --- |
| Structured session protocol | Agent SDK over stream-json | JSON-RPC app-server (stdio, experimental WebSocket) | JSONL RPC over stdio |
| Exact-id resume from structured side | yes | yes | yes |
| Native terminal resume of the same session | yes | yes | yes |
| Second writer while a terminal holds the session | allowed; silent fork in one file | refused ("active writer") | allowed; silent fork in one file |
| Explicit permission request | `canUseTool` with request id | `requestApproval` server request | extension dialog only |
| Explicit user question | `AskUserQuestion` via `canUseTool` | `requestUserInput` unavailable in default mode; async question | extension dialog only |
| Interrupt pending request | request aborted, turn errors, next turn fine | request resolved, turn interrupted, next turn fine | abort blocked until dialog answered |
| Stale answer after cancel | dropped silently (SDK) | dropped silently | releases the pending abort |
| Owner process killed mid-turn | CLI finishes work and a model turn unobserved, then exits | app-server exits within 5 s, `turn_aborted` | pi exits at once, dangling `toolCall` |
| Live multi-client attach | background daemon + pty attach only (undocumented sockets) | shared app-server, broadcast approvals, first answer wins | none |
| Discovery of foreign sessions | `listSessions`, `claude agents --json` | `thread/list`, `thread/read` | directory scan |

## Follow-up (2026-11-06): reconnect, suspension, Codex questions

Brief: `docs/future/harness-follow-up-probes.md`, retired on 2026-11-11 and kept in git history. Versions unchanged: Claude Code 2.1.318 with Agent SDK 0.3.318, codex-cli 0.166.0, pi 1.1.0. Same fixtures. New probe pieces: `probes/common/relay.mjs` and `client.mjs` (owner-side in-memory relay over a unix socket and a browser-equivalent client), `--relay SOCK [--hold]` owner mode in all three probes, `probes/claude/run-reconnect.sh`, `probes/claude/run-suspend.sh`, `probes/common/run-grouped.sh`, and the fork notes `probes/codex/NOTES-followup.md` and `probes/pi/NOTES-followup.md`. Everything the relay does (event replay from `since`, pending-request snapshot, first-answer-wins, `answer_rejected`) is shim behavior held in the owner's memory; it is not a harness guarantee and does not survive an owner crash. Observations below are `[observed]` unless marked.

### 1. Client disconnect and reconnect

| Scenario | Claude | Codex | pi |
| --- | --- | --- | --- |
| Client drops mid-command; owner keeps draining; fresh client replays from `since 0` without resubmitting | pass | pass | pass |
| Request pending while no client is attached; reconnecting client sees it in the snapshot, answers once, effect verified | pass (question) | pass (approval) | pass (dialog) |
| Request resolved while the client is away; reconnect shows nothing pending; late answer rejected | pass (interrupted) | pass (interrupted) | pass (answered by another client; and by an 8 s dialog timeout) |
| Harness reaction to a raw late response | not sent (SDK drops it) | ignored silently, no error frame | ignored silently, no frame |

Claude reproduction: `probes/claude/run-reconnect.sh` (session `44f17987-adbe-408c-8851-9148e18aae2e`). Excerpts:

```
client A  request c1c46db6-… permission -> answering {"decision":"allow"} -> disconnecting (simulated client loss)
owner     tool_result … slept ; assistant text: RECONNECT-WORK-1 ; result success   (no client connected)
client B  snapshot: state={"phase":"idle",…} pending=none seq=16 ; replay includes result "RECONNECT-WORK-1"
client C  prompt (AskUserQuestion) ; request 809ff908-… question ; disconnected stillPending=809ff908-…
client D  snapshot pending=[{id:809ff908-…, kind:question}] -> answers Blue -> assistant text: Blue RECONNECT-Q-2
client E  request 9333a7ee-… permission ; disconnected
client F  interrupt -> owner: canUseTool signal aborted -> request_resolved {outcome:"aborted", by:"harness"}
client G  snapshot pending=[] ; late allow -> answer_rejected {reason:"not_pending"}
client H  prompt -> RECONNECT-AFTER-4 ; close -> owner exit 0
```

What each harness contributes versus the shim:

- Outputs: ordinary stream messages on all three; replay is entirely the shim's ring buffer. Claude's `pending_permission_requests` on initialize `[docs: sdk.d.ts]` was not exercised; pi and Codex never resend a request.
- Requests: the pending set must be reconstructed by the owner. Claude signals resolution through the `canUseTool` abort signal; Codex emits `serverRequest/resolved {threadId, requestId}` on answer, turn end or interrupt; pi emits nothing for an answered, cancelled or timed-out dialog except the downstream `tool_execution_end`, so the owner must mark resolution itself (including from its own timer for `timeout`).
- Responses: first answer wins. Stale answers are dropped silently by the Codex server and by pi; the Claude SDK drops them before they reach the CLI. The only visible rejection is the shim's `answer_rejected`.
- Deadlines: no harness imposed a deadline on an unattended request in these windows (Claude 150 s earlier, Codex 6 s here, pi until answered), except pi's explicit dialog `timeout`, which resolves to `undefined` (the fixture hook then blocks the command).

Implication: outputs, requests and responses do need different reconnect treatment. A client that reconnects needs the owner's pending set and the resolution events, not just the event stream.

### 2. Clean suspension and reopening

| Case | Claude | Codex | pi |
| --- | --- | --- | --- |
| Idle: stop, verify exit, reopen without work, marked follow-up | pass (reopen = stored history only) | pass (`thread/resume` restores history and starts MCP, no turn) | pass (`--session` + `get_state`/`get_messages`, no model call) |
| Stop with a request pending, no approval | partial: stdin close did not end the process; the CLI aborted the request itself after 294 s and exited | pass: stdin close exits in 26 ms, rollout `turn_aborted` | pass: `{"cancelled":true}` denies the operation and the run settles; or stdin close exits in 20 ms leaving a dangling `toolCall` |
| Active command: stop (interrupt) then close | pass: command killed, `error_during_execution`, exit | partial: turn `interrupted` and server exit, but `codex-linux-sandbox` plus the Python child survived (52 s later) | pass: `abort` kills the child at once |
| Active command: wait until idle then close | pass (23 s) | pass | pass (22 s) |
| Grouped: two idle sessions from a manifest, stop, reopen, follow-up | pass (Claude + Codex) | | |

Claude details (`probes/claude/run-suspend.sh`, sessions in `probes/claude/logs/recovery-session-ids.txt`):

- Idle (`c72c5fa0-…`): probe exit 0; no CLI process and no `~/.claude/sessions` registry entry remained. Reopening with an empty turns file started a CLI that ran `SessionStart` hooks and exited before `init` without any model work, so the SDK offers no "load and wait" state: reading stored history is `getSessionMessages` with no process, and activating the conversation is the next prompt. The marked follow-up recited `ROUNDHOUSE-MARKER-7f3a / SUSPEND-IDLE-1 / SUSPEND-RESUME-2` on a single chain.
- Pending permission (`971e5f11-…`): the relay client sent `close` (SDK input ended) while `Bash(sleep 5)` awaited approval. The process stayed alive; 294 s later the request's signal aborted, the transcript recorded `tool_result: Tool permission request failed: AbortError: Tool permission stream closed before…`, no `result` message was emitted, and the CLI exited 0. The headless documentation says the prompt is cancelled "as soon as the input ends" `[docs]`; the installed version took about five minutes `[observed]`. Safe sequence for mega: interrupt (or deny) first, then close input.
- Active command (`d4d8ed86-…`, `709238b0-…`): interrupt ended the turn with `error_during_execution` and the owner exited within seconds; wait-until-idle took 23 s and ended with `success`. The first run's child-process check matched nothing because of shell quoting; the corrected rerun (`ONLY=active`, sessions `20e939fc-…` and `c75beb82-…`) showed the Python child present while the command ran (1) and gone after the interrupt (0) and after the natural completion (0), with no CLI process left in either case.

Codex (`probes/codex/NOTES-followup.md`): closing stdin is a safe cancel for a pending approval; `turn/interrupt` ends the turn but can leave the sandboxed tool process running, so a suspend must verify `codex-linux-sandbox` children itself or wait for `turn/completed`. `thread/resume` separates reopen (history, MCP readiness) from activate (a turn).

pi (`probes/pi/NOTES-followup.md`): shutdown is stdin EOF and completed within a second in every case, including with a dialog pending and an `abort` outstanding; a pending dialog blocks `abort` but not shutdown. Cancelling through the protocol (`cancelled: true`) denies the fixture operation and lets the run settle. Dialog `timeout` works in RPC mode but fires silently.

Grouped (`probes/common/run-grouped.sh`, manifest `probes/common/grouped-manifest.json`): Claude `996cede8-…` and Codex `01a1aec9-…` each completed one marked turn; both processes were gone within 1 s of the probes exiting (`claude agents --json` showed no live entry); the Claude history was read from the transcript without a process and the Codex thread was resumed with `priorTurns: 1` and zero `turn/started`; the follow-ups recited `GROUP-CLAUDE-1, GROUP-CLAUDE-2` and `GROUP-CODEX-1, GROUP-CODEX-2` on single chains; no process remained afterwards. This establishes the lifecycle facts only; nothing here is transactional.

Limits: process verification used `pgrep` by session or thread id and `claude agents --json`; sandboxed children (Codex) and background tasks (Claude `run_in_background` tools) are not enumerated by any supported interface and must be checked by process listing.

### 3. Codex questions and steering (codex-cli 0.166.0)

- Default-mode async question: the model emits an `agentMessage` item with `delivery: "async"` and `questions: [{title, options}]`, then sleeps in `sleep` items while the turn stays active. `turn/steer {threadId, expectedTurnId, input: [text "Blue"]}` returned the same `turnId`; the answer became a `userMessage` item inside the active turn, the sleep item ended early, and the reply was `Blue STEER-GAMMA-5e6f`. pass.
- Collaboration mode for `item/tool/requestUserInput`: blocked. The v2 schema's `ModeKind` is `"plan" | "default"`, but no client request or `thread/start`, `thread/resume` or `turn/start` parameter sets it; `collaborationMode` appears read-only on `ThreadResumeResponse`, `ThreadSettings` and `thread/settings/updated`. Feature flags reported `collaboration_modes removed` and `default_mode_request_user_input … under development, false`. `requestUserInput` therefore cannot be enabled through the installed protocol, and no mode recommendation follows for planning or implementation agents.

Implication: mega can treat a Codex `agentMessage` with `questions[]` as an explicit question and route the reply through `turn/steer` while the turn is active, falling back to `turn/start` after it ends.

### What the follow-up permits us to promise

- Reconnect: a browser-equivalent client can drop and rejoin without losing output or the pending request, provided the machine-local owner stays alive and keeps the event buffer and pending set. This is an owner-side promise; the harness supplies the resolution signals (Claude abort signal, Codex `serverRequest/resolved`, pi nothing beyond the tool result).
- Suspension at idle: all three harnesses stop cleanly and reopen the exact identity with history and no model work (Claude only as stored history until the next prompt).
- Suspension with a request pending: resolve or interrupt first on Claude (otherwise about five minutes until the CLI gives up); closing input is itself safe on Codex; on pi cancel the dialog or close input and accept a dangling tool call.
- Suspension during active work: wait for the turn to end when the result matters; interrupting is clean on Claude and pi but leaves a Codex sandbox child that must be checked separately.

Remaining blockers: no Codex collaboration mode exposes blocking questions in 0.166.0; the Claude five-minute pending-request exit is slow for an orderly suspend; owner-crash recovery of pending requests is untested on all three (the relay is in-memory).

## Operator decision (2026-11-06): early ownership discipline

Recorded separately from the technical findings. Michael has decided that operator care is sufficient for session ownership in the earliest mega versions: no leases, native-process exclusion, terminal wrappers, or cross-process locking for now. The operating procedure is explicit handoff: close the previous writer before opening another, where an idle terminal is still a writer. The concurrent-writer hazards above (Claude C9, pi P7) remain real and documented; stronger enforcement stays a possible later direction, not an immediate requirement. Tests must not resume a session still held by a different process; the Codex shared app-server multi-client case is a distinct, intentional configuration.

## Follow-up (2026-11-09): Claude permission modes and auto mode

Probed for the auto-mode roadmap card, the one sanctioned reopening of the Claude investigation. Claude Code 2.1.318 (`~/.local/share/claude/versions/2.1.318`; `claude` on `PATH` was 2.1.321 and was not used), Agent SDK 0.3.318, fixture `claude-rt1`, whose `.claude/settings.json` allows `Read` and `Bash(cat *)` and asks for `Bash(sleep *)`. The operator's settings name no `permissions.defaultMode`. `probes/claude/probe.mjs` gained a per-turn `set_mode` (calls `setPermissionMode` before the prompt) and `--set-mode-on-permission MODE` (calls it while the first permission request is pending). The probe allows every Bash call that reaches `canUseTool`. Logs are `probes/claude/logs/auto-mode*`, `mode-omitted*`, and the fixture transcripts named below.

| Run | Launch | Scenario | Result |
| --- | --- | --- | --- |
| A | `permissionMode: 'auto'` | `scenarios/auto-mode.json` | pass: the classifier runs under the SDK |
| B | `'default'`, then `setPermissionMode('auto')` between turns | `scenarios/auto-switch.json` | pass |
| C | `'default'`, then `setPermissionMode('auto')` while a request is pending | `scenarios/auto-switch-pending.json` | pass |
| D | option omitted | `scenarios/mode-omitted.json` | starts in `default` |

- Auto mode is available headless under the SDK in stream-json mode for this account `[observed]`. In run A (session `c94fb593-0bf2-42fd-ab7b-d8c4e91d24b9`), `system/init` reported `permissionMode: "auto"`.
- `system/init` is emitted again at the start of every turn, not only at launch, each time with the current mode `[observed]`. It is not emitted mid-turn.
- Which calls reach `canUseTool` in auto mode `[observed]`:
  - `wc -l fixture.txt` and `touch /tmp/rh-auto-probe-outside.txt` (a write outside the checkout) never reached `canUseTool`. The transcript records each as `permissionDecision: {decision: "accept", source: "config", reasonType: "classifier"}`.
  - `sleep 3`, covered by the project `ask` rule, still reached `canUseTool`, with no `decisionReason`. Its transcript entry is `{source: "user_temporary", ruleSource: "projectSettings", reasonCode: "rule:projectSettings"}`.
  - So an `ask` rule still produces a request in auto mode, and the classifier answers only what no rule settles. A denial by the classifier was not observed: every call the classifier saw, it approved.
- What the transcript records `[observed]`:
  - Each user prompt entry carries `permissionMode` (`"auto"` or `"default"`).
  - Tool results carry `permissionDecision` as above, and assistant entries carry `serverClassifierRequest` ids.
  - An `auto_mode` attachment is added to the context (the "While auto mode is active" reminder).
  - No separate classifier API request appears among the transcript's `api-request` entries (7 entries for 7 model steps).
- In `default` mode, a read-only command such as `wc -l` is accepted by Claude itself without asking (`reasonType: "subcommandResults"`) `[observed]`. A call that does not ask in `default` therefore proves nothing about the classifier.
- Run B (session `11ed775f-1d9a-494b-8600-97c9681c4749`): `setPermissionMode('auto')` between turns resolved at once, the next turn's `system/init` reported `auto`, the next prompt entry recorded `auto`, and the next call was decided with `reasonType: "classifier"` `[observed]`.
- Run C (session `a3dc8dc7-362d-4e38-8efa-ffa5f9374cfa`), with the switch made while a request was pending `[observed]`:
  - The pending request was not aborted: its signal had not fired two seconds after `setPermissionMode` resolved.
  - The operator's answer still applied (`source: "user_temporary"`).
  - The next tool call in the same turn (`touch /tmp/rh-auto-probe-b.txt`) went to the classifier without asking.
  - No `system/init` was emitted until the next turn, so the stream does not report a mid-turn change.
  - The switch therefore takes effect from the next tool call and leaves pending requests alone.
- Run D (session `29f0751a-cd75-4f4c-af1c-afa6131d768c`): with `permissionMode` omitted and no `defaultMode` in any settings, `system/init` reported `default` `[observed]`. The SDK's documentation says an omitted mode falls back to `auto` where available `[docs]`; this version, with these settings, did not. mega still passes `default` explicitly, so a later version cannot change it silently `[inference]`.

### Where Claude reads project settings for a session below the repository root (2026-11-09)

Claude Code 2.1.318, SDK 0.3.318. Each probe is a one-turn `scenarios/mode-omitted.json` run with no `permissionMode` on `claude-rt1`, which is a git repository and trusted. A temporary `sub/` directory and temporary settings were restored afterwards; the fixture's `.claude/settings.json` was put back byte for byte. The mode is what `system/init` reported. Logs are `probes/claude/logs/nested-*`.

| Probe | Setting | Session started in | Mode | Session |
| --- | --- | --- | --- | --- |
| P1 | root `.claude/settings.local.json` names `plan` | `sub/` | `plan` | `95856cc0-dc17-477e-9445-9bcc50f48c18` |
| P2 | `sub/.claude/settings.json` names `plan` | `sub/` | `plan` | `a55e533f-47a9-4ef5-893d-cd552054f937` |
| P3 | root `.claude/settings.json` names `plan` | `sub/` | `default` | `53323da2-e8cd-418b-8f78-df08cade4e5d` |
| P3 control | root `.claude/settings.json` names `plan` | root | `plan` | `76e90876-6672-4171-b384-c31b37d17daa` |
| P4 | root `.claude/settings.local.json` names `auto` | root | `default` | `fdf6461f-b2aa-46d4-a03d-cd0c62c541ad` |

- The local file is read at the repository root for a session below it (P1) `[observed]`.
- The shared `settings.json` is read only from the session's directory: the root's is not read from below it (P3), and the session directory's own is (P2) `[observed]`.
- `auto` named in local settings does not take effect (P4) `[observed]`.
- All three match the settings documentation `[docs]`:
  - the local file sits at the repository root since 2.1.211, and is still also read from the starting directory, the root winning;
  - it stays in the starting directory outside a repository, when the root is the home directory, in a worktree (where the main checkout's root is used), or when the root, `.git` or `.claude` is not owned by the user;
  - `auto` and `bypassPermissions` in `defaultMode` don't take effect from project or local settings.

## Follow-up (2026-11-09): Codex MCP tool approvals and elicitations

codex-cli 0.166.0, through `probes/codex/probe.mjs`, which gained `--elicit`: a `;`-separated plan of answers to successive `mcpServer/elicitation/request` frames. The fixture `codex-rt1` gained a dependency-free stdio MCP server, `mcp/fixture-server.mjs`. Its `echo {text}` tool returns `echo: <text>` and appends to `mcp/mcp-calls.log`; its `pick_color` tool raises a typed form elicitation. It is named in a project `.codex/config.toml` as `[mcp_servers.fixture]`; the checkout is already trusted in the operator's config. The full schema bundle was regenerated with `codex app-server generate-json-schema --experimental`. It types the request's `_meta` as `true` and the response's `_meta` only as "optional client metadata", with no `persist` field anywhere, so the persistence shape below comes from the probe. Logs are `probes/codex/logs/mcp-*`.

| Run | Scenario | Answers | Result |
| --- | --- | --- | --- |
| S1 | `mcp-accept-decline.json` | `accept`, then `decline` | the first call ran; the second did not, and its item failed |
| S2 | `mcp-session.json` | accept with `_meta: {persist: "session"}` | the second call was not asked again, and ran |
| S3 | `mcp-interrupt.json` | interrupt while pending, then a stale accept | resolved by the server; the tool never ran |
| S4 | `mcp-form.json` | accept for the session, accept `pick_color`, then a form answer | session approval is per tool; the form content reached the server |
| S5 | `mcp-always.json` | accept with `_meta: {persist: "always"}` | written into the config that defines the server |

- The approval request `[observed]`. `mode` is `"form"`, with an empty `requestedSchema` (`{type: "object", properties: {}}`) and `message: "Allow the fixture MCP server to run tool \"echo\"?"`. `params` also carries `threadId`, `turnId` and `serverName: "fixture"`, and `_meta`:
  - `codex_approval_kind: "mcp_tool_call"`;
  - `persist: ["session", "always"]`;
  - `tool_description` (the tool's own description);
  - `tool_params: {text: "MCP-ALPHA"}`;
  - `tool_params_display: [{name: "text", value: "MCP-ALPHA", display_name: "text"}]`.
- `{action: "accept"}` with no `content` runs the tool, and the `mcpToolCall` item completes with its result (S1) `[observed]`.
- `{action: "decline"}` stops it: the item completes `failed`, with `error.message: "user rejected MCP tool call"`, and the model went on (S1) `[observed]`. The response has no message field, so a deny reason cannot travel `[observed: schema]`.
- "Approve for this session" is `{action: "accept", _meta: {persist: "session"}}` (S2) `[observed]`:
  - the next call to the same tool in the same thread was not asked again, and ran;
  - no config file changed;
  - the approval is per tool: another tool on the same server was still asked (S4).
- "Always" is `{action: "accept", _meta: {persist: "always"}}`, run under a scratch `CODEX_HOME` so the operator's own config could not change (S5) `[observed]`:
  - it ran the tool and wrote `[mcp_servers.fixture.tools.echo] approval_mode = "approve"` into the config file that defines the server, here the project's `.codex/config.toml`;
  - neither the scratch user config nor the operator's own changed;
  - for a server defined in the operator's `~/.codex/config.toml`, the same entry would presumably land there `[inference]`;
  - the fixture's config was restored at once.
- `turn/interrupt` with an elicitation pending (S3) `[observed]`:
  - the turn completed `interrupted`, and `serverRequest/resolved` for the elicitation's id followed 1 ms later;
  - a stale accept sent 3 seconds afterwards got no reply and did not run the tool.
- A typed form elicitation from the MCP server itself (S4) `[observed]`:
  - it arrives with `mode: "form"`, `_meta: null`, the server's own message ("Which color do you prefer?"), and the real `requestedSchema`: `color` a string enum `["Red", "Blue"]`, `note` a string, `required: ["color"]`;
  - `{action: "accept", content: {color: "Blue", note: "..."}}` delivered exactly that content to the server.

## Observation (2026-11-10): the history reader synthesizes user messages the live stream never delivers

Claude Code 2.1.318, Agent SDK 0.3.318, seen in an operator conversation and in its transcript; no probe was run.
- A subagent's hand-back reaches the main session as a queued command `[observed]`. The transcript records a `queue-operation` (enqueue, then remove with reason `absorbed_mid_turn` and a `commandUuid`) and an `attachment` of type `queued_command`.
- It is never streamed live as a user message `[observed]`.
- `getSessionMessages` returns it as an SDK user message `[observed]` with:
  - `is_meta: true` and `isQueuedCommand: true`;
  - string content beginning `<agent-message from=…>`;
  - an `origin` block: `kind: "peer"`, `handback: true`, `name` (here `web-fetch`), `from`, `senderTaskId`, and the report as `body`.
- The SDK's `SDKUserMessage` type declares `origin` but not `is_meta` or `isQueuedCommand` `[docs]`.
- Across the operator's logs on 2026-11-10 `[observed]`, native-origin user events break down as:
  - 227 without either flag;
  - 2 with `isQueuedCommand` alone: prompts typed ahead while a turn ran, the operator's own;
  - 2 with `is_meta`: both this hand-back case.
- So a history import can contain user messages that are not prompts, and only `is_meta` tells them apart `[inference]`.

## Follow-up (2026-11-10): model and effort

### Claude Code 2.1.318, Agent SDK 0.3.318

`probes/claude/models.mjs`, on `claude-rt1`. Logs are `probes/claude/logs/models-*`.

| Run | Launch | Turns | Result |
| --- | --- | --- | --- |
| inherit | no model or effort | marker, after `setModel('sonnet')`, after `applyFlagSettings({effortLevel: 'low'})` | `system/init` reported `claude-opus-5-5`, then `claude-sonnet-5-5` twice |
| explicit | `model: 'haiku'`, `effort: 'low'` | the same three | `claude-haiku-5-5`, then `claude-sonnet-5-5` twice |
| idle only | no prompt | none | the model list answered, and the session left nothing behind |

- `supportedModels()` answers on an idle session in streaming input mode before any prompt, in 395 to 405 ms from `query()` `[observed]`. `initializationResult()` then answers at once, and also carries a `models` list.
- The list held 14 entries for this account `[observed]`. Each has `value`, `resolvedModel`, `displayName`, `supportsEffort` and `supportedEffortLevels`:
  - aliases first: `default` (Default, recommended, resolving to `claude-opus-5-5`), `opus`, `sonnet`, `haiku` and `fable`;
  - then explicit ids;
  - every entry offers `low` to `max` except `claude-opus-4-6` and `claude-sonnet-4-6` (no `xhigh`) and `claude-haiku-4-5-20251001` (no effort at all).
- The idle start costs no turn `[observed]`. The CLI starts, answers, and closes after about 1.1 s when input closes with no prompt. That session, launched under a known id, left no transcript under `~/.claude/projects` and did not show in `listSessions` for the checkout, so a models query does not pollute discovery.
- The `model` launch option is applied and reported by `system/init` (`claude-haiku-5-5` for `haiku`) `[observed]`.
- `setModel('sonnet')` resolved in 5 to 21 ms, and the next turn's `system/init` reported `claude-sonnet-5-5` `[observed]`. The assistant message's `model` agreed. `system/init` reports the resolved id, not the alias.
- `applyFlagSettings({effortLevel: 'low'})` was accepted, resolving in 22 to 23 ms `[observed]`.
- Effort is never reported on the stream `[observed]`. `system/init` carried no `effort` field in any turn, launched or switched; the type declares one only for Remote Control bridge frames `[docs]`. No message in any run mentioned effort, so the effective effort cannot be read from the stream.

### codex-cli 0.166.0

`probes/codex/probe.mjs`: a new `models` command, and turn-file items may carry `turn_json`. Logs are `probes/codex/logs/models-list.*`, `effort-override.*` and `model-override.*`.

- `model/list` on a thread-less app-server answered in 3 ms `[observed]`. It returned 8 models, with no next cursor. Each has `id` (equal to `model`), `displayName`, `isDefault`, `hidden`, `defaultReasoningEffort` and `supportedReasoningEfforts`: `low` to `max` everywhere, plus `ultra` on six of them.
- The catalog's `isDefault` model (`gpt-6.1-sol`) is not what the operator's config chooses `[observed]`. `~/.codex/config.toml` sets `model = "gpt-6-astra"` and `model_reasoning_effort = "high"`, and a thread started without a model ran `gpt-6-astra` at `high`. So `isDefault` does not say what "inherit" means.
- `config/read {cwd}` returns the effective config for the checkout, project layers included: `model: gpt-6-astra`, `model_reasoning_effort: high`, in 15 ms `[observed]`. That is the cheap, exact answer to what "inherit" means.
- The `thread/start` response reports `model` and `reasoningEffort`: `gpt-6-astra` and `high` with none passed; `gpt-6-luna` and `high` with `model: gpt-6-luna` passed `[observed]`.
- A `turn/start` override of `effort` (or of `model` and `effort`) produces a `thread/settings/updated` notification before the turn, with `threadSettings.model` and `threadSettings.effort` (`gpt-6-astra` and `low`) `[observed]`. `turn/started` and the turn itself carry neither.
- A later `turn/start` without overrides produced no update and kept the override, as documented ("this turn and subsequent turns") `[observed]`.
- The override also survives `thread/resume` `[observed]`. Resuming the overridden thread (`01a1c625-07ca-74c0-a31b-38df6ad16fd3`) with no model or effort reported `gpt-6-astra` at `low`, not the config's `high` (`logs/resume-after-override.*`). Once a thread has had an override, passing nothing keeps that override; only sending the config's values explicitly returns to them `[inference]`.

## Follow-up (2026-11-10): background jobs

Work an agent keeps running after its turn ends: what each harness reports about it on the stream mega reads.

### Claude Code 2.1.318, Agent SDK 0.3.318

`probes/claude/background.mjs`, on `claude-rt1`. Logs are `probes/claude/logs/background-{bash,two,agent,close}.*`. The scenarios were one backgrounded Bash call, two at once, one backgrounded subagent, and one Bash call with input closed while it ran.

- `system/background_tasks_changed` is emitted on every change, each a whole snapshot of what is still running, down to an empty list `[observed]`. With two jobs the snapshots were `[A]`, `[A, B]`, `[B]`, `[]`.
- Each task is `{task_id, run_id, task_type, description}`, and a subagent adds `subagent_type` `[observed]`. No task, snapshot or `task_started` frame carries a start time. `task_updated`'s `patch.end_time` is the only time, and only at the end.
- A backgrounded Bash call has `task_type: local_bash`, with the call's `description` as its description `[observed]`. Its tool result returns at once ("Command running in background with ID: …").
- A backgrounded subagent appears as a task with `task_type: local_agent` and `subagent_type: general-purpose` `[observed]`. Its reply reaches the stream as an `assistant` message with `parent_tool_use_id` set, between turns.
- When a job finishes after the turn ended, the stream sends `task_updated` (`patch.status: completed`), `task_notification` (`status`, `summary`, `output_file`), then the new `background_tasks_changed` snapshot `[observed]`.
- That completion then wakes the agent: a fresh `system/init`, an assistant reply and a `result` follow, a turn nobody prompted `[observed]`. No `user` message comes on the stream. Each job's completion woke its own turn.
- Closing input while a job runs does not end the session promptly `[observed]`. The CLI waited for the job to finish (about 12 s), the completion woke one more full turn (here a `Read` and a reply), and only then did the process exit. A release with background jobs therefore lasts until they finish and the agent has taken that turn.

### codex-cli 0.166.0

`probes/codex/background.mjs`, on `codex-rt1`. Logs are `probes/codex/logs/background{,-session}.*`. The generated `--experimental` schema bundle was searched for anything about background work.

- Codex has background terminals: commands kept running as unified-exec terminal sessions (`unified_exec` is `stable`, on). The request `thread/backgroundTerminals/list {threadId}` returns `{command, cwd, itemId, processId, osPid?, cpuPercent?, rssKb?}` per terminal, with `terminate` and `clean` beside it. No server notification announces a change.
- Asked only to run a command "in the background", the model detached it with `nohup … &` inside a one-off shell call `[observed]`. That call completed at once, the background-terminal list stayed empty, and nothing reported the detached process. Work detached that way is invisible to the app-server.
- Asked to run the command directly with a short yield, so it keeps running as a terminal session, the stream showed the following `[observed]`:
  - `item/started` (`commandExecution`, `status: inProgress`, a `processId`), then the reply and `turn/completed`, with no `item/completed` for the command;
  - `thread/backgroundTerminals/list` held the command (`sleep 30 && echo BG-CODEX-DONE`, without the `/bin/bash -lc` wrapper) until it exited;
  - `item/completed` for it arrived about 27 s after the turn's end, outside any turn, and the list then emptied;
  - nothing woke a new turn.
- So on the stream, a command whose `item/started` has no `item/completed` by its turn's `turn/completed` is background work until that `item/completed` arrives `[observed]`. mega derives the Codex background list from that, with no polling. Before this batch the adapter mapped that late `item/completed` to tool events, which made the service open a turn that never ended; it is now recorded as the command's call and result, opening no turn, its result unread at once, under the identities a history import gives the same item.

## Follow-up (2026-11-10): `@path` in a programmatic prompt

Whether a prompt sent the way mega sends it treats `@path` as a file mention, as the terminal does. This decides what the reply box's path search inserts. Both probes ask for the exact contents of `fixture.txt` and forbid tools. The model can therefore only answer if the mention attached the file, and otherwise replies `NOT-ATTACHED`.

### Claude Code 2.1.318, Agent SDK 0.3.318

`probes/claude/mention.mjs`, on `claude-rt1`, as a streaming-input `query`. `Read`, `Bash`, `Glob`, `Grep`, `Agent` and `Task` are disallowed and every permission request is denied. Logs are `probes/claude/logs/mention{,-plain}.*`.

- With `@fixture.txt` in the prompt, the reply was the file's exact content, with no tool call `[observed]`. The mention attached the file, as in the terminal.
- The attachment is not visible on the SDK stream `[observed]`. Between `system/init` and the assistant reply, no message carries the file's content. It reaches the model inside the CLI.
- The control, the same prompt naming `fixture.txt` without the `@`, replied `NOT-ATTACHED` `[observed]`. So the `@` is what attaches it.

### codex-cli 0.166.0

`probes/codex/mention.mjs`, on `codex-rt1`: two `turn/start`s in one thread, every approval declined. The log is `probes/codex/logs/mention.*`.

- `@fixture.txt` inside a `text` input arrives literally `[observed]`. The stored `userMessage` is the text as sent with empty `text_elements`, no command ran, and the reply was `NOT-ATTACHED`.
- The v2 `UserInput` union also has a `mention {name, path}` item. Passing `{name: "fixture.txt", path: <absolute path>}` beside the text did not attach the file either `[observed]`: it was stored as a `mention` item and the reply was `NOT-ATTACHED`. What that item is for was not established here.
- The terminal's `@` is a file-search picker. What it sends was not observed in this probe.

### A path with spaces (Claude Code 2.1.318, Agent SDK 0.3.318)

The same probe, with `--file 'spaced name.txt' --form bare|quoted|escaped`. A temporary untracked `spaced name.txt` was written in `claude-rt1` and removed afterwards. Logs are `probes/claude/logs/mention-spaced-{bare,quoted,escaped}.*`.

- `@spaced name.txt`, bare, did not attach `[observed]`. No tool ran, and the reply was `NOT-ATTACHED`.
- `@spaced\ name.txt`, the space backslash-escaped, did not attach either `[observed]`.
- `@"spaced name.txt"`, quoted, attached `[observed]`: the reply was the file's exact content, with no tool call.

### Consequence

The path search keeps the leading `@` for Claude, where it attaches the file, and quotes a path that contains whitespace (`@"a b.txt"`), the one form that attaches it. For Codex it inserts the bare relative path, spaces and all, which the agent reads with its own tools under its own permissions.

## Follow-up (2026-11-10): a runtime started with no prompt

What a resumed session does when nothing is sent, for the Start control. Both probes seed a fresh session with one short turn, resume it with no prompt, and hold it idle. Throughout, they record every message, the transcript or rollout file, and the memory of every process the runtime started. They then close input and time the exit. Memory is resident set size (RSS) and proportional set size (PSS) from `/proc`; PSS divides shared pages among the processes sharing them, so it is the fairer cost per runtime.

### Claude Code 2.1.318, Agent SDK 0.3.318

`probes/claude/idle-resume.mjs`, on `claude-rt1`. Logs are `probes/claude/logs/idle-resume{,-prompt,-hook-none,-hook-plan}.*`. The first run held for 300 s with a stdio MCP server passed for that query only (the `codex-rt1` fixture server). A second run resumed the same session for 30 s, switched the mode while idle, and then sent one prompt. Two short runs registered an SDK `SessionStart` callback.

- A resumed session with streaming input and no prompt holds idle for the whole hold `[observed]`. Nothing arrives on the stream except the checkout's own `SessionStart:resume` hook frames (`hook_started`, `hook_response`), each carrying the session id.
- No `system/init` arrives until a turn starts `[observed]`. The first prompt's turn brings it, with the same `session_id`, the model and the mode.
- The SDK still answers without a turn `[observed]`:
  - `initializationResult()` in about 0.5 s, with `claude_code_version`;
  - `getContextUsage({detail: 'summary'})` in about 60 ms, with the session's `model` (`claude-opus-5-5`);
  - `mcpServerStatus()`.

  Neither the initialize reply nor anything else reports the permission mode. An SDK `SessionStart` callback registered in `options.hooks` was not called on resume.
- MCP servers connect with no turn `[observed]`. The passed fixture server was `connected` with its two tools within about 3 s. The operator's user-scope servers connected too, and the claude.ai connectors went from `pending` to `connected` or `needs-auth`. They stayed connected at the end of the hold.
- An idle `setPermissionMode('acceptEdits')` is confirmed in about 5 ms, and a `system/status` frame with `permissionMode: acceptEdits` follows `[observed]`. The next turn's `system/init` reports `acceptEdits`.
- An idle resume writes nothing to the transcript `[observed]`. The size and line count were unchanged through the hold; only the mtime moved, by under a second, at the resume.
- Closing input on a never-prompted resumed session ends it promptly `[observed]`: the stream ended and every process was gone within about 0.3 to 0.5 s, with no message after the close. On exit the CLI appended two bookkeeping lines to the transcript, `mode` and `cost-state`, and no message. With a prompt answered first, the close behaved the same.
- Idle memory, sampled at 5, 30, 60, 120 and 300 s `[observed]`:
  - the CLI: about 220 to 235 MB RSS, 145 to 180 MB PSS, drifting down over the hold;
  - the probe's own Node process, a stand-in for the shim: about 81 MB RSS, 47 to 53 MB PSS;
  - each MCP server is its own process: here `lore` about 10 MB PSS, `terminus` about 8 MB, the Node fixture server 14 to 20 MB.

  One prompt took the CLI to about 248 MB RSS, 172 MB PSS.

### codex-cli 0.166.0

`probes/codex/idle-resume.mjs`, on `codex-rt1`. Logs are `probes/codex/logs/idle-resume.*`. One app-server seeded the thread and exited. A second app-server sent `thread/resume` and no `turn/start`, and held for 300 s.

- `thread/resume` answered in about 0.6 s with the model, effort, approval policy and CLI version, and the thread `idle` with its one prior turn `[observed]`. This agrees with X4 and the earlier idle row.
- MCP startup is reported with no turn: `mcpServer/startupStatus/updated` for each configured server within about 0.6 s `[observed]`. `thread/status/changed` (idle), `thread/tokenUsage/updated`, `thread/goal/cleared` and `account/updated` follow. Nothing more arrived during the hold.
- An idle resume writes nothing to the rollout `[observed]`: size and mtime unchanged through the hold and after the exit.
- Closing stdin ends the app-server at once: exit code 0 in about 20 ms, every child gone with it `[observed]`.
- Idle memory, sampled at 5, 30, 60, 120 and 300 s `[observed]`:
  - the app-server: about 235 to 290 MB RSS, 165 to 215 MB PSS, peaking near a minute;
  - its MCP servers and plugins, each its own process: 5 to 28 MB PSS apiece (`lore`, `codebase-memory-mcp`, `terminus`, the Node fixture server, a computer-use plugin's Node and its REPL), about 70 MB together here.

### What this permits

An idle runtime can be held for either harness. It is started by exact id, costs no turn, and writes no message. Its MCP servers connect, and it takes a mode, model or effort change at once. Each idle runtime costs about 200 to 250 MB PSS with its MCP servers, depending on how many servers the operator configures. A handful is reasonable on `fortyfive`; twenty would be several gigabytes, which is why Start is a gesture per conversation and nothing starts on its own. Claude reports no permission mode until a turn, so mega shows the launch mode as requested until Claude's first report (Michael's choice, 2026-11-10).

## Follow-up (2026-11-11): a moved checkout and its Claude session store

Whether a Claude session resumes by exact id from a checkout's new path once the session store directory is moved to the name that path maps to. This rehearses the cutover, which moves this project's checkout and its store together. Claude Code 2.1.318, Agent SDK 0.3.318.

`probes/claude/moved-store.mjs`; the logs are `probes/claude/logs/moved-store.*`. The `claude-rt1` fixture was copied to a scratch path. Its store directory (`~/.claude/projects/` plus the fixture path with every non-alphanumeric character turned into `-`) was copied, read-only on the original, under a staging name. The copy was then renamed to the name the scratch path maps to. Session `9313f4e0-…`, from the idle-resume probe, had two replies, `SEEDED` and `RESUMED`, and every line recorded the old `cwd`. The probe's own store and checkout copies were deleted afterwards. The fixture's store was unchanged, and its transcript hashed the same before and after.

- From the new path, `listSessions` lists the session, and `getSessionMessages` returns its four messages with both replies `[observed]`.
- A resume by exact id from the new path, with streaming input and tools disallowed, reported `system/init` with the same `session_id` and the new `cwd` `[observed]`. Asked for its earlier replies, it answered `SEEDED, RESUMED, MOVED`. The history was intact through the model, not only on disk.
- The transcript grew in the moved store: 45 lines before, 65 after. The new lines record the new `cwd`, and no other store directory was created `[observed]`. The old lines keep the old `cwd`; nothing rewrote them, and nothing needed to.

So moving the store directory to the new path's name, with the service stopped and no terminal open, carries the sessions across. mega's import (`getSessionMessages` with the checkout as `dir`) and its resume by id (`--resume` in the new `cwd`) both work from the new path.

## Recommendations

- Early versions rely on Michael's operator-care decision: close the previous writer before resuming a session elsewhere. Claude interactive sessions and pi do not refuse a second writer. mega should track its own managed processes, but its registry does not prove that an independently launched native process is absent. Stronger exclusion remains deferred, not an implementation requirement.
- Treat "switch to terminal" as a release: stop the managed process, let the terminal resume by exact id, and re-adopt only after that terminal process has exited. Idle is not closed. Only Codex (shared app-server) and Claude background sessions support true live attach, and only Codex exposes it structurally.
- Explicit signals to surface: Claude `canUseTool` (question vs other tool); Codex `activeFlags`, server requests, and async `agentMessage` items carrying `questions[]`; pi `extension_ui_request` (generic dialogs, not necessarily permissions). The follow-up demonstrated answering Codex's async question with `turn/steer` while its turn remained active. Ordinary prose questions remain unread activity unless separately classified.
- Reconnect requires owner-side output replay and a current pending-request set, including resolution handling. The follow-up demonstrated these with an in-memory relay while the owner stayed alive; it did not establish owner-crash or power-loss durability.
- Idle suspension and exact-session reopening without new model work passed on all three. Claude can expose stored history without a running harness until the next prompt. For pending requests, use the harness-specific cancellation sequence described in the follow-up.
- Do not equate an interrupted turn or server exit with all tool execution having stopped: the Codex follow-up observed a surviving sandboxed child. Waiting for natural completion and interrupting have different implications; report remaining or uncertain execution rather than asserting complete suspension.
- Expect unobserved work after an owner crash on Claude; reconcile from the transcript on restart rather than assuming the last seen message is the last message.
- Keep fixtures free of bare `sleep`; Claude blocks it as a standalone command.

## Remaining uncertainties and deferred investigations

The follow-up resolved the earlier browser-disconnect, pi dialog-timeout, and Codex steering questions. These are no longer pending experiments. It also found no supported way to enable blocking `requestUserInput` through the installed Codex protocol; revisit that limitation when the protocol changes, rather than assuming an untested mode option exists.

Remaining uncertainties:

1. Owner-crash recovery of pending requests and whole-machine power-loss durability. The reconnect relay was in-memory; client reconnect while the owner survives does not answer either question.
2. Complete cleanup of interrupted execution, especially the surviving Codex sandbox child. The observations establish the hazard, not a general process-containment solution.
3. Claude supported multi-client attachment and branch selection after concurrent writes. Neither is needed for the initial cold-handoff path with operator care. Do not depend on undocumented daemon sockets or repeat concurrent-writer experiments as ordinary operation.
4. Codex shared-daemon behavior for a normal TUI without `--remote`. The tested shared app-server plus remote TUI path works; behavior of the operator's existing daemon was not established.

These are boundaries and possible later investigations, not a request for another broad probe round.

## Handoff

What works: exact-id round trips between structured and native terminal modes on all three harnesses, with history intact and no fork when the previous writer process has exited (an idle terminal is still a writer); owner-backed client reconnect with missed-output replay and a current pending-request snapshot; idle suspension and reopening without new model work; Codex async-question answers through `turn/steer`. Native-history recovery after the specific owner/harness termination tests also worked, with the differing outcomes documented above.

What requires care: Claude and pi can silently branch a transcript when independently opened by concurrent writers. Early versions rely on operator handoff discipline. Claude needs interruption or denial before promptly shutting down a pending permission request. Pi has no built-in permission model; its extension dialogs can block abort, but protocol cancellation, explicit dialog timeouts, and stdin shutdown were exercised. Codex interruption can leave a sandboxed child alive even after the server exits.

What remains unavailable or unproven: supported Claude SDK attachment to a running native session; Codex blocking user-input mode through the installed protocol; durable pending-request recovery after an owner crash; whole-machine power-loss recovery; general cleanup of all child/background execution. See Remaining uncertainties for the narrower deferred investigations.

Terminal steps completed by Michael: Claude resume and native-origin session, Claude attach to a background session, the Claude split test, Codex TUI resume and remote TUI, pi TUI resume with the approval dialog. Fixtures remain under `/home/michael/Repos/q/research/roundhouse-fixtures/`; test sessions remain in each harness's session store and can be purged (`claude purge <path>` exists for Claude). Everything is uncommitted.

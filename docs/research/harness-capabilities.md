# Harness capabilities: Claude Code, Codex, pi

Evidence-backed findings from the Roundhouse integration investigation (brief: `docs/future/agent-workspace-investigation.md`). Each scenario reports a result (`pass`, `partial`, `fail`, `blocked`), observed behavior, continuity, reproduction, and sanitized excerpts. Statements are tagged `[observed]`, `[docs]` (documentation claim, not verified here), or `[inference]`.

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

`getSessionMessages` returned the terminal branch; a subsequent SDK `resume` continued the SDK branch (`... LIVE-PROBE-gg77, AFTER-SPLIT-SDK-ii99`) `[observed]`. Interactive sessions do not lock the transcript against a second writer, and readers choose branches differently `[inference: the resume appears to follow the latest `last-prompt` leaf, which the still-open terminal had not yet written]`. Roundhouse must treat "is a live process holding this session" as a precondition it checks itself (`claude agents --json`), not something the CLI enforces for interactive sessions.

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
- R4 `--kill-owner-after-ms 10000`: the SDK host was SIGKILLed while a backgrounded `sleep 30` ran. The CLI did not exit: it stayed alive about 40 s, received the task notification, ran a further model turn (`OWNER-KILL-1`) that nobody consumed, persisted it, and then exited. A later SDK resume recited `ROUNDHOUSE-MARKER-7f3a / OWNER-KILL-1 / RECOVERED-ff66`. Terminating the owner therefore neither stops the harness promptly nor loses history, but it does allow unobserved model turns.
- Long hold: a `Bash(sleep 2)` permission request held for 150 s (`--approve-delay-ms 150000`) was still accepted; the tool ran and the turn completed (`LONG-HOLD-mm22`). No timeout or park message appeared on the stream in that window `[observed]`.
- Fixture note: the installed Bash tool blocks bare `sleep N` (`Blocked: standalone sleep 40. To wait for a condition, use Monitor…`); the model then reran it with `run_in_background: true`, and closing the SDK input while that task ran did not end the process until the task completed and a follow-up turn had run (E1 in the logs).

### Claude answers to the report questions

1. A faithful conversation surface without terminal parsing: yes `[observed]`. The stream carries user, assistant, tool use, tool result, question, permission and result events with ids; the on-disk transcript holds the same chain for history.
2. Signals: working = messages flowing after a prompt (no dedicated state event was emitted); explicit input required = `canUseTool` with `AskUserQuestion`; permission required = any other `canUseTool`; turn ended = `result`; failed = `result.is_error` with `subtype`; unknown = a process alive with no `result` and no pending request. A prose question is an ordinary assistant text with `stop_reason: end_turn`; the protocol does not distinguish it `[observed]`.
3. Cross-mode continuity in both directions: yes for the same id when the other side is not running `[observed]`. No history replacement, no fork, `entrypoint` records who wrote each turn.
4. Changes across the transition: trust state and project permission rules, auto-memory, permission mode; tools, instructions and MCP servers were equivalent; queued input and background tasks do not transfer (a background task finished inside the orphaned process in R4).
5. Live attachment: the native terminal can attach to a background session (`claude --bg` + `claude attach`), and the SDK is refused while it runs; the SDK cannot attach to a running terminal or background process through a supported interface. Supervising a `--bg` session structurally would require the undocumented daemon sockets, so `[blocked]` as a supported path. Version: 2.1.318.
6. Independently started terminal sessions: discovery `pass` (`listSessions`, `claude agents --json`), stored-history access `pass`, cold resumption `pass`, live control `fail` (no supported channel, and a hot resume forks the chain).
7. Multiple clients: one SDK host per CLI process was tested; the protocol declares multi-client semantics for hosted sessions `[docs]`, untested locally.
8. Recoverable from native history: everything persisted up to the last `result`, including turns completed by an orphaned CLI. Lost or uncertain: in-flight approvals (cancelled on interrupt), background task state, and which branch a reader will follow after a concurrent write.
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

Approval requests are therefore broadcast to every connected client, the first answer wins, duplicates are dropped silently, and the native TUI is a peer client on the same thread. Codex offers no exclusive input ownership; Roundhouse would impose it `[inference from X8/X9]`.

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
| R3 | Dialog never answered | fail-open hang | run blocks indefinitely (no `timeout` set); stdin close still exits cleanly |
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
3. Cross-mode continuity: yes in both directions when the other side is not running.
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

## Recommendations

- Roundhouse's machine-local service must own session liveness itself: check for live processes before any resume (Claude: `claude agents --json`; pi: no signal, so Roundhouse's own registry) because Claude interactive sessions and pi do not refuse a second writer.
- Treat "switch to terminal" as a release: stop the managed process, let the terminal resume by exact id, and re-adopt afterwards. Only Codex (shared app-server) and Claude background sessions support true live attach, and only Codex exposes it structurally.
- Explicit signals to surface: Claude `canUseTool` (question vs other tool), Codex `activeFlags` + server requests, pi `extension_ui_request`. Everything else is "unread activity".
- Expect unobserved work after an owner crash on Claude; reconcile from the transcript on restart rather than assuming the last seen message is the last message.
- Keep fixtures free of bare `sleep`; Claude blocks it as a standalone command.

## Open questions and smallest next experiments

1. Claude multi-client: `pending_permission_requests` on initialize and the initialize re-send exist in the SDK types. Experiment: run two SDK hosts against one background (`--bg`) session only if a supported transport appears; otherwise treat as unavailable.
2. Claude branch selection after a split: confirm which leaf `--resume` follows by writing a `last-prompt` entry from the terminal and resuming again. Experiment: repeat C9 with the terminal exited cleanly first.
3. Codex `turn/steer` into a sleeping async-question turn, and whether a non-default collaboration mode enables blocking `requestUserInput`. Experiment: start a thread with the collaboration mode option from the schema and repeat X3.
4. Codex shared daemon: whether `codex agents` / the daemon exposes the same multi-client semantics to the normal TUI without `--remote`. Experiment: read its config, do not restart it.
5. pi dialog timeouts: whether a `timeout` on `ctx.ui.select` auto-resolves in RPC mode and what the run does afterwards.
6. Browser-equivalent disconnect and central-client restart were not run on any harness; the local owner held every connection throughout. Experiment: a loopback shim between probe and harness that buffers events and replays pending requests on reconnect.

## Handoff

What works: exact-id round trips between structured and native terminal modes on all three harnesses, with history intact and no fork when the other side is idle; structured approvals and interrupts on Claude and Codex; recovery from native history after owner or harness termination on all three.

What does not: a second writer on a live Claude interactive session or a live pi session forks the transcript silently; Codex blocking user-input requests are unavailable in default mode; pi has no permission model of its own and a dialog can hang a run forever; Claude's SDK cannot attach to a running terminal or background session.

What remains uncertain: Claude multi-client and branch selection after a split, Codex non-default collaboration modes and the shared daemon, pi dialog timeouts, and all browser-disconnect scenarios.

Terminal steps completed by Michael: Claude resume and native-origin session, Claude attach to a background session, the Claude split test, Codex TUI resume and remote TUI, pi TUI resume with the approval dialog. Fixtures remain under `/home/michael/Repos/q/research/roundhouse-fixtures/`; test sessions remain in each harness's session store and can be purged (`claude purge <path>` exists for Claude). Everything is uncommitted.

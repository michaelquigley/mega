# Roundhouse as built

This describes what exists now: the whole first slice, whose work order was retired when its last stage gated (git history holds it; the spec is `docs/future/roundhouse.md`). That is the environment service with projects; Claude Code and Codex conversations launched, prompted, answered, interrupted, released for a terminal, and resumed; native sessions discovered and adopted; and attention, with a needs-you list, reading positions, and a next-waiting shortcut.

## Shape of the code

One Go binary, `roundhouse`, in the module `git.hq.quigley.com/research/roundhouse`, with `github.com/michaelquigley/df/dl` for logging and `df/dd` for YAML and JSON. Both the product name and the module path are provisional: the repository moves to the `products/` tree when the name settles, so the name stays out of data and identifiers, except the binary and the XDG directory names, which are a single-constant rename. TypeScript appears only in the Claude shim and the UI.

```text
cmd/roundhouse/          cobra root; serve (the default) and version
internal/config/         roundhouse.yaml; defaults; path expansion
internal/model/          Project, Conversation, Event, Request, attention states; no I/O, no protocol
internal/registry/       registry.yaml, ids, the synced atomic write, the data-directory lock
internal/events/         the per-conversation event log
internal/durable/        SyncDir and a parent-syncing MkdirAll
internal/harness/        the Harness, Runtime and Sink interfaces, and a fake for tests
internal/harness/claude/ the shim process, its protocol, mapping, history, discovery, trust
internal/harness/codex/  the app-server child, JSON-RPC, mapping, questions, history, discovery
internal/service/        the single authority: registry, logs, runtimes, pending sets, attention
internal/server/         the HTTP API, the stream, the embedded UI
shim/claude/             the Node shim, its pinned SDK, and its test
ui/                      the Vite, React and TypeScript UI, with no state library and plain CSS
```

Dependencies point inward. `server` depends on `service`; `service` on `harness`, `registry`, `events` and `model`; the adapters on `model` and `harness` only; `registry` and `events` on `durable`; `model` on nothing. Everything protocol-shaped (SDK messages, JSON-RPC methods, the shim's frames) lives inside its adapter package or the shim.

## Running it

`make` builds the UI, installs the Claude shim's pinned dependencies (`npm ci` in `shim/claude`), and installs the `roundhouse` binary into the project's `GOBIN`, stamped with the shim's location in the checkout. `roundhouse` (or `roundhouse serve`) runs the service; `roundhouse version` prints build information. `make clean` removes only what the project builds: `ui/dist`, `ui/node_modules`, `shim/claude/node_modules`, and the binaries in the project's `GOBIN`. `--config PATH`, `--data DIR` and `--verbose` apply to both forms.

| Item | Default | Override |
| --- | --- | --- |
| Config file | `$XDG_CONFIG_HOME/roundhouse/roundhouse.yaml` (`~/.config/...`), optional | `--config`, which must exist |
| Data directory | `$XDG_DATA_HOME/roundhouse/` (`~/.local/share/...`) | `data` in the config, then `--data` |
| Listener | `127.0.0.1:8470` | `listen` in the config |
| Node | `node` on `PATH` | `claude.node` |
| Claude shim | `shim/claude` in the checkout `make` ran in | `claude.shim` |
| Claude Code | `~/.local/share/claude/versions/2.1.318` | `claude.executable` |
| Codex | `codex` on `PATH` | `codex.binary` |

```yaml
listen: 127.0.0.1:8470
data: ~/.local/share/roundhouse
claude:
  node: node
  shim: ~/Repos/q/research/roundhouse/shim/claude
  executable: ~/.local/share/claude/versions/2.1.318
codex:
  binary: codex
```

The executable defaults to 2.1.318 because the shim pins Agent SDK 0.3.318, which was matched against that build; `claude` on `PATH` may be newer.

`listen` must be a loopback address; the service refuses to start otherwise, because this slice has no login. The service binds its port before reading any data, so a second instance on the same port exits without writing. It then takes an exclusive hold on `<data>/lock` before loading the registry, so a second instance on another port that shares the data directory also exits, and the directory has one writer. The hold is released by the kernel when the process exits; the file itself stays.

## Data

The data directory holds `registry.yaml`, the authority for projects and conversations, and `conversations/<id>/events.jsonl`, one append-only event history per conversation. Only the service writes either. Both are private to the operator (mode `0600`, directories `0700`).

The registry is plain YAML, readable with the service stopped. It is written to a temporary file, synced, and renamed, and the service's in-memory state only advances once that succeeds, so what a browser sees is what a restart would read. A registry with an unknown `version` is refused rather than guessed at. If the directory sync after the rename fails, the change was never confirmed: memory keeps the last confirmed document and reads continue, the condition is logged once at error level, nothing is announced to browsers, and every later change is refused as `unavailable` with `registry durability uncertain after a failed directory sync, restart required`. The file may hold that one unconfirmed change until a restart reads whatever reached the disk. A failure before the rename changes nothing and can be retried. Every directory the service creates under the data directory, including the data directory itself, has its parent synced, so a new workspace or conversation cannot vanish in a crash while its synced contents would have survived.

An event log assigns each event the next `seq`. An append returns only after the line is synced, and a failed write or sync is truncated back with the `seq` unchanged, so an event a browser has been sent survives a crash. If that truncate also fails, the log keeps serving every confirmed event, since reads never go past the confirmed length, but refuses every later append with `event log durability uncertain after a failed rollback, restart required`, logged once at error level. The leftover bytes stay on disk until a restart reopens the log, where an unterminated remainder is dropped as a torn tail. Creating a log also syncs the directory that names it. Only completed events are logged; streaming text is never written, so this costs a few syncs per turn. On open, an unterminated final line is a write torn by a crash and is dropped; any other unreadable line stops the open and leaves the file alone.

An event is `{seq, at, kind, origin, text, native_id, status, tool, tool_use_id, req, facts, raw}`. `kind` is `runtime` (with `status` starting, started, not_started, stopping, stopped, unsupervised, resuming, resumed or error; a `started` event carries the runtime's `facts`: session, model, version, permission mode, cwd), `user`, `assistant`, `tool_use`, `tool_result`, `request`, `request_resolved`, `turn_end` (`status` success, error or interrupted), or `native` (anything else the harness emits, kept in `raw`). `origin` is `managed` for what a runtime managed by the service produced, `native` for what an import read from the harness's own history, and `service` for the service's own bookkeeping. `native_id` is the harness's identity for the message an event came from; an event whose `native_id` the log already holds is never recorded again, which is how an import adds only what is missing.

## Conversations and runtimes

A conversation is bound to one native session. Launching starts a runtime for a new session, records the session id, and sends the opening prompt. A conversation holds exactly one native session for its whole life. A harness may only reveal the id after launch (Codex always does), so a session the conversation created is held in memory until the registry has it, and is seeded from the log's last `started` event when a conversation is opened. The next launch saves it in the same registry write that records the runtime running, and resumes it; if that write fails, the launch is refused. A fresh session starts only on proof that none exists: the log shows no earlier launch, or every earlier `starting` was followed by a recorded `not_started`, the status of a launch that failed before any session could exist. Only the adapter can know that, so a launch error records `not_started` only when it wraps `harness.ErrNotStarted`, which each adapter applies at the failure sites it can prove came first: for Claude, failing to start the shim; for Codex, failing to start the app-server, a failed `initialize`, or an explicit JSON-RPC error reply to `thread/start`. The service's own failures before any process starts (the registry write, an unknown harness) also record `not_started`. Anything unclassified, such as a timeout or a lost connection while `thread/start` was outstanding, records `error` and counts as session unknown. Any other conversation with no known session refuses prompts and says so, pointing at "Adopt a session" to pick the orphan up into a new conversation; a resume does not lift that. The accepted cost is a conversation whose launch failure itself could not be recorded: it stays refused although no session exists, which is the closed side of the failure. Every later runtime resumes exactly that id; nothing picks a session by recency, forks one, or starts a fresh one in its place. A runtime that reports a different session than the one it was launched for is recorded as an error and stopped.

Runtimes are long-lived: a runtime starts at launch, or with the first prompt after an adoption, a release, a clean restart or a resume, and stays up between turns until the operator releases it, the service stops, or the harness exits by itself. There is no per-turn mode and no suspension operation; restarting the service is the way to put everything down. The service never looks for or kills processes it did not start, or ones it lost track of: an orphaned runtime after an unclean restart, or a sandboxed command that outlived Codex, is reported, not stopped.

The registry records each conversation's `runtime` as the service last confirmed it: `none`, `running` (written before the process starts), or `unsupervised`. The service also reports transient states: `starting` until the runtime reports the session it holds, `stopping`, and `resuming` while it reads native history.

Every change to the displayed runtime state is a recorded `runtime` event, and the change is shown and announced only once that event is recorded. If the log cannot record it, the last confirmed state stays on screen. Whatever the change does to the process happens anyway: a release, a shutdown, or the stop of a runtime bound to the wrong session still stops the process, because a storage fault must never trap a running agent, and the next start settles what the log could not record. There are no exceptions: a launch that failed shows no runtime once its `not_started` event is recorded, and if the log cannot record it, the last confirmed `starting` stays on screen, the conversation stays recorded `running` so a restart settles it, and the log's refusal of later writes (`unavailable`, restart required) is what tells the operator. Operations are guarded internally, apart from what is displayed, so a start or a resume that ended unrecorded never blocks the next one, and a runtime is asked to stop at most once, however many releases or shutdowns ask.

- **Prompt** records the text as a `user` event with a service-minted uuid and passes that uuid to the harness, which records it as the message's identity (observed with Agent SDK 0.3.318), so an import recognizes the prompt. One turn at a time: a prompt while a turn is in flight is refused. A prompt to a conversation with no runtime first imports native history, then resumes the session by its id.
- **Requests** (a permission or an `AskUserQuestion`) get a service-minted id, unique across every runtime the conversation has had, and join the conversation's pending set, which lives only in memory. The harness's own request id stays inside the adapter, so a stale card can never match a newer request that reused a native number (Codex's restart at 0 for every app-server). A request ends `answered`, `aborted` (the harness settled it, or its runtime exited), or `lost` (the service restarted while it was pending). An answer takes the request out of the pending set before it is delivered, so the first answer from any browser wins and every later one gets `not_pending`. An answer the harness has already settled, for example on an interrupt, also gets `not_pending`. A denial without a reason is sent as `the operator denied this in roundhouse`.
- **Interrupt** stops the active turn; the harness aborts any pending request.
- **Release** is the terminal handoff. If a turn or a request is outstanding it interrupts first, then closes input, and the conversation shows `stopping` until the process is seen to exit, when it records `none`. Claude Code 2.1.318 exits with status 1 when input closes after an interrupted turn; a requested release with no turn in flight is still recorded as `stopped`, with the status in the text. An exit nobody asked for, or one in the middle of a turn, is recorded as `error`.
- **Resume** imports native history, appending what the log lacks (for example turns made in the terminal) as `native`-origin events, records `resumed` with how many it added, and clears `unsupervised`. It starts no runtime; the next prompt does. A resume whose `resuming` event cannot be recorded is refused before anything is read.
- **Shutdown** (SIGINT or SIGTERM) releases every runtime the same way and waits up to 30 seconds for each to exit. Each that exits records `none`; any still running stays `running`.
- **Start** settles the previous run without replaying anything: every request the log shows unresolved gets `request_resolved` with outcome `lost`, and every conversation still recorded `running` gets a `runtime unsupervised` event and becomes `unsupervised`, because its runtime was never seen to exit and may still be working. Its history is not imported until the operator resumes it, and prompts are refused until then.

A project with a conversation that has a runtime cannot be removed until that runtime is released.

## Attention

Attention is computed from confirmed facts every time the workspace is read, and never stored. In order:

- `request` when a question or approval is pending;
- `unsupervised` when the service restarted without seeing the runtime exit, an uncertain state never shown as running or idle;
- `running` while a turn is in flight;
- `error` when the last runtime ended badly (an error, an exit during a turn, or a launch that did not start), until a runtime starts or stops cleanly;
- `unread` when `output_seq` is past `read_seq`;
- `idle` otherwise.

`output_seq` advances only on agent output (`assistant`, `tool_use`, `tool_result`, and the harness's own `native` events) recorded during a turn, and only when that turn ends; output still being produced is `running`. Runtime and request bookkeeping and recovery never advance it. Imported history counts by how the import happened. Resuming after an unclean restart advances `output_seq` to the newest agent output anywhere in the log, cached or imported, because the orphaned runtime may have finished turns nobody saw, including the one it was in when the service died; the reading position then decides whether that output is unread. Output of a turn the harness reports still in progress is left out: `harness.History` names those events, and Codex marks the items of an `inProgress` turn in `thread/read`. Claude's history carries no turn status, so it counts as complete, and resuming beside a live orphan is the operator's hazard the resume text already warns about. Resuming after a release, and adopting, leave `output_seq` where it is, because the operator was the one in the terminal or chose the session. So reading a conversation and then releasing it, restarting the service cleanly, or resuming after a terminal visit all leave it `idle`.

`last_seq` is read from the event log, never stored, so recording an event never rewrites the registry. `read_seq` is the reading position. Reading moves it forward to at most the last event, never back. It acknowledges what was read and nothing more: a pending request stays pending, and `request`, until it is answered. Both registry writes, `output_seq` at a turn's end and `read_seq` on reading, follow the registry's rule that memory moves only after the write is confirmed.

The needs-you order lists every conversation in `request` or `unread`, requests first, then each group oldest first by the time of the fact that put it there: the oldest pending request, or the unread output.

## Discovery and adoption

Discovery lists the native sessions stored for a project's checkout that no conversation holds yet: Claude through the shim's `list` (the SDK's `listSessions`), Codex through `thread/list` on a short-lived app-server. It never chooses one. Adoption binds the session the operator picked, which must be one discovery lists, to a new conversation, imports its history as `native`-origin events between `resuming` and `resumed` runtime events, and starts no runtime; the first prompt resumes the session by its id. Close any terminal that holds the session first; an idle terminal is still a writer.

## The Codex adapter

Each runtime is a private `codex app-server` child per conversation, driven over stdio JSON-RPC in its own process group, so its lifecycle matches Claude's (a runtime is a child process) and a crash stays local. It starts with `initialize` (client `roundhouse`, `experimentalApi` on) and `initialized`, then `thread/start {cwd}` or `thread/resume {threadId, cwd}`. The Go types are written by hand for the subset used, from the 0.166.0 schema checked in under `internal/harness/codex/schema/` with the command that generates it. A turn is in flight from `turn/started` (or the `turn/start` response) to `turn/completed`, whose status becomes the `turn_end` status. It never sends an approval policy, a sandbox, or a config override, so the operator's codex configuration decides them. That includes per-project trust: a trusted checkout gets a workspace-write sandbox, where only a command reaching outside the checkout asks for approval. The runtime reports `started` once the thread starts, with the model, the CLI version, the approval policy and sandbox, and the instruction sources.

- Completed items map to events: a user message to `user` (its `clientId`, the service's prompt identity passed as `clientUserMessageId`, becomes its native identity, so an import recognizes it); an agent message to `assistant`; a shell command, a file change or an MCP call to `tool_use` and `tool_result`; anything else to `native`. Streaming agent text becomes deltas. Other notifications are transport chatter and are not recorded, except `error`.
- Command, file-change and permissions approvals become `permission` requests, mapped by the server's own request id inside the adapter. Allow answers `accept`, deny answers `decline`; a permissions request is granted exactly what it asked for, for the turn, or nothing. `serverRequest/resolved` aborts a request the server settled itself. Codex gives no acknowledgment of an answer, so an answer after that notice is refused as `not_pending`.
- An agent message carrying `questions[]` (the default-mode question, since `requestUserInput` is unavailable in 0.166.0) is also recorded as a `question` request. An answer goes into its turn with `turn/steer` while that turn is active, or starts the next turn if the turn completed with the question unanswered. Interrupt, release, a turn ending `interrupted` or `failed`, and the runtime's exit all abort it.
- `item/tool/requestUserInput`, which 0.166.0 does not send in default mode, is handled if it arrives: it becomes a `question` request answered by question id.
- A server request roundhouse cannot put to the operator is not answered either way on their behalf. A `native` event says so, and the turn waits until the operator interrupts it.
- Interrupt is `turn/interrupt`. Release interrupts if needed, then closes stdin. At each interrupt and close the adapter notes every process running under the app-server, because the sandbox codex runs commands in starts its own session and so leaves the app-server's process group. After the exit it waits up to 10 seconds for those processes to go. If any are still running, the runtime is recorded as an `error` naming their pids, never as a clean stop. Roundhouse does not kill them.

## The Claude shim

`shim/claude/shim.mjs` is a policy-free bridge between the service and `@anthropic-ai/claude-agent-sdk` 0.3.318, speaking one JSON object per line on stdio. The service runs `node shim.mjs run --cwd DIR (--session-id UUID | --resume UUID) --exec PATH`, with the executable from `claude.executable`. The SDK's optional `claude-agent-sdk-linux-x64` package, which `npm ci` installs, is not used.

```text
shim to service
  {"type":"event","msg":<sdk message verbatim>}
  {"type":"request","id":<requestId>,"kind":"permission"|"question","tool":...,"input":{...},"toolUseId":...}
  {"type":"request_resolved","id":...,"outcome":"aborted"}
  {"type":"answer_accepted","id":...}      the answer reached the waiting request
  {"type":"answer_rejected","id":...}      nothing was waiting under that id
  {"type":"interrupted","response":{...}}
  {"type":"error","message":...}
  {"type":"exit","code":N}
service to shim
  {"type":"prompt","uuid":<service-minted message uuid>,"text":...}
  {"type":"answer","id":...,"decision":"allow"|"deny","message":...}
  {"type":"answer","id":...,"answers":{"<question>":"<label>"}}
  {"type":"interrupt"}
  {"type":"close"}
```

The adapter maps the first `system/init` to `runtime started`, `assistant` messages to `assistant` and `tool_use`, user tool results to `tool_result`, `result` to `turn_end` (`interrupted` if an interrupt was sent in that turn), streaming text to deltas, and everything else to `native`; a subagent's messages are `native` too. `AskUserQuestion` is a `question` request, and every other `canUseTool` a `permission` request. A backgrounded tool keeps the CLI alive after `close` until it finishes, so release has no deadline and shows `stopping` until the exit. `run` starts or resumes a session with `permissionMode: 'default'` and default setting sources, so every approval reaches the operator and the checkout's instructions, hooks and permission rules load as in a terminal. Each request waits for the service's answer, and the shim confirms every answer as accepted or rejected, so an answer that lost a race with an abort is reported `not_pending` rather than as answered. A request becomes answerable only once its event is recorded. On `close` it waits for an interrupt in flight before ending input, because ending input with a request pending keeps Claude alive for minutes. `messages` wraps `getSessionMessages` and `list` wraps `listSessions`. The service runs the shim in its own process group, so a terminal interrupt aimed at the service does not cut short the orderly release.

## HTTP surface

Everything is JSON with `snake_case` keys. Errors carry `{code, message}` with `code` one of `invalid` (400), `not_found` (404), `conflict` (409), `not_pending` (409, an answer to a request that is no longer pending), `unavailable` (503, the service cannot make changes durable until it restarts), or `internal` (500, detail logged rather than returned).

| Route | Body | Result |
| --- | --- | --- |
| `GET /api/workspace` | | `{projects, conversations, needs_you}` |
| `POST /api/projects` | `{name, checkout}` | 201 with the project |
| `PATCH /api/projects/{id}` | `{name}` | 200 with the project |
| `DELETE /api/projects/{id}` | | 204 |
| `POST /api/projects/{id}/conversations` | `{harness, name, prompt}` | 201 with the conversation |
| `GET /api/projects/{id}/discover?harness=` | | `{sessions}`, each `{native_id, preview, created_at, updated_at}` |
| `POST /api/projects/{id}/adopt` | `{harness, native_id, name}` | 201 with the conversation |
| `PATCH /api/conversations/{id}` | `{name, label}` | 200 with the conversation |
| `GET /api/conversations/{id}/events?since=N` | | `{events}` after seq `N` |
| `POST /api/conversations/{id}/prompt` | `{text}` | 202 |
| `POST /api/conversations/{id}/answer` | `{request_id, decision, message}` or `{request_id, answers}` | 202 |
| `POST /api/conversations/{id}/interrupt` | | 202 |
| `POST /api/conversations/{id}/release` | | 202 |
| `POST /api/conversations/{id}/resume` | | 202 |
| `POST /api/conversations/{id}/read` | `{seq}` | 202 |
| `WS /api/stream` | | frames below |

A project is `{id, name, checkout, untrusted}`. `checkout` must be an absolute path to an existing directory and may be registered once; an empty `name` becomes the directory name. Removing a project takes it and its conversations out of the workspace and touches nothing else: the checkout, native histories, and stored event logs stay where they are.

A conversation is `{id, project_id, name, label, harness, native_id, mode, state, turn_active, attention, read_seq, last_seq, output_seq, model, version, permission_mode, pending}`, where `pending` is the current pending set as `{id, kind, payload}`. A request's payload is plain data for display: `tool` and `input` (plus `title`, `description`, `decision_reason`, `blocked_path` where Claude gives them) for a permission, `questions` for a question. `harness` is `claude` or `codex`. Accepted commands answer 202, and their effects arrive on the stream.

Every write is a command to the service, the one writer of the registry and the logs; the server holds no state. A launch runs outside the service's lock, because Codex reports its start before the launch call returns; other operations on that conversation stay refused until it settles, and a runtime that exits while launching is settled like any other exit.

Every other path serves the embedded UI, falling back to `index.html`.

The stream carries `{conversation_id, event}` for each recorded event, `{conversation_id, delta}` for streaming text (never stored), `{workspace_changed: true}` after any confirmed change to the workspace (the registry, a runtime's state, a pending set), and `{heartbeat: <time>}` every 20 seconds so a page can see that a quiet stream is alive. It replays nothing: a client that reconnects re-reads confirmed state. A client that falls 256 updates behind is disconnected rather than allowed to slow the service.

Requests are refused with 403 unless their `Host` is loopback and any `Origin` matches that `Host`. The loopback bind keeps the network out; this keeps out web pages in the operator's own browser, which could otherwise reach `127.0.0.1` through DNS rebinding or a cross-site request.

## The trust notice

`untrusted` is computed from `~/.claude.json` on every read and never persisted, so a terminal visit clears it without a restart. It looks up `projects[<key>].hasTrustDialogAccepted`, where the key is the checkout's enclosing git root, or its real path outside a repository. That mirrors the check Claude Code 2.1.318 uses to gate project permission rules; it is an observation of that version, not a documented contract, and a linked worktree may read as untrusted. A missing file reads as untrusted, and an unreadable one shows no notice and logs a warning.

The service only ever reads this file. Trust is granted by the operator opening the checkout in `claude` interactively.

## The UI

A left column with the needs-you list at the top, then the projects with their conversations, each marked with its attention: a square for a request, a filled dot for unread output, a pulsing ring while running, a red ring for an error, red for supervision lost, and a plain ring when idle. Each needs-you entry names its project and opens the conversation inside it. Ctrl+/ moves the selection to the next waiting conversation in that order, and only the selection, so a reply in progress keeps its place; a bare n does the same while focus is outside any input, textarea, select or editable element, so typing never moves it. While a conversation is open, the page is visible and the workspace is current, the UI posts `read` with the conversation's last seq, so opening it acknowledges what is on screen; a pending request keeps its card and its place in the list. The selection lives in the URL hash. A project shows its trust notice, conversation list, rename and remove; one dialog launches a conversation with a harness and an opening prompt, and another lists the checkout's unadopted sessions for either harness and adopts the one picked. A conversation shows its harness, model, version, mode, permission mode, checkout and session id, and can be renamed and labeled from its header; its history with tool activity collapsed and native events hidden behind a toggle; streaming text while a turn runs; an answerable card for each request in the pending set (approvals with allow and deny, questions with their options or a written answer); interrupt, release and resume; the handoff rule with the exact `claude --resume` or `codex resume` command; and a reply box (Ctrl+Enter) whose draft stays in the browser. Requests in the history are only history; a card exists only while the service lists the request as pending.

The UI re-reads the workspace whenever the stream connects or reports a change. Re-reads are numbered and coalesced: at most one is in flight, changes reported meanwhile get one follow-up, and only the newest response sent is applied, so an older response can never bring back a request the service no longer lists. It also fetches the selected conversation's events after the last seq it holds on every reconnect, so two browsers converge on the service's confirmed state. From the moment the stream drops, or a workspace refresh fails, until a refresh sent on the current connection succeeds, runtime state on the page is labeled last known: the header adds `(last known)`, marks dim, and an in-flight tool reads `running when last seen`. A refresh still out when its connection dropped cannot mark a later connection up to date. `make test` builds and lints it; `npm --prefix ui run dev` serves it on port 5173 against a running service.

## Not built, and what to watch

- Refinements named but not built: SQLite if queries outgrow scanning a file (the registry stays a readable file regardless); contract-first generation of the API and its TypeScript types once the surface settles; one shared Codex app-server per environment, which would also allow live `codex --remote` attach.
- Deferred by the spec: pi, per-turn runtimes, suspension, the re-entry briefing, the central application, and more than one environment. The UI has no side-by-side view, no themes, and no mobile layout.
- Each Codex conversation costs an app-server process. That is fine for a handful; the shared server is the answer when it is not.
- A version change shows in the conversation header, which carries the harness version each runtime reported at its start.
- Nothing detects a terminal holding a session. Resuming a Claude session that a terminal still has open forks its transcript silently, so the handoff rule (release, use the terminal, close it, then resume) is the operator's to keep.

## Tests

`make test` runs the shim's protocol test against a fake SDK, the Go unit tests, and the UI lint and build. The service is tested against a fake harness whose request numbers restart in every runtime; the Codex adapter against hand-written JSON-RPC frames over an in-memory wire. The Claude trust tests put their checkouts under the user cache directory, because codex 0.166.0 creates an empty `/tmp/.git` while any of its sandboxed commands runs on the machine, which would make every checkout under `/tmp` resolve to it as a git root.

`make e2e` runs `internal/e2e` with the real harnesses and the operator's logins, and is run by hand:

- Claude, against `claude-rt1`: a marker turn, a question, a delayed approval, release, an import that must add nothing, and a resumed turn that recites every marker.
- Codex, against `codex-rt1`: discovery and history with no model turn; then a marker turn, a delayed approval for a write outside the checkout, the async question steered into its turn, a clean release, an import that must add nothing, and a resumed turn; and a release during a running command, which must report what the sandbox left running.

Each Claude run costs a few model turns, and the Codex runs together about five.
